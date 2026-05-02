// Package fetcher retrieves and validates individual blocks announced by
// peers.
//
// # Overview
//
// On a peer announcement, fetcher requests the missing block, runs it through
// the verifier, and hands it to the chain. Bulk catch-up is handled by
// [github.com/zenon-network/go-zenon/protocol/downloader].
//
// Per-package documentation is being filled in incrementally. See
// docs/STYLE.md for the full template applied in subsequent PRs.
package fetcher
