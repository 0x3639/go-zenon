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

// ConsensusCache is the read-only view used by [PillarApi] to
// expose pillar weights and the current-epoch stats without paying
// the cost of a fresh consensus computation per request. Production
// implementation: [consensusCache], cached for 5 minutes.
type ConsensusCache interface {
	Get() (weights map[string]*big.Int, currentStats *api.EpochStats)
}

// consensusCache is the production [ConsensusCache] — refreshes the
// memoised weights / epoch-stats every 5 minutes (or on every Get
// call when testing=true so unit tests see fresh data immediately).
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

// Get returns the cached pillar weights and current-epoch stats.
// Triggers an asynchronous refresh when the cache is stale; in
// testing mode the refresh runs synchronously so tests see fresh
// data without timing dependencies.
func (cache *consensusCache) Get() (weights map[string]*big.Int, currentStats *api.EpochStats) {
	cache.changes.Lock()
	defer cache.changes.Unlock()

	// while testing serve only hot data
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

// shouldUpdate reports whether the cache is stale and not already
// being refreshed by another caller.
func (cache *consensusCache) shouldUpdate() bool {
	if cache.updating {
		return false
	}
	return cache.nextTime == nil || common.Clock.Now().After(*cache.nextTime)
}

// releaseUpdate clears the in-flight flag once the refresh
// goroutine finishes (success or failure).
func (cache *consensusCache) releaseUpdate() {
	cache.changes.Lock()
	defer cache.changes.Unlock()
	cache.updating = false
}

// update refreshes the memoised pillar weights and current-epoch
// stats from the consensus reader at the chain head. Errors are
// logged but not surfaced — Get keeps returning the previous values
// until the next successful refresh.
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

// NewConsensusCache constructs the production cache. testing=true
// runs every Get refresh synchronously and skips the 5-minute
// staleness window (useful in unit tests).
func NewConsensusCache(z zenon.Zenon, testing bool) ConsensusCache {
	return &consensusCache{
		testing:   testing,
		log:       common.RPCLogger.New("submodule", "consensus-cache"),
		chain:     z.Chain(),
		consensus: z.Consensus(),
	}
}
