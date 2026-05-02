package pillar

import (
	"sync"
	"time"

	"github.com/zenon-network/go-zenon/chain"
	"github.com/zenon-network/go-zenon/chain/store"
	"github.com/zenon-network/go-zenon/common"
	"github.com/zenon-network/go-zenon/common/types"
	"github.com/zenon-network/go-zenon/consensus"
	"github.com/zenon-network/go-zenon/protocol"
	"github.com/zenon-network/go-zenon/vm"
	"github.com/zenon-network/go-zenon/wallet"
)

// worker is the per-pillar production engine. One Process call
// drives a complete slot's work (momentum + auto-receives + embedded
// updates). Init / Start / Stop control its lifetime: Start opens
// the closed channel; Stop closes it and waits for in-flight Process
// calls to drain via the children WaitGroup.
//
// The worker can run only one Process at a time — the working mutex
// serializes them and the embedded contracts list defines which
// addresses participate in the auto-receive sweep.
type worker struct {
	log      common.Logger
	closed   chan struct{}
	working  sync.Mutex
	children sync.WaitGroup

	contracts []types.Address
	coinbase  *wallet.KeyPair

	// modules
	chain       chain.Chain
	supervisor  *vm.Supervisor
	broadcaster protocol.Broadcaster
}

func newWorker(chain chain.Chain, supervisor *vm.Supervisor, broadcaster protocol.Broadcaster) *worker {
	return &worker{
		log:         common.PillarLogger.New("submodule", "worker"),
		contracts:   types.EmbeddedContracts,
		supervisor:  supervisor,
		chain:       chain,
		broadcaster: broadcaster,
	}
}

func (w *worker) Init() error {
	return nil
}
func (w *worker) Start() error {
	w.closed = make(chan struct{})
	w.log.Info("start contract worker")

	return nil
}
func (w *worker) Stop() error {
	close(w.closed)

	w.log.Info("stop all task")
	w.children.Wait()
	w.log.Info("end stop all task")
	w.log.Info("stopped")

	return nil
}

// shouldStop reports whether Stop has been called. Polled at every
// stage boundary in [worker.work] so a late Stop bails out promptly.
func (w *worker) shouldStop() bool {
	select {
	case <-w.closed:
		return true
	default:
	}
	return false
}

// Process schedules one slot of producer work for event e and
// returns a Task the caller can wait on (or force-stop). Returns
// nil if the worker has already been Stop'd.
func (w *worker) Process(e consensus.ProducerEvent) common.Task {
	w.children.Add(1)
	w.working.Lock()

	if w.shouldStop() {
		w.children.Done()
		w.working.Unlock()
		return nil
	}

	task := common.NewTask(func(task common.TaskResolver) {
		defer common.RecoverStack()
		w.work(task, e)
		w.children.Done()
		w.working.Unlock()
	})

	return task
}

// work is the actual slot pipeline: generate-momentum → broadcast
// → auto-receive embedded contracts → periodic-update embedded
// contracts. Bails out at every stage on task.ShouldStop or
// worker.shouldStop. Skips the broadcast step if the momentum took
// more than 3 seconds to generate (peers would reject it as
// stale).
func (w *worker) work(task common.TaskResolver, e consensus.ProducerEvent) {
	var momentumStore store.Momentum

	w.log.Info("producing momentum", "event", e)
	momentum, err := w.generateMomentum(e)
	if err != nil {
		w.log.Error("failed to generate momentum", "reason", err)
		return
	}

	if task.ShouldStop() {
		return
	}
	if w.shouldStop() {
		return
	}
	if common.Clock.Now().After(e.StartTime.Add(3 * time.Second)) {
		w.log.Error("do not broadcast own momentum", "identifier", momentum.Momentum.Identifier(), "reason", "too-late")
	} else {
		w.log.Info("broadcasting own momentum", "identifier", momentum.Momentum.Identifier())
		w.broadcaster.CreateMomentum(momentum)
	}

	if task.ShouldStop() {
		return
	}
	if w.shouldStop() {
		return
	}
	w.log.Info("start creating autoreceive blocks")
	momentumStore = w.chain.GetFrontierMomentumStore()
	for {
		one := false
		for _, contractAddress := range w.contracts {
			if task.ShouldStop() {
				return
			}
			if w.shouldStop() {
				return
			}

			transaction, err := w.generateNext(momentumStore, contractAddress)
			if err == ErrNothingToGenerate {
				continue
			}
			if err != nil {
				w.log.Error("unable to generate receive block for contract", "reason", err)
				return
			}
			w.broadcaster.CreateAccountBlock(transaction)
			w.log.Info("created autoreceive-block", "identifier", transaction.Block.Header())

			one = true
		}
		if !one {
			break
		}
	}

	if task.ShouldStop() {
		return
	}
	if w.shouldStop() {
		return
	}
	w.log.Info("checking if can update contracts")
	momentumStore = w.chain.GetFrontierMomentumStore()
	if err := w.updateContracts(momentumStore); err != nil {
		w.log.Error("failed to update contracts", "reason", err)
		return
	}
}
