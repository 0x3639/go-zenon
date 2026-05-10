// Package p2p manages peer connections and message transport at the wire
// layer.
//
// # Overview
//
// p2p is a port of the go-ethereum devp2p stack, lightly adapted for Zenon
// (notably the [Net] config type, [DefaultSeeders] list, and listening
// port [DefaultListenPort]=35995). It owns:
//
//   - [Server] — accepts inbound connections, dials outbound peers, and
//     drives a single event loop in [Server.run] that gates the per-peer
//     handshake pipeline.
//   - [Peer] — one per established connection, multiplexing the registered
//     subprotocols (matched by [Cap]) over a single [MsgReadWriter].
//   - The RLPx transport (rlpx.go) — encrypted, MAC'd framing with a
//     two-step handshake (encryption then protocol).
//   - The dialer (dial.go) — schedules outbound dial tasks and bootstraps
//     discovery via [github.com/zenon-network/go-zenon/p2p/discover].
//
// Higher-level routing (block / transaction relay, sync) belongs to
// [github.com/zenon-network/go-zenon/protocol], which registers itself as
// a [Protocol] on the server.
//
// # Connection Lifecycle
//
// Each connection passes through three stages tracked via the unexported
// [connFlag] bits:
//
//  1. Encryption handshake — RLPx auth (initiator) / auth-resp (receiver)
//     produces shared AES + MAC secrets. After this, the remote node ID
//     is known but not yet verified.
//  2. Protocol handshake — exchange [protoHandshake] containing capability
//     advertisements ([Cap]). Mismatched protocol versions disconnect with
//     [DiscIncompatibleVersion].
//  3. Run — [Peer.run] launches read/write/ping loops and dispatches into
//     each matched subprotocol's [Protocol.Run] callback.
//
// The two checkpoints (post-encryption, post-protocol) call back into
// [Server.run] so the central goroutine can enforce admission rules
// (max-peer cap, trusted-list bypass, dedup against existing peer set).
//
// # Concurrency
//
// One central goroutine in [Server.run] is the single writer of the
// peer map. All public Server methods that read peer state ([Server.Peers],
// [Server.PeerCount]) post a closure on `peerOp` and wait for it to
// execute on the loop goroutine. Per-connection goroutines:
//
//   - listenLoop — accepts inbound connections (rate-limited by a
//     semaphore of [maxAcceptConns]=50 slots).
//   - One setupConn goroutine per pending connection, ending when both
//     handshakes complete (or the connection is rejected).
//   - One [Peer.run] goroutine per established peer, plus three child
//     goroutines per peer (readLoop, pingLoop, and one writer per
//     subprotocol).
//
// [MsgReadWriter] implementations must be safe for concurrent
// ReadMsg/WriteMsg from different goroutines; [rlpx] and the netWrapper
// achieve this with separate read/write mutexes.
//
// # Tunables
//
// Defaults are conservative and exposed via the [Net] config type:
//
//   - [DefaultMaxPeers]=60, [DefaultMinConnectedPeers]=16 (the p2p
//     dial target), [DefaultMaxPendingPeers]=10. [DefaultMinPeers]=8
//     is consumed by the protocol layer's sync gate, not by the p2p
//     dialer — see protocol/handler.go.
//   - [defaultDialTimeout]=15s, [refreshPeersInterval]=30s,
//     [staticPeerCheckInterval]=15s
//   - [frameReadTimeout]=30s (effective idle timeout),
//     [frameWriteTimeout]=20s
//   - [handshakeTimeout]=5s for the combined enc + proto handshake
//   - [pingInterval]=15s heartbeats keep idle connections alive
//
// # Generated Files
//
// None. The .go files in this package carry the original go-ethereum
// LGPL-3.0+ headers; config.go is Zenon-specific (no upstream header).
//
// # Related Packages
//
//   - [github.com/zenon-network/go-zenon/p2p/discover] — Kademlia-style
//     UDP node discovery; supplies the seed pool and random-lookup
//     candidates the dialer consumes.
//   - [github.com/zenon-network/go-zenon/p2p/nat] — NAT traversal
//     (UPnP, NAT-PMP) used to map the listening port.
//   - [github.com/zenon-network/go-zenon/protocol] — the Zenon
//     wire-protocol manager, registered here as a [Protocol].
//   - [github.com/zenon-network/go-zenon/node] — owns the [Server]
//     instance and wires it into the application lifecycle.
package p2p
