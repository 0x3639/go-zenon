// Package chain is the ledger orchestration layer of the Network of
// Momentum.
//
// # Overview
//
// chain owns the dual-ledger primitives — an in-memory pool of
// uncommitted account blocks and a persistent pool of committed
// momentums — and serializes every mutation through one global insert
// lock. All inserts pass through here:
//
//   - account blocks from RPC and from contract execution
//     ([accountPool.AddAccountBlockTransaction]);
//   - momentums from the elected pillar or from peer sync
//     ([momentumPool.AddMomentumTransaction]).
//
// The momentum-event manager broadcasts insert and delete events to
// subscribers (the account pool itself, the broadcaster, RPC
// subscriptions). A spork-not-implemented detection at boot and on
// every momentum insert will terminate the binary so an out-of-date
// node cannot continue to operate against an upgraded chain.
//
// # Key Concepts
//
//   - Chain — the public surface composing genesis store, account
//     pool, momentum pool, and event manager into one handle.
//   - Insert lock — the single mutex serializing every mutation. Held
//     via [Chain.AcquireInsert]; the returned [sync.Locker] is a
//     one-shot handle that panics on Lock().
//   - Account pool — per-address managers holding uncommitted blocks
//     on top of the stable layer. Inserts may rollback uncommitted
//     blocks via the [higherPriority] tie-break (TotalPlasma /
//     BasePlasma ratio, hash as final tie-break) but never confirmed
//     ones.
//   - Momentum pool — persistent versioned chain managed via
//     [common/db.Manager]; commit and rollback both broadcast events.
//   - Stable interface — abstracts the persistent layer the account
//     pool reads from. The momentum pool implements it; tests can
//     substitute a mock.
//   - GotAllActiveSporksImplemented — boot- and per-momentum check
//     that aborts the node if any active spork is unrecognized.
//   - MomentumEventListener — observers of insert/delete; run
//     synchronously on the broadcaster's goroutine.
//
// # Concurrency
//
// Every public method on [Chain] is safe for concurrent use. The
// account pool and momentum pool each have their own internal mutex on
// top of the global insert lock so reads do not block on mutations.
// Listener callbacks run with the source pool's mutex temporarily
// released to keep listener re-entry safe.
//
// # Related Packages
//
//   - [github.com/zenon-network/go-zenon/chain/store] — interface
//     contracts the chain hands out (Account, AccountMailbox,
//     Momentum, Genesis).
//   - [github.com/zenon-network/go-zenon/chain/account],
//     [.../chain/account/mailbox], [.../chain/momentum],
//     [.../chain/genesis] — implementations the chain composes.
//   - [github.com/zenon-network/go-zenon/chain/nom] — block model.
//   - [github.com/zenon-network/go-zenon/common/db] — versioned
//     LevelDB layer.
//   - [github.com/zenon-network/go-zenon/verifier] — runs against the
//     stores chain hands out.
//   - [github.com/zenon-network/go-zenon/protocol] — registers as a
//     [MomentumEventListener] to broadcast new momentums to peers.
package chain
