package pillar

import (
	"fmt"

	"github.com/pkg/errors"

	"github.com/zenon-network/go-zenon/chain/nom"
	"github.com/zenon-network/go-zenon/chain/store"
	"github.com/zenon-network/go-zenon/common"
	"github.com/zenon-network/go-zenon/common/types"
)

// ErrNothingToGenerate is the soft sentinel returned when the
// contract's sequencer queue is empty — the auto-receive sweep
// uses it to skip the contract without aborting the sweep.
var (
	ErrNothingToGenerate = errors.Errorf("nothing to generate. sequencer-queue is empty")
)

// generateNext produces one auto-receive block for the next pending
// send addressed to embedded. Returns ErrNothingToGenerate when
// the contract's mailbox is empty; any other error is fatal to the
// sweep.
func (w *worker) generateNext(momentumStore store.Momentum, embedded types.Address) (*nom.AccountBlockTransaction, error) {
	insert := w.chain.AcquireInsert("contract-generator")
	defer insert.Unlock()

	store := w.chain.GetFrontierAccountStore(embedded)
	toReceive := store.SequencerFront(momentumStore.GetAccountMailbox(embedded))

	if toReceive == nil {
		return nil, ErrNothingToGenerate
	}

	sendBlock, err := momentumStore.GetAccountBlock(*toReceive)
	common.DealWithErr(err)
	if sendBlock == nil {
		return nil, errors.Errorf("can't get block but it exists in sequencer")
	}
	log := w.log.New("send-block-header", sendBlock.Header())
	executionResult, err := w.supervisor.GenerateAutoReceive(sendBlock)

	// check for internal errors
	if err != nil {
		return nil, fmt.Errorf("failed to generate block. Reason: %w", err)
	}

	if executionResult.Transaction != nil {
		block := executionResult.Transaction.Block
		log.Info("generated embedded-block", "identifier", block.Header(), "send-block-hash", sendBlock.Hash, "returned-error", executionResult.ReturnedError)
		return executionResult.Transaction, nil
	} else {
		return nil, errors.Errorf("no block has been returned by VM. Impossible")
	}
}
