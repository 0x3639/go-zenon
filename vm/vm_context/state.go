package vm_context

import (
	"github.com/zenon-network/go-zenon/chain/nom"
)

// GetFrontierMomentum returns the most recent committed momentum from
// the underlying momentum store.
func (ctx *accountVmContext) GetFrontierMomentum() (*nom.Momentum, error) {
	return ctx.momentumStore.GetFrontierMomentum()
}

// GetGenesisMomentum returns the genesis momentum from the underlying
// momentum store.
func (ctx *accountVmContext) GetGenesisMomentum() *nom.Momentum {
	return ctx.momentumStore.GetGenesisMomentum()
}
