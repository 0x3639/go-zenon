package chain

import (
	"sync"

	"github.com/zenon-network/go-zenon/chain/nom"
)

// momentumEventManager is the [MomentumEventManager] implementation: a
// dynamic list of [MomentumEventListener]s that the momentum pool
// broadcasts insert and delete events to.
//
// Concurrency: register/unregister/broadcast all take changes; the
// manager is safe for concurrent use. Listeners run synchronously on
// whichever goroutine drove the originating [momentumPool.AddMomentumTransaction]
// or [momentumPool.RollbackTo] call — heavy work should be deferred.
type momentumEventManager struct {
	listeners []MomentumEventListener
	changes   sync.Mutex
}

// newMomentumEventManager returns a fresh manager with no listeners.
func newMomentumEventManager() *momentumEventManager {
	return &momentumEventManager{
		listeners: make([]MomentumEventListener, 0),
	}
}

// broadcastInsertMomentum invokes [MomentumEventListener.InsertMomentum]
// on every registered listener. Called by [momentumPool.AddMomentumTransaction]
// after the underlying database mutation succeeds.
func (em *momentumEventManager) broadcastInsertMomentum(detailed *nom.DetailedMomentum) {
	em.changes.Lock()
	defer em.changes.Unlock()

	for _, listener := range em.listeners {
		listener.InsertMomentum(detailed)
	}
}

// broadcastDeleteMomentum invokes [MomentumEventListener.DeleteMomentum]
// on every registered listener. Called by [momentumPool.RollbackTo]
// after each rollback step.
func (em *momentumEventManager) broadcastDeleteMomentum(detailed *nom.DetailedMomentum) {
	em.changes.Lock()
	defer em.changes.Unlock()

	for _, listener := range em.listeners {
		listener.DeleteMomentum(detailed)
	}
}

// Register appends listener to the broadcast list. The same listener
// may be registered multiple times — the caller is responsible for
// idempotency.
func (em *momentumEventManager) Register(listener MomentumEventListener) {
	em.changes.Lock()
	defer em.changes.Unlock()

	em.listeners = append(em.listeners, listener)
}

// UnRegister removes the first occurrence of listener (by pointer
// equality) from the broadcast list. No-op if listener is not present.
func (em *momentumEventManager) UnRegister(listener MomentumEventListener) {
	em.changes.Lock()
	defer em.changes.Unlock()

	for index, current := range em.listeners {
		if current == listener {
			em.listeners = append(em.listeners[:index], em.listeners[index+1:]...)
			break
		}
	}
}
