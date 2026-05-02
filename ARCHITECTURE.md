# Architecture

This document is the canonical, version-controlled overview of the `go-zenon` reference node. It is the source of truth that every package's godoc cross-links to. Read this file once, then use [AGENTS.md](AGENTS.md) for the package-by-package map.

## Mental Model in One Paragraph

Zenon's Network of Momentum (NoM) is a **dual-ledger** blockchain. Every account has its own private chain of [`AccountBlock`s](chain/nom/account_block.go) — a send/receive ledger where each transfer is two atomic blocks (one send, one receive). A second, global chain of [`Momentum`s](chain/nom/momentum.go) anchors batches of account blocks into a single timeline produced by elected validators called **Pillars**. Account blocks settle the *what* (token transfers, contract calls); momentums settle the *when* and *who agrees* (consensus, ordering, finality). Anti-spam is enforced by **Plasma** (a fused-QSR resource budget per account), and protocol upgrades are gated by **Sporks** that flip features on at a specific momentum height.

## The Two Ledgers

### Account Block (per-account chain)

Defined in [`chain/nom/account_block.go`](chain/nom/account_block.go).

| Field | Purpose |
|---|---|
| `Address` | Owning account's chain. |
| `Height`, `PreviousHash` | Position within the account's chain. |
| `BlockType` | One of `UserSend`, `UserReceive`, `ContractSend`, `ContractReceive`, `GenesisReceive`. |
| `FromBlockHash` | On *receive* blocks, points to the matching *send* block on the sender's chain. |
| `Amount`, `TokenStandard` | Transferred value. |
| `Data` | Contract method call payload (ABI-encoded). |
| `FusedPlasma`, `Difficulty`, `Nonce` | Plasma fuel: either fused (paid up front via stake) or PoW-burned. |
| `DescendantBlocks` | When a contract receive triggers further sends, those sends are nested here. |
| `Signature`, `PublicKey` | Ed25519 signature by the account owner. |

**Invariant.** Every send eventually has exactly one receive. An unreceived send is in the *unconfirmed mailbox* of the recipient until the recipient's next block consumes it.

### Momentum (consensus chain)

Defined in [`chain/nom/momentum.go`](chain/nom/momentum.go).

| Field | Purpose |
|---|---|
| `Height`, `PreviousHash` | Position in the global momentum chain. |
| `TimestampUnix` | Wall-clock timestamp; used by the consensus tick scheduler. |
| `Content` | List of `(account, hash, height)` tuples — the account blocks finalized by this momentum. |
| `ChangesHash` | Merkle root of state changes applied; binds state to consensus. |
| `Signature`, `PublicKey` | Ed25519 signature by the producing pillar. |

**Invariant.** The producer's public key must be the pillar elected for the momentum's tick (see *Election* below).

## Lifecycle of a Transaction

```
1.  User signs an AccountBlock (send) and submits it.
2.  Verifier checks signature, plasma, prev-block link, and contract preconditions.
3.  Chain adds the block to the account's pool. Insert lock serializes mutations.
4.  Recipient (user or embedded contract) eventually authors a matching receive.
    For embedded contracts, the contract's ReceiveBlock implementation runs in the VM
    and may emit DescendantBlocks (further sends).
5.  At the next tick, the elected pillar produces a Momentum referencing the new
    account blocks; the Verifier validates and the Chain commits the momentum.
6.  ProtocolManager broadcasts to peers; downloaders fill gaps for nodes that lag.
```

## Key Components

| Layer | Package | Role |
|---|---|---|
| Entry | [`cmd/znnd`](cmd/znnd) | Standalone node binary. |
| Entry | [`cmd/libznn`](cmd/libznn) | Embeddable C-shared library. |
| App | [`app/`](app) | Process-level lifecycle; wires config → node → znnd. |
| Node | [`node/`](node) | P2P server + wallet + Zenon core glue; data-dir locking. |
| Core | [`zenon/`](zenon) | Top-level orchestrator; sequences `chain → consensus → verifier → pillar → protocol`. |
| Ledger | [`chain/`](chain) | Account pool + momentum pool; insert lock; genesis compatibility. |
| Storage | [`chain/store/`](chain/store), [`chain/account/`](chain/account), [`chain/momentum/`](chain/momentum) | Versioned LevelDB stores per ledger. |
| Block model | [`chain/nom/`](chain/nom) | `AccountBlock`, `Momentum`, transaction wrappers, hashing. |
| Genesis | [`chain/genesis/`](chain/genesis) | Embedded alphanet genesis + chain-identifier checks. |
| Consensus | [`consensus/`](consensus) | Pillar election, points, tick scheduler, ProducerEvent. |
| Verifier | [`verifier/`](verifier) | Stateless + stateful block validation. |
| VM | [`vm/`](vm) | Supervisor + per-block execution context. |
| Embedded contracts | [`vm/embedded/`](vm/embedded) | Pillar, Sentinel, Stake, Token, Plasma, Spork, Swap, Accelerator, HTLC, Bridge, Liquidity. |
| Pillar | [`pillar/`](pillar) | Block-producing role; reacts to `ProducerEvent` from consensus. |
| Protocol | [`protocol/`](protocol) | ChainBridge, Broadcaster, Downloader, Fetcher; bridges `chain` ↔ P2P. |
| P2P | [`p2p/`](p2p) | Peer connection, discovery, NAT traversal. |
| RPC | [`rpc/`](rpc) | HTTP/WS RPC server; `api/`, `api/embedded/`, `api/subscribe/`. |
| Wallet | [`wallet/`](wallet) | Keystore, keyfile, key derivation. |
| PoW | [`pow/`](pow) | Plasma proof-of-work generator. |
| Common | [`common/`](common) | Hashes, addresses, token standards, sporks, db patches, crypto, logs. |

## Election (Consensus)

Defined in [`consensus/`](consensus). Time is divided into **ticks**; each tick maps deterministically to one elected pillar based on a weighted shuffle of registered pillars (weight comes from delegated stake and historical points). When the local node's coinbase matches the elected pillar for the current tick, the consensus layer emits a `ProducerEvent`; [`pillar/`](pillar) listens and produces a momentum. Points accumulate for produced momentums and decay for missed slots, feeding back into future election weights.

## Plasma

Plasma is the anti-spam fuel that lets account blocks be created without per-block fees. Each account can either:
- **Fuse QSR** (stake QSR to a beneficiary) — produces a steady plasma yield, OR
- **Burn PoW** — solve a small proof-of-work per block.

Plasma cost per block depends on `BlockType` and `Data` size; the rules live in [`vm/`](vm) and [`vm/embedded/implementation/plasma.go`](vm/embedded/implementation/plasma.go).

## Sporks

A spork is a protocol-upgrade switch: a hash committed by the spork-controlling address that activates a new feature once a momentum-height threshold is crossed. Defined in [`common/types/spork.go`](common/types/spork.go) and used throughout `vm/embedded/embedded.go` to dispatch between contract revisions. Live sporks: **Accelerator**, **HTLC**, **BridgeAndLiquidity**.

## Embedded Contracts

Contracts at fixed system addresses, dispatched by [`vm/embedded/embedded.go`](vm/embedded/embedded.go). Each contract method implements three hooks: `GetPlasma()`, `ValidateSendBlock()`, `ReceiveBlock()`. Definitions (ABI, types) live under [`definition/`](vm/embedded/definition); behavior under [`implementation/`](vm/embedded/implementation).

| Contract | Purpose |
|---|---|
| Pillar | Register/revoke pillars, claim rewards, weight delegation. |
| Sentinel | Sentinel node registration & reward. |
| Stake | Lock ZNN to receive QSR rewards. |
| Token | ZTS token issuance, mint, burn. |
| Plasma | Fuse QSR for plasma generation. |
| Spork | Spork lifecycle (create, activate). |
| Swap | Legacy-chain swap claims. |
| Accelerator | Project funding via votes. |
| HTLC | Hashed timelock contracts. |
| Bridge | Cross-chain bridge endpoints (wrap/unwrap). |
| Liquidity | Liquidity-program rewards. |

## Concurrency Model

- The chain serializes all insertions through a single mutex (`chain.AcquireInsert`). Reads are versioned via `db.Manager` patches and do not block writes once committed.
- Consensus runs the tick scheduler in a background goroutine and emits events through channels.
- The P2P server runs each peer in its own goroutine; the protocol manager arbitrates which peers feed the chain.
- The pillar runs an event loop reacting to `ProducerEvent` and momentum events from chain.

## Storage

- LevelDB-backed; versioned via [`common/db/`](common/db) `Manager` and `Patch`.
- Two top-level dbs per node: `nom` (chain) and `consensus`.
- Patches are atomic state transitions; commits finalize them to disk and emit events for subscribers.

## Where to Look First

- "How does a transaction get mined?" → [`chain/account_pool.go`](chain/account_pool.go), [`chain/momentum_pool.go`](chain/momentum_pool.go), [`pillar/`](pillar).
- "How is a pillar elected?" → [`consensus/election.go`](consensus/election.go).
- "What does an embedded contract method look like?" → [`vm/embedded/implementation/`](vm/embedded/implementation) (start with `token.go` — small).
- "How does the RPC API work?" → [`rpc/api/ledger.go`](rpc/api/ledger.go) for the public-facing query API.
- "How does plasma work?" → [`vm/embedded/implementation/plasma.go`](vm/embedded/implementation/plasma.go).
- "How is a feature gated by spork?" → grep for `IsActive` in [`vm/embedded/embedded.go`](vm/embedded/embedded.go).

## Glossary

See [AGENTS.md](AGENTS.md#glossary) for the canonical vocabulary used in godoc comments throughout the codebase. Every package doc.go uses these terms exactly — no synonyms.
