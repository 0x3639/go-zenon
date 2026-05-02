package nom

import (
	"crypto/ed25519"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math/big"

	"github.com/pkg/errors"
	"google.golang.org/protobuf/proto"

	"github.com/zenon-network/go-zenon/common"
	"github.com/zenon-network/go-zenon/common/db"
	"github.com/zenon-network/go-zenon/common/types"
)

// Account-block kinds. Send and receive blocks always come in pairs:
// `BlockTypeUserSend` is matched by `BlockTypeUserReceive` (or
// `BlockTypeContractReceive` when the recipient is an embedded contract);
// `BlockTypeContractSend` is matched by `BlockTypeUserReceive` /
// `BlockTypeContractReceive` depending on the recipient.
//
// `BlockTypeGenesisReceive` is the special-case block used by genesis to
// seed initial state — it is a receive block with no matching send.
const (
	// BlockTypeGenesisReceive marks the genesis-time receive block; used
	// only by [github.com/zenon-network/go-zenon/chain/genesis] to seed
	// initial token supplies and embedded-contract state.
	BlockTypeGenesisReceive = 1 // receive

	// BlockTypeUserSend marks a send authored by a user account.
	BlockTypeUserSend = 2 // send
	// BlockTypeUserReceive marks a receive authored by a user account.
	BlockTypeUserReceive = 3 // receive

	// BlockTypeContractSend marks a send emitted by an embedded contract
	// during the receive of a triggering send (a [DescendantBlocks] entry).
	BlockTypeContractSend = 4 // send
	// BlockTypeContractReceive marks the receive an embedded contract
	// authors when consuming an inbound send.
	BlockTypeContractReceive = 5 // receive
)

// AccountBlockTransaction is the [db.Transaction] wrapper around an
// account block and the [db.Patch] capturing the state changes it caused.
// Used by the chain layer to commit an account block (and any contract
// descendants) atomically.
type AccountBlockTransaction struct {
	Block   *AccountBlock
	Changes db.Patch
}

// GetCommits returns the descendant blocks first followed by the outer
// block, in the order required by the database manager (descendants must
// commit before the parent so their state is visible when the parent
// finalizes).
func (t *AccountBlockTransaction) GetCommits() []db.Commit {
	list := make([]db.Commit, len(t.Block.DescendantBlocks)+1)
	for index := range t.Block.DescendantBlocks {
		list[index] = t.Block.DescendantBlocks[index]
	}
	list[len(list)-1] = t.Block
	return list
}

// StealChanges hands ownership of the patch to the caller and clears the
// field on the transaction. Used by the database manager to avoid copying
// the patch.
func (t *AccountBlockTransaction) StealChanges() db.Patch {
	changes := t.Changes
	t.Changes = nil
	return changes
}

// Nonce is the 8-byte plasma proof-of-work nonce. Together with
// [AccountBlock.Difficulty] it lets a sender pay plasma cost via PoW
// instead of fused QSR.
type Nonce struct {
	Data [8]byte
}

// Copy returns a deep copy of n.
func (n *Nonce) Copy() Nonce {
	dn := Nonce{}
	copy(dn.Data[:], n.Data[:])
	return dn
}

// MarshalText emits the nonce as lowercase hex. Implements
// [encoding.TextMarshaler] so JSON encoders render the nonce as a string
// (the binary form would not round-trip cleanly through JSON).
func (n *Nonce) MarshalText() ([]byte, error) {
	return []byte(hex.EncodeToString(n.Data[:])), nil
}

// UnmarshalText parses a hex-encoded 8-byte nonce. Returns an error on
// malformed hex or wrong length.
func (n *Nonce) UnmarshalText(input []byte) error {
	bytes, err := hex.DecodeString(string(input))
	if err != nil {
		return fmt.Errorf("failed to decode nonce:%v", err)
	}
	if len(bytes) != 8 {
		return errors.Errorf("invalid nonce length")
	}
	copy(n.Data[:], bytes)
	return nil
}

// Serialize returns a fresh slice over the nonce bytes.
func (n *Nonce) Serialize() []byte {
	return n.Data[:]
}

// DeSerializeNonce decodes 8 raw bytes into a [Nonce]. Panics on length
// mismatch — the protobuf shape is fixed at 8 bytes, so a mismatch
// indicates a corrupt message.
func DeSerializeNonce(bytes []byte) Nonce {
	if len(bytes) != 8 {
		panic("invalid nonce length")
	}
	var n Nonce
	copy(n.Data[:], bytes)
	return n
}

// AccountBlock is one transaction on a single account's chain. Send blocks
// transfer tokens (or invoke a contract); receive blocks consume a matching
// send. Every transfer on the network is therefore a (send, receive) pair.
//
// Field grouping:
//
//   - Identity: Version, ChainIdentifier, BlockType, Hash, PreviousHash,
//     Height. PreviousHash + Height position the block in its account
//     chain; ChainIdentifier prevents replay across networks.
//   - Send payload: ToAddress, Amount, TokenStandard, and the optional
//     Data field (ABI-encoded contract call).
//   - Receive payload: FromBlockHash points back to the send this block
//     consumes.
//   - Contract output: DescendantBlocks holds [BlockTypeContractSend]
//     blocks emitted while receiving a triggering send. Their hashes
//     contribute to the parent block's hash.
//   - Plasma: FusedPlasma + (Difficulty, Nonce) cover the per-block
//     resource cost. BasePlasma is the minimum required; TotalPlasma is
//     the actual budget supplied. BasePlasma and TotalPlasma are not part
//     of the canonical hash.
//   - Acknowledgement: MomentumAcknowledged pins the most recent momentum
//     the author has observed; the verifier uses it to bound chain reorgs.
//   - State commitment: ChangesHash is the hash of the patch the receive
//     produced; not part of the canonical hash but bound through the
//     [Momentum.ChangesHash] aggregate.
//   - Authentication: PublicKey + Signature; producer is a cached derived
//     value (see [AccountBlock.Producer]).
type AccountBlock struct {
	Version         uint64 `json:"version"`
	ChainIdentifier uint64 `json:"chainIdentifier"`
	BlockType       uint64 `json:"blockType"`

	Hash                 types.Hash       `json:"hash"`
	PreviousHash         types.Hash       `json:"previousHash"`
	Height               uint64           `json:"height"`
	MomentumAcknowledged types.HashHeight `json:"momentumAcknowledged"`

	Address types.Address `json:"address"`

	// Send information
	ToAddress     types.Address            `json:"toAddress"`
	Amount        *big.Int                 `json:"amount"`
	TokenStandard types.ZenonTokenStandard `json:"tokenStandard"`

	// Receive information
	FromBlockHash types.Hash `json:"fromBlockHash"`

	// Batch information
	DescendantBlocks []*AccountBlock `json:"descendantBlocks"` // hash of DescendantBlocks is included in hash

	Data []byte `json:"data"` // hash of Data is included in hash

	FusedPlasma uint64 `json:"fusedPlasma"`
	Difficulty  uint64 `json:"difficulty"`
	Nonce       Nonce  `json:"nonce"`
	BasePlasma  uint64 `json:"basePlasma"` // not included in hash, the smallest value of TotalPlasma required for block
	TotalPlasma uint64 `json:"usedPlasma"` // not included in hash, TotalPlasma = FusedPlasma + PowPlasma

	ChangesHash types.Hash `json:"changesHash"` // not included in hash

	producer  *types.Address    // not included in hash, for caching purpose only
	PublicKey ed25519.PublicKey `json:"publicKey"` // not included in hash
	Signature []byte            `json:"signature"` // not included in hash
}

// Identifier returns the (Hash, Height) pair locating ab in its account
// chain. Implements [db.Commit].
func (ab *AccountBlock) Identifier() types.HashHeight {
	return types.HashHeight{
		Hash:   ab.Hash,
		Height: ab.Height,
	}
}

// Previous returns the previous-block locator. For descendant-bearing
// blocks the locator points to the previous of the first descendant — the
// effective head before this transaction was applied. Implements
// [db.Commit].
func (ab *AccountBlock) Previous() types.HashHeight {
	if len(ab.DescendantBlocks) != 0 {
		return ab.DescendantBlocks[0].Previous()
	}
	return types.HashHeight{
		Hash:   ab.PreviousHash,
		Height: ab.Height - 1,
	}
}

// Header returns the (Address, Hash, Height) triple used by momentums to
// commit to the set of account blocks they finalize.
func (ab *AccountBlock) Header() types.AccountHeader {
	return types.AccountHeader{
		Address: ab.Address,
		HashHeight: types.HashHeight{
			Hash:   ab.Hash,
			Height: ab.Height,
		},
	}
}

// Copy returns a deep copy of ab — every slice, map, and big.Int is
// re-allocated, including the recursive DescendantBlocks tree.
func (ab *AccountBlock) Copy() *AccountBlock {
	cBlock := *ab

	if ab.Amount != nil {
		cBlock.Amount = new(big.Int).Set(ab.Amount)
	}

	cBlock.Data = make([]byte, len(ab.Data))
	copy(cBlock.Data, ab.Data)

	cBlock.Nonce = ab.Nonce.Copy()

	if len(ab.Signature) > 0 {
		cBlock.Signature = make([]byte, len(ab.Signature))
		copy(cBlock.Signature, ab.Signature)
	}

	cBlock.DescendantBlocks = make([]*AccountBlock, 0, len(ab.DescendantBlocks))
	for _, dBlock := range ab.DescendantBlocks {
		cBlock.DescendantBlocks = append(cBlock.DescendantBlocks, dBlock.Copy())
	}
	return &cBlock
}

// DescendantBlocksHash hashes the concatenation of every descendant
// block's hash. This rolls the descendants into the parent's canonical
// hash so that altering any descendant invalidates the parent.
func (ab *AccountBlock) DescendantBlocksHash() types.Hash {
	source := make([]byte, 0, types.HashSize*len(ab.DescendantBlocks))
	for _, dBlock := range ab.DescendantBlocks {
		source = append(source, dBlock.Hash.Bytes()...)
	}
	return types.NewHash(source)
}

// ComputeHash returns the canonical hash of ab. The hash binds every field
// the protocol considers part of the block's identity — explicitly
// excluding [BasePlasma], [TotalPlasma], [ChangesHash], [PublicKey], and
// [Signature], which are added as part of execution and authentication.
//
// Invariant: an account block's stored Hash always equals ComputeHash().
func (ab *AccountBlock) ComputeHash() types.Hash {
	return types.NewHash(common.JoinBytes(
		common.Uint64ToBytes(ab.Version),
		common.Uint64ToBytes(ab.ChainIdentifier),
		common.Uint64ToBytes(ab.BlockType),
		ab.PreviousHash.Bytes(),
		common.Uint64ToBytes(ab.Height),
		ab.MomentumAcknowledged.Bytes(),
		ab.Address.Bytes(),
		ab.ToAddress.Bytes(),
		common.BigIntToBytes(ab.Amount),
		ab.TokenStandard.Bytes(),
		ab.FromBlockHash.Bytes(),
		ab.DescendantBlocksHash().Bytes(),
		types.NewHash(ab.Data).Bytes(),
		common.Uint64ToBytes(ab.FusedPlasma),
		common.Uint64ToBytes(ab.Difficulty),
		ab.Nonce.Data[:],
	))
}

// Producer returns the [types.Address] derived from [PublicKey] (memoized
// after the first call). For send blocks this is the sender's account
// address; for receive blocks authored by users it is the receiver's.
//
// Concurrency: the cache is not synchronized; callers that share an
// [AccountBlock] across goroutines should call Producer first or treat
// the cache field as racy.
func (ab *AccountBlock) Producer() types.Address {
	if ab.producer == nil {
		producer := types.PubKeyToAddress(ab.PublicKey)
		ab.producer = &producer
	}

	return *ab.producer
}

// IsSendBlock reports whether ab is a send block.
func (ab *AccountBlock) IsSendBlock() bool {
	return IsSendBlock(ab.BlockType)
}

// IsReceiveBlock reports whether ab is a receive block.
func (ab *AccountBlock) IsReceiveBlock() bool {
	return IsReceiveBlock(ab.BlockType)
}

// IsSendBlock reports whether blockType is one of the send variants.
func IsSendBlock(blockType uint64) bool {
	return blockType == BlockTypeUserSend || blockType == BlockTypeContractSend
}

// IsReceiveBlock reports whether blockType is one of the receive variants
// (including [BlockTypeGenesisReceive]).
func IsReceiveBlock(blockType uint64) bool {
	return blockType == BlockTypeUserReceive || blockType == BlockTypeContractReceive || blockType == BlockTypeGenesisReceive
}

// Proto wraps ab in an [AccountBlockProto] for protobuf serialization.
// Recursively encodes every entry of [DescendantBlocks].
func (ab *AccountBlock) Proto() *AccountBlockProto {
	pb := &AccountBlockProto{
		Version:              ab.Version,
		ChainIdentifier:      ab.ChainIdentifier,
		BlockType:            ab.BlockType,
		Hash:                 ab.Hash.Proto(),
		PreviousHash:         ab.PreviousHash.Proto(),
		Height:               ab.Height,
		MomentumAcknowledged: ab.MomentumAcknowledged.Proto(),
		Address:              ab.Address.Proto(),

		ToAddress:     ab.ToAddress.Proto(),
		Amount:        common.BigIntToBytes(ab.Amount),
		TokenStandard: ab.TokenStandard.Bytes(),

		FromBlockHash: ab.FromBlockHash.Proto(),

		DescendantBlocks: nil,

		Data: ab.Data,

		FusedPlasma: ab.FusedPlasma,
		Difficulty:  ab.Difficulty,
		Nonce:       ab.Nonce.Serialize(),
		BasePlasma:  ab.BasePlasma,
		TotalPlasma: ab.TotalPlasma,

		ChangesHash: ab.ChangesHash.Proto(),

		PublicKey: ab.PublicKey,
		Signature: ab.Signature,
	}

	pb.DescendantBlocks = make([]*AccountBlockProto, 0, len(ab.DescendantBlocks))
	for _, dBlock := range ab.DescendantBlocks {
		pb.DescendantBlocks = append(pb.DescendantBlocks, dBlock.Proto())
	}

	return pb
}

// DeProtoAccountBlock decodes an [AccountBlockProto] back into an
// [AccountBlock], recursively reconstructing the descendants tree.
func DeProtoAccountBlock(pb *AccountBlockProto) *AccountBlock {
	ab := &AccountBlock{
		Version:              pb.Version,
		ChainIdentifier:      pb.ChainIdentifier,
		BlockType:            pb.BlockType,
		Hash:                 *types.DeProtoHash(pb.Hash),
		PreviousHash:         *types.DeProtoHash(pb.PreviousHash),
		Height:               pb.Height,
		MomentumAcknowledged: *types.DeProtoHashHeight(pb.MomentumAcknowledged),
		Address:              *types.DeProtoAddress(pb.Address),
		ToAddress:            *types.DeProtoAddress(pb.ToAddress),
		Amount:               common.BytesToBigInt(pb.Amount),
		TokenStandard:        types.BytesToZTSPanic(pb.TokenStandard),
		FromBlockHash:        *types.DeProtoHash(pb.FromBlockHash),
		DescendantBlocks:     make([]*AccountBlock, len(pb.DescendantBlocks)),
		Data:                 pb.Data,
		FusedPlasma:          pb.FusedPlasma,
		Difficulty:           pb.Difficulty,
		Nonce:                DeSerializeNonce(pb.Nonce),
		BasePlasma:           pb.BasePlasma,
		TotalPlasma:          pb.TotalPlasma,

		ChangesHash: *types.DeProtoHash(pb.ChangesHash),

		PublicKey: pb.PublicKey,
		Signature: pb.Signature,
	}

	for index, dBlockProto := range pb.DescendantBlocks {
		ab.DescendantBlocks[index] = DeProtoAccountBlock(dBlockProto)
	}
	return ab
}

// Serialize encodes ab as protobuf bytes — the on-disk and on-the-wire
// representation.
func (ab *AccountBlock) Serialize() ([]byte, error) {
	return proto.Marshal(ab.Proto())
}

// DeserializeAccountBlock decodes protobuf bytes into an [AccountBlock].
// Returns an error if the bytes are not a valid encoded [AccountBlockProto].
func DeserializeAccountBlock(data []byte) (*AccountBlock, error) {
	pb := &AccountBlockProto{}
	if err := proto.Unmarshal(data, pb); err != nil {
		return nil, err
	}
	return DeProtoAccountBlock(pb), nil
}

// AccountBlockMarshal is the JSON-friendly twin of [AccountBlock]. It
// renders [AccountBlock.Amount] and [AccountBlock.Nonce] as strings so they
// round-trip cleanly through JavaScript and other languages without
// big-integer support, and is produced/consumed transparently by
// [AccountBlock.MarshalJSON] / [AccountBlock.UnmarshalJSON].
type AccountBlockMarshal struct {
	Version         uint64 `json:"version"`
	ChainIdentifier uint64 `json:"chainIdentifier"`
	BlockType       uint64 `json:"blockType"`

	Hash                 types.Hash       `json:"hash"`
	PreviousHash         types.Hash       `json:"previousHash"`
	Height               uint64           `json:"height"`
	MomentumAcknowledged types.HashHeight `json:"momentumAcknowledged"`

	Address types.Address `json:"address"`

	// Send information
	ToAddress     types.Address            `json:"toAddress"`
	Amount        string                   `json:"amount"`
	TokenStandard types.ZenonTokenStandard `json:"tokenStandard"`

	// Receive information
	FromBlockHash types.Hash `json:"fromBlockHash"`

	// Batch information
	DescendantBlocks []*AccountBlock `json:"descendantBlocks"` // hash of DescendantBlocks is included in hash

	Data []byte `json:"data"` // hash of Data is included in hash

	FusedPlasma uint64 `json:"fusedPlasma"`
	Difficulty  uint64 `json:"difficulty"`
	Nonce       string `json:"nonce"`
	BasePlasma  uint64 `json:"basePlasma"` // not included in hash, the smallest value of TotalPlasma required for block
	TotalPlasma uint64 `json:"usedPlasma"` // not included in hash, TotalPlasma = FusedPlasma + PowPlasma

	ChangesHash types.Hash `json:"changesHash"` // not included in hash

	producer  *types.Address    // not included in hash, for caching purpose only
	PublicKey ed25519.PublicKey `json:"publicKey"` // not included in hash
	Signature []byte            `json:"signature"` // not included in hash
}

// ToNomMarshalJson projects ab into the JSON-friendly twin, stringifying
// [AccountBlock.Amount] and hex-encoding [AccountBlock.Nonce]. The
// descendant slice is shared by reference.
func (ab *AccountBlock) ToNomMarshalJson() *AccountBlockMarshal {
	aux := &AccountBlockMarshal{
		Version:              ab.Version,
		ChainIdentifier:      ab.ChainIdentifier,
		BlockType:            ab.BlockType,
		Hash:                 ab.Hash,
		PreviousHash:         ab.PreviousHash,
		Height:               ab.Height,
		MomentumAcknowledged: ab.MomentumAcknowledged,
		Address:              ab.Address,
		ToAddress:            ab.ToAddress,
		Amount:               ab.Amount.String(),
		TokenStandard:        ab.TokenStandard,
		FromBlockHash:        ab.FromBlockHash,
		Data:                 ab.Data,
		FusedPlasma:          ab.FusedPlasma,
		Difficulty:           ab.Difficulty,
		Nonce:                hex.EncodeToString(ab.Nonce.Data[:]),
		BasePlasma:           ab.BasePlasma,
		TotalPlasma:          ab.TotalPlasma,
		ChangesHash:          ab.ChangesHash,
		PublicKey:            ab.PublicKey,
		Signature:            ab.Signature,
	}

	aux.DescendantBlocks = make([]*AccountBlock, 0, len(ab.DescendantBlocks))
	for _, dBlock := range ab.DescendantBlocks {
		aux.DescendantBlocks = append(aux.DescendantBlocks, dBlock)
	}
	return aux
}

// FromNomMarshalJson is the inverse of [AccountBlock.ToNomMarshalJson]:
// converts the JSON-friendly twin back into an [AccountBlock]. Errors
// while parsing the hex nonce are ignored — the resulting block simply
// has a zero nonce — matching the original (lenient) behavior expected
// by RPC consumers.
func (ab *AccountBlockMarshal) FromNomMarshalJson() *AccountBlock {
	aux := &AccountBlock{
		Version:              ab.Version,
		ChainIdentifier:      ab.ChainIdentifier,
		BlockType:            ab.BlockType,
		Hash:                 ab.Hash,
		PreviousHash:         ab.PreviousHash,
		Height:               ab.Height,
		MomentumAcknowledged: ab.MomentumAcknowledged,
		Address:              ab.Address,
		ToAddress:            ab.ToAddress,
		Amount:               common.StringToBigInt(ab.Amount),
		TokenStandard:        ab.TokenStandard,
		FromBlockHash:        ab.FromBlockHash,
		Data:                 ab.Data,
		FusedPlasma:          ab.FusedPlasma,
		Difficulty:           ab.Difficulty,
		BasePlasma:           ab.BasePlasma,
		TotalPlasma:          ab.TotalPlasma,
		ChangesHash:          ab.ChangesHash,
		PublicKey:            ab.PublicKey,
		Signature:            ab.Signature,
	}
	// ignore the error, it will just not set the nonce
	aux.Nonce.UnmarshalText([]byte(ab.Nonce))

	aux.DescendantBlocks = make([]*AccountBlock, 0, len(ab.DescendantBlocks))
	for _, dBlock := range ab.DescendantBlocks {
		aux.DescendantBlocks = append(aux.DescendantBlocks, dBlock)
	}
	return aux
}

// MarshalJSON renders ab through [AccountBlockMarshal] so amounts and the
// nonce serialize as strings. Implements [json.Marshaler].
func (ab *AccountBlock) MarshalJSON() ([]byte, error) {
	return json.Marshal(ab.ToNomMarshalJson())
}

// UnmarshalJSON parses ab from JSON via [AccountBlockMarshal]. Returns
// an error if the JSON is malformed or the nonce hex is invalid.
// Implements [json.Unmarshaler].
func (ab *AccountBlock) UnmarshalJSON(data []byte) error {
	aux := new(AccountBlockMarshal)
	if err := json.Unmarshal(data, aux); err != nil {
		return err
	}

	ab.Version = aux.Version
	ab.ChainIdentifier = aux.ChainIdentifier
	ab.BlockType = aux.BlockType
	ab.Hash = aux.Hash
	ab.PreviousHash = aux.PreviousHash
	ab.Height = aux.Height
	ab.MomentumAcknowledged = aux.MomentumAcknowledged
	ab.Address = aux.Address
	ab.ToAddress = aux.ToAddress
	ab.Amount = common.StringToBigInt(aux.Amount)
	ab.TokenStandard = aux.TokenStandard
	ab.FromBlockHash = aux.FromBlockHash
	ab.DescendantBlocks = make([]*AccountBlock, len(aux.DescendantBlocks))
	ab.Data = aux.Data
	ab.FusedPlasma = aux.FusedPlasma
	ab.Difficulty = aux.Difficulty
	if err := ab.Nonce.UnmarshalText([]byte(aux.Nonce)); err != nil {
		return err
	}
	ab.BasePlasma = aux.BasePlasma
	ab.TotalPlasma = aux.TotalPlasma
	ab.ChangesHash = aux.ChangesHash
	ab.PublicKey = aux.PublicKey
	ab.Signature = aux.Signature
	for index, dBlock := range aux.DescendantBlocks {
		ab.DescendantBlocks[index] = dBlock
	}

	return nil
}
