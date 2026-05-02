package zenon

import (
	"github.com/zenon-network/go-zenon/chain"
	"github.com/zenon-network/go-zenon/consensus"
	"github.com/zenon-network/go-zenon/pillar"
	"github.com/zenon-network/go-zenon/protocol"
	"github.com/zenon-network/go-zenon/verifier"
)

// Zenon is the facade exposed by the orchestrator. The production
// implementation is [zenon]; tests use the mock in
// [github.com/zenon-network/go-zenon/zenon/mock]. Methods returning
// subsystems give callers direct access to the underlying interfaces
// — no wrapping or reduced view.
type Zenon interface {
	Init() error
	Start() error
	Stop() error

	Chain() chain.Chain
	Consensus() consensus.Consensus
	Verifier() verifier.Verifier
	Protocol() *protocol.ProtocolManager
	Producer() pillar.Manager
	Config() *Config
	Broadcaster() protocol.Broadcaster
}
