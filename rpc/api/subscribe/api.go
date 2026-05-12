package subscribe

import (
	"context"
	"sync"

	"github.com/inconshreveable/log15"

	"github.com/zenon-network/go-zenon/chain"
	"github.com/zenon-network/go-zenon/chain/nom"
	"github.com/zenon-network/go-zenon/common"
	"github.com/zenon-network/go-zenon/common/types"
	rpc "github.com/zenon-network/go-zenon/rpc/server"
)

const (
	acChanSize    = 100
	mChanSize     = 100
	installSize   = 100
	uninstallSize = 100
)

var (
	oneSingleton sync.Mutex
	singleton    *Server
)

// Momentum is the compact event payload broadcast on momentum
// subscriptions: just the hash and height of the inserted
// momentum. Subscribers that need the full body must follow up
// with LedgerApi.GetMomentumByHash.
type Momentum struct {
	Hash   types.Hash `json:"hash"`
	Height uint64     `json:"height"`
}

// AccountBlock is the compact event payload broadcast on
// account-block subscriptions: enough to identify the block and
// the parties involved without prefetching the full record.
// FromHash is the source send block's hash for receive-side
// events, and the empty hash for send-side events.
type AccountBlock struct {
	BlockType uint64        `json:"blockType"`
	Hash      types.Hash    `json:"hash"`
	Height    uint64        `json:"height"`
	Address   types.Address `json:"address"`
	ToAddress types.Address `json:"toAddress"`
	FromHash  types.Hash    `json:"fromHash"`
}

func newAccountBlock(block *nom.AccountBlock) []*AccountBlock {
	all := make([]*AccountBlock, 1, len(block.DescendantBlocks)+1)
	all[0] = &AccountBlock{
		BlockType: block.BlockType,
		Hash:      block.Hash,
		Height:    block.Height,
		Address:   block.Address,
		ToAddress: block.ToAddress,
		FromHash:  block.FromBlockHash,
	}
	for _, dBlock := range block.DescendantBlocks {
		all = append(all, newAccountBlock(dBlock)...)
	}
	return all
}

// Api is the RPC handler half of the subscribe layer: the small
// surface that znnd registers under the "ledger" namespace for
// subscription methods (Momentums / AllAccountBlocks /
// AccountBlocksByAddress / UnreceivedAccountBlocksByAddress).
// Each method installs a Subscription into the Server's per-type
// map via installCh; the Server's worker goroutine drains that
// channel and the matching event channels.
type Api struct {
	chain     chain.Chain
	log       log15.Logger
	installCh chan *Subscription // add subscription
}

// Server is the process-wide event broker: it holds the live
// subscriptions, fans incoming chain events out to them, and
// registers/deregisters itself with chain.Chain as a momentum
// listener. The embedded *Api gives the Server access to the same
// installCh that the public subscription methods write to. Server
// is a singleton constructed by GetSubscribeServer; GetSubscribeApi
// returns its embedded *Api after Start().
type Server struct {
	*Api

	started       bool
	uninstallCh   chan *Subscription // remove subscription
	acCh          chan []*AccountBlock
	mCh           chan *Momentum
	stopped       chan struct{}
	subscriptions map[SubscriptionType]map[rpc.ID]*Subscription

	wg sync.WaitGroup
}

// GetSubscribeServer returns the process-wide subscribe Server,
// lazily constructing it on first call bound to the supplied
// chain.Chain. Subsequent calls return the existing singleton and
// ignore the chain argument. Safe to call concurrently
// (synchronised via oneSingleton). Stop() resets the singleton so
// the next call rebuilds it — useful for tests that tear down
// between cases.
func GetSubscribeServer(chain chain.Chain) *Server {
	oneSingleton.Lock()
	defer oneSingleton.Unlock()

	if singleton == nil {
		singleton = &Server{
			Api: &Api{
				chain:     chain,
				log:       common.RPCLogger.New("module", "subscribe_api"),
				installCh: make(chan *Subscription, installSize),
			},

			acCh:          make(chan []*AccountBlock, acChanSize),
			mCh:           make(chan *Momentum, mChanSize),
			uninstallCh:   make(chan *Subscription, uninstallSize),
			stopped:       make(chan struct{}),
			subscriptions: make(map[SubscriptionType]map[rpc.ID]*Subscription),
		}
	}
	return singleton
}

// GetSubscribeApi returns the embedded *Api on the running
// singleton Server. **Panics** if GetSubscribeServer has not yet
// been called, or if the Server has been constructed but not yet
// Start()ed — RPC routing must not hand subscription handlers to
// clients before the event-fanout goroutine is alive.
func GetSubscribeApi() *Api {
	oneSingleton.Lock()
	defer oneSingleton.Unlock()
	if singleton == nil {
		panic("must call GetSubscribeServer once before calling GetSubscribeApi")
	}
	if !singleton.started {
		panic("must start SubscribeServer before calling GetSubscribeApi")
	}
	return singleton.Api
}

// Init prepares the Server's per-type subscription map. Must be
// called before Start. Returns nil unconditionally; the (error)
// return is kept for forward compatibility.
func (s *Server) Init() error {
	s.log.Info("init")
	defer s.log.Info("finish init")
	for i := FirstSubscriptionType; i < LastSubscriptionType; i++ {
		s.subscriptions[i] = make(map[rpc.ID]*Subscription)
	}
	return nil
}

// Start registers the Server as a momentum listener on its chain
// (chain.Register), marks the Server as started so GetSubscribeApi
// will hand out the *Api, and spawns the worker goroutine that
// drains the install/uninstall/event channels. Returns nil
// unconditionally.
func (s *Server) Start() error {
	s.log.Info("start")
	defer s.log.Info("finish start")
	s.started = true
	s.chain.Register(s)
	s.wg.Add(1)
	go func() {
		s.work()
		s.wg.Done()
	}()
	return nil
}

// Stop tears the Server down: unregisters from chain.Chain,
// signals the worker via the stopped channel, waits for the
// goroutine to exit, and clears the singleton so the next
// GetSubscribeServer call constructs a fresh Server. Active
// subscriptions are dropped when the worker exits and clears
// s.subscriptions. Returns nil unconditionally.
func (s *Server) Stop() error {
	s.log.Info("stop")
	defer s.log.Info("finish stop")
	s.started = false
	s.chain.UnRegister(s)
	close(s.stopped)
	singleton = nil
	s.log.Debug("wg.Wait() api Server.Stop()")
	s.wg.Wait()
	s.log.Debug("wg.Wait() api Server.Stop() finish")
	return nil
}

// InsertMomentum implements the chain.MomentumEventListener side
// of the Server: receives a newly inserted detailed momentum and
// enqueues two events onto the worker channels — a Momentum and
// the flattened slice of AccountBlock events for every block (and
// recursive descendant block) in the momentum. **Non-blocking**:
// if either channel is full, the event is dropped with an error
// log rather than stalling the chain's insertion goroutine. The
// trade-off favours liveness of the chain over delivery
// guarantees for subscribers.
func (s *Server) InsertMomentum(detailed *nom.DetailedMomentum) {
	select {
	case s.mCh <- &Momentum{
		Hash:   detailed.Momentum.Hash,
		Height: detailed.Momentum.Height,
	}:
	default:
		s.log.Error("can't insert momentum for broadcast", "reason", "channel is full", "momentum-identifier", detailed.Momentum.Identifier())
	}

	abEvents := make([]*AccountBlock, 0, len(detailed.AccountBlocks))
	for _, block := range detailed.AccountBlocks {
		abEvents = append(abEvents, newAccountBlock(block)...)
	}
	select {
	case s.acCh <- abEvents:
	default:
		s.log.Error("can't insert account-blocks for broadcast", "reason", "channel is full", "momentum-identifier", detailed.Momentum.Identifier())
	}
	return
}

// DeleteMomentum implements the chain.MomentumEventListener side
// of the Server for rollbacks. The current implementation is a
// no-op: subscribers receive insert events but no compensating
// delete events on chain reorgs. Callers that need rollback
// awareness must reconcile against LedgerApi reads.
func (s *Server) DeleteMomentum(*nom.DetailedMomentum) {
}

func (s *Server) work() {
	log := s.log.New("module", "worker")
	defer common.RecoverStack()
	log.Info("start event loop")
	defer log.Info("stop event loop")
	for {
		select {
		case <-s.stopped:
			log.Info("stopped")
			s.subscriptions = nil
			return
		case sub := <-s.installCh:
			s.install(sub)
		case sub := <-s.uninstallCh:
			s.uninstall(sub)
		case momentums := <-s.mCh:
			s.broadcastMomentums(momentums)
		case blocks := <-s.acCh:
			s.broadcastBlocks(blocks)
		}
	}
}

// BroadcastStats accumulates the result of one broadcast pass:
// how many subscriptions received a Notify, and how many were
// uninstalled in-place because they reported Closed. Logged at
// info level at the end of each broadcastMomentums /
// broadcastBlocks call so operators can correlate event volume
// with subscriber churn.
type BroadcastStats struct {
	NumNotify     int
	NumUninstalls int
}

func (s *Server) install(subscription *Subscription) {
	s.log.Info("install", "id", subscription.rpc.ID)
	s.subscriptions[subscription.options.subscriptionType][subscription.rpc.ID] = subscription
}
func (s *Server) uninstall(subscription *Subscription) {
	s.log.Info("uninstall", "id", subscription.rpc.ID)
	delete(s.subscriptions[subscription.options.subscriptionType], subscription.rpc.ID)
}
func (s *Server) broadcast(subscription *Subscription, data interface{}, stats *BroadcastStats) {
	if subscription.Closed() {
		stats.NumUninstalls += 1
		s.uninstall(subscription)
	} else {
		stats.NumNotify += 1
		subscription.Notify(data)
	}
}
func (s *Server) broadcastMomentums(momentum *Momentum) {
	if momentum == nil {
		return
	}
	startTime := common.Clock.Now()
	stats := &BroadcastStats{}

	for _, f := range s.subscriptions[MomentumsSubscription] {
		s.broadcast(f, []interface{}{momentum}, stats)
	}

	s.log.Info("finish broadcasting momentum", "identifier", momentum, "elapsed", common.Clock.Now().Sub(startTime), "stats", stats)
}
func (s *Server) broadcastBlocks(blocks []*AccountBlock) {
	if len(blocks) == 0 {
		return
	}
	startTime := common.Clock.Now()
	stats := &BroadcastStats{}

	byAddress := make(map[types.Address][]*AccountBlock)
	unreceivedByAddress := make(map[types.Address][]*AccountBlock)
	for _, block := range blocks {
		if _, ok := byAddress[block.Address]; !ok {
			byAddress[block.Address] = make([]*AccountBlock, 0)
		}
		byAddress[block.Address] = append(byAddress[block.Address], block)
		if nom.IsSendBlock(block.BlockType) {
			if _, ok := unreceivedByAddress[block.ToAddress]; !ok {
				unreceivedByAddress[block.ToAddress] = make([]*AccountBlock, 0)
			}
			unreceivedByAddress[block.ToAddress] = append(unreceivedByAddress[block.ToAddress], block)
		}
	}

	for _, f := range s.subscriptions[AllAccountBlocksSubscription] {
		s.broadcast(f, blocks, stats)
	}
	for _, f := range s.subscriptions[AccountBlocksSubscriptionByAddress] {
		if blocks, ok := byAddress[f.options.address]; ok {
			s.broadcast(f, blocks, stats)
		}
	}
	for _, f := range s.subscriptions[UnreceivedAccountBlocksSubscriptionByAddress] {
		if blocks, ok := unreceivedByAddress[f.options.address]; ok {
			s.broadcast(f, blocks, stats)
		}
	}

	s.log.Info("finish broadcasting account-blocks", "elapsed", common.Clock.Now().Sub(startTime), "stats", stats)
}

func (s *Api) subscribe(ctx context.Context, options *subscriptionOptions) (*rpc.Subscription, error) {
	notifier, supported := rpc.NotifierFromContext(ctx)
	if !supported {
		return nil, rpc.ErrNotificationsUnsupported
	}
	subscription := NewSubscription(notifier, options)
	s.installCh <- subscription
	return subscription.rpc, nil
}

// Momentums installs a subscription that fires once per momentum
// inserted by the chain. Each event carries Momentum (hash +
// height); subscribers needing the full body must follow up with
// LedgerApi.GetMomentumByHash. Returns
// rpc.ErrNotificationsUnsupported when the underlying transport
// does not support push notifications (e.g. plain HTTP).
func (s *Api) Momentums(ctx context.Context) (*rpc.Subscription, error) {
	s.log.Info("new subscription", "type", "Momentums")
	return s.subscribe(ctx, NewMomentumsSubscription())
}

// AllAccountBlocks installs an unfiltered account-block
// subscription: every block emitted by every account, including
// recursively-walked descendant blocks. Same transport caveat as
// Momentums (rpc.ErrNotificationsUnsupported on plain HTTP).
func (s *Api) AllAccountBlocks(ctx context.Context) (*rpc.Subscription, error) {
	s.log.Info("new subscription", "type", "AllAccountBlocks")
	return s.subscribe(ctx, NewBlocksSubscription())
}

// AccountBlocksByAddress installs an account-block subscription
// filtered to blocks where AccountBlock.Address == address (the
// producer side). Same transport caveat as Momentums.
func (s *Api) AccountBlocksByAddress(ctx context.Context, address types.Address) (*rpc.Subscription, error) {
	s.log.Info("new subscription", "type", "AccountBlocksByAddress")
	return s.subscribe(ctx, NewBlocksByAddressSubscription(address))
}

// UnreceivedAccountBlocksByAddress installs a subscription that
// fires when a send block destined for address is broadcast — the
// receive side, so address knows it owes an acknowledgement.
// Only send blocks (where AccountBlock.ToAddress == address) reach
// the subscriber. Same transport caveat as Momentums.
func (s *Api) UnreceivedAccountBlocksByAddress(ctx context.Context, address types.Address) (*rpc.Subscription, error) {
	s.log.Info("new subscription", "type", "UnreceivedAccountBlocksByAddress")
	return s.subscribe(ctx, NewToUnreceivedBlocksSubscription(address))
}
