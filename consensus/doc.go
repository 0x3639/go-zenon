// Package consensus runs the pillar-election scheduler that determines
// which pillar produces each momentum, and aggregates per-pillar
// performance into period and epoch points.
//
// # Overview
//
// Time is divided into ticks; each tick maps deterministically to one
// elected pillar through a weighted shuffle of the registered pillar
// set. consensus owns:
//
//   - The tick scheduler — an unexported `work` loop on the
//     [consensus] type — runs in a background goroutine started by
//     [Consensus.Start] and broadcasts a [ProducerEvent] at every
//     elected pillar's start time.
//   - The election manager ([electionManager], [electionAlgorithm])
//     that resolves any tick to its producer slate. Results are cached
//     keyed by the proof block's hash so reorgs invalidate them
//     implicitly.
//   - The points subsystem ([Points], [periodPoints],
//     [compoundPoints]) that aggregates per-pillar performance per
//     period (one election cycle) and per epoch (24h by default).
//   - The verifier hook ([Consensus.VerifyMomentumProducer]) that
//     gates momentum acceptance on the elected-pillar match.
//   - A read-only [API] surface for RPC queries of weights, epoch
//     stats, and per-epoch delegations.
//
// # Key Concepts
//
//   - Tick — one election cycle; wall-clock duration is
//     `BlockTime × NodeCount` seconds (see [Context]).
//   - Proof block — the most recent momentum strictly before
//     `endTime(tick - 2)`. Using tick-2 prevents producers from
//     influencing their own elections by reordering blocks.
//   - Election algorithm — weighted top-N filter with a RandCount
//     promotion rule from group B and a deterministic random shuffle.
//     See [electionAlgorithm.SelectProducers].
//   - Period vs Epoch points — period is one tick; epoch is the
//     wall-clock aggregate of [EpochDuration] (default 24h). Compound
//     epoch points are derived by averaging the per-pillar weights of
//     the constituent period points.
//   - ProducerEvent — the per-pillar (start, end) window broadcast by
//     the tick scheduler so [github.com/zenon-network/go-zenon/pillar]
//     knows when to author a momentum.
//
// # Concurrency
//
// Every public method on [Consensus] is safe for concurrent use. The
// tick scheduler runs in its own goroutine started by [Consensus.Start]
// and stopped by [Consensus.Stop]. Listener callbacks
// ([EventListener.NewProducerEvent]) run synchronously on the
// scheduler's goroutine — heavy work should be deferred to a separate
// goroutine.
//
// # Related Packages
//
//   - [github.com/zenon-network/go-zenon/chain] — supplies the chain
//     handle the consensus layer reads frontier and momentum stores
//     from.
//   - [github.com/zenon-network/go-zenon/consensus/api] — read-only
//     API types ([api.PillarReader], [api.EpochStats]) the [API]
//     handle implements.
//   - [github.com/zenon-network/go-zenon/consensus/storage] — LevelDB
//     wrapper for cached election results and pre-computed points.
//   - [github.com/zenon-network/go-zenon/pillar] — registers an
//     [EventListener] and authors momentums when its coinbase matches.
//   - [github.com/zenon-network/go-zenon/verifier] — calls
//     [Consensus.VerifyMomentumProducer] during momentum-transaction
//     validation.
//   - [github.com/zenon-network/go-zenon/vm/constants] — defines
//     [constants.ConsensusConfig] (BlockTime, NodeCount, RandCount).
package consensus
