package embedded

import (
	"math/big"
	"sync"
	"time"

	"github.com/zenon-network/go-zenon/chain"
	"github.com/zenon-network/go-zenon/common"
	"github.com/zenon-network/go-zenon/consensus"
	"github.com/zenon-network/go-zenon/consensus/api"
	"github.com/zenon-network/go-zenon/zenon"
)

// ConsensusCache exposes a single Get method that returns the
// most recently cached pillar weights (keyed by pillar name) and
// epoch stats. The implementation backs PillarApi so per-call
// pillar enumeration does not have to walk the consensus reader
// every time. See NewConsensusCache for the refresh policy.
type ConsensusCache interface {
	Get() (weights map[string]*big.Int, currentStats *api.EpochStats)
}

type consensusCache struct {
	testing   bool
	log       common.Logger
	chain     chain.Chain
	consensus consensus.Consensus

	updating     bool
	changes      sync.Mutex
	nextTime     *time.Time
	weights      map[string]*big.Int
	currentStats *api.EpochStats
}

// Get returns the currently cached pillar weights and epoch stats.
//
// In production (testing == false) Get is non-blocking: it
// returns the existing snapshot and, if the snapshot's nextTime
// has passed, launches a background refresh via `go cache.update()`.
// The very first call before any refresh has populated the cache
// returns (nil, nil) — callers must tolerate that.
//
// In test mode (testing == true) Get refreshes synchronously: it
// drops the lock, runs cache.update() inline, retakes the lock,
// and only then reads the snapshot. This makes pillar-enumeration
// tests deterministic at the cost of blocking the caller for the
// duration of the consensus reader call.
func (cache *consensusCache) Get() (weights map[string]*big.Int, currentStats *api.EpochStats) {
	cache.changes.Lock()
	defer cache.changes.Unlock()

	if cache.testing {
		cache.changes.Unlock()
		cache.update()
		cache.changes.Lock()
	} else if cache.shouldUpdate() {
		cache.updating = true
		go cache.update()
	}

	weights = cache.weights
	currentStats = cache.currentStats
	return
}

func (cache *consensusCache) shouldUpdate() bool {
	if cache.updating {
		return false
	}
	return cache.nextTime == nil || common.Clock.Now().After(*cache.nextTime)
}
func (cache *consensusCache) releaseUpdate() {
	cache.changes.Lock()
	defer cache.changes.Unlock()
	cache.updating = false
}
func (cache *consensusCache) update() {
	defer cache.releaseUpdate()
	startTime := common.Clock.Now()

	frontierMomentum, err := cache.chain.GetFrontierMomentumStore().GetFrontierMomentum()
	if err != nil {
		cache.log.Error("failed to get frontier momentum", "reason", err)
		return
	}
	if frontierMomentum == nil {
		cache.log.Error("failed to get frontier momentum", "reason", "frontier-momentum is missing")
		return
	}

	reader := cache.consensus.FixedPillarReader(frontierMomentum.Identifier())
	epoch := reader.EpochTicker().ToTick(*frontierMomentum.Timestamp)

	cache.log.Debug("updating rpc consensus cache", "identifier", frontierMomentum.Identifier(), "epoch", epoch)

	weights, err := reader.GetPillarWeights()
	if err != nil {
		cache.log.Error("failed to get pillar weights", "reason", err, "momentum-identifier", frontierMomentum.Identifier())
		return
	}
	stats, err := reader.EpochStats(epoch)
	if err != nil {
		cache.log.Error("failed to get epoch stats", "reason", err, "momentum-identifier", frontierMomentum.Identifier())
		return
	}

	cache.changes.Lock()
	defer cache.changes.Unlock()
	nextTime := common.Clock.Now().Add(time.Minute * 5)
	cache.weights = weights
	cache.currentStats = stats
	cache.nextTime = &nextTime

	endTime := common.Clock.Now()
	cache.log.Debug("finish updating rpc consensus", "elapsed", endTime.Sub(startTime), "next-time", nextTime)
}

// NewConsensusCache returns a fresh ConsensusCache for z. The
// testing flag chooses the refresh strategy:
//
//   - testing == false: Get returns the existing snapshot
//     immediately and triggers an asynchronous refresh when
//     shouldUpdate reports the cache is stale. Refresh cadence is
//     5 minutes between successful updates.
//
//   - testing == true: Get refreshes synchronously on every call
//     so tests do not race against the background goroutine.
//
// The cache starts empty; the first Get in production mode will
// return nil maps until the first background refresh completes.
func NewConsensusCache(z zenon.Zenon, testing bool) ConsensusCache {
	return &consensusCache{
		testing:   testing,
		log:       common.RPCLogger.New("submodule", "consensus-cache"),
		chain:     z.Chain(),
		consensus: z.Consensus(),
	}
}
