package consensus

import (
	"time"

	"github.com/zenon-network/go-zenon/chain/nom"
	"github.com/zenon-network/go-zenon/common/types"
	"github.com/zenon-network/go-zenon/consensus/api"
)

// Verifier is the interface that can verify block consensus. The
// [github.com/zenon-network/go-zenon/verifier] consumes it to confirm
// the producer of an inbound momentum is the pillar elected for that
// momentum's tick.
type Verifier interface {
	// VerifyMomentumProducer reports whether momentum.Producer matches
	// the pillar elected for momentum.Timestamp. Returns false (and no
	// error) on a clean mismatch.
	VerifyMomentumProducer(momentum *nom.Momentum) (bool, error)
}

// ProducerEvent is the per-tick signal the consensus layer broadcasts
// to subscribers (notably the [github.com/zenon-network/go-zenon/pillar]
// producer). It names the elected pillar's address (and display name)
// and the wall-clock window the producer is expected to operate in.
type ProducerEvent struct {
	StartTime time.Time
	EndTime   time.Time
	Producer  types.Address
	Name      string
}

// EventListener is the contract a subsystem implements to react to
// [ProducerEvent]s. The pillar registers one to learn when it has been
// elected.
type EventListener interface {
	// NewProducerEvent is called once per tick the consensus layer
	// observes; the producer's coinbase decides whether the listener
	// should act on the event.
	NewProducerEvent(ProducerEvent)
}

// EventManager is the registration surface for [EventListener]s.
type EventManager interface {
	// Register adds callback to the broadcast list. Idempotent on
	// pointer equality is the caller's responsibility.
	Register(callback EventListener)
	// UnRegister removes callback (by pointer equality) from the
	// broadcast list. No-op if not registered.
	UnRegister(callback EventListener)
}

// Consensus is the public surface of the consensus subsystem: producer
// election, points (pillar performance), and verifier integration.
//
// Concurrency: every method is safe for concurrent use. The internal
// election scheduler runs in a background goroutine started by [Start]
// and stopped by [Stop].
type Consensus interface {
	Verifier
	EventManager

	// Init is currently a no-op; reserved for future use. Returned for
	// symmetry with the chain / verifier subsystems.
	Init() error
	// Start launches the tick scheduler goroutine and registers the
	// consensus state listeners with the chain.
	Start() error
	// Stop stops the tick scheduler and unregisters listeners. Blocks
	// until the scheduler goroutine has exited.
	Stop() error

	// GetMomentumProducer returns the pillar address elected to produce
	// the momentum at timestamp.
	GetMomentumProducer(timestamp time.Time) (*types.Address, error)

	// FrontierPillarReader returns a read-only [api.PillarReader] view
	// pinned at the chain frontier.
	FrontierPillarReader() api.PillarReader
	// FixedPillarReader returns a read-only [api.PillarReader] view
	// pinned at the supplied identifier.
	FixedPillarReader(types.HashHeight) api.PillarReader
}
