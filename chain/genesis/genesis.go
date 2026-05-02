package genesis

import (
	"github.com/zenon-network/go-zenon/chain/nom"
	"github.com/zenon-network/go-zenon/chain/store"
	"github.com/zenon-network/go-zenon/common/types"
)

// genesis is the [store.Genesis] implementation: it caches the
// fully-assembled genesis [nom.MomentumTransaction] (momentum + initial
// state patch) so callers can fetch the genesis momentum, hash, or
// transaction in O(1).
type genesis struct {
	config              *GenesisConfig
	momentumTransaction *nom.MomentumTransaction
}

// NewGenesis builds the genesis-time [store.Genesis] from config: runs
// every embedded contract's seeding logic against an in-memory account
// pool, assembles the genesis momentum from the resulting account
// blocks, and stores the whole bundle.
func NewGenesis(config *GenesisConfig) store.Genesis {
	accountPool := newGenesisAccountBlocks(config)
	momentumTransaction := newGenesisMomentum(config, accountPool)

	return &genesis{
		config:              config,
		momentumTransaction: momentumTransaction,
	}
}

// ChainIdentifier returns the chain identifier for the network this
// genesis describes.
func (g *genesis) ChainIdentifier() uint64 {
	return g.config.ChainIdentifier
}

// IsGenesisMomentum reports whether hash names the genesis momentum.
func (g *genesis) IsGenesisMomentum(hash types.Hash) bool {
	return hash == g.momentumTransaction.Momentum.Hash
}

// GetGenesisMomentum returns the genesis momentum.
func (g *genesis) GetGenesisMomentum() *nom.Momentum {
	return g.momentumTransaction.Momentum
}

// GetGenesisTransaction returns the full genesis [nom.MomentumTransaction]
// — momentum plus the patch that produces the initial state.
func (g *genesis) GetGenesisTransaction() *nom.MomentumTransaction {
	return g.momentumTransaction
}

// GetSporkAddress returns the configured spork-controlling address for
// this network. May be nil when the chain has no spork governance set up.
func (g *genesis) GetSporkAddress() *types.Address {
	return g.config.SporkAddress
}
