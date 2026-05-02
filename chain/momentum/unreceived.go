package momentum

import (
	"github.com/zenon-network/go-zenon/chain/nom"
	"github.com/zenon-network/go-zenon/common/types"
)

// GetBlockWhichReceives returns the receive block that consumed the
// send identified by hash, or nil if the send is still pending. Resolves
// in two cheap lookups: send hash → send block (via the
// [accountHeaderByHashPrefix] reverse index), then send → receive header
// (via the recipient's mailbox), then receive header → receive block.
func (ms *momentumStore) GetBlockWhichReceives(hash types.Hash) (*nom.AccountBlock, error) {
	block, err := ms.GetAccountBlockByHash(hash)
	if err != nil || block == nil {
		return nil, err
	}

	fromHeader := ms.GetAccountMailbox(block.Address).GetBlockWhichReceives(hash)
	if fromHeader == nil {
		return nil, nil
	}
	return ms.GetAccountBlock(*fromHeader)
}
