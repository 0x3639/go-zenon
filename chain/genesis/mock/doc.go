// Package g provides a programmable mock genesis used by the
// embedded-contract test suite.
//
// # Overview
//
// Tests build a genesis with arbitrary balances, pillar registrations,
// stake entries, and spork states by composing this package's deterministic
// fixtures, then hand the result to
// [github.com/zenon-network/go-zenon/zenon/mock] to spin up an in-memory
// node initialized at that synthetic state.
//
// The package name is `g` (a single-letter package name) to keep call
// sites short — tests refer to the keypairs and pillar names as
// `g.Pillar1`, `g.User1`, and so on. The single-letter name is unusual
// for production code but acceptable here because every consumer is a
// test file.
//
// # Key Fixtures
//
//   - Pillar1..Pillar8 — deterministic pillar keypairs.
//   - User1..User10 — deterministic user keypairs.
//   - Spork — the spork-controlling keypair.
//   - Pillar1Name..Pillar8Name — fixed display names matched to the
//     pillar keypairs.
//   - PillarKeys, AllKeyPairs — convenience slices for iteration.
//   - EmbeddedGenesis — the canonical [genesis.GenesisConfig] every
//     test starts from.
//   - genesisTimestamp, Zexp — anchor constants for time and
//     balance arithmetic.
//
// # Related Packages
//
//   - [github.com/zenon-network/go-zenon/chain/genesis] — the
//     production genesis loader; this package's [EmbeddedGenesis]
//     conforms to its [genesis.GenesisConfig] schema.
//   - [github.com/zenon-network/go-zenon/zenon/mock] — consumes this
//     genesis to construct an in-memory test node.
//   - [github.com/zenon-network/go-zenon/wallet] — supplies the
//     deterministic key derivation that produces the fixtures.
package g
