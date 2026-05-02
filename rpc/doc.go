// Package rpc is the entry point for the JSON-RPC server.
//
// # Overview
//
// rpc wires the transport layer in
// [github.com/zenon-network/go-zenon/rpc/server] to the public API surface in
// [github.com/zenon-network/go-zenon/rpc/api]. Configuration controls which
// transports (HTTP, WS, IPC) are enabled and which API namespaces are exposed
// on each.
//
// Per-package documentation is being filled in incrementally. See
// docs/STYLE.md for the full template applied in subsequent PRs.
package rpc
