// Package common holds the shared utilities used across the codebase:
// structured logging, error helpers, byte and big.Int conversion
// primitives, a clock abstraction, ticker-based time conversion, a
// lightweight cancellable-task wrapper, and a small assertion framework
// used by the test suites.
//
// # Overview
//
// common is the second-deepest dependency after
// [github.com/zenon-network/go-zenon/common/types]. Subsystems consume it
// for:
//
//   - Logging — every subsystem logs through one of the per-module
//     [log15.Logger] handles defined here ([ChainLogger], [ConsensusLogger],
//     [VmLogger], …) so production output can be filtered by module.
//   - Byte and big.Int helpers — [JoinBytes], [Uint64ToBytes],
//     [BigIntToBytes], etc. compose the canonical key and hash forms used
//     by the chain and VM.
//   - Time and ticks — [Clock] is the swappable time source the consensus
//     layer reads through, and [Ticker] maps wall time onto the integer
//     tick numbers that pillar election operates on.
//   - Tasks — [NewTask] wraps a goroutine in a stop-able, joinable handle
//     used by long-running subsystem loops.
//   - Errors — [DealWithErr], [RecoverStack], [NewErrorWCode], and the
//     `Expect*` test helpers.
//
// # Key Concepts
//
//   - Logger — alias of [log15.Logger]; obtained via the per-subsystem
//     variables in this package, never via direct `log15.New` calls.
//   - Clock — package-level [ClockType] swapped to a fake during tests.
//   - Ticker — start-time + interval tick scheduler consumed by
//     [github.com/zenon-network/go-zenon/consensus].
//   - Task / TaskResolver — cooperative cancellation contract for
//     long-running goroutines.
//   - Expecter / `Expect*` helpers — testing assertions used by every
//     `*_test.go` in the codebase. They live in the production package so
//     non-test packages can be exercised from external test packages
//     without exposing testing internals through every import.
//
// # Usage
//
// Logging:
//
//	common.ChainLogger.Info("inserted momentum", "height", h, "hash", hash)
//
// Byte composition (canonical database key):
//
//	key := common.JoinBytes(common.Uint64ToBytes(height), hash.Bytes())
//
// Cancellable goroutine:
//
//	t := common.NewTask(func(r common.TaskResolver) {
//	    for !r.ShouldStop() {
//	        // do work
//	    }
//	})
//	defer t.ForceStop()
//
// Time:
//
//	now := common.Clock.Now()
//	ticker := common.NewTicker(genesisTime, 10*time.Second)
//	tick := ticker.ToTick(now)
//
// # Related Packages
//
//   - [github.com/zenon-network/go-zenon/common/types] — primitive
//     identifier types ([Hash], [Address], [HashHeight]).
//   - [github.com/zenon-network/go-zenon/common/db] — versioned LevelDB
//     manager. Uses [JoinBytes] and [Uint64ToBytes] for key composition
//     and [DealWithErr] at boundaries where errors are bugs.
//   - [github.com/zenon-network/go-zenon/common/crypto] — hashing /
//     signing primitives consumed by other packages alongside common.
//   - [github.com/zenon-network/go-zenon/consensus] — consumes [Ticker]
//     and [Clock] for tick scheduling.
package common
