//go:build !libznn

// Command znnd is the standalone Zenon Network of Momentum node binary.
//
// # Overview
//
// znnd parses command-line flags and an optional `config.json`, opens or
// initializes the data directory, and starts the
// [github.com/zenon-network/go-zenon/zenon] core via
// [github.com/zenon-network/go-zenon/app] and
// [github.com/zenon-network/go-zenon/node].
//
// Per-command documentation is being filled in incrementally. See
// docs/STYLE.md for the full template applied in subsequent PRs.
package main
