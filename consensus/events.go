package consensus

import (
	"sync"
)

// eventManager is the [EventManager] implementation: a dynamic list of
// [EventListener]s that the consensus tick scheduler broadcasts
// [ProducerEvent]s to.
//
// Concurrency: every public method acquires the `changes` mutex; the
// manager is safe for concurrent use. Listener callbacks run
// synchronously on the scheduler's goroutine — heavy work should be
// deferred.
type eventManager struct {
	listeners []EventListener
	changes   sync.Mutex
}

// newEventManager returns a fresh manager with no listeners.
func newEventManager() *eventManager {
	return &eventManager{
		listeners: make([]EventListener, 0),
	}
}

// broadcastNewProducerEvent invokes [EventListener.NewProducerEvent]
// on every registered listener in order. Called by the tick scheduler.
func (em *eventManager) broadcastNewProducerEvent(event ProducerEvent) {
	em.changes.Lock()
	defer em.changes.Unlock()

	for _, listener := range em.listeners {
		listener.NewProducerEvent(event)
	}
}

// Register appends listener to the broadcast list. Same-pointer
// listeners may be registered multiple times — caller is responsible
// for idempotency.
func (em *eventManager) Register(listener EventListener) {
	em.changes.Lock()
	defer em.changes.Unlock()

	em.listeners = append(em.listeners, listener)
}

// UnRegister removes the first occurrence of listener (by pointer
// equality) from the broadcast list. No-op if not registered.
func (em *eventManager) UnRegister(listener EventListener) {
	em.changes.Lock()
	defer em.changes.Unlock()

	for index, current := range em.listeners {
		if current == listener {
			em.listeners = append(em.listeners[:index], em.listeners[index+1:]...)
			break
		}
	}
}
