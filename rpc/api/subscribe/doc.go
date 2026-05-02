// Package subscribe implements the RPC subscription channel for
// momentum and account-block notifications.
//
// # Overview
//
// subscribe runs a singleton [Server] that registers as a chain
// listener, fans out chain events into per-subscription channels,
// and tears each subscription down when its client disconnects or
// unsubscribes. The [Api] surface (returned by [GetSubscribeApi])
// is registered under the "ledger" namespace via the
// [github.com/zenon-network/go-zenon/rpc] wiring; the
// [github.com/zenon-network/go-zenon/rpc/server] framework
// transports notifications back to the client over WebSocket.
//
// # Subscription Types
//
//   - "ledger.momentums" → momentum-header stream.
//   - "ledger.allAccountBlocks" → all account blocks (firehose).
//   - "ledger.accountBlocksByAddress" → blocks where Address matches.
//   - "ledger.unreceivedAccountBlocksByAddress" → unreceived
//     send-blocks targeting the given address.
//
// # Concurrency
//
// One goroutine ([Server.work]) owns the subscriptions map; install /
// uninstall / broadcast all marshal through channels with these
// buffer sizes:
//
//   - acChanSize = 100 (account-block events)
//   - mChanSize = 100 (momentum events)
//   - installSize = 100 (new subscriptions)
//   - uninstallSize = 100 (cancellations)
//
// If a channel fills up, the offending event is logged and dropped
// — the chain side never blocks waiting on slow subscribers.
//
// # Singleton Pattern
//
// The server is a process-wide singleton: [GetSubscribeServer]
// constructs / returns it; [GetSubscribeApi] returns its [Api]
// view. [GetSubscribeApi] panics if called before
// [GetSubscribeServer] / [Server.Start]. [Server.Stop] resets the
// singleton so subsequent calls can re-bootstrap (used by
// integration tests).
//
// # Generated Files
//
// None. Files are Zenon-specific (no upstream header).
//
// # Related Packages
//
//   - [github.com/zenon-network/go-zenon/rpc/server] — pub/sub
//     plumbing (Notifier, Subscription).
//   - [github.com/zenon-network/go-zenon/chain] — event source via
//     the chain.Listener interface implemented by [Server].
package subscribe
