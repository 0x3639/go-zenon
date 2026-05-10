// Package crypto wraps the hash primitives used throughout the chain,
// VM, and wallet layers.
//
// # Overview
//
// crypto exposes three hash functions and nothing else:
//
//   - [Hash] — SHA3-256 (variadic; concatenates its arguments before
//     hashing). This is the canonical hash used by [chain/nom] for block
//     hashes, by the embedded contracts for storage keys, and by the VM
//     for changes-hash computation.
//   - [HashSHA256] — SHA-256, used for ABI-style preimages where a
//     widely-implemented standard digest is required.
//   - [Keccak256] — Legacy Keccak256 (Ethereum-compatible), used by the
//     bridge embedded contract when computing pre-EIP-152 EVM hashes.
//
// Ed25519 signing/verification lives in
// [github.com/zenon-network/go-zenon/wallet]; address derivation lives
// in [github.com/zenon-network/go-zenon/common/types].
//
// # Related Packages
//
//   - [github.com/zenon-network/go-zenon/wallet] — Ed25519 keypair
//     generation, signing, and verification.
//   - [github.com/zenon-network/go-zenon/common/types] — address and
//     hash value types, plus public-key-to-address derivation.
package crypto
