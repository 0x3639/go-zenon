package vm_context

import (
	"math/big"

	"github.com/zenon-network/go-zenon/common"
	"github.com/zenon-network/go-zenon/common/types"
)

// AddBalance credits amount of ts to the executing account by reading
// the current balance, adding, and writing it back. Panics through
// [common.DealWithErr] on read/write failure — those errors are bugs
// at this point.
func (ctx *accountVmContext) AddBalance(ts *types.ZenonTokenStandard, amount *big.Int) {
	b, err := ctx.GetBalance(*ts)
	common.DealWithErr(err)
	b.Add(b, amount)
	common.DealWithErr(ctx.SetBalance(*ts, b))
}

// SubBalance debits amount of ts from the executing account. Panics
// if the resulting balance would be negative — the VM checks
// enoughFunds before calling this, so an underflow here is a logic
// error.
func (ctx *accountVmContext) SubBalance(ts *types.ZenonTokenStandard, amount *big.Int) {
	b, err := ctx.GetBalance(*ts)
	common.DealWithErr(err)
	if b.Cmp(amount) >= 0 {
		b.Sub(b, amount)
		common.DealWithErr(ctx.SetBalance(*ts, b))
	} else {
		panic("negative balance after sub")
	}
}
