// Package metadata exposes build-time information baked into the binary.
//
// # Overview
//
// metadata holds the running znnd version string and git commit hash, used by
// RPC `utility` calls and structured log lines. The git commit value is
// generated at build time by the Makefile.
//
// Per-package documentation is being filled in incrementally. See
// docs/STYLE.md for the full template applied in subsequent PRs.
package metadata
