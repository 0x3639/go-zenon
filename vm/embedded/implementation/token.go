package implementation

import (
	"math/big"
	"regexp"

	"github.com/zenon-network/go-zenon/chain/nom"
	"github.com/zenon-network/go-zenon/common"
	"github.com/zenon-network/go-zenon/common/types"
	"github.com/zenon-network/go-zenon/vm/constants"
	"github.com/zenon-network/go-zenon/vm/embedded/definition"
	"github.com/zenon-network/go-zenon/vm/vm_context"
)

// tokenLog is the per-contract logger; tagged with `contract=token`.
var (
	tokenLog = common.EmbeddedLogger.New("contract", "token")
)

// IssueMethod implements ZTS-token issuance: charges
// [constants.TokenIssueAmount] ZNN as a fee, registers a new
// [definition.TokenInfo] keyed by a hash of the originating send,
// and emits a descendant transfer of the initial total supply to
// the issuer.
type IssueMethod struct {
	MethodName string
}

// checkToken validates the issuance parameters: name length and
// charset, symbol charset (uppercase A-Z + digits, no `ZNN`/`QSR`
// reservation), domain shape, decimals, and the
// supply / max-supply / mintability invariants.
func checkToken(param definition.IssueParam) error {
	// Valid names
	if len(param.TokenName) == 0 || len(param.TokenName) > constants.TokenNameLengthMax {
		return constants.ErrTokenInvalidText
	}
	if len(param.TokenSymbol) == 0 || len(param.TokenSymbol) > constants.TokenSymbolLengthMax {
		return constants.ErrTokenInvalidText
	}
	if len(param.TokenDomain) > constants.TokenDomainLengthMax {
		return constants.ErrTokenInvalidText
	}

	if ok, _ := regexp.MatchString("^([a-zA-Z0-9]+[-._]?)*[a-zA-Z0-9]$", param.TokenName); !ok {
		return constants.ErrTokenInvalidText
	}
	if ok, _ := regexp.MatchString("^[A-Z0-9]+$", param.TokenSymbol); !ok {
		return constants.ErrTokenInvalidText
	}
	if ok, _ := regexp.MatchString("^([A-Za-z0-9][A-Za-z0-9-]{0,61}[A-Za-z0-9]\\.)+[A-Za-z]{2,}$", param.TokenDomain); !ok && len(param.TokenDomain) != 0 {
		return constants.ErrTokenInvalidText
	}

	if param.TokenSymbol == "ZNN" || param.TokenSymbol == "QSR" {
		return constants.ErrTokenInvalidText
	}

	if param.Decimals > uint8(constants.TokenMaxDecimals) {
		return constants.ErrTokenInvalidText
	}

	// 0 or too big
	if param.MaxSupply.Cmp(constants.TokenMaxSupplyBig) > 0 {
		return constants.ErrTokenInvalidAmount
	}
	if param.MaxSupply.Cmp(common.Big0) == 0 {
		return constants.ErrTokenInvalidAmount
	}

	// total supply is less and equal in case of non-mintable coins
	if param.MaxSupply.Cmp(param.TotalSupply) == -1 {
		return constants.ErrTokenInvalidAmount
	}
	if !param.IsMintable && param.MaxSupply.Cmp(param.TotalSupply) != 0 {
		return constants.ErrTokenInvalidAmount
	}
	return nil
}

// newTokenID derives a deterministic [types.ZenonTokenStandard]
// from the originating send-block hash. Different sends produce
// different IDs.
func newTokenID(sendBlockHash types.Hash) types.ZenonTokenStandard {
	return types.NewZenonTokenStandard(sendBlockHash.Bytes())
}

// GetPlasma returns the with-withdraw plasma cost (issuance emits
// one descendant transfer).
func (p *IssueMethod) GetPlasma(plasmaTable *constants.PlasmaTable) (uint64, error) {
	return plasmaTable.EmbeddedWWithdraw, nil
}

// ValidateSendBlock requires exactly [constants.TokenIssueAmount]
// ZNN as the issuance fee and checks the parameters via
// [checkToken].
func (p *IssueMethod) ValidateSendBlock(block *nom.AccountBlock) error {
	var err error
	param := new(definition.IssueParam)

	if err := definition.ABIToken.UnpackMethod(param, p.MethodName, block.Data); err != nil {
		return constants.ErrUnpackError
	}
	if err = checkToken(*param); err != nil {
		return err
	}

	if block.TokenStandard != types.ZnnTokenStandard {
		return constants.ErrInvalidTokenOrAmount
	}
	if block.Amount.Cmp(constants.TokenIssueAmount) != 0 {
		return constants.ErrInvalidTokenOrAmount
	}

	block.Data, err = definition.ABIToken.PackMethod(
		p.MethodName,
		param.TokenName,
		param.TokenSymbol,
		param.TokenDomain,
		param.TotalSupply,
		param.MaxSupply,
		param.Decimals,
		param.IsMintable,
		param.IsBurnable,
		param.IsUtility)
	return err
}

// ReceiveBlock registers the new token (rejecting duplicate IDs
// with [constants.ErrIDNotUnique]), credits the contract's balance
// with TotalSupply, and emits a descendant transfer of the supply
// to the issuer.
func (p *IssueMethod) ReceiveBlock(context vm_context.AccountVmContext, sendBlock *nom.AccountBlock) ([]*nom.AccountBlock, error) {
	if err := p.ValidateSendBlock(sendBlock); err != nil {
		return nil, err
	}

	param := new(definition.IssueParam)
	err := definition.ABIToken.UnpackMethod(param, p.MethodName, sendBlock.Data)
	common.DealWithErr(err)

	tokenStandard := newTokenID(sendBlock.Hash)
	if _, err := definition.GetTokenInfo(context.Storage(), tokenStandard); err == constants.ErrDataNonExistent {
	} else if err == nil {
		return nil, constants.ErrIDNotUnique
	} else if err != constants.ErrDataNonExistent {
		common.DealWithErr(err)
	}

	tokenInfo := definition.TokenInfo{
		Owner:         sendBlock.Address,
		TokenName:     param.TokenName,
		TokenSymbol:   param.TokenSymbol,
		TokenDomain:   param.TokenDomain,
		TotalSupply:   param.TotalSupply,
		MaxSupply:     param.MaxSupply,
		Decimals:      param.Decimals,
		IsMintable:    param.IsMintable,
		IsBurnable:    param.IsBurnable,
		IsUtility:     param.IsUtility,
		TokenStandard: tokenStandard,
	}

	common.DealWithErr(tokenInfo.Save(context.Storage()))

	// add minted token to TokenContract
	context.AddBalance(&tokenStandard, param.TotalSupply)
	tokenLog.Debug("issued ZTS", "token", tokenInfo)
	return []*nom.AccountBlock{
		{
			Address:       types.TokenContract,
			ToAddress:     sendBlock.Address,
			BlockType:     nom.BlockTypeContractSend,
			Amount:        param.TotalSupply,
			TokenStandard: tokenStandard,
			Data:          []byte{},
		},
	}, nil
}

// MintMethod implements per-token minting. Owner authority varies:
// ZNN/QSR may only be minted by an embedded contract (e.g., swap,
// staking rewards); other tokens require their owner.
type MintMethod struct {
	MethodName string
}

// GetPlasma returns the with-withdraw plasma cost.
func (p *MintMethod) GetPlasma(plasmaTable *constants.PlasmaTable) (uint64, error) {
	return plasmaTable.EmbeddedWWithdraw, nil
}

// ValidateSendBlock decodes the (token, amount, recipient) tuple
// and rejects zero amounts or value-bearing calls.
func (p *MintMethod) ValidateSendBlock(block *nom.AccountBlock) error {
	var err error
	param := new(definition.MintParam)
	if err := definition.ABIToken.UnpackMethod(param, p.MethodName, block.Data); err != nil {
		return constants.ErrUnpackError
	}
	if param.Amount.Sign() <= 0 {
		return constants.ErrInvalidTokenOrAmount
	}
	if block.Amount.Sign() != 0 {
		return constants.ErrInvalidTokenOrAmount
	}

	block.Data, err = definition.ABIToken.PackMethod(p.MethodName, param.TokenStandard, param.Amount, param.ReceiveAddress)
	return err
}

// ReceiveBlock checks IsMintable, the MaxSupply ceiling, the
// caller's authority (embedded for ZNN/QSR; owner for everything
// else), credits the supply, and emits a descendant transfer to
// the recipient. Embedded recipients receive a Donate-encoded
// payload so the receive triggers their donation hook.
func (p *MintMethod) ReceiveBlock(context vm_context.AccountVmContext, sendBlock *nom.AccountBlock) ([]*nom.AccountBlock, error) {
	if err := p.ValidateSendBlock(sendBlock); err != nil {
		return nil, err
	}

	param := new(definition.MintParam)
	err := definition.ABIToken.UnpackMethod(param, p.MethodName, sendBlock.Data)
	common.DealWithErr(err)

	tokenInfo, err := definition.GetTokenInfo(context.Storage(), param.TokenStandard)
	if err == constants.ErrDataNonExistent {
		return nil, err
	}
	common.DealWithErr(err)

	if !tokenInfo.IsMintable {
		return nil, constants.ErrPermissionDenied
	}
	if new(big.Int).Sub(tokenInfo.MaxSupply, tokenInfo.TotalSupply).Cmp(param.Amount) < 0 {
		return nil, constants.ErrTokenInvalidAmount
	}

	// check owner, all embedded contracts for ZNN and QSR
	if param.TokenStandard == types.ZnnTokenStandard {
		if !types.IsEmbeddedAddress(sendBlock.Address) {
			return nil, constants.ErrPermissionDenied
		}
	} else if param.TokenStandard == types.QsrTokenStandard {
		if !types.IsEmbeddedAddress(sendBlock.Address) {
			return nil, constants.ErrPermissionDenied
		}
	} else if tokenInfo.Owner != sendBlock.Address {
		return nil, constants.ErrPermissionDenied
	}

	tokenInfo.TotalSupply.Add(tokenInfo.TotalSupply, param.Amount)
	common.DealWithErr(tokenInfo.Save(context.Storage()))

	// add minted token to TokenContract
	context.AddBalance(&param.TokenStandard, param.Amount)
	tokenLog.Debug("minted ZTS", "token", tokenInfo, "minted-amount", param.Amount, "to-address", param.ReceiveAddress)
	var data []byte
	if types.IsEmbeddedAddress(param.ReceiveAddress) {
		data, err = definition.ABICommon.PackMethod(definition.DonateMethodName)
		if err != nil {
			return nil, err
		}
	}
	return []*nom.AccountBlock{
		{
			Address:       types.TokenContract,
			ToAddress:     param.ReceiveAddress,
			BlockType:     nom.BlockTypeContractSend,
			Amount:        param.Amount,
			TokenStandard: param.TokenStandard,
			Data:          data,
		},
	}, nil
}

// BurnMethod implements per-token burning: anyone may burn a token
// when [definition.TokenInfo.IsBurnable] is true; the token's
// owner may always burn it. Non-mintable tokens have their
// MaxSupply reduced in lockstep so the cap stays accurate.
type BurnMethod struct {
	MethodName string
}

// GetPlasma returns the simple-call plasma cost.
func (p *BurnMethod) GetPlasma(plasmaTable *constants.PlasmaTable) (uint64, error) {
	return plasmaTable.EmbeddedSimple, nil
}

// ValidateSendBlock requires a positive transferred amount (the
// tokens to burn) and an empty argument list.
func (p *BurnMethod) ValidateSendBlock(block *nom.AccountBlock) error {
	var err error

	if err := definition.ABIToken.UnpackEmptyMethod(p.MethodName, block.Data); err != nil {
		return constants.ErrUnpackError
	}

	if block.Amount.Sign() != 1 {
		return constants.ErrInvalidTokenOrAmount
	}

	block.Data, err = definition.ABIToken.PackMethod(p.MethodName)
	return err
}

// ReceiveBlock validates the burn authority, reduces TotalSupply
// (and MaxSupply for non-mintable tokens), and debits the
// contract's cached balance.
func (p *BurnMethod) ReceiveBlock(context vm_context.AccountVmContext, sendBlock *nom.AccountBlock) ([]*nom.AccountBlock, error) {
	if err := p.ValidateSendBlock(sendBlock); err != nil {
		return nil, err
	}

	tokenInfo, err := definition.GetTokenInfo(context.Storage(), sendBlock.TokenStandard)
	if err == constants.ErrDataNonExistent {
		return nil, err
	}
	common.DealWithErr(err)

	if !tokenInfo.IsBurnable && tokenInfo.Owner != sendBlock.Address {
		return nil, constants.ErrPermissionDenied
	}

	// for non-mintable coins, drop MaxSupply as well
	if !tokenInfo.IsMintable {
		tokenInfo.MaxSupply.Sub(tokenInfo.MaxSupply, sendBlock.Amount)
	}
	tokenInfo.TotalSupply.Sub(tokenInfo.TotalSupply, sendBlock.Amount)
	common.DealWithErr(tokenInfo.Save(context.Storage()))

	// remove received token from TokenContract
	context.SubBalance(&sendBlock.TokenStandard, sendBlock.Amount)
	tokenLog.Debug("burned ZTS", "token", tokenInfo, "burned-amount", sendBlock.Amount)
	return nil, nil
}

// UpdateTokenMethod implements metadata rotation: only the current
// owner may transfer ownership or flip the mintable/burnable
// flags. Once IsMintable is set to false it cannot be flipped back
// — and MaxSupply collapses to TotalSupply.
type UpdateTokenMethod struct {
	MethodName string
}

// GetPlasma returns the simple-call plasma cost.
func (p *UpdateTokenMethod) GetPlasma(plasmaTable *constants.PlasmaTable) (uint64, error) {
	return plasmaTable.EmbeddedSimple, nil
}

// ValidateSendBlock decodes the update parameters and rejects
// value-bearing calls.
func (p *UpdateTokenMethod) ValidateSendBlock(block *nom.AccountBlock) error {
	var err error
	param := new(definition.UpdateTokenParam)

	if err := definition.ABIToken.UnpackMethod(param, p.MethodName, block.Data); err != nil {
		return constants.ErrUnpackError
	}

	if block.Amount.Sign() > 0 {
		return constants.ErrInvalidTokenOrAmount
	}

	block.Data, err = definition.ABIToken.PackMethod(p.MethodName, param.TokenStandard, param.Owner, param.IsMintable, param.IsBurnable)
	return err
}

// ReceiveBlock applies the metadata changes, enforcing the
// one-way IsMintable=false transition. Returns
// [constants.ErrPermissionDenied] when the caller is not the
// owner, [constants.ErrForbiddenParam] when attempting to
// re-enable IsMintable after it has been turned off.
func (p *UpdateTokenMethod) ReceiveBlock(context vm_context.AccountVmContext, sendBlock *nom.AccountBlock) ([]*nom.AccountBlock, error) {
	if err := p.ValidateSendBlock(sendBlock); err != nil {
		return nil, err
	}

	param := new(definition.UpdateTokenParam)
	err := definition.ABIToken.UnpackMethod(param, p.MethodName, sendBlock.Data)
	common.DealWithErr(err)

	tokenInfo, err := definition.GetTokenInfo(context.Storage(), param.TokenStandard)
	if err == constants.ErrDataNonExistent {
		return nil, err
	}
	common.DealWithErr(err)

	if tokenInfo.Owner != sendBlock.Address {
		return nil, constants.ErrPermissionDenied
	}

	if tokenInfo.IsMintable != param.IsMintable {
		if !tokenInfo.IsMintable {
			return nil, constants.ErrForbiddenParam
		}
		tokenLog.Debug("updating token IsMintable", "old", tokenInfo.IsMintable, "new", param.IsMintable)
		tokenInfo.IsMintable = param.IsMintable
		tokenInfo.MaxSupply = tokenInfo.TotalSupply
	}

	if tokenInfo.Owner != param.Owner {
		tokenLog.Debug("updating token owner", "old", tokenInfo.Owner, "new", param.Owner)
		tokenInfo.Owner = param.Owner
	}

	if tokenInfo.IsBurnable != param.IsBurnable {
		tokenLog.Debug("updating token IsBurnable", "old", tokenInfo.IsBurnable, "new", param.IsBurnable)
		tokenInfo.IsBurnable = param.IsBurnable
	}

	tokenLog.Debug("updated ZTS", "token", tokenInfo)
	common.DealWithErr(tokenInfo.Save(context.Storage()))
	return nil, nil
}
