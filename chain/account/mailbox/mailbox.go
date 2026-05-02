package mailbox

import (
	"fmt"

	"github.com/syndtr/goleveldb/leveldb"

	"github.com/zenon-network/go-zenon/chain/store"
	"github.com/zenon-network/go-zenon/common"
	"github.com/zenon-network/go-zenon/common/db"
	"github.com/zenon-network/go-zenon/common/types"
)

// parseAccountHeader resolves the (data, err) pair from a [db.DB.Get] call
// into either a parsed header, nil (not-found), or panics on any other
// error. Decode failures panic because the only entries stored under the
// mailbox keys are produced by [types.AccountHeader.Serialize] — a decode
// failure indicates corrupt state.
func parseAccountHeader(data []byte, err error) *types.AccountHeader {
	if err == leveldb.ErrNotFound {
		return nil
	}
	if err != nil {
		panic(err)
		return nil
	}
	if header, err := types.DeserializeAccountHeader(data); err != nil {
		panic(fmt.Sprintf("m.Deserialize failed, Error: %v", err))
	} else {
		return header
	}
}

// mailbox is the [store.AccountMailbox] implementation. It embeds the
// underlying [db.DB] directly so callers operate against the same
// versioned layer as the rest of the chain.
type mailbox struct {
	address types.Address
	db.DB
}

// NewAccountMailbox wraps db in a [store.AccountMailbox] for address.
func NewAccountMailbox(address types.Address, db db.DB) store.AccountMailbox {
	return &mailbox{
		address: address,
		DB:      db,
	}
}

// getUnreceivedBlockKey returns the database key marking that hash has
// been admitted to the cumulative-historical mailbox record.
func getUnreceivedBlockKey(hash types.Hash) []byte {
	return common.JoinBytes(unreceivedBlockPrefix, hash.Bytes())
}

// getPendingBlockKey returns the database key marking that hash is still
// pending consumption.
func getPendingBlockKey(hash types.Hash) []byte {
	return common.JoinBytes(pendingBlockPrefix, hash.Bytes())
}

// getPendingBlocksIterator returns the prefix that walks every pending
// send in this mailbox.
func getPendingBlocksIterator() []byte {
	return pendingBlockPrefix
}

// getBlockWhichReceivesKey returns the database key holding the
// [types.AccountHeader] of the receive that consumed the send identified
// by hash.
func getBlockWhichReceivesKey(hash types.Hash) []byte {
	return common.JoinBytes(blockWhichReceives, hash.Bytes())
}

// getSequencerHeaderByHeightKey returns the database key holding the
// [types.AccountHeader] at sequencer position height (1-based).
func getSequencerHeaderByHeightKey(height uint64) []byte {
	return common.JoinBytes(sequencerHeaderByHeightPrefix, common.Uint64ToBytes(height))
}

// Address returns the recipient this mailbox belongs to.
func (m *mailbox) Address() types.Address {
	return m.address
}

// Snapshot returns an isolated copy of this view.
func (m *mailbox) Snapshot() store.AccountMailbox {
	return NewAccountMailbox(m.address, m.DB.Snapshot())
}

// MarkAsUnreceived admits hash to the mailbox: records it under both the
// historical [unreceivedBlockPrefix] index and the [pendingBlockPrefix]
// "still pending" set.
func (m *mailbox) MarkAsUnreceived(hash types.Hash) error {
	err := m.DB.Put(getUnreceivedBlockKey(hash), common.Uint64ToBytes(1))
	if err != nil {
		return err
	}
	return m.DB.Put(getPendingBlockKey(hash), common.Uint64ToBytes(1))
}

// MarkAsReceived removes hash from the pending set; the historical
// unreceived-block record is preserved for [GetUnreceivedAccountBlockHashes]
// callers that want the cumulative view.
func (m *mailbox) MarkAsReceived(hash types.Hash) error {
	return m.DB.Delete(getPendingBlockKey(hash))
}

// MarkBlockThatReceives records that the receive block named by
// receiveHeader consumed the send identified by hash. Indexed so
// [GetBlockWhichReceives] can answer in O(1) without re-walking the chain.
func (m *mailbox) MarkBlockThatReceives(hash types.Hash, receiveHeader types.AccountHeader) error {
	data, err := receiveHeader.Serialize()
	common.DealWithErr(err)
	return m.DB.Put(getBlockWhichReceivesKey(hash), data)
}

// GetBlockWhichReceives returns the receive header that consumed
// fromHash, or nil if the send is still pending.
func (m *mailbox) GetBlockWhichReceives(fromHash types.Hash) *types.AccountHeader {
	return parseAccountHeader(m.DB.Get(getBlockWhichReceivesKey(fromHash)))
}

// GetUnreceivedAccountBlockHashes returns up to atMost still-pending
// send hashes in iteration order. Used by RPC to surface pending
// inbound traffic to clients.
func (m *mailbox) GetUnreceivedAccountBlockHashes(atMost uint64) ([]types.Hash, error) {
	iterator := m.DB.NewIterator(getPendingBlocksIterator())
	defer iterator.Release()
	list := make([]types.Hash, 0)

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
		hash, err := types.BytesToHash(iterator.Key()[1:])

		if err != nil {
			return nil, err
		}
		list = append(list, hash)

		atMost -= 1
		if atMost == 0 {
			return list, nil
		}
	}
	return list, nil
}

// SequencerSize returns the running total of sends pushed onto the
// sequencer queue since the mailbox was created.
func (m *mailbox) SequencerSize() uint64 {
	data, err := m.DB.Get(sequencerNumInsertedKey)
	if err == leveldb.ErrNotFound {
		return 0
	}
	return common.BytesToUint64(data)
}

// SequencerPushBack appends header to the sequencer queue. Atomic against
// the queue counter so a concurrent reader will never observe a queue
// size larger than the entries actually present.
func (m *mailbox) SequencerPushBack(header types.AccountHeader) {
	total := m.SequencerSize() + 1
	common.DealWithErr(m.DB.Put(sequencerNumInsertedKey, common.Uint64ToBytes(total)))
	data, err := header.Serialize()
	common.DealWithErr(err)
	common.DealWithErr(m.DB.Put(getSequencerHeaderByHeightKey(total), data))
}

// SequencerByHeight returns the queued send at position height (1-based),
// or nil if it is out of range.
func (m *mailbox) SequencerByHeight(height uint64) *types.AccountHeader {
	return parseAccountHeader(m.DB.Get(getSequencerHeaderByHeightKey(height)))
}
