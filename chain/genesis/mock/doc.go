// Package g provides a programmable mock genesis used by the embedded
// contract test suite.
//
// # Overview
//
// Tests build a genesis with arbitrary balances, pillar registrations, stake
// entries, and spork states by composing this package's helpers, then hand
// the result to [github.com/zenon-network/go-zenon/zenon/mock] to spin up an
// in-memory node initialized at that synthetic state.
//
// Per-package documentation is being filled in incrementally. See
// docs/STYLE.md for the full template applied in subsequent PRs.
package g
