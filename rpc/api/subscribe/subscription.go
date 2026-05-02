package subscribe

import (
	"time"

	"github.com/inconshreveable/log15"
	rpc "github.com/zenon-network/go-zenon/rpc/server"

	"github.com/zenon-network/go-zenon/common"
	"github.com/zenon-network/go-zenon/common/types"
)

// SubscriptionType discriminates the four subscription kinds the
// dispatcher handles. Used as the outer key in
// [Server.subscriptions].
type SubscriptionType byte

// Subscription type values. FirstSubscriptionType /
// LastSubscriptionType are the half-open range bounds used by
// [Server.Init] to seed the per-type maps.
const (
	FirstSubscriptionType SubscriptionType = iota
	// AllAccountBlocksSubscription — firehose on every committed
	// account block.
	AllAccountBlocksSubscription
	// AccountBlocksSubscriptionByAddress — blocks issued by the
	// given address.
	AccountBlocksSubscriptionByAddress
	// UnreceivedAccountBlocksSubscriptionByAddress — send blocks
	// addressed to the given address.
	UnreceivedAccountBlocksSubscriptionByAddress
	// MomentumsSubscription — momentum-header notifications.
	MomentumsSubscription
	LastSubscriptionType
)

// subscriptionOptions captures the parameters of a subscription —
// the type discriminator plus, for address-scoped variants, the
// target address. createTime is recorded for diagnostics.
type subscriptionOptions struct {
	subscriptionType SubscriptionType
	createTime       time.Time
	address          types.Address
}

// newSubscription constructs an options record for the given type
// with createTime stamped to now.
func newSubscription(subscriptionType SubscriptionType) *subscriptionOptions {
	return &subscriptionOptions{
		subscriptionType: subscriptionType,
		createTime:       time.Now(),
	}
}

// NewBlocksSubscription returns options for the firehose
// AllAccountBlocksSubscription.
func NewBlocksSubscription() *subscriptionOptions {
	return newSubscription(AllAccountBlocksSubscription)
}

// NewBlocksByAddressSubscription returns options for an
// AccountBlocksSubscriptionByAddress filter on addr.
func NewBlocksByAddressSubscription(addr types.Address) *subscriptionOptions {
	sub := newSubscription(AccountBlocksSubscriptionByAddress)
	sub.address = addr
	return sub
}

// NewToUnreceivedBlocksSubscription returns options for an
// UnreceivedAccountBlocksSubscriptionByAddress filter on addr (send
// blocks where ToAddress == addr).
func NewToUnreceivedBlocksSubscription(addr types.Address) *subscriptionOptions {
	sub := newSubscription(UnreceivedAccountBlocksSubscriptionByAddress)
	sub.address = addr
	return sub
}

// NewMomentumsSubscription returns options for a
// MomentumsSubscription.
func NewMomentumsSubscription() *subscriptionOptions {
	return newSubscription(MomentumsSubscription)
}

// Subscription is one connected client's subscription. Bridges the
// dispatcher (which calls Notify on chain events) to the
// JSON-RPC framework (which transports the notification to the
// client and signals disconnect via rpc.Subscription.Err()).
type Subscription struct {
	log      log15.Logger
	options  *subscriptionOptions
	notifier *rpc.Notifier
	rpc      *rpc.Subscription
}

// NewSubscription wraps a freshly-created [rpc.Subscription] from
// the framework alongside the dispatch options.
func NewSubscription(notifier *rpc.Notifier, options *subscriptionOptions) *Subscription {
	rpcSub := notifier.CreateSubscription()
	return &Subscription{
		log:      common.RPCLogger.New("module", "subscription", "id", rpcSub.ID),
		options:  options,
		notifier: notifier,
		rpc:      rpcSub,
	}
}

// Notify pushes data to the client over the RPC subscription
// channel. No-ops if the subscription has been closed; logs (does
// not surface) transport errors.
func (s *Subscription) Notify(data interface{}) {
	if s.Closed() {
		return
	}

	err := s.notifier.Notify(s.rpc.ID, data)
	if err != nil {
		s.log.Info("failed to notify", "reason", err)
	}
}

// Closed reports whether the underlying RPC subscription has been
// torn down — either because the client unsubscribed (rpc.Err
// fired) or the WebSocket connection closed (notifier.Closed
// fired). Cleared notifier is the persistent record of "closed".
func (s *Subscription) Closed() bool {
	if s.notifier == nil {
		return true
	}
	select {
	case err := <-s.rpc.Err():
		s.log.Info("unsubscribing due to rpc-sub", "reason", err)
		s.notifier = nil
	case <-s.notifier.Closed():
		s.log.Info("unsubscribing", "reason", "notifier-closed")
		s.notifier = nil
	default:
	}
	return s.notifier == nil
}
