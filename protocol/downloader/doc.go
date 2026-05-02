// Package downloader bulk-syncs the chain when a node lags behind its peers.
//
// # Overview
//
// downloader requests momentum and account-block batches from peers, validates
// them through [github.com/zenon-network/go-zenon/verifier], and feeds them
// to [github.com/zenon-network/go-zenon/chain] in order. It complements
// [github.com/zenon-network/go-zenon/protocol/fetcher], which handles single
// blocks announced individually.
//
// Per-package documentation is being filled in incrementally. See
// docs/STYLE.md for the full template applied in subsequent PRs.
package downloader
