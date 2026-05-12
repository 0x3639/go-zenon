// Package rpc is the top-level binding glue between the JSON-RPC
// server primitives in rpc/server and the per-domain handler
// packages — rpc/api (LedgerApi, StatsApi), rpc/api/subscribe
// (pub-sub for momentum / account-block events), and the
// per-contract handlers in rpc/api/embedded.
//
// # Modules and namespaces
//
// znnd selects which API surface to expose with a list of module
// names. getApi maps each name to a slice of rpc.API descriptors
// (Namespace / Version / Service / Public). The known module names
// and the namespaces they register are:
//
//   - "ledger"          → "ledger"            (LedgerApi)
//   - "ledgerSubscribe" → "ledger"            (subscribe.Api)
//   - "stats"           → "stats"             (StatsApi)
//   - "embedded"        → "embedded.token",
//     "embedded.sentinel",
//     "embedded.pillar",
//     "embedded.plasma",
//     "embedded.stake",
//     "embedded.swap",
//     "embedded.spork",
//     "embedded.accelerator",
//     "embedded.htlc",
//     "embedded.bridge",
//     "embedded.liquidity"
//
// "ledger" and "ledgerSubscribe" share the same JSON-RPC namespace
// ("ledger") but register different services — call methods land on
// LedgerApi, subscription methods (eth_subscribe-style) land on
// subscribe.Api. The two are registered separately so a node can
// expose synchronous reads without enabling streaming, and vice
// versa.
//
// An unknown module name yields an empty slice rather than an
// error; callers compose the public surface with GetPublicApis or
// pass an explicit module list to GetApis.
//
// # What this package does NOT do
//
// JSON-RPC wire-format encoding, request dispatch, transport
// management (HTTP / WebSocket / IPC / stdio), and subscription
// machinery all live in rpc/server. The per-domain request handlers
// live in rpc/api and its subpackages. This package is purely the
// configuration step that tells the server which services to bind
// under which namespaces.
package rpc
