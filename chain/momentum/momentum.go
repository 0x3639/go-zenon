package momentum

import (
	"fmt"

	"github.com/syndtr/goleveldb/leveldb"

	"github.com/zenon-network/go-zenon/chain/nom"
	"github.com/zenon-network/go-zenon/common/db"
	"github.com/zenon-network/go-zenon/common/types"
)

// SetFrontier writes momentum as the new frontier of the momentum chain
// via the shared [db.SetFrontier] helper (which atomically updates the
// frontier identifier, the hash → height index, and the height → bytes
// record).
func (ms *momentumStore) SetFrontier(momentum *nom.Momentum) error {
	data, err := momentum.Serialize()
	if err != nil {
		return err
	}

	return db.SetFrontier(ms.DB, momentum.Identifier(), data)
}

// parseMomentum is the read-side counterpart to [momentumStore.SetFrontier]:
// resolves the (data, err) pair from a [db.GetEntryBy*] call into either
// a parsed momentum, nil (not-found), or an error.
func parseMomentum(data []byte, err error) (*nom.Momentum, error) {
	if err == leveldb.ErrNotFound {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return nom.DeserializeMomentum(data)
}

// GetFrontierMomentum returns the most recent momentum in this view, or
// nil if the chain is empty.
func (ms *momentumStore) GetFrontierMomentum() (*nom.Momentum, error) {
	return parseMomentum(db.GetEntryByHeight(ms.DB, db.GetFrontierIdentifier(ms.DB).Height))
}

// GetMomentumByHash looks up a momentum by hash.
func (ms *momentumStore) GetMomentumByHash(hash types.Hash) (*nom.Momentum, error) {
	return parseMomentum(db.GetEntryByHash(ms.DB, hash))
}

// GetMomentumsByHash returns up to count momentums starting at the
// momentum identified by blockHash, walking forward (higher == true) or
// backward.
func (ms *momentumStore) GetMomentumsByHash(blockHash types.Hash, higher bool, count uint64) ([]*nom.Momentum, error) {
	momentum, err := ms.GetMomentumByHash(blockHash)
	if err != nil {
		return nil, err
	}
	return ms.GetMomentumsByHeight(momentum.Height, higher, count)
}

// GetMomentumByHeight looks up a momentum by height.
func (ms *momentumStore) GetMomentumByHeight(height uint64) (*nom.Momentum, error) {
	return parseMomentum(db.GetEntryByHeight(ms.DB, height))
}

// GetMomentumsByHeight returns up to count momentums starting at height,
// walking forward or backward depending on `higher`. Used by sync to
// stream chunks of the momentum chain to peers.
func (ms *momentumStore) GetMomentumsByHeight(height uint64, higher bool, count uint64) ([]*nom.Momentum, error) {
	var to, from uint64
	if higher {
		from = height
		to = height + count
	} else {
		if height+1 <= count {
			from = 1
		} else {
			from = height + 1 - count
		}
		to = height + 1
	}
	return ms.getMomentumsByRange(from, to)
}

// PrefetchMomentum bundles momentum together with the full account
// blocks it commits to, returning the [nom.DetailedMomentum] that the
// verifier consumes.
func (ms *momentumStore) PrefetchMomentum(momentum *nom.Momentum) (*nom.DetailedMomentum, error) {
	accountBlocks := make([]*nom.AccountBlock, len(momentum.Content))
	for index := range momentum.Content {
		var err error
		accountBlocks[index], err = ms.GetAccountBlock(*momentum.Content[index])
		if err != nil {
			return nil, fmt.Errorf("error while prefetching account-blocks for insert-momentum event. %w", err)
		}
	}

	return &nom.DetailedMomentum{
		Momentum:      momentum,
		AccountBlocks: accountBlocks,
	}, nil
}

// getMomentumsByRange returns the momentums with heights in [from, to).
func (ms *momentumStore) getMomentumsByRange(from, to uint64) ([]*nom.Momentum, error) {
	list := make([]*nom.Momentum, 0, to-from)
	for i := from; i < to; i += 1 {
		momentum, err := ms.GetMomentumByHeight(i)
		if err != nil {
			return nil, err
		}
		list = append(list, momentum)
	}
	return list, nil
}
