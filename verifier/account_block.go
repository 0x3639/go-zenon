package verifier

import (
	"fmt"

	"github.com/pkg/errors"

	"github.com/zenon-network/go-zenon/chain"
	"github.com/zenon-network/go-zenon/chain/nom"
	"github.com/zenon-network/go-zenon/chain/store"
	"github.com/zenon-network/go-zenon/common/types"
	"github.com/zenon-network/go-zenon/consensus"
	"github.com/zenon-network/go-zenon/pow"
	"github.com/zenon-network/go-zenon/wallet"
)

// ReceiverMismatchEnforcementHeight is the momentum height at which
// [ErrABFromBlockReceiverMismatch] becomes a hard verification failure.
// Before this height the receiver-mismatch case was tolerated for
// backwards compatibility with previously-relayed traffic; after it the
// verifier rejects any receive whose target address differs from the
// originating send's `ToAddress`.
//
// Targeting 2025-04-16 12:00:00 UTC.
var (
	ReceiverMismatchEnforcementHeight uint64 = 10109240 // Targeting 2025-04-16 12:00:00 UTC
)

// isBatched reports whether block is a contract-emitted send (one of the
// nested `BlockTypeContractSend` entries inside a parent receive's
// [nom.AccountBlock.DescendantBlocks]). Such blocks must not appear
// stand-alone; the parent receive carries them through verification.
func isBatched(block *nom.AccountBlock) bool {
	return block.IsSendBlock() && types.IsEmbeddedAddress(block.Address)
}

// isContractReceive reports whether block is a receive authored by an
// embedded contract (i.e., a [nom.BlockTypeContractReceive] block on a
// system address).
func isContractReceive(block *nom.AccountBlock) bool {
	return block.IsReceiveBlock() && types.IsEmbeddedAddress(block.Address)
}

// AccountBlockVerifier validates account blocks against consensus rules.
// Validation runs in two passes:
//
//   - [AccountBlockVerifier.AccountBlock] — block-as-payload checks
//     (version, type, amounts, plasma PoW, previous-block linkage,
//     momentum-acknowledged consistency, send/receive matching).
//   - [AccountBlockVerifier.AccountBlockTransaction] — transactional
//     checks tied to the patch (canonical hash, signature, producer
//     address, descendant-block validation).
type AccountBlockVerifier interface {
	AccountBlock(block *nom.AccountBlock) error
	AccountBlockTransaction(transaction *nom.AccountBlockTransaction) error
}

// accountVerifier is the [AccountBlockVerifier] implementation. Holds
// references to the chain (for store lookups) and consensus (for
// producer validation).
type accountVerifier struct {
	chain     chain.Chain
	consensus consensus.Consensus
}

// getContext resolves the account-store and momentum-store views the
// per-check helpers operate against. Returns one of [ErrABMHeightMissing],
// [ErrABPrevHashMustBeZero], [ErrABPrevHashMissing], [ErrABMAMustNotBeZero],
// [ErrABMAMissing], [ErrABPrevHasCementedOnTop], [ErrABPrevHeightExists],
// [ErrABPreviousMissing], or [ErrVerifierInternal] when the surrounding
// state required for verification is unavailable.
func (av *accountVerifier) getContext(block *nom.AccountBlock) (store.Account, store.Momentum, error) {
	if block.Height == 0 {
		return nil, nil, ErrABMHeightMissing
	}
	if block.Height == 1 && !block.PreviousHash.IsZero() {
		return nil, nil, ErrABPrevHashMustBeZero
	}
	if block.Height != 1 && block.PreviousHash.IsZero() {
		return nil, nil, ErrABPrevHashMissing
	}

	if block.MomentumAcknowledged.IsZero() {
		return nil, nil, ErrABMAMustNotBeZero
	}
	momentumStore := av.chain.GetMomentumStore(block.MomentumAcknowledged)
	if momentumStore == nil {
		return nil, nil, ErrABMAMissing
	}

	accountStore := av.chain.GetAccountStore(block.Address, block.Previous())

	if accountStore == nil {
		// Refine the error: the previous-identifier lookup miss could be
		// "ErrABPrevHasCementedOnTop" (the chain advanced past the
		// referenced previous), "ErrABPrevHeightExists" (a different
		// block sits at that height), or the catch-all
		// "ErrABPreviousMissing". Fall back to the global frontier
		// store to disambiguate.
		globalStore := av.chain.GetFrontierMomentumStore().GetAccountStore(block.Address)
		globalFrontier, err := globalStore.Frontier()
		if err != nil {
			return nil, nil, InternalError(err)
		}

		if globalFrontier.Height > block.Height-1 {
			block, err := globalStore.ByHash(block.PreviousHash)
			if err != nil {
				return nil, nil, InternalError(err)
			}
			if block != nil {
				return nil, nil, ErrABPrevHasCementedOnTop
			}
			return nil, nil, ErrABPrevHeightExists
		} else {
			return nil, nil, ErrABPreviousMissing
		}
	}

	return accountStore, momentumStore, nil
}

// AccountBlock validates a stand-alone account block. Rejects
// [nom.BlockTypeContractSend] (those are nested under a parent receive
// and must not be submitted directly).
func (av *accountVerifier) AccountBlock(block *nom.AccountBlock) error {
	if block.BlockType == nom.BlockTypeContractSend {
		return ErrABTypeInvalidExternal
	}

	accountStore, momentumStore, err := av.getContext(block)
	if err != nil {
		return err
	}

	return (&accountBlockVerifier{
		block:         block,
		accountStore:  accountStore,
		momentumStore: momentumStore,
		frontierStore: av.chain.GetFrontierMomentumStore(),
	}).all()
}

// AccountBlockTransaction validates the transactional layer of an
// account block: hash, signature, producer-address consistency, and
// every nested descendant block.
func (av *accountVerifier) AccountBlockTransaction(transaction *nom.AccountBlockTransaction) error {
	if transaction.Block.BlockType == nom.BlockTypeContractSend {
		return ErrABTypeInvalidExternal
	}

	accountStore, momentumStore, err := av.getContext(transaction.Block)
	if err != nil {
		return err
	}

	return (&accountBlockTransactionVerifier{
		transaction:   transaction,
		accountStore:  accountStore,
		momentumStore: momentumStore,
	}).all()
}

// NewAccountBlockVerifier constructs an [AccountBlockVerifier] backed by
// chain (for store lookups) and consensus (for producer validation).
func NewAccountBlockVerifier(chain chain.Chain, consensus consensus.Consensus) AccountBlockVerifier {
	return &accountVerifier{
		chain:     chain,
		consensus: consensus,
	}
}

// accountBlockVerifier holds the in-flight state for a single
// block-as-payload validation pass. The stores are resolved once by
// [accountVerifier.getContext]; per-check methods then run in fixed
// order via [accountBlockVerifier.all].
type accountBlockVerifier struct {
	block         *nom.AccountBlock
	accountStore  store.Account
	momentumStore store.Momentum
	frontierStore store.Momentum
}

// all runs every block-as-payload check in dependency order and returns
// the first error encountered.
func (abv *accountBlockVerifier) all() error {
	if err := abv.version(); err != nil {
		return err
	}
	if err := abv.chainIdentifier(); err != nil {
		return err
	}
	if err := abv.blockType(); err != nil {
		return err
	}
	if err := abv.amounts(); err != nil {
		return err
	}
	if err := abv.pow(); err != nil {
		return err
	}
	if err := abv.previous(); err != nil {
		return err
	}
	if err := abv.momentumAcknowledged(); err != nil {
		return err
	}
	if err := abv.fromHash(); err != nil {
		return err
	}
	if err := abv.sequencer(); err != nil {
		return err
	}
	return nil
}

// version checks the block's protocol version field. Currently only
// version 1 is accepted.
func (abv *accountBlockVerifier) version() error {
	if abv.block.Version == 0 {
		return ErrABVersionMissing
	}
	if abv.block.Version != 1 {
		return ErrABVersionInvalid
	}
	return nil
}

// chainIdentifier checks the block names this network's chain identifier
// (replay protection across networks).
func (abv *accountBlockVerifier) chainIdentifier() error {
	if abv.block.ChainIdentifier == 0 {
		return ErrMChainIdentifierMissing
	}
	if abv.block.ChainIdentifier != abv.momentumStore.ChainIdentifier() {
		return fmt.Errorf("%w - expected %v but received %v", ErrMChainIdentifierMismatch, abv.momentumStore.ChainIdentifier(), abv.block.ChainIdentifier)
	}
	return nil
}

// blockType checks the BlockType is well-formed and matches the address
// kind: embedded-contract addresses must use the contract send/receive
// variants, and user addresses the user variants.
func (abv *accountBlockVerifier) blockType() error {
	if abv.block.BlockType == 0 {
		return ErrABTypeMissing
	}
	if abv.block.BlockType == nom.BlockTypeGenesisReceive {
		return ErrABTypeMustNotBeGenesis
	}
	if abv.block.IsSendBlock() || abv.block.IsReceiveBlock() {
	} else {
		return ErrABTypeUnsupported
	}

	if types.IsEmbeddedAddress(abv.block.Address) {
		if abv.block.BlockType == nom.BlockTypeContractReceive || abv.block.BlockType == nom.BlockTypeContractSend {
		} else {
			return ErrABTypeMustBeContract
		}
	} else {
		if abv.block.BlockType == nom.BlockTypeUserReceive || abv.block.BlockType == nom.BlockTypeUserSend {
		} else {
			return ErrABTypeMustBeUser
		}
	}
	return nil
}

// amounts checks the (Amount, TokenStandard, ToAddress, FromBlockHash)
// quartet is internally consistent for the block's send/receive role.
func (abv *accountBlockVerifier) amounts() error {
	if abv.block.IsSendBlock() {
		if abv.block.Amount.Sign() == -1 {
			return ErrABAmountNegative
		}
		if abv.block.Amount.BitLen() > 255 {
			return ErrABAmountTooBig
		}
		if abv.block.Amount.Sign() == +1 && abv.block.TokenStandard == types.ZeroTokenStandard {
			return ErrABZtsMissing
		}
		// ToAddress can be null

		if !abv.block.FromBlockHash.IsZero() {
			return ErrABFromBlockHashMustBeZero
		}
	} else {
		if abv.block.Amount != nil && abv.block.Amount.Sign() != 0 {
			return ErrABAmountMustBeZero
		}
		if abv.block.TokenStandard != types.ZeroTokenStandard {
			return ErrABZtsMustBeZero
		}
		if abv.block.ToAddress != types.ZeroAddress {
			return ErrABToAddressMustBeZero
		}

		if abv.block.FromBlockHash.IsZero() {
			return ErrABFromBlockHashMissing
		}
	}
	return nil
}

// pow validates the plasma PoW nonce when one is claimed. Embedded
// contracts cannot pay plasma via PoW; if Difficulty is non-zero on an
// embedded address the block is rejected.
func (abv *accountBlockVerifier) pow() error {
	if abv.block.Difficulty != 0 {
		if types.IsEmbeddedAddress(abv.block.Address) {
			return ErrABPoWInvalid
		}
		if !pow.CheckPoWNonce(abv.block) {
			return ErrABPoWInvalid
		}
	}
	return nil
}

// previous checks the block's height/previous-hash linkage against the
// account's frontier. Skipped for embedded-contract receives: the chain
// layer assembles those out of order during a momentum's batch
// processing.
func (abv *accountBlockVerifier) previous() error {
	// for consistency, check again
	if abv.block.Height == 0 {
		return ErrABMHeightMissing
	}
	if abv.block.Height == 1 && !abv.block.PreviousHash.IsZero() {
		return ErrABPrevHashMustBeZero
	}
	if abv.block.Height != 1 && abv.block.PreviousHash.IsZero() {
		return ErrABPrevHashMissing
	}

	// start blocks don't expect previous
	if abv.block.Height == 1 {
		return nil
	}

	// don't check previous on contract
	if types.IsEmbeddedAddress(abv.block.Address) {
		return nil
	}

	block, err := abv.accountStore.Frontier()
	if err != nil {
		return InternalError(err)
	}
	if block == nil {
		return InternalError(errors.Errorf("empty frontier account-block"))
	}
	if block.Identifier() != abv.block.Previous() {
		return ErrABPreviousMissing
	}
	return nil
}

// momentumAcknowledged validates the block's MomentumAcknowledged field:
// it must point at an existing momentum, must equal the
// MomentumAcknowledged of every descendant for contract receives, must
// equal the confirmation height of the originating send for
// auto-generated blocks, and must not regress relative to the previous
// block in the same account chain.
func (abv *accountBlockVerifier) momentumAcknowledged() error {
	momentum, err := abv.momentumStore.GetFrontierMomentum()
	if err != nil {
		return InternalError(err)
	}
	if momentum.Identifier() != abv.block.MomentumAcknowledged {
		return InternalError(errors.Errorf("impossible scenario. verifier momentum-store exists but frontier is different. Expected MomentumAcknowledged %v but got %v from MomentumStore", abv.block.MomentumAcknowledged, momentum.Identifier()))
	}

	// all checks are done by the parent
	if isBatched(abv.block) {
		return nil
	}

	// MomentumAcknowledged is the same as all the ones in dBlocks
	if isContractReceive(abv.block) {
		for _, dBlock := range abv.block.DescendantBlocks {
			if dBlock.MomentumAcknowledged != abv.block.MomentumAcknowledged {
				return ErrABMAMustBeTheSame
			}
		}

		height, err := abv.momentumStore.GetBlockConfirmationHeight(abv.block.FromBlockHash)
		if err != nil {
			return InternalError(err)
		}
		if height != abv.block.MomentumAcknowledged.Height {
			return ErrABMAInvalidForAutoGenerated
		}
		return nil
	}

	// current MomentumAcknowledged is bigger than previous
	if previous := abv.block.Previous(); previous != types.ZeroHashHeight {
		previousBlock, err := abv.accountStore.ByHeight(previous.Height)
		if err != nil {
			return InternalError(err)
		}
		if previousBlock.MomentumAcknowledged.Height > abv.block.MomentumAcknowledged.Height {
			return ErrABMAGap
		}
	}

	return nil
}

// fromHash validates send/receive matching: the receive's
// [nom.AccountBlock.FromBlockHash] must point to a real send, that send
// must not have been received already, and (above
// [ReceiverMismatchEnforcementHeight]) the receiver must equal the
// send's ToAddress.
func (abv *accountBlockVerifier) fromHash() error {
	if abv.block.IsSendBlock() {
		return nil
	}

	// check that from-hash is a valid hash
	sendBlock, err := abv.momentumStore.GetAccountBlockByHash(abv.block.FromBlockHash)
	if err != nil {
		return InternalError(err)
	} else if sendBlock == nil {
		return ErrABFromBlockMissing
	}

	if abv.block.Address != sendBlock.ToAddress {
		// Use the momentum ledger's true frontier height when comparing
		if abv.frontierStore.Identifier().Height >= ReceiverMismatchEnforcementHeight {
			return ErrABFromBlockReceiverMismatch
		}
	}

	// check if abv.block was already received
	status := abv.accountStore.IsReceived(abv.block.FromBlockHash)
	if status {
		return ErrABFromBlockAlreadyReceived
	}

	return nil
}

// sequencer ensures embedded-contract receives consume from the head of
// their address mailbox in order — contracts cannot cherry-pick which
// inbound send to receive. Only applies to embedded-address receives;
// user accounts receive in any order.
func (abv *accountBlockVerifier) sequencer() error {
	if types.IsEmbeddedAddress(abv.block.Address) && abv.block.IsReceiveBlock() {
	} else {
		return nil
	}

	nextInLine := abv.accountStore.SequencerFront(abv.momentumStore.GetAccountMailbox(abv.block.Address))
	if nextInLine == nil {
		return ErrABSequencerNothing
	}

	sendBlock, err := abv.momentumStore.GetAccountBlockByHash(abv.block.FromBlockHash)
	if err != nil {
		return InternalError(err)
	}
	if sendBlock.Header() != *nextInLine {
		return ErrABSequencerNotNext
	}

	return nil
}

// accountBlockTransactionVerifier holds the in-flight state for the
// transactional pass: validates the (Hash, Signature, Producer)
// authentication triplet plus every descendant block.
type accountBlockTransactionVerifier struct {
	transaction   *nom.AccountBlockTransaction
	accountStore  store.Account
	momentumStore store.Momentum
}

// all runs every transactional check in dependency order and returns the
// first error encountered.
func (abvt *accountBlockTransactionVerifier) all() error {
	if err := abvt.hash(); err != nil {
		return err
	}
	if err := abvt.signature(); err != nil {
		return err
	}
	if err := abvt.producer(); err != nil {
		return err
	}
	if err := abvt.descendantBlocks(); err != nil {
		return err
	}

	return nil
}

// signature validates the Ed25519 signature. Embedded-contract blocks
// must carry no PublicKey or Signature (their authority is the contract
// dispatch path, not a key); user blocks must carry both and the signature
// must verify against the block's hash.
func (abvt *accountBlockTransactionVerifier) signature() error {
	block := abvt.transaction.Block
	if types.IsEmbeddedAddress(block.Address) {
		if len(block.PublicKey) != 0 {
			return ErrABPublicKeyMustBeZero
		}
		if len(block.Signature) != 0 {
			return ErrABSignatureMustBeZero
		}
		return nil
	}

	if len(block.Signature) == 0 {
		return ErrABSignatureMissing
	}
	if len(block.PublicKey) == 0 {
		return ErrABPublicKeyMissing
	}
	isVerified, err := wallet.VerifySignature(block.PublicKey, block.Hash.Bytes(), block.Signature)
	if err != nil {
		return ErrABSignatureInvalid
	}
	if !isVerified {
		return ErrABSignatureInvalid
	}
	return nil
}

// hash checks that the block carries a non-zero hash and that the stored
// hash matches the canonical [nom.AccountBlock.ComputeHash] result.
func (abvt *accountBlockTransactionVerifier) hash() error {
	block := abvt.transaction.Block

	// check expected hash matches
	computedHash := block.ComputeHash()
	if block.Hash.IsZero() {
		return ErrABHashMissing
	}
	if computedHash != block.Hash {
		return ErrABHashInvalid
	}
	return nil
}

// producer checks the public key derives to the block's claimed address
// (skipped for embedded contracts, which have no key).
func (abvt *accountBlockTransactionVerifier) producer() error {
	block := abvt.transaction.Block

	if types.IsEmbeddedAddress(block.Address) {
		return nil
	}
	if types.PubKeyToAddress(block.PublicKey) != block.Address {
		return ErrABPublicKeyWrongAddress
	}

	return nil
}

// descendantBlocks recursively validates every nested
// [nom.BlockTypeContractSend]. Non-contract-receive blocks must have an
// empty [nom.AccountBlock.DescendantBlocks]; descendants are wrapped in
// [DescendantVerifyError] so callers can tell which layer failed.
func (abvt *accountBlockTransactionVerifier) descendantBlocks() error {
	block := abvt.transaction.Block
	if !isContractReceive(block) && len(block.DescendantBlocks) > 0 {
		return ErrABDescendantMustBeZero
	}
	for _, dBlock := range block.DescendantBlocks {
		if err := (&accountBlockVerifier{
			block:         dBlock,
			accountStore:  abvt.accountStore,
			momentumStore: abvt.momentumStore,
		}).all(); err != nil {
			return DescendantVerifyError(err)
		}
	}
	return nil
}
