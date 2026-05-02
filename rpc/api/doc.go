// Package api implements the public JSON-RPC namespaces (ledger, network,
// utility, stats, subscribe).
//
// # Overview
//
// Each namespace is a Go struct whose exported methods become RPC endpoints
// after registration with [github.com/zenon-network/go-zenon/rpc/server].
// api exposes read access to chain and consensus state plus write paths for
// user-submitted account blocks.
//
// Per-package documentation is being filled in incrementally. See
// docs/STYLE.md for the full template applied in subsequent PRs.
package api
