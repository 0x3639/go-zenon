package api

import (
	"github.com/pkg/errors"

	"github.com/zenon-network/go-zenon/chain"
	"github.com/zenon-network/go-zenon/chain/nom"
	"github.com/zenon-network/go-zenon/common/types"
	"github.com/zenon-network/go-zenon/vm/vm_context"
)

const (
	// RpcMaxPageSize caps the page-size accepted by paginated
	// endpoints; oversize requests yield [ErrPageSizeParamTooBig].
	RpcMaxPageSize = 1024
	// RpcMaxCountSize caps the count parameter on range-style
	// endpoints; oversize requests yield [ErrCountParamTooBig].
	RpcMaxCountSize = 1024
)

// GetRange computes the [start, end) slice indices for a paginated
// query, clamping to listLen so callers never overrun. start==end is
// returned for out-of-range pages, signalling an empty result.
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

// GetFrontierContext returns the current chain head momentum and a
// read-only VM context bound to addr's frontier state. Used by
// embedded-contract RPC wrappers that need to call into VM-level
// view methods at the chain tip.
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

// checkTokenIdValid validates that ts (when non-nil and non-zero)
// resolves to a known token in the frontier momentum store. Used by
// PublishRawTransaction to reject blocks that reference a
// non-existent token before they enter VM dispatch.
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
