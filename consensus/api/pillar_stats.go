package api

import (
	"math/big"

	"github.com/zenon-network/go-zenon/common"
	"github.com/zenon-network/go-zenon/common/types"
)

// EpochPillarStats summarizes one pillar's performance over a single
// epoch. Used by the RPC pillar / stats endpoints to surface per-pillar
// reliability and reward inputs to clients.
type EpochPillarStats struct {
	Epoch            uint64   `json:"epoch"`
	BlockNum         uint64   `json:"blockNum"`
	ExceptedBlockNum uint64   `json:"exceptedBlockNum"`
	Weight           *big.Int `json:"weight"`
	Name             string   `json:"name"`
}

// EpochStats is the aggregated per-epoch view: every pillar's
// individual stats keyed by name, plus the totals across the network.
type EpochStats struct {
	Epoch       uint64                       `json:"epoch"`
	Pillars     map[string]*EpochPillarStats `json:"pillars"`
	TotalWeight *big.Int                     `json:"totalWeight"`
	// TotalBlocks is the total number of blocks generated in the epoch.
	TotalBlocks uint64 `json:"totalBlocks"`
}

// PillarReader is the read-only consensus surface the RPC layer
// consumes. Implementations bind to a specific point on the chain
// (frontier or fixed identifier) — see
// [github.com/zenon-network/go-zenon/consensus.Consensus.FrontierPillarReader]
// and [github.com/zenon-network/go-zenon/consensus.Consensus.FixedPillarReader].
type PillarReader interface {
	// GetPillarWeights returns the per-pillar election weight at the
	// most recently completed period. Map key is the pillar's name.
	GetPillarWeights() (map[string]*big.Int, error)
	// EpochTicker returns the [common.Ticker] consumers use to convert
	// times ↔ epoch numbers compatibly with the points subsystem.
	EpochTicker() common.Ticker
	// EpochStats returns aggregated per-pillar statistics for the
	// epoch. Returns nil when the epoch has not started yet.
	EpochStats(epoch uint64) (*EpochStats, error)
	// GetPillarDelegationsByEpoch returns the per-epoch averaged
	// delegation snapshot keyed by pillar name. Used by reward
	// calculations.
	GetPillarDelegationsByEpoch(epoch uint64) (map[string]*types.PillarDelegationDetail, error)
}
