// Package tests holds the integration test suite for the embedded contracts.
//
// # Overview
//
// tests exercises each embedded contract end to end through a mock node from
// [github.com/zenon-network/go-zenon/zenon/mock], composing scenarios from
// the helpers in [github.com/zenon-network/go-zenon/chain/genesis/mock]. It
// is built and executed by `go test`; the package contains no production
// code.
//
// Per-package documentation is being filled in incrementally. See
// docs/STYLE.md for the full template applied in subsequent PRs.
package tests
