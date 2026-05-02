package momentum

import (
	"fmt"
	"math/big"
	"sort"

	"github.com/zenon-network/go-zenon/chain/store"
	"github.com/zenon-network/go-zenon/common/types"
	"github.com/zenon-network/go-zenon/vm/embedded/definition"
)

// GetActivePillars returns every registered pillar that is still active
// at this view. Reads through the pillar embedded contract.
func (ms *momentumStore) GetActivePillars() ([]*definition.PillarInfo, error) {
	sd, err := ms.getEmbeddedStore(types.PillarContract)
	if err != nil {
		return nil, fmt.Errorf("getEmbeddedStore failed: %w", err)
	}

	return definition.GetPillarsList(sd.Storage(), true, definition.AnyPillarType)
}

// getAllDelegations returns every delegation record (backer → pillar)
// currently on record. Used as the input to [momentumStore.computeBackers].
func (ms *momentumStore) getAllDelegations() ([]*definition.DelegationInfo, error) {
	sd, err := ms.getEmbeddedStore(types.PillarContract)
	if err != nil {
		return nil, fmt.Errorf("getEmbeddedStore failed: %w", err)
	}

	return definition.GetDelegationsList(sd.Storage())
}

// computeBackers groups delegations by pillar name and resolves each
// backer's cached ZNN balance. Returns a `pillar-name → backer → balance`
// map suitable for [momentumStore.ComputePillarDelegations].
func (ms *momentumStore) computeBackers(infos []*definition.DelegationInfo) (*map[string]map[types.Address]*big.Int, error) {
	result := map[string]map[types.Address]*big.Int{}

	addresses := make([]types.Address, 0, len(infos))
	balanceMap := make(map[types.Address]*big.Int)
	for _, delegation := range infos {
		balance, err := ms.getZnnBalance(delegation.Backer)
		if err != nil {
			return nil, err
		}
		balanceMap[delegation.Backer] = balance
		addresses = append(addresses, delegation.Backer)
	}

	for _, delegation := range infos {
		balance, ok := balanceMap[delegation.Backer]
		if !ok {
			balance = big.NewInt(0)
		}

		delegators, ok := result[delegation.Name]
		if !ok {
			delegators = map[types.Address]*big.Int{}
		}

		delegators[delegation.Backer] = balance
		result[delegation.Name] = delegators
	}
	return &result, nil
}

// ComputePillarDelegations re-derives every pillar's aggregated
// delegation weight (and the per-backer breakdown) from the current
// stake and vote records. Sorted by descending weight per
// [types.SortPDDByWeight] so consumers see the most influential pillars
// first.
//
// Used by the consensus layer when constructing election snapshots.
func (ms *momentumStore) ComputePillarDelegations() ([]*types.PillarDelegationDetail, error) {
	delegations, _ := ms.getAllDelegations()
	backers, err := ms.computeBackers(delegations)
	if err != nil {
		return nil, err
	}

	// query register info
	registerList, _ := ms.GetActivePillars()
	pillarDelegationDetails := make([]*types.PillarDelegationDetail, 0, len(registerList))
	for _, registration := range registerList {
		pillarDelegationDetails = append(pillarDelegationDetails, &types.PillarDelegationDetail{
			PillarDelegation: types.PillarDelegation{
				Name:      registration.Name,
				Producing: registration.BlockProducingAddress,
				Weight:    big.NewInt(0),
			},
			Backers: make(map[types.Address]*big.Int, 0),
		})
	}

	for pillarName, delegators := range *backers {
		// Get registration
		var delegation *types.PillarDelegationDetail
		for _, r := range pillarDelegationDetails {
			if r.Name == pillarName {
				delegation = r
			}
		}

		if delegation == nil {
			continue
		}

		totalBalance := big.NewInt(0)
		for _, balance := range delegators {
			totalBalance.Add(totalBalance, balance)
		}

		delegation.Weight.Set(totalBalance)
		delegation.Backers = delegators
	}

	sort.Sort(types.SortPDDByWeight(pillarDelegationDetails))
	return pillarDelegationDetails, nil
}

// GetStakeBeneficialAmount returns the total stake amount addr is the
// beneficial owner of (the input to per-account staking rewards). Reads
// through the plasma embedded contract.
func (ms *momentumStore) GetStakeBeneficialAmount(addr types.Address) (*big.Int, error) {
	sd, err := ms.getEmbeddedStore(types.PlasmaContract)
	if err != nil {
		return nil, fmt.Errorf("getEmbeddedStore failed: %w", err)
	}

	fused, err := definition.GetFusedAmount(sd.Storage(), addr)
	if err != nil {
		return nil, err
	}
	return fused.Amount, nil
}

// GetTokenInfoByTs returns the issuance metadata for the token
// identified by ts. Reads through the token embedded contract.
func (ms *momentumStore) GetTokenInfoByTs(ts types.ZenonTokenStandard) (*definition.TokenInfo, error) {
	sd, err := ms.getEmbeddedStore(types.TokenContract)
	if err != nil {
		return nil, fmt.Errorf("getEmbeddedStore failed: %w", err)
	}

	return definition.GetTokenInfo(sd.Storage(), ts)
}

// GetAllDefinedSporks returns every spork record (active and pending)
// recorded by the spork embedded contract.
func (ms *momentumStore) GetAllDefinedSporks() ([]*definition.Spork, error) {
	sd, err := ms.getEmbeddedStore(types.SporkContract)
	if err != nil {
		return nil, fmt.Errorf("getEmbeddedStore failed: %w", err)
	}

	return definition.GetAllSporks(sd.Storage()), nil
}

// IsSporkActive reports whether the spork named by implemented has been
// activated at this view's frontier height. Always returns false when
// the chain is at genesis (height == 1) — spork activation can only
// happen at height ≥ 2.
func (ms *momentumStore) IsSporkActive(implemented *types.ImplementedSpork) (bool, error) {
	frontier, err := ms.GetFrontierMomentum()
	if err != nil {
		return false, err
	}
	if frontier.Height == 1 {
		return false, nil
	}

	sporks, err := ms.GetAllDefinedSporks()
	if err != nil {
		return false, err
	}

	for _, spork := range sporks {
		if spork.Activated && spork.EnforcementHeight <= frontier.Height && spork.Id == implemented.SporkId {
			return true, nil
		}
	}

	return false, nil
}

// getEmbeddedStore returns the [store.Account] view for an embedded
// contract address. Thin wrapper used to keep contract-storage reads
// uniform across the per-contract helpers above.
func (ms *momentumStore) getEmbeddedStore(address types.Address) (store.Account, error) {
	return ms.GetAccountStore(address), nil
}
