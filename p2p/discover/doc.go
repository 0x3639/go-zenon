// Package discover finds peers on the network using a Kademlia-style
// distributed hash table.
//
// # Overview
//
// discover keeps a routing table of known nodes keyed by node ID, refreshes
// it through periodic find-node queries, and exposes the result to the p2p
// server so it can dial new peers as needed.
//
// Per-package documentation is being filled in incrementally. See
// docs/STYLE.md for the full template applied in subsequent PRs.
package discover
