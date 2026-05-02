package momentum

import (
	"math/big"

	"github.com/syndtr/goleveldb/leveldb"

	"github.com/zenon-network/go-zenon/common"
	"github.com/zenon-network/go-zenon/common/types"
)

// getAccountZNNBalance returns the database key holding address's cached
// ZNN balance at the momentum-store level. The same value also lives in
// the per-account store; the duplicate exists so consensus delegation
// math can read every backer's balance in O(1) without re-deriving each
// account view.
func getAccountZNNBalance(address types.Address) []byte {
	return common.JoinBytes(accountZNNBalancePrefix, address.Bytes())
}

// getZnnBalance returns address's cached ZNN balance, or zero (no error)
// if the cache has no entry yet.
func (ms *momentumStore) getZnnBalance(address types.Address) (*big.Int, error) {
	data, err := ms.DB.Get(getAccountZNNBalance(address))
	if err == leveldb.ErrNotFound {
		return big.NewInt(0), nil
	}
	if err != nil {
		return nil, err
	}
	return big.NewInt(0).SetBytes(data), nil
}

// setZnnBalance overwrites address's cached ZNN balance. Called by
// [momentumStore.AddAccountBlockTransaction] every time an account block
// for address is admitted.
func (ms *momentumStore) setZnnBalance(address types.Address, balance *big.Int) error {
	return ms.DB.Put(getAccountZNNBalance(address), common.BigIntToBytes(balance))
}
