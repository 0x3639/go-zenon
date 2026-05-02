package vm

import (
	"fmt"
	"math/big"
	"runtime/debug"

	"github.com/pkg/errors"

	"github.com/zenon-network/go-zenon/chain"
	"github.com/zenon-network/go-zenon/chain/nom"
	"github.com/zenon-network/go-zenon/common"
	"github.com/zenon-network/go-zenon/common/db"
	"github.com/zenon-network/go-zenon/common/types"
	"github.com/zenon-network/go-zenon/consensus"
	"github.com/zenon-network/go-zenon/verifier"
	"github.com/zenon-network/go-zenon/vm/constants"
	"github.com/zenon-network/go-zenon/vm/vm_context"
)

// SignFunc is the function type defining the callback when a block
// requires a method to sign the transaction in supervisor.
//
// Returns (signedData, address, publicKey, err). The address and
// public key are used by the supervisor to populate the account
// block's identity fields.
type SignFunc func(data []byte) (signedData []byte, addr *types.Address, pubkey []byte, err error)

// Supervisor is the VM-layer entry point: it routes blocks and
// momentums to the right execution context, runs the verifier before
// applying state, and packs the result into the
// [nom.AccountBlockTransaction] / [nom.MomentumTransaction] forms the
// chain layer commits.
//
// One supervisor instance is shared across the node lifetime; it is
// safe for concurrent use because every method constructs fresh
// contexts and the underlying chain/verifier/consensus handles are
// themselves goroutine-safe.
type Supervisor struct {
	log common.Logger

	chain     chain.Chain
	consensus consensus.Consensus
	verifier  verifier.Verifier
}

// ContractExecution is the result of an auto-receive: the produced
// transaction plus the contract method's returned error (if any).
// ReturnedError is informational — the transaction is still valid and
// will be committed regardless.
type ContractExecution struct {
	Transaction   *nom.AccountBlockTransaction
	ReturnedError error
}

// NewSupervisor wires a [Supervisor] over chain and consensus. The
// supervisor builds its own [verifier.Verifier] from those handles.
func NewSupervisor(chain chain.Chain, consensus consensus.Consensus) *Supervisor {
	return &Supervisor{
		log:       common.SupervisorLogger,
		chain:     chain,
		consensus: consensus,
		verifier:  verifier.NewVerifier(chain, consensus),
	}
}

// newBlockContext builds the per-account-block VM context from chain
// and consensus state. Panics on missing dependencies — they would
// indicate the verifier admitted a block whose preconditions were
// already broken.
func (s *Supervisor) newBlockContext(block *nom.AccountBlock) vm_context.AccountVmContext {
	momentumStore := s.chain.GetMomentumStore(block.MomentumAcknowledged)
	accountStore := s.chain.GetAccountStore(block.Address, block.Previous())
	cache := s.consensus.FixedPillarReader(block.MomentumAcknowledged)
	if momentumStore == nil {
		panic(fmt.Sprintf("can't find momentumStore for %v", block.MomentumAcknowledged))
	}
	if accountStore == nil {
		panic(fmt.Sprintf("can't find accountStore for %v %v", block.Address, block.Previous()))
	}
	if cache == nil {
		panic(fmt.Sprintf("can't find cache for %v", block.MomentumAcknowledged))
	}
	return vm_context.NewAccountContext(
		momentumStore,
		accountStore,
		cache,
	)
}

// newMomentumContext builds the per-momentum VM context anchored at
// the previous momentum's view (so the new momentum's content applies
// onto a snapshot taken just before it).
func (s *Supervisor) newMomentumContext(momentum *nom.Momentum) vm_context.MomentumVMContext {
	return vm_context.NewMomentumVMContext(
		s.chain.GetMomentumStore(momentum.Previous()),
	)
}

// ApplyBlock validates and executes block, returning the resulting
// [nom.AccountBlockTransaction]. Rejects [nom.BlockTypeContractSend]
// — those are nested inside a parent receive and must be applied
// together with it via the contract-receive path.
func (s *Supervisor) ApplyBlock(block *nom.AccountBlock) (*nom.AccountBlockTransaction, error) {
	if block.BlockType == nom.BlockTypeContractSend {
		return nil, errors.Errorf("can't apply BlockTypeContractSend")
	}
	return s.applyBlock(block, nil)
}

// ApplyMomentum verifies the momentum and applies its content to a
// fresh momentum context, returning the [nom.MomentumTransaction]
// (momentum + state patch) the chain layer commits.
//
// Wraps the work in a recover/panic-to-error guard so a VM panic
// becomes [constants.ErrVmRunPanic] for the caller rather than
// crashing the node.
func (s *Supervisor) ApplyMomentum(detailed *nom.DetailedMomentum) (result *nom.MomentumTransaction, internalErr error) {
	momentum := detailed.Momentum
	defer func() {
		if err := recover(); err != nil {
			s.log.Error("vm panic when applying momentum", "identifier", momentum.Identifier(), "reason", err, "stack", string(debug.Stack()))

			result = nil
			internalErr = constants.ErrVmRunPanic
		}
	}()

	if err := s.verifier.Momentum(detailed); err != nil {
		return nil, err
	}
	context := s.newMomentumContext(momentum)
	vm := NewMomentumVM(context)
	err := vm.applyMomentum(s.chain, momentum)
	if err != nil {
		return nil, err
	}
	transaction, err := s.packMomentum(context, momentum, nil, false)
	if err != nil {
		return nil, err
	}
	return transaction, nil
}

// GenerateFromTemplate fills in the missing fields of template
// (chain identifier, version, MomentumAcknowledged, previous hash,
// height, plasma) and runs the block through the apply pipeline,
// signing with signFunc.
func (s *Supervisor) GenerateFromTemplate(template *nom.AccountBlock, signFunc SignFunc) (*nom.AccountBlockTransaction, error) {
	if err := s.setAll(template); err != nil {
		return nil, err
	}
	context := s.newBlockContext(template)
	if err := s.setBlockPlasma(context, template); err != nil {
		return nil, err
	}
	return s.applyBlock(template, signFunc)
}

// GenerateAutoReceive synthesizes the contract-receive transaction
// for sendBlock. Used by the chain layer (and by the receive-mailbox
// drain in [github.com/zenon-network/go-zenon/pillar]) to create the
// auto-receive that completes a contract send.
func (s *Supervisor) GenerateAutoReceive(sendBlock *nom.AccountBlock) (*ContractExecution, error) {
	template := &nom.AccountBlock{
		BlockType:     nom.BlockTypeContractReceive,
		Address:       sendBlock.ToAddress,
		FromBlockHash: sendBlock.Hash,
	}
	if err := s.setAll(template); err != nil {
		return nil, err
	}

	if err := s.verifier.AccountBlock(template); err != nil {
		return nil, err
	}
	context := s.newBlockContext(template)
	if err := s.setBlockPlasma(context, template); err != nil {
		return nil, err
	}
	vm := NewVM(context)
	block, methodErr, err := vm.generateEmbeddedReceive(template.FromBlockHash)
	if err := s.verifier.AccountBlock(block); err != nil {
		return nil, err
	}

	if err != nil {
		return nil, err
	}
	transaction, err := s.packBlock(context, block, nil)
	if err != nil {
		return nil, err
	}

	return &ContractExecution{
		Transaction:   transaction,
		ReturnedError: methodErr,
	}, nil
}

// GenerateMomentum verifies, applies, and signs a momentum produced
// from detailed. Used by [github.com/zenon-network/go-zenon/pillar]
// when the local node is the elected producer for the momentum's
// tick.
func (s *Supervisor) GenerateMomentum(detailed *nom.DetailedMomentum, signFunc SignFunc) (result *nom.MomentumTransaction, internalErr error) {
	template := detailed.Momentum
	defer func() {
		if err := recover(); err != nil {
			s.log.Error("vm panic when applying momentum", "identifier", template.Identifier(), "reason", err, "stack", string(debug.Stack()))

			result = nil
			internalErr = constants.ErrVmRunPanic
		}
	}()

	if err := s.verifier.Momentum(detailed); err != nil {
		return nil, err
	}
	context := s.newMomentumContext(template)
	vm := NewMomentumVM(context)
	err := vm.applyMomentum(s.chain, template)
	if err != nil {
		return nil, err
	}
	transaction, err := s.packMomentum(context, template, signFunc, false)
	if err != nil {
		return nil, err
	}
	return transaction, nil
}

// GenerateGenesisMomentum builds the height-1 genesis transaction
// from the supplied template and account pool. Bypasses the verifier
// (genesis is the only momentum that does not flow through it) and
// the producer signature (no producer at genesis).
func (s *Supervisor) GenerateGenesisMomentum(template *nom.Momentum, pool chain.AccountPool) (result *nom.MomentumTransaction, internalErr error) {
	defer func() {
		if err := recover(); err != nil {
			s.log.Error("vm panic when applying momentum", "identifier", template.Identifier(), "reason", err, "stack", string(debug.Stack()))

			result = nil
			internalErr = constants.ErrVmRunPanic
		}
	}()

	context := vm_context.NewGenesisMomentumVMContext()
	vm := NewMomentumVM(context)
	err := vm.applyMomentum(pool, template)
	if err != nil {
		return nil, err
	}
	transaction, err := s.packMomentum(context, template, nil, true)
	if err != nil {
		return nil, err
	}
	return transaction, nil
}

// applyBlock is the common path for [ApplyBlock] /
// [GenerateFromTemplate]. Verifies block, runs it through the per-block
// VM, packs the result, and (if signFunc is non-nil) signs it.
//
// Wraps the work in a recover/panic-to-error guard so a VM panic
// becomes [constants.ErrVmRunPanic] for the caller.
func (s *Supervisor) applyBlock(block *nom.AccountBlock, signFunc SignFunc) (transaction *nom.AccountBlockTransaction, internalErr error) {
	defer func() {
		if err := recover(); err != nil {
			l := s.log.New("block", block.Header())
			l.Error("vm panic when applying block", "reason", err, "stack", string(debug.Stack()))

			transaction = nil
			internalErr = constants.ErrVmRunPanic
		}
	}()

	if err := s.verifier.AccountBlock(block); err != nil {
		return nil, err
	}
	context := s.newBlockContext(block)
	vm := NewVM(context)
	err := vm.applyBlock(block)
	if err != nil {
		return nil, err
	}

	transaction, err = s.packBlock(context, block, signFunc)
	if err != nil {
		return nil, err
	}

	return transaction, nil
}

// setAll fills in the template's MomentumAcknowledged, previous-block
// linkage, and identity fields in order. Calls into the per-field
// setters below.
func (s *Supervisor) setAll(template *nom.AccountBlock) error {
	if err := s.setBlockMomentum(template); err != nil {
		return err
	}
	if err := s.setBlockHH(template); err != nil {
		return err
	}
	s.setBlockFields(template)
	return nil
}

// packBlock finalizes a block: extracts the patch from context,
// computes the canonical hash, signs (if signFunc is non-nil), then
// runs the transactional verifier as a final sanity check.
//
// Note: when signFunc is non-nil, the block is signed twice — first
// against ComputeHash without ChangesHash, then with ChangesHash set.
// The second signature overwrites the first.
func (s *Supervisor) packBlock(context vm_context.AccountVmContext, block *nom.AccountBlock, signFunc SignFunc) (*nom.AccountBlockTransaction, error) {
	changes, err := context.Changes()
	if err != nil {
		return nil, err
	}

	if signFunc != nil {
		block.Hash = block.ComputeHash()
		signature, _, publicKey, err := signFunc(block.Hash.Bytes())
		if err != nil {
			return nil, err
		}
		block.Signature = signature
		block.PublicKey = publicKey
	}
	if signFunc != nil {
		block.ChangesHash = db.PatchHash(changes)
		block.Hash = block.ComputeHash()
		signature, _, publicKey, err := signFunc(block.Hash.Bytes())
		if err != nil {
			return nil, err
		}
		block.Signature = signature
		block.PublicKey = publicKey
	}

	transaction := &nom.AccountBlockTransaction{
		Block:   block,
		Changes: changes,
	}
	if err := s.verifier.AccountBlockTransaction(transaction); err != nil {
		return nil, err
	}

	return transaction, nil
}

// packMomentum finalizes a momentum: extracts the patch, sets
// ChangesHash + Hash, signs with signFunc when supplied, and runs the
// transactional verifier (skipped at genesis since there is no
// previous momentum to validate against).
func (s *Supervisor) packMomentum(context vm_context.MomentumVMContext, momentum *nom.Momentum, signFunc SignFunc, isGenesis bool) (*nom.MomentumTransaction, error) {
	changes, err := context.Changes()

	if err != nil {
		return nil, err
	}

	if signFunc != nil || isGenesis {
		momentum.ChangesHash = db.PatchHash(changes)
		momentum.Hash = momentum.ComputeHash()
	}
	if signFunc != nil {
		signature, _, publicKey, err := signFunc(momentum.Hash.Bytes())
		if err != nil {
			return nil, err
		}
		momentum.Signature = signature
		momentum.PublicKey = publicKey
	}

	transaction := &nom.MomentumTransaction{
		Momentum: momentum,
		Changes:  changes,
	}
	if !isGenesis {
		if err := s.verifier.MomentumTransaction(transaction); err != nil {
			return nil, err
		}
	}

	return transaction, nil
}

// setBlockPlasma fills the block's FusedPlasma from the base-plasma
// computation when the caller has not explicitly set Difficulty or
// FusedPlasma.
func (s *Supervisor) setBlockPlasma(context vm_context.AccountVmContext, block *nom.AccountBlock) error {
	if block.Difficulty == 0 && block.FusedPlasma == 0 {
		base, err := GetBasePlasmaForAccountBlock(context, block)
		if err != nil {
			return err
		}
		block.FusedPlasma = base
	}
	return nil
}

// setBlockFields fills the block's chain identifier, version, and
// type-dependent amount/token fields. Receives must carry zero
// amount and zero token; sends default to zero amount when not set.
func (s *Supervisor) setBlockFields(block *nom.AccountBlock) {
	block.ChainIdentifier = s.chain.ChainIdentifier()
	if block.Version == 0 {
		block.Version = 1
	}
	switch block.BlockType {
	case nom.BlockTypeUserSend, nom.BlockTypeContractSend:
		if block.Amount == nil {
			block.Amount = big.NewInt(0)
		}
	case nom.BlockTypeUserReceive, nom.BlockTypeContractReceive:
		block.Amount = common.Big0
		block.TokenStandard = types.ZeroTokenStandard
	}
}

// setBlockHH resolves the block's previous-hash + height from the
// account chain frontier when the caller left them as zero.
func (s *Supervisor) setBlockHH(block *nom.AccountBlock) error {
	if block.PreviousHash == types.ZeroHash && block.Height == 0 {
		store := s.chain.GetFrontierAccountStore(block.Address)
		frontier := store.Identifier()

		block.PreviousHash = frontier.Hash
		block.Height = frontier.Height + 1
	}
	return nil
}

// setBlockMomentum fills MomentumAcknowledged. For embedded
// contracts (auto-receives) it pins the value to the momentum that
// confirmed the originating send, so the verifier's
// auto-generated-block check matches. For user blocks it pins the
// frontier momentum.
func (s *Supervisor) setBlockMomentum(block *nom.AccountBlock) error {
	store := s.chain.GetFrontierMomentumStore()
	frontierMomentum, err := store.GetFrontierMomentum()
	if err != nil {
		return err
	}
	if block.MomentumAcknowledged.IsZero() {
		if types.IsEmbeddedAddress(block.Address) {
			confirmation, err := store.GetBlockConfirmationHeight(block.FromBlockHash)
			if err != nil {
				return err
			}
			if confirmation == 0 {
				return errors.Errorf("can't find block that confirms contract-receive")
			}
			momentum, err := store.GetMomentumByHeight(confirmation)
			if err != nil {
				return err
			}
			block.MomentumAcknowledged = momentum.Identifier()
		} else {
			block.MomentumAcknowledged = frontierMomentum.Identifier()
		}
	}
	return nil
}
