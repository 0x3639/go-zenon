package vm_context

import (
	"github.com/zenon-network/go-zenon/common"
	"github.com/zenon-network/go-zenon/common/types"
)

// IsAcceleratorSporkEnforced reports whether the accelerator spork is
// active at this context's momentum height. Panics through
// [common.DealWithErr] on a momentum-store read failure (which would
// indicate corrupt state).
func (ctx *accountVmContext) IsAcceleratorSporkEnforced() bool {
	active, err := ctx.momentumStore.IsSporkActive(types.AcceleratorSpork)
	common.DealWithErr(err)
	return active
}

// IsHtlcSporkEnforced reports whether the HTLC spork is active.
func (ctx *accountVmContext) IsHtlcSporkEnforced() bool {
	active, err := ctx.momentumStore.IsSporkActive(types.HtlcSpork)
	common.DealWithErr(err)
	return active
}

// IsBridgeAndLiquiditySporkEnforced reports whether the bridge +
// liquidity spork is active.
func (ctx *accountVmContext) IsBridgeAndLiquiditySporkEnforced() bool {
	active, err := ctx.momentumStore.IsSporkActive(types.BridgeAndLiquiditySpork)
	common.DealWithErr(err)
	return active
}
