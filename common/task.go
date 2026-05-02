package common

import (
	"sync"
	"time"
)

// Task is a cancellable, joinable handle to a goroutine started via
// [NewTask]. Subsystems use it to launch background work whose lifetime is
// bounded by an explicit ForceStop.
type Task interface {
	// Wait blocks until the task's action returns.
	Wait()
	// Finished returns a channel that closes when the action returns.
	Finished() chan struct{}
	// ForceStop signals the task's TaskResolver to stop. The action must
	// poll [TaskResolver.ShouldStop] for the request to take effect.
	ForceStop()
}

// TaskResolver is the cooperation contract a task's action implements: it
// must check [TaskResolver.ShouldStop] periodically and exit promptly when
// it returns true.
type TaskResolver interface {
	// ShouldStop reports whether [Task.ForceStop] has been invoked.
	ShouldStop() bool
}

// NewTask launches action in a fresh goroutine and returns a [Task] that
// can be waited on or stopped. The action receives a [TaskResolver] it
// must cooperatively poll to honor stop requests.
func NewTask(action func(TaskResolver)) Task {
	t := &task{
		forceClosed: make(chan struct{}),
		closed:      make(chan struct{}),
	}

	go func() {
		action(t)
		t.finish()
	}()

	return t
}

// task is the [Task] / [TaskResolver] implementation backing [NewTask].
type task struct {
	forceClosed chan struct{}
	closed      chan struct{}
	changes     sync.Mutex
}

// Wait blocks until the goroutine's action returns. Polls every 100ms so
// that a goroutine that exits via panic-recovery still releases waiters.
func (t *task) Wait() {
	for {
		select {
		case <-t.closed:
			return
		case <-time.After(time.Millisecond * 100):
		}
	}
}

// Finished returns the channel closed once the task's action returns.
func (t *task) Finished() chan struct{} {
	return t.closed
}

// ForceStop signals stop. Idempotent; subsequent calls are no-ops.
//
// Concurrency: safe under t.changes — multiple callers may invoke ForceStop.
func (t *task) ForceStop() {
	t.changes.Lock()
	defer t.changes.Unlock()
	select {
	case <-t.forceClosed:
	default:
		close(t.forceClosed)
	}
}

// ShouldStop reports whether [task.ForceStop] has been invoked. Used by
// the task's action to decide whether to exit early.
func (t *task) ShouldStop() bool {
	select {
	case <-t.forceClosed:
		return true
	default:
		return false
	}
}

// finish closes the completion channel. Called once when the task's action
// returns.
func (t *task) finish() {
	t.changes.Lock()
	defer t.changes.Unlock()
	close(t.closed)
}
