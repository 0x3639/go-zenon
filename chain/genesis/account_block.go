package genesis

import (
	"math/big"
	"sync"

	"github.com/zenon-network/go-zenon/chain"
	"github.com/zenon-network/go-zenon/chain/nom"
	"github.com/zenon-network/go-zenon/common"
	"github.com/zenon-network/go-zenon/common/db"
	"github.com/zenon-network/go-zenon/common/types"
	"github.com/zenon-network/go-zenon/vm/embedded/definition"
	"github.com/zenon-network/go-zenon/vm/vm_context"
)

// mockStable is the placeholder [chain.StableChain] handed to the
// account pool during genesis assembly: it returns a fresh in-memory
// database for every account so the pool can stage initial state
// without a real chain backing it.
type mockStable struct {
}

// GetStableAccountDB returns a fresh in-memory [db.DB] for any address —
// genesis assembly never reads previous state, so an empty store is the
// correct answer.
func (m *mockStable) GetStableAccountDB(address types.Address) db.DB {
	return db.NewMemDB()
}

// newGenesisAccountBlocks materializes the genesis account-block set
// from cfg by running each embedded contract's seeding routine in
// order, then layering on every additional balance block. Returns the
// populated [chain.AccountPool] that
// [github.com/zenon-network/go-zenon/chain/genesis.newGenesisMomentum]
// then snapshots into the genesis momentum.
//
// Order matters: embedded contracts (Pillar, Token, Plasma, Swap, and
// optionally Spork) are seeded first so they appear at known heights
// in their account chains; arbitrary genesis balance blocks follow.
func newGenesisAccountBlocks(cfg *GenesisConfig) chain.AccountPool {
	pool := chain.NewAccountPool(new(mockStable))

	changes := new(sync.Mutex)
	pool.AddAccountBlockTransaction(changes, genesisPillarContractConfig(cfg))
	pool.AddAccountBlockTransaction(changes, genesisTokenContractConfig(cfg))
	pool.AddAccountBlockTransaction(changes, genesisPlasmaContractConfig(cfg))
	pool.AddAccountBlockTransaction(changes, genesisSwapContractConfig(cfg))

	alreadySet := map[types.Address]interface{}{
		types.PillarContract: struct{}{},
		types.TokenContract:  struct{}{},
		types.PlasmaContract: struct{}{},
		types.SwapContract:   struct{}{},
	}

	if cfg.SporkConfig != nil {
		pool.AddAccountBlockTransaction(changes, genesisSporkContractConfig(cfg))
		alreadySet[types.SporkContract] = struct{}{}
	}

	list := genesisBalanceBlocksConfig(cfg, alreadySet)
	for _, el := range list {
		pool.AddAccountBlockTransaction(changes, el)
	}
	return pool
}

// wrap turns an in-progress [vm_context.AccountVmContext] (already
// populated with the address's seeded storage) into a complete
// [nom.AccountBlockTransaction] of type [nom.BlockTypeGenesisReceive]:
// applies any matching genesis balances, computes the changes hash and
// the canonical block hash, and pairs the block with its patch.
func wrap(cfg *GenesisConfig, context vm_context.AccountVmContext) *nom.AccountBlockTransaction {
	address := *context.Address()
	block := &nom.AccountBlock{
		Version:         1,
		ChainIdentifier: cfg.ChainIdentifier,
		BlockType:       nom.BlockTypeGenesisReceive,
		Height:          1,
		Address:         address,
	}

	for _, block := range cfg.GenesisBlocks.Blocks {
		if block.Address != address {
			continue
		}

		for zts, balance := range block.BalanceList {
			common.DealWithErr(context.SetBalance(zts, balance))
		}
	}

	changes, err := context.Changes()
	common.DealWithErr(err)
	block.ChangesHash = db.PatchHash(changes)
	block.Hash = block.ComputeHash()

	return &nom.AccountBlockTransaction{
		Block:   block,
		Changes: changes,
	}
}

// newContext returns a fresh genesis-time
// [vm_context.AccountVmContext] for address, plus a handle on its
// underlying storage view.
func newContext(address types.Address) (vm_context.AccountVmContext, db.DB) {
	context := vm_context.NewGenesisAccountContext(address)
	contextStorage := context.Storage()
	return context, contextStorage
}

// genesisPillarContractConfig seeds the pillar embedded contract: every
// configured pillar's registration record (and producing-key index),
// every delegation record, and every legacy pillar entry are written
// into the contract's storage namespace.
func genesisPillarContractConfig(cfg *GenesisConfig) *nom.AccountBlockTransaction {
	config := cfg.PillarConfig
	context, contextStorage := newContext(types.PillarContract)

	for _, pillar := range config.Pillars {
		common.DealWithErr(pillar.Save(contextStorage))
		common.DealWithErr((&definition.ProducingPillar{
			Name:      pillar.Name,
			Producing: &pillar.BlockProducingAddress,
		}).Save(contextStorage))
	}
	for _, delegation := range config.Delegations {
		common.DealWithErr(delegation.Save(contextStorage))
	}
	for _, legacyEntry := range config.LegacyEntries {
		common.DealWithErr(legacyEntry.Save(contextStorage))
	}

	return wrap(cfg, context)
}

// genesisTokenContractConfig seeds the token embedded contract: every
// configured token's issuance record is written into the contract's
// storage namespace.
func genesisTokenContractConfig(cfg *GenesisConfig) *nom.AccountBlockTransaction {
	config := cfg.TokenConfig
	context, contextStorage := newContext(types.TokenContract)

	for _, token := range config.Tokens {
		common.DealWithErr(token.Save(contextStorage))
	}

	return wrap(cfg, context)
}

// genesisPlasmaContractConfig seeds the plasma embedded contract: every
// configured fusion entry is saved, and the per-beneficiary cumulative
// fused-amount index is rebuilt from those entries.
func genesisPlasmaContractConfig(cfg *GenesisConfig) *nom.AccountBlockTransaction {
	config := cfg.PlasmaConfig
	context, contextStorage := newContext(types.PlasmaContract)

	fusedAmount := make(map[types.Address]*big.Int)
	for _, entry := range config.Fusions {
		common.DealWithErr(entry.Save(contextStorage))

		amount, ok := fusedAmount[entry.Beneficiary]
		if ok {
			amount.Add(amount, entry.Amount)
		} else {
			fusedAmount[entry.Beneficiary] = new(big.Int).Set(entry.Amount)
		}
	}
	for addr, amount := range fusedAmount {
		common.DealWithErr((&definition.FusedAmount{
			Beneficiary: addr,
			Amount:      amount,
		}).Save(contextStorage))
	}

	return wrap(cfg, context)
}

// genesisSwapContractConfig seeds the swap embedded contract with the
// configured legacy-chain redemption entries.
func genesisSwapContractConfig(cfg *GenesisConfig) *nom.AccountBlockTransaction {
	config := cfg.SwapConfig
	context, contextStorage := newContext(types.SwapContract)

	for _, entry := range config.Entries {
		common.DealWithErr(entry.Save(contextStorage))
	}

	return wrap(cfg, context)
}

// genesisSporkContractConfig seeds the spork embedded contract with the
// configured initial spork records (when the genesis defines any).
func genesisSporkContractConfig(cfg *GenesisConfig) *nom.AccountBlockTransaction {
	config := cfg.SporkConfig
	context, contextStorage := newContext(types.SporkContract)

	for _, entry := range config.Sporks {
		entry.Save(contextStorage)
	}

	return wrap(cfg, context)
}

// genesisBalanceBlocksConfig returns one genesis-receive block per
// non-embedded address in [GenesisConfig.GenesisBlocks]. Embedded
// addresses listed in alreadySet are skipped because their seeding has
// already happened via the per-contract helpers above.
func genesisBalanceBlocksConfig(cfg *GenesisConfig, alreadySet map[types.Address]interface{}) []*nom.AccountBlockTransaction {
	list := make([]*nom.AccountBlockTransaction, 0, len(cfg.GenesisBlocks.Blocks))
	for _, genesisBlock := range cfg.GenesisBlocks.Blocks {
		if _, ok := alreadySet[genesisBlock.Address]; ok {
			continue
		}

		context, _ := newContext(genesisBlock.Address)
		list = append(list, wrap(cfg, context))
	}

	return list
}
