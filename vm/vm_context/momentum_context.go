package vm_context

import (
	"github.com/zenon-network/go-zenon/chain/momentum"
	"github.com/zenon-network/go-zenon/chain/store"
)

// MomentumVMContext is the per-momentum execution surface used by
// [github.com/zenon-network/go-zenon/vm.MomentumVM]. It is exactly
// [store.Momentum] — momentum execution doesn't need balance helpers
// or contract-receive lifecycle hooks; admitting account-block
// transactions through [store.Momentum.AddAccountBlockTransaction]
// is the only operation.
type MomentumVMContext interface {
	store.Momentum
}

// momentumVMContext is the [MomentumVMContext] implementation;
// embeds [store.Momentum] directly so every store method passes
// through unchanged.
type momentumVMContext struct {
	store.Momentum
}

// NewMomentumVMContext wraps an existing momentum store in the
// per-momentum execution context. The supervisor passes the store
// pinned at the previous momentum so the new momentum's content
// applies onto a snapshot taken just before it.
func NewMomentumVMContext(store store.Momentum) MomentumVMContext {
	return &momentumVMContext{
		Momentum: store,
	}
}

// NewGenesisMomentumVMContext returns a [MomentumVMContext] backed
// by an empty in-memory store. Used by
// [github.com/zenon-network/go-zenon/vm.Supervisor.GenerateGenesisMomentum]
// where there is no previous momentum to read from.
func NewGenesisMomentumVMContext() MomentumVMContext {
	return &momentumVMContext{
		Momentum: momentum.NewGenesisStore(),
	}
}
