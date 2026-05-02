// Package pillar implements the block-producing role on the Zenon
// network.
//
// # Overview
//
// A pillar listens for ProducerEvent emissions from
// [github.com/zenon-network/go-zenon/consensus] and, when its
// configured coinbase matches the elected pillar for the current
// tick, authors a [github.com/zenon-network/go-zenon/chain/nom.Momentum]
// over the pending account blocks. The momentum is committed via
// the chain insert lock and then handed to the
// [github.com/zenon-network/go-zenon/protocol.Broadcaster] for peer
// announcement.
//
// # Production Pipeline
//
// On each accepted ProducerEvent the worker runs three stages:
//
//  1. Generate momentum — assemble pending account blocks
//     ([worker.generateMomentum]), sign with the coinbase keypair,
//     commit through chain.AcquireInsert.
//  2. Auto-receive embedded contracts — for each embedded contract
//     with pending sends, generate a paired receive block via the
//     VM supervisor ([worker.generateNext]) until the sequencer
//     queue is drained.
//  3. Periodic embedded updates — for contracts that expose an
//     `Update` method (Pillar, Token, Plasma, etc.), call it if
//     [implementation.CanPerformUpdate] reports the cooldown has
//     elapsed ([worker.updateContracts]).
//
// Each stage checks [worker.shouldStop] and the per-task
// [common.TaskResolver] so the manager can force-stop the worker
// when the producer slot is about to expire.
//
// # Admission Rules
//
// [manager.shouldProcess] gates each ProducerEvent — events are
// dropped (with a log line) when:
//
//   - the node is not yet [protocol.SyncDone];
//   - no coinbase keypair is configured;
//   - the elected producer is not our coinbase;
//   - the slot's StartTime is in the future or EndTime in the past.
//
// # Concurrency
//
// One in-flight Process per worker is enforced by working sync.Mutex.
// Children sync.WaitGroup tracks Process goroutines so [worker.Stop]
// can drain them. The manager's [manager.processSupervised] uses the
// task's Finished channel plus a 250ms safety margin before slot
// EndTime to force-stop work that is about to overrun.
//
// # Generated Files
//
// None. Files are Zenon-specific (no upstream header).
//
// # Related Packages
//
//   - [github.com/zenon-network/go-zenon/consensus] — emits the
//     ProducerEvent stream this package consumes.
//   - [github.com/zenon-network/go-zenon/vm] — supervisor used to
//     generate momentums and contract-receive blocks.
//   - [github.com/zenon-network/go-zenon/protocol] — Broadcaster
//     used to commit momentums and account blocks.
//   - [github.com/zenon-network/go-zenon/zenon] — instantiates one
//     pillar per node when [zenon.Config.ProducingKeyPair] is set.
package pillar
