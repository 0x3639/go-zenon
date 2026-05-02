// Package momentum stores the global momentum chain and exposes versioned
// readers over it.
//
// # Overview
//
// momentum is the [github.com/zenon-network/go-zenon/chain/store.Momentum]
// implementation. Together with
// [github.com/zenon-network/go-zenon/chain/account] (per-account chains)
// and [github.com/zenon-network/go-zenon/chain/account/mailbox]
// (per-recipient queues), it forms the dual-ledger storage that
// [github.com/zenon-network/go-zenon/chain] orchestrates.
//
// On top of the raw momentum chain ([momentum.go], [range.go]) the
// store maintains a small set of cross-cutting indexes that the
// verifier, consensus layer, and RPC consume:
//
//   - account-header reverse index — `hash → AccountHeader`
//     ([account_block.go]); lets [GetAccountBlockByHash] resolve a
//     block without knowing the address.
//   - block-confirmation index — `hash → momentum height`
//     ([confirmed.go]); used by the verifier to validate
//     [chain/nom.AccountBlock.MomentumAcknowledged] for auto-generated
//     blocks.
//   - per-account ZNN balance cache ([balance.go]); duplicated from the
//     account store so consensus delegation math reads in O(1) per
//     backer.
//   - embedded-contract read-throughs ([embedded.go]) for active
//     pillars, defined sporks, stake amounts, and token info.
//
// # Key Concepts
//
//   - momentumStore — the [store.Momentum] implementation. Embeds
//     [store.Genesis] so chain identifier and genesis constants are
//     reachable, and embeds [db.DB] directly so range queries flow
//     through the same versioned layer.
//   - AddAccountBlockTransaction — single entry point that admits an
//     account block (with descendants) into this view, updating every
//     cross-cutting index in one pass. Skips batched contract sends
//     (their parent receive carries them).
//   - GetMomentumBeforeTime — estimate-and-step plus binary-search
//     lookup for time-anchored queries, optimized for the typical
//     case where momentum cadence is roughly even.
//
// # Related Packages
//
//   - [github.com/zenon-network/go-zenon/chain/store] — interface this
//     package implements.
//   - [github.com/zenon-network/go-zenon/chain/account],
//     [github.com/zenon-network/go-zenon/chain/account/mailbox] —
//     per-account components this view composes.
//   - [github.com/zenon-network/go-zenon/common/db] — the versioned
//     LevelDB layer.
//   - [github.com/zenon-network/go-zenon/vm/embedded/definition] —
//     embedded-contract storage helpers consumed by [embedded.go].
package momentum
