package vm_context

import (
	"github.com/zenon-network/go-zenon/common"
)

// Save snapshots the working account view: parks the existing handle
// in accountStoreSnapshot and replaces the working handle with a
// fresh snapshot that subsequent mutations write into. Called before
// invoking a contract method so the supervisor can reset on failure.
func (ctx *accountVmContext) Save() {
	s := ctx.Account.Snapshot()
	ctx.accountStoreSnapshot = ctx.Account
	ctx.Account = s
}

// Reset reverts to the saved snapshot, discarding everything written
// since the matching [accountVmContext.Save]. Used by the rollback
// path after a contract method fails.
func (ctx *accountVmContext) Reset() {
	ctx.Account = ctx.accountStoreSnapshot
	ctx.accountStoreSnapshot = nil
}

// Done finalizes the saved snapshot: extracts the changes patch
// from the working view, restores the saved view as the current
// handle, and applies the patch onto it. The contract method's
// successful effects propagate to the underlying store.
//
// Panics through [common.DealWithErr] on apply failure.
func (ctx *accountVmContext) Done() {
	changes, _ := ctx.Account.Changes()
	ctx.Account = ctx.accountStoreSnapshot
	common.DealWithErr(ctx.Account.Apply(changes))
}
