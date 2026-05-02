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

// sentinelLog is the per-contract logger; tagged with `contract=sentinel`.
var (
	sentinelLog = common.EmbeddedLogger.New("contract", "sentinel")
)

// GetSentinelRevokeStatus returns whether a sentinel registered at
// registrationTime can be revoked at the supplied momentum, plus
// the time delta until/while revocation is permitted.
//
// Sentinels follow a repeating (lock, revoke-window) cycle of
// length [constants.SentinelLockTimeWindow] +
// [constants.SentinelRevokeTimeWindow]. Within the lock segment
// revocation is blocked; within the revoke-window segment it is
// allowed. The returned int64 is the seconds-until-revoke in the
// blocked case, or seconds-while-can-revoke in the allowed case.
//
//   - returns true, timeWhileCanRevoke if sentinel *can* be
//     revoked.
//   - returns false, timeUntilCanRevoke if sentinel *can't* be
//     revoked.
func GetSentinelRevokeStatus(registrationTime int64, m *nom.Momentum) (bool, int64) {
	epochTime := (m.Timestamp.Unix() - registrationTime) % (constants.SentinelLockTimeWindow + constants.SentinelRevokeTimeWindow)
	if epochTime < constants.SentinelLockTimeWindow {
		return false, constants.SentinelLockTimeWindow - epochTime
	} else {
		return true, (constants.SentinelLockTimeWindow + constants.SentinelRevokeTimeWindow) - epochTime
	}
}

// RegisterSentinelMethod implements sentinel registration: locks
// [constants.SentinelZnnRegisterAmount] ZNN (sent with the call)
// and consumes [constants.SentinelQsrDepositAmount] from the
// caller's prior QSR deposit.
type RegisterSentinelMethod struct {
	MethodName string
}

// GetPlasma returns the simple-call plasma cost.
func (method *RegisterSentinelMethod) GetPlasma(plasmaTable *constants.PlasmaTable) (uint64, error) {
	return plasmaTable.EmbeddedSimple, nil
}

// ValidateSendBlock requires exactly the configured ZNN
// registration amount.
func (method *RegisterSentinelMethod) ValidateSendBlock(block *nom.AccountBlock) error {
	var err error

	if err := definition.ABISentinel.UnpackEmptyMethod(method.MethodName, block.Data); err != nil {
		return constants.ErrUnpackError
	}

	if block.TokenStandard != types.ZnnTokenStandard || block.Amount.Cmp(constants.SentinelZnnRegisterAmount) != 0 {
		return constants.ErrInvalidTokenOrAmount
	}

	block.Data, err = definition.ABISentinel.PackMethod(method.MethodName)
	return err
}

// ReceiveBlock rejects re-registration
// ([constants.ErrAlreadyRegistered]) and missing QSR deposit, then
// persists a fresh [definition.SentinelInfo].
func (method *RegisterSentinelMethod) ReceiveBlock(context vm_context.AccountVmContext, sendBlock *nom.AccountBlock) ([]*nom.AccountBlock, error) {
	if err := method.ValidateSendBlock(sendBlock); err != nil {
		sentinelLog.Debug("invalid register - syntactic validation failed", "address", sendBlock.Address, "reason", err)
		return nil, err
	}

	sentinel := definition.GetSentinelInfoByOwner(context.Storage(), sendBlock.Address)
	if sentinel != nil {
		sentinelLog.Debug("invalid register - existing address", "address", sendBlock.Address)
		return nil, constants.ErrAlreadyRegistered
	}

	if err := checkAndConsumeQsr(context, sendBlock.Address, constants.SentinelQsrDepositAmount); err != nil {
		sentinelLog.Debug("invalid register - not enough deposited qsr", "address", sendBlock.Address)
		return nil, err
	}

	frontierMomentum, err := context.GetFrontierMomentum()
	common.DealWithErr(err)

	sentinel = &definition.SentinelInfo{
		SentinelInfoKey: definition.SentinelInfoKey{
			Owner: sendBlock.Address,
		},
		RegistrationTimestamp: frontierMomentum.Timestamp.Unix(),
		RevokeTimestamp:       0,
		ZnnAmount:             constants.SentinelZnnRegisterAmount,
		QsrAmount:             constants.SentinelQsrDepositAmount,
	}
	sentinel.Save(context.Storage())
	sentinelLog.Debug("successfully register", "sentinel", sentinel)
	return nil, nil
}

// RevokeSentinelMethod implements revocation: refunds the locked
// ZNN/QSR back to the owner and marks the sentinel record as
// revoked. Only allowed within the revoke-window phase of the
// sentinel cycle (see [GetSentinelRevokeStatus]).
type RevokeSentinelMethod struct {
	MethodName string
}

// GetPlasma returns the double-withdraw plasma cost (revocation
// emits both ZNN and QSR refund descendants).
func (method *RevokeSentinelMethod) GetPlasma(plasmaTable *constants.PlasmaTable) (uint64, error) {
	return plasmaTable.EmbeddedWDoubleWithdraw, nil
}

// ValidateSendBlock requires no transferred value and an empty
// argument list.
func (method *RevokeSentinelMethod) ValidateSendBlock(block *nom.AccountBlock) error {
	var err error

	if err := definition.ABISentinel.UnpackEmptyMethod(method.MethodName, block.Data); err != nil {
		return constants.ErrUnpackError
	}

	if block.Amount.Sign() != 0 {
		return constants.ErrInvalidTokenOrAmount
	}

	block.Data, err = definition.ABISentinel.PackMethod(method.MethodName)
	return err
}

// ReceiveBlock checks the revoke-window gate and emits two
// descendant refunds (ZNN, QSR). Returns
// [constants.ErrDataNonExistent] for unknown sentinels,
// [constants.ErrAlreadyRevoked] for double-revoke,
// [constants.RevokeNotDue] when the lock window has not yet
// expired.
func (method *RevokeSentinelMethod) ReceiveBlock(context vm_context.AccountVmContext, sendBlock *nom.AccountBlock) ([]*nom.AccountBlock, error) {
	if err := method.ValidateSendBlock(sendBlock); err != nil {
		sentinelLog.Debug("invalid revoke - syntactic validation failed", "address", sendBlock.Address, "reason", err)
		return nil, err
	}

	frontierMomentum, err := context.GetFrontierMomentum()
	common.DealWithErr(err)

	sentinel := definition.GetSentinelInfoByOwner(context.Storage(), sendBlock.Address)
	if sentinel == nil {
		sentinelLog.Debug("invalid revoke - sentinel is not registered", "address", sendBlock.Address)
		return nil, constants.ErrDataNonExistent
	}

	if sentinel.RevokeTimestamp != 0 {
		sentinelLog.Debug("invalid revoke - sentinel is already revoked", "address", sendBlock.Address)
		return nil, constants.ErrAlreadyRevoked
	}

	if canRevoke, untilRevoke := GetSentinelRevokeStatus(sentinel.RegistrationTimestamp, frontierMomentum); !canRevoke {
		sentinelLog.Debug("invalid revoke - cannot be revoked yet", "address", sendBlock.Address, "until-revoke", untilRevoke)
		return nil, constants.RevokeNotDue
	}

	znnAmount := new(big.Int).Set(sentinel.ZnnAmount)
	qsrAmount := new(big.Int).Set(sentinel.QsrAmount)

	sentinel.RevokeTimestamp = frontierMomentum.Timestamp.Unix()
	sentinel.ZnnAmount.Set(common.Big0)
	sentinel.QsrAmount.Set(common.Big0)
	sentinel.Save(context.Storage())
	sentinelLog.Debug("successfully revoke", "sentinel", sentinel)
	return []*nom.AccountBlock{
		{
			ToAddress:     sentinel.Owner,
			Amount:        znnAmount,
			TokenStandard: types.ZnnTokenStandard,
		},
		{
			ToAddress:     sentinel.Owner,
			Amount:        qsrAmount,
			TokenStandard: types.QsrTokenStandard,
		},
	}, nil
}

// UpdateEmbeddedSentinelMethod is the periodic-update entry point:
// advances the per-epoch reward computation.
type UpdateEmbeddedSentinelMethod struct {
	MethodName string
}

// GetPlasma returns the simple-call plasma cost.
func (method *UpdateEmbeddedSentinelMethod) GetPlasma(plasmaTable *constants.PlasmaTable) (uint64, error) {
	return plasmaTable.EmbeddedSimple, nil
}

// ValidateSendBlock requires no transferred value and an empty
// argument list.
func (method *UpdateEmbeddedSentinelMethod) ValidateSendBlock(block *nom.AccountBlock) error {
	var err error

	if err := definition.ABISentinel.UnpackEmptyMethod(method.MethodName, block.Data); err != nil {
		return constants.ErrUnpackError
	}

	if block.Amount.Sign() != 0 {
		return constants.ErrInvalidTokenOrAmount
	}

	block.Data, err = definition.ABISentinel.PackMethod(method.MethodName)
	return err
}

// ReceiveBlock advances the per-epoch reward computation via
// [updateSentinelRewards]. Rate-limited by
// [checkAndPerformUpdate].
func (method *UpdateEmbeddedSentinelMethod) ReceiveBlock(context vm_context.AccountVmContext, sendBlock *nom.AccountBlock) ([]*nom.AccountBlock, error) {
	if err := method.ValidateSendBlock(sendBlock); err != nil {
		sentinelLog.Debug("invalid update - syntactic validation failed", "address", sendBlock.Address, "reason", err)
		return nil, err
	}

	if err := checkAndPerformUpdate(context); err != nil {
		sentinelLog.Debug("invalid update - cannot perform update", "address", sendBlock.Address, "reason", err)
		return nil, err
	}

	// Update epochRewards
	err := updateSentinelRewards(context)
	return nil, err
}

// getWeightedSentinel returns 1 if the sentinel was active for at
// least 90% of the supplied epoch window, otherwise 0. Used as a
// uniform-weight uptime metric: only sentinels that meet the 90%
// threshold split the per-epoch reward pool.
func getWeightedSentinel(info *definition.SentinelInfo, startTime, endTime int64) *big.Int {
	epochDuration := endTime - startTime
	startTime = common.MaxInt64(startTime, info.RegistrationTimestamp)
	if info.RevokeTimestamp != 0 {
		endTime = common.MinInt64(endTime, info.RevokeTimestamp)
	}

	if startTime >= endTime {
		return common.Big0
	}
	if epochDuration*90 < (endTime-startTime)*100 {
		return common.Big1
	}
	return common.Big0
}

// computeSentinelRewardsForEpoch divides the per-epoch sentinel
// reward pool (ZNN + QSR) evenly across qualifying sentinels (those
// that pass the 90% uptime test). Each qualifying sentinel
// receives `pool × weight / cumulatedSentinel`; weights are 0/1 so
// the math reduces to equal-share among qualifiers.
func computeSentinelRewardsForEpoch(context vm_context.AccountVmContext, epoch uint64) error {
	startTime, endTime := context.EpochTicker().ToTime(epoch)

	cumulatedSentinel := big.NewInt(0)
	totalZnnAmount, totalQsrAmount := constants.SentinelRewardForEpoch(epoch)

	err := definition.IterateSentinelEntries(context.Storage(), func(sentinelInfo *definition.SentinelInfo) error {
		cumulatedSentinel.Add(cumulatedSentinel, getWeightedSentinel(sentinelInfo, startTime.Unix(), endTime.Unix()))
		return nil
	})
	if err != nil {
		sentinelLog.Debug("unable to update sentinel reward", "epoch", epoch, "start-time", startTime.Unix(), "end-time", endTime.Unix(), "reason", err)
		return err
	}

	sentinelLog.Debug("updating sentinel reward", "epoch", epoch, "total-znn-reward", totalZnnAmount, "total-qsr-reward", totalQsrAmount, "cumulated-sentinel", cumulatedSentinel, "start-time", startTime.Unix(), "end-time", endTime.Unix())
	if cumulatedSentinel.Sign() == 0 {
		return nil
	}

	err = definition.IterateSentinelEntries(context.Storage(), func(sentinelInfo *definition.SentinelInfo) error {
		weight := getWeightedSentinel(sentinelInfo, startTime.Unix(), endTime.Unix())
		if weight.Sign() == 0 {
			return nil
		}

		znnReward := new(big.Int).Set(totalZnnAmount)
		znnReward.Mul(znnReward, weight)
		znnReward.Quo(znnReward, cumulatedSentinel)
		qsrReward := new(big.Int).Set(totalQsrAmount)
		qsrReward.Mul(qsrReward, weight)
		qsrReward.Quo(qsrReward, cumulatedSentinel)

		sentinelLog.Debug("giving rewards", "address", sentinelInfo.Owner, "epoch", epoch, "znn-amount", znnReward, "qsr-amount", qsrReward)
		addReward(context, epoch, definition.RewardDeposit{
			Address: &sentinelInfo.Owner,
			Znn:     znnReward,
			Qsr:     qsrReward,
		})

		return nil
	})
	if err != nil {
		return err
	}

	return nil
}

// updateSentinelRewards walks every still-unprocessed epoch and
// computes its reward distribution, stopping when
// [checkAndPerformUpdateEpoch] reports the next epoch is too
// recent.
func updateSentinelRewards(context vm_context.AccountVmContext) error {
	lastEpoch, err := definition.GetLastEpochUpdate(context.Storage())
	if err != nil {
		return err
	}

	for {
		if err := checkAndPerformUpdateEpoch(context, lastEpoch); err == constants.ErrEpochUpdateTooRecent {
			sentinelLog.Debug("invalid update - rewards not due yet", "epoch", lastEpoch.LastEpoch+1)
			return nil
		} else if err != nil {
			sentinelLog.Error("unknown panic", "reason", err)
			return err
		}
		if err := computeSentinelRewardsForEpoch(context, uint64(lastEpoch.LastEpoch)); err != nil {
			return err
		}
	}
}
