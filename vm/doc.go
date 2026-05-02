// Package vm executes account blocks: it routes them to the right
// embedded contract, enforces plasma costs, and produces the
// resulting state patch.
//
// # Overview
//
// vm exposes a [Supervisor] that, given a
// [chain/nom.AccountBlock], constructs an execution context, invokes
// the appropriate [vm/embedded] contract method (or performs a plain
// user transfer), and returns the resulting block transaction to the
// chain layer for insertion. A second machine ([MomentumVM]) commits
// a momentum's worth of (already-validated) account-block patches at
// once.
//
// # Key Concepts
//
//   - Supervisor — the public entry point. Owns chain, consensus,
//     verifier handles and exposes [Supervisor.ApplyBlock],
//     [Supervisor.ApplyMomentum], [Supervisor.GenerateFromTemplate],
//     [Supervisor.GenerateAutoReceive], [Supervisor.GenerateMomentum],
//     and [Supervisor.GenerateGenesisMomentum].
//   - VM — per-account-block executor. Dispatches by block type:
//     send, user receive, contract receive (with descendant blocks +
//     rollback). The contract-receive flow auto-generates the receive
//     block from the originating send, refunding tokens on failure.
//   - MomentumVM — per-momentum executor. Walks the momentum content
//     and admits each (header, patch) pair into the momentum view.
//   - SignFunc — pluggable signing callback used by
//     [Supervisor.GenerateFromTemplate] / [Supervisor.GenerateMomentum].
//   - Plasma — per-block resource cost. Derived from fused QSR
//     ([FussedAmountToPlasma]), PoW difficulty
//     ([DifficultyToPlasma]), or the contract method's
//     [embedded.Method.GetPlasma]. The verifier rejects blocks below
//     [GetBasePlasmaForAccountBlock].
//
// # Concurrency
//
// The supervisor is goroutine-safe: every method constructs fresh
// contexts and the underlying handles (chain, verifier, consensus)
// are themselves safe for concurrent use. Per-block VM and
// per-momentum MomentumVM instances are short-lived and not shared.
//
// All Apply / Generate entry points wrap execution in a recover
// guard: a VM panic becomes [constants.ErrVmRunPanic] for the
// caller rather than crashing the node.
//
// # Related Packages
//
//   - [github.com/zenon-network/go-zenon/chain] — supplies the chain
//     handle and the destination for the produced transactions.
//   - [github.com/zenon-network/go-zenon/consensus] — supplies
//     [consensus.Consensus.FixedPillarReader] to the per-block
//     execution context.
//   - [github.com/zenon-network/go-zenon/verifier] — runs every
//     block and momentum through validation before execution.
//   - [github.com/zenon-network/go-zenon/vm/embedded] — dispatches
//     contract method calls during block execution.
//   - [github.com/zenon-network/go-zenon/vm/vm_context] — defines
//     the per-block and per-momentum execution contexts.
//   - [github.com/zenon-network/go-zenon/vm/constants] — plasma
//     limits and the embedded-contract sentinel errors.
package vm
