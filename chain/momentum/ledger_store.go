package momentum

import (
	"github.com/pkg/errors"
	"github.com/syndtr/goleveldb/leveldb"

	"github.com/zenon-network/go-zenon/chain/account"
	"github.com/zenon-network/go-zenon/chain/account/mailbox"
	"github.com/zenon-network/go-zenon/chain/nom"
	"github.com/zenon-network/go-zenon/chain/store"
	"github.com/zenon-network/go-zenon/common"
	"github.com/zenon-network/go-zenon/common/db"
	"github.com/zenon-network/go-zenon/common/types"
)

// momentumStore is the [store.Momentum] implementation. It composes a
// [store.Genesis] (for chain identifier and genesis-block constants)
// with a versioned [db.DB] holding everything else: per-account chains,
// per-account mailboxes, the cached ZNN balances used for delegation
// math, the block-confirmation index, and the account-header reverse
// index for hash-only lookups.
type momentumStore struct {
	store.Genesis
	db.DB
}

// getAccountStorePrefix returns the prefix that namespaces an account's
// chain storage within the momentum database.
func getAccountStorePrefix(address types.Address) []byte {
	return common.JoinBytes(accountStorePrefix, address.Bytes())
}

// getAccountMailboxPrefix returns the prefix that namespaces an account's
// mailbox within the momentum database.
func getAccountMailboxPrefix(address types.Address) []byte {
	return common.JoinBytes(accountMailboxPrefix, address.Bytes())
}

// Snapshot returns an isolated copy of this view; subsequent writes are
// captured separately and won't reach the source.
func (ms *momentumStore) Snapshot() store.Momentum {
	return NewStore(ms.Genesis, ms.DB.Snapshot())
}

// GetAccountDB returns the raw [db.DB] subset for address (snapshotted),
// used by callers that read contract storage directly.
func (ms *momentumStore) GetAccountDB(address types.Address) db.DB {
	return ms.DB.Subset(getAccountStorePrefix(address)).Snapshot()
}

// GetAccountStore wraps the raw account database in the [store.Account]
// implementation from
// [github.com/zenon-network/go-zenon/chain/account].
func (ms *momentumStore) GetAccountStore(address types.Address) store.Account {
	return account.NewAccountStore(address, ms.GetAccountDB(address))
}

// getAccountMailbox returns the live (non-snapshotted) mailbox for
// address — used internally where the caller needs to mutate.
func (ms *momentumStore) getAccountMailbox(address types.Address) store.AccountMailbox {
	return mailbox.NewAccountMailbox(address, ms.DB.Subset(getAccountMailboxPrefix(address)))
}

// GetAccountMailbox returns a snapshotted [store.AccountMailbox] for
// address — read-only callers see a stable view of the queue.
func (ms *momentumStore) GetAccountMailbox(address types.Address) store.AccountMailbox {
	return ms.getAccountMailbox(address).Snapshot()
}

// AddAccountBlockTransaction admits the (header, patch) pair into this
// view, applying the per-account patch and updating every cross-cutting
// index touched by the block:
//
//   - Per-account ZNN balance cache (consumed by delegation math).
//   - Per-block confirmation height (consumed by the verifier's
//     auto-generated MomentumAcknowledged check).
//   - Account-header reverse index (so the block can be looked up by
//     hash alone).
//   - Mailbox queues for sends and receives, including the
//     embedded-contract sequencer queue when the recipient is a
//     contract.
//
// Patches with empty dumps (batched contract sends) are no-ops here —
// the parent receive carries them through the chain. See
// [chain/nom.AccountBlockTransaction] for the batching shape.
func (ms *momentumStore) AddAccountBlockTransaction(header types.AccountHeader, patch db.Patch) error {
	// skip batched blocks
	if len(patch.Dump()) == 0 {
		return nil
	}
	identifier := ms.Identifier()
	if err := ms.DB.Subset(getAccountStorePrefix(header.Address)).Apply(patch); err != nil {
		return nil
	}

	// Set znn balance
	accountStore := ms.GetAccountStore(header.Address)
	znnBalance, err := accountStore.GetBalance(types.ZnnTokenStandard)
	if err != nil {
		return err
	}
	if err := ms.setZnnBalance(header.Address, znnBalance); err != nil {
		return err
	}

	block, err := ms.GetAccountBlock(header)
	if err != nil {
		return err
	}
	if block == nil {
		return errors.Errorf("can't find block for header %v", header)
	}

	blocks := []*nom.AccountBlock{block}
	blocks = append(blocks, block.DescendantBlocks...)

	for _, block := range blocks {
		if err := ms.addAccountBlockHeader(block.Header()); err != nil {
			return err
		}
		if err := ms.setBlockConfirmationHeight(block.Hash, identifier.Height+1); err != nil {
			return nil
		}

		if block.IsSendBlock() {
			othStore := ms.getAccountMailbox(block.ToAddress)
			if err := othStore.MarkAsUnreceived(block.Hash); err != nil {
				return err
			}

			if types.IsEmbeddedAddress(block.ToAddress) {
				othStore.SequencerPushBack(block.Header())
			}
		} else if block.BlockType != nom.BlockTypeGenesisReceive {
			fromBlock, err := ms.GetAccountBlockByHash(block.FromBlockHash)
			if err != nil {
				return err
			}
			if fromBlock == nil {
				return errors.Errorf("Impossible. Can't find from-block in store")
			}
			fromStore := ms.getAccountMailbox(fromBlock.Address)
			if err := fromStore.MarkBlockThatReceives(block.FromBlockHash, block.Header()); err != nil {
				return err
			}

			myStore := ms.getAccountMailbox(block.Address)
			if err := myStore.MarkAsReceived(block.FromBlockHash); err != nil {
				return err
			}
		}
	}

	return nil
}

// Identifier returns the (Hash, Height) of the frontier momentum in this
// view, or [types.ZeroHashHeight] for an empty momentum chain.
func (ms *momentumStore) Identifier() types.HashHeight {
	frontier, err := ms.GetFrontierMomentum()
	if frontier == nil || err == leveldb.ErrNotFound {
		return types.HashHeight{
			Height: 0,
			Hash:   types.ZeroHash,
		}
	} else {
		common.DealWithErr(err)
		return frontier.Identifier()
	}
}

// NewStore wraps db in a [store.Momentum] backed by genesis.
// Panics if db is nil — the store cannot operate without backing storage.
func NewStore(genesis store.Genesis, db db.DB) store.Momentum {
	if db == nil {
		panic("momentum store can't operate with nil db")
	}
	return &momentumStore{
		Genesis: genesis,
		DB:      db,
	}
}

// NewGenesisStore returns an in-memory [store.Momentum] suitable for
// constructing the genesis transaction — the chain layer uses this
// during boot before the persistent store has been opened.
func NewGenesisStore() store.Momentum {
	return &momentumStore{
		Genesis: nil,
		DB:      db.NewMemDB(),
	}
}
