// Package app holds the process-level lifecycle for the znnd binary.
//
// # Overview
//
// app is the glue between command-line configuration, the on-disk
// data directory, and the [github.com/zenon-network/go-zenon/node]
// container. It is consumed by two entry points:
//
//   - [github.com/zenon-network/go-zenon/cmd/znnd] — the standard
//     CLI binary.
//   - [github.com/zenon-network/go-zenon/cmd/libznn] — the C-shared
//     library variant, which exports [Run] / [Stop] for embedding
//     into other languages.
//
// The CLI surface is built on `urfave/cli/v2` ([cli.App] is
// initialised in init()): flags from [AllFlags], the version /
// license sub-commands, and the default action that boots the node.
//
// # Boot Sequence
//
// [Run] hands control to cli, which calls:
//
//  1. [beforeAction] — print version banner, set GOMAXPROCS,
//     optionally start the pprof HTTP server.
//  2. [action] — build the node config via [MakeConfig],
//     instantiate a [Manager], and call [Manager.Start].
//
// [MakeConfig] starts from [node.DefaultNodeConfig], merges
// `config.json` (if present), then overlays per-flag overrides; it
// also resolves all paths to absolute and initialises logging.
//
// # Shared Library vs. CLI
//
// Two build tags select between manager_libznn.go and manager.go:
//
//   - default (`!libznn`) — installs SIGINT/SIGTERM handlers and
//     blocks on [node.Node.Wait] inside [Manager.Start].
//   - `libznn` — Start returns immediately after node startup;
//     embedders call the exported [Stop] when they want to tear
//     down.
//
// # Generated Files
//
// None. Files are Zenon-specific (no upstream header).
//
// # Related Packages
//
//   - [github.com/zenon-network/go-zenon/node] — the actual process
//     container.
//   - [github.com/zenon-network/go-zenon/metadata] — version /
//     git-commit constants printed by the version command.
//   - [github.com/zenon-network/go-zenon/cmd/znnd] — main binary
//     that calls [Run].
//   - [github.com/zenon-network/go-zenon/cmd/libznn] — C-shared
//     library variant.
package app
