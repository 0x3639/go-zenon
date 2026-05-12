// Package subscribe implements the JSON-RPC pub-sub surface for
// account-block and momentum events. It is registered under the
// "ledger" RPC namespace (alongside LedgerApi's synchronous
// methods) by rpc/apis.go's "ledgerSubscribe" module so clients
// invoke subscriptions via ledger_subscribe / ledger_unsubscribe.
//
// # Server singleton
//
// The pub-sub state is a process-wide singleton. GetSubscribeServer
// returns (and lazily constructs) the Server bound to a given
// chain.Chain; calling it twice returns the same instance
// regardless of the second chain argument. The singleton must be
// Init()'d and Start()'d before the RPC layer hands subscriptions
// out: GetSubscribeApi panics if it is called before the server
// has started. Stop() resets the singleton so a fresh
// GetSubscribeServer call rebuilds it; this is mostly relevant for
// tests, which tear down and reinitialise between cases.
//
// # Event delivery pipeline
//
// The Server registers itself as a chain listener via
// chain.Register. When the chain inserts a new detailed momentum,
// it calls Server.InsertMomentum, which enqueues two events onto
// buffered channels: a Momentum event and a flattened slice of
// AccountBlock events (descendant blocks are walked in
// newAccountBlock). A background goroutine started by Start()
// drains those channels and fans events out to every active
// Subscription for the matching SubscriptionType. Channel sizes
// are bounded (acChanSize / mChanSize, both 100); when full the
// Server logs and drops the event rather than blocking the chain
// goroutine.
//
// # Subscription types
//
// Four subscription shapes are supported, all defined in
// subscription.go:
//
//   - MomentumsSubscription                          (no args)
//   - AllAccountBlocksSubscription                   (no args)
//   - AccountBlocksSubscriptionByAddress             (address filter)
//   - UnreceivedAccountBlocksSubscriptionByAddress   (address filter,
//     receive side)
//
// Each Api method documented in api.go installs one of these.
// Subscriptions are removed automatically when the underlying
// rpc/server.Subscription closes (client unsubscribe or transport
// drop); Subscription.Closed inspects the rpc-side error channel
// and the notifier-closed channel before each Notify.
//
// # What this package does NOT do
//
// Subscription transport (WebSocket / IPC framing,
// notifier lifecycle) and the eth_subscribe/eth_unsubscribe wire
// protocol live in rpc/server.Notifier. Chain-side event sourcing
// (when a new momentum is published) lives in chain/. This package
// is the bridge between those two layers, plus the four
// per-shape subscription installers.
package subscribe
