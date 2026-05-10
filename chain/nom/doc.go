// Package nom defines the on-chain block model: account blocks, momentums,
// momentum content, the per-block plasma nonce, and the database
// transaction wrappers that bind blocks to their atomic state changes.
//
// # Overview
//
// nom is the schema layer everything downstream depends on. Every other
// package in the codebase — chain, consensus, verifier, vm, embedded,
// rpc, protocol, p2p — speaks in terms of [AccountBlock] and [Momentum].
// nom owns:
//
//   - The block-type enumeration ([BlockTypeUserSend], [BlockTypeUserReceive],
//     [BlockTypeContractSend], [BlockTypeContractReceive],
//     [BlockTypeGenesisReceive]).
//   - Canonical hashing for both block kinds ([AccountBlock.ComputeHash],
//     [Momentum.ComputeHash], [MomentumContent.Hash]).
//   - Ed25519 producer derivation and caching.
//   - Protobuf and JSON serialization, including the
//     [AccountBlockMarshal] wire-friendly twin used by RPC.
//   - The [db.Transaction] wrappers ([AccountBlockTransaction],
//     [MomentumTransaction]) the chain layer uses to commit blocks
//     atomically alongside their state patch.
//
// # Key Concepts
//
//   - Account block — one transaction on a single account's chain. Send
//     blocks transfer; receive blocks consume a matching send. Every
//     transfer on the network is a (send, receive) pair.
//   - Momentum — a consensus block produced by an elected pillar,
//     committing to a sorted list of [types.AccountHeader]s and the patch
//     hash those blocks produced. See
//     [github.com/zenon-network/go-zenon/ARCHITECTURE.md] for the
//     dual-ledger model.
//   - Descendant blocks — [BlockTypeContractSend] account blocks emitted
//     by an embedded contract while receiving a triggering send. They
//     nest inside the parent receive block via
//     [AccountBlock.DescendantBlocks] and contribute to the parent's
//     hash through [AccountBlock.DescendantBlocksHash].
//   - Plasma — per-block resource cost. Paid via fused QSR
//     ([AccountBlock.FusedPlasma]) or via PoW
//     ([AccountBlock.Difficulty]+[Nonce]).
//   - Momentum acknowledgement — every account block names a
//     [AccountBlock.MomentumAcknowledged] pinning the most recent
//     momentum the author had observed; the verifier uses it to bound
//     reorgs.
//   - Changes hash — each block carries the hash of the state patch its
//     execution produced, binding state to consensus.
//   - Producer caching — both [AccountBlock] and [Momentum] derive the
//     producer address from [PublicKey] lazily and cache it. The cache
//     is not synchronized; [Momentum.EnsureCache] primes it after
//     deserialization.
//
// # Usage
//
// Build, hash, and serialize:
//
//	ab := &nom.AccountBlock{ /* fields */ }
//	ab.Hash = ab.ComputeHash()
//	bytes, err := ab.Serialize()
//
// Decode from the wire:
//
//	ab, err := nom.DeserializeAccountBlock(bytes)
//
// Wrap for atomic commit:
//
//	tx := &nom.AccountBlockTransaction{Block: ab, Changes: patch}
//	if err := mgr.Add(tx); err != nil { /* handle */ }
//
// # Invariants
//
//   - For any [AccountBlock], the stored Hash equals
//     [AccountBlock.ComputeHash].
//   - For any [Momentum], the stored Hash equals [Momentum.ComputeHash].
//   - Every non-genesis send eventually has exactly one matching
//     receive (cross-chain invariant; enforced by the verifier and
//     chain). [BlockTypeGenesisReceive] blocks have no matching send —
//     they bootstrap the per-address chains at height 1 with the
//     genesis-configured balances and have no FromBlockHash.
//   - [MomentumContent] is sorted by canonical header bytes so producers
//     building from the same account-block set agree on the same content
//     hash.
//
// # Related Packages
//
//   - [github.com/zenon-network/go-zenon/common/types] — the [Address],
//     [Hash], [HashHeight], [AccountHeader], and [ZenonTokenStandard]
//     types this package builds blocks from.
//   - [github.com/zenon-network/go-zenon/common/db] — defines [db.Patch]
//     and [db.Transaction] / [db.Commit] this package implements.
//   - [github.com/zenon-network/go-zenon/common] — provides the byte
//     helpers used to compose canonical hash inputs.
//   - [github.com/zenon-network/go-zenon/chain] — orchestrates the
//     account and momentum stores and inserts these blocks.
//   - [github.com/zenon-network/go-zenon/verifier] — validates blocks
//     against the canonical hash and signature.
//   - [github.com/zenon-network/go-zenon/vm/embedded] — populates
//     [AccountBlock.DescendantBlocks] when a contract receive emits
//     further sends.
package nom
