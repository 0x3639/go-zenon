package verifier

import (
	"fmt"
	"time"

	"github.com/inconshreveable/log15"
	"github.com/pkg/errors"

	"github.com/zenon-network/go-zenon/chain"
	"github.com/zenon-network/go-zenon/chain/nom"
	"github.com/zenon-network/go-zenon/chain/store"
	"github.com/zenon-network/go-zenon/common"
	"github.com/zenon-network/go-zenon/common/db"
	"github.com/zenon-network/go-zenon/common/types"
	"github.com/zenon-network/go-zenon/consensus"
	"github.com/zenon-network/go-zenon/wallet"
)

// MomentumVerifier validates momentums against consensus rules. Like
// [AccountBlockVerifier], validation runs in two passes:
//
//   - [MomentumVerifier.Momentum] — momentum-as-payload checks (chain
//     identifier, version, timestamp monotonicity, previous-momentum
//     linkage, content size, and account-header continuity against the
//     prefetched account blocks).
//   - [MomentumVerifier.MomentumTransaction] — transactional checks
//     ([nom.Momentum.ChangesHash], canonical hash, signature, and
//     producer eligibility for the momentum's tick).
type MomentumVerifier interface {
	Momentum(momentum *nom.DetailedMomentum) error
	MomentumTransaction(transaction *nom.MomentumTransaction) error
}

// momentumVerifier is the [MomentumVerifier] implementation.
type momentumVerifier struct {
	log       log15.Logger
	chain     chain.Chain
	consensus consensus.Consensus
}

// getContext resolves the previous-momentum store the per-check helpers
// run against. Returns one of [ErrMNotGenesis], [ErrMPrevHashMissing], or
// [ErrMPreviousMissing] when the surrounding state is missing or
// inconsistent.
func (mv *momentumVerifier) getContext(momentum *nom.Momentum) (store.Momentum, error) {
	if momentum.Height == 1 {
		return nil, ErrMNotGenesis
	}
	if momentum.PreviousHash.IsZero() {
		return nil, ErrMPrevHashMissing
	}

	momentumStore := mv.chain.GetMomentumStore(momentum.Previous())
	if momentumStore == nil {
		return nil, ErrMPreviousMissing
	}
	return momentumStore, nil
}

// Momentum validates a momentum together with the [nom.AccountBlock]s
// prefetched for it (the [nom.DetailedMomentum] form). The prefetched
// list lets the verifier check that the account-header continuity claimed
// by [nom.MomentumContent] is reproducible from the block bodies.
func (mv *momentumVerifier) Momentum(detailed *nom.DetailedMomentum) error {
	momentumStore, err := mv.getContext(detailed.Momentum)
	if err != nil {
		return err
	}

	return (&rawMomentumVerifier{
		momentum:      detailed.Momentum,
		accountBlocks: detailed.AccountBlocks,
		momentumStore: momentumStore,
	}).all()
}

// MomentumTransaction validates the transactional layer: changes-hash,
// canonical hash, signature, and producer eligibility.
func (mv *momentumVerifier) MomentumTransaction(transaction *nom.MomentumTransaction) error {
	return (&momentumTransactionVerifier{
		transaction: transaction,
		consensus:   mv.consensus,
	}).all()
}

// NewMomentumVerifier constructs a [MomentumVerifier] backed by chain
// (for store lookups) and consensus (for producer validation).
func NewMomentumVerifier(chain chain.Chain, consensus consensus.Consensus) MomentumVerifier {
	return &momentumVerifier{
		log:       common.VerifierLogger.New("type", "momentum"),
		chain:     chain,
		consensus: consensus,
	}
}

// rawMomentumVerifier holds the in-flight state for the
// momentum-as-payload pass.
type rawMomentumVerifier struct {
	momentum      *nom.Momentum
	accountBlocks []*nom.AccountBlock
	momentumStore store.Momentum
}

// all runs every momentum-as-payload check in dependency order and
// returns the first error encountered.
func (rmv *rawMomentumVerifier) all() error {
	if err := rmv.chainIdentifier(); err != nil {
		return err
	}
	if err := rmv.version(); err != nil {
		return err
	}
	if err := rmv.timestamp(); err != nil {
		return err
	}
	if err := rmv.previous(); err != nil {
		return err
	}
	if err := rmv.data(); err != nil {
		return err
	}
	if err := rmv.content(); err != nil {
		return err
	}
	return nil
}

// chainIdentifier checks the momentum names this network's chain
// identifier (replay protection across networks).
func (rmv *rawMomentumVerifier) chainIdentifier() error {
	if rmv.momentum.ChainIdentifier == 0 {
		return ErrABChainIdentifierMissing
	}
	if rmv.momentum.ChainIdentifier != rmv.momentumStore.ChainIdentifier() {
		return fmt.Errorf("%w - expected %v but received %v", ErrABChainIdentifierMismatch, rmv.momentumStore.ChainIdentifier(), rmv.momentum.ChainIdentifier)
	}
	return nil
}

// version checks the momentum's protocol version field. Currently only
// version 1 is accepted.
func (rmv *rawMomentumVerifier) version() error {
	if rmv.momentum.Version == 0 {
		return ErrMVersionMissing
	}
	if rmv.momentum.Version != 1 {
		return ErrMVersionInvalid
	}
	return nil
}

// timestamp checks the momentum's timestamp is present, not more than
// 10 seconds in the future relative to wall time, and strictly increasing
// against the previous-momentum frontier.
func (rmv *rawMomentumVerifier) timestamp() error {
	if rmv.momentum.Timestamp.Unix() == 0 {
		return ErrMTimestampMissing
	}
	if rmv.momentum.Timestamp.After(time.Now().Add(time.Second * 10)) {
		return ErrMTimestampInTheFuture
	}

	previous, err := rmv.momentumStore.GetFrontierMomentum()
	if err != nil {
		return InternalError(err)
	}
	if previous.TimestampUnix >= rmv.momentum.TimestampUnix {
		return ErrMTimestampNotIncreasing
	}
	return nil
}

// previous checks the previous-hash linkage against the previous-momentum
// store's frontier. The genesis momentum (Height == 1) is rejected here;
// genesis is loaded via [github.com/zenon-network/go-zenon/chain/genesis].
func (rmv *rawMomentumVerifier) previous() error {
	// for consistency, check again
	if rmv.momentum.Height == 1 {
		return ErrMNotGenesis
	}
	if rmv.momentum.PreviousHash.IsZero() {
		return ErrMPrevHashMissing
	}

	previous, err := rmv.momentumStore.GetFrontierMomentum()
	if err != nil {
		return InternalError(err)
	}
	if rmv.momentum.Previous() != previous.Identifier() {
		return ErrMPreviousMissing
	}
	return nil
}

// data checks the per-momentum [nom.Momentum.Data] field is empty. The
// field is reserved for future use; non-empty data is currently a hard
// rejection.
func (rmv *rawMomentumVerifier) data() error {
	if len(rmv.momentum.Data) != 0 {
		return ErrMDataMustBeZero
	}
	return nil
}

// content validates the account-header list:
//
//   - Total size is at most [chain.MaxAccountBlocksInMomentum].
//   - Each header in the content is present in the prefetched
//     account-block list (sizes match exactly, except for batched
//     contract sends which are skipped).
//   - For every (address, header) pair, the header's previous matches
//     either the previous header for that address in this momentum or
//     the address's frontier from before this momentum — i.e., the
//     content is a contiguous extension of every touched account chain.
//
// The verifier does not validate the account blocks themselves here —
// that is [AccountBlockVerifier]'s job. content only checks that the
// momentum-content shape is sane.
func (rmv *rawMomentumVerifier) content() error {
	if len(rmv.momentum.Content) > chain.MaxAccountBlocksInMomentum {
		return ErrMContentTooBig
	}
	blocksLookup := make(map[types.HashHeight]*nom.AccountBlock)

	// insert all account-blocks in lookup map
	for _, block := range rmv.accountBlocks {
		blocksLookup[block.Identifier()] = block
	}

	// sizes are the same
	if len(blocksLookup) != len(rmv.momentum.Content) {
		return errors.Errorf("momentum content size is different than the size of the prefetched account-blocks")
	}

	// account identifiers make sense when 'applying' blocks; i.e: all pairs of (previous, identifier) match
	// Note: use prefetched blocks to get block.previous
	// Note: at this point, we don't care if account-blocks are valid or not, just that the momentum contains all the
	// blocks and the headers are put in a valid order, since the pillar selects which blocks and in which order
	// are inserted in the momentum
	heads := make(map[types.Address]types.HashHeight)
	for _, header := range rmv.momentum.Content {
		previous, ok := heads[header.Address]
		if !ok {
			pastFrontier, err := rmv.momentumStore.GetFrontierAccountBlock(header.Address)
			if err != nil {
				return InternalError(err)
			}
			if pastFrontier == nil {
				previous = types.ZeroHashHeight
			} else {
				previous = pastFrontier.Identifier()
			}
		}

		block, ok := blocksLookup[header.Identifier()]
		if isBatched(block) {
			continue
		}
		if !ok {
			return errors.Errorf("momentum content header is not present in prefetched account-blocks")
		}

		if block.Previous() != previous {
			return errors.Errorf("gap in previous Expected %v but got %v", previous, block.Previous())
		}

		heads[header.Address] = block.Identifier()
	}

	return nil
}

// momentumTransactionVerifier holds the in-flight state for the
// transactional pass on a momentum.
type momentumTransactionVerifier struct {
	transaction *nom.MomentumTransaction
	consensus   consensus.Consensus
}

// all runs every transactional check in dependency order and returns the
// first error encountered.
func (mv *momentumTransactionVerifier) all() error {
	if err := mv.changesHash(mv.transaction); err != nil {
		return err
	}
	if err := mv.hash(mv.transaction); err != nil {
		return err
	}
	if err := mv.signature(mv.transaction); err != nil {
		return err
	}
	if err := mv.producer(mv.transaction); err != nil {
		return err
	}
	return nil
}

// signature validates the Ed25519 signature on the momentum hash.
// Returns [ErrMSignatureInvalid] on a mismatch and [ErrVerifierInternal]
// on key-format errors.
func (mv *momentumTransactionVerifier) signature(transaction *nom.MomentumTransaction) error {
	momentum := transaction.Momentum

	if len(momentum.Signature) == 0 {
		return ErrMSignatureMissing
	}
	if len(momentum.PublicKey) == 0 {
		return ErrMPublicKeyMissing
	}
	isVerified, err := wallet.VerifySignature(momentum.PublicKey, momentum.Hash.Bytes(), momentum.Signature)
	if err != nil {
		return InternalError(err)
	}
	if !isVerified {
		return ErrMSignatureInvalid
	}
	return nil
}

// changesHash checks the momentum's [nom.Momentum.ChangesHash] equals the
// hash of the transaction's patch. Binds state to consensus: the
// momentum can only be accepted if every node computes the same patch
// for the same set of account blocks.
func (mv *momentumTransactionVerifier) changesHash(transaction *nom.MomentumTransaction) error {
	computedHash := db.PatchHash(transaction.Changes)
	if computedHash != transaction.Momentum.ChangesHash {
		log.Info("changes-hash differ", "expected", computedHash, "got-instead", transaction.Momentum.ChangesHash)
		return ErrMChangesHashInvalid
	}
	return nil
}

// hash checks the stored hash matches the canonical
// [nom.Momentum.ComputeHash] result.
func (mv *momentumTransactionVerifier) hash(transaction *nom.MomentumTransaction) error {
	momentum := transaction.Momentum
	computedHash := momentum.ComputeHash()
	if computedHash != momentum.Hash {
		return ErrMHashInvalid
	}
	return nil
}

// producer checks the consensus layer accepts the signing pillar as the
// elected producer for the momentum's tick. The actual eligibility logic
// (election + points) lives in
// [github.com/zenon-network/go-zenon/consensus].
func (mv *momentumTransactionVerifier) producer(transaction *nom.MomentumTransaction) error {
	// MomentumTransaction producer
	result, err := mv.consensus.VerifyMomentumProducer(transaction.Momentum)
	if err != nil {
		return InternalError(err)
	} else if !result {
		return ErrMProducerInvalid
	}
	return nil
}
