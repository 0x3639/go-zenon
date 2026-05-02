// Package store holds the storage primitives shared by the account and
// momentum stores.
//
// # Overview
//
// store factors out the common index/encoding/iteration logic used by
// [github.com/zenon-network/go-zenon/chain/account] and
// [github.com/zenon-network/go-zenon/chain/momentum] so that both ledgers
// share consistent patch semantics over LevelDB.
//
// Per-package documentation is being filled in incrementally. See
// docs/STYLE.md for the full template applied in subsequent PRs.
package store
