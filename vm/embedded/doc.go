// Package embedded is the dispatcher for Zenon's built-in system contracts.
//
// # Overview
//
// embedded maps a system address (Pillar, Sentinel, Stake, Token, Plasma,
// Spork, Swap, Accelerator, HTLC, Bridge, Liquidity) to its current
// implementation, taking into account active sporks. Each contract method
// implements three hooks: GetPlasma (cost), ValidateSendBlock (preconditions
// on the inbound send), and ReceiveBlock (the state transition).
//
// Per-package documentation is being filled in incrementally. See
// docs/STYLE.md for the full template applied in subsequent PRs.
package embedded
