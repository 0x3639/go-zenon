package consensus

import (
	"github.com/pkg/errors"

	"github.com/zenon-network/go-zenon/chain"
	"github.com/zenon-network/go-zenon/chain/nom"
	"github.com/zenon-network/go-zenon/common"
)

// ChainTicker extends [common.Ticker] with chain-aware queries: per-tick
// progress checks (started / finished against the chain frontier) and
// per-tick content lookups (the head momentum and the full block list
// inside a tick window).
//
// The points subsystem consumes a ChainTicker to walk completed ticks
// and aggregate per-pillar performance statistics.
type ChainTicker interface {
	common.Ticker
	// IsFinished reports whether the chain has progressed past tick's
	// end time.
	IsFinished(tick uint64) bool
	// HasStarted reports whether the chain has reached or passed
	// tick's start time.
	HasStarted(tick uint64) bool
	// GetEndBlock returns the head momentum of tick (the most recent
	// momentum strictly before tick's end time).
	GetEndBlock(tick uint64) (*nom.Momentum, error)
	// GetContent returns every momentum committed within tick's
	// wall-clock window, in ascending height order.
	GetContent(tick uint64) ([]*nom.Momentum, error)
}

// chainTicker is the [ChainTicker] implementation: a [common.Ticker]
// glued to a [chain.Chain] so it can read the live frontier.
type chainTicker struct {
	common.Ticker
	chain.Chain
}

// IsFinished reports whether the chain frontier has reached or passed
// tick's end time. Panics on tick values close to math.MaxUint64 — the
// caller has almost certainly hit an arithmetic overflow.
func (ct *chainTicker) IsFinished(tick uint64) bool {
	if tick > (1<<62)-1 {
		panic("most probably an overflow error")
	}
	_, eTime := ct.ToTime(tick)
	block, err := ct.GetFrontierMomentumStore().GetFrontierMomentum()
	common.DealWithErr(err)
	if block.Timestamp.After(eTime) || block.Timestamp.Equal(eTime) {
		return true
	}
	return false
}

// HasStarted reports whether the chain frontier has reached or passed
// tick's start time. Panics on overflow as in [IsFinished].
func (ct *chainTicker) HasStarted(tick uint64) bool {
	if tick > (1<<62)-1 {
		panic("most probably an overflow error")
	}
	sTime, _ := ct.ToTime(tick)
	block, err := ct.GetFrontierMomentumStore().GetFrontierMomentum()
	common.DealWithErr(err)
	if block.Timestamp.Before(sTime) {
		return false
	}
	return true
}

// GetEndBlock returns the most recent momentum strictly before tick's
// end time. By construction this is the last momentum that belongs to
// `tick` (or, when `tick` produced no momentum at all, the trailing
// momentum from an earlier tick — the underlying lookup walks
// backwards from end-time via [store.Momentum.GetMomentumBeforeTime]).
// Returns an error if no momentum exists before that time at all
// (e.g. a tick before genesis).
func (ct *chainTicker) GetEndBlock(tick uint64) (*nom.Momentum, error) {
	if tick > (1<<62)-1 {
		panic("most probably an overflow error")
	}
	_, eTime := ct.ToTime(tick)
	block, err := ct.GetFrontierMomentumStore().GetMomentumBeforeTime(&eTime)
	if err != nil {
		return nil, err
	}
	if block == nil {
		return nil, errors.Errorf("chainTicker.GetEndBlock failed to get block for tick %v endTime %v", tick, eTime.Unix())
	}
	return block, err
}

// GetContent returns every momentum committed within tick's wall-clock
// window, in ascending order. Returns an empty slice when no blocks
// fall in the window (off-line tick) or when start and end resolve to
// the same height.
//
// Tick 0 anchors at the genesis momentum; later ticks walk between the
// previous tick's end-block and this tick's end-block.
func (ct *chainTicker) GetContent(tick uint64) ([]*nom.Momentum, error) {
	if tick > (1<<62)-1 {
		panic("most probably an overflow error")
	}
	sTime, _ := ct.ToTime(tick)
	endBlock, err := ct.GetEndBlock(tick)
	if err != nil {
		return nil, err
	}

	if !endBlock.Timestamp.Before(sTime) {
		var startBlock *nom.Momentum
		if tick == 0 {
			startBlock = ct.GetGenesisMomentum()
		} else {
			startBlock, err = ct.GetEndBlock(tick - 1)
			if err != nil {
				return nil, err
			}
			if startBlock == nil {
				return nil, errors.Errorf("failed to get startBlock for content. Tick:%v", tick)
			}
		}

		if startBlock.Height == endBlock.Height {
			return []*nom.Momentum{}, nil
		}

		store := ct.GetFrontierMomentumStore()
		blocks, err := store.GetMomentumsByHeight(startBlock.Height+1, true, endBlock.Height-startBlock.Height)
		if err != nil {
			return nil, err
		}

		// empty genesis tick
		if len(blocks) == 0 {
			return []*nom.Momentum{}, nil
		}

		// make sure proof is right
		if endBlock.Hash != blocks[len(blocks)-1].Hash {
			return nil, errors.Errorf("chainTicker.GetContent failed expects %v but got %v", endBlock.Hash, blocks[0].Hash)
		}
		return blocks, nil
	} else {
		return []*nom.Momentum{}, nil
	}
}

// newChainTicker wires a [chainTicker] from chain and ticker.
func newChainTicker(chain chain.Chain, ticker common.Ticker) *chainTicker {
	return &chainTicker{
		Chain:  chain,
		Ticker: ticker,
	}
}
