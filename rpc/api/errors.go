package api

import (
	"github.com/zenon-network/go-zenon/common"
)

// JSON-RPC -32000 ("application error") codes returned for bad
// request parameters. Each is rejected before any state is touched.
var (
	// ErrPageSizeParamTooBig — pageSize exceeded the per-method cap
	// (typically RpcMaxPageSize=1024).
	ErrPageSizeParamTooBig = common.NewErrorWCode(-32000, "page-size parameter is too big")
	// ErrPageIndexParamTooBig — pageIndex past the configured upper
	// bound (e.g. unreceivedMaxPageIndex for unreceived-block
	// queries).
	ErrPageIndexParamTooBig = common.NewErrorWCode(-32000, "page-index parameter is too big")
	// ErrCountParamTooBig — count parameter exceeds RpcMaxCountSize.
	ErrCountParamTooBig = common.NewErrorWCode(-32000, "count parameter is too big")
	// ErrHeightParamIsZero — height==0 is reserved as a sentinel and
	// not a valid query target.
	ErrHeightParamIsZero = common.NewErrorWCode(-32000, "height parameter must be strictly greater than zero")
	// ErrParamIsNull — a required parameter was nil.
	ErrParamIsNull = common.NewErrorWCode(-32000, "parameter must not be null")
)
