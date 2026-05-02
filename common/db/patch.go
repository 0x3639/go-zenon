package db

import (
	"encoding/hex"
	"fmt"

	"github.com/syndtr/goleveldb/leveldb"

	"github.com/zenon-network/go-zenon/common"
	"github.com/zenon-network/go-zenon/common/types"
)

// patch is the canonical [Patch] implementation, backed by goleveldb's
// `*leveldb.Batch` so that patches can be dumped, reloaded, and applied
// atomically against a real LevelDB.
type patch struct {
	*leveldb.Batch
}

// Replay walks every operation in p in insertion order, calling pr.Put or
// pr.Delete as appropriate.
func (p *patch) Replay(pr PatchReplayer) error {
	return p.Batch.Replay(pr)
}

// NewPatch returns a fresh empty [Patch].
func NewPatch() Patch {
	return &patch{
		Batch: new(leveldb.Batch),
	}
}

// NewPatchFromDump rebuilds a [Patch] from bytes produced by [Patch.Dump].
// Returns an error if the bytes are not a valid leveldb batch encoding.
func NewPatchFromDump(data []byte) (Patch, error) {
	p := &patch{
		Batch: new(leveldb.Batch),
	}
	err := p.Batch.Load(data)
	return p, err
}

// patchPrinter is a [PatchReplayer] that accumulates a hex dump of every
// operation. Used by [DebugPatch].
type patchPrinter struct {
	dump string
}

// Put appends a `key - value` line to the printer's dump.
func (pp *patchPrinter) Put(key []byte, value []byte) {
	pp.dump = pp.dump + fmt.Sprintf("%v - %v\n", hex.EncodeToString(key), hex.EncodeToString(value))
}

// Delete appends a `key - DELETE` line to the printer's dump.
func (pp *patchPrinter) Delete(key []byte) {
	pp.dump = pp.dump + fmt.Sprintf("%v - DELETE\n", hex.EncodeToString(key))
}

// patchRollback constructs the inverse of a forward patch: for each key the
// forward patch touches, it records the prior value (or a delete if the key
// did not exist). Used by the manager to compute rollback patches at insert
// time so that [Manager.Pop] can undo without re-reading the entire chain.
type patchRollback struct {
	db DB
	rb Patch
}

// rollback records the inverse of a write or delete on key by reading the
// current value from the underlying DB.
func (pr *patchRollback) rollback(key []byte) {
	value, err := pr.db.Get(key)
	if err == leveldb.ErrNotFound {
		pr.rb.Delete(key)
	} else {
		pr.rb.Put(key, value)
	}
}

// Put records the inverse of a forward Put: the previous value of key.
func (pr *patchRollback) Put(key []byte, _ []byte) {
	pr.rollback(key)
}

// Delete records the inverse of a forward Delete: the previous value of key.
func (pr *patchRollback) Delete(key []byte) {
	pr.rollback(key)
}

// patchApplier replays a [Patch] against a [DB], short-circuiting on the
// first error so it propagates back to the caller without further writes.
type patchApplier struct {
	err error
	db  DB
}

// Put writes through to the underlying DB unless a prior op already failed.
func (pa *patchApplier) Put(key []byte, value []byte) {
	if pa.err != nil {
		return
	}
	pa.err = pa.db.Put(key, value)
}

// Delete writes through to the underlying DB unless a prior op already failed.
func (pa *patchApplier) Delete(key []byte) {
	if pa.err != nil {
		return
	}
	pa.err = pa.db.Delete(key)
}

// patchValuePrefixer is a [PatchReplayer] that prepends a fixed prefix to
// every value written. Used by [PrefixPatchValues] to namespace the values
// of an entire patch in a single pass.
type patchValuePrefixer struct {
	prefix []byte
	Patch
}

// Put records key → prefix||value into the wrapped patch.
func (pa *patchValuePrefixer) Put(key []byte, value []byte) {
	pa.Patch.Put(key, common.JoinBytes(pa.prefix, value))
}

// patchApplierWO ("write-once") replays a patch against a low-level [db]
// while skipping any keys that already exist. Used by [ApplyWithoutOverride]
// to layer rollback patches onto cached snapshots without overwriting newer
// state already present in the cache.
type patchApplierWO struct {
	err error
	db  db
}

// Put writes value at key only if key is absent. The value is prefixed with
// `0x00` to interoperate with the [enableDeleteDB] tombstone byte.
func (pa *patchApplierWO) Put(key []byte, value []byte) {
	if pa.err != nil {
		return
	}
	if ok, err := pa.db.Has(key); err != nil {
		pa.err = err
	} else if !ok {
		pa.err = pa.db.Put(key, common.JoinBytes([]byte{0}, value))
	}
}

// Delete writes a single tombstone byte at key only if key is absent.
func (pa *patchApplierWO) Delete(key []byte) {
	if pa.err != nil {
		return
	}
	if ok, err := pa.db.Has(key); err != nil {
		pa.err = err
	} else if !ok {
		pa.err = pa.db.Put(key, []byte{0})
	}
}

// DebugPatch returns a hex dump of patch, one operation per line. Intended
// for ad-hoc inspection in tests and logs; the output format is not stable.
func DebugPatch(patch Patch) string {
	pp := new(patchPrinter)
	err := patch.Replay(pp)
	common.DealWithErr(err)
	return pp.dump
}

// PatchHash hashes the canonical serialization of patch with the package
// hash function. Used as the [chain/nom.Momentum.ChangesHash] commitment so
// every momentum binds the state changes it caused.
func PatchHash(patch Patch) types.Hash {
	return types.NewHash(patch.Dump())
}

// DebugDB returns a hex dump of every key/value pair in db. Intended for
// ad-hoc inspection; the output format is not stable.
func DebugDB(db DB) string {
	iterator := db.NewIterator([]byte{})
	defer iterator.Release()

	s := ""
	for {
		if !iterator.Next() {
			common.DealWithErr(iterator.Error())
			break
		}

		value := iterator.Value()
		if value == nil {
			continue
		}
		key := iterator.Key()
		s = s + fmt.Sprintf("%v - %v\n", hex.EncodeToString(key), hex.EncodeToString(value))
	}
	return s
}

// DumpDB returns a [Patch] that, when applied to an empty DB, reproduces
// the contents of db. Used by tests and import/export tooling.
func DumpDB(db DB) Patch {
	p := NewPatch()
	iterator := db.NewIterator(nil)
	defer iterator.Release()

	for {
		if !iterator.Next() {
			if iterator.Error() != nil {
				common.DealWithErr(iterator.Error())
			}
			break
		}

		value := iterator.Value()
		key := iterator.Key()

		p.Put(key, value)
	}

	return p
}

// PrefixPatchValues returns a copy of patch with prefix prepended to every
// value. Useful when staging a patch for namespaced storage.
func PrefixPatchValues(patch Patch, prefix []byte) Patch {
	pa := &patchValuePrefixer{
		prefix: prefix,
		Patch:  NewPatch(),
	}
	common.DealWithErr(patch.Replay(pa))
	return pa.Patch
}

// ApplyPatch replays patch against db using a [patchApplier]. Returns the
// first error encountered.
func ApplyPatch(db DB, patch Patch) error {
	pa := &patchApplier{
		db: db,
	}

	if err := patch.Replay(pa); err != nil {
		return err
	}
	return pa.err
}

// ApplyWithoutOverride replays patch against db using a [patchApplierWO];
// keys that already exist are left untouched. Used by the LevelDB manager's
// rollback cache to layer rollback patches.
func ApplyWithoutOverride(db db, patch Patch) error {
	pa := &patchApplierWO{
		db: db,
	}

	if err := patch.Replay(pa); err != nil {
		return err
	}
	return pa.err
}

// RollbackPatch computes the inverse of patch by reading the prior values
// of every touched key from db. Combined forward + rollback patches are
// stored together so [Manager.Pop] can undo a commit cheaply.
func RollbackPatch(db DB, patch Patch) Patch {
	pr := &patchRollback{
		db: db,
		rb: NewPatch(),
	}

	err := patch.Replay(pr)
	common.DealWithErr(err)

	return pr.rb
}
