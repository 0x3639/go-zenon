// Package wallet manages the on-disk keystore and signing keys for znnd.
//
// # Overview
//
// wallet handles encrypted keyfiles, key derivation from BIP-39 mnemonics, and
// the signing primitives used by [github.com/zenon-network/go-zenon/pillar]
// (coinbase) and the RPC layer (user-submitted transactions).
//
// Per-package documentation is being filled in incrementally. See
// docs/STYLE.md for the full template applied in subsequent PRs.
package wallet
