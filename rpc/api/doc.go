// Package api implements the public JSON-RPC namespaces (ledger,
// stats, plus the embedded and subscribe sub-packages).
//
// # Overview
//
// Each "namespace" is a Go struct whose exported methods become
// JSON-RPC endpoints once registered with
// [github.com/zenon-network/go-zenon/rpc/server]. The package exposes
// read access to chain and consensus state plus the write path for
// user-submitted account blocks.
//
// # Namespaces
//
//   - [LedgerApi] — "ledger" namespace. Read/write access to account
//     blocks and momentums (heights, hashes, paged lookups, balance
//     queries, transaction publication).
//   - [StatsApi] — "stats" namespace. Process / OS / network / sync
//     introspection. Used by operators and dashboards.
//
// Sub-packages:
//
//   - [github.com/zenon-network/go-zenon/rpc/api/embedded] — RPC
//     wrappers for every embedded contract (token, pillar, plasma,
//     stake, sentinel, swap, spork, accelerator, htlc, bridge,
//     liquidity).
//   - [github.com/zenon-network/go-zenon/rpc/api/subscribe] — pub/sub
//     surface for ledger events.
//
// # Conventions
//
// All paginated endpoints accept a (pageIndex, pageSize) pair and
// reject pageSize > [RpcMaxPageSize]=1024 with [ErrPageSizeParamTooBig].
// Range-style endpoints take (height, count) and reject count >
// [RpcMaxCountSize]=1024 with [ErrCountParamTooBig]. height==0 is
// invalid and surfaces as [ErrHeightParamIsZero].
//
// Wire types (suffix -Marshal) decouple JSON serialisation from the
// in-memory shape: big.Int values render as decimal strings rather
// than JSON numbers to avoid precision loss in browser clients.
//
// # Concurrency
//
// API instances are stateless after construction. The underlying
// [zenon.Zenon] / [chain.Chain] objects are shared and goroutine-safe.
//
// # Generated Files
//
// None. Files are Zenon-specific (no upstream header).
//
// # Related Packages
//
//   - [github.com/zenon-network/go-zenon/rpc] — wires modules to
//     transports.
//   - [github.com/zenon-network/go-zenon/rpc/server] — JSON-RPC
//     framework (ported from go-ethereum).
//   - [github.com/zenon-network/go-zenon/zenon] — orchestration
//     interface API methods reach into.
//   - [github.com/zenon-network/go-zenon/chain] — block / momentum
//     stores backing [LedgerApi].
package api
