// Package storage persists consensus state — election results, points,
// epoch snapshots — to a [github.com/zenon-network/go-zenon/common/db]
// LevelDB instance with hot LRU caches in front.
//
// # Overview
//
// storage holds two record kinds:
//
//   - ElectionData ([election_data.go]) — cached output of one
//     election keyed by the proof block's hash. Lets the election
//     manager skip re-running the algorithm on repeat lookups.
//   - Point ([point.go]) — per-tick performance summary. Comes in
//     two granularities (period vs epoch) namespaced by
//     [PrefixPeriodPoint] and [PrefixEpochPoint] respectively.
//
// Cache sizes are configured by the caller of [NewConsensusDB] —
// [github.com/zenon-network/go-zenon/consensus.NewConsensus] picks a
// size that holds roughly one week of points at the chain's
// configured block-time and node-count.
//
// # Key Concepts
//
//   - DB — the storage handle. Composes a [common/db.DB] backing
//     store with per-feature LRU caches.
//   - Point — `(PrevHash, EndHash, Pillars, TotalWeight)`. The
//     hash pair brackets the momentum range the point covers; the
//     pillars map and total weight are aggregable via
//     [Point.LeftAppend].
//   - ElectionData — `(Producers, Delegations)`. Producers is the
//     ordered slate; delegations is the snapshot the slate was
//     elected against.
//
// # Generated Files
//
// Two protobuf-generated files (`election_data.pb.go`, `point.pb.go`)
// implement the wire format. They are not manually documented and
// carry the standard generated-file marker.
//
// # Related Packages
//
//   - [github.com/zenon-network/go-zenon/consensus] — the only
//     consumer.
//   - [github.com/zenon-network/go-zenon/common/db] — backing
//     versioned LevelDB layer.
package storage
