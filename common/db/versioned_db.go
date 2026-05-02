package db

import (
	"runtime"
	"sync"

	lru "github.com/hashicorp/golang-lru"
	"github.com/pkg/errors"
	"github.com/syndtr/goleveldb/leveldb"
	"github.com/syndtr/goleveldb/leveldb/opt"

	"github.com/zenon-network/go-zenon/common"
	"github.com/zenon-network/go-zenon/common/types"
)

// LRU sizes and the cache cliff used by [ldbManager.Get] to decide whether
// a historical view should be cached.
const (
	// l1CacheSize bounds the count of recent historical views cached.
	l1CacheSize = 400
	// l2CacheSize bounds the count of older historical views cached.
	l2CacheSize = 100
	// maximumCacheHeightDifference is the height delta below which a view
	// is admitted to the L1 cache; older views fall through to L2.
	maximumCacheHeightDifference = 360
)

// Single-byte storage prefixes used by [ldbManager] to namespace its three
// records on top of a shared LevelDB instance.
var (
	// frontierByte namespaces the live (frontier) chain state.
	frontierByte = []byte{85}
	// patchByte namespaces forward patches keyed by height.
	patchByte = []byte{102}
	// rollbackByte namespaces rollback patches keyed by height.
	rollbackByte = []byte{119}
)

// absDiff returns |x - y| for unsigned heights without underflow.
func absDiff(x, y uint64) uint64 {
	if x < y {
		return y - x
	}
	return x - y
}

// getOpenFilesCacheCapacity returns the LevelDB open-files cache size for
// the chain database. macOS has a tighter default ulimit so we cap it lower.
func getOpenFilesCacheCapacity() int {
	switch runtime.GOOS {
	case "darwin":
		return 100
	case "windows":
		return 200
	default:
		return 200
	}
}

// Manager is the versioned database the chain orchestrates. It exposes a
// frontier (the most recent commit) and lookups for any historical
// [HashHeight] still in scope, while serializing forward and rollback
// commits behind a single mutex. Both an in-memory and a LevelDB-backed
// implementation are provided.
//
// Concurrency: every method is safe for concurrent use; the manager owns
// the mutex.
type Manager interface {
	// Frontier returns a [DB] view of the most recent commit.
	Frontier() DB
	// Get returns a [DB] view at the given identifier, or nil if it is no
	// longer reachable.
	Get(types.HashHeight) DB
	// GetPatch returns the forward patch that produced identifier, or nil
	// if not available.
	GetPatch(identifier types.HashHeight) Patch

	// Add commits a transaction (sequence of commits + their patch) onto
	// the frontier.
	Add(Transaction) error
	// Pop reverses the most recent commit.
	Pop() error

	// Stop releases resources held by the manager.
	Stop() error
	// Location reports where the underlying storage lives ("in-memory" or
	// the LevelDB directory path).
	Location() string
}

// memdbManager is the [Manager] implementation backed entirely by in-memory
// stores. Every committed version retains a full snapshot. Used for tests
// and short-lived nodes.
type memdbManager struct {
	stableDB           DB
	stableIdentifier   types.HashHeight
	frontierIdentifier types.HashHeight
	previous           map[types.HashHeight]types.HashHeight
	versions           map[types.HashHeight]DB
	patches            map[types.HashHeight]Patch

	changes sync.Mutex
}

// NewMemDBManager constructs a memory-backed [Manager] seeded from the
// frontier already present in rawDB.
func NewMemDBManager(rawDB DB) Manager {
	frontierIdentifier := GetFrontierIdentifier(rawDB)
	return &memdbManager{
		stableDB:           rawDB,
		stableIdentifier:   frontierIdentifier,
		frontierIdentifier: frontierIdentifier,
		previous:           map[types.HashHeight]types.HashHeight{},
		versions:           map[types.HashHeight]DB{frontierIdentifier: rawDB},
		patches:            map[types.HashHeight]Patch{},
	}
}

// Frontier returns a snapshot of the most recent committed version.
func (m *memdbManager) Frontier() DB {
	m.changes.Lock()
	frontierIdentifier := m.frontierIdentifier
	m.changes.Unlock()
	return m.Get(frontierIdentifier)
}

// Get returns a snapshot at identifier or nil if the version has been
// pruned or never existed.
func (m *memdbManager) Get(identifier types.HashHeight) DB {
	m.changes.Lock()
	defer m.changes.Unlock()
	db, ok := m.versions[identifier]
	if ok {
		return db.Snapshot()
	}
	return nil
}

// GetPatch returns the forward patch that produced identifier or nil if it
// is not retained.
func (m *memdbManager) GetPatch(identifier types.HashHeight) Patch {
	m.changes.Lock()
	defer m.changes.Unlock()
	return m.patches[identifier]
}

// Add applies transaction onto the current frontier, advancing it to the
// transaction's last commit. Returns an error if the transaction does not
// chain from the current frontier.
func (m *memdbManager) Add(transaction Transaction) error {
	commits := transaction.GetCommits()
	previous := commits[0].Previous()
	head := commits[len(commits)-1].Identifier()

	if previous != m.frontierIdentifier {
		return errors.Errorf("can't insert identifier %v. previous doesn't match with current frontier %v", head, m.frontierIdentifier)
	}

	// apply transaction on db
	db := m.Get(previous)
	if db == nil {
		return errors.Errorf("can't find prev")
	}

	patch := transaction.StealChanges()

	for _, commit := range commits {
		temp := NewMemDB()
		data, err := commit.Serialize()
		if err != nil {
			return err
		}
		if err := SetFrontier(temp, commit.Identifier(), data); err != nil {
			return err
		}
		frontierPatch, err := temp.Changes()
		if err != nil {
			return err
		}
		if err := frontierPatch.Replay(patch); err != nil {
			return err
		}
		if err := db.Apply(patch); err != nil {
			return err
		}
	}

	m.changes.Lock()
	defer m.changes.Unlock()

	m.frontierIdentifier = head
	m.previous[head] = previous
	m.versions[head] = db
	m.patches[head] = patch

	for _, commit := range commits[:len(commits)-1] {
		m.versions[commit.Identifier()] = db
		m.patches[commit.Identifier()] = NewPatch()
	}

	return nil
}

// Pop reverses the most recent commit unless the frontier is at the stable
// identifier (the snapshot the manager was constructed against).
func (m *memdbManager) Pop() error {
	m.changes.Lock()
	defer m.changes.Unlock()
	if m.stableIdentifier == m.frontierIdentifier {
		return errors.Errorf("can't rollback stable db")
	}

	previous, ok := m.previous[m.frontierIdentifier]
	if !ok {
		return errors.Errorf("can't find previous for ")
	}

	delete(m.previous, m.frontierIdentifier)
	delete(m.versions, m.frontierIdentifier)
	delete(m.patches, m.frontierIdentifier)
	m.frontierIdentifier = previous
	return nil
}

// Stop discards retained versions; the in-memory manager has no persistent
// resources to release.
func (m *memdbManager) Stop() error {
	m.frontierIdentifier = types.ZeroHashHeight
	m.versions = nil
	m.patches = nil
	return nil
}

// Location reports the well-known sentinel "in-memory" for callers that log
// the database path.
func (m *memdbManager) Location() string {
	return "in-memory"
}

// rollbackCache holds a partially-rolled-back view of the frontier,
// memoizing the application of historical rollback patches up to a
// particular height. Used by [ldbManager.Get] to amortize the cost of
// reconstructing a historical view.
type rollbackCache struct {
	frontier types.HashHeight
	raw      db
}

// ldbManager is the [Manager] implementation backed by a LevelDB on disk.
// It stores forward and rollback patches per height alongside the live
// frontier, and amortizes historical-view reconstruction with two LRUs.
type ldbManager struct {
	location string
	l1Cache  *lru.Cache
	l2Cache  *lru.Cache
	ldb      *leveldb.DB
	changes  sync.Mutex
	stopped  bool
}

// NewLevelDBManager opens a LevelDB at dir and returns a manager that
// persists every commit (forward + rollback patches and the live frontier).
// Panics on open failure — a missing or corrupt directory is fatal here.
func NewLevelDBManager(dir string) Manager {
	opts := &opt.Options{OpenFilesCacheCapacity: getOpenFilesCacheCapacity()}
	ldb, err := leveldb.OpenFile(dir, opts)
	common.DealWithErr(err)
	l1Cache, err := lru.New(l1CacheSize)
	common.DealWithErr(err)
	l2Cache, err := lru.New(l2CacheSize)
	common.DealWithErr(err)
	return &ldbManager{
		location: dir,
		l1Cache:  l1Cache,
		l2Cache:  l2Cache,
		ldb:      ldb,
	}
}

// Frontier returns a [DB] view of the live frontier sub-namespace, or nil
// if the manager has been stopped.
func (m *ldbManager) Frontier() DB {
	m.changes.Lock()
	defer m.changes.Unlock()
	if m.stopped {
		return nil
	}
	snapshot, _ := m.ldb.GetSnapshot()
	return NewLevelDBSnapshotWrapper(snapshot).Subset(frontierByte)
}

// Get returns a [DB] view at identifier. If identifier is in the past, the
// view is materialized by replaying rollback patches from the frontier
// back to identifier and is then cached for subsequent lookups. Returns
// nil if identifier is unknown or unreachable.
func (m *ldbManager) Get(identifier types.HashHeight) DB {
	m.changes.Lock()
	defer m.changes.Unlock()
	if m.stopped {
		return nil
	}
	snapshot, _ := m.ldb.GetSnapshot()
	// check if has snapshot
	frontier := NewLevelDBSnapshotWrapper(snapshot).Subset(frontierByte)
	frontierIdentifier := GetFrontierIdentifier(frontier)

	if identifier.IsZero() {
		return NewMemDB()
	}
	if identifier == frontierIdentifier {
		return frontier
	}

	trueIdentifier, err := GetIdentifierByHash(frontier, identifier.Hash)
	if err == leveldb.ErrNotFound {
		return nil
	}
	common.DealWithErr(err)
	if *trueIdentifier != identifier {
		return nil
	}

	var rawChanges db
	var toIdentifier types.HashHeight

	if cache, ok := m.l1Cache.Get(identifier); ok {
		toIdentifier = cache.(*rollbackCache).frontier
		rawChanges = cache.(*rollbackCache).raw
	} else if cache, ok := m.l2Cache.Get(identifier); ok {
		toIdentifier = cache.(*rollbackCache).frontier
		rawChanges = cache.(*rollbackCache).raw
	} else {
		rawChanges = newMemDBInternal()
		toIdentifier = identifier
	}

	for i := toIdentifier.Height + 1; i <= frontierIdentifier.Height; i += 1 {
		rollback := m.getRollback(i)
		if err := ApplyWithoutOverride(rawChanges, rollback); err != nil {
			common.DealWithErr(err)
		}
	}

	if absDiff(identifier.Height, frontierIdentifier.Height) < maximumCacheHeightDifference {
		m.l1Cache.Add(identifier, &rollbackCache{
			frontier: frontierIdentifier,
			raw:      rawChanges,
		})
	} else {
		m.l2Cache.Add(identifier, &rollbackCache{
			frontier: frontierIdentifier,
			raw:      rawChanges,
		})
	}

	u := newMergedDb([]db{
		newMemDBInternal(),
		newSkipDelete(
			newMergedDb([]db{
				rawChanges,
				newSubDB(frontierByte, newLevelDBSnapshotWrapper(snapshot)),
			})),
	})
	return enableDelete(u)
}

// GetPatch returns the persisted forward patch for identifier, or nil if
// none is stored.
func (m *ldbManager) GetPatch(identifier types.HashHeight) Patch {
	m.changes.Lock()
	defer m.changes.Unlock()
	if m.stopped {
		return nil
	}
	return m.getPatch(identifier)
}

// getPatch reads the forward patch for identifier from disk. Caller must
// hold m.changes.
func (m *ldbManager) getPatch(identifier types.HashHeight) Patch {
	snapshot, _ := m.ldb.GetSnapshot()
	value, err := snapshot.Get(common.JoinBytes(patchByte, common.Uint64ToBytes(identifier.Height)), nil)
	if err == leveldb.ErrNotFound {
		return nil
	}
	common.DealWithErr(err)

	patch, err := NewPatchFromDump(value)
	common.DealWithErr(err)
	return patch
}

// getRollback reads the rollback patch for height from disk.
func (m *ldbManager) getRollback(height uint64) Patch {
	snapshot, _ := m.ldb.GetSnapshot()
	value, err := snapshot.Get(common.JoinBytes(rollbackByte, common.Uint64ToBytes(height)), nil)
	if err == leveldb.ErrNotFound {
		return nil
	}
	common.DealWithErr(err)

	patch, err := NewPatchFromDump(value)
	common.DealWithErr(err)
	return patch
}

// Add applies transaction onto the current frontier and persists both the
// forward patch and the matching rollback patch.
//
// Concurrency: serialized by m.changes.
func (m *ldbManager) Add(transaction Transaction) error {
	commits := transaction.GetCommits()

	previous := commits[0].Previous()
	identifier := commits[len(commits)-1].Identifier()

	// apply transaction on db
	db := m.Get(previous)
	if db == nil {
		return errors.Errorf("can't find prev")
	}

	patch := transaction.StealChanges()

	for _, commit := range commits {
		temp := NewMemDB()
		data, err := commit.Serialize()
		if err != nil {
			return err
		}
		if err := SetFrontier(temp, commit.Identifier(), data); err != nil {
			return err
		}
		frontierPatch, err := temp.Changes()
		if err != nil {
			return err
		}
		if err := frontierPatch.Replay(patch); err != nil {
			return err
		}
	}

	rollbackPatch := RollbackPatch(db, patch)

	m.changes.Lock()
	defer m.changes.Unlock()

	frontierIdentifier := GetFrontierIdentifier(db)

	if previous == frontierIdentifier {
		if err := m.ldb.Put(common.JoinBytes(patchByte, common.Uint64ToBytes(identifier.Height)), patch.Dump(), nil); err != nil {
			return err
		}
		if err := m.ldb.Put(common.JoinBytes(rollbackByte, common.Uint64ToBytes(identifier.Height)), rollbackPatch.Dump(), nil); err != nil {
			return err
		}
		if err := ApplyPatch(NewLevelDBWrapper(m.ldb).Subset(frontierByte), patch); err != nil {
			return err
		}
	}
	return nil
}

// Pop applies the most recent rollback patch and discards the corresponding
// forward and rollback records from disk.
func (m *ldbManager) Pop() error {
	frontierIdentifier := GetFrontierIdentifier(m.Frontier())
	rollbackPatch := m.getRollback(frontierIdentifier.Height)

	if err := ApplyPatch(NewLevelDBWrapper(m.ldb).Subset(frontierByte), rollbackPatch); err != nil {
		return err
	}
	if err := m.ldb.Delete(common.JoinBytes(patchByte, common.Uint64ToBytes(frontierIdentifier.Height)), nil); err != nil {
		return err
	}
	if err := m.ldb.Delete(common.JoinBytes(rollbackByte, common.Uint64ToBytes(frontierIdentifier.Height)), nil); err != nil {
		return err
	}

	return nil
}

// Stop closes the underlying LevelDB and clears the caches. After Stop
// returns, every method on the manager returns nil/zero values.
func (m *ldbManager) Stop() error {
	m.changes.Lock()
	defer m.changes.Unlock()
	if err := m.ldb.Close(); err != nil {
		return err
	}
	m.stopped = true
	m.ldb = nil
	m.l1Cache = nil
	m.l2Cache = nil
	return nil
}

// Location returns the directory the underlying LevelDB lives in.
func (m *ldbManager) Location() string {
	return m.location
}
