// Package downloader bulk-syncs the chain when a node lags behind
// its peers.
//
// # Overview
//
// downloader implements the manual full chain synchronisation
// protocol — given a peer announcement of a longer / heavier
// chain, the downloader walks back to a common ancestor, requests
// the missing blocks in parallel from active peers, validates
// each batch via cross-checks against neighbouring peers, and
// inserts the result into the local chain through the
// [chainInsertFn] callback.
//
// # Phases
//
//   - Hash walk: starting from a peer's claimed head, the
//     downloader requests up to [MaxHashFetch] predecessor hashes
//     repeatedly until it finds a hash already in the local
//     chain (the common ancestor).
//   - Block fetch: hashes between the ancestor and the head go
//     into the [queue], which schedules them across peers (one
//     fetch request per peer at a time, bounded by
//     [MaxBlockFetch]).
//   - Cross-check: as blocks come back, the downloader spot-checks
//     them against neighbours so a single misbehaving peer cannot
//     poison the import.
//   - Insert: validated, contiguous batches of up to
//     [maxBlockProcess] blocks are pushed into the chain.
//
// # Tuning
//
// All knobs are vars (not consts) so tests can shrink them.
// Production defaults: [MaxHashFetch] = [MinHashFetch] = 512,
// [MaxBlockFetch] = 128, [maxQueuedHashes] = 256K
// (DOS protection), [hashTTL] = 5s, [blockHardTTL] = 9s.
//
// # Concurrency
//
// The [Downloader] runs three internal goroutines (hash fetcher,
// block fetcher, processor) coordinated through channels and the
// [queue] scheduler. Public methods are safe for concurrent use.
//
// # Generated Files
//
// None. The .go files in this package carry the original
// go-ethereum LGPL-3.0+ headers.
//
// # Related Packages
//
//   - [github.com/zenon-network/go-zenon/protocol] — the parent
//     manager that wires the downloader to the wire-protocol
//     loop.
//   - [github.com/zenon-network/go-zenon/protocol/fetcher] —
//     handles single-block fetches; the downloader handles bulk
//     catch-up.
//   - [github.com/zenon-network/go-zenon/chain/nom] — block model
//     the downloader inserts.
package downloader
