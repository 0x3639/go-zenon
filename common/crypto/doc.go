// Package crypto wraps the hashing and Ed25519 signature primitives used
// throughout the chain, VM, and wallet layers.
//
// # Overview
//
// crypto provides BLAKE2b hashing, Ed25519 sign/verify, and helpers for
// deriving addresses from public keys.
//
// Per-package documentation is being filled in incrementally. See
// docs/STYLE.md for the full template applied in subsequent PRs.
package crypto
