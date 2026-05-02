// Package account stores the per-account block chain and exposes
// versioned readers over it.
//
// # Overview
//
// account is the [github.com/zenon-network/go-zenon/chain/store.Account]
// implementation. Each account on the network owns one chain of
// [github.com/zenon-network/go-zenon/chain/nom.AccountBlock]s; account
// persists those chains to the underlying versioned LevelDB and serves
// point-in-time reads through [github.com/zenon-network/go-zenon/common/db.Manager]
// patches.
//
// In addition to the raw chain, the account store keeps three caches
// that the verifier and VM read in their hot paths:
//
//   - Per-token balance ([balance.go]).
//   - Per-account chain-plasma counter ([plasma.go]).
//   - "Already received" set keyed by the originating send's hash
//     ([received.go]). Together with the per-recipient mailbox in
//     [github.com/zenon-network/go-zenon/chain/account/mailbox] this is
//     how the verifier rejects double-receives.
//
// The embedded-contract sequencer cursor lives here too ([sequencer.go]):
// it tracks how far this account has consumed from the address's mailbox.
//
// # Key Concepts
//
//   - accountStore — the [store.Account] implementation. Embeds
//     [db.DB] directly so range queries and snapshots flow through the
//     same versioned layer as the rest of the chain.
//   - Storage namespace — embedded contracts reach into per-account
//     storage via [accountStore.Storage] (a [db.DB] view rooted at
//     [storageKeyPrefix]) wrapped in [db.DisableNotFound] so callers can
//     branch on `len(value) == 0`.
//   - Sequencer cursor — single uint64 stored under
//     [sequencerLastReceivedKey]; advances by one for every committed
//     embedded-contract receive.
//
// # Related Packages
//
//   - [github.com/zenon-network/go-zenon/chain/store] — interface this
//     package implements.
//   - [github.com/zenon-network/go-zenon/chain/account/mailbox] — the
//     per-recipient pending-send queue this store reads through the
//     sequencer logic.
//   - [github.com/zenon-network/go-zenon/common/db] — the versioned
//     LevelDB layer; account uses [db.SetFrontier], [db.GetEntryByHash],
//     and [db.GetEntryByHeight] so its on-disk layout matches the rest
//     of the chain.
//   - [github.com/zenon-network/go-zenon/chain/nom] — block model the
//     store serializes.
package account
