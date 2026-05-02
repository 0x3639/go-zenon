package implementation

import (
	"encoding/base64"
	"math/big"

	"github.com/zenon-network/go-zenon/chain/nom"
	"github.com/zenon-network/go-zenon/common"
	"github.com/zenon-network/go-zenon/common/types"
	"github.com/zenon-network/go-zenon/vm/constants"
	"github.com/zenon-network/go-zenon/vm/embedded/definition"
	"github.com/zenon-network/go-zenon/vm/vm_context"
)

// swapLog is the per-contract logger; tagged with `contract=swap`.
var (
	swapLog = common.EmbeddedLogger.New("contract", "swap")
)

// ApplyDecay reduces a swap deposit's redeemable ZNN/QSR according
// to the decay schedule: full value before
// [constants.SwapAssetDecayEpochsOffset], then drops by
// [constants.SwapAssetDecayTickValuePercentage] every
// [constants.SwapAssetDecayTickEpochs] epochs until zero. Mutates
// deposit in place.
func ApplyDecay(deposit *definition.SwapAssets, currentEpoch int) {
	percentageToGive := 100
	if currentEpoch < constants.SwapAssetDecayEpochsOffset {
		percentageToGive = 100
	} else {
		numTicks := (currentEpoch - constants.SwapAssetDecayEpochsOffset + 1) / constants.SwapAssetDecayTickEpochs
		decayFactor := constants.SwapAssetDecayTickValuePercentage * numTicks
		if decayFactor > 100 {
			percentageToGive = 0
		} else {
			percentageToGive = 100 - decayFactor
		}
	}

	deposit.Znn.Mul(deposit.Znn, big.NewInt(int64(percentageToGive)))
	deposit.Znn.Div(deposit.Znn, common.Big100)
	deposit.Qsr.Mul(deposit.Qsr, big.NewInt(int64(percentageToGive)))
	deposit.Qsr.Div(deposit.Qsr, common.Big100)
}

// SwapRetrieveAssetsMethod implements the legacy-claim retrieval
// flow: a holder of a legacy-chain key signs the canonical swap
// message, this contract verifies the signature, applies the decay
// schedule, mints the redeemed ZNN/QSR amounts via the token
// contract, and zeroes out the swap entry.
type SwapRetrieveAssetsMethod struct {
	MethodName string
}

// GetPlasma returns the double-withdraw plasma cost (the receive
// emits up to two descendant Mint calls — one ZNN, one QSR).
func (p *SwapRetrieveAssetsMethod) GetPlasma(plasmaTable *constants.PlasmaTable) (uint64, error) {
	return plasmaTable.EmbeddedWDoubleWithdraw, nil
}

// ValidateSendBlock decodes the (publicKey, signature) pair,
// verifies the signature against the canonical swap message, and
// rejects calls that carry value.
func (p *SwapRetrieveAssetsMethod) ValidateSendBlock(block *nom.AccountBlock) error {
	var err error
	param := new(definition.ParamRetrieveAssets)

	if err := definition.ABISwap.UnpackMethod(param, p.MethodName, block.Data); err != nil {
		return constants.ErrUnpackError
	}

	if _, err := CheckSwapSignature(SwapRetrieveAssets, block.Address, param.PublicKey, param.Signature); err != nil {
		return err
	}
	if block.Amount.Sign() != 0 {
		return constants.ErrInvalidTokenOrAmount
	}

	block.Data, err = definition.ABISwap.PackMethod(p.MethodName, param.PublicKey, param.Signature)
	return err
}

// ReceiveBlock looks up the swap entry by the legacy-key id hash,
// applies the epoch-based decay, emits up to two descendant
// [TokenContract] mint calls (one for ZNN, one for QSR), and
// zeroes the storage entry. Returns
// [constants.ErrDataNonExistent] if the entry does not exist or is
// already drained.
func (p *SwapRetrieveAssetsMethod) ReceiveBlock(context vm_context.AccountVmContext, sendBlock *nom.AccountBlock) ([]*nom.AccountBlock, error) {
	if err := p.ValidateSendBlock(sendBlock); err != nil {
		return nil, err
	}

	param := new(definition.ParamRetrieveAssets)
	err := definition.ABISwap.UnpackMethod(param, p.MethodName, sendBlock.Data)
	common.DealWithErr(err)
	swapLog.Debug("swap-assets-log", "address", sendBlock.Address, "public-key", param.PublicKey, "signature", param.Signature)

	publicKey, err := base64.StdEncoding.DecodeString(param.PublicKey)
	if err != nil {
		return nil, constants.ErrForbiddenParam
	}
	deposit, err := definition.GetSwapAssetsByKeyIdHash(context.Storage(), PubKeyToKeyIdHash(publicKey))
	if err == constants.ErrDataNonExistent {
		return nil, err
	} else {
		common.DealWithErr(err)
	}

	if deposit.Qsr.Cmp(common.Big0) == 0 && deposit.Znn.Cmp(common.Big0) == 0 {
		return nil, constants.ErrDataNonExistent
	}

	swapLog.Debug("deposit to withdraw", "znn", deposit.Znn, "qsr", deposit.Qsr)
	currentM, err := context.GetFrontierMomentum()
	common.DealWithErr(err)
	currentEpoch := int(context.EpochTicker().ToTick(*currentM.Timestamp))
	ApplyDecay(deposit, currentEpoch)

	result := make([]*nom.AccountBlock, 0, 2)
	if deposit.Znn.Sign() == +1 {
		result = append(result, &nom.AccountBlock{
			Address:       types.SwapContract,
			ToAddress:     types.TokenContract,
			BlockType:     nom.BlockTypeContractSend,
			Amount:        big.NewInt(0),
			TokenStandard: types.ZnnTokenStandard,
			Data: definition.ABIToken.PackMethodPanic(
				definition.MintMethodName,
				types.ZnnTokenStandard,
				deposit.Znn,
				sendBlock.Address,
			),
		})
	}
	if deposit.Qsr.Sign() == +1 {
		result = append(result, &nom.AccountBlock{
			Address:       types.SwapContract,
			ToAddress:     types.TokenContract,
			BlockType:     nom.BlockTypeContractSend,
			Amount:        big.NewInt(0),
			TokenStandard: types.ZnnTokenStandard,
			Data: definition.ABIToken.PackMethodPanic(
				definition.MintMethodName,
				types.QsrTokenStandard,
				deposit.Qsr,
				sendBlock.Address,
			),
		})
	}

	deposit.Znn = common.Big0
	deposit.Qsr = common.Big0
	common.DealWithErr(deposit.Save(context.Storage()))

	return result, nil
}
