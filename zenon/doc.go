// Package zenon is the top-level orchestrator of the Network of Momentum core.
//
// # Overview
//
// zenon constructs and sequences the major subsystems —
// [github.com/zenon-network/go-zenon/chain],
// [github.com/zenon-network/go-zenon/consensus],
// [github.com/zenon-network/go-zenon/verifier],
// [github.com/zenon-network/go-zenon/pillar], and
// [github.com/zenon-network/go-zenon/protocol] — and exposes them as a single
// [Zenon] facade. Initialization order is fixed: chain → consensus → event
// printer → subscription → pillar → protocol.
//
// Per-package documentation is being filled in incrementally. See
// docs/STYLE.md for the full template applied in subsequent PRs.
package zenon
