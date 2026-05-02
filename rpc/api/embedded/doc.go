// Package embedded exposes per-contract RPC endpoints for the
// embedded contracts.
//
// # Overview
//
// One namespace per contract, each providing read accessors over
// that contract's on-chain storage. Send-side calls (state-mutating
// transactions) go through the generic account-block submission path
// in [github.com/zenon-network/go-zenon/rpc/api.LedgerApi.PublishRawTransaction];
// this package supplies only the view side.
//
// # Namespaces
//
//   - [TokenAPI] — "embedded.token". Token info / supply queries.
//   - [PillarApi] — "embedded.pillar". Pillar registry, votes,
//     uncollected rewards, delegations, election history.
//   - [SentinelApi] — "embedded.sentinel". Sentinel registrations
//     and rewards.
//   - [StakeApi] — "embedded.stake". ZNN staking entries and
//     uncollected rewards.
//   - [PlasmaApi] — "embedded.plasma". Plasma fusion entries.
//   - [SwapApi] — "embedded.swap". Pillar → coinbase swap accounting.
//   - [SporkApi] — "embedded.spork". Active and pending sporks.
//   - [AcceleratorApi] — "embedded.accelerator". Project / phase
//     funding queries.
//   - [HtlcApi] — "embedded.htlc". Hash-time-locked contracts.
//   - [BridgeApi] — "embedded.bridge". Cross-chain bridge state
//     (orchestrator, networks, wraps, unwraps).
//   - [LiquidityApi] — "embedded.liquidity". Liquidity-mining state.
//
// # Conventions
//
// Each constructor takes the ambient [zenon.Zenon] handle and caches
// the chain / consensus pointers it needs. APIs are stateless after
// construction; safe for concurrent use.
//
// All paginated endpoints honour the same [api.RpcMaxPageSize]=1024
// cap as the parent package. Big.Int values render as decimal
// strings via -Marshal twin types — the same convention used
// throughout the rpc/api surface.
//
// Read methods route through [api.GetFrontierContext] to obtain a
// VM-level view of the contract's storage at the chain head.
//
// # Generated Files
//
// None. Files are Zenon-specific (no upstream header).
//
// # Related Packages
//
//   - [github.com/zenon-network/go-zenon/rpc/api] — parent ledger /
//     stats namespaces; supplies the shared types and the
//     PublishRawTransaction write path.
//   - [github.com/zenon-network/go-zenon/vm/embedded/definition] —
//     storage layout these wrappers read from.
//   - [github.com/zenon-network/go-zenon/vm/embedded/implementation]
//     — receive-side execution that produces the storage these APIs
//     read.
package embedded
