// Package types defines the foundational identifier types used everywhere on
// the chain: addresses, hashes, hash/height pairs, token standards, account
// headers, and the spork registry.
//
// # Overview
//
// types is the deepest dependency in the codebase — every other package
// imports it directly or transitively. It owns the canonical encodings
// (bech32 for [Address] and [ZenonTokenStandard], hex for [Hash]) and the
// fixed-size binary layouts that protobuf wrappers ([AddressProto],
// [HashProto], [HashHeightProto], [AccountHeaderProto]) marshal to and from
// for on-chain storage and peer-to-peer transport.
//
// # Key Concepts
//
//   - Address — 20-byte account identifier; one type byte ([UserAddrByte] or
//     [ContractAddrByte]) plus 19 payload bytes. See [PubKeyToAddress] for
//     the user-account derivation rule and [EmbeddedContracts] for the
//     enumerated system addresses.
//   - Hash — 32-byte SHA3-256 digest. See [NewHash].
//   - HashHeight — `(hash, height)` pair locating any block on either ledger.
//     See [github.com/zenon-network/go-zenon/ARCHITECTURE.md] for the
//     dual-ledger model.
//   - AccountHeader — `(address, hash, height)` triple identifying a block
//     within a specific account chain.
//   - ZenonTokenStandard (ZTS) — 10-byte token identifier, the analog of an
//     ERC-20 contract address.
//   - ImplementedSpork — binary-side handle for a protocol upgrade gated by
//     the spork contract. The constants in this file (
//     [AcceleratorSpork], [HtlcSpork], [BridgeAndLiquiditySpork]) enumerate
//     the upgrades this binary recognizes.
//   - PillarDelegation — consensus-layer summary of a registered pillar's
//     name, coinbase, and aggregated stake weight.
//
// # Usage
//
// Construct identifiers with the typed constructors rather than copying raw
// bytes; the constructors validate length and panic on programmer error so
// invariants stay enforced at the boundary:
//
//   - [ParseAddress] / [ParseAddressPanic] — bech32 string to [Address].
//   - [PubKeyToAddress] — Ed25519 public key to user [Address].
//   - [NewHash] — bytes to [Hash] via the canonical hash function.
//   - [HexToHash] / [HexToHashPanic] — hex string to [Hash].
//   - [NewZenonTokenStandard] — derive a fresh ZTS from issuance bytes.
//
// Convert to/from protobuf with the `Proto`/`DeProto*` family for storage
// and wire formats; use [encoding.TextMarshaler]/[encoding.TextUnmarshaler]
// (already implemented) for transparent JSON encoding.
//
// # Related Packages
//
//   - [github.com/zenon-network/go-zenon/common] — provides the byte and
//     time helpers ([common.JoinBytes], [common.Uint64ToBytes]) used to
//     compose canonical key forms.
//   - [github.com/zenon-network/go-zenon/common/crypto] — supplies the
//     SHA3-256 hash primitive consumed by [NewHash] and
//     [NewZenonTokenStandard].
//   - [github.com/zenon-network/go-zenon/chain/nom] — uses these types to
//     define [chain/nom.AccountBlock] and [chain/nom.Momentum].
//   - [github.com/zenon-network/go-zenon/vm/embedded] — dispatches on the
//     [EmbeddedContracts] addresses defined here.
package types
