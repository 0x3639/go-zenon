package pillar

import (
	"github.com/zenon-network/go-zenon/chain"
	"github.com/zenon-network/go-zenon/chain/nom"
	"github.com/zenon-network/go-zenon/chain/store"
	"github.com/zenon-network/go-zenon/common/types"
	"github.com/zenon-network/go-zenon/vm/constants"
	"github.com/zenon-network/go-zenon/vm/embedded/definition"
	"github.com/zenon-network/go-zenon/vm/embedded/implementation"
	"github.com/zenon-network/go-zenon/vm/vm_context"
)

// canPerformEmbeddedUpdate asks the contract whether its periodic
// Update method is currently callable (returns nil) or is
// suppressed by cooldown / missing-method (returns the relevant
// constants.Err* value).
func canPerformEmbeddedUpdate(momentumStore store.Momentum, pool chain.AccountPool, contract types.Address) error {
	store := pool.GetFrontierAccountStore(contract)
	context := vm_context.NewAccountContext(momentumStore, store, nil)
	return implementation.CanPerformUpdate(context)
}

// updateContracts walks every embedded contract that participates
// in periodic updates ([types.EmbeddedWUpdate]) and produces a
// signed Update send-block whenever the cooldown has elapsed.
// Cooldown / method-not-found errors are swallowed; any other
// error aborts the sweep.
func (w *worker) updateContracts(momentumStore store.Momentum) error {
	for _, address := range types.EmbeddedWUpdate {
		if err := canPerformEmbeddedUpdate(momentumStore, w.chain, address); err == nil {
			w.log.Info("producing block to update embedded-contract", "contract-address", address)
			if block, err := w.supervisor.GenerateFromTemplate(&nom.AccountBlock{
				BlockType: nom.BlockTypeUserSend,
				Address:   w.coinbase.Address,
				ToAddress: address,
				Data:      definition.ABICommon.PackMethodPanic(definition.UpdateMethodName),
			}, w.coinbase.Signer); err != nil {
				return err
			} else {
				w.broadcaster.CreateAccountBlock(block)
			}
		} else if err == constants.ErrUpdateTooRecent || err == constants.ErrContractMethodNotFound {
		} else {
			return err
		}
	}
	return nil
}
