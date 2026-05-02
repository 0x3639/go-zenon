package account

import (
	"github.com/syndtr/goleveldb/leveldb"

	"github.com/zenon-network/go-zenon/common"
	"github.com/zenon-network/go-zenon/common/types"
)

// receivedBlockKey returns the database key marking that hash (a send
// block referenced via FromBlockHash) has been consumed.
func receivedBlockKey(hash types.Hash) []byte {
	return common.JoinBytes(receivedBlockPrefix, hash.Bytes())
}

// MarkAsReceived records that the send identified by hash has been
// consumed; subsequent attempts to receive the same hash will be rejected
// by [github.com/zenon-network/go-zenon/verifier].
func (as *accountStore) MarkAsReceived(hash types.Hash) error {
	return as.DB.Put(receivedBlockKey(hash), common.Uint64ToBytes(Received))
}

// IsReceived reports whether hash has been recorded as received. Panics
// (via [common.DealWithErr]) on a database error other than not-found —
// IsReceived is called from hot verification paths where errors are bugs.
func (as *accountStore) IsReceived(hash types.Hash) bool {
	_, err := as.DB.Get(receivedBlockKey(hash))
	if err == leveldb.ErrNotFound {
		return false
	}
	common.DealWithErr(err)
	return true
}
