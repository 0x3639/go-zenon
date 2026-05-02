package db

import (
	"github.com/zenon-network/go-zenon/common/types"
)

// PatchReplayer receives the individual operations of a [Patch] when the
// patch is replayed. Implementations capture or transform the per-key
// operations, used by `patchApplier`, `patchRollback`, `patchPrinter`, and
// the value-prefixing helper to compose patch behaviors without copying.
type PatchReplayer interface {
	// Put records a write of value at key.
	Put(key []byte, value []byte)
	// Delete records the removal of key.
	Delete(key []byte)
}

// Patch is an ordered, replayable batch of put/delete operations against a
// [DB]. Patches are the unit of atomic state change throughout the chain:
// the VM produces a patch per executed account block, and the database
// manager commits the patch atomically alongside the block's serialized
// form.
type Patch interface {
	// Put appends a write of value at key.
	Put(key []byte, value []byte)
	// Delete appends a removal of key.
	Delete(key []byte)

	// Replay drives a [PatchReplayer] over every recorded operation in order.
	Replay(PatchReplayer) error
	// Dump serializes the patch to bytes; pair with [NewPatchFromDump].
	Dump() []byte
}

// Commit is the chain-side handle of any object that can be persisted as
// part of an atomic transaction: account blocks, momentums, or descendant
// blocks. Implementations expose their position on the chain
// ([HashHeight]) and the bytes the database stores at that position.
type Commit interface {
	// Identifier returns the [HashHeight] that uniquely names this commit.
	Identifier() types.HashHeight
	// Previous returns the [HashHeight] this commit chains from.
	Previous() types.HashHeight
	// Serialize returns the bytes to persist at Identifier.
	Serialize() ([]byte, error)
}

// Transaction bundles one or more [Commit]s with the [Patch] that captures
// their combined state changes. The chain layer constructs a transaction
// from VM output and hands it to [Manager.Add] for atomic insertion.
type Transaction interface {
	// GetCommits returns the commits in the transaction, in canonical order.
	GetCommits() []Commit
	// StealChanges returns the patch and clears the field on the
	// transaction; intended to transfer ownership without copying.
	StealChanges() Patch
}

// StorageIterator walks key/value pairs in lexicographic key order. Callers
// must call [StorageIterator.Release] when done; iterators returned from a
// [DB] may hold underlying snapshot resources.
type StorageIterator interface {
	// Next advances to the next pair and reports whether one was reached.
	Next() bool

	// Key returns the current key. Valid only after a successful Next.
	Key() []byte
	// Value returns the current value. Valid only after a successful Next.
	Value() []byte
	// Error returns any error encountered during iteration.
	Error() error
	// Release frees iterator resources. Safe to call exactly once.
	Release()
}

// DB is the higher-level key/value abstraction the chain consumes.
// Implementations layer on top of [db] to add the cross-cutting features
// every consumer expects: deletion (via tombstones), changes capture, and
// snapshotting.
type DB interface {
	// Get returns the value at key or [leveldb.ErrNotFound].
	Get([]byte) ([]byte, error)
	// Has reports whether key exists.
	Has([]byte) (bool, error)
	// Put writes value at key.
	Put(key, value []byte) error
	// Delete removes key (tombstone — keys may resurface in Snapshot views
	// when an underlying layer still has them).
	Delete(key []byte) error

	// NewIterator walks every key starting with prefix.
	NewIterator(prefix []byte) StorageIterator
	// Subset returns a view that transparently prepends prefix to every key.
	Subset(prefix []byte) DB

	// Apply replays patch onto this DB.
	Apply(Patch) error
	// Changes returns a patch describing every write made to this DB since
	// it was created (typically meaningful only for in-memory views layered
	// on top of an immutable snapshot).
	Changes() (Patch, error)
	// Snapshot returns an isolated copy that subsequent writes go into; the
	// underlying read-through layer remains shared.
	Snapshot() DB
}

// db is the lower-level interface implemented by raw backends ([levelDBWrapper],
// [memDBWrapper]) and by structural decorators ([subDB], [mergedDB],
// [skipDeletedDb]). Higher-level [DB] features are layered on via
// [enableDeleteDB].
type db interface {
	Get([]byte) ([]byte, error)
	Has([]byte) (bool, error)
	Put(key, value []byte) error

	NewIterator(prefix []byte) StorageIterator

	changesInternal(prefix []byte) (Patch, error)
}
