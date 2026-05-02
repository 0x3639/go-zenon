// Package metadata exposes the build-time information baked into the
// node binary.
//
// # Overview
//
// metadata holds two values: the human-readable [Version] string (a
// build-tagged constant; one variant per build target) and the [GitCommit]
// hash captured by the Go toolchain at build time. RPC `utility` endpoints
// surface both to clients, and structured log lines include them on boot
// so a running node can be traced back to a specific commit.
//
// # Key Concepts
//
//   - Version — the release-channel string. Two variants are defined,
//     gated by the `libznn` build tag, so the standalone `znnd` and the
//     `libznn` C-shared-library builds advertise themselves distinctly.
//   - GitCommit — the VCS revision from [debug.BuildInfo], populated when
//     the binary is built inside a VCS-aware context.
//
// # Related Packages
//
//   - [github.com/zenon-network/go-zenon/cmd/znnd] — entry point that
//     prints [Version] and [GitCommit] at startup.
//   - [github.com/zenon-network/go-zenon/rpc/api] — `utility` namespace
//     surfaces these values to RPC clients.
package metadata
