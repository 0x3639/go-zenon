package protocol

import (
	"fmt"

	"github.com/pkg/errors"

	"github.com/zenon-network/go-zenon/chain"
	"github.com/zenon-network/go-zenon/chain/nom"
	"github.com/zenon-network/go-zenon/common"
	"github.com/zenon-network/go-zenon/common/types"
	"github.com/zenon-network/go-zenon/consensus"
	"github.com/zenon-network/go-zenon/verifier"
	"github.com/zenon-network/go-zenon/vm"
)

// chainBridge is the [ChainBridge] implementation: it adapts the
// chain / consensus / verifier / VM-supervisor stack to the
// peer-facing API the protocol manager uses.
//
// On the read side it exposes momentum / account-block lookups,
// the genesis identifier, and the (height, hash, genesis hash)
// status triple used by the protocol handshake. On the write
// side, [chainBridge.AddAccountBlocks] applies single account
// blocks via the supervisor, while [chainBridge.InsertChain]
// commits a contiguous batch of [nom.DetailedMomentum]s — used by
// the bulk-sync downloader.
type chainBridge struct {
	chain      chain.Chain
	consensus  consensus.Consensus
	verifier   verifier.Verifier
	supervisor *vm.Supervisor
}

// NewChainBridge wires a [ChainBridge] over the supplied chain,
// consensus, verifier, and VM supervisor handles.
func NewChainBridge(chain chain.Chain, consensus consensus.Consensus, verifier verifier.Verifier, supervisor *vm.Supervisor) ChainBridge {
	return chainBridge{
		chain:      chain,
		consensus:  consensus,
		verifier:   verifier,
		supervisor: supervisor,
	}
}

// AddAccountBlocks runs each block through the VM supervisor and
// commits it under the chain insert lock. Skips blocks whose
// patch is already known and ignores [nom.BlockTypeContractSend]
// (those are nested inside their parent receive).
func (c chainBridge) AddAccountBlocks(blocks []*nom.AccountBlock) error {
	insert := c.chain.AcquireInsert(fmt.Sprintf("Insert blocks in chain-bridge. Len:%v", len(blocks)))
	defer insert.Unlock()
	for _, block := range blocks {
		if patch := c.chain.GetPatch(block.Address, block.Identifier()); patch != nil {
			continue
		}
		if block.BlockType == nom.BlockTypeContractSend {
			continue
		}
		transaction, err := c.supervisor.ApplyBlock(block)
		if err != nil {
			log.Error("error while applying account-block", "reason", err, "account-block-header", block.Header())
			return err
		}

		if err := c.chain.AddAccountBlockTransaction(insert, transaction); err != nil {
			log.Error("error while inserting account-block in pool", "reason", err, "account-block-header", block.Header())
			return err
		}
	}
	return nil
}

// GetTransactions loads the Transactions record from storage.
func (c chainBridge) GetTransactions() []*nom.AccountBlock {
	blocks := c.chain.GetAllUncommittedAccountBlocks()
	return blocks
}

// HasBlock is part of the receiver's public API.
func (c chainBridge) HasBlock(hash types.Hash) bool {
	m, _ := c.chain.GetFrontierMomentumStore().GetMomentumByHash(hash)
	return m != nil
}

// GetBlockHashesFromHash loads the BlockHashesFromHash record from storage.
func (c chainBridge) GetBlockHashesFromHash(hash types.Hash, amount uint64) ([]types.Hash, error) {
	momentums, err := c.chain.GetFrontierMomentumStore().GetMomentumsByHash(hash, false, amount)
	if err != nil {
		return nil, err
	}
	hashes := make([]types.Hash, len(momentums))
	for i := range momentums {
		hashes[i] = momentums[i].Hash
	}
	return hashes, nil
}

// GetBlock loads the Block record from storage.
func (c chainBridge) GetBlock(hash types.Hash) *nom.DetailedMomentum {
	store := c.chain.GetFrontierMomentumStore()
	momentum, _ := store.GetMomentumByHash(hash)
	if momentum == nil {
		return nil
	}
	prefetched := make([]*nom.AccountBlock, len(momentum.Content))

	for i := range prefetched {
		block, _ := store.GetAccountBlock(*momentum.Content[i])
		prefetched[i] = block
	}

	return &nom.DetailedMomentum{
		Momentum:      momentum,
		AccountBlocks: prefetched,
	}
}

// CurrentBlock is part of the receiver's public API.
func (c chainBridge) CurrentBlock() *nom.Momentum {
	store := c.chain.GetFrontierMomentumStore()
	momentum, err := store.GetFrontierMomentum()
	common.DealWithErr(err)

	return momentum
}

// GetBlockByNumber loads the BlockByNumber record from storage.
func (c chainBridge) GetBlockByNumber(num uint64) (*nom.Momentum, error) {
	store := c.chain.GetFrontierMomentumStore()
	return store.GetMomentumByHeight(num)
}

// Status is part of the receiver's public API.
func (c chainBridge) Status() (td uint64, currentBlock types.Hash, genesisBlock types.Hash) {
	store := c.chain.GetFrontierMomentumStore()
	frontier, err := store.GetFrontierMomentum()
	common.DealWithErr(err)

	return frontier.Height, frontier.Hash, c.chain.GetGenesisMomentum().Hash
}

// InsertChain commits a contiguous batch of [nom.DetailedMomentum]
// entries under a single insert lock. Steps:
//
//  1. Strip already-inserted momentums from the head.
//  2. If the new chain branches off an older ancestor (side-chain
//     reorg), check the rollback distance is bounded (≤ 30
//     momentums) and that the new chain is strictly longer; then
//     roll the local chain back to the branch point.
//  3. For each momentum, apply every account block via the
//     supervisor (force-add since the chain content is already
//     committed by the producer), then commit the momentum.
//
// Returns the number of momentums successfully inserted before
// any error.
func (c chainBridge) InsertChain(momentums []*nom.DetailedMomentum) (int, error) {
	a := momentums[0]
	b := momentums[len(momentums)-1]
	log.Info("start inserting chain", "num-momentums", len(momentums), "start-identifier", a.Momentum.Identifier(), "end-identifier", b.Momentum.Identifier())
	insert := c.chain.AcquireInsert(fmt.Sprintf("Insert momentums in chain-bridge. Start-identifier:%v End-identifier:%v", a.Momentum.Identifier(), b.Momentum.Identifier()))
	defer insert.Unlock()

	store := c.chain.GetFrontierMomentumStore()

	// remove momentums which we already have
	start := 0
	for ; start < len(momentums); start += 1 {
		our, err := store.GetMomentumByHeight(momentums[start].Momentum.Height)
		if err != nil {
			log.Info("failed to get momentum by height", "reason", err)
			return start, err
		}
		if our == nil {
			break
		}

		if our.Hash != momentums[start].Momentum.Hash {
			break
		}
	}

	// nothing to add, all momentums are already inserted
	if start == len(momentums) {
		log.Info("nothing to insert. All momentums already inserted")
		return 0, nil
	}
	momentums = momentums[start:]

	head := momentums[0].Momentum
	tail := momentums[len(momentums)-1].Momentum
	ourFrontier, err := store.GetFrontierMomentum()
	if err != nil {
		return 0, err
	}

	// if we are dealing with a side-chain, check if it should replace our chain and rollback for insertion
	if head.Previous() != ourFrontier.Identifier() {
		// check if we can roll back for insertion
		target, err := store.GetMomentumByHeight(head.Height - 1)
		if err != nil {
			return 0, err
		}
		if target.Identifier() != head.Previous() {
			log.Error("can't link momentums to insert", "first")
			return 0, errors.Errorf("can't link momentums to insert. First momentum Prev is %v but he have %v", head.Previous(), target.Identifier())
		}

		// check that the distance allows rollback
		if ourFrontier.Height-target.Height > 30 {
			return 0, errors.Errorf("can't rollback to %v. Too far. Frontier is %v. Wanted to be able to insert %v", target.Identifier(), ourFrontier.Identifier(), head.Identifier())
		}

		// check that current tail is longer than frontier
		if tail.Height <= ourFrontier.Height {
			return 0, errors.Errorf("won't insert side-chain which is not longer")
		}

		err = c.chain.RollbackTo(insert, target.Identifier())
		if err != nil {
			return 0, errors.Errorf("unable to rollback to %v. Reason:%v", target.Identifier(), err)
		}
	}

	// Insert momentum now
	for index, detailed := range momentums {
		for _, block := range detailed.AccountBlocks {
			if block.BlockType == nom.BlockTypeContractSend {
				continue
			}
			if patch := c.chain.GetPatch(block.Address, block.Identifier()); patch != nil {
				// already applied
				continue
			}
			transaction, err := c.supervisor.ApplyBlock(block)
			if err != nil {
				log.Error("error while applying account-block", "reason", err, "account-block-header", block.Header())
				return index + start, err
			}
			if err := c.chain.ForceAddAccountBlockTransaction(insert, transaction); err != nil {
				log.Error("error while inserting account-block in pool", "reason", err, "account-block-header", block.Header())
				return index + start, err
			}
		}

		transaction, err := c.supervisor.ApplyMomentum(detailed)
		if err != nil {
			return index + start, err
		}
		if err := c.chain.AddMomentumTransaction(insert, transaction); err != nil {
			log.Error("error while inserting momentum", "reason", err, "momentum-identifier", detailed.Momentum.Identifier())
			return index + start, err
		}
	}

	return 0, nil
}
