// Package node wires the P2P server, wallet, and Zenon core into a single
// long-running process and arbitrates their lifecycle.
//
// # Overview
//
// node sits one layer above [github.com/zenon-network/go-zenon/zenon]. It owns
// the data-directory lock that prevents two znnd instances from sharing a
// single chain database, exposes the configured [Wallet] manager to the rest
// of the process, and propagates start/stop signals to subsystems.
//
// Per-package documentation is being filled in incrementally. See
// docs/STYLE.md for the full template applied in subsequent PRs.
package node
