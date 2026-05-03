# go-zenon API documentation

Auto-generated rendering of every package's godoc, refreshed by `make doc-api`.
The canonical source of truth is the godoc comments in the Go source — this
directory is a convenience for GitHub-side review. For local interactive
browsing run `make docs` (godoc on :6060). After upstream merge the same
content publishes to pkg.go.dev under the canonical module path.

Module: `github.com/zenon-network/go-zenon`

## Packages

- [`github.com/zenon-network/go-zenon/app`](app/README.md) — Process-level lifecycle; wires config → node → znnd.
- [`github.com/zenon-network/go-zenon/chain`](chain/README.md) — Ledger; account pool, momentum pool, insert lock.
- [`github.com/zenon-network/go-zenon/chain/account`](chain/account/README.md) — Per-account chain store.
- [`github.com/zenon-network/go-zenon/chain/account/mailbox`](chain/account/mailbox/README.md) — Unconfirmed-send mailbox per recipient.
- [`github.com/zenon-network/go-zenon/chain/genesis`](chain/genesis/README.md) — Embedded alphanet genesis + chain identifier.
- [`github.com/zenon-network/go-zenon/chain/genesis/mock`](chain/genesis/mock/README.md) — Test-only mock genesis.
- [`github.com/zenon-network/go-zenon/chain/momentum`](chain/momentum/README.md) — Momentum-chain store.
- [`github.com/zenon-network/go-zenon/chain/nom`](chain/nom/README.md) — Block model: `AccountBlock`, `Momentum`, transaction wrappers.
- [`github.com/zenon-network/go-zenon/chain/store`](chain/store/README.md) — Common storage primitives shared by account/momentum stores.
- [`github.com/zenon-network/go-zenon/cmd/libznn`](cmd/libznn/README.md) — Build target: C-shared library.
- [`github.com/zenon-network/go-zenon/cmd/znnd`](cmd/znnd/README.md) — Build target: standalone node binary.
- [`github.com/zenon-network/go-zenon/common`](common/README.md) — Shared utilities: logging, errors, byte helpers.
- [`github.com/zenon-network/go-zenon/common/crypto`](common/crypto/README.md) — Hashing/Ed25519 helpers.
- [`github.com/zenon-network/go-zenon/common/db`](common/db/README.md) — Versioned LevelDB manager + patch system.
- [`github.com/zenon-network/go-zenon/common/types`](common/types/README.md) — Address, Hash, TokenStandard, Spork, HashHeight.
- [`github.com/zenon-network/go-zenon/consensus`](consensus/README.md) — Pillar election, points, tick scheduler.
- [`github.com/zenon-network/go-zenon/consensus/api`](consensus/api/README.md) — Read-only consensus query interface.
- [`github.com/zenon-network/go-zenon/consensus/storage`](consensus/storage/README.md) — LevelDB-backed consensus state (protobuf).
- [`github.com/zenon-network/go-zenon/metadata`](metadata/README.md) — Build version + git commit.
- [`github.com/zenon-network/go-zenon/node`](node/README.md) — P2P server + wallet + Zenon glue; data-dir locking.
- [`github.com/zenon-network/go-zenon/p2p`](p2p/README.md) — Peer connection lifecycle, server.
- [`github.com/zenon-network/go-zenon/p2p/discover`](p2p/discover/README.md) — Kademlia-style peer discovery.
- [`github.com/zenon-network/go-zenon/p2p/nat`](p2p/nat/README.md) — NAT traversal (UPnP / PMP).
- [`github.com/zenon-network/go-zenon/pillar`](pillar/README.md) — Block producer; reacts to `ProducerEvent`.
- [`github.com/zenon-network/go-zenon/pow`](pow/README.md) — Plasma proof-of-work generator.
- [`github.com/zenon-network/go-zenon/protocol`](protocol/README.md) — ChainBridge, Broadcaster; bridges chain ↔ P2P.
- [`github.com/zenon-network/go-zenon/protocol/downloader`](protocol/downloader/README.md) — Bulk-sync chain catch-up.
- [`github.com/zenon-network/go-zenon/protocol/fetcher`](protocol/fetcher/README.md) — Single-block fetch on announcement.
- [`github.com/zenon-network/go-zenon/rpc`](rpc/README.md) — RPC server entry.
- [`github.com/zenon-network/go-zenon/rpc/api`](rpc/api/README.md) — Public RPC API (`ledger`, `network`, `utility`).
- [`github.com/zenon-network/go-zenon/rpc/api/embedded`](rpc/api/embedded/README.md) — Per-contract RPC endpoints.
- [`github.com/zenon-network/go-zenon/rpc/api/subscribe`](rpc/api/subscribe/README.md) — Subscription / notification channel.
- [`github.com/zenon-network/go-zenon/rpc/server`](rpc/server/README.md) — HTTP/WS/IPC RPC transport (ported from go-ethereum).
- [`github.com/zenon-network/go-zenon/verifier`](verifier/README.md) — Stateless + stateful block validation.
- [`github.com/zenon-network/go-zenon/vm`](vm/README.md) — VM core: supervisor + execution context.
- [`github.com/zenon-network/go-zenon/vm/abi`](vm/abi/README.md) — Contract ABI encode/decode (Solidity-compatible).
- [`github.com/zenon-network/go-zenon/vm/constants`](vm/constants/README.md) — VM-wide constants and error definitions.
- [`github.com/zenon-network/go-zenon/vm/embedded`](vm/embedded/README.md) — Embedded contract dispatcher (spork-aware).
- [`github.com/zenon-network/go-zenon/vm/embedded/definition`](vm/embedded/definition/README.md) — Per-contract ABI + types.
- [`github.com/zenon-network/go-zenon/vm/embedded/implementation`](vm/embedded/implementation/README.md) — Per-contract behavior (`GetPlasma`, `ValidateSendBlock`, `ReceiveBlock`).
- [`github.com/zenon-network/go-zenon/vm/embedded/tests`](vm/embedded/tests/README.md) — Embedded-contract test suite.
- [`github.com/zenon-network/go-zenon/vm/vm_context`](vm/vm_context/README.md) — Per-block execution context.
- [`github.com/zenon-network/go-zenon/wallet`](wallet/README.md) — Keystore, keyfile, key derivation.
- [`github.com/zenon-network/go-zenon/zenon`](zenon/README.md) — Top-level orchestrator (`chain`+`consensus`+`pillar`+`protocol`).
- [`github.com/zenon-network/go-zenon/zenon/mock`](zenon/mock/README.md) — Test-only mock node.

## Cross-references

- [ARCHITECTURE.md](../../ARCHITECTURE.md) — system overview and concept glossary.
- [AGENTS.md](../../AGENTS.md) — package map and where-to-look index.
- [STYLE.md](../STYLE.md) — godoc style guide.
