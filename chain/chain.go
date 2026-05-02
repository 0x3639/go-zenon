package chain

import (
	"fmt"
	"os"
	"sync"

	"github.com/pkg/errors"

	"github.com/zenon-network/go-zenon/chain/store"
	"github.com/zenon-network/go-zenon/common"
	"github.com/zenon-network/go-zenon/common/db"
	"github.com/zenon-network/go-zenon/common/types"
)

// inserterLog is the dedicated log handle for chain insert-lock
// acquisitions. Logging at this granularity makes lock contention easy
// to spot in production traces without polluting the main chain log.
var (
	inserterLog = common.ChainLogger.New("submodule", "chain-insert-mutex")
)

// chain is the [Chain] implementation. It composes the genesis store,
// the in-memory account pool, the persistent momentum pool, and the
// momentum-event manager. The single insert mutex serializes every
// mutation and is exposed to callers via [chain.AcquireInsert].
type chain struct {
	log common.Logger

	store.Genesis
	*accountPool
	*momentumPool
	*momentumEventManager

	chainManager db.Manager
	insert       sync.Mutex
}

// NewChain wires the components together and returns a *chain. Callers
// outside the package should treat the return value as a [Chain].
func NewChain(chainManager db.Manager, genesis store.Genesis) *chain {
	momentumPool := NewMomentumPool(chainManager, genesis)
	return &chain{
		log:                  common.ChainLogger,
		Genesis:              genesis,
		accountPool:          newAccountPool(momentumPool),
		momentumPool:         momentumPool,
		momentumEventManager: momentumPool.momentumEventManager,
		chainManager:         chainManager,
	}
}

// Init brings the chain up: checks the embedded genesis against the
// on-disk chain, sets [types.SporkAddress] from the genesis config,
// registers the account-pool listener, prints the current frontier,
// and refuses to start when an on-chain active spork is not implemented
// by this binary (binary terminates with exit code 2 to make the
// upgrade requirement obvious).
func (c *chain) Init() error {
	c.log.Info("initializing ...")
	defer c.log.Info("initialized")

	c.log.Info("starting chain module with db", "location", c.chainManager.Location(), "frontier-identifier", c.GetFrontierMomentumStore().Identifier())

	// check if the configured genesis matches the existent chain
	if err := c.checkGenesisCompatibility(); err != nil {
		return err
	}
	types.SporkAddress = c.genesis.GetSporkAddress()
	c.Register(c.accountPool)

	frontierStore := c.GetFrontierMomentumStore()
	frontier, err := frontierStore.GetFrontierMomentum()
	if err != nil {
		return err
	}
	fmt.Printf("Initialized NoM. Height: %v, Hash: %v\n", frontier.Height, frontier.Hash)
	c.log.Info("initialized nom", "identifier", frontier.Identifier())

	if _, unimplemented, err := GotAllActiveSporksImplemented(frontierStore); err != nil {
		return err
	} else if unimplemented != nil {
		c.log.Crit("can't start node because don't have all sporks implemented",
			"hash", frontier.Hash, "height", frontier.Height, "unimplemented", unimplemented)

		fmt.Printf("===== Error =====\n")
		fmt.Printf("Can't start node. %v height %v\n", frontier.Hash, frontier.Height)
		fmt.Printf("Detected an unimplemented spork.\n")
		for _, spork := range unimplemented {
			fmt.Printf("  Spork name `%v` id:`%v`\n", spork.Name, spork.Id)
		}
		fmt.Printf("\n")
		fmt.Printf("Please upgrade your znnd binary\n")
		fmt.Printf("znnd is terminating\n")
		os.Exit(2)
	}

	return nil
}

// Start is currently a no-op; the chain has no background loops of its
// own. Reserved for future use.
func (c *chain) Start() error {
	c.log.Info("starting ...")
	defer c.log.Info("started")

	return nil
}

// Stop unregisters the account-pool listener and closes the underlying
// database manager. Safe to call exactly once.
func (c *chain) Stop() error {
	c.log.Info("stopping ...")
	defer c.log.Info("stopped")

	c.UnRegister(c.accountPool)

	return c.chainManager.Stop()
}

// checkGenesisCompatibility reconciles the embedded genesis with the
// on-disk chain. If the chain is empty, the genesis transaction is
// inserted. If the chain has a height-1 momentum, its hash must match
// the embedded genesis — otherwise the node refuses to start. The
// remediation message points operators at "remove the database
// manually", which is the only way to recover from a genesis swap.
func (c *chain) checkGenesisCompatibility() error {
	frontierStore := c.GetFrontierMomentumStore()
	if frontierStore.Identifier().IsZero() {
		insert := c.AcquireInsert("add genesis momentum")
		defer insert.Unlock()
		c.log.Info("did not find any blocks. Inserting genesis block")
		// chain is empty, apply genesis
		if err := c.momentumPool.AddMomentumTransaction(insert, c.GetGenesisTransaction()); err != nil {
			return err
		}
	} else {
		genesisMomentum, err := frontierStore.GetMomentumByHeight(1)
		if err != nil {
			return err
		}
		if genesisMomentum.Hash != c.GetGenesisMomentum().Hash {
			return errors.Errorf("The genesis state is incorrect. " +
				"You can fix the problem by removing the database manually.")
		}
		c.log.Info("found momentums in DB. genesis-hash matches")
	}
	return nil
}

// AcquireInsert blocks until the global insert lock is available, then
// returns an [inserter] sync.Locker tagged with reason for log lines.
// The returned locker is single-use: calling Lock() on it panics, only
// Unlock() is valid. Mutation methods on [Chain] check that the locker
// is non-nil but do not validate its identity — passing a different
// sync.Locker is a programmer error.
func (c *chain) AcquireInsert(reason string) sync.Locker {
	inserterLog.Debug("waiting", "reason", reason)
	c.insert.Lock()

	inserterLog.Debug("acquired", "reason", reason)
	return &inserter{
		reason: reason,
		mutex:  &c.insert,
	}
}

// inserter is the [sync.Locker] returned by [chain.AcquireInsert]. It
// wraps the chain's insert mutex with a reason string used for logging
// at acquire/release boundaries.
type inserter struct {
	reason string
	mutex  *sync.Mutex
}

// Lock panics — an inserter handle has already locked the mutex; the
// only valid operation is Unlock. The panic catches programmer errors
// where a caller treats the handle as a fresh locker.
func (i *inserter) Lock() {
	panic("can't lock an insert - already locked")
}

// Unlock releases the underlying mutex and clears the handle so a
// subsequent Unlock would dereference nil and panic loudly. One-shot
// by design.
func (i *inserter) Unlock() {
	inserterLog.Debug("released", "reason", i.reason)
	i.mutex.Unlock()
	i.mutex = nil
}
