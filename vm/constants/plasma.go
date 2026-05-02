package constants

import "math/big"

// PlasmaTable is used to query plasma used by op code and transactions.
// Each contract method's [embedded.Method.GetPlasma] receives a
// [PlasmaTable] so the plasma cost of a call is configurable per build.
type PlasmaTable struct {
	// TxPlasma is the per-block base plasma cost.
	TxPlasma uint64
	// TxDataPlasma is the per-byte data surcharge for plain transfers.
	TxDataPlasma uint64

	// Embedded plasma costs

	// EmbeddedSimple is the plasma cost for a simple embedded call
	// (no auto-receive descendants).
	EmbeddedSimple uint64
	// EmbeddedWWithdraw is the cost for a call that triggers a
	// single-block auto-response (e.g., reward withdrawal).
	EmbeddedWWithdraw uint64
	// EmbeddedWDoubleWithdraw is the cost for a call that triggers
	// two-block auto-responses (e.g., a paired ZNN/QSR reward
	// withdrawal).
	EmbeddedWDoubleWithdraw uint64
}

// AlphanetPlasmaTable is the live plasma table the alphanet build
// uses; consumed by [github.com/zenon-network/go-zenon/vm.GetBasePlasmaForAccountBlock].
var (
	AlphanetPlasmaTable = PlasmaTable{
		TxPlasma:     AccountBlockBasePlasma,
		TxDataPlasma: ABByteDataPlasma,

		EmbeddedSimple:          EmbeddedSimplePlasma,
		EmbeddedWWithdraw:       EmbeddedWResponse,
		EmbeddedWDoubleWithdraw: EmbeddedWDoubleResponse,
	}
)

// Plasma cost constants. The base/data costs deliberately mirror
// Ethereum's gas costs for ergonomic familiarity.
const (
	// AccountBlockBasePlasma is the base plasma cost per account
	// block.
	AccountBlockBasePlasma = 21000
	// ABByteDataPlasma is the per-byte plasma cost for the account
	// block's Data field on plain transfers.
	ABByteDataPlasma = 68

	// EmbeddedSimplePlasma is the contract-call cost for methods that
	// emit no descendants.
	EmbeddedSimplePlasma = 2.5 * AccountBlockBasePlasma
	// EmbeddedWResponse is the cost for methods that emit a single
	// descendant send (typical for one-token reward withdrawals).
	EmbeddedWResponse = 3.5 * AccountBlockBasePlasma
	// EmbeddedWDoubleResponse is the cost for methods that emit two
	// descendant sends (e.g., paired ZNN/QSR rewards).
	EmbeddedWDoubleResponse = 4.5 * AccountBlockBasePlasma

	// NumFusionUnitsForBasePlasma is the number of fusion units that
	// produce the per-block base plasma — i.e., 10 fusion units cover
	// one minimum-cost block.
	NumFusionUnitsForBasePlasma = 10
	// PlasmaPerFusionUnit is the plasma yielded per fusion unit.
	PlasmaPerFusionUnit = AccountBlockBasePlasma / NumFusionUnitsForBasePlasma
	// CostPerFusionUnit is the QSR amount (in atomic units) one
	// fusion unit costs.
	CostPerFusionUnit = 100000000

	// PoWDifficultyPerPlasma is the PoW difficulty needed per unit of
	// plasma earned via proof of work.
	PoWDifficultyPerPlasma = 1500

	// MaxDataLength defines limit of account-block data to 16Kb.
	MaxDataLength = 1024 * 16

	// MaxPlasmaForAccountBlock defines max available plasma for an
	// account block.
	MaxPlasmaForAccountBlock = MaxFusionPlasmaForAccount

	// MaxPoWPlasmaForAccountBlock caps how much plasma a single
	// account block may earn from PoW alone.
	MaxPoWPlasmaForAccountBlock = EmbeddedWDoubleResponse
	// MaxDifficultyForAccountBlock is the PoW difficulty cap that
	// produces [MaxPoWPlasmaForAccountBlock] plasma.
	MaxDifficultyForAccountBlock = MaxPoWPlasmaForAccountBlock * PoWDifficultyPerPlasma

	// MaxFusionUnitsPerAccount limits each account to a maximum of
	// 5000 fusion units. All units above this will not increase the
	// maximum plasma.
	MaxFusionUnitsPerAccount = 5000
	// MaxFusionPlasmaForAccount is the per-account plasma cap that
	// follows from [MaxFusionUnitsPerAccount].
	MaxFusionPlasmaForAccount = MaxFusionUnitsPerAccount * PlasmaPerFusionUnit
	// MaxFussedAmountForAccount is the QSR amount (atomic units) that
	// reaches [MaxFusionUnitsPerAccount] when fused.
	MaxFussedAmountForAccount = CostPerFusionUnit * MaxFusionUnitsPerAccount
)

// MaxFussedAmountForAccountBig is the [big.Int] form of
// [MaxFussedAmountForAccount] for callers (e.g.,
// [github.com/zenon-network/go-zenon/vm.AvailablePlasma]) that
// compare against arbitrary-precision integers.
var (
	MaxFussedAmountForAccountBig = big.NewInt(MaxFussedAmountForAccount)
)
