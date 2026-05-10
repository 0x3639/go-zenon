package pillar

import (
	"fmt"
	"time"

	"github.com/inconshreveable/log15"

	"github.com/zenon-network/go-zenon/chain"
	"github.com/zenon-network/go-zenon/common"
	"github.com/zenon-network/go-zenon/common/types"
	"github.com/zenon-network/go-zenon/consensus"
	"github.com/zenon-network/go-zenon/protocol"
	"github.com/zenon-network/go-zenon/vm"
	"github.com/zenon-network/go-zenon/wallet"
)

// manager is the production [Manager] implementation. Wraps a
// single [worker] that handles the actual momentum / contract
// receive / contract update pipeline.
type manager struct {
	log      log15.Logger
	coinbase *wallet.KeyPair

	worker *worker

	consensus   consensus.Consensus
	broadcaster protocol.Broadcaster
}

// NewPillar constructs a pillar manager wired to the given chain,
// consensus, and broadcaster. The returned Manager is non-producing
// until [Manager.SetCoinBase] is called.
func NewPillar(chain chain.Chain, consensus consensus.Consensus, broadcaster protocol.Broadcaster) Manager {
	supervisor := vm.NewSupervisor(chain, consensus)
	return &manager{
		consensus:   consensus,
		broadcaster: broadcaster,
		worker:      newWorker(chain, supervisor, broadcaster),
		log:         common.PillarLogger.New("submodule", "manager"),
	}
}

// Init prepares the receiver for use.
func (m *manager) Init() error {
	m.log.Info("initializing ...")
	defer m.log.Info("initialized")

	if err := m.worker.Init(); err != nil {
		return err
	}

	return nil
}

// Start begins the receiver's background work.
func (m *manager) Start() error {
	m.log.Info("starting ...")
	defer m.log.Info("started")

	m.consensus.Register(m)
	if err := m.worker.Start(); err != nil {
		m.log.Error("failed to start contract worker", "reason", err)
	}

	return nil
}

// Stop tears down the receiver.
func (m *manager) Stop() error {
	m.log.Info("stopping ...")
	defer m.log.Info("stopped")

	m.consensus.UnRegister(m)
	if err := m.worker.Stop(); err != nil {
		return err
	}

	return nil
}

// NewProducerEvent is the [consensus.EventListener] callback —
// dispatches each event to a goroutine so the consensus loop
// continues without blocking on the producer pipeline.
func (m *manager) NewProducerEvent(e consensus.ProducerEvent) {
	go m.processSupervised(e)
}

// shouldProcess gates whether this pillar should act on event e.
// Returns one of the [errors.go] sentinels when the event is not
// actionable; nil means "produce now". Each rule is checked
// before any work begins so we never half-commit a momentum.
func (m *manager) shouldProcess(e consensus.ProducerEvent) error {
	if m.broadcaster.SyncInfo().State != protocol.SyncDone {
		return ErrSyncNotDone
	}
	if m.coinbase == nil {
		return ErrPillarNotDefined
	}
	if m.coinbase.Address != e.Producer {
		return ErrNotOurEvent
	}
	if common.Clock.Now().Before(e.StartTime) {
		return ErrEventHasNotStarted
	}
	if common.Clock.Now().After(e.EndTime) {
		return ErrEventEnded
	}
	return nil
}

// processSupervised drives one slot end-to-end: applies the
// admission rules, then waits for the worker task to finish, force-
// stopping it 250ms before slot EndTime so a late finish does not
// produce a momentum that other pillars will reject as expired.
func (m *manager) processSupervised(e consensus.ProducerEvent) {
	if err := m.shouldProcess(e); err != nil {
		m.log.Info("do not process current event", "event", e, "reason", err)
		return
	}

	fmt.Printf("Producing momentum ...\n")
	m.log.Info("momentum producer triggered", "event", e)
	defer m.log.Info("momentum producer trigger finished", "event", e)

	endTime := e.EndTime.Add(time.Millisecond * -250)
	task := m.worker.Process(e)
	for {
		select {
		case <-task.Finished():
			return
		case <-time.After(time.Millisecond * 100):
		}

		// Check for work expiration period
		if currentTime := time.Now(); currentTime.After(endTime) {
			m.log.Info("force-stopping producer task")
			task.ForceStop()
			break
		}
	}
}

// Process bypasses the admission rules and synchronously hands
// the event to the worker. Used by tests (and by [zenon/mock]) to
// drive the pipeline at virtual-clock speed.
func (m *manager) Process(e consensus.ProducerEvent) common.Task {
	// keep this section commented since it's used by the testing environment
	// when we find a nice way to move the clock in the future consider de-commenting this

	//if err := m.shouldProcess(e); err != nil {
	//	m.log.Error("do not process current event", "event", e, "reason", err)
	//	return nil
	//}
	return m.worker.Process(e)
}

// SetCoinBase configures the keypair this pillar produces under.
// Must be called before [Manager.Start] for the pillar to act on
// any ProducerEvent. Updates both the manager (used in admission
// checks) and the worker (used to sign produced blocks).
func (m *manager) SetCoinBase(coinbase *wallet.KeyPair) {
	m.coinbase = coinbase
	m.worker.coinbase = coinbase
}

// GetCoinBase returns the configured coinbase address, or nil for
// a non-producing pillar.
func (m *manager) GetCoinBase() *types.Address {
	if m.coinbase == nil {
		return nil
	}
	return &m.coinbase.Address
}
