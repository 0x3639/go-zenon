package zenon

import (
	"path"

	"github.com/syndtr/goleveldb/leveldb"

	"github.com/zenon-network/go-zenon/chain/store"
	"github.com/zenon-network/go-zenon/common/db"
	"github.com/zenon-network/go-zenon/wallet"
)

// Config bundles the non-default knobs needed to construct a
// [Zenon]. DataDir is the on-disk root for chain and consensus
// stores; ProducingKeyPair is non-nil only on pillar nodes;
// GenesisConfig is loaded by the caller (typically [app] reads
// genesis.json or falls back to the embedded Alphanet genesis).
type Config struct {
	MinPeers          int
	MinConnectedPeers int
	DataDir           string
	ProducingKeyPair  *wallet.KeyPair
	GenesisConfig     store.Genesis
}

// NewDBManager opens a versioned [db.Manager] rooted at
// DataDir/inside — the canonical chain-store layout consumed by
// the chain subsystem.
func (c *Config) NewDBManager(inside string) db.Manager {
	return db.NewLevelDBManager(path.Join(c.DataDir, inside))
}

// NewLevelDB opens a raw leveldb at DataDir/inside, returning both
// the abstracted [db.DB] handle and the underlying leveldb.DB so
// [zenon.Stop] can Close it explicitly.
func (c *Config) NewLevelDB(inside string) (db.DB, *leveldb.DB) {
	return db.NewLevelDB(path.Join(c.DataDir, inside))
}
