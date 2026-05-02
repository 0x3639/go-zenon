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

// Channel buffer sizes for the work-loop fan-in. Sized for typical
// chain throughput; overflow drops the event with a log line rather
// than blocking the chain side.
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

// Momentum is the lightweight notification-shape momentum (hash +
// height only) — clients refetch full content via the regular
// ledger API if they need it.
type Momentum struct {
	Hash   types.Hash `json:"hash"`
	Height uint64     `json:"height"`
}

// AccountBlock is the lightweight notification-shape account block —
// just the fields needed to identify the block and its
// send/receive direction.
type AccountBlock struct {
	BlockType uint64        `json:"blockType"`
	Hash      types.Hash    `json:"hash"`
	Height    uint64        `json:"height"`
	Address   types.Address `json:"address"`
	ToAddress types.Address `json:"toAddress"`
	FromHash  types.Hash    `json:"fromHash"`
}

// newAccountBlock projects a full chain-layer account block into
// one or more notification-shape AccountBlocks, recursively
// flattening descendant blocks (contract-emitted children).
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

// Api is the client-facing RPC surface — registered under the
// "ledger" namespace by [github.com/zenon-network/go-zenon/rpc].
// Its methods (Momentums, AllAccountBlocks, etc.) install
// subscriptions on the singleton [Server].
type Api struct {
	chain     chain.Chain
	log       log15.Logger
	installCh chan *Subscription // add subscription
}

// Server is the singleton subscription dispatcher. Embeds [Api] so
// callers of GetSubscribeApi see the install channel without
// needing the dispatch internals. Implements [chain.Listener] so
// chain events flow into InsertMomentum / DeleteMomentum and out
// to subscribed clients.
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

// GetSubscribeServer returns the process-wide subscription server,
// constructing it on first call. Idempotent across goroutines.
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

// GetSubscribeApi returns the [Api] handle for RPC registration.
// Panics if called before GetSubscribeServer / Server.Start — the
// node startup sequence guarantees both run first.
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

// Init initialises the per-subscription-type maps. Called by the
// node lifecycle before Start.
func (s *Server) Init() error {
	s.log.Info("init")
	defer s.log.Info("finish init")
	for i := FirstSubscriptionType; i < LastSubscriptionType; i++ {
		s.subscriptions[i] = make(map[rpc.ID]*Subscription)
	}
	return nil
}

// Start registers the server as a chain listener and launches the
// work-loop goroutine. After Start, GetSubscribeApi may be called.
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

// Stop unregisters from the chain, closes the work loop, and
// resets the singleton so future GetSubscribeServer calls can
// rebootstrap (used by integration tests).
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

// InsertMomentum is the chain-listener callback invoked on each
// committed momentum. Pushes one momentum event and one batch of
// account-block events into the work-loop channels; full channels
// drop the event with an error log.
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

// DeleteMomentum is part of the chain-listener interface but
// unused — reorgs don't currently emit retract notifications.
func (s *Server) DeleteMomentum(*nom.DetailedMomentum) {
}

// work is the central event loop: drains install / uninstall /
// momentum / account-block channels and dispatches each. Owns the
// subscriptions map; no other goroutine reads or writes it.
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

// BroadcastStats is the counter pair logged after each
// broadcast pass — useful for diagnosing slow subscribers.
type BroadcastStats struct {
	NumNotify     int
	NumUninstalls int
}

// install adds subscription to the per-type map. Called from the
// work-loop only.
func (s *Server) install(subscription *Subscription) {
	s.log.Info("install", "id", subscription.rpc.ID)
	s.subscriptions[subscription.options.subscriptionType][subscription.rpc.ID] = subscription
}

// uninstall removes subscription from the per-type map. Called
// from the work-loop only — either via the uninstallCh path
// (client disconnect) or inline from broadcast when a notify
// fails.
func (s *Server) uninstall(subscription *Subscription) {
	s.log.Info("uninstall", "id", subscription.rpc.ID)
	delete(s.subscriptions[subscription.options.subscriptionType], subscription.rpc.ID)
}

// broadcast notifies one subscriber, uninstalling on the spot if
// the connection has already closed.
func (s *Server) broadcast(subscription *Subscription, data interface{}, stats *BroadcastStats) {
	if subscription.Closed() {
		stats.NumUninstalls += 1
		s.uninstall(subscription)
	} else {
		stats.NumNotify += 1
		subscription.Notify(data)
	}
}

// broadcastMomentums fans a single momentum event out to every
// subscriber on MomentumsSubscription.
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

// broadcastBlocks dispatches a batch of account-block events:
// firehose to AllAccountBlocksSubscription, per-address filters to
// AccountBlocksSubscriptionByAddress, and unreceived-only filtering
// (send blocks only) to UnreceivedAccountBlocksSubscriptionByAddress.
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

// subscribe is the shared body of the RPC subscription methods —
// extracts the [rpc.Notifier] from ctx, installs the subscription,
// and returns the rpc.Subscription handle the framework uses to
// transmit notifications and detect client disconnect.
func (s *Api) subscribe(ctx context.Context, options *subscriptionOptions) (*rpc.Subscription, error) {
	notifier, supported := rpc.NotifierFromContext(ctx)
	if !supported {
		return nil, rpc.ErrNotificationsUnsupported
	}
	subscription := NewSubscription(notifier, options)
	s.installCh <- subscription
	return subscription.rpc, nil
}

// Momentums opens a subscription that fires on every committed
// momentum.
func (s *Api) Momentums(ctx context.Context) (*rpc.Subscription, error) {
	s.log.Info("new subscription", "type", "Momentums")
	return s.subscribe(ctx, NewMomentumsSubscription())
}

// AllAccountBlocks opens a firehose subscription on every committed
// account block — high-volume; intended for explorers and indexers,
// not user wallets.
func (s *Api) AllAccountBlocks(ctx context.Context) (*rpc.Subscription, error) {
	s.log.Info("new subscription", "type", "AllAccountBlocks")
	return s.subscribe(ctx, NewBlocksSubscription())
}

// AccountBlocksByAddress opens a subscription on account blocks
// whose Address (issuer) matches the given address.
func (s *Api) AccountBlocksByAddress(ctx context.Context, address types.Address) (*rpc.Subscription, error) {
	s.log.Info("new subscription", "type", "AccountBlocksByAddress")
	return s.subscribe(ctx, NewBlocksByAddressSubscription(address))
}

// UnreceivedAccountBlocksByAddress opens a subscription on send
// blocks whose ToAddress matches address — i.e., new "incoming"
// transfers the recipient hasn't yet receipted. Useful for wallets
// auto-prompting to claim receives.
func (s *Api) UnreceivedAccountBlocksByAddress(ctx context.Context, address types.Address) (*rpc.Subscription, error) {
	s.log.Info("new subscription", "type", "UnreceivedAccountBlocksByAddress")
	return s.subscribe(ctx, NewToUnreceivedBlocksSubscription(address))
}
