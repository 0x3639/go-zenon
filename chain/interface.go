package chain

import (
	"sync"

	"github.com/zenon-network/go-zenon/chain/nom"
	"github.com/zenon-network/go-zenon/chain/store"
	"github.com/zenon-network/go-zenon/common/db"
	"github.com/zenon-network/go-zenon/common/types"
)

// Chain is the public surface of the chain subsystem. It composes the
// genesis store, the account pool, the momentum pool, and the momentum
// event manager into one handle so consumers (verifier, VM, RPC,
// protocol) can reach every chain-state primitive through a single
// interface.
//
// Concurrency: every method is safe for concurrent use; mutations
// serialize through the chain insert lock obtained via [Chain.AcquireInsert].
type Chain interface {
	// Init prepares the chain: cross-checks the embedded genesis against
	// what is on disk, registers the account-pool listener with the
	// momentum-event manager, and refuses to start when an active spork
	// is unrecognized by this binary.
	Init() error
	// Start currently a no-op alongside Init; reserved for future
	// background-loop wiring.
	Start() error
	// Stop unregisters the account-pool listener and closes the
	// underlying database manager.
	Stop() error

	// AcquireInsert is used to limit insert operations in a global way inside the chain module.
	// The actual sync.Locker object returned is used for logging purposes and any method receiving such argument
	// does not enforce in any way the validity, only the fact that is non-nil.
	AcquireInsert(reason string) sync.Locker

	store.Genesis
	AccountPool
	MomentumPool
	MomentumEventManager
}

// MomentumEventListener is the contract a subsystem implements to
// observe momentum-chain mutations. The chain layer broadcasts
// InsertMomentum after a momentum is committed and DeleteMomentum
// after one is rolled back. Listeners are invoked synchronously on
// the goroutine that called [MomentumPool.AddMomentumTransaction] or
// [MomentumPool.RollbackTo] — heavy work should be deferred.
type MomentumEventListener interface {
	// InsertMomentum is called once per committed momentum, after the
	// underlying database mutation has succeeded.
	InsertMomentum(*nom.DetailedMomentum)
	// DeleteMomentum is called once per rolled-back momentum, after the
	// rollback has been applied.
	DeleteMomentum(*nom.DetailedMomentum)
}

// MomentumEventManager is the registration surface for
// [MomentumEventListener]. The account pool, the broadcaster, and the
// RPC subscription server all register through this interface.
type MomentumEventManager interface {
	// Register adds listener to the broadcast list. Idempotent on
	// pointer equality is the caller's responsibility — registering the
	// same listener twice will cause two broadcasts per event.
	Register(MomentumEventListener)
	// UnRegister removes listener (by pointer equality) from the
	// broadcast list. No-op if listener is not registered.
	UnRegister(MomentumEventListener)
}

// MomentumPool is the read/write surface for the global momentum chain.
// Mutations require the chain insert lock; readers do not.
type MomentumPool interface {
	// AddMomentumTransaction commits a [nom.MomentumTransaction] onto
	// the frontier. The insertLocker must be the [sync.Locker] returned
	// by [Chain.AcquireInsert]; it is used as a tag for logging and to
	// signal that the caller already holds the global insert lock.
	AddMomentumTransaction(insertLocker sync.Locker, transaction *nom.MomentumTransaction) error
	// RollbackTo reverses momentums down to identifier. Used by the
	// reorg path; nothing is committed below identifier.
	RollbackTo(insertLocker sync.Locker, identifier types.HashHeight) error

	// GetFrontierMomentumStore returns a [store.Momentum] view of the
	// most recent committed momentum.
	GetFrontierMomentumStore() store.Momentum
	// GetMomentumStore returns a [store.Momentum] view at identifier,
	// or nil if the identifier is unknown or has been pruned.
	GetMomentumStore(identifier types.HashHeight) store.Momentum
}

// AccountPool is the read/write surface for the in-memory layer of
// uncommitted account blocks that sit on top of the persistent chain.
// Inserts may rollback uncommitted blocks but never confirmed ones.
type AccountPool interface {
	// AddAccountBlockTransaction implements the whole logic required to manage an account-chain.
	// When inserting a new account-block-transaction, is possible to trigger rollbacks of other unconfirmed account-blocks.
	//
	// Note: A confirmed account-block will never be rollback.
	//
	// In case a fork is detected, there is a deterministic way to find the longest chain, as follows:
	//  - the account-block with the biggest TotalPlasma/BasePlasma is selected
	//  - the account-block with the smallest hash
	AddAccountBlockTransaction(insertLocker sync.Locker, transaction *nom.AccountBlockTransaction) error
	// ForceAddAccountBlockTransaction inserts unconditionally, skipping
	// the [higherPriority] tie-break. Used by tests and by the genesis
	// loader; production paths use [AddAccountBlockTransaction].
	ForceAddAccountBlockTransaction(insertLocker sync.Locker, transaction *nom.AccountBlockTransaction) error

	// GetPatch returns the forward patch for the given (address,
	// identifier) pair, or nil when the identifier is not in the pool.
	GetPatch(address types.Address, identifier types.HashHeight) db.Patch
	// GetAccountStore returns a [store.Account] view at identifier, or
	// nil when the identifier is older than the stable store or absent
	// from the pool.
	GetAccountStore(address types.Address, identifier types.HashHeight) store.Account
	// GetFrontierAccountStore returns a [store.Account] view of the
	// pool's frontier for address (committed + uncommitted blocks).
	GetFrontierAccountStore(address types.Address) store.Account

	// GetNewMomentumContent returns the slice of uncommitted account
	// blocks the next momentum should commit, capped at
	// [MaxAccountBlocksInMomentum] and respecting batched-descendant
	// boundaries (a parent receive and its contract sends are committed
	// together or not at all).
	GetNewMomentumContent() []*nom.AccountBlock
	// GetAllUncommittedAccountBlocks enumerates every uncommitted
	// account block currently in the pool, across all addresses.
	GetAllUncommittedAccountBlocks() []*nom.AccountBlock
	// GetUncommittedAccountBlocksByAddress returns the uncommitted
	// blocks for one address only.
	GetUncommittedAccountBlocksByAddress(address types.Address) []*nom.AccountBlock
}
