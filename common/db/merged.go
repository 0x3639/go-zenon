package db

import (
	"github.com/syndtr/goleveldb/leveldb"
	"github.com/syndtr/goleveldb/leveldb/comparer"
)

// newMergedDb stacks dbs into a read-through chain: lookups try each layer
// in order and return the first hit. Writes go to dbs[0] only; the
// remaining layers are read-only. The typical layout is `[memdb,
// levelDBSnapshot]`, giving a writable overlay in front of an immutable
// snapshot.
func newMergedDb(dbs []db) db {
	return &mergedDB{
		dbs: dbs,
	}
}

// mergedDB is the [db] returned by [newMergedDb].
type mergedDB struct {
	dbs []db
}

// Get returns the value at key from the first layer that has it, or
// [leveldb.ErrNotFound] if no layer has it.
func (u *mergedDB) Get(key []byte) ([]byte, error) {
	for _, db := range u.dbs {
		if ok, err := db.Has(key); err != nil {
			return nil, err
		} else if ok {
			return db.Get(key)
		}
	}
	return nil, leveldb.ErrNotFound
}

// Has reports whether any layer contains key.
func (u *mergedDB) Has(key []byte) (bool, error) {
	for _, db := range u.dbs {
		if ok, err := db.Has(key); err != nil {
			return false, err
		} else if ok {
			return true, nil
		}
	}
	return false, nil
}

// Put writes key/value to the topmost (writable) layer.
func (u *mergedDB) Put(key, value []byte) error {
	return u.dbs[0].Put(key, value)
}

// NewIterator returns an iterator that walks every key in lexicographic
// order across all layers, deduplicating by key (the topmost layer wins).
func (u *mergedDB) NewIterator(prefix []byte) StorageIterator {
	iterators := make([]StorageIterator, len(u.dbs))
	for i := range u.dbs {
		iterators[i] = u.dbs[i].NewIterator(prefix)
	}
	return newMergedIterator(iterators)
}

// changesInternal returns the changes captured by the writable top layer
// only — read-only layers contribute their existing state, not change-set.
func (u *mergedDB) changesInternal(prefix []byte) (Patch, error) {
	return u.dbs[0].changesInternal(prefix)
}

// Sentinel values for [mergedIterator.current] and per-iterator status.
const (
	// noCurrent indicates the merged iterator has no active per-layer
	// iterator (initial state, or every layer is exhausted).
	noCurrent = -1
	// iteratorFinished marks a per-layer iterator as exhausted; the merge
	// algorithm skips it on subsequent steps.
	iteratorFinished = 1
)

// mergedIterator walks multiple [StorageIterator]s in lockstep, presenting
// a single sorted, deduplicated stream. The earlier iterator in the input
// list shadows later iterators on key collision (matching the read-through
// precedence of [mergedDB.Get]).
type mergedIterator struct {
	cmp comparer.BasicComparer

	iterators []StorageIterator
	current   int
	status    []byte

	err error
}

// newMergedIterator advances every per-layer iterator to its first key,
// captures any per-layer error, and returns the merge iterator ready for
// [mergedIterator.Next].
func newMergedIterator(iterators []StorageIterator) StorageIterator {
	mi := &mergedIterator{
		cmp:       comparer.DefaultComparer,
		iterators: iterators,
		status:    make([]byte, len(iterators)),
		current:   noCurrent,
	}

	for index, i := range iterators {
		if !i.Next() {
			if err := i.Error(); err != nil && err != leveldb.ErrNotFound {
				mi.err = err
			}
			mi.status[index] = iteratorFinished
		}
	}
	return mi
}

// Next advances to the next merged key; returns false when every per-layer
// iterator is exhausted or an error is sticky.
func (mi *mergedIterator) Next() bool {
	return mi.step()
}

// Key returns the current merged key, or nil if there is no current key or
// an error has been captured.
func (mi *mergedIterator) Key() []byte {
	if mi.current == noCurrent || mi.err != nil {
		return nil
	}
	return mi.iterators[mi.current].Key()
}

// Value returns the current merged value, or nil if there is no current key
// or an error has been captured.
func (mi *mergedIterator) Value() []byte {
	if mi.current == noCurrent || mi.err != nil {
		return nil
	}
	return mi.iterators[mi.current].Value()
}

// Error returns the first per-layer error encountered, if any.
func (mi *mergedIterator) Error() error {
	return mi.err
}

// Release frees every per-layer iterator. Safe to call exactly once.
func (mi *mergedIterator) Release() {
	for _, iter := range mi.iterators {
		iter.Release()
	}
}

// step picks the next key in lexicographic order across all layers,
// advancing every per-layer iterator that was sitting on the previous key
// (so duplicates collapse into a single emitted entry).
func (mi *mergedIterator) step() bool {
	if mi.err != nil {
		return false
	}

	// call next on all iterators which have the key equal to current iterator before going forward
	if mi.current != noCurrent {
		i := mi.iterators[mi.current]
		key := make([]byte, len(i.Key()))
		copy(key, i.Key())

		for index, i := range mi.iterators {
			if mi.status[index] == iteratorFinished {
				continue
			}
			if mi.cmp.Compare(key, i.Key()) == 0 {
				if !i.Next() {
					if err := i.Error(); err != nil && err != leveldb.ErrNotFound {
						mi.err = err
						return false
					}
					mi.status[index] = iteratorFinished
				}
			}
		}
	}

	bestIndex := noCurrent
	var bestKey []byte
	for index, iterator := range mi.iterators {
		if mi.status[index] == iteratorFinished {
			continue
		}

		key := make([]byte, len(iterator.Key()))
		copy(key, iterator.Key())

		if bestIndex == noCurrent || bestKey == nil || mi.cmp.Compare(key, bestKey) == -1 {
			bestIndex = index
			bestKey = key
		}
	}

	mi.current = bestIndex
	if bestIndex == noCurrent {
		return false
	}
	return true
}
