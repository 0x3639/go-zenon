// Package account stores the per-account block chain and exposes versioned
// readers over it.
//
// # Overview
//
// Each account on the network maintains its own ordered chain of account
// blocks. account is the storage layer that persists those chains to LevelDB,
// indexes them by hash and height, and serves point-in-time reads through
// [github.com/zenon-network/go-zenon/common/db.Manager] patches.
//
// Per-package documentation is being filled in incrementally. See
// docs/STYLE.md for the full template applied in subsequent PRs.
package account
