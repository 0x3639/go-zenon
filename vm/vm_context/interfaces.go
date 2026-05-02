package vm_context

import (
	"math/big"

	"github.com/zenon-network/go-zenon/chain/nom"
	"github.com/zenon-network/go-zenon/chain/store"
	"github.com/zenon-network/go-zenon/common/types"
	"github.com/zenon-network/go-zenon/consensus/api"
)

// AccountVmContext is the per-account-block execution surface handed
// to embedded contracts and to the VM's send/receive paths. It bundles
// chain reads ([store.Account] for the account chain,
// [store.Momentum] for the surrounding chain state), consensus reads
// ([api.PillarReader] for pillar weights and epoch stats), balance
// helpers, lifecycle hooks (Save/Reset/Done for the contract-receive
// rollback path), and spork-status checks.
type AccountVmContext interface {
	api.PillarReader
	store.Account
	// MomentumStore returns the [store.Momentum] view this context is
	// pinned at — typically the [chain/nom.AccountBlock.MomentumAcknowledged]
	// of the block under execution.
	MomentumStore() store.Momentum

	// State

	// GetFrontierMomentum returns the most recent committed momentum.
	GetFrontierMomentum() (*nom.Momentum, error)
	// GetGenesisMomentum returns the genesis momentum.
	GetGenesisMomentum() *nom.Momentum

	// Lifecycle

	// Save snapshots the account view so subsequent writes can be
	// rolled back via [AccountVmContext.Reset]. Used by
	// contract-receive execution before invoking the contract method.
	Save()
	// Reset reverts to the snapshot taken by [AccountVmContext.Save].
	// Called when contract execution fails.
	Reset()
	// Done finalizes the snapshot: extracts the patch from the working
	// view and applies it onto the saved view. Called when contract
	// execution succeeds.
	Done()

	// Balance

	// AddBalance credits amount of ts to the executing account.
	AddBalance(ts *types.ZenonTokenStandard, amount *big.Int)
	// SubBalance debits amount of ts from the executing account.
	// Panics if the resulting balance would be negative — the VM is
	// expected to have checked enoughFunds beforehand.
	SubBalance(ts *types.ZenonTokenStandard, amount *big.Int)

	// Spork

	// IsAcceleratorSporkEnforced reports whether the accelerator spork
	// is active at this context's momentum height.
	IsAcceleratorSporkEnforced() bool
	// IsHtlcSporkEnforced reports whether the HTLC spork is active.
	IsHtlcSporkEnforced() bool
	// IsBridgeAndLiquiditySporkEnforced reports whether the bridge +
	// liquidity spork is active.
	IsBridgeAndLiquiditySporkEnforced() bool
}
