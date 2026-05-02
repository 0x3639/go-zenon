// Package verifier validates account blocks and momentums against the
// consensus rules before [github.com/zenon-network/go-zenon/chain]
// inserts them.
//
// # Overview
//
// Verification runs in two phases per block kind so that the cheap,
// stateless checks can fail fast before the verifier reaches into the
// stores. The same split applies to both account blocks and momentums:
//
//   - Block-as-payload pass — version, type, amounts, previous-block
//     linkage, momentum-acknowledged consistency, send/receive matching,
//     plasma PoW. Implemented by [accountBlockVerifier] /
//     [rawMomentumVerifier]; entry points are
//     [AccountBlockVerifier.AccountBlock] and [MomentumVerifier.Momentum].
//   - Transactional pass — canonical hash, signature, producer-address
//     consistency, descendants (account blocks) or changes-hash + producer
//     eligibility (momentums). Implemented by
//     [accountBlockTransactionVerifier] / [momentumTransactionVerifier];
//     entry points are [AccountBlockVerifier.AccountBlockTransaction] and
//     [MomentumVerifier.MomentumTransaction].
//
// # Key Concepts
//
//   - Stateless checks — those that depend only on the block payload
//     (e.g., hash, signature, version). Cheap; run first.
//   - Stateful checks — those that depend on the surrounding ledger
//     state (e.g., previous-block linkage, send/receive matching, the
//     embedded-contract sequencer). Resolved through stores returned by
//     [chain.Chain.GetMomentumStore] / [chain.Chain.GetAccountStore].
//   - Sequencer — the rule that embedded-contract receives must consume
//     from the head of their address mailbox in order. User accounts are
//     not subject to this — they may receive any pending send.
//   - Receiver-mismatch enforcement — historically tolerated, now a hard
//     rejection above [ReceiverMismatchEnforcementHeight].
//   - Internal vs rule errors — rule violations return one of the
//     `ErrAB*` / `ErrM*` sentinels; unexpected store/IO errors are wrapped
//     in [ErrVerifierInternal] via [InternalError]. Callers should branch
//     on the sentinel, not on the error message.
//
// # Usage
//
// Construct once at boot, alongside the chain and consensus subsystems:
//
//	v := verifier.NewVerifier(chain, consensus)
//
// Validate an inbound account block (payload first, then transactional):
//
//	if err := v.AccountBlock(block); err != nil { /* reject */ }
//	if err := v.AccountBlockTransaction(tx);    err != nil { /* reject */ }
//
// Validate an inbound momentum:
//
//	if err := v.Momentum(detailed);          err != nil { /* reject */ }
//	if err := v.MomentumTransaction(tx);     err != nil { /* reject */ }
//
// # Related Packages
//
//   - [github.com/zenon-network/go-zenon/chain/nom] — the block model
//     this package validates.
//   - [github.com/zenon-network/go-zenon/chain] — supplies the
//     [chain.Chain] handle the verifier reads stores from.
//   - [github.com/zenon-network/go-zenon/chain/store] — store interfaces
//     consumed by the per-check helpers.
//   - [github.com/zenon-network/go-zenon/consensus] — supplies producer
//     eligibility through [consensus.Consensus.VerifyMomentumProducer].
//   - [github.com/zenon-network/go-zenon/pow] — PoW nonce verification.
//   - [github.com/zenon-network/go-zenon/wallet] — Ed25519 signature
//     verification.
//   - [github.com/zenon-network/go-zenon/common/db] — provides
//     [db.PatchHash] for the changes-hash check.
package verifier
