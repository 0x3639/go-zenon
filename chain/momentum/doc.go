// Package momentum stores the global momentum chain and exposes versioned
// readers over it.
//
// # Overview
//
// momentum persists the consensus chain produced by elected pillars,
// indexed by hash and height. Together with
// [github.com/zenon-network/go-zenon/chain/account], it forms the dual-ledger
// state that [github.com/zenon-network/go-zenon/chain] orchestrates.
//
// Per-package documentation is being filled in incrementally. See
// docs/STYLE.md for the full template applied in subsequent PRs.
package momentum
