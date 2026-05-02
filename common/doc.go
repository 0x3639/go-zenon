// Package common holds the shared utilities used across the codebase:
// structured logging, error helpers, and byte-manipulation primitives.
//
// # Overview
//
// Subpackages carry the more substantial primitives:
// [github.com/zenon-network/go-zenon/common/types] for `Hash`, `Address`, and
// `TokenStandard`; [github.com/zenon-network/go-zenon/common/db] for the
// versioned LevelDB manager and patch model; and
// [github.com/zenon-network/go-zenon/common/crypto] for hashing and Ed25519
// helpers.
//
// Per-package documentation is being filled in incrementally. See
// docs/STYLE.md for the full template applied in subsequent PRs.
package common
