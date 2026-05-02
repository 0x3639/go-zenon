package constants

import (
	"github.com/zenon-network/go-zenon/common/types"
)

// Consensus bundles the chain-wide tuning knobs the consensus layer
// reads. Held in a struct so tests can swap variants in via
// [ConsensusConfig].
type Consensus struct {
	// BlockTime is the wall-clock interval between two momentums, in
	// seconds.
	BlockTime int64
	// NodeCount is the number of pillars elected per tick. Each tick
	// schedules NodeCount momentums.
	NodeCount uint8
	// RandCount is the number of pillars promoted from group B (lower
	// weight) into the slate per tick. The remaining
	// (NodeCount - RandCount) slots come from the top-weight group.
	RandCount uint8
	// CountingZTS is the token used to compute pillar weights — every
	// backer's balance of this token contributes to the pillar they
	// delegate to.
	CountingZTS types.ZenonTokenStandard
}

// ConsensusConfig is the live configuration consumed by the consensus
// layer. The values here are alphanet defaults: 30 pillars per tick,
// 10s blocks (5-minute election cycles), 15 random promotions, ZNN as
// the counting token.
var (
	ConsensusConfig = &Consensus{
		BlockTime:   10,
		NodeCount:   30,
		RandCount:   15,
		CountingZTS: types.ZnnTokenStandard,
	}
)
