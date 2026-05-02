// Package zenon is the top-level orchestrator of the Network of
// Momentum core.
//
// # Overview
//
// zenon constructs and sequences the major subsystems —
// [github.com/zenon-network/go-zenon/chain],
// [github.com/zenon-network/go-zenon/consensus],
// [github.com/zenon-network/go-zenon/verifier],
// [github.com/zenon-network/go-zenon/pillar],
// [github.com/zenon-network/go-zenon/protocol], and the
// [github.com/zenon-network/go-zenon/rpc/api/subscribe] dispatcher
// — and exposes them as a single [Zenon] facade. Every higher-level
// component (RPC handlers, the node lifecycle, embedded-contract
// tests) takes a [Zenon] handle rather than reaching for individual
// subsystems.
//
// # Initialization Order
//
// [Zenon.Init] runs each subsystem's Init in order:
//
//  1. chain — open stores, validate genesis compatibility
//  2. consensus — election manager, points system
//  3. event printer — chain listener that logs momentum inserts
//  4. subscription server — RPC pub/sub dispatcher
//  5. pillar — block producer (no-op until coinbase is configured)
//
// [Zenon.Start] then calls each Start in the same order, with
// protocol started last (it would otherwise begin requesting
// blocks before the verifier is wired). [Zenon.Stop] unwinds in
// reverse: protocol → pillar → subscribe → printer → consensus →
// chain → leveldb close.
//
// # Coinbase
//
// If [Config.ProducingKeyPair] is non-nil at construction time, the
// embedded pillar manager is initialised in producer mode and will
// attempt to mint momentums when elected. A nil keypair runs the
// node as a non-producing validator.
//
// # Generated Files
//
// None. Files are Zenon-specific (no upstream header).
//
// # Related Packages
//
//   - [github.com/zenon-network/go-zenon/zenon/mock] — drop-in test
//     harness with the same [Zenon] surface backed by mocked stores.
//   - [github.com/zenon-network/go-zenon/node] — owns the [Zenon]
//     instance plus the p2p server, RPC stack, and process
//     lifecycle.
//   - [github.com/zenon-network/go-zenon/app] — CLI driver that
//     constructs [Config] from flags and instantiates the node.
package zenon
