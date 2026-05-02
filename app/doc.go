// Package app holds the process-level lifecycle for the znnd binary.
//
// # Overview
//
// app is the glue between command-line configuration, the on-disk data
// directory, and the [github.com/zenon-network/go-zenon/zenon] core. It owns
// the boot sequence (load config, open data dir, construct node, start
// services) and the inverse on shutdown.
//
// Per-package documentation is being filled in incrementally. See
// docs/STYLE.md for the full template applied in subsequent PRs.
package app
