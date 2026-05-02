// Package store defines the read/write interfaces every chain consumer
// uses to query state at a specific point on either ledger.
//
// # Overview
//
// store is interface-only: the implementations live under
// [github.com/zenon-network/go-zenon/chain/account] and
// [github.com/zenon-network/go-zenon/chain/momentum]. Splitting
// definitions out lets the chain layer compose them without circular
// imports and gives the verifier a stable, narrow surface to mock against
// in tests.
//
// # Key Concepts
//
//   - Account — read/write view of one account's chain pinned at a
//     specific (hash, height). Includes balance and plasma caches plus
//     the per-account "received" set used to reject double-receives.
//   - AccountMailbox — per-recipient FIFO of inbound sends awaiting
//     consumption. The embedded-contract sequencer rule reads this.
//   - Momentum — read/write view of the global momentum chain pinned at
//     a specific frontier. The unifying view that answers every
//     chain-state question (which momentum, which account block,
//     embedded-contract state, balances, etc.).
//   - Genesis — the read-only subset every momentum view exposes
//     (chain identifier, the genesis momentum / transaction, and the
//     spork-controlling address).
//
// # Usage
//
// Consumers obtain views through the chain layer, never by constructing
// implementations directly:
//
//	mview := chain.GetMomentumStore(hashHeight)
//	frontier, _ := mview.GetFrontierMomentum()
//	aview := mview.GetAccountStore(address)
//	balance, _ := aview.GetBalance(types.ZnnTokenStandard)
//
// # Related Packages
//
//   - [github.com/zenon-network/go-zenon/chain] — the orchestrator that
//     hands out views.
//   - [github.com/zenon-network/go-zenon/chain/account] — implements
//     [Account].
//   - [github.com/zenon-network/go-zenon/chain/account/mailbox] —
//     implements [AccountMailbox].
//   - [github.com/zenon-network/go-zenon/chain/momentum] — implements
//     [Momentum] and [Genesis].
//   - [github.com/zenon-network/go-zenon/verifier] — primary consumer of
//     these interfaces.
//   - [github.com/zenon-network/go-zenon/vm], [.../vm/embedded] — read
//     contract state through [Account.Storage] and [Momentum] queries.
package store
