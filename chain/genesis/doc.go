// Package genesis provides the embedded alphanet genesis configuration,
// the JSON-shaped runtime config loader, and the consistency checks that
// bind the two together.
//
// # Overview
//
// genesis is the [github.com/zenon-network/go-zenon/chain/store.Genesis]
// implementation. It seeds the initial state on a fresh data directory
// — the genesis momentum, the embedded-contract receive blocks that
// mint initial token supplies and pillar registrations, and the chain
// identifier that distinguishes one network from another. On every node
// boot the chain layer cross-checks the on-disk frontier against the
// embedded genesis, refusing to start when they disagree.
//
// Genesis assembly runs entirely in-memory at boot time via
// [account_block.go] (per-contract seeding) and [momentum.go]
// (assembly into a height-1 momentum). The genesis momentum is the only
// momentum that does not flow through
// [github.com/zenon-network/go-zenon/verifier].
//
// # Key Concepts
//
//   - GenesisConfig — the JSON-shaped genesis description. Includes
//     per-embedded-contract configs (PillarConfig, TokenConfig,
//     PlasmaConfig, SwapConfig, optional SporkConfig) and the catalog
//     of genesis-receive blocks that seed user balances.
//   - Embedded genesis — the canonical alphanet [GenesisConfig], baked
//     into the binary via the auto-generated [embeddedGenesisStr] in
//     embedded_genesis_string.go. Surfaced by
//     [MakeEmbeddedGenesisConfig].
//   - File genesis — a runtime override loaded by
//     [ReadGenesisConfigFromFile]; used by test networks and by node
//     operators who want to spin up custom chains.
//   - Consistency checks ([shared_tests.go]) — rules every genesis
//     must satisfy: token-supply totals, pillar balance totals,
//     plasma-fusion totals, swap entry well-formedness, required-field
//     presence.
//
// # Usage
//
// Most binaries use the embedded genesis:
//
//	g, err := genesis.MakeEmbeddedGenesisConfig()
//
// Tests and custom chains load from disk:
//
//	g, err := genesis.ReadGenesisConfigFromFile("testnet.json")
//
// # Related Packages
//
//   - [github.com/zenon-network/go-zenon/chain/store] — interface this
//     package implements.
//   - [github.com/zenon-network/go-zenon/chain] — orchestrates the
//     genesis store alongside the live momentum chain.
//   - [github.com/zenon-network/go-zenon/chain/genesis/mock] — the
//     test-only fork of this package that tests use to compose
//     synthetic genesis configurations.
//   - [github.com/zenon-network/go-zenon/vm/embedded/definition] —
//     supplies the per-contract record types (PillarInfo, TokenInfo,
//     FusionInfo, SwapAssets, Spork, etc.) that seeding writes.
package genesis
