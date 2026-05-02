// Package db is the versioned LevelDB layer used by chain, consensus, and VM
// stores.
//
// # Overview
//
// db wraps go-leveldb in a `Manager` that exposes versioned snapshots, atomic
// `Patch` mutations, and a `Commit` interface that ledger objects implement to
// describe their state-change footprint. Higher-level code stages a patch,
// validates against the current version, and commits in one atomic step under
// the chain insert lock.
//
// Per-package documentation is being filled in incrementally. See
// docs/STYLE.md for the full template applied in subsequent PRs.
package db
