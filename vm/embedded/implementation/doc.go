// Package implementation contains the executable behavior for each embedded
// contract.
//
// # Overview
//
// One file per contract (pillar, sentinel, stake, token, plasma, spork, swap,
// accelerator, htlc, bridge, liquidity). Each method implements `GetPlasma`,
// `ValidateSendBlock`, and `ReceiveBlock`, and writes its outputs through the
// supplied [github.com/zenon-network/go-zenon/vm/vm_context] handle.
//
// Per-package documentation is being filled in incrementally. See
// docs/STYLE.md for the full template applied in subsequent PRs.
package implementation
