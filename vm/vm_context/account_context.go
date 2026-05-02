package vm_context

import (
	"github.com/zenon-network/go-zenon/chain/account"
	"github.com/zenon-network/go-zenon/chain/store"
	"github.com/zenon-network/go-zenon/common/db"
	"github.com/zenon-network/go-zenon/common/types"
	"github.com/zenon-network/go-zenon/consensus/api"
)

// accountVmContext is the [AccountVmContext] implementation. The
// embedded [store.Account] is the working view that contract code
// reads/writes; accountStoreSnapshot holds the saved view for
// [accountVmContext.Reset] / [accountVmContext.Done].
type accountVmContext struct {
	accountStoreSnapshot store.Account
	api.PillarReader
	store.Account
	momentumStore store.Momentum
}

// MomentumStore returns the momentum view this context is pinned at.
func (ctx *accountVmContext) MomentumStore() store.Momentum {
	return ctx.momentumStore
}

// NewAccountContext wires an [AccountVmContext] over the supplied
// momentum store, account view, and pillar reader. The supervisor
// constructs one per executed account block.
func NewAccountContext(momentumStore store.Momentum, accountBlock store.Account, pillarReader api.PillarReader) AccountVmContext {
	return &accountVmContext{
		momentumStore: momentumStore,
		Account:       accountBlock,
		PillarReader:  pillarReader,
	}
}

// NewGenesisAccountContext returns an [AccountVmContext] for an
// embedded contract being seeded at genesis: empty momentum store and
// pillar reader, fresh in-memory account view rooted at address.
// Used by [github.com/zenon-network/go-zenon/chain/genesis] when
// running each contract's seeding routine.
func NewGenesisAccountContext(address types.Address) AccountVmContext {
	return NewAccountContext(nil, account.NewAccountStore(address, db.NewMemDB()), nil)
}
