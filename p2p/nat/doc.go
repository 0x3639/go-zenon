// Package nat handles NAT traversal so a node behind a home router can be
// reached by external peers.
//
// # Overview
//
// nat negotiates port mappings via UPnP or NAT-PMP, refreshes them
// periodically, and exposes the discovered external address to
// [github.com/zenon-network/go-zenon/p2p].
//
// Per-package documentation is being filled in incrementally. See
// docs/STYLE.md for the full template applied in subsequent PRs.
package nat
