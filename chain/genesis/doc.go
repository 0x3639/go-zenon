// Package genesis provides the embedded alphanet genesis configuration and
// chain-identifier compatibility checks.
//
// # Overview
//
// genesis seeds the initial state on a fresh data directory: the genesis
// momentum, the embedded-contract receive blocks that mint initial token
// supplies, and the chain identifier that distinguishes mainnet from local
// test networks. On every node boot it cross-checks the on-disk chain against
// the embedded genesis to detect database mismatches.
//
// Per-package documentation is being filled in incrementally. See
// docs/STYLE.md for the full template applied in subsequent PRs.
package genesis
