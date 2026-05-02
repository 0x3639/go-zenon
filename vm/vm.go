package vm

import (
	"math/big"

	"github.com/pkg/errors"

	"github.com/zenon-network/go-zenon/chain"
	"github.com/zenon-network/go-zenon/chain/nom"
	"github.com/zenon-network/go-zenon/common"
	"github.com/zenon-network/go-zenon/common/db"
	"github.com/zenon-network/go-zenon/common/types"
	"github.com/zenon-network/go-zenon/vm/constants"
	"github.com/zenon-network/go-zenon/vm/embedded"
	"github.com/zenon-network/go-zenon/vm/vm_context"
)

// log is the package-level VM logger; alias of [common.VmLogger].
var (
	log = common.VmLogger
)

// Result codes embedded in a contract-receive block's [nom.AccountBlock.Data]
// (encoded as a uint64) to advertise whether the embedded execution
// succeeded or failed. resultInvalid is the zero value and should never
// appear on a finalized block.
const (
	resultInvalid uint64 = iota
	resultSuccess
	resultFail
)

// errToStatus collapses a method error into one of the result codes
// stored in the contract-receive block's data. Any non-nil error
// becomes resultFail; nil becomes resultSuccess.
func errToStatus(err error) uint64 {
	switch err {
	case nil:
		return resultSuccess
	default:
		return resultFail
	}
}

// VM is the per-account-block execution machine: it holds the
// [vm_context.AccountVmContext] for the block under execution and
// exposes the four block-type entry points
// ([VM.applySend], [VM.applyReceive], and the two contract-receive
// flows). One VM is constructed per call to
// [Supervisor.applyBlock] / [Supervisor.GenerateAutoReceive].
type VM struct {
	context vm_context.AccountVmContext
}

// NewVM wraps context in a VM ready to execute one account block.
func NewVM(context vm_context.AccountVmContext) *VM {
	return &VM{
		context: context,
	}
}

// enoughPlasma checks the block's plasma claim against context state:
// embedded contracts have unlimited plasma; user accounts must have
// enough fused-or-PoW plasma to cover the block's base cost. Updates
// block.TotalPlasma and block.BasePlasma in place and credits the
// account's per-chain plasma counter on success.
//
// Returns one of [constants.ErrNotEnoughPlasma],
// [constants.ErrBlockPlasmaLimitReached], or
// [constants.ErrNotEnoughTotalPlasma] on rejection.
func enoughPlasma(context vm_context.AccountVmContext, block *nom.AccountBlock) error {
	// embedded address have unlimited plasma
	if types.IsEmbeddedAddress(block.Address) {
		return nil
	}

	available, err := AvailablePlasma(context.MomentumStore(), context)
	common.DealWithErr(err)
	if available < block.FusedPlasma {
		return constants.ErrNotEnoughPlasma
	}

	powPlasma := DifficultyToPlasma(block.Difficulty)
	block.TotalPlasma = powPlasma + block.FusedPlasma
	if block.TotalPlasma > constants.MaxPlasmaForAccountBlock {
		return constants.ErrBlockPlasmaLimitReached
	}

	block.BasePlasma, err = GetBasePlasmaForAccountBlock(context, block)
	common.DealWithErr(err)

	if block.TotalPlasma < block.BasePlasma {
		return constants.ErrNotEnoughTotalPlasma
	}

	return context.AddChainPlasma(block.FusedPlasma)
}

// enoughFunds reports whether the executing account has at least
// block.Amount of block.TokenStandard. Always true for zero-amount
// transfers (which carry no token standard).
func enoughFunds(context vm_context.AccountVmContext, block *nom.AccountBlock) bool {
	if block.TokenStandard == types.ZeroTokenStandard {
		return true
	}

	balance, err := context.GetBalance(block.TokenStandard)
	common.DealWithErr(err)
	if balance.Cmp(block.Amount) == -1 {
		return false
	}

	return true
}

// applyBlock applies block on top of vm.context. After applyBlock
// returns, vm.context.Changes() carries the patch needed to assemble
// a [nom.AccountBlockTransaction].
//
// Dispatches by block type: send/contract-send paths run
// [VM.applySend], user receives run [VM.applyReceive], and contract
// receives go through [VM.generateEmbeddedReceive] then verify the
// generated block matches the inbound one.
func (vm *VM) applyBlock(block *nom.AccountBlock) error {
	if err := enoughPlasma(vm.context, block); err != nil {
		return err
	}

	// In case vm will update some fields of block, make a copy of block.
	switch block.BlockType {
	case nom.BlockTypeUserSend, nom.BlockTypeContractSend:
		return vm.applySend(block)
	case nom.BlockTypeUserReceive:
		return vm.applyReceive(block)
	case nom.BlockTypeContractReceive:
		generated, _, err := vm.generateEmbeddedReceive(block.FromBlockHash)
		if err != nil {
			return err
		}
		if generated.ChangesHash != block.ChangesHash {
			return errors.Errorf("auto-received block has different changes-hash expected %v but got %v", generated.ChangesHash, block.ChangesHash)
		}
		computed := generated.ComputeHash()
		if computed != block.Hash {
			return errors.Errorf("auto-received block has different hash expected %v but got %v", computed, generated)
		}
		return nil
	default:
		panic("unknown block type")
	}
}

// applySend executes a send (user or contract): if the recipient is
// an embedded contract, runs the contract method's
// [embedded.Method.ValidateSendBlock] precondition; then deducts
// block.Amount of block.TokenStandard from the sender's balance.
// Returns [constants.ErrInsufficientBalance] when the sender's
// balance is below block.Amount.
func (vm *VM) applySend(block *nom.AccountBlock) error {
	// check can make transaction
	if method, err := embedded.GetEmbeddedMethod(vm.context, block.ToAddress, block.Data); err != constants.ErrNotContractAddress {
		if err != nil {
			return err
		}

		// validate block
		err = method.ValidateSendBlock(block)
		if err != nil {
			return err
		}
	}

	// affect balance
	if !enoughFunds(vm.context, block) {
		return constants.ErrInsufficientBalance
	}

	vm.context.SubBalance(&block.TokenStandard, block.Amount)

	return nil
}

// applyReceive executes a user receive: marks the inbound send as
// consumed and credits its amount/token to the recipient's balance.
func (vm *VM) applyReceive(block *nom.AccountBlock) error {
	fromBlock, err := vm.context.MomentumStore().GetAccountBlockByHash(block.FromBlockHash)
	if err != nil {
		return err
	}

	err = vm.context.MarkAsReceived(block.FromBlockHash)
	if err != nil {
		return err
	}

	vm.context.AddBalance(&fromBlock.TokenStandard, fromBlock.Amount)
	return nil
}

// generateEmbeddedReceive synthesizes the contract-receive block for
// the inbound send identified by fromBlockHash. The receive is
// auto-generated (contracts don't sign), so the caller only needs the
// originating send hash.
//
// Flow:
//   - Pop the head of the contract's sequencer queue (mailbox order).
//   - Resolve the embedded method; if the method has been removed
//     by a spork between send and receive, [VM.rollbackEmbedded]
//     refunds and returns.
//   - Save state, credit the contract's balance, run the contract
//     method, apply every emitted descendant send, and finalize.
//
// Returns (generatedBlock, methodErr, internalErr). methodErr is the
// contract's own returned error (failure path) and internalErr is a
// VM-internal failure (the only one currently is a refund-descendant
// failure during rollback).
//
// generateEmbeddedReceive is used to generate the embedded receive
// nom.AccountBlock from a fromBlockHash. After calling applyBlock
// vm.context.Changes() has all the changes necessary to create a
// nom.AccountBlockTransaction.
func (vm *VM) generateEmbeddedReceive(fromBlockHash types.Hash) (*nom.AccountBlock, error, error) {
	// mark block as received (only for contracts, using sequencer)
	vm.context.SequencerPopFront()

	sendBlock, err := vm.context.MomentumStore().GetAccountBlockByHash(fromBlockHash)
	if err != nil {
		return nil, nil, err
	}
	method, err := embedded.GetEmbeddedMethod(vm.context, sendBlock.ToAddress, sendBlock.Data)

	// can happen when a method is deleted in a spork (height 100) and someone calls it before the spork (height 95)
	// and the autoReceive uses momentum height 105 for various reasons
	if err == constants.ErrContractMethodNotFound {
		return vm.rollbackEmbedded(fromBlockHash, err)
	}

	vm.context.Save()
	// balance
	vm.context.AddBalance(&sendBlock.TokenStandard, sendBlock.Amount)
	// call code
	descendantBlocks, err := method.ReceiveBlock(vm.context, sendBlock)
	if err != nil {
		return vm.rollbackEmbedded(fromBlockHash, err)
	}
	// apply send-descendant-blocks
	for _, dblock := range descendantBlocks {
		err := vm.applySend(dblock)
		if err != nil {
			return vm.rollbackEmbedded(fromBlockHash, err)
		}
	}

	// everything went right, no rollback required
	vm.context.Done()
	return vm.finalizeEmbedded(fromBlockHash, descendantBlocks, nil)
}

// rollbackEmbedded reverts the in-flight contract-receive context and
// emits a refund descendant when the originating send carried tokens.
// methodErr is propagated to [VM.finalizeEmbedded] so the resulting
// block records resultFail. The internal error return covers refund
// descendant failures (rare; a contract running out of balance to
// refund itself).
func (vm *VM) rollbackEmbedded(fromBlockHash types.Hash, methodErr error) (*nom.AccountBlock, error, error) {
	sendBlock, err := vm.context.MomentumStore().GetAccountBlockByHash(fromBlockHash)
	common.DealWithErr(err) // impossible to not find send-block at rollback

	vm.context.Reset()
	// If sendBlock contains amount, add current amount to embedded to be able to refund it
	// This operation was rollbacked with vm.context.Reset()
	vm.context.AddBalance(&sendBlock.TokenStandard, sendBlock.Amount)
	descendantBlocks := make([]*nom.AccountBlock, 0, 1)

	// If sendBlock contained tokens, refund them
	if sendBlock.Amount.Sign() > 0 {
		dBlock := &nom.AccountBlock{
			BlockType:     nom.BlockTypeContractSend,
			Address:       sendBlock.ToAddress,
			ToAddress:     sendBlock.Address,
			Amount:        new(big.Int).Set(sendBlock.Amount),
			TokenStandard: sendBlock.TokenStandard,
		}

		err := vm.applySend(dBlock)
		if err != nil {
			log.Error("Unable to apply descendant blocks for refund", "reason", err, "send-block-hash", sendBlock.Hash)
			return nil, nil, err
		}

		descendantBlocks = append(descendantBlocks, dBlock)
	}

	return vm.finalizeEmbedded(fromBlockHash, descendantBlocks, methodErr)
}

// finalizeEmbedded fills in the synthesized contract-receive block and
// every descendant: assigns versions, chain identifier, the
// contract's address, MomentumAcknowledged (the current frontier),
// previous-hash linkage along the chain of (descendant₀, …,
// descendantₙ, parent), data (the result code from executionError),
// and the canonical hash. Returns the parent receive block and the
// preserved executionError so callers know the contract reported a
// failure even though the receive itself was committed.
func (vm *VM) finalizeEmbedded(fromBlockHash types.Hash, descendantBlocks []*nom.AccountBlock, executionError error) (*nom.AccountBlock, error, error) {
	var err error

	prevFrontier, err := vm.context.Frontier()
	common.DealWithErr(err)
	prevHash := types.ZeroHash
	height := uint64(1)
	if prevFrontier != nil {
		prevHash = prevFrontier.Hash
		height = prevFrontier.Height + 1
	}

	momentum, err := vm.context.MomentumStore().GetFrontierMomentum()
	common.DealWithErr(err)

	for _, dblock := range descendantBlocks {
		dblock.Version = 1
		dblock.ChainIdentifier = vm.context.MomentumStore().ChainIdentifier()
		dblock.BlockType = nom.BlockTypeContractSend
		dblock.Address = *vm.context.Address()
		dblock.MomentumAcknowledged = momentum.Identifier()
		dblock.PreviousHash = prevHash
		dblock.Height = height
		dblock.ChangesHash = types.ZeroHash
		dblock.Hash = dblock.ComputeHash()
		prevHash = dblock.Hash
		height = height + 1
	}

	changes, err := vm.context.Changes()
	common.DealWithErr(err)
	block := &nom.AccountBlock{
		Version:              1,
		ChainIdentifier:      vm.context.MomentumStore().ChainIdentifier(),
		BlockType:            nom.BlockTypeContractReceive,
		Address:              *vm.context.Address(),
		FromBlockHash:        fromBlockHash,
		MomentumAcknowledged: momentum.Identifier(),
		PreviousHash:         prevHash,
		Height:               height,
		Data:                 common.Uint64ToBytes(errToStatus(executionError)),
		DescendantBlocks:     descendantBlocks,
		ChangesHash:          db.PatchHash(changes),
	}

	block.Hash = block.ComputeHash()
	return block, executionError, nil
}

// MomentumVM is the momentum-level execution machine: it commits one
// momentum's worth of (already-validated) account-block transactions
// atomically into the underlying momentum context so the resulting
// patch can be paired with the momentum into a
// [nom.MomentumTransaction].
type MomentumVM struct {
	context vm_context.MomentumVMContext
}

// NewMomentumVM wraps context in a [MomentumVM].
func NewMomentumVM(context vm_context.MomentumVMContext) *MomentumVM {
	return &MomentumVM{
		context: context,
	}
}

// applyMomentum walks the momentum's content list and admits each
// (header, patch) pair into the context via
// [store.Momentum.AddAccountBlockTransaction].
//
// Caller is responsible for verifying the momentum first; applyMomentum
// is a pure state-application step.
func (vm *MomentumVM) applyMomentum(pool chain.AccountPool, momentum *nom.Momentum) error {
	momentumStore := vm.context

	for _, header := range momentum.Content {
		if err := momentumStore.AddAccountBlockTransaction(*header, pool.GetPatch(header.Address, header.Identifier())); err != nil {
			return err
		}
	}

	return nil
}
