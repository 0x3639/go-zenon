package db

import (
	"github.com/syndtr/goleveldb/leveldb"

	"github.com/zenon-network/go-zenon/common"
	"github.com/zenon-network/go-zenon/common/types"
)

// getFrontierIdentifierKey returns the database key holding the current
// chain frontier's [HashHeight].
func getFrontierIdentifierKey() []byte {
	return frontierIdentifierKey
}

// getHeightByHashKey returns the database key that maps hash → height.
func getHeightByHashKey(hash types.Hash) []byte {
	return common.JoinBytes(heightByHashPrefix, hash.Bytes())
}

// getEntryByHeightKey returns the database key that maps height →
// serialized entry bytes.
func getEntryByHeightKey(height uint64) []byte {
	return common.JoinBytes(entryByHeightPrefix, common.Uint64ToBytes(height))
}

// SetFrontier writes the (version, data) pair into db: it updates the
// frontier identifier, the hash → height index, and the height → entry
// data record. Used by [Manager.Add] to advance the chain by one commit.
func SetFrontier(db DB, version types.HashHeight, data []byte) error {
	if err := db.Put(getFrontierIdentifierKey(), version.Serialize()); err != nil {
		return err
	}
	if err := db.Put(getHeightByHashKey(version.Hash), common.Uint64ToBytes(version.Height)); err != nil {
		return err
	}
	if err := db.Put(getEntryByHeightKey(version.Height), data); err != nil {
		return err
	}
	return nil
}

// GetFrontierIdentifier returns the [HashHeight] of the most recent commit
// in db, or [types.ZeroHashHeight] if the database is empty.
func GetFrontierIdentifier(db DB) types.HashHeight {
	data, err := db.Get(getFrontierIdentifierKey())
	if err == leveldb.ErrNotFound {
		return types.ZeroHashHeight
	}
	common.DealWithErr(err)
	hh, err := types.DeserializeHashHeight(data)
	common.DealWithErr(err)
	return *hh
}

// GetIdentifierByHash looks up the [HashHeight] of the entry whose hash is
// hash. Returns [leveldb.ErrNotFound] if no such entry exists.
func GetIdentifierByHash(db DB, hash types.Hash) (*types.HashHeight, error) {
	heightData, err := db.Get(getHeightByHashKey(hash))
	if err != nil {
		return nil, err
	}
	height := common.BytesToUint64(heightData)
	return &types.HashHeight{
		Height: height,
		Hash:   hash,
	}, nil
}

// GetEntryByHash returns the serialized bytes of the entry whose hash is
// hash. Returns [leveldb.ErrNotFound] if no such entry exists.
func GetEntryByHash(db DB, hash types.Hash) ([]byte, error) {
	heightData, err := db.Get(getHeightByHashKey(hash))
	if err != nil {
		return nil, err
	}
	height := common.BytesToUint64(heightData)
	return GetEntryByHeight(db, height)
}

// GetEntryByHeight returns the serialized bytes of the entry at height.
// Returns [leveldb.ErrNotFound] if no such entry exists.
func GetEntryByHeight(db DB, height uint64) ([]byte, error) {
	return db.Get(getEntryByHeightKey(height))
}
