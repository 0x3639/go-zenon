// Package vm executes account blocks: it routes them to the right embedded
// contract, enforces plasma costs, and produces the resulting state patch.
//
// # Overview
//
// vm exposes a Supervisor that, given an [github.com/zenon-network/go-zenon/chain/nom.AccountBlock],
// constructs an execution context, invokes the appropriate
// [github.com/zenon-network/go-zenon/vm/embedded] contract method (or
// performs a plain user transfer), and returns the resulting block transaction
// to the chain layer for insertion.
//
// Per-package documentation is being filled in incrementally. See
// docs/STYLE.md for the full template applied in subsequent PRs.
package vm
