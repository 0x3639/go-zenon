// Package discover finds peers on the network using a Kademlia-style
// distributed hash table.
//
// # Overview
//
// discover is a port of the go-ethereum devp2p discovery v4 protocol.
// Each node maintains a routing [Table] of known peers keyed by node ID
// (a 512-bit secp256k1 public key), refreshes it through periodic
// FindNode lookups, and exposes the result so the
// [github.com/zenon-network/go-zenon/p2p] dialer can pull dial
// candidates.
//
// The on-the-wire packets ([ping], [pong], [findnode], [neighbors])
// travel over UDP, signed with the local secp256k1 key so the sender's
// node ID can be recovered. Each packet carries an absolute
// [expiration]=20s timestamp to bound replay windows.
//
// # Routing Table
//
// The table is split into [nBuckets]=257 k-buckets indexed by the log
// distance ([logdist]) between [Table.self] and the candidate's
// node-ID hash. Each bucket holds up to [bucketSize]=16 entries,
// ordered by recency of activity. Insertion uses the standard
// "ping the oldest entry; if it responds, the newcomer is dropped"
// LRU-eviction rule (see [Table.pingreplace]).
//
// # Bonding
//
// Before answering a FindNode request, both sides must complete a
// ping/pong exchange — the "bond". This protects against IP-spoofing
// amplification attacks (a forged FindNode would otherwise yield a
// large neighbors packet to the spoofed victim). The bonding workflow
// is implemented in [Table.bond] / [Table.pingpong]; concurrency is
// capped via [maxBondingPingPongs]=16 slots.
//
// # Lookups
//
// [Table.Lookup] performs the standard Kademlia search: pick the
// [alpha]=3 closest known nodes to the target, ask them for their
// closest neighbors, fold the responses into a result set sorted by
// distance, and repeat until no closer node is returned. Findnode
// failures are tracked in [nodeDB] and after [maxFindnodeFailures]=5
// the entry is evicted via [Table.del].
//
// # Persistence
//
// Known nodes survive restarts in a leveldb-backed [nodeDB]. The
// schema is documented in database.go (lastping, lastpong, findfail
// counters per node-ID). Entries unseen for [nodeDBNodeExpiration]=24h
// are dropped by the periodic expirer.
//
// # Concurrency
//
// One UDP read goroutine ([udp.readLoop]) decodes inbound packets;
// one event-loop goroutine ([udp.loop]) dispatches them against a
// pending-reply queue keyed on (NodeID, packet-type). Lookups,
// bonding, and refreshes spawn additional goroutines tracked by the
// [Table.wg] WaitGroup so [Table.Close] can drain them.
//
// # Generated Files
//
// None. The .go files in this package carry the original go-ethereum
// LGPL-3.0+ headers.
//
// # Related Packages
//
//   - [github.com/zenon-network/go-zenon/p2p] — consumes this package
//     via the discoverTable interface to drive outbound dialing.
//   - [github.com/zenon-network/go-zenon/p2p/nat] — used by
//     [ListenUDP] to map the discovery UDP port through NAT.
package discover
