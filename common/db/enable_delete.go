package db

import (
	"github.com/syndtr/goleveldb/leveldb"

	"github.com/zenon-network/go-zenon/common"
)

// existsByte is the one-byte LIVE-VALUE prefix that the [enableDeleteDB]
// decorator prepends onto every Put. Live entries are stored as
// `existsByte || value` (length ≥ 1); deletions are stored as the empty
// byte slice `[]byte{}` (length 0). The Get/Has/iterator paths treat
// any zero-length value as a tombstone (see [enableDeleteDB.Get],
// [enableDeleteDB.Has], [enableDeleteIterator.Value]).
//
// Note: existsByte is "0x00" both because it's a stable, unambiguous
// marker and because at the raw-LevelDB layer the writer in
// [patchApplierWO] also prepends a `0x00` to maintain the same shape
// when bypassing the high-level decorator.
var (
	existsByte = []byte{0}
)

// enableDeletePatch is a [PatchReplayer] that decodes tombstone-encoded
// patches back to the natural delete/put representation. Used by
// [enableDeleteDB.Changes] to re-surface the change-set after the
// tombstone-byte transform has been applied internally.
type enableDeletePatch struct {
	p Patch
}

// Put records a put or delete depending on whether value carries the
// tombstone byte.
func (p *enableDeletePatch) Put(key []byte, value []byte) {
	if len(value) == 0 {
		p.p.Delete(key)
	} else {
		p.p.Put(key, value[1:])
	}
}

// Delete is unreachable: the tombstone-encoded patch never emits raw
// deletes, only puts.
func (p *enableDeletePatch) Delete(key []byte) {
	panic("impossible")
}

// enableDeleteDB layers tombstone-based deletion on top of a low-level
// [db] that itself has no Delete operation. Live values are stored as
// `existsByte || value` (one prefix byte plus the original value);
// deleted keys are stored as the empty byte slice `[]byte{}` (zero
// length). This makes LevelDB snapshots representable as immutable
// data while still letting chain reorgs roll keys forward and back.
type enableDeleteDB struct {
	db db
}

// Has reports whether key has a live value (i.e., a non-tombstone entry).
func (d *enableDeleteDB) Has(key []byte) (bool, error) {
	data, err := d.db.Get(key)
	if err == leveldb.ErrNotFound {
		return false, nil
	} else if err != nil {
		return false, err
	}
	if len(data) == 0 {
		return false, nil
	}
	return true, nil
}

// Get returns the live value at key with the tombstone byte stripped.
// Returns [leveldb.ErrNotFound] for tombstoned or missing keys.
func (d *enableDeleteDB) Get(key []byte) ([]byte, error) {
	data, err := d.db.Get(key)
	if err != nil {
		return nil, err
	}
	if len(data) == 0 {
		return nil, leveldb.ErrNotFound
	}
	return data[1:], nil
}

// Put writes key with the live-value prefix prepended.
func (d *enableDeleteDB) Put(key, value []byte) error {
	return d.db.Put(key, common.JoinBytes(existsByte, value))
}

// Delete writes the empty byte slice at key — that zero-length value
// is the tombstone the read path recognizes (see [enableDeleteDB.Get]).
// Note this is NOT the same as the `[]byte{0}` written by the
// low-level [patchApplierWO] path; the WO path writes a length-1 value
// (existsByte || empty) so its writes look like a live empty value to
// the high-level decorator.
func (d *enableDeleteDB) Delete(key []byte) error {
	return d.db.Put(key, []byte{})
}

// NewIterator walks live keys, transparently stripping the tombstone byte
// from values.
func (d *enableDeleteDB) NewIterator(prefix []byte) StorageIterator {
	return newEnableDeleteIterator(d.db.NewIterator(prefix))
}

// Changes returns a [Patch] of every live put / tombstone delete recorded
// against the underlying db, with values decoded back to their natural form.
func (d *enableDeleteDB) Changes() (Patch, error) {
	p, err := d.db.changesInternal([]byte{})
	if err != nil {
		return nil, err
	}
	edp := &enableDeletePatch{
		p: NewPatch(),
	}
	err = p.Replay(edp)
	if err != nil {
		return nil, err
	}
	return edp.p, nil
}

// Snapshot returns a [DB] view that buffers writes in a fresh in-memory
// layer in front of the current contents — used by the manager to expose
// per-version writable handles without copying the full state.
func (d *enableDeleteDB) Snapshot() DB {
	return enableDelete(newMergedDb([]db{newMemDBInternal(), d.db}))
}

// Apply replays patch onto this DB. Returns the first error encountered.
func (d *enableDeleteDB) Apply(patch Patch) error {
	pa := &patchApplier{
		err: nil,
		db:  d,
	}
	if err := patch.Replay(pa); err != nil {
		return err
	}
	return pa.err
}

// Subset returns a namespaced view of this DB whose keys are transparently
// prefixed; deletion semantics are preserved through the namespace.
func (d *enableDeleteDB) Subset(prefix []byte) DB {
	return enableDelete(newSubDB(prefix, d.db))
}

// enableDeleteIterator strips the tombstone-byte prefix from values
// emitted by the wrapped iterator.
type enableDeleteIterator struct {
	StorageIterator
}

// Value returns the live value with the tombstone byte stripped, or nil
// for tombstoned entries.
func (i *enableDeleteIterator) Value() []byte {
	val := i.StorageIterator.Value()
	if len(val) == 0 {
		return nil
	}
	return val[1:]
}

// newEnableDeleteIterator returns an iterator that hides the tombstone-byte
// encoding from its consumer.
func newEnableDeleteIterator(iterator StorageIterator) StorageIterator {
	return &enableDeleteIterator{
		StorageIterator: iterator,
	}
}

// enableDelete wraps a low-level [db] in an [enableDeleteDB] decorator,
// promoting it to the high-level [DB] interface.
func enableDelete(db db) DB {
	return &enableDeleteDB{
		db: db,
	}
}
