// Package p2p manages peer connections and message transport at the wire
// layer.
//
// # Overview
//
// p2p owns the Server that accepts and dials peers, the per-peer goroutines
// that read and write framed messages, and the configurable sets of static,
// bootstrap, and trusted nodes. Higher-level routing belongs to
// [github.com/zenon-network/go-zenon/protocol].
//
// Per-package documentation is being filled in incrementally. See
// docs/STYLE.md for the full template applied in subsequent PRs.
package p2p
