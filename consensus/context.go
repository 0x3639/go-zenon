package consensus

import (
	"time"

	"github.com/zenon-network/go-zenon/common"
	"github.com/zenon-network/go-zenon/vm/constants"
)

type Context struct {
	common.Ticker
	constants.Consensus
	GenesisTime time.Time
}

func NewConsensusContext(genesisTime time.Time, configs ...*constants.Consensus) *Context {
	config := constants.ConsensusConfig
	if len(configs) > 0 && configs[0] != nil {
		config = configs[0]
	}
	context := &Context{
		Consensus:   *config,
		GenesisTime: genesisTime,
	}

	context.Ticker = common.NewTicker(genesisTime, time.Second*time.Duration(uint64(config.BlockTime)*uint64(config.NodeCount)))
	return context
}
