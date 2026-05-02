package db

// skipDeletedDb is a low-level [db] decorator whose iterator skips
// tombstone-encoded entries. Used inside the [ldbManager.Get] read-through
// stack so an iterator over a historical view does not surface keys whose
// tombstone has already been honored on disk.
type skipDeletedDb struct {
	db
}

// NewIterator returns an iterator that skips tombstoned entries.
func (db *skipDeletedDb) NewIterator(prefix []byte) StorageIterator {
	return newSkipDeletedIterator(db.db.NewIterator(prefix))
}

// skipDeletedIterator filters out tombstoned values during iteration.
type skipDeletedIterator struct {
	StorageIterator
}

// Next advances past tombstoned entries, returning false only when every
// remaining key in the wrapped iterator is a tombstone.
func (i *skipDeletedIterator) Next() bool {
	for {
		if !i.StorageIterator.Next() {
			return false
		}
		val := i.StorageIterator.Value()
		if len(val) > 1 {
			return true
		}
	}
}

// newSkipDeletedIterator wraps iterator to drop tombstoned entries.
func newSkipDeletedIterator(iterator StorageIterator) StorageIterator {
	return &skipDeletedIterator{
		StorageIterator: iterator,
	}
}

// newSkipDelete wraps db so its iterator skips tombstoned entries.
func newSkipDelete(db db) db {
	return &skipDeletedDb{
		db: db,
	}
}
