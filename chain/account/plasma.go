package account

import (
	"math/big"

	"github.com/syndtr/goleveldb/leveldb"

	"github.com/zenon-network/go-zenon/common"
)

// getChainPlasmaKey returns the database key holding this account's
// chain-plasma counter.
func getChainPlasmaKey() []byte {
	return chainPlasmaKey
}

// GetChainPlasma returns the per-account plasma counter. Missing key
// returns zero (no error) — fresh accounts simply have no entry yet.
func (as *accountStore) GetChainPlasma() (*big.Int, error) {
	data, err := as.DB.Get(getChainPlasmaKey())
	if err == leveldb.ErrNotFound {
		return big.NewInt(0), nil
	}
	if err != nil {
		return nil, err
	}

	return big.NewInt(0).SetBytes(data), nil
}

// AddChainPlasma adds the supplied delta to the per-account counter.
// Used by the VM to credit plasma earned by fused QSR.
func (as *accountStore) AddChainPlasma(add uint64) error {
	plasma, err := as.GetChainPlasma()
	if err != nil {
		return err
	}
	plasma.Add(plasma, big.NewInt(int64(add)))
	if err := as.DB.Put(getChainPlasmaKey(), common.BigIntToBytes(plasma)); err != nil {
		return err
	}
	return nil
}
