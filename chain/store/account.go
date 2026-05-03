package store

import (
	"math/big"

	"github.com/zenon-network/go-zenon/chain/nom"
	"github.com/zenon-network/go-zenon/common/db"
	"github.com/zenon-network/go-zenon/common/types"
)

// Account is the read/write surface for a single account's chain. The chain
// layer hands one out per `(address, point-in-time)` so consumers operate
// against an isolated, versioned snapshot of that account's history.
//
// Concurrency: snapshots taken via [Account.Snapshot] are not goroutine-safe;
// callers should derive a fresh snapshot per goroutine.
type Account interface {
	// Identifier returns the (Hash, Height) of this account chain's
	// frontier in the view — i.e., the last account block visible here.
	Identifier() types.HashHeight
	// Address returns the account this store belongs to.
	Address() *types.Address

	// Storage returns the underlying [db.DB] for the account so embedded
	// contracts and the VM can read/write contract storage directly.
	Storage() db.DB

	// Frontier returns the frontier account block for this view.
	Frontier() (*nom.AccountBlock, error)
	// ByHash looks up a block in this account chain by hash.
	ByHash(hash types.Hash) (*nom.AccountBlock, error)
	// ByHeight looks up the block at height in this account chain.
	ByHeight(height uint64) (*nom.AccountBlock, error)
	// MoreByHeight returns up to count blocks starting at height in
	// ascending height order.
	MoreByHeight(height, count uint64) ([]*nom.AccountBlock, error)

	// GetBalance returns the cached balance for the supplied token. Used
	// by the verifier and VM to short-circuit overdraft checks without
	// re-walking the chain.
	GetBalance(zts types.ZenonTokenStandard) (*big.Int, error)
	// SetBalance overwrites the cached balance for the supplied token.
	SetBalance(zts types.ZenonTokenStandard, balance *big.Int) error
	// GetBalanceMap returns the cached balances for every token this
	// account holds.
	GetBalanceMap() (map[types.ZenonTokenStandard]*big.Int, error)

	// GetChainPlasma returns the per-account plasma counter the
	// rate-limiter uses.
	GetChainPlasma() (*big.Int, error)
	// AddChainPlasma adds the supplied value to the per-account plasma counter.
	AddChainPlasma(uint64) error

	// MarkAsReceived records that hash (a send block referenced by
	// FromBlockHash) has been consumed; subsequent receives of the same
	// hash are rejected by the verifier.
	MarkAsReceived(hash types.Hash) error
	// IsReceived reports whether hash has already been consumed.
	IsReceived(hash types.Hash) bool

	// SequencerFront returns the next inbound send the account is
	// expected to consume per the embedded-contract sequencer rule, or
	// nil when the mailbox is empty for this account.
	SequencerFront(mailbox AccountMailbox) *types.AccountHeader
	// SequencerPopFront drops the head of the sequencer queue; called by
	// the chain layer after a contract receive has been committed.
	SequencerPopFront()

	// Apply replays patch onto this account view. Returns the first
	// error encountered.
	Apply(patch db.Patch) error
	// Snapshot returns an isolated copy of this view; subsequent writes
	// go into the snapshot, not the source.
	Snapshot() Account
	// Changes returns the patch describing every write made to this view
	// since it was created.
	Changes() (db.Patch, error)
}

// AccountMailbox is the per-recipient queue of inbound sends awaiting
// consumption. Together with the sequencer rules in [Account] it lets
// embedded contracts process inbound traffic in a deterministic, FIFO
// order that every node agrees on.
type AccountMailbox interface {
	// Address returns the recipient address whose mailbox this is.
	Address() types.Address
	// Snapshot returns an isolated copy of the mailbox.
	Snapshot() AccountMailbox

	// MarkAsUnreceived records that hash (a send block) has been
	// admitted to the mailbox awaiting consumption.
	MarkAsUnreceived(hash types.Hash) error
	// MarkAsReceived records that hash has been consumed; the verifier
	// uses this state to reject double-receives.
	MarkAsReceived(hash types.Hash) error
	// MarkBlockThatReceives records that the receive block named by
	// receiveHeader consumed the send identified by hash. Used to answer
	// [Momentum.GetBlockWhichReceives] without re-walking the chain.
	MarkBlockThatReceives(hash types.Hash, receiveHeader types.AccountHeader) error

	// GetBlockWhichReceives returns the receive header that consumed
	// fromHash, or nil if the send is still unreceived.
	GetBlockWhichReceives(fromHash types.Hash) *types.AccountHeader
	// GetUnreceivedAccountBlockHashes returns up to atMost still-unreceived
	// inbound send hashes in their canonical order.
	GetUnreceivedAccountBlockHashes(atMost uint64) ([]types.Hash, error)

	// SequencerPushBack appends header to the sequencer queue (the FIFO
	// of pending sends) and is called when a new send to this address is
	// confirmed.
	SequencerPushBack(types.AccountHeader)
	// SequencerSize returns the number of pending sends in the queue.
	SequencerSize() uint64
	// SequencerByHeight returns the queued send at position height (1-based),
	// or nil if it is out of range.
	SequencerByHeight(uint64) *types.AccountHeader
}
