package account

import (
	"github.com/syndtr/goleveldb/leveldb"

	"github.com/zenon-network/go-zenon/chain/store"
	"github.com/zenon-network/go-zenon/common"
	"github.com/zenon-network/go-zenon/common/types"
)

// sequencerFrontIndex returns the height of the last sequencer entry
// already consumed by this account; new receives must consume the entry
// at sequencerFrontIndex + 1.
func (as *accountStore) sequencerFrontIndex() uint64 {
	data, err := as.DB.Get(sequencerLastReceivedKey)
	if err == leveldb.ErrNotFound {
		return 0
	}
	return common.BytesToUint64(data)
}

// SequencerFront returns the [types.AccountHeader] of the next inbound
// send this account is expected to consume per the embedded-contract
// sequencer rule, or nil if the mailbox queue is empty for this account.
//
// Panics if mailbox.Address() does not match this account — passing the
// wrong mailbox is a programmer error.
func (as *accountStore) SequencerFront(mailbox store.AccountMailbox) *types.AccountHeader {
	if mailbox.Address() != as.address {
		panic("not my mailbox")
	}
	last := as.sequencerFrontIndex()
	total := mailbox.SequencerSize()
	if last == total {
		return nil
	}
	return mailbox.SequencerByHeight(last + 1)
}

// SequencerPopFront advances the sequencer cursor by one. Called by the
// chain layer after a contract receive has been committed so the next
// receive sees the head of the queue advance.
//
// Panics on a database error — sequencer mutations are committed inside
// the chain insert lock and an IO failure here is a fatal condition.
func (as *accountStore) SequencerPopFront() {
	last := as.sequencerFrontIndex()
	common.DealWithErr(as.DB.Put(sequencerLastReceivedKey, common.Uint64ToBytes(last+1)))
}
