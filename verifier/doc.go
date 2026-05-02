// Package verifier validates account blocks and momentums against consensus
// rules before [github.com/zenon-network/go-zenon/chain] inserts them.
//
// # Overview
//
// verifier provides two orthogonal layers: stateless checks (signature, hash,
// height linkage, plasma sufficiency) and stateful checks (account-chain
// continuity, send/receive matching, contract preconditions, pillar
// eligibility for a momentum's tick). Both must pass before insertion.
//
// Per-package documentation is being filled in incrementally. See
// docs/STYLE.md for the full template applied in subsequent PRs.
package verifier
