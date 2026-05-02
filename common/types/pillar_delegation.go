package types

import (
	"fmt"
	"math/big"
)

// PillarDelegation is the consensus-layer summary of a registered pillar:
// its display name, its producing-key address (coinbase), and the total
// weight currently delegated to it. Weight is the sum of stake-weighted
// delegations from voting accounts.
type PillarDelegation struct {
	Name      string
	Producing Address
	Weight    *big.Int
}

// PillarDelegationDetail extends [PillarDelegation] with a per-backer
// breakdown. Used by snapshot/election code that needs to credit backers
// individually; consumers that only care about totals should call
// [ToPillarDelegation] to drop the backer map and save memory.
type PillarDelegationDetail struct {
	PillarDelegation
	Backers map[Address]*big.Int
}

// String renders v in `name@weight` form for log output.
func (v *PillarDelegation) String() string {
	return fmt.Sprintf("%v@%v", v.Name, v.Weight)
}

// Merge folds oth into pdd in place: weights are added, and per-backer
// amounts accumulate. Used by the election code to combine partial
// delegation snapshots across momentum windows.
func (pdd *PillarDelegationDetail) Merge(oth *PillarDelegationDetail) {
	pdd.Weight.Add(pdd.Weight, oth.Weight)
	for addr, amount := range oth.Backers {
		cAmount, ok := pdd.Backers[addr]
		if !ok {
			pdd.Backers[addr] = new(big.Int).Set(amount)
		} else {
			cAmount.Add(cAmount, amount)
		}
	}
}

// Reduce divides every weight (the aggregate and per-backer) by count,
// producing the average across that many samples.
func (pdd *PillarDelegationDetail) Reduce(count int64) {
	countBig := big.NewInt(count)
	pdd.Weight.Quo(pdd.Weight, countBig)
	for _, amount := range pdd.Backers {
		amount.Quo(amount, countBig)
	}
}

// SortPDDByWeight sorts a slice of [PillarDelegationDetail] by descending
// weight, breaking ties alphabetically by name. Implements [sort.Interface].
type SortPDDByWeight []*PillarDelegationDetail

// Len returns the number of pillars in a.
func (a SortPDDByWeight) Len() int { return len(a) }

// Swap exchanges entries i and j in a.
func (a SortPDDByWeight) Swap(i, j int) { a[i], a[j] = a[j], a[i] }

// Less reports whether entry i sorts before entry j: heavier weight first,
// alphabetical name as the tiebreaker.
func (a SortPDDByWeight) Less(i, j int) bool {
	r := a[j].Weight.Cmp(a[i].Weight)
	if r == 0 {
		return a[i].Name < a[j].Name
	} else {
		return r < 0
	}
}

// SortPDByWeight sorts a slice of [PillarDelegation] by descending weight,
// breaking ties alphabetically by name. Implements [sort.Interface].
type SortPDByWeight []*PillarDelegation

// Len returns the number of pillars in a.
func (a SortPDByWeight) Len() int { return len(a) }

// Swap exchanges entries i and j in a.
func (a SortPDByWeight) Swap(i, j int) { a[i], a[j] = a[j], a[i] }

// Less reports whether entry i sorts before entry j: heavier weight first,
// alphabetical name as the tiebreaker.
func (a SortPDByWeight) Less(i, j int) bool {
	r := a[j].Weight.Cmp(a[i].Weight)
	if r == 0 {
		return a[i].Name < a[j].Name
	} else {
		return r < 0
	}
}

// ToPillarDelegation projects a slice of detailed delegations down to plain
// [PillarDelegation]s, dropping the per-backer maps. Use this whenever the
// caller only needs aggregate weights — it cuts memory significantly when
// the network has many backers.
func ToPillarDelegation(details []*PillarDelegationDetail) []*PillarDelegation {
	result := make([]*PillarDelegation, len(details))
	for i, detail := range details {
		result[i] = &PillarDelegation{
			Name:      detail.Name,
			Producing: detail.Producing,
			Weight:    new(big.Int).Set(detail.Weight),
		}
	}
	return result
}
