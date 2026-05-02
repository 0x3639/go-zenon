// Package embedded exposes per-contract RPC endpoints for the embedded
// contracts.
//
// # Overview
//
// One namespace per contract (pillar, sentinel, stake, token, plasma, spork,
// swap, accelerator, htlc, bridge, liquidity), each providing read accessors
// over the contract's storage. Send-side calls go through the generic
// account-block submission path in [github.com/zenon-network/go-zenon/rpc/api].
//
// Per-package documentation is being filled in incrementally. See
// docs/STYLE.md for the full template applied in subsequent PRs.
package embedded
