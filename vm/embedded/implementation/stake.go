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

// stakeLog is the per-contract logger; tagged with `contract=stake`.
var (
	stakeLog = common.EmbeddedLogger.New("contract", "stake")
)

// StakeMethod implements ZNN staking: locks the caller's ZNN for a
// chosen duration and persists a [definition.StakeInfo] keyed by
// the originating send hash. The duration determines the weight
// (and therefore the share of staking-reward distribution).
type StakeMethod struct {
	MethodName string
}

// getWeightedStakeAmount computes the time-weighted stake amount.
//
// Formula: `(9 + duration/StakeTimeUnit) × amount / 10` — so a
// 1-unit stake weighs 1.0× its principal, a 2-unit stake weighs
// 1.1×, and a 12-unit stake weighs 2.1×. Longer durations earn
// more rewards per locked ZNN.
func getWeightedStakeAmount(amount *big.Int, stakingTime int64) *big.Int {
	weighted := big.NewInt(9 + stakingTime/constants.StakeTimeUnitSec)
	weighted.Mul(weighted, amount)
	weighted.Div(weighted, big.NewInt(10))
	return weighted
}

// GetPlasma returns the simple-call plasma cost.
func (p *StakeMethod) GetPlasma(plasmaTable *constants.PlasmaTable) (uint64, error) {
	return plasmaTable.EmbeddedSimple, nil
}

// ValidateSendBlock requires the call to carry at least
// [constants.StakeMinAmount] ZNN and a duration that is a whole
// multiple of [constants.StakeTimeUnitSec] within the [Min, Max]
// bounds.
func (p *StakeMethod) ValidateSendBlock(block *nom.AccountBlock) error {
	var err error
	var stakeTime int64

	if err := definition.ABIStake.UnpackMethod(&stakeTime, p.MethodName, block.Data); err != nil {
		return constants.ErrUnpackError
	}

	if block.Amount.Cmp(constants.StakeMinAmount) == -1 || block.TokenStandard != types.ZnnTokenStandard {
		return constants.ErrInvalidTokenOrAmount
	}
	if stakeTime < constants.StakeTimeMinSec || stakeTime > constants.StakeTimeMaxSec || stakeTime%constants.StakeTimeUnitSec != 0 {
		return constants.ErrInvalidStakingPeriod
	}

	block.Data, err = definition.ABIStake.PackMethod(p.MethodName, stakeTime)
	return err
}

// ReceiveBlock persists the [definition.StakeInfo] for the new
// stake. The Id is the originating send-block hash.
func (p *StakeMethod) ReceiveBlock(context vm_context.AccountVmContext, sendBlock *nom.AccountBlock) ([]*nom.AccountBlock, error) {
	if err := p.ValidateSendBlock(sendBlock); err != nil {
		return nil, err
	}

	var stakeTime int64
	common.DealWithErr(definition.ABIStake.UnpackMethod(&stakeTime, p.MethodName, sendBlock.Data))

	momentum, err := context.GetFrontierMomentum()
	common.DealWithErr(err)

	stakeInfo := definition.StakeInfo{
		Amount:         sendBlock.Amount,
		WeightedAmount: getWeightedStakeAmount(sendBlock.Amount, stakeTime),
		StartTime:      momentum.Timestamp.Unix(),
		RevokeTime:     0,
		ExpirationTime: momentum.Timestamp.Unix() + stakeTime,
		StakeAddress:   sendBlock.Address,
		Id:             sendBlock.Hash,
	}

	common.DealWithErr(stakeInfo.Save(context.Storage()))
	stakeLog.Debug("created stake entry", "id", stakeInfo.Id, "owner", stakeInfo.StakeAddress, "amount", stakeInfo.Amount, "weighted-amount", stakeInfo.WeightedAmount, "duration-in-days", stakeTime/24/60/60)
	return nil, nil
}

// CancelStakeMethod implements stake cancellation: refunds the
// locked ZNN once ExpirationTime has elapsed. The stake record is
// kept (with Amount zeroed and RevokeTime set) so the reward
// distributor can still credit pre-revocation epochs accurately.
type CancelStakeMethod struct {
	MethodName string
}

// GetPlasma returns the with-withdraw plasma cost (cancellation
// emits one descendant ZNN refund).
func (p *CancelStakeMethod) GetPlasma(plasmaTable *constants.PlasmaTable) (uint64, error) {
	return plasmaTable.EmbeddedWWithdraw, nil
}

// ValidateSendBlock decodes the target stake id and rejects
// value-bearing calls.
func (p *CancelStakeMethod) ValidateSendBlock(block *nom.AccountBlock) error {
	var err error
	id := new(types.Hash)

	if err := definition.ABIStake.UnpackMethod(id, p.MethodName, block.Data); err != nil {
		return constants.ErrUnpackError
	}

	if block.Amount.Sign() != 0 {
		return constants.ErrInvalidTokenOrAmount
	}

	block.Data, err = definition.ABIStake.PackMethod(p.MethodName, id)
	return err
}

// ReceiveBlock looks up the stake, refuses if its ExpirationTime
// has not been reached ([constants.RevokeNotDue]), zeroes the
// amount (signaling "already refunded") and emits the refund.
func (p *CancelStakeMethod) ReceiveBlock(context vm_context.AccountVmContext, sendBlock *nom.AccountBlock) ([]*nom.AccountBlock, error) {
	if err := p.ValidateSendBlock(sendBlock); err != nil {
		return nil, err
	}

	id := new(types.Hash)
	common.DealWithErr(definition.ABIStake.UnpackMethod(id, p.MethodName, sendBlock.Data))

	stakeInfo, err := definition.GetStakeInfo(context.Storage(), *id, sendBlock.Address)
	if err == constants.ErrDataNonExistent {
		return nil, constants.ErrDataNonExistent
	} else {
		common.DealWithErr(err)
	}

	momentum, err := context.GetFrontierMomentum()
	common.DealWithErr(err)

	if stakeInfo.ExpirationTime > momentum.Timestamp.Unix() {
		return nil, constants.RevokeNotDue
	}

	amount := stakeInfo.Amount
	stakeInfo.RevokeTime = momentum.Timestamp.Unix()
	// signal that the amount has been received, to future-proof
	stakeInfo.Amount = common.Big0
	common.DealWithErr(stakeInfo.Save(context.Storage()))

	stakeLog.Debug("revoked stake entry", "id", stakeInfo.Id, "owner", stakeInfo.StakeAddress, "start-time", stakeInfo.StartTime, "revoke-time", stakeInfo.RevokeTime)

	return []*nom.AccountBlock{
		{
			Address:       types.StakeContract,
			ToAddress:     stakeInfo.StakeAddress,
			BlockType:     nom.BlockTypeContractSend,
			Amount:        amount,
			TokenStandard: types.ZnnTokenStandard,
			Data:          nil,
		},
	}, nil
}

// UpdateEmbeddedStakeMethod is the periodic-update entry point:
// advances the per-epoch staking-reward computation.
type UpdateEmbeddedStakeMethod struct {
	MethodName string
}

// GetPlasma returns the simple-call plasma cost.
func (p *UpdateEmbeddedStakeMethod) GetPlasma(plasmaTable *constants.PlasmaTable) (uint64, error) {
	return plasmaTable.EmbeddedSimple, nil
}

// ValidateSendBlock requires no value and no arguments.
func (p *UpdateEmbeddedStakeMethod) ValidateSendBlock(block *nom.AccountBlock) error {
	var err error

	if err := definition.ABIStake.UnpackEmptyMethod(p.MethodName, block.Data); err != nil {
		return constants.ErrUnpackError
	}

	if block.Amount.Sign() != 0 {
		return constants.ErrInvalidTokenOrAmount
	}

	block.Data, err = definition.ABIStake.PackMethod(p.MethodName)
	return err
}

// ReceiveBlock advances the per-epoch reward computation via
// [updateStakeRewards]. Rate-limited by [checkAndPerformUpdate].
func (p *UpdateEmbeddedStakeMethod) ReceiveBlock(context vm_context.AccountVmContext, sendBlock *nom.AccountBlock) ([]*nom.AccountBlock, error) {
	if err := p.ValidateSendBlock(sendBlock); err != nil {
		return nil, err
	}

	if err := checkAndPerformUpdate(context); err != nil {
		return nil, err
	}

	// Update epochRewards
	err := updateStakeRewards(context)
	return nil, err
}

// getWeightedStake returns the time-weighted stake amount for one
// stake's contribution to a given epoch window:
// `min(end, revoke or end) - max(start, stakeStart)) ×
// WeightedAmount`. Stakes that don't overlap the window
// contribute zero.
func getWeightedStake(info *definition.StakeInfo, startTime, endTime int64) *big.Int {
	startTime = common.MaxInt64(startTime, info.StartTime)
	if info.RevokeTime != 0 {
		endTime = common.MinInt64(endTime, info.RevokeTime)
	}

	if startTime >= endTime {
		return big.NewInt(0)
	}
	cumulatedStake := big.NewInt(endTime - startTime)
	cumulatedStake.Mul(cumulatedStake, info.WeightedAmount)

	return cumulatedStake
}

// computeStakeRewardsForEpoch divides the per-epoch QSR pool
// proportionally to each stake's [getWeightedStake] contribution.
// Revoked-and-past-end stakes are deleted to keep storage bounded.
func computeStakeRewardsForEpoch(context vm_context.AccountVmContext, epoch uint64) error {
	startTime, endTime := context.EpochTicker().ToTime(epoch)

	cumulatedStake := big.NewInt(0)
	totalAmount := constants.StakeQsrRewardPerEpoch(epoch)

	err := definition.IterateStakeEntries(context.Storage(), func(stakeInfo *definition.StakeInfo) error {
		cumulatedStake.Add(cumulatedStake, getWeightedStake(stakeInfo, startTime.Unix(), endTime.Unix()))
		return nil
	})
	if err != nil {
		return err
	}

	stakeLog.Debug("updating stake reward", "epoch", epoch, "total-reward", constants.StakeQsrRewardPerEpoch(epoch), "cumulated-stake", cumulatedStake, "start-time", startTime.Unix(), "end-time", endTime.Unix())
	if cumulatedStake.Sign() == 0 {
		return nil
	}

	err = definition.IterateStakeEntries(context.Storage(), func(stakeInfo *definition.StakeInfo) error {
		reward := new(big.Int).Set(totalAmount)
		reward.Mul(reward, getWeightedStake(stakeInfo, startTime.Unix(), endTime.Unix()))
		reward.Quo(reward, cumulatedStake)

		addReward(context, epoch, definition.RewardDeposit{
			Address: &stakeInfo.StakeAddress,
			Znn:     common.Big0,
			Qsr:     reward,
		})

		stakeLog.Debug("giving rewards", "address", stakeInfo.StakeAddress, "id", stakeInfo.Id, "epoch", epoch, "qsr-amount", reward)

		if stakeInfo.RevokeTime != 0 && stakeInfo.RevokeTime < endTime.Unix() {
			stakeLog.Debug("deleted stake entry", "id", stakeInfo.Id, "revoke-time", stakeInfo.RevokeTime)
			common.DealWithErr(stakeInfo.Delete(context.Storage()))
		}
		return nil
	})
	if err != nil {
		return err
	}

	return nil
}

// updateStakeRewards walks every still-unprocessed epoch and
// computes its reward distribution, stopping when
// [checkAndPerformUpdateEpoch] reports the next epoch is too
// recent.
func updateStakeRewards(context vm_context.AccountVmContext) error {
	lastEpoch, err := definition.GetLastEpochUpdate(context.Storage())
	if err != nil {
		return err
	}

	for {
		if err := checkAndPerformUpdateEpoch(context, lastEpoch); err == constants.ErrEpochUpdateTooRecent {
			stakeLog.Debug("invalid update - rewards not due yet", "epoch", lastEpoch.LastEpoch+1)
			return nil
		} else if err != nil {
			stakeLog.Error("unknown panic", "reason", err)
			return err
		}
		if err := computeStakeRewardsForEpoch(context, uint64(lastEpoch.LastEpoch)); err != nil {
			return err
		}
	}
}
