// Package pow generates the proof-of-work nonces that satisfy the plasma
// difficulty for an account block.
//
// # Overview
//
// Plasma may be paid either by fusing QSR (steady yield) or by burning a small
// proof of work per block; pow implements the second path. Callers supply a
// seed derived from the block contents and a target difficulty, and the
// package returns an 8-byte nonce whose hash satisfies the difficulty target.
//
// Per-package documentation is being filled in incrementally. See
// docs/STYLE.md for the full template applied in subsequent PRs.
package pow
