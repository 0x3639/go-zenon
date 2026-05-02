package consensus

import (
	"time"

	"github.com/zenon-network/go-zenon/common"
	"github.com/zenon-network/go-zenon/vm/constants"
)

// Context bundles the consensus tick parameters used by the election
// algorithm and the points subsystem: a [common.Ticker] for time → tick
// conversion, the [constants.Consensus] tuning knobs (block time,
// node count, RandCount), and the chain's genesis time.
type Context struct {
	common.Ticker
	constants.Consensus
	GenesisTime time.Time
}

// NewConsensusContext builds a [Context] anchored at genesisTime,
// with the ticker interval derived from the consensus config:
// `BlockTime × NodeCount` seconds per tick (the wall-clock duration
// of one election cycle).
func NewConsensusContext(genesisTime time.Time) *Context {
	config := constants.ConsensusConfig
	context := &Context{
		Consensus:   *config,
		GenesisTime: genesisTime,
	}

	context.Ticker = common.NewTicker(genesisTime, time.Second*time.Duration(uint64(config.BlockTime)*uint64(config.NodeCount)))
	return context
}
