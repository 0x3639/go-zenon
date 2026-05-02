package account

import (
	"github.com/pkg/errors"
	"github.com/syndtr/goleveldb/leveldb"

	"github.com/zenon-network/go-zenon/chain/nom"
	"github.com/zenon-network/go-zenon/common/db"
	"github.com/zenon-network/go-zenon/common/types"
)

// SetFrontier writes block as the new frontier of this account chain via
// the shared [db.SetFrontier] helper (which updates the frontier
// identifier, the hash → height index, and the height → bytes record
// in one atomic batch).
func (as *accountStore) SetFrontier(block *nom.AccountBlock) error {
	data, err := block.Serialize()
	if err != nil {
		return err
	}

	return db.SetFrontier(as.DB, block.Identifier(), data)
}

// parseAccountBlock is the read-side counterpart of [accountStore.SetFrontier]:
// resolves the (data, err) pair from a [db.GetEntryBy*] call into either
// a parsed block, nil (not-found), or an error.
func parseAccountBlock(data []byte, err error) (*nom.AccountBlock, error) {
	if err == leveldb.ErrNotFound {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if block, err := nom.DeserializeAccountBlock(data); err != nil {
		return nil, errors.Errorf("failed to deserialize account-block; reason: %v", err.Error())
	} else {
		return block, nil
	}
}

// Frontier returns the frontier block of this account chain, or nil if
// the chain is empty.
func (as *accountStore) Frontier() (*nom.AccountBlock, error) {
	return parseAccountBlock(db.GetEntryByHeight(as.DB, db.GetFrontierIdentifier(as.DB).Height))
}

// ByHash looks up a block in this account chain by hash.
func (as *accountStore) ByHash(hash types.Hash) (*nom.AccountBlock, error) {
	return parseAccountBlock(db.GetEntryByHash(as.DB, hash))
}

// ByHeight looks up the block at height in this account chain.
func (as *accountStore) ByHeight(height uint64) (*nom.AccountBlock, error) {
	return parseAccountBlock(db.GetEntryByHeight(as.DB, height))
}

// MoreByHeight returns up to count blocks starting at height in
// ascending order. Missing blocks contribute nil entries to the slice.
func (as *accountStore) MoreByHeight(height, count uint64) ([]*nom.AccountBlock, error) {
	answer := make([]*nom.AccountBlock, 0)
	for i := 0; i < int(count); i += 1 {
		block, err := as.ByHeight(height + uint64(i))
		if err != nil {
			return nil, err
		}
		answer = append(answer, block)
	}
	return answer, nil
}
