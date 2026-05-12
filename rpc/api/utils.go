package api

import (
	"github.com/pkg/errors"

	"github.com/zenon-network/go-zenon/chain"
	"github.com/zenon-network/go-zenon/chain/nom"
	"github.com/zenon-network/go-zenon/common/types"
	"github.com/zenon-network/go-zenon/vm/vm_context"
)

// Per-call caps shared across the package's paged readers. Callers
// passing a pageSize / count above these are rejected with
// ErrPageSizeParamTooBig / ErrCountParamTooBig before any chain
// read.
const (
	// RpcMaxPageSize bounds (pageIndex, pageSize)-style readers.
	RpcMaxPageSize = 1024
	// RpcMaxCountSize bounds (height, count)-style readers.
	RpcMaxCountSize = 1024
)

// GetRange computes the [start, end) slice bounds of the page
// identified by (index, count) over a list of length listLen.
//
// Returns (listLen, listLen) when start is past the end of the
// list (empty page beyond the data), and (start, listLen) when
// the page is partially populated at the tail. Otherwise returns
// (start, start+count). The returned start is always <= end, so
// callers can use list[start:end] without an extra bounds check.
func GetRange(index, count, listLen uint32) (uint32, uint32) {
	start := index * count
	if start >= listLen {
		return listLen, listLen
	}
	end := start + count
	if end >= listLen {
		return start, listLen
	}
	return start, end
}

// GetFrontierContext returns the current frontier momentum and an
// AccountVmContext scoped to addr's account store at that frontier.
// Used by every read-only handler in this package and in
// rpc/api/embedded as the entry point for chain access: handlers
// receive a momentum to attach as the as-of point, and a context
// to feed to the relevant definition.* / implementation.* readers.
//
// Returns (nil, nil, err) if the frontier momentum is not
// available; the AccountVmContext is built unconditionally once
// the frontier read succeeds and is safe to use even if addr has
// no on-chain state.
func GetFrontierContext(c chain.Chain, addr types.Address) (*nom.Momentum, vm_context.AccountVmContext, error) {
	store := c.GetFrontierMomentumStore()

	frontier, err := store.GetFrontierMomentum()
	if err != nil {
		return nil, nil, err
	}

	context := vm_context.NewAccountContext(
		store,
		c.GetFrontierAccountStore(addr),
		nil,
	)
	return frontier, context, nil
}

func checkTokenIdValid(chain chain.Chain, ts *types.ZenonTokenStandard) error {
	store := chain.GetFrontierMomentumStore()
	if ts != nil && (*ts) != types.ZeroTokenStandard {
		tokenStandard, err := store.GetTokenInfoByTs(*ts)
		if err != nil {
			return err
		}
		if tokenStandard == nil {
			return errors.New("ts doesn’t exist")
		}
	}
	return nil
}
