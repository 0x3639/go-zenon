package momentum

import (
	"github.com/syndtr/goleveldb/leveldb"

	"github.com/zenon-network/go-zenon/common"
	"github.com/zenon-network/go-zenon/common/types"
)

// getBlockConfirmationHeightKey returns the database key under which
// the confirmation height of the account block identified by hash is
// stored.
func getBlockConfirmationHeightKey(hash types.Hash) []byte {
	return common.JoinBytes(blockConfirmationHeightPrefix, hash.Bytes())
}

// GetBlockConfirmationHeight returns the momentum height that confirmed
// hash, or zero if the block has not been confirmed in this view. The
// verifier consumes this value when validating
// [chain/nom.AccountBlock.MomentumAcknowledged] for auto-generated
// blocks.
func (ms *momentumStore) GetBlockConfirmationHeight(hash types.Hash) (uint64, error) {
	data, err := ms.DB.Get(getBlockConfirmationHeightKey(hash))
	if err == leveldb.ErrNotFound {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	return common.BytesToUint64(data), nil
}

// setBlockConfirmationHeight records the momentum height at which hash
// was confirmed. Called by [momentumStore.AddAccountBlockTransaction] for
// every block (and descendant) admitted.
func (ms *momentumStore) setBlockConfirmationHeight(hash types.Hash, height uint64) error {
	return ms.DB.Put(getBlockConfirmationHeightKey(hash), common.Uint64ToBytes(height))
}
