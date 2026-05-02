// Package abi encodes and decodes embedded-contract method calls in a format
// compatible with the Solidity ABI.
//
// # Overview
//
// abi mirrors the encoding rules used by EVM tooling so that contract clients
// can pack call data with familiar libraries. It supports the type set used
// by Zenon's embedded contracts (uint variants, address, bytes, fixed arrays,
// strings) and provides method-selector and event-signature hashing.
//
// Per-package documentation is being filled in incrementally. See
// docs/STYLE.md for the full template applied in subsequent PRs.
package abi
