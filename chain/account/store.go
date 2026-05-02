package account

import (
	"github.com/zenon-network/go-zenon/chain/store"
	"github.com/zenon-network/go-zenon/common"
	"github.com/zenon-network/go-zenon/common/db"
	"github.com/zenon-network/go-zenon/common/types"
)

// getStorageIterator returns the key prefix that namespaces an account's
// embedded-contract storage area within the account database.
func getStorageIterator() []byte {
	return storageKeyPrefix
}

// accountStore is the [store.Account] implementation. It embeds [db.DB]
// directly so range queries flow through the same versioned layer as the
// rest of the chain; per-feature key namespaces live in keys.go.
type accountStore struct {
	address types.Address
	db.DB
}

// Address returns a pointer to the address this account store belongs to.
func (as *accountStore) Address() *types.Address {
	return &as.address
}

// Storage returns the contract-storage subset of the account database
// (under [storageKeyPrefix]) wrapped in [db.DisableNotFound] so callers
// can branch on `len(value) == 0` rather than a typed error.
func (as *accountStore) Storage() db.DB {
	return db.DisableNotFound(as.Subset(getStorageIterator()))
}

// Snapshot returns an isolated copy of this view; subsequent writes are
// captured separately and won't reach the source.
func (as *accountStore) Snapshot() store.Account {
	return NewAccountStore(as.address, as.DB.Snapshot())
}

// Identifier returns the (Hash, Height) of the frontier block in this
// view, or [types.ZeroHashHeight] if the account chain is empty.
func (as *accountStore) Identifier() types.HashHeight {
	frontier, err := as.Frontier()
	common.DealWithErr(err)
	if frontier == nil {
		return types.ZeroHashHeight
	}
	return frontier.Identifier()
}

// NewAccountStore wraps db in a [store.Account] for address. Panics if
// db is nil — the store cannot operate without backing storage.
func NewAccountStore(address types.Address, db db.DB) store.Account {
	if db == nil {
		panic("account store can't operate with nil db")
	}
	return &accountStore{
		address: address,
		DB:      db,
	}
}
