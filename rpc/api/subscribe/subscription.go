package subscribe

import (
	"time"

	"github.com/inconshreveable/log15"
	rpc "github.com/zenon-network/go-zenon/rpc/server"

	"github.com/zenon-network/go-zenon/common"
	"github.com/zenon-network/go-zenon/common/types"
)

// SubscriptionType tags one of the four event shapes the
// subscribe Server can broadcast. The zero value
// (FirstSubscriptionType) is reserved as a low bound for the
// Server.subscriptions map initialisation loop; LastSubscriptionType
// is the matching high bound. Real subscriptions must use one of
// the four enumerated middle values.
type SubscriptionType byte

// Subscription-type identifiers. The numeric values are not part
// of any wire protocol — they exist only as map keys inside the
// Server. FirstSubscriptionType / LastSubscriptionType bracket
// the iteration in Server.Init.
const (
	FirstSubscriptionType SubscriptionType = iota
	// AllAccountBlocksSubscription fires for every account block
	// produced by every account (no address filter).
	AllAccountBlocksSubscription
	// AccountBlocksSubscriptionByAddress fires for account blocks
	// produced by a specific address.
	AccountBlocksSubscriptionByAddress
	// UnreceivedAccountBlocksSubscriptionByAddress fires for send
	// blocks destined for a specific address (the receive side).
	UnreceivedAccountBlocksSubscriptionByAddress
	// MomentumsSubscription fires once per inserted momentum.
	MomentumsSubscription
	LastSubscriptionType
)

type subscriptionOptions struct {
	subscriptionType SubscriptionType
	createTime       time.Time
	address          types.Address
}

func newSubscription(subscriptionType SubscriptionType) *subscriptionOptions {
	return &subscriptionOptions{
		subscriptionType: subscriptionType,
		createTime:       time.Now(),
	}
}

// NewBlocksSubscription returns options for an unfiltered
// account-block subscription (AllAccountBlocksSubscription).
func NewBlocksSubscription() *subscriptionOptions {
	return newSubscription(AllAccountBlocksSubscription)
}

// NewBlocksByAddressSubscription returns options for an
// account-block subscription scoped to addr — fires for blocks
// where AccountBlock.Address == addr.
func NewBlocksByAddressSubscription(addr types.Address) *subscriptionOptions {
	sub := newSubscription(AccountBlocksSubscriptionByAddress)
	sub.address = addr
	return sub
}

// NewToUnreceivedBlocksSubscription returns options for an
// unreceived-block subscription scoped to addr — fires for send
// blocks where AccountBlock.ToAddress == addr (i.e., transfers
// the address still needs to acknowledge).
func NewToUnreceivedBlocksSubscription(addr types.Address) *subscriptionOptions {
	sub := newSubscription(UnreceivedAccountBlocksSubscriptionByAddress)
	sub.address = addr
	return sub
}

// NewMomentumsSubscription returns options for a momentum
// subscription (one event per momentum, no filter).
func NewMomentumsSubscription() *subscriptionOptions {
	return newSubscription(MomentumsSubscription)
}

// Subscription is the package's wrapper around one
// rpc/server.Subscription: it captures the notifier handle, the
// JSON-RPC subscription id, and the subscriptionOptions that
// describe which event shape this subscription wants. The Server
// holds these in its per-type map and uses Notify to push events
// out.
type Subscription struct {
	log      log15.Logger
	options  *subscriptionOptions
	notifier *rpc.Notifier
	rpc      *rpc.Subscription
}

// NewSubscription registers a fresh rpc/server.Subscription with
// notifier and returns the wrapper bound to options. The new
// Subscription is installed into the Server's per-type map by
// Api.subscribe (not here) — this constructor only assembles the
// value.
func NewSubscription(notifier *rpc.Notifier, options *subscriptionOptions) *Subscription {
	rpcSub := notifier.CreateSubscription()
	return &Subscription{
		log:      common.RPCLogger.New("module", "subscription", "id", rpcSub.ID),
		options:  options,
		notifier: notifier,
		rpc:      rpcSub,
	}
}

// Notify pushes data to the subscriber if the underlying
// rpc/server.Subscription is still open. A failed Notify is
// logged at info level but does not mark the subscription closed
// or surface an error — the next Closed check will pick up the
// underlying transport drop. data may be any JSON-marshalable
// value; rpc/server handles the serialisation.
func (s *Subscription) Notify(data interface{}) {
	if s.Closed() {
		return
	}

	err := s.notifier.Notify(s.rpc.ID, data)
	if err != nil {
		s.log.Info("failed to notify", "reason", err)
	}
}

// Closed reports whether the subscription has been torn down. It
// inspects two signals from rpc/server: the subscription's error
// channel (set when the client unsubscribes) and the notifier's
// closed channel (set when the connection drops). Either one
// causes Closed to clear s.notifier so subsequent calls short-
// circuit and the Server can uninstall the subscription on its
// next broadcast pass.
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
