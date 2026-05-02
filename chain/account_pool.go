package chain

import (
	"bytes"
	"fmt"
	"sync"

	"github.com/inconshreveable/log15"
	"github.com/pkg/errors"

	"github.com/zenon-network/go-zenon/chain/account"
	"github.com/zenon-network/go-zenon/chain/nom"
	"github.com/zenon-network/go-zenon/chain/store"
	"github.com/zenon-network/go-zenon/common"
	"github.com/zenon-network/go-zenon/common/db"
	"github.com/zenon-network/go-zenon/common/types"
)

// Sentinel errors returned by [AccountPool.AddAccountBlockTransaction]
// and the constant capping how many account blocks a single momentum
// may commit.
var (
	// ErrFailedToAddAccountBlockTransaction is the umbrella sentinel
	// for any account-pool insert failure; the wrapped error carries
	// the specific reason.
	ErrFailedToAddAccountBlockTransaction = errors.Errorf("failed to insert account-block-transaction")
	// ErrPlasmaRatioIsWorse signals that an inbound block has a worse
	// plasma price (TotalPlasma/BasePlasma ratio) than the block
	// already at the same height. See [higherPriority].
	ErrPlasmaRatioIsWorse = errors.Errorf("plasma ratio is smaller for current block")
	// ErrHashTieBreak signals that an inbound block has the same plasma
	// price as the existing block but a numerically larger hash; the
	// smaller hash wins.
	ErrHashTieBreak = errors.Errorf("hash tie-break is worse for current block")

	// MaxAccountBlocksInMomentum is the cap on the [nom.Momentum.Content]
	// list. Counts every batched contract send (descendants count
	// individually toward the cap), even though they ride along with a
	// parent receive in the wire form.
	MaxAccountBlocksInMomentum = 100
)

// Stable abstracts the persistent layer the account pool layers
// uncommitted state on top of. The momentum pool implements this
// interface; tests can substitute a mock to drive the pool against
// synthetic state.
type Stable interface {
	// GetStableAccountDB returns the persistent [db.DB] view of
	// address's account chain at the most recent committed momentum.
	GetStableAccountDB(address types.Address) db.DB
}

// accountPool holds the in-memory, uncommitted account-block state per
// address. Each address has its own [db.Manager] (created lazily on
// first touch) sitting on top of the stable store.
//
// Concurrency: every public method takes ap.changes; the pool is safe
// for concurrent use.
type accountPool struct {
	log      log15.Logger
	stable   Stable
	managers map[types.Address]db.Manager
	changes  sync.Mutex
}

// getAccountManager returns address's per-address manager, creating it
// lazily atop the stable store on first request.
func (ap *accountPool) getAccountManager(address types.Address) db.Manager {
	manager := ap.managers[address]
	if manager == nil {
		manager = db.NewMemDBManager(ap.stable.GetStableAccountDB(address))
		ap.managers[address] = manager
	}
	return manager
}

// canRollback reports whether the in-flight block can replace the
// uncommitted entry currently sitting at its height. Refuses when the
// proposed identifier is older than the stable store (already
// finalized) or when its previous hash does not match the chain at
// height-1. Returns nil when a rollback is permissible.
func (ap *accountPool) canRollback(block *nom.AccountBlock) error {
	log := ap.log.New("header", block.Header())
	address := block.Address
	identifier := block.Identifier()
	previous := block.Previous()

	stable := ap.getStableAccountStore(address)
	stableIdentifier := stable.Identifier()

	// can't insert at all since it's too old
	if stableIdentifier.Height >= identifier.Height {
		log.Info("failed to insert account-block-transaction", "reason", "older than stable identifier", "stable-identifier", stableIdentifier)
		return fmt.Errorf(`%w reason:%v; stable-identifier:%v; identifier:%v`, ErrFailedToAddAccountBlockTransaction, "older than stable identifier", stableIdentifier, identifier)
	}

	frontier := ap.getFrontierAccountStore(address)
	frontierIdentifier := frontier.Identifier()

	// previous doesn't match
	truePrevious, err := frontier.ByHeight(identifier.Height - 1)
	if err != nil {
		log.Info("failed to insert account-block-transaction", "reason", err, "frontier-identifier", frontierIdentifier)
		return fmt.Errorf(`%w reason:%v; frontier-identifier:%v; identifier:%v`, ErrFailedToAddAccountBlockTransaction, err, frontierIdentifier, identifier)
	}
	if truePrevious == nil {
		log.Info("failed to insert account-block-transaction", "reason", "no previous", "frontier-identifier", frontierIdentifier)
		return fmt.Errorf(`%w reason:%v; frontier-identifier:%v; identifier:%v`, ErrFailedToAddAccountBlockTransaction, "missing previous", frontierIdentifier, identifier)
	}
	if truePrevious.Identifier() != previous {
		log.Info("failed to insert account-block-transaction", "reason", "previous mismatch", "frontier-identifier", frontierIdentifier)
		return fmt.Errorf(`%w reason:%v; frontier-identifier:%v; identifier:%v`, ErrFailedToAddAccountBlockTransaction, "missing previous", frontierIdentifier, identifier)
	}

	return nil
}

// higherPriority compares two competing account blocks at the same
// height. The winner is the block with the higher plasma price
// (TotalPlasma/BasePlasma ratio), with the smaller hash as deterministic
// tie-breaker. Returns nil when a is strictly better than b, otherwise
// one of [ErrPlasmaRatioIsWorse] or [ErrHashTieBreak].
//
// The cross-multiply form avoids floating-point and integer-overflow
// concerns while remaining branch-free.
func higherPriority(a, b *nom.AccountBlock) error {
	if a.TotalPlasma*b.BasePlasma < b.TotalPlasma*a.BasePlasma {
		return ErrPlasmaRatioIsWorse
	} else if a.TotalPlasma*b.BasePlasma == b.TotalPlasma*a.BasePlasma && bytes.Compare(a.Hash.Bytes()[:], b.Hash.Bytes()[:]) > -1 {
		return ErrHashTieBreak
	}

	return nil
}

// AddAccountBlockTransaction commits transaction onto the pool's
// frontier for transaction.Block.Address. Caller must hold the chain
// insert lock (the insertLocker argument). Inserts may rollback
// uncommitted blocks above the new block's height when the new block
// wins the [higherPriority] tie-break.
func (ap *accountPool) AddAccountBlockTransaction(insertLocker sync.Locker, transaction *nom.AccountBlockTransaction) error {
	if insertLocker == nil {
		return errors.Errorf("insertLocker can't be nil")
	}
	ap.changes.Lock()
	defer ap.changes.Unlock()
	return ap.addAccountBlockTransaction(transaction, false)
}

// ForceAddAccountBlockTransaction is the unconditional variant of
// [AddAccountBlockTransaction]; it bypasses the [higherPriority]
// tie-break. Used by tests and the genesis loader.
func (ap *accountPool) ForceAddAccountBlockTransaction(insertLocker sync.Locker, transaction *nom.AccountBlockTransaction) error {
	if insertLocker == nil {
		return errors.Errorf("insertLocker can't be nil")
	}
	ap.changes.Lock()
	defer ap.changes.Unlock()
	return ap.addAccountBlockTransaction(transaction, true)
}

// addAccountBlockTransaction is the internal insert path shared by
// [AddAccountBlockTransaction] and [ForceAddAccountBlockTransaction].
// Caller must hold ap.changes. Behavior:
//
//   - Fast-forward append when previous matches the current frontier.
//   - Idempotent return when the block is already inserted.
//   - Reject when the block is older than the stable store or its
//     previous-hash linkage is broken (via [canRollback]).
//   - Conditional rollback + insert when a higher-priority block
//     supersedes a lower-priority one (skipped under forceAdd).
func (ap *accountPool) addAccountBlockTransaction(transaction *nom.AccountBlockTransaction, forceAdd bool) error {
	block := transaction.Block
	address := block.Address
	identifier := block.Identifier()
	previous := block.Previous()

	log := ap.log.New("header", block.Header())

	frontier := ap.getFrontierAccountStore(address)
	frontierIdentifier := frontier.Identifier()

	// fast-forward insert on top of chain
	if previous == frontierIdentifier {
		log.Info("fast-forward inserting account-block")
		return ap.getAccountManager(address).Add(transaction)
	}

	// already inserted
	trueBlock, err := frontier.ByHeight(identifier.Height)
	if err != nil {
		log.Info("failed to insert account-block-transaction", "reason", err, "frontier-identifier", frontierIdentifier)
		return fmt.Errorf(`%w reason:%v; frontier-identifier:%v; identifier:%v`, ErrFailedToAddAccountBlockTransaction, err, frontierIdentifier, identifier)
	}
	if trueBlock != nil && trueBlock.Identifier() == identifier {
		log.Info("account-block is already inserted")
		return nil
	}

	if err := ap.canRollback(block); err != nil {
		return err
	}
	if err := higherPriority(block, trueBlock); !forceAdd && err != nil {
		log.Info("failed to insert account-block-transaction", "reason", err, "frontier-identifier", frontierIdentifier)
		return err
	}

	// rollback blocks and insert this one
	manager := ap.getAccountManager(address)
	for {
		currentIdentifier := db.GetFrontierIdentifier(manager.Frontier())
		if currentIdentifier == previous {
			break
		}
		log.Info("rolling back account-block-transaction", "current-identifier", currentIdentifier)
		err = manager.Pop()
		if err != nil {
			log.Info("failed to insert account-block-transaction. can't pop manager", "reason", err, "frontier-identifier", currentIdentifier)
			return fmt.Errorf(`%w can't pop manager; reason:%v; frontier-identifier:%v; identifier:%v`, ErrFailedToAddAccountBlockTransaction, err, currentIdentifier, identifier)
		}
	}

	log.Info("inserting account-block after rollback")
	return ap.getAccountManager(address).Add(transaction)
}

// GetPatch returns the forward patch the pool committed at identifier
// for address, or nil when identifier is not in the pool.
func (ap *accountPool) GetPatch(address types.Address, identifier types.HashHeight) db.Patch {
	ap.changes.Lock()
	defer ap.changes.Unlock()

	return ap.getAccountManager(address).GetPatch(identifier)
}

// GetAccountStore returns a [store.Account] view at identifier. Returns
// nil when identifier is older than the stable store (rolled off the
// pool) or when no such version is retained.
func (ap *accountPool) GetAccountStore(address types.Address, identifier types.HashHeight) store.Account {
	ap.changes.Lock()
	defer ap.changes.Unlock()

	stable := ap.getStableAccountStore(address)
	stableIdentifier := stable.Identifier()
	if stableIdentifier == identifier {
		return stable
	} else if stableIdentifier.Height > identifier.Height {
		ap.log.Info("unable to get account store", "address", address, "stable-identifier", stableIdentifier, "reason", "older than most stable account")
		return nil
	}

	manager := ap.getAccountManager(address)
	accountDb := manager.Get(identifier)
	if accountDb == nil {
		frontier := db.GetFrontierIdentifier(manager.Frontier())
		ap.log.Info("unable to get account store", "address", address, "frontier-identifier", frontier, "reason", "missing-db")
		return nil
	}
	return account.NewAccountStore(address, accountDb)
}

// GetFrontierAccountStore returns a [store.Account] view of the pool's
// frontier (committed + uncommitted blocks) for address.
func (ap *accountPool) GetFrontierAccountStore(address types.Address) store.Account {
	ap.changes.Lock()
	defer ap.changes.Unlock()

	return ap.getFrontierAccountStore(address)
}

// getStableAccountStore returns a [store.Account] over the stable
// (committed) frontier for address. Caller must hold ap.changes.
func (ap *accountPool) getStableAccountStore(address types.Address) store.Account {
	return account.NewAccountStore(address, db.NewMemDBManager(ap.stable.GetStableAccountDB(address)).Frontier())
}

// getFrontierAccountStore returns a [store.Account] over the pool's
// frontier (committed + uncommitted) for address. Caller must hold
// ap.changes.
func (ap *accountPool) getFrontierAccountStore(address types.Address) store.Account {
	return account.NewAccountStore(address, ap.getAccountManager(address).Frontier())
}

// InsertMomentum implements [MomentumEventListener]: when a momentum is
// committed the pool prunes any per-address managers whose blocks are
// now finalized and re-applies any still-uncommitted blocks on top of
// the new stable layer.
func (ap *accountPool) InsertMomentum(detailed *nom.DetailedMomentum) {
	ap.changes.Lock()
	defer ap.changes.Unlock()

	if err := ap.rebuild(detailed); err != nil {
		common.ChainLogger.Error("failed to handle InsertMomentum in AccountPool", "reason", err)
	}
}

// DeleteMomentum implements [MomentumEventListener]: drops every
// per-address manager so the pool re-derives its state from the new
// stable frontier on next access.
func (ap *accountPool) DeleteMomentum(*nom.DetailedMomentum) {
	ap.changes.Lock()
	defer ap.changes.Unlock()

	ap.managers = make(map[types.Address]db.Manager)
}

// rebuild walks every per-address manager, snapshots its uncommitted
// blocks, recreates the manager on top of the new stable layer, and
// re-applies the uncommitted blocks. Used by [InsertMomentum] to keep
// the pool consistent with the persistent chain after a commit.
func (ap *accountPool) rebuild(detailed *nom.DetailedMomentum) error {
	addresses := make([]types.Address, 0, len(ap.managers))
	for address := range ap.managers {
		addresses = append(addresses, address)
	}

	ap.log.Debug("started rebuilding account-pool", "momentum-identifier", detailed.Momentum.Identifier())
	for _, address := range addresses {
		log := ap.log.New("address", address)
		log.Debug("start rebuilding")

		uncommitted := make([]*nom.AccountBlock, 0)
		oldManager := ap.managers[address]

		stable := account.NewAccountStore(address, ap.stable.GetStableAccountDB(address))
		uncommittedStore := account.NewAccountStore(address, oldManager.Frontier())
		for i := stable.Identifier().Height + 1; i <= uncommittedStore.Identifier().Height; i += 1 {
			block, err := uncommittedStore.ByHeight(i)
			common.DealWithErr(err)
			uncommitted = append(uncommitted, block)
		}

		delete(ap.managers, address)

		if len(uncommitted) == 0 {
			log.Debug("no uncommitted changes")
			continue
		}

		log.Debug("staring applying blocks", "num-uncommitted", len(uncommitted))
		manager := db.NewMemDBManager(ap.stable.GetStableAccountDB(address))
		for _, block := range uncommitted {
			patch := oldManager.GetPatch(block.Identifier())
			err := manager.Add(&nom.AccountBlockTransaction{
				Block:   block,
				Changes: patch,
			})
			if err != nil {
				return errors.Errorf("account pool rebuild error. Unable to re-apply block %v. Reason %v", block.Header(), err)
			}
		}
		ap.managers[address] = manager

		log.Debug("successfully rebuild", "num-uncommitted", len(uncommitted))
	}

	ap.log.Debug("finished rebuilding account-pool")
	return nil
}

// GetNewMomentumContent returns the slice of uncommitted account blocks
// the next momentum should commit, capped at [MaxAccountBlocksInMomentum]
// and respecting batched-descendant boundaries.
func (ap *accountPool) GetNewMomentumContent() []*nom.AccountBlock {
	return ap.filterBlocksToCommit(ap.GetAllUncommittedAccountBlocks())
}

// filterBlocksToCommit walks blocks accumulating batches of blocks
// terminating at non-[nom.BlockTypeContractSend] entries. A batch is
// admitted only if the running total stays under
// [MaxAccountBlocksInMomentum]; otherwise the loop stops short to avoid
// fragmenting a parent receive from its descendants.
func (ap *accountPool) filterBlocksToCommit(blocks []*nom.AccountBlock) []*nom.AccountBlock {
	toCommit := make([]*nom.AccountBlock, 0, len(blocks))
	batch := make([]*nom.AccountBlock, 0, MaxAccountBlocksInMomentum)
	for index := range blocks {
		batch = append(batch, blocks[index])
		if blocks[index].BlockType != nom.BlockTypeContractSend {
			if len(toCommit)+len(batch) > MaxAccountBlocksInMomentum {
				break
			}
			toCommit = append(toCommit, batch...)
			batch = batch[:0]
		}
	}
	return toCommit
}

// GetAllUncommittedAccountBlocks enumerates every uncommitted account
// block currently in the pool, across all addresses.
func (ap *accountPool) GetAllUncommittedAccountBlocks() []*nom.AccountBlock {
	ap.changes.Lock()
	defer ap.changes.Unlock()

	blocks := make([]*nom.AccountBlock, 0)
	for address := range ap.managers {
		blocks = append(blocks, ap.getUncommittedAccountBlocksByAddress(address)...)
	}

	return blocks
}

// GetUncommittedAccountBlocksByAddress returns the uncommitted blocks
// for one address only.
func (ap *accountPool) GetUncommittedAccountBlocksByAddress(address types.Address) []*nom.AccountBlock {
	ap.changes.Lock()
	defer ap.changes.Unlock()

	return ap.getUncommittedAccountBlocksByAddress(address)
}

// getUncommittedAccountBlocksByAddress is the locked-caller variant of
// [GetUncommittedAccountBlocksByAddress]; caller must hold ap.changes.
// Walks every height between the stable frontier and the pool frontier.
func (ap *accountPool) getUncommittedAccountBlocksByAddress(address types.Address) []*nom.AccountBlock {
	blocks := make([]*nom.AccountBlock, 0)

	stable := ap.getStableAccountStore(address)
	frontier := ap.getFrontierAccountStore(address)
	for i := stable.Identifier().Height + 1; i <= frontier.Identifier().Height; i += 1 {
		block, err := frontier.ByHeight(i)
		common.DealWithErr(err)
		blocks = append(blocks, block)
	}

	return blocks
}

// newAccountPool constructs an [accountPool] backed by stable. Internal
// constructor used by [NewChain]; external callers use [NewAccountPool].
func newAccountPool(stable Stable) *accountPool {
	return &accountPool{
		log:      common.ChainLogger.New("module", "account-pool"),
		stable:   stable,
		managers: make(map[types.Address]db.Manager),
	}
}

// NewAccountPool wraps [newAccountPool] in the [AccountPool] interface
// for callers outside this package (notably the genesis loader).
func NewAccountPool(stable Stable) AccountPool {
	return newAccountPool(stable)
}
