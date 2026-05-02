// Package protocol bridges the chain layer to the P2P network.
//
// # Overview
//
// protocol owns the ChainBridge that wires
// [github.com/zenon-network/go-zenon/chain] reads/writes to peer-facing
// requests, the Broadcaster that announces newly produced blocks, and the
// ProtocolManager that arbitrates which peers feed the chain.
//
// Per-package documentation is being filled in incrementally. See
// docs/STYLE.md for the full template applied in subsequent PRs.
package protocol
