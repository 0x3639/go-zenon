package common

import "time"

// Clock is the package-level clock abstraction. Production binaries use a
// [realClock]; tests can swap in a deterministic clock to drive consensus
// ticks without waiting on wall time.
var (
	Clock ClockType
)

// ClockType is the minimal time-source interface. Tests substitute a fake
// implementation to advance time deterministically.
type ClockType interface {
	// Now returns the current time according to this clock.
	Now() time.Time
}

// realClock is the default [ClockType] backed by [time.Now].
//
// TODO: implementing `After(d time.Duration) <-chan time.Time` would allow
// us to better emulate the real world in a testing environment.
type realClock struct{}

// Now returns the current wall-clock time.
func (realClock) Now() time.Time { return time.Now() }

// init seeds the package-level [Clock] with a [realClock]. Tests reassign
// Clock before exercising consensus or other time-sensitive code paths.
func init() {
	Clock = new(realClock)
}
