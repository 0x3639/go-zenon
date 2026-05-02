package db

import (
	"runtime"

	"github.com/syndtr/goleveldb/leveldb"
	"github.com/syndtr/goleveldb/leveldb/iterator"
	"github.com/syndtr/goleveldb/leveldb/opt"
	"github.com/syndtr/goleveldb/leveldb/util"

	"github.com/zenon-network/go-zenon/common"
)

// getConsensusOpenFilesCacheCapacity returns the LevelDB open-files cache
// size for the consensus database. macOS has a tighter default ulimit so we
// cap it lower there.
func getConsensusOpenFilesCacheCapacity() int {
	switch runtime.GOOS {
	case "darwin":
		return 20
	case "windows":
		return 200
	default:
		return 200
	}
}

// LevelDBLikeRO is the read-only subset of `*leveldb.DB` that
// [levelDBROWrapper] adapts. Snapshots and the live database both satisfy
// this interface.
type LevelDBLikeRO interface {
	Get(key []byte, ro *opt.ReadOptions) (value []byte, err error)
	Has(key []byte, ro *opt.ReadOptions) (ret bool, err error)
	NewIterator(slice *util.Range, ro *opt.ReadOptions) iterator.Iterator
}

// levelDBROWrapper adapts a [LevelDBLikeRO] to the package's low-level [db]
// interface, so a snapshot can be merged into a [mergedDB] alongside a
// writable in-memory layer.
type levelDBROWrapper struct {
	db LevelDBLikeRO
}

// Get reads key from the wrapped read-only LevelDB.
func (ro *levelDBROWrapper) Get(key []byte) ([]byte, error) {
	return ro.db.Get(key, nil)
}

// Has reports whether key exists in the wrapped read-only LevelDB.
func (ro *levelDBROWrapper) Has(key []byte) (bool, error) {
	return ro.db.Has(key, nil)
}

// Put panics — a read-only wrapper must never be written to. Callers that
// need writes must layer a writable in-memory db in front via [newMergedDb].
func (ro *levelDBROWrapper) Put(key []byte, value []byte) error {
	panic("unimplemented")
}

// changesInternal panics — a read-only wrapper has no per-prefix change
// tracking; only the in-memory layer in front of it captures writes.
func (ro *levelDBROWrapper) changesInternal(prefix []byte) (Patch, error) {
	panic("unimplemented")
}

// NewIterator walks the read-only LevelDB starting at prefix.
func (ro *levelDBROWrapper) NewIterator(prefix []byte) StorageIterator {
	return ro.db.NewIterator(util.BytesPrefix(prefix), nil)
}

// LevelDBLike is the read-write subset of `*leveldb.DB` that
// [levelDBWrapper] adapts.
type LevelDBLike interface {
	LevelDBLikeRO
	Put(key []byte, value []byte, wo *opt.WriteOptions) error
}

// levelDBWrapper adapts a writable [LevelDBLike] to the package's low-level
// [db] interface.
type levelDBWrapper struct {
	db LevelDBLike
}

// Get reads key from the wrapped LevelDB.
func (ldbw *levelDBWrapper) Get(key []byte) ([]byte, error) {
	return ldbw.db.Get(key, nil)
}

// Has reports whether key exists in the wrapped LevelDB.
func (ldbw *levelDBWrapper) Has(key []byte) (bool, error) {
	return ldbw.db.Has(key, nil)
}

// Put writes key/value into the wrapped LevelDB.
func (ldbw *levelDBWrapper) Put(key, value []byte) error {
	return ldbw.db.Put(key, value, nil)
}

// NewIterator walks the wrapped LevelDB starting at prefix.
func (ldbw *levelDBWrapper) NewIterator(prefix []byte) StorageIterator {
	return ldbw.db.NewIterator(util.BytesPrefix(prefix), nil)
}

// changesInternal panics — direct LevelDB writes are committed
// transactionally elsewhere; the wrapper does not track them per-prefix.
func (ldbw *levelDBWrapper) changesInternal(prefix []byte) (Patch, error) {
	panic("unimplemented")
}

// newLevelDBSnapshotWrapper returns a low-level [db] view of a LevelDB
// snapshot with a writable in-memory overlay merged in front, so consumers
// can buffer writes against an immutable read-through layer.
func newLevelDBSnapshotWrapper(ldb *leveldb.Snapshot) db {
	return newMergedDb([]db{
		newMemDBInternal(),
		&levelDBROWrapper{
			db: ldb,
		},
	})
}

// NewLevelDBSnapshotWrapper returns the high-level [DB] view of a LevelDB
// snapshot, with deletion support enabled via the tombstone-byte encoding.
func NewLevelDBSnapshotWrapper(ldb *leveldb.Snapshot) DB {
	return enableDelete(newMergedDb([]db{
		newMemDBInternal(),
		&levelDBROWrapper{
			db: ldb,
		},
	}))
}

// NewLevelDBWrapper returns a high-level [DB] view of a writable LevelDB
// instance.
func NewLevelDBWrapper(db *leveldb.DB) DB {
	return enableDelete(
		&levelDBWrapper{
			db: db,
		})
}

// NewLevelDB opens (or creates) a LevelDB at dirname and returns both the
// high-level [DB] view and the underlying `*leveldb.DB` (for callers that
// need to take snapshots, close it, etc.). Panics on open failure.
func NewLevelDB(dirname string) (DB, *leveldb.DB) {
	opts := &opt.Options{OpenFilesCacheCapacity: getConsensusOpenFilesCacheCapacity()}
	db, err := leveldb.OpenFile(dirname, opts)
	common.DealWithErr(err)
	return NewLevelDBWrapper(db), db
}
