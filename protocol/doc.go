// Package protocol implements the eth-derived wire protocol that
// glues the chain layer to the P2P network.
//
// # Overview
//
// protocol owns three things:
//
//   - The [ProtocolManager] (handler.go) running per-peer message
//     loops on top of [github.com/zenon-network/go-zenon/p2p].
//     Implements the status handshake, broadcasts new blocks and
//     transactions, and dispatches incoming messages to the
//     downloader, fetcher, and chain bridge.
//   - The [Broadcaster] (broadcaster.go) — the local-side surface
//     for self-produced blocks. The pillar producer and the RPC
//     signing path call here; the broadcaster commits via the
//     chain insert lock and forwards to the protocol manager for
//     peer announcement.
//   - The [ChainBridge] (chain_bridge.go) — adapts the
//     chain / consensus / verifier / VM-supervisor stack to the
//     peer-facing API, handling both single-block applies and
//     contiguous-batch chain inserts (with bounded reorg).
//
// Bulk catch-up runs through the [protocol/downloader]
// sub-package; single-block on-announcement fetches go through
// [protocol/fetcher].
//
// # Wire Format
//
// The protocol-message codes live in protocol.go (StatusMsg,
// NewBlockHashesMsg, TxMsg, GetBlockHashes/BlockHashes,
// GetBlocks/Blocks, NewBlockMsg, GetBlockHashesFromNumberMsg).
// The protocol-version (61) and message length (9) are inherited
// from the eth-61 protocol family that Zenon's p2p layer was
// derived from.
//
// # Peer Suppression
//
// Each [peer] keeps an LRU set of recently-seen transaction and
// block hashes ([maxKnownTxs] / [maxKnownBlocks]) so the same
// announcement is not re-broadcast in a loop. The bounds also
// cap per-peer memory consumption, preventing a flood of unique
// announcements from exhausting node memory.
//
// # Concurrency
//
// Each peer runs its message loop in its own goroutine; the
// manager owns one [ProtocolManager.txsyncLoop] goroutine for
// pending-transaction sync to new peers, and the downloader /
// fetcher each run their own scheduler goroutines.
//
// # Related Packages
//
//   - [github.com/zenon-network/go-zenon/p2p] — supplies the
//     transport-layer Peer / MsgReadWriter primitives.
//   - [github.com/zenon-network/go-zenon/chain] — chain reads
//     and writes through [ChainBridge].
//   - [github.com/zenon-network/go-zenon/verifier] — validates
//     inbound blocks before insertion.
//   - [github.com/zenon-network/go-zenon/vm] — applies account
//     blocks and momentums.
//   - [github.com/zenon-network/go-zenon/protocol/downloader] —
//     bulk-sync chain catch-up.
//   - [github.com/zenon-network/go-zenon/protocol/fetcher] —
//     single-block fetch on announcement.
//
// # License
//
// The handler.go, peer.go, protocol.go, and sync.go files
// (along with the downloader and fetcher sub-packages) carry
// the original go-ethereum LGPL-3.0+ headers. New Zenon-specific
// files (broadcaster.go, chain_bridge.go, interfaces.go) are
// covered by this project's license.
package protocol
