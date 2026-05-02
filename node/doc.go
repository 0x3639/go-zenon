// Package node wires the P2P server, wallet, and Zenon core into a
// single long-running process and arbitrates their lifecycle.
//
// # Overview
//
// node sits one layer above [github.com/zenon-network/go-zenon/zenon].
// It owns the data-directory lock that prevents two znnd instances
// from sharing a single chain database, exposes the configured
// [github.com/zenon-network/go-zenon/wallet.Manager] to the rest of
// the process, and propagates start/stop signals to subsystems in
// the right order.
//
// # Lifecycle
//
// [NewNode] performs setup-only work: open the data dir, take the
// `.lock` file, start the wallet, build a [zenon.Config] from the
// node config, instantiate the [zenon.Zenon] facade, and assemble
// the [p2p.Server] descriptor. [Node.Start] then runs the actual
// boot sequence:
//
//  1. zenon Init + Start (chain, consensus, pillar, etc.)
//  2. p2p server Start (listen + dial)
//  3. publish RPC APIs ([api.GetPublicApis]) and start the HTTP /
//     WebSocket transports
//
// [Node.Stop] unwinds in reverse: p2p → wallet → zenon → RPC stack
// → release the data-dir lock. [Node.Wait] blocks until Stop is
// called (used by the CLI to keep the process alive).
//
// # Data Directory
//
// [Node.openDataDir] mkdir's [Config.DataPath] and acquires an
// exclusive `flock` on `<DataPath>/.lock`. A second znnd against the
// same directory fails fast with [ErrDataDirUsed] rather than
// corrupting the store.
//
// # RPC Stack
//
// [httpServer] (rpcstack.go) hosts both HTTP-JSON-RPC and
// WebSocket transports against the namespaces registered through
// [github.com/zenon-network/go-zenon/rpc]. Defaults: HTTP on
// [p2p.DefaultHTTPPort]=35997, WS on [p2p.DefaultWSPort]=35998,
// CORS / WSOrigins both `["*"]` in [DefaultNodeConfig].
//
// # Generated Files
//
// None. rpcstack.go is ported from go-ethereum but does not carry
// the upstream LGPL header in this fork; the rest are
// Zenon-specific.
//
// # Related Packages
//
//   - [github.com/zenon-network/go-zenon/zenon] — the core facade
//     this node owns.
//   - [github.com/zenon-network/go-zenon/p2p] — peer-to-peer server
//     started by [Node.Start].
//   - [github.com/zenon-network/go-zenon/rpc] — module wiring used
//     when registering APIs.
//   - [github.com/zenon-network/go-zenon/app] — CLI driver that
//     constructs [Config] from flags and calls [NewNode].
package node
