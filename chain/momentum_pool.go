package chain

import (
	"fmt"
	"os"
	"sync"

	"github.com/inconshreveable/log15"
	"github.com/pkg/errors"

	"github.com/zenon-network/go-zenon/chain/momentum"
	"github.com/zenon-network/go-zenon/chain/nom"
	"github.com/zenon-network/go-zenon/chain/store"
	"github.com/zenon-network/go-zenon/common"
	"github.com/zenon-network/go-zenon/common/db"
	"github.com/zenon-network/go-zenon/common/types"
	"github.com/zenon-network/go-zenon/vm/embedded/definition"
)

// momentumPool is the persistent half of the chain: it owns the
// versioned [db.Manager] backing the momentum chain and emits
// momentum-event broadcasts on insert and rollback.
//
// Concurrency: every public method takes c.changes; the pool is safe
// for concurrent use. Broadcasts run with the mutex temporarily released
// so listeners cannot deadlock against a re-entry into the pool.
type momentumPool struct {
	*momentumEventManager
	chainManager db.Manager
	genesis      store.Genesis
	log          log15.Logger
	changes      sync.Mutex
}

// AddMomentumTransaction commits transaction onto the chain frontier,
// broadcasts InsertMomentum to listeners, and inspects the resulting
// state for two spork-related conditions:
//
//   - Activation: a freshly activated spork prints a congratulations
//     banner.
//   - Unimplemented: an active on-chain spork this binary does not
//     recognize is fatal — the node logs at Crit level and terminates
//     with exit code 2.
//
// Caller must hold the chain insert lock (the insertLocker argument).
func (c *momentumPool) AddMomentumTransaction(insertLocker sync.Locker, transaction *nom.MomentumTransaction) error {
	c.log.Info("inserting new momentum", "identifier", transaction.Momentum.Identifier())
	if insertLocker == nil {
		return errors.Errorf("insertLocker can't be nil")
	}
	c.changes.Lock()
	defer c.changes.Unlock()

	momentum := transaction.Momentum

	if err := c.chainManager.Add(transaction); err != nil {
		return err
	}

	store := c.getFrontierStore()
	detailed, err := store.PrefetchMomentum(momentum)
	if err != nil {
		return err
	}

	c.changes.Unlock()
	c.broadcastInsertMomentum(detailed)
	c.changes.Lock()

	frontier := c.getFrontierStore()
	if justNow, unimplemented, err := GotAllActiveSporksImplemented(frontier); err != nil {
		return err
	} else if unimplemented != nil {
		c.log.Crit("can't insert momentum because don't have all sporks implemented",
			"hash", momentum.Hash, "height", momentum.Height, "unimplemented", unimplemented)

		fmt.Printf("===== Error =====\n")
		fmt.Printf("Can't insert momentum %v height %v\n", momentum.Hash, momentum.Height)
		fmt.Printf("Detected an unimplemented spork.\n")
		for _, spork := range unimplemented {
			fmt.Printf("  Spork name `%v`\n", spork.Name)
		}
		fmt.Printf("\n")
		fmt.Printf("Please upgrade your znnd binary\n")
		fmt.Printf("znnd is terminating\n")
		os.Exit(2)
	} else if justNow != nil {
		fmt.Printf("\n")
		fmt.Printf("===== Congratulations! =====\n")
		fmt.Printf("Just activated spork '%v'\n", justNow.Name)
		fmt.Printf("\n")
	}

	return nil
}

// RollbackTo reverses every momentum down to identifier (inclusive),
// broadcasting DeleteMomentum for each one removed. Refuses if the
// chain at identifier.Height does not match identifier.Hash.
//
// Caller must hold the chain insert lock (the insertLocker argument).
func (c *momentumPool) RollbackTo(insertLocker sync.Locker, identifier types.HashHeight) error {
	c.log.Info("rollbacking momentums", "to-identifier", identifier)
	if insertLocker == nil {
		return errors.Errorf("insertLocker can't be nil")
	}
	c.changes.Lock()
	defer c.changes.Unlock()
	c.log.Info("preparing to rollback momentums", "identifier", identifier)
	store := c.getFrontierStore()
	momentum, err := store.GetMomentumByHeight(identifier.Height)
	if err != nil {
		return err
	}
	if momentum.Hash != identifier.Hash {
		return errors.Errorf("can't rollback momentums. Expected %v but got %v instead", momentum.Identifier(), identifier)
	}

	for {
		store := c.getFrontierStore()
		frontier, err := store.GetFrontierMomentum()
		if err != nil {
			return err
		}

		if frontier.Height == identifier.Height {
			break
		}
		c.log.Info("rollbacking", "momentum-identifier", frontier.Identifier())
		detailed, err := store.PrefetchMomentum(frontier)
		if err != nil {
			return err
		}
		if err := c.chainManager.Pop(); err != nil {
			return err
		}

		c.changes.Unlock()
		c.broadcastDeleteMomentum(detailed)
		c.changes.Lock()
	}

	return nil
}

// GotAllActiveSporksImplemented inspects the spork records in store and
// reports two things: the spork (if any) whose enforcement height
// matches the current frontier (i.e., just activated this momentum),
// and any active spork this binary does not recognize.
//
// Used by [chain.Init] (boot-time check) and
// [momentumPool.AddMomentumTransaction] (per-momentum check) to refuse
// to operate against an upgraded chain that needs a newer binary.
func GotAllActiveSporksImplemented(store store.Momentum) (justNow *definition.Spork, unimplemented []*definition.Spork, err error) {
	momentum, err := store.GetFrontierMomentum()
	if err != nil {
		return nil, nil, err
	}

	// Query previous momentum for DB, since this function can be called from verifier when inserting a new momentum
	sporks, err := store.GetAllDefinedSporks()
	if err != nil {
		return nil, nil, err
	}

	for _, spork := range sporks {
		if spork.Activated && spork.EnforcementHeight <= momentum.Height {
			_, ok := types.ImplementedSporksMap[spork.Id]
			if !ok {
				unimplemented = append(unimplemented, spork)
			}
		}
		if spork.Activated && spork.EnforcementHeight == momentum.Height {
			justNow = spork
		}
	}

	if len(unimplemented) == 0 {
		return justNow, nil, nil
	}

	return justNow, unimplemented, nil
}

// getFrontierStore returns a [store.Momentum] view of the chain
// manager's current frontier, or nil when the manager is shutting down.
// Caller must hold c.changes.
func (c *momentumPool) getFrontierStore() store.Momentum {
	if momentumDB := c.chainManager.Frontier(); momentumDB == nil {
		return nil
	} else {
		return momentum.NewStore(c.genesis, momentumDB)
	}
}

// GetFrontierMomentumStore returns the locked variant of
// [getFrontierStore].
func (c *momentumPool) GetFrontierMomentumStore() store.Momentum {
	c.changes.Lock()
	defer c.changes.Unlock()
	return c.getFrontierStore()
}

// GetMomentumStore returns a [store.Momentum] view at identifier, or
// nil when the identifier is unknown or has been pruned.
func (c *momentumPool) GetMomentumStore(identifier types.HashHeight) store.Momentum {
	c.changes.Lock()
	defer c.changes.Unlock()
	momentumDB := c.chainManager.Get(identifier)
	if momentumDB == nil {
		return nil
	}

	return momentum.NewStore(c.genesis, momentumDB)
}

// GetStableAccountDB satisfies the [Stable] interface for the account
// pool: returns the persistent [db.DB] view of address at the most
// recent committed momentum.
func (c *momentumPool) GetStableAccountDB(address types.Address) db.DB {
	c.changes.Lock()
	defer c.changes.Unlock()
	return c.getFrontierStore().GetAccountDB(address)
}

// NewMomentumPool wires a fresh [momentumPool] around chainManager and
// the supplied genesis. Used by [NewChain]; external callers should go
// through [NewChain] rather than constructing pools directly.
func NewMomentumPool(chainManager db.Manager, genesis store.Genesis) *momentumPool {
	return &momentumPool{
		momentumEventManager: newMomentumEventManager(),
		chainManager:         chainManager,
		genesis:              genesis,
		log:                  common.ChainLogger.New("submodule", "momentum-pool"),
	}
}
