// Package types defines the foundational identifier types used everywhere on
// the chain: addresses, hashes, token standards, sporks, and hash/height
// pairs.
//
// # Overview
//
// types is the deepest dependency in the codebase. It owns `Address` and its
// human-readable encoding, the 32-byte `Hash`, the `HashHeight` location
// pair used to reference points on either ledger, the `TokenStandard`
// identifier for ZTS tokens, and the `Spork` definitions that gate protocol
// upgrades.
//
// Per-package documentation is being filled in incrementally. See
// docs/STYLE.md for the full template applied in subsequent PRs.
package types
