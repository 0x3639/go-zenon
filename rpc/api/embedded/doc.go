// Package embedded implements the RPC layer for the embedded smart
// contracts that live at fixed addresses on the Zenon chain (pillar,
// sentinel, stake, token, plasma, spork, accelerator, htlc, bridge,
// liquidity, swap).
//
// One Api struct per contract
//
// Each contract has its own handler struct (PillarApi, BridgeApi, …)
// constructed via the corresponding New<Name>Api factory. The struct
// holds a chain.Chain handle (frontier reads) and, where relevant, a
// consensus.Consensus handle (epoch tickers and pillar weights). A
// log15.Logger from common.RPCLogger is attached per submodule so RPC
// errors surface with module context.
//
// Per-call pattern
//
// A typical handler method:
//
//  1. Acquires a frontier read context for the target contract address
//     via api.GetFrontierContext, returning the frontier momentum and
//     a vm_context.AccountVmContext.
//  2. Reads from the context's Storage() through one or more
//     definition.* helpers — these are the canonical ABI-aware
//     readers for the contract's persisted state.
//  3. Optionally calls implementation.* for derived computations
//     (revoke-cooldown evaluation, decay application, plasma
//     calculations) that need more than a raw storage read.
//  4. Pages the result with api.GetRange when the response is a list,
//     rejecting page sizes above api.RpcMaxPageSize with
//     api.ErrPageSizeParamTooBig.
//  5. Wraps the result in a response struct defined in this package.
//
// Response types and Marshal twins
//
// Many response structs in this package come in pairs:
// <Name> uses *big.Int for token amounts (so the runtime keeps
// arbitrary precision) and <Name>Marshal uses string-encoded
// decimals for the same fields. The MarshalJSON / UnmarshalJSON
// methods on the public type bridge between the two so JSON-RPC
// clients receive amounts as strings (avoiding 2^53 float precision
// loss) while in-process code keeps *big.Int.
//
// The convention is that the conversion method is named
// To<TypeName>Marshal — a few methods in this package use names
// like ToStakeEntryMarshal or ToRewardDepositMarshal that do not
// match their <TypeName> exactly; those are pre-existing names
// preserved to avoid breaking callers.
//
// What this package does NOT do
//
// Embedded-contract state mutation flows through send-block
// submission, not through this package; the handlers here are
// read-only. Method-call validation and execution live in
// vm/embedded/implementation. ABI encoding/decoding for state
// records lives in vm/embedded/definition. The HTTP/WebSocket
// transport and JSON-RPC routing live one level up in rpc/server
// and rpc/api.
package embedded
