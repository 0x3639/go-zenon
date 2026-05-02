// Package abi encodes and decodes embedded-contract method calls
// and storage records in a format compatible with the Solidity ABI.
//
// # Overview
//
// abi mirrors the encoding rules used by EVM tooling so that
// contract clients can pack call data with familiar libraries. It
// supports the type set used by Zenon's embedded contracts: signed
// and unsigned integers in every Solidity-supported size, address,
// tokenStandard, hash, bytes (fixed and dynamic), strings, booleans,
// and arbitrary nesting through fixed-size arrays and dynamic slices.
//
// # Key Concepts
//
//   - ABIContract — the parsed contract surface. Holds [Method]s
//     (function entries) and [Variable]s (storage shapes used by
//     embedded contracts to encode their on-chain records). Loaded
//     from JSON via [JSONToABIContract] / [ABIContract.UnmarshalJSON].
//   - Method — one ABI function entry. Tracks the canonical
//     signature, the 4-byte method id (call selector), and the
//     argument tuple ([Arguments]).
//   - Variable — one storage record shape. Used by embedded
//     contracts to (de)serialize records without ever calling a
//     function.
//   - Type — the parsed type descriptor. Supports the Solidity
//     type set; `tokenStandard` and `hash` are Zenon-specific
//     additions.
//   - IntPool — pool of [big.Int] handles consumed by the
//     unpacker's [lengthPrefixPointsTo] arithmetic to amortize
//     allocations. Wrapped in [intPoolPool] for cross-call reuse.
//
// # Encoding Layout
//
// Method calls: `4-byte id || head_args || tail_args`. Head args
// occupy [WordSize] each (32 bytes); dynamic args (string, bytes,
// dynamic slices) place a 32-byte offset in the head and append
// their `[length, padded_value]` tail at the end.
//
// Static arrays of static elements are encoded inline in the head
// with no length prefix. Mixed/nested cases follow Solidity's
// "static elements inline; dynamic elements pointer-then-tail" rule.
//
// # Usage
//
// Build calls and decode replies:
//
//	contract := abi.JSONToABIContract(jsonReader)
//	data, _ := contract.PackMethod("Transfer", to, amount)
//	var args struct { To types.Address; Amount *big.Int }
//	_ = contract.UnpackMethod(&args, "Transfer", inboundData)
//
// Read/write embedded-contract storage:
//
//	encoded, _ := contract.PackVariable("PillarInfo", info)
//	var info definition.PillarInfo
//	_ = contract.UnpackVariable(&info, "PillarInfo", encoded)
//
// # Related Packages
//
//   - [github.com/zenon-network/go-zenon/vm/embedded] — primary
//     consumer; every embedded contract owns an [ABIContract] for
//     dispatch and storage.
//   - [github.com/zenon-network/go-zenon/common/types] — supplies
//     [types.Address], [types.ZenonTokenStandard], [types.Hash]
//     handled as first-class ABI types.
//   - [github.com/zenon-network/go-zenon/common/crypto] — supplies
//     the hash function the codec uses to derive method ids.
package abi
