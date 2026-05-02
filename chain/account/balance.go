package account

import (
	"math/big"

	"github.com/syndtr/goleveldb/leveldb"

	"github.com/zenon-network/go-zenon/common"
	"github.com/zenon-network/go-zenon/common/types"
)

// getBalanceKey returns the database key holding the cached balance for
// zts on this account.
func getBalanceKey(zts types.ZenonTokenStandard) []byte {
	return common.JoinBytes(balanceKeyPrefix, zts.Bytes())
}

// getBalancePrefix returns the iterator prefix that walks every cached
// balance for this account.
func getBalancePrefix() []byte {
	return common.JoinBytes(balanceKeyPrefix)
}

// GetBalance returns the cached balance for zts. A missing key returns
// zero (no error) — accounts that have never held the token simply have
// no entry.
func (as *accountStore) GetBalance(zts types.ZenonTokenStandard) (*big.Int, error) {
	data, err := as.DB.Get(getBalanceKey(zts))
	if err == leveldb.ErrNotFound {
		return big.NewInt(0), nil
	}
	if err != nil {
		return nil, err
	}

	return big.NewInt(0).SetBytes(data), nil
}

// SetBalance overwrites the cached balance for zts. The balance is
// stored as a big-endian unsigned integer.
func (as *accountStore) SetBalance(zts types.ZenonTokenStandard, balance *big.Int) error {
	if err := as.DB.Put(getBalanceKey(zts), common.BigIntToBytes(balance)); err != nil {
		return err
	}
	return nil
}

// GetBalanceMap returns every cached balance for this account, keyed by
// token. The 1-byte prefix is stripped from each key during iteration.
func (as *accountStore) GetBalanceMap() (map[types.ZenonTokenStandard]*big.Int, error) {
	iterator := as.DB.NewIterator(getBalancePrefix())
	defer iterator.Release()
	result := make(map[types.ZenonTokenStandard]*big.Int, 0)

	for {
		if !iterator.Next() {
			if iterator.Error() != nil {
				return nil, iterator.Error()
			}
			break
		}
		if iterator.Value() == nil {
			continue
		}

		zts, err := types.BytesToZTS(iterator.Key()[1:])

		if err != nil {
			return nil, err
		}
		result[zts] = common.BytesToBigInt(iterator.Value())
	}
	return result, nil
}
