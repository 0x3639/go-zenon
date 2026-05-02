package implementation

import (
	"math/big"

	"github.com/zenon-network/go-zenon/chain/nom"
	"github.com/zenon-network/go-zenon/common"
	"github.com/zenon-network/go-zenon/common/types"
	"github.com/zenon-network/go-zenon/vm/constants"
	"github.com/zenon-network/go-zenon/vm/embedded/definition"
	"github.com/zenon-network/go-zenon/vm/vm_context"
)

// plasmaLog is the per-contract logger; tagged with `contract=plasma`.
var (
	plasmaLog = common.EmbeddedLogger.New("contract", "plasma")
)

// FuseMethod implements QSR fusing: locks the caller's QSR for
// [constants.FuseExpiration] momentums, granting plasma to the
// supplied beneficiary in proportion to the locked amount.
type FuseMethod struct {
	MethodName string
}

// GetPlasma returns the simple-call plasma cost.
func (p *FuseMethod) GetPlasma(plasmaTable *constants.PlasmaTable) (uint64, error) {
	return plasmaTable.EmbeddedSimple, nil
}

// ValidateSendBlock checks the call carries QSR (not ZNN), the
// amount meets [constants.FuseMinAmount] and is a whole multiple
// of [constants.CostPerFusionUnit].
func (p *FuseMethod) ValidateSendBlock(block *nom.AccountBlock) error {
	var err error
	param := new(types.Address)

	if err := definition.ABIPlasma.UnpackMethod(param, p.MethodName, block.Data); err != nil {
		return constants.ErrUnpackError
	}

	// make sure users send QSR and more than min amount
	if block.TokenStandard != types.QsrTokenStandard || block.Amount.Cmp(constants.FuseMinAmount) < 0 {
		return constants.ErrInvalidTokenOrAmount
	}

	// make sure users send multiple of constants.CostPerFusionUnit
	mod := new(big.Int).Mod(block.Amount, big.NewInt(constants.CostPerFusionUnit))
	if mod.Sign() != 0 {
		return constants.ErrInvalidTokenOrAmount
	}

	block.Data, err = definition.ABIPlasma.PackMethod(p.MethodName, param)
	return err
}

// ReceiveBlock persists a [definition.FusionInfo] keyed by the
// originating send hash and increments the per-beneficiary
// [definition.FusedAmount] counter so the plasma layer can read
// the total in O(1).
func (p *FuseMethod) ReceiveBlock(context vm_context.AccountVmContext, sendBlock *nom.AccountBlock) ([]*nom.AccountBlock, error) {
	if err := p.ValidateSendBlock(sendBlock); err != nil {
		return nil, err
	}

	beneficiary := new(types.Address)
	err := definition.ABIPlasma.UnpackMethod(beneficiary, p.MethodName, sendBlock.Data)
	common.DealWithErr(err)

	momentum, err := context.GetFrontierMomentum()
	common.DealWithErr(err)

	fusionInfo := definition.FusionInfo{
		Owner:            sendBlock.Address,
		Id:               sendBlock.Hash,
		Amount:           sendBlock.Amount,
		Beneficiary:      *beneficiary,
		ExpirationHeight: momentum.Height + constants.FuseExpiration,
	}
	common.DealWithErr(fusionInfo.Save(context.Storage()))

	fused, err := definition.GetFusedAmount(context.Storage(), *beneficiary)
	common.DealWithErr(err)
	fused.Amount.Add(fused.Amount, sendBlock.Amount)
	common.DealWithErr(fused.Save(context.Storage()))

	plasmaLog.Debug("fused new entry", "fusionInfo", fusionInfo, "beneficiary", fused)
	return nil, nil
}

// CancelFuseMethod implements unfusing: releases a previously-fused
// QSR position once its ExpirationHeight has elapsed and refunds
// the QSR back to the original owner.
type CancelFuseMethod struct {
	MethodName string
}

// GetPlasma returns the with-withdraw plasma cost (the receive
// emits one descendant refund).
func (p *CancelFuseMethod) GetPlasma(plasmaTable *constants.PlasmaTable) (uint64, error) {
	return plasmaTable.EmbeddedWWithdraw, nil
}

// ValidateSendBlock decodes the target fusion id and rejects calls
// that carry value.
func (p *CancelFuseMethod) ValidateSendBlock(block *nom.AccountBlock) error {
	var err error
	param := new(types.Hash)

	if err := definition.ABIPlasma.UnpackMethod(param, p.MethodName, block.Data); err != nil {
		return constants.ErrUnpackError
	}

	if block.Amount.Sign() > 0 {
		return constants.ErrInvalidTokenOrAmount
	}

	block.Data, err = definition.ABIPlasma.PackMethod(p.MethodName, param)
	return err
}

// ReceiveBlock looks up the fusion record, refuses if its
// ExpirationHeight has not been reached
// ([constants.RevokeNotDue]), and emits a descendant QSR refund to
// the caller. Updates the per-beneficiary cumulative counter
// (deleting it if it would reach zero).
func (p *CancelFuseMethod) ReceiveBlock(context vm_context.AccountVmContext, sendBlock *nom.AccountBlock) ([]*nom.AccountBlock, error) {
	if err := p.ValidateSendBlock(sendBlock); err != nil {
		return nil, err
	}

	id := new(types.Hash)
	err := definition.ABIPlasma.UnpackMethod(id, p.MethodName, sendBlock.Data)
	common.DealWithErr(err)

	momentum, err := context.GetFrontierMomentum()
	common.DealWithErr(err)

	fusionInfo, err := definition.GetFusionInfo(context.Storage(), sendBlock.Address, *id)
	if err == constants.ErrDataNonExistent {
		return nil, err
	}
	common.DealWithErr(err)

	if fusionInfo.ExpirationHeight > momentum.Height {
		return nil, constants.RevokeNotDue
	}

	fused, err := definition.GetFusedAmount(context.Storage(), fusionInfo.Beneficiary)
	common.DealWithErr(err)
	fused.Amount.Sub(fused.Amount, fusionInfo.Amount)

	plasmaLog.Debug("canceled fusion entry", "fusionInfo", fusionInfo, "beneficiary-remaining", fused)

	if fused.Amount.Sign() == 0 {
		common.DealWithErr(fused.Delete(context.Storage()))
	} else {
		common.DealWithErr(fused.Save(context.Storage()))
	}
	common.DealWithErr(fusionInfo.Delete(context.Storage()))

	return []*nom.AccountBlock{
		{
			Address:       types.PlasmaContract,
			ToAddress:     sendBlock.Address,
			BlockType:     nom.BlockTypeContractSend,
			Amount:        fusionInfo.Amount,
			TokenStandard: types.QsrTokenStandard,
			Data:          []byte{},
		},
	}, nil
}
