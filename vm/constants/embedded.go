package constants

import (
	"github.com/zenon-network/go-zenon/common/types"
	"math/big"

	"github.com/zenon-network/go-zenon/common"
)

// Generic chain scale constants.
const (
	// Decimals is 10^8 — the atomic-unit scale factor for both ZNN
	// and QSR balances.
	Decimals = 100000000
	// SecsInDay is the number of seconds in 24 hours; used in time
	// calculations across staking, sentinel, and pillar contracts.
	SecsInDay = 24 * 60 * 60
)

// Embedded-contract tunables. Grouped by contract; reward / stake /
// timing values are alphanet defaults.
var (
	// === Common ===

	// MomentumsPerHour is the number of momentums per hour at the
	// configured 10s block time. Should be used instead of plain
	// '3600' or similar.
	MomentumsPerHour int64 = 3600 / 10
	// MomentumsPerEpoch is the number of momentums per 24h epoch.
	MomentumsPerEpoch = MomentumsPerHour * 24
	// RewardTimeLimit caps how far back (in seconds) reward
	// computations may look.
	RewardTimeLimit int64 = 3600

	// UpdateMinNumMomentums is the number of momentums between two
	// UpdateEmbedded* calls which will execute, used for all
	// applicable contracts.
	UpdateMinNumMomentums = uint64(MomentumsPerHour * 5 / 6)
	// MaxEpochsPerUpdate is the maximum number of epochs an
	// UpdateEmbedded* call may advance through in one invocation.
	MaxEpochsPerUpdate = 20

	// === Accelerator ===

	// ProjectNameLengthMax is the maximum byte length of an
	// accelerator project name.
	ProjectNameLengthMax = 30
	// ProjectDescriptionLengthMax is the maximum byte length of an
	// accelerator project description.
	ProjectDescriptionLengthMax = 240
	// ProjectZnnMaximumFunds caps the ZNN amount one accelerator
	// project may request.
	ProjectZnnMaximumFunds = big.NewInt(5000 * Decimals)
	// ProjectQsrMaximumFunds caps the QSR amount one accelerator
	// project may request.
	ProjectQsrMaximumFunds = big.NewInt(50000 * Decimals)
	// ProjectCreationAmount is the ZNN deposit required to create an
	// accelerator project.
	ProjectCreationAmount = big.NewInt(1 * Decimals)
	// PhaseTimeUnit is the time unit (in seconds) for accelerator
	// phase / voting durations.
	PhaseTimeUnit int64 = 24 * 60 * 60
	// AcceleratorDuration is the total duration of the accelerator
	// program.
	AcceleratorDuration = 20 * 12 * 30 * PhaseTimeUnit
	// VoteAcceptanceThreshold is the percent of pillar votes (out of
	// 100) required for an accelerator project phase to pass.
	VoteAcceptanceThreshold uint32 = 33
	// AcceleratorProjectVotingPeriod is the wall-clock duration of an
	// accelerator project's voting window.
	AcceleratorProjectVotingPeriod = 14 * PhaseTimeUnit
	// MaxBlocksPerUpdate caps how many blocks one update call may
	// process at once.
	MaxBlocksPerUpdate = 40

	// === Pillar constants ===

	// PillarStakeAmount is the ZNN amount a pillar must lock to
	// register.
	PillarStakeAmount = big.NewInt(15e3 * Decimals)
	// PillarQsrStakeBaseAmount is the amount of QSR used for legacy
	// pillars and for the first pillar in the network.
	PillarQsrStakeBaseAmount = big.NewInt(150e3 * Decimals)
	// PillarQsrStakeIncreaseAmount is the increase of cost for each
	// pillar after PillarsQsrStakeNumWithInitial.
	PillarQsrStakeIncreaseAmount = big.NewInt(10e3 * Decimals)
	// PillarEpochLockTime is the minimum lock duration (seconds)
	// before a pillar's stake can be revoked.
	PillarEpochLockTime int64 = 83 * SecsInDay
	// PillarEpochRevokeTime is the cooldown window (seconds) between
	// the lock expiration and the revoke effective time.
	PillarEpochRevokeTime int64 = 7 * SecsInDay
	// PillarNameLengthMax is the maximum byte length of a pillar's
	// display name.
	PillarNameLengthMax = 40

	// === Sentinel constants ===

	// SentinelZnnRegisterAmount is the ZNN amount required to
	// register a sentinel.
	SentinelZnnRegisterAmount = big.NewInt(5e3 * Decimals)
	// SentinelQsrDepositAmount is the QSR amount required to register
	// a sentinel.
	SentinelQsrDepositAmount = big.NewInt(50e3 * Decimals)
	// SentinelLockTimeWindow is the minimum lock duration (seconds)
	// before a sentinel's stake can be revoked.
	SentinelLockTimeWindow int64 = 27 * SecsInDay
	// SentinelRevokeTimeWindow is the cooldown window (seconds)
	// between the lock expiration and the revoke effective time.
	SentinelRevokeTimeWindow int64 = 3 * SecsInDay

	// === Staking constants ===

	// StakeTimeUnitSec is the staking-duration unit; durations must
	// be a multiple of this. Testnet value.
	StakeTimeUnitSec int64 = 30 * SecsInDay
	// StakeTimeMinSec is the minimum staking duration.
	StakeTimeMinSec = StakeTimeUnitSec * 1
	// StakeTimeMaxSec is the maximum staking duration (12 units).
	StakeTimeMaxSec = StakeTimeUnitSec * 12
	// StakeMinAmount is the minimum ZNN amount one stake may lock.
	StakeMinAmount = big.NewInt(1 * Decimals)

	// === Plasma constants ===

	// FuseMinAmount is the minimum QSR amount that may be fused at
	// once.
	FuseMinAmount = big.NewInt(10 * Decimals)
	// FuseExpiration is the minimum lock duration (in momentums)
	// before a fusion can be unfused. For testnet, 10 hours.
	FuseExpiration = uint64(MomentumsPerHour * 10)

	// === Token constants ===

	// TokenIssueAmount is the ZNN issuance fee charged when issuing a
	// new ZTS token.
	TokenIssueAmount = big.NewInt(1 * Decimals)
	// TokenNameLengthMax is the maximum length of a token name.
	TokenNameLengthMax = 40
	// TokenSymbolLengthMax is the maximum length of a token symbol.
	TokenSymbolLengthMax = 10
	// TokenDomainLengthMax is the maximum length of a token domain.
	TokenDomainLengthMax = 128
	// TokenMaxSupplyBig is the maximum representable token supply
	// (2^255 - 1, matching the [common.BigP255m1] cap).
	TokenMaxSupplyBig = common.BigP255m1
	// TokenMaxDecimals is the maximum decimals a ZTS token may
	// declare.
	TokenMaxDecimals = 18

	// === Spork constants ===

	// SporkMinHeightDelay is the minimum number of momentums between
	// the spork-creation block and its enforcement height.
	SporkMinHeightDelay = uint64(6)
	// SporkNameMinLength is the minimum byte length of a spork name.
	SporkNameMinLength = 5
	// SporkNameMaxLength is the maximum byte length of a spork name.
	SporkNameMaxLength = 40
	// SporkDescriptionMaxLength is the maximum byte length of a
	// spork description.
	SporkDescriptionMaxLength = 400

	// === Swap constants ===

	// SwapAssetDecayEpochsOffset is the number of epochs before the
	// swap-asset decay kicks in.
	SwapAssetDecayEpochsOffset = 30 * 3
	// SwapAssetDecayTickEpochs is the number of epochs for each decay
	// tick.
	SwapAssetDecayTickEpochs = 30
	// SwapAssetDecayTickValuePercentage is the percentage that is
	// lost in each tick — equal to 10% per
	// [SwapAssetDecayTickEpochs], after [SwapAssetDecayEpochsOffset].
	SwapAssetDecayTickValuePercentage = 10

	// === Bridge constants ===

	// InitialBridgeAdministrator is the bridge administrator address
	// active at chain birth before governance can rotate it.
	InitialBridgeAdministrator = types.ParseAddressPanic("z1qr9vtwsfr2n0nsxl2nfh6l5esqjh2wfj85cfq9")
	// MaximumFee is the maximum bridge fee (in bps, denominator 10000).
	MaximumFee = uint32(10000)
	// MinUnhaltDurationInMomentums is the minimum number of momentums
	// the bridge must remain unhalted between halts. Mainnet value.
	MinUnhaltDurationInMomentums = uint64(6 * MomentumsPerHour)
	// MinAdministratorDelay is the minimum delay (in momentums)
	// before a queued administrator change becomes active. Mainnet.
	MinAdministratorDelay = uint64(2 * MomentumsPerEpoch)
	// MinSoftDelay is the minimum delay (in momentums) before a queued
	// soft change becomes active. Mainnet value.
	MinSoftDelay = uint64(MomentumsPerEpoch)
	// MinGuardians is the minimum number of bridge guardians. Mainnet.
	MinGuardians = 5

	// DecompressedECDSAPubKeyLength is the byte length of an
	// uncompressed secp256k1 public key (0x04-prefixed).
	DecompressedECDSAPubKeyLength = 65
	// CompressedECDSAPubKeyLength is the byte length of a compressed
	// secp256k1 public key.
	CompressedECDSAPubKeyLength = 33
	// ECDSASignatureLength is the byte length of a secp256k1
	// signature (r || s || v).
	ECDSASignatureLength = 65

	// === Reward constants ===

	// RewardTickDurationInEpochs represents the duration (in epochs)
	// for each reward tick — the granularity at which the network
	// reward schedule progresses.
	RewardTickDurationInEpochs uint64 = 30

	// NetworkZnnRewardConfig is the per-tick ZNN reward schedule.
	// Index `i` is consumed at epoch tick `i`; once exhausted, the
	// last value applies in perpetuity.
	NetworkZnnRewardConfig = []int64{
		10 * MomentumsPerEpoch / 6 * Decimals,
		6 * MomentumsPerEpoch / 6 * Decimals,
		5 * MomentumsPerEpoch / 6 * Decimals,
		7 * MomentumsPerEpoch / 6 * Decimals,
		5 * MomentumsPerEpoch / 6 * Decimals,
		4 * MomentumsPerEpoch / 6 * Decimals,
		7 * MomentumsPerEpoch / 6 * Decimals,
		4 * MomentumsPerEpoch / 6 * Decimals,
		3 * MomentumsPerEpoch / 6 * Decimals,
		7 * MomentumsPerEpoch / 6 * Decimals,
		3 * MomentumsPerEpoch / 6 * Decimals,
	}

	// NetworkQsrRewardConfig is the per-tick QSR reward schedule.
	// Same indexing semantics as [NetworkZnnRewardConfig].
	NetworkQsrRewardConfig = []int64{
		20000 * Decimals,
		20000 * Decimals,
		20000 * Decimals,
		20000 * Decimals,
		15000 * Decimals,
		15000 * Decimals,
		15000 * Decimals,
		5000 * Decimals,
	}

	// DelegationZnnRewardPercentage is the percentage of the per-epoch
	// ZNN reward distributed to delegators.
	DelegationZnnRewardPercentage int64 = 24
	// MomentumProducingZnnRewardPercentage is the percentage of the
	// per-epoch ZNN reward distributed to producing pillars.
	MomentumProducingZnnRewardPercentage int64 = 50
	// SentinelZnnRewardPercentage is the percentage of the per-epoch
	// ZNN reward distributed to sentinels.
	SentinelZnnRewardPercentage int64 = 13
	// LiquidityZnnRewardPercentage is the percentage of the per-epoch
	// ZNN reward distributed to the liquidity program.
	LiquidityZnnRewardPercentage int64 = 13
	// LiquidityZnnTotalPercentages is the denominator for liquidity
	// ZNN reward shares (basis points: 10000 = 100%).
	LiquidityZnnTotalPercentages uint32 = 10000

	// StakingQsrRewardPercentage is the percentage of the per-epoch
	// QSR reward distributed to stakers.
	StakingQsrRewardPercentage int64 = 50
	// SentinelQsrRewardPercentage is the percentage of the per-epoch
	// QSR reward distributed to sentinels.
	SentinelQsrRewardPercentage int64 = 25
	// LiquidityQsrRewardPercentage is the percentage of the per-epoch
	// QSR reward distributed to the liquidity program.
	LiquidityQsrRewardPercentage int64 = 25
	// LiquidityQsrTotalPercentages is the denominator for liquidity
	// QSR reward shares (basis points: 10000 = 100%).
	LiquidityQsrTotalPercentages uint32 = 10000
	// LiquidityStakeWeights is the per-stake-tier weight schedule
	// used by the liquidity contract; index 0 is unstaked, index 12
	// is the longest tier.
	LiquidityStakeWeights = []int64{
		0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12,
	}
)

// NetworkZnnRewardPerEpoch returns the per-epoch ZNN reward at the
// supplied epoch number, indexing into [NetworkZnnRewardConfig] by
// `epoch / RewardTickDurationInEpochs`. Once the schedule runs out the
// last value applies in perpetuity.
func NetworkZnnRewardPerEpoch(epoch uint64) int64 {
	tick := int(epoch / RewardTickDurationInEpochs)
	if tick >= len(NetworkZnnRewardConfig) {
		return NetworkZnnRewardConfig[len(NetworkZnnRewardConfig)-1]
	} else {
		return NetworkZnnRewardConfig[tick]
	}
}

// NetworkQsrRewardPerEpoch is the QSR analog of
// [NetworkZnnRewardPerEpoch].
func NetworkQsrRewardPerEpoch(epoch uint64) int64 {
	tick := int(epoch / RewardTickDurationInEpochs)
	if tick >= len(NetworkQsrRewardConfig) {
		return NetworkQsrRewardConfig[len(NetworkQsrRewardConfig)-1]
	} else {
		return NetworkQsrRewardConfig[tick]
	}
}

// PillarRewardPerMomentum returns delegation & producing reward per
// momentum: the per-epoch ZNN reward times the respective percentage,
// divided by [MomentumsPerEpoch] to give the per-block share.
func PillarRewardPerMomentum(epoch uint64) (*big.Int, *big.Int) {
	delegation := (NetworkZnnRewardPerEpoch(epoch) * DelegationZnnRewardPercentage) / 100 / MomentumsPerEpoch
	producing := (NetworkZnnRewardPerEpoch(epoch) * MomentumProducingZnnRewardPercentage) / 100 / MomentumsPerEpoch
	return big.NewInt(delegation), big.NewInt(producing)
}

// SentinelRewardForEpoch returns sentinel ZNN and QSR reward for a
// specific epoch.
func SentinelRewardForEpoch(epoch uint64) (*big.Int, *big.Int) {
	znn := (NetworkZnnRewardPerEpoch(epoch) * SentinelZnnRewardPercentage) / 100
	qsr := (NetworkQsrRewardPerEpoch(epoch) * SentinelQsrRewardPercentage) / 100
	return big.NewInt(znn), big.NewInt(qsr)
}

// LiquidityRewardForEpoch returns liquidity-program ZNN and QSR
// reward for a specific epoch.
func LiquidityRewardForEpoch(epoch uint64) (*big.Int, *big.Int) {
	znn := (NetworkZnnRewardPerEpoch(epoch) * LiquidityZnnRewardPercentage) / 100
	qsr := (NetworkQsrRewardPerEpoch(epoch) * LiquidityQsrRewardPercentage) / 100
	return big.NewInt(znn), big.NewInt(qsr)
}

// StakeQsrRewardPerEpoch returns the staking QSR reward for a
// specific epoch.
func StakeQsrRewardPerEpoch(epoch uint64) *big.Int {
	qsr := (NetworkQsrRewardPerEpoch(epoch) * StakingQsrRewardPercentage) / 100
	return big.NewInt(qsr)
}
