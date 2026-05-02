package momentum

import (
	"github.com/syndtr/goleveldb/leveldb"

	"github.com/zenon-network/go-zenon/chain/nom"
	"github.com/zenon-network/go-zenon/common"
	"github.com/zenon-network/go-zenon/common/types"
)

// getAccountHeaderByHashKey returns the database key under which the
// serialized [types.AccountHeader] for the supplied account-block hash
// is stored.
func getAccountHeaderByHashKey(hash types.Hash) []byte {
	return common.JoinBytes(accountHeaderByHashPrefix, hash.Bytes())
}

// GetFrontierAccountBlock returns the frontier account block of address
// in this view (a convenience over [store.Account.Frontier]).
func (ms *momentumStore) GetFrontierAccountBlock(address types.Address) (*nom.AccountBlock, error) {
	return ms.GetAccountStore(address).Frontier()
}

// GetAccountBlock looks up the block named by header (address + height).
func (ms *momentumStore) GetAccountBlock(header types.AccountHeader) (*nom.AccountBlock, error) {
	return ms.GetAccountStore(header.Address).ByHeight(header.Height)
}

// GetAccountBlockByHeight is a convenience for callers that already know
// the address and just need the block at height.
func (ms *momentumStore) GetAccountBlockByHeight(address types.Address, height uint64) (*nom.AccountBlock, error) {
	return ms.GetAccountStore(address).ByHeight(height)
}

// GetAccountBlocksByHeight returns up to count blocks starting at height
// in the supplied account chain, in ascending order.
func (ms *momentumStore) GetAccountBlocksByHeight(address types.Address, height, count uint64) ([]*nom.AccountBlock, error) {
	return ms.GetAccountStore(address).MoreByHeight(height, count)
}

// addAccountBlockHeader writes header into the
// [accountHeaderByHashPrefix] reverse index so future
// [momentumStore.GetAccountBlockByHash] calls can locate the block
// without knowing the address.
func (ms *momentumStore) addAccountBlockHeader(header types.AccountHeader) error {
	data, err := header.Serialize()
	if err != nil {
		return err
	}
	return ms.DB.Put(getAccountHeaderByHashKey(header.Hash), data)
}

// GetAccountBlockByHash looks up an account block by hash via the
// hash → header reverse index. Returns (nil, nil) when no such block
// exists.
func (ms *momentumStore) GetAccountBlockByHash(hash types.Hash) (*nom.AccountBlock, error) {
	data, err := ms.DB.Get(getAccountHeaderByHashKey(hash))

	if err == leveldb.ErrNotFound {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	if header, err := types.DeserializeAccountHeader(data); err != nil {
		return nil, err
	} else {
		return ms.GetAccountStore(header.Address).ByHeight(header.Height)
	}
}
