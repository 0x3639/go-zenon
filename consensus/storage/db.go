package storage

import (
	"encoding/binary"

	lru "github.com/hashicorp/golang-lru"
	"github.com/pkg/errors"
	"github.com/syndtr/goleveldb/leveldb"

	"github.com/zenon-network/go-zenon/common/db"
	"github.com/zenon-network/go-zenon/common/types"
)

// Single-byte key prefixes used by the consensus storage. Together with
// the granularity-specific point caches they let the consensus layer
// keep period and epoch points (and election results) cleanly
// partitioned in a single LevelDB instance.
const (
	// PrefixPeriodPoint namespaces per-period [Point]s.
	PrefixPeriodPoint = byte(0)
	// PrefixEpochPoint namespaces per-epoch [Point]s.
	PrefixEpochPoint = byte(1)
	// NumPointTypes is the count of point granularities — currently
	// period and epoch. Used to size the per-prefix cache slice in [DB].
	NumPointTypes = 2
	// PrefixElectionResult namespaces cached [ElectionData] keyed by
	// proof-block hash.
	PrefixElectionResult = byte(10)
)

// DB is the consensus storage handle: a [common/db.DB] backing store
// plus per-feature LRU caches for hot reads. The cache sizes are
// chosen so that ~1 week of points fit in memory at the configured
// block-time and node-count.
type DB struct {
	db            db.DB
	electionCache *lru.Cache
	pointCache    []*lru.Cache
}

// NewConsensusDB wires a [DB] over db with separate LRU caches for
// election results and per-prefix points. Panics on cache-construction
// failure — the LRU library only fails on invalid sizes, which would
// be a programmer error here.
func NewConsensusDB(db db.DB, electionCacheSize int, pointCacheSize int) *DB {
	electionCache, err := lru.New(electionCacheSize)
	if err != nil {
		panic(err)
	}

	pointCache := make([]*lru.Cache, NumPointTypes)
	for i := 0; i < NumPointTypes; i += 1 {
		pointCache[i], err = lru.New(pointCacheSize)
		if err != nil {
			panic(err)
		}
	}

	return &DB{
		db:            db,
		electionCache: electionCache,
		pointCache:    pointCache,
	}
}

// GetPointByHeight returns the cached [Point] at (prefix, height),
// loading from disk on cache miss. Returns (nil, nil) when no entry
// exists at that key.
func (db *DB) GetPointByHeight(prefix byte, height uint64) (*Point, error) {
	// Get from cache
	cacheValue, ok := db.pointCache[prefix].Get(height)
	if ok {
		return cacheValue.(*Point), nil
	}

	// Get from DB
	key := CreatePointKey(prefix, height)
	value, err := db.db.Get(key)
	if err != nil {
		if err == leveldb.ErrNotFound {
			return nil, nil
		}
		return nil, err
	}

	// Create object from bytes
	data := &Point{}
	err = data.Unmarshal(value)
	if err != nil {
		return nil, errors.Errorf("error Unmarshal Point prefix %v height %v reason %e", prefix, height, err)
	}

	// Update cache
	db.pointCache[prefix].Add(height, data)
	return data, nil
}

// DeletePointByHeight evicts the cached point and removes the
// underlying database entry. Used by [points] when a stale point's
// EndHash no longer matches the live chain.
func (db *DB) DeletePointByHeight(prefix byte, height uint64) error {
	db.pointCache[prefix].Remove(height)
	return db.db.Delete(CreatePointKey(prefix, height))
}

// StorePointByHeight persists p under (prefix, height) and updates the
// cache. Caller is responsible for only persisting points whose tick
// is finished — partial points are kept in memory only.
func (db *DB) StorePointByHeight(prefix byte, height uint64, p *Point) error {
	bytes, err := p.Marshal()
	if err != nil {
		return err
	}
	err = db.db.Put(CreatePointKey(prefix, height), bytes)
	if err != nil {
		return err
	}

	// Saved to DB ok - Update cache
	db.pointCache[prefix].Add(height, p)
	return nil
}

// GetElectionResultByHash returns the cached [ElectionData] for the
// supplied proof-block hash, loading from disk on cache miss. Returns
// (nil, nil) when no result has been cached for hash.
func (db *DB) GetElectionResultByHash(hash types.Hash) (*ElectionData, error) {
	// Get from cache
	cacheValue, ok := db.electionCache.Get(hash)
	if ok {
		return cacheValue.(*ElectionData), nil
	}

	// Get from DB
	key := CreateElectionResultKey(hash)
	value, err := db.db.Get(key)
	if err != nil {
		if err == leveldb.ErrNotFound {
			return nil, nil
		}
		return nil, err
	}

	// Create object from bytes
	data := &ElectionData{}
	err = data.Unmarshal(value)
	if err != nil {
		return nil, errors.Errorf("error Unmarshal ElectionData hash %v reason %e", hash, err)
	}

	// Update cache
	db.electionCache.Add(hash, data)
	return data, nil
}

// StoreElectionResultByHash persists data keyed by hash and updates
// the cache. Used by [electionManager.generateProducers] to memoize
// the algorithm output so subsequent ElectionByTick calls do not
// re-run the shuffle.
func (db *DB) StoreElectionResultByHash(hash types.Hash, data *ElectionData) error {
	bytes, err := data.Marshal()
	if err != nil {
		return err
	}
	err = db.db.Put(CreateElectionResultKey(hash), bytes)
	if err != nil {
		return err
	}

	// Saved to DB ok - Update cache
	db.electionCache.Add(hash, data)
	return nil
}

// CreateElectionResultKey returns the database key holding the
// [ElectionData] for the supplied proof-block hash:
// `PrefixElectionResult || hash`.
func CreateElectionResultKey(hash types.Hash) []byte {
	key := make([]byte, 1+types.HashSize)
	key[0] = PrefixElectionResult
	copy(key[1:types.HashSize+1], hash.Bytes())
	return key
}

// CreatePointKey returns the database key holding the [Point] at
// (prefix, height): `prefix || big-endian uint64`.
func CreatePointKey(prefix byte, height uint64) []byte {
	key := make([]byte, 1+8)
	key[0] = prefix
	binary.BigEndian.PutUint64(key[1:9], height)
	return key
}
