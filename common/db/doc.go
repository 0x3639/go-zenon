// Package db is the versioned key/value layer used by the chain, consensus,
// and embedded-contract stores.
//
// # Overview
//
// db wraps goleveldb in two abstractions: a low-level [DB] interface that
// adds tombstone-based deletion, change-set capture, and snapshotting on top
// of LevelDB; and a higher-level [Manager] interface that composes [DB]
// snapshots with per-version forward and rollback [Patch]es so the chain can
// expose any committed [HashHeight] as a queryable historical view.
//
// Every chain mutation flows through this package: the VM produces a [Patch]
// per executed account block, the chain layer wraps it in a [Transaction]
// alongside the relevant [Commit]s, and [Manager.Add] persists the lot
// atomically. Reorgs use [Manager.Pop] to apply the matching rollback patch
// in the inverse direction.
//
// # Key Concepts
//
//   - DB — the high-level handle. Layered on top of [db] via
//     [enableDelete], adding tombstone-encoded deletion, [DB.Changes]
//     capture for in-memory overlays, and [DB.Snapshot] for cheap
//     point-in-time forks.
//   - Patch — a replayable batch of put/delete operations. Constructed by
//     the VM and committed by the manager.
//   - Commit — a chain object (account block, momentum) that knows its own
//     [HashHeight] and serialized form.
//   - Transaction — a sequence of [Commit]s plus the [Patch] capturing
//     their state effects. The unit [Manager.Add] consumes.
//   - Manager — the versioned database. Holds the live frontier, every
//     historical view in scope, and the forward / rollback patches needed
//     to navigate between them.
//   - SubDB — a namespace decorator that prefixes every key. Used to keep
//     unrelated keyspaces separate within a single LevelDB instance.
//   - MergedDB — a read-through stack of [db]s. The first layer is
//     writable; later layers are read-only. Used to put an in-memory
//     overlay in front of an immutable LevelDB snapshot.
//
// # Usage
//
// Open a backing store and wrap it in a manager:
//
//	mgr := db.NewLevelDBManager("data/nom")
//	defer mgr.Stop()
//
// Read at a specific height:
//
//	view := mgr.Get(types.HashHeight{Hash: h, Height: 42})
//	if view == nil { /* no longer reachable */ }
//	value, err := view.Get(key)
//
// Commit a transaction (typically built by the VM and chain layer):
//
//	if err := mgr.Add(tx); err != nil { /* handle */ }
//
// Tombstone-encoded deletion: writes through [DB.Delete] are persisted
// as a zero-length byte slice (the empty tombstone). Live values are
// stored as `existsByte || value` so the read path can distinguish
// the two. LevelDB snapshots remain immutable while still letting
// iteration skip deleted keys via [enableDeleteIterator]. (The
// low-level [patchApplierWO] writer uses a different encoding — a
// length-1 `existsByte` value — because it bypasses the high-level
// decorator.)
//
// # Concurrency
//
// Every method on a [Manager] is goroutine-safe; the manager owns the mutex
// that serializes [Manager.Add] / [Manager.Pop] against reads. Individual
// [DB] views are not reentrant — derive a fresh snapshot via [DB.Snapshot]
// per goroutine.
//
// # Related Packages
//
//   - [github.com/zenon-network/go-zenon/common/types] — defines
//     [types.HashHeight] and the protobuf wrappers patches serialize.
//   - [github.com/zenon-network/go-zenon/common] — provides the byte
//     helpers used to compose canonical key forms.
//   - [github.com/zenon-network/go-zenon/chain] — the primary consumer;
//     wraps every state mutation in a [Transaction] and commits via
//     [Manager.Add].
//   - [github.com/zenon-network/go-zenon/vm] — produces the [Patch] for
//     each executed account block; reads from [DB] views supplied by the
//     chain.
//   - [github.com/zenon-network/go-zenon/consensus/storage] — uses the
//     same primitives to persist consensus state.
package db
