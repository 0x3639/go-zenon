package db

import (
	"github.com/zenon-network/go-zenon/common"
)

// removePatchKeyPrefix is a [PatchReplayer] that drops the leading
// prefixLength bytes of every key before passing the operation through to
// the wrapped [Patch]. Used by [subDB.changesInternal] to strip the
// namespace prefix when surfacing changes upstream.
type removePatchKeyPrefix struct {
	prefixLength int
	Patch
}

// Put writes key[prefixLength:] / value into the wrapped patch.
func (edp *removePatchKeyPrefix) Put(key []byte, value []byte) {
	edp.Patch.Put(key[edp.prefixLength:], value)
}

// subDB is a namespacing decorator: every read and write transparently has
// the prefix prepended, giving a logically isolated keyspace inside a
// shared underlying [db]. Used to keep the chain and meta keyspaces
// separate within a single LevelDB.
type subDB struct {
	prefix []byte
	db     db
}

// newSubDB returns a [db] that namespaces every key under prefix in db.
func newSubDB(prefix []byte, db db) db {
	return &subDB{
		prefix: prefix,
		db:     db,
	}
}

// Get reads (prefix||key) from the wrapped db.
func (u *subDB) Get(key []byte) ([]byte, error) {
	return u.db.Get(common.JoinBytes(u.prefix, key))
}

// Has reports whether (prefix||key) exists in the wrapped db.
func (u *subDB) Has(key []byte) (bool, error) {
	return u.db.Has(common.JoinBytes(u.prefix, key))
}

// Put writes (prefix||key) / value into the wrapped db.
func (u *subDB) Put(key, value []byte) error {
	return u.db.Put(common.JoinBytes(u.prefix, key), value)
}

// NewIterator walks every key beginning with (prefix||queryPrefix), with
// the namespace prefix stripped from each emitted key.
func (u *subDB) NewIterator(prefix []byte) StorageIterator {
	return newSubIterator(len(u.prefix), u.db.NewIterator(common.JoinBytes(u.prefix, prefix)))
}

// changesInternal returns the changes from the wrapped db restricted to the
// subDB's namespace, with the prefix stripped from each key so callers see
// the unprefixed view.
func (u *subDB) changesInternal(prefix []byte) (Patch, error) {
	changes, err := u.db.changesInternal(common.JoinBytes(u.prefix, prefix))
	if err != nil {
		return nil, err
	}

	p := &removePatchKeyPrefix{
		prefixLength: len(u.prefix),
		Patch:        NewPatch(),
	}

	if err := changes.Replay(p); err != nil {
		return nil, err
	}
	return p.Patch, nil
}

// subIterator strips the leading prefixLen bytes from each key emitted by
// the wrapped iterator. Values pass through unchanged.
type subIterator struct {
	prefixLen int
	StorageIterator
}

// Key returns the wrapped iterator's key with the namespace prefix removed.
func (si *subIterator) Key() []byte {
	return si.StorageIterator.Key()[si.prefixLen:]
}

// newSubIterator wraps iterator so emitted keys have their leading
// prefixLen bytes stripped.
func newSubIterator(prefixLen int, iterator StorageIterator) StorageIterator {
	return &subIterator{
		prefixLen:       prefixLen,
		StorageIterator: iterator,
	}
}
