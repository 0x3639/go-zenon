package consensus

import (
	"math/rand"
	"sort"

	"github.com/zenon-network/go-zenon/common/types"
)

// AlgorithmConfig is the input snapshot the election algorithm runs
// against: the candidate delegations and the proof-block (hash, height)
// pair used to derive the random seed.
type AlgorithmConfig struct {
	delegations []*types.PillarDelegation
	hashH       *types.HashHeight
}

// NewAlgorithmContext builds an [AlgorithmConfig] for the supplied
// delegations and proof-block identifier.
func NewAlgorithmContext(delegations []*types.PillarDelegation, hashH *types.HashHeight) *AlgorithmConfig {
	return &AlgorithmConfig{
		delegations: delegations,
		hashH:       hashH,
	}
}

// ElectionAlgorithm is the strategy interface the election manager
// dispatches to. The default implementation
// ([electionAlgorithm.SelectProducers]) is weight-driven with a
// RandCount-based promotion rule and a deterministic shuffle.
type ElectionAlgorithm interface {
	// SelectProducers returns the ordered producer plan for one tick,
	// derived from context.delegations. The result has exactly
	// [Context.NodeCount] entries.
	SelectProducers(context *AlgorithmConfig) []*types.PillarDelegation
}

// electionAlgorithm is the canonical [ElectionAlgorithm] implementation.
type electionAlgorithm struct {
	group *Context
}

// NewElectionAlgorithm wires an [electionAlgorithm] over the consensus
// context. The context supplies NodeCount, RandCount, and the ticker
// used for time conversion.
func NewElectionAlgorithm(group *Context) *electionAlgorithm {
	return &electionAlgorithm{
		group: group,
	}
}

// findSeed produces the deterministic random seed used by every
// algorithm step for a given context. The seed depends solely on the
// proof-block height so all honest nodes derive the same shuffle.
//
// Generates a deterministic seed based on the context. Formula
// depends on seed, weights and momentumHeight.
func (ea *electionAlgorithm) findSeed(context *AlgorithmConfig) int64 {
	return int64(context.hashH.Height)
}

// SelectProducers runs the three-step algorithm: split candidates into
// the top-NodeCount group A and the leftovers group B
// ([electionAlgorithm.filterByWeight]); promote a configurable number
// (RandCount) from B in place of randomly-dropped A entries
// ([electionAlgorithm.filterRandom]); deterministically shuffle the
// resulting NodeCount selection ([electionAlgorithm.shuffleOrder]).
func (ea *electionAlgorithm) SelectProducers(context *AlgorithmConfig) []*types.PillarDelegation {
	// Split into groups based on weight
	groupA, groupB := ea.filterByWeight(context)

	producers := ea.filterRandom(groupA, groupB, context)
	producers = ea.shuffleOrder(producers, context)

	return producers
}

// shuffleOrder permutes producers using a seed derived from the
// proof-block height so that every node arrives at the same order.
//
// Shuffles the order in which momentums are produced, based on seed.
func (ea *electionAlgorithm) shuffleOrder(producers []*types.PillarDelegation, context *AlgorithmConfig) (result []*types.PillarDelegation) {
	random := rand.New(rand.NewSource(ea.findSeed(context)))
	perm := random.Perm(len(producers))

	for _, v := range perm {
		result = append(result, producers[v])
	}

	return result
}

// filterByWeight partitions context.delegations by stake weight: the
// top NodeCount entries become group A, the rest group B. When fewer
// than NodeCount candidates exist all of them are placed in group A
// and group B is empty.
//
// Splits into 2 groups.
func (ea *electionAlgorithm) filterByWeight(context *AlgorithmConfig) (groupA []*types.PillarDelegation, groupB []*types.PillarDelegation) {
	if len(context.delegations) <= int(ea.group.NodeCount) {
		return context.delegations, groupB
	}

	sort.Sort(types.SortPDByWeight(context.delegations))
	groupA = context.delegations[0:ea.group.NodeCount]
	groupB = context.delegations[ea.group.NodeCount:]

	return groupA, groupB
}

// filterRandom promotes pillars per the RandCount rule. When group A
// has fewer than NodeCount entries the round is filled with
// permutations of group A (so the slate stays full even if pillars are
// off-line). Otherwise, the top (NodeCount - RandCount) entries from
// group A are kept, and RandCount slots are awarded by uniform random
// pick from a pool of (group B + the displaced group-A entries) — so
// pillars below the top NodeCount-by-weight line still have a chance
// to produce.
//
// Applies RandCount rules.
func (ea *electionAlgorithm) filterRandom(groupA, groupB []*types.PillarDelegation, context *AlgorithmConfig) []*types.PillarDelegation {
	var result []*types.PillarDelegation
	total := int(ea.group.NodeCount)
	sort.Sort(types.SortPDByWeight(groupA))
	sort.Sort(types.SortPDByWeight(groupB))

	seed := ea.findSeed(context)
	// Number of active pillars is lower that the number of nodes in the consensus group.
	// Fill up result as many times as needed so there are no empty spots.
	if total != len(groupA) {
		for len(result) < total {
			random1 := rand.New(rand.NewSource(seed))
			arr := random1.Perm(len(groupA))
			for _, index := range arr {
				result = append(result, groupA[index])
			}
		}
		return result[:total]
	}

	// Select top pillars
	topTotal := total - int(ea.group.RandCount)
	topIndex := rand.New(rand.NewSource(seed)).Perm(len(groupA))

	for index := 0; index < topTotal; index += 1 {
		result = append(result, groupA[topIndex[index]])
	}

	// Insert unselected pillars in groupB for a second chx at becoming pillars.
	for index := topTotal; index < total; index += 1 {
		groupB = append(groupB, groupA[topIndex[index]])
	}

	// Select random pillars.
	randomIndex := rand.New(rand.NewSource(seed + 1)).Perm(len(groupB))[:ea.group.RandCount]
	for _, v := range randomIndex {
		promotion := groupB[v]
		result = append(result, promotion)
	}

	return result
}
