package store

import (
	"github.com/zenon-network/go-zenon/chain/nom"
	"github.com/zenon-network/go-zenon/common/types"
)

// Genesis is the read-only surface for genesis-time invariants every
// momentum store must expose. Embedded into [Momentum] so callers always
// have access to the chain identifier and the genesis-block / spork-address
// constants without needing a separate handle.
type Genesis interface {
	// ChainIdentifier returns the network this chain belongs to (alphanet,
	// testnet, etc.) — bound into every block and momentum for replay
	// protection.
	ChainIdentifier() uint64
	// IsGenesisMomentum reports whether hash names the genesis momentum.
	IsGenesisMomentum(hash types.Hash) bool
	// GetGenesisMomentum returns the genesis momentum.
	GetGenesisMomentum() *nom.Momentum
	// GetGenesisTransaction returns the genesis [nom.MomentumTransaction]
	// — momentum plus the patch describing the initial state.
	GetGenesisTransaction() *nom.MomentumTransaction
	// GetSporkAddress returns the address authorized to create / activate
	// sporks. May be nil when the chain has no spork governance configured.
	GetSporkAddress() *types.Address
}
