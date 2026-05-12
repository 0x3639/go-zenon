// Package api implements the core (non-embedded) RPC handler
// surface for znnd: the ledger read/write API, the stats /
// introspection API, and the shared types those handlers emit
// over JSON-RPC. Per-contract embedded handlers live in the
// rpc/api/embedded subpackage; the subscription/pub-sub server
// lives in rpc/api/subscribe.
//
// # Per-handler structure
//
// Each handler is a small struct constructed via a New<Name>Api
// factory that captures a zenon.Zenon handle (and, for StatsApi,
// the p2p.Server). The factories are wired up one level up in
// rpc/apis.go by binding them to JSON-RPC namespaces ("ledger" for
// LedgerApi, "stats" for StatsApi). A log15.Logger from
// common.RPCLogger is attached per submodule so handler errors
// surface with module context.
//
// # Wire types and Marshal twins
//
// Response shapes that carry token amounts use *big.Int internally
// (so the runtime keeps arbitrary precision) and emit decimal
// strings on the wire. Each public type that needs this comes
// paired with a <TypeName>Marshal twin and round-trips through
// MarshalJSON / UnmarshalJSON on the public type. The convention
// is documented per-method on the affected types
// (AccountBlock, AccountBlockList, BalanceInfo, Token).
//
// # Page-size and count guards
//
// List-returning methods accept a (pageIndex, pageSize) pair or a
// (height, count) pair. RpcMaxPageSize (1024) and RpcMaxCountSize
// (1024) cap the per-call result size; methods reject larger
// requests with ErrPageSizeParamTooBig or ErrCountParamTooBig
// before any chain read. ErrHeightParamIsZero rejects a height
// argument of zero where the underlying store treats height 0 as
// "no record" rather than as a valid key.
//
// # Frontier-only reads
//
// GetFrontierContext returns the momentum and an AccountVmContext
// pinned to the current frontier momentum. All handlers in this
// package read against the frontier; there is no historical
// momentum or per-snapshot reader exposed here.
package api
