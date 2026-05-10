// Package fetcher retrieves and validates individual blocks
// announced by peers.
//
// # Overview
//
// The fetcher is the announce-driven counterpart to the
// [github.com/zenon-network/go-zenon/protocol/downloader]:
// optimized for the latency-sensitive case where a peer has just
// produced a single block and the local node should fetch it
// promptly to keep up with the chain head.
//
// On a peer announcement ([NewBlockHashesMsg]), the fetcher
// records the announce, waits a brief window
// ([arriveTimeout] = 500ms) in case the block arrives via
// natural broadcast, then explicitly requests it. The
// [blockValidatorFn] callback the protocol layer wires in is
// currently a stub that returns nil (see protocol/handler.go's
// `validator := func(...) error { return nil }`); real consensus
// validation happens later when the chain layer applies the block
// via the supervisor. Inserted via the [chainInsertFn] callback,
// with per-peer hash and block caps ([hashLimit], [blockLimit])
// for DOS protection.
//
// # Concurrency
//
// One goroutine drives the [Fetcher.loop] state machine; public
// methods enqueue work via channels.
//
// # Generated Files
//
// None. fetcher.go carries the original go-ethereum LGPL-3.0+
// header.
//
// # Related Packages
//
//   - [github.com/zenon-network/go-zenon/protocol] — the parent
//     manager that wires the fetcher to the wire-protocol loop.
//   - [github.com/zenon-network/go-zenon/protocol/downloader] —
//     handles bulk catch-up; the fetcher handles individual
//     blocks.
//   - [github.com/zenon-network/go-zenon/chain/nom] — block model
//     the fetcher inserts.
package fetcher
