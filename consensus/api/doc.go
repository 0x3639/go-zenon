// Package api exposes the read-only consensus query surface used by the RPC
// layer and by other in-process subsystems.
//
// # Overview
//
// api separates queries (election lookups, points queries, pillar weights)
// from mutations so that callers reading consensus state cannot accidentally
// reach into the scheduler.
//
// Per-package documentation is being filled in incrementally. See
// docs/STYLE.md for the full template applied in subsequent PRs.
package api
