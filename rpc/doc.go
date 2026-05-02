// Package rpc is the entry point for the JSON-RPC server.
//
// # Overview
//
// rpc is a thin wiring layer between the JSON-RPC framework in
// [github.com/zenon-network/go-zenon/rpc/server] and the public API
// surface in [github.com/zenon-network/go-zenon/rpc/api]. It exposes
// only one helper, [GetApis] / [GetPublicApis], used by
// [github.com/zenon-network/go-zenon/node] to translate config-style
// module names ("ledger", "embedded", "stats", "ledgerSubscribe") into
// the [rpc/server.API] descriptors the server registers.
//
// # API Modules
//
// Each module bundles one or more namespaces:
//
//   - "ledger"          → "ledger" namespace ([api.NewLedgerApi])
//   - "ledgerSubscribe" → "ledger" namespace, pub/sub
//     ([subscribe.GetSubscribeApi])
//   - "embedded"        → 11 "embedded.*" namespaces (token, sentinel,
//     pillar, plasma, stake, swap, spork, accelerator, htlc, bridge,
//     liquidity)
//   - "stats"           → "stats" namespace ([api.NewStatsApi])
//
// All registered APIs are marked Public — there is no admin-only
// surface in this build.
//
// # Generated Files
//
// None. apis.go is Zenon-specific (no upstream header).
//
// # Related Packages
//
//   - [github.com/zenon-network/go-zenon/rpc/server] — JSON-RPC 2.0
//     framework (transports, codec, dispatch, subscriptions). Ported
//     from go-ethereum, LGPL-3.0+.
//   - [github.com/zenon-network/go-zenon/rpc/api] — public ledger /
//     stats namespaces.
//   - [github.com/zenon-network/go-zenon/rpc/api/embedded] — RPC
//     wrappers for the embedded contracts.
//   - [github.com/zenon-network/go-zenon/rpc/api/subscribe] — pub/sub
//     surface for ledger events.
//   - [github.com/zenon-network/go-zenon/node] — instantiates the
//     server and selects which transports listen on which modules.
package rpc
