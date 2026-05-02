package genesis

import (
	"math/big"

	"github.com/pkg/errors"

	"github.com/zenon-network/go-zenon/common"
	"github.com/zenon-network/go-zenon/common/types"
)

// checkAccountBalance verifies that the genesis-receive block for addr
// declares exactly the balances in required: every token in required is
// present in the matching genesis block, every token in the block is
// listed in required, and the amounts agree.
func checkAccountBalance(g *GenesisConfig, addr types.Address, required map[types.ZenonTokenStandard]*big.Int) error {
	// Check account balance for enough qsr
	for _, block := range g.GenesisBlocks.Blocks {
		if block.Address != addr {
			continue
		}

		for zts, amount := range block.BalanceList {
			requiredAmount, ok := required[zts]
			if !ok {
				return errors.Errorf("invalid balance for %v Extra token %v", addr, zts)
			} else {
				if requiredAmount.Cmp(amount) != 0 {
					return errors.Errorf("invalid balance for %v Expected %v %v but got %v", addr, requiredAmount, zts, amount)
				}
			}
		}

		for token := range required {
			_, ok := block.BalanceList[token]
			if !ok && required[token].Cmp(common.Big0) != 0 {
				return errors.Errorf("invalid balance for %v Expected token %v to be present", addr, token)
			}
		}
	}

	return nil
}

// CheckGenesis runs every consistency check on g in order:
// required-fields presence, plasma fusion totals against the plasma
// contract balance, swap entry well-formedness, pillar deposit totals
// against the pillar contract balance, and per-token total-supply
// reconciliation. Returns the first failure encountered.
func CheckGenesis(g *GenesisConfig) error {
	if err := CheckFieldsExist(g); err != nil {
		return err
	}
	if err := CheckPlasmaInfo(g); err != nil {
		return err
	}
	if err := CheckSwapAccount(g); err != nil {
		return err
	}
	if err := CheckPillarBalance(g); err != nil {
		return err
	}
	if err := CheckTokenTotalSupply(g); err != nil {
		return err
	}
	return nil
}

// CheckFieldsExist verifies that every required top-level field of g is
// present.
func CheckFieldsExist(g *GenesisConfig) error {
	if g.GenesisBlocks == nil {
		return errors.Errorf("GenesisBlocks is nil")
	}
	if g.TokenConfig == nil {
		return errors.Errorf("TokenConfig is nil")
	}
	if g.PillarConfig == nil {
		return errors.Errorf("PillarConfig is nil")
	}
	if g.SporkAddress == nil {
		return errors.Errorf("SporkAddress is nil")
	}
	if g.PlasmaConfig == nil {
		return errors.Errorf("PlasmaConfig is nil")
	}
	if g.SwapConfig == nil {
		return errors.Errorf("SwapConfig is nil")
	}
	return nil
}

// CheckPlasmaInfo verifies the QSR locked into the plasma contract by
// the genesis-receive block matches the sum of every fusion entry's
// amount.
func CheckPlasmaInfo(g *GenesisConfig) error {
	totalAmount := big.NewInt(0)

	for addr, fusion := range g.PlasmaConfig.Fusions {
		if fusion == nil {
			return errors.Errorf("nil FusionInfo for %v", addr)
		}
		totalAmount.Add(totalAmount, fusion.Amount)
	}

	return checkAccountBalance(g, types.PlasmaContract, map[types.ZenonTokenStandard]*big.Int{
		types.QsrTokenStandard: totalAmount,
	})
}

// CheckSwapAccount verifies the swap contract's genesis-receive
// declares zero ZNN and zero QSR (legacy-chain claims pull from the
// contract's mint authority, not from a pre-funded balance) and that
// every entry has both ZNN and QSR amounts populated.
func CheckSwapAccount(g *GenesisConfig) error {
	given := map[types.ZenonTokenStandard]*big.Int{
		types.ZnnTokenStandard: big.NewInt(0),
		types.QsrTokenStandard: big.NewInt(0),
	}

	for _, entry := range g.SwapConfig.Entries {
		if entry.Qsr == nil || entry.Znn == nil {
			return errors.Errorf("invalid swap balance for KeyIdHash %v", entry.KeyIdHash)
		}
	}

	return checkAccountBalance(g, types.SwapContract, given)
}

// CheckPillarBalance verifies the ZNN deposited into the pillar
// contract by the genesis-receive matches the sum of every registered
// pillar's stake amount.
func CheckPillarBalance(g *GenesisConfig) error {
	totalAmount := big.NewInt(0)

	for _, el := range g.PillarConfig.Pillars {
		totalAmount.Add(totalAmount, el.Amount)
	}

	return checkAccountBalance(g, types.PillarContract, map[types.ZenonTokenStandard]*big.Int{
		types.ZnnTokenStandard: totalAmount,
	})
}

// CheckTokenTotalSupply verifies that for every token declared in
// [TokenContractConfig], the sum of balances assigned to that token
// across every genesis-receive equals the declared total supply, and
// that no token is given out without being declared.
func CheckTokenTotalSupply(g *GenesisConfig) error {
	given := make(map[types.ZenonTokenStandard]*big.Int)
	for _, block := range g.GenesisBlocks.Blocks {
		for zts, amount := range block.BalanceList {
			total, ok := given[zts]
			if !ok {
				given[zts] = new(big.Int).Set(amount)
			} else {
				total.Add(total, amount)
			}
		}
	}

	for _, token := range g.TokenConfig.Tokens {
		total, ok := given[token.TokenStandard]
		if !ok {
			return errors.Errorf("token %v declared but not given at all", token)
		} else if token.TotalSupply.Cmp(total) != 0 {
			return errors.Errorf("invalid token total balance for %v Expected %v but got %v", token, total, token.TotalSupply)
		}
	}

	for zts := range given {
		found := false
		for _, token := range g.TokenConfig.Tokens {
			if token.TokenStandard == zts {
				found = true
				break
			}
		}

		if !found {
			return errors.Errorf("invalid token %v given but not declared", zts)
		}
	}
	return nil
}

// CheckGenesisCheckSum ensures that the hash of the account blocks
// don't change during the build: rebuilds the genesis from g and
// compares the resulting genesis-momentum hash to expected. Used by
// release tests to verify that incidental refactors do not change the
// committed alphanet state.
func CheckGenesisCheckSum(g *GenesisConfig, expected types.Hash) error {
	genesis := NewGenesis(g)
	checkSum := genesis.GetGenesisMomentum().Hash
	if checkSum != expected {
		return errors.Errorf("invalid genesis-momentum hash. Expected %v but got %v", expected, checkSum)
	}
	return nil
}
