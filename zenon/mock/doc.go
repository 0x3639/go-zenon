// Package mock provides a test harness that builds an in-memory Zenon node
// suitable for unit and integration tests.
//
// # Overview
//
// mock wires a fully working but ephemeral chain, consensus, verifier, and VM
// stack against an in-memory LevelDB. Tests use it to drive scenarios end to
// end without standing up a real network.
//
// Per-package documentation is being filled in incrementally. See
// docs/STYLE.md for the full template applied in subsequent PRs.
package mock
