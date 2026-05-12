package api

import (
	"github.com/zenon-network/go-zenon/common"
)

// Shared JSON-RPC argument-validation errors. All carry the JSON-RPC
// error code -32000 (server error, application-defined per JSON-RPC
// 2.0 §5.1), so they surface to clients as "Server error" with the
// human message below.
var (
	// ErrPageSizeParamTooBig is returned when a caller's pageSize
	// exceeds RpcMaxPageSize on a paged read.
	ErrPageSizeParamTooBig = common.NewErrorWCode(-32000, "page-size parameter is too big")

	// ErrPageIndexParamTooBig is returned when a caller's pageIndex
	// exceeds a per-method ceiling (currently only the unreceived
	// blocks query in LedgerApi.GetUnreceivedBlocksByAddress, where
	// page index >= 10 is rejected).
	ErrPageIndexParamTooBig = common.NewErrorWCode(-32000, "page-index parameter is too big")

	// ErrCountParamTooBig is returned by height-based readers when
	// count exceeds RpcMaxCountSize.
	ErrCountParamTooBig = common.NewErrorWCode(-32000, "count parameter is too big")

	// ErrHeightParamIsZero is returned by height-based readers when
	// height == 0. Heights in this codebase are 1-indexed; a zero
	// height is treated as "no record" by the backing store and is
	// rejected at the RPC layer to avoid silently returning empty
	// results.
	ErrHeightParamIsZero = common.NewErrorWCode(-32000, "height parameter must be strictly greater than zero")

	// ErrParamIsNull is returned when a required body parameter
	// (currently only LedgerApi.PublishRawTransaction's block) is
	// nil after JSON decoding.
	ErrParamIsNull = common.NewErrorWCode(-32000, "parameter must not be null")
)
