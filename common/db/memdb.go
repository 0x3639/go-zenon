package db

import (
	"github.com/syndtr/goleveldb/leveldb/comparer"
	"github.com/syndtr/goleveldb/leveldb/memdb"
	"github.com/syndtr/goleveldb/leveldb/util"
)

// memDBWrapper adapts goleveldb's `*memdb.DB` (an in-memory ordered map) to
// the package's low-level [db] interface.
type memDBWrapper struct {
	*memdb.DB
}

// Has reports whether key is present in the in-memory store.
func (mdbw *memDBWrapper) Has(key []byte) (bool, error) {
	return mdbw.Contains(key), nil
}

// NewIterator walks every key/value beginning with prefix.
func (mdbw *memDBWrapper) NewIterator(prefix []byte) StorageIterator {
	return mdbw.DB.NewIterator(util.BytesPrefix(prefix))
}

// changesInternal returns a [Patch] capturing every key/value present under
// prefix. Used to derive a forward patch from an in-memory overlay before
// merging it into the durable layer.
func (mdbw *memDBWrapper) changesInternal(prefix []byte) (Patch, error) {
	p := NewPatch()
	iterator := mdbw.NewIterator(prefix)
	defer iterator.Release()

	for {
		if !iterator.Next() {
			if iterator.Error() != nil {
				return nil, iterator.Error()
			}
			break
		}

		value := iterator.Value()
		key := iterator.Key()
		p.Put(key, value)
	}

	return p, nil
}

// newMemDBInternal returns a low-level in-memory [db] for use in [mergedDB]
// stacks where the package wires deletion support separately.
func newMemDBInternal() db {
	return &memDBWrapper{
		DB: memdb.New(comparer.DefaultComparer, 0),
	}
}

// NewMemDB returns a fresh in-memory [DB] with deletion support enabled.
// Used by tests and by the manager's per-version overlays.
func NewMemDB() DB {
	return enableDelete(newMemDBInternal())
}
