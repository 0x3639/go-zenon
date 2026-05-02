package db

import (
	"github.com/syndtr/goleveldb/leveldb"
)

// disableNotFoundDB is a [DB] decorator that translates
// [leveldb.ErrNotFound] into an empty-byte-slice success. Used by callers
// that prefer to branch on `len(value) == 0` rather than on a specific
// error sentinel.
type disableNotFoundDB struct {
	DB
}

// Get reads key. Missing keys return ([]byte{}, nil) instead of
// (_, [leveldb.ErrNotFound]); other errors pass through unchanged.
func (d *disableNotFoundDB) Get(key []byte) ([]byte, error) {
	data, err := d.DB.Get(key)
	if err == leveldb.ErrNotFound {
		return []byte{}, nil
	}
	return data, err
}

// DisableNotFound wraps db so missing-key reads return an empty slice and
// nil error. Useful in code paths where absence is not exceptional.
func DisableNotFound(db DB) DB {
	return &disableNotFoundDB{DB: db}
}
