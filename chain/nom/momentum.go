package nom

import (
	"crypto/ed25519"
	"time"

	"google.golang.org/protobuf/proto"

	"github.com/zenon-network/go-zenon/common"
	"github.com/zenon-network/go-zenon/common/db"
	"github.com/zenon-network/go-zenon/common/types"
)

// emptyEd25519PublicKey is the all-zeros (nil-equivalent) public key used as
// a sentinel by [Momentum.EnsureCache] to skip producer derivation when the
// public key has not been populated yet (e.g., on a freshly deserialized
// momentum from a peer before signature verification).
var (
	emptyEd25519PublicKey ed25519.PublicKey
)

// MomentumTransaction is the [db.Transaction] wrapper around a momentum
// and the [db.Patch] capturing the state changes its content produced.
// Used by the chain layer to commit a momentum atomically with its
// derived state.
type MomentumTransaction struct {
	Momentum *Momentum
	Changes  db.Patch
}

// GetCommits returns the single momentum as the only commit in the
// transaction; the account blocks the momentum references are committed
// separately by their own [AccountBlockTransaction]s.
func (t *MomentumTransaction) GetCommits() []db.Commit {
	return []db.Commit{t.Momentum}
}

// StealChanges hands the patch to the caller and clears the field.
func (t *MomentumTransaction) StealChanges() db.Patch {
	changes := t.Changes
	t.Changes = nil
	return changes
}

// Momentum is a consensus block: an entry on the global momentum chain
// produced by the elected pillar for the current tick. Each momentum
// commits to a list of account-block headers ([Content]) and to the patch
// hash those blocks produced ([ChangesHash]), giving a single root the
// network agrees on per tick.
//
// Field grouping:
//
//   - Identity: Version, ChainIdentifier, Hash, PreviousHash, Height.
//   - Time: TimestampUnix is canonical; Timestamp is a derived
//     [time.Time] cached for convenience and excluded from the hash.
//   - Content: Data is opaque per-momentum metadata; Content is the list
//     of [types.AccountHeader]s anchored.
//   - State commitment: ChangesHash binds the aggregate state delta of
//     this momentum's account blocks.
//   - Authentication: PublicKey + Signature; producer is a cached derived
//     value (see [Momentum.Producer]).
type Momentum struct {
	Version         uint64 `json:"version"`
	ChainIdentifier uint64 `json:"chainIdentifier"`

	Hash         types.Hash `json:"hash"`
	PreviousHash types.Hash `json:"previousHash"`
	Height       uint64     `json:"height"`

	TimestampUnix uint64     `json:"timestamp"` // hash item 3
	Timestamp     *time.Time `json:"-" rlp:"-"` // not included in hash, for caching purpose only

	Data    []byte          `json:"data"`    // hash of Data is included in hash
	Content MomentumContent `json:"content"` // hash of Content is included in hash

	ChangesHash types.Hash `json:"changesHash"`

	producer  *types.Address    `rlp:"-"`          // not included in hash, for caching purpose only
	PublicKey ed25519.PublicKey `json:"publicKey"` // not included in hash
	Signature []byte            `json:"signature"` // not included in hash
}

// DetailedMomentum bundles a [Momentum] with the full [AccountBlock] bodies
// it commits to. RPC handlers return this when callers want a momentum
// together with the transactions it finalized.
type DetailedMomentum struct {
	Momentum      *Momentum       `json:"momentum"`
	AccountBlocks []*AccountBlock `json:"accountBlocks"`
}

// ComputeHash returns the canonical hash of m. Binds the version, chain
// identifier, previous hash, height, timestamp, [Data] hash, [Content]
// hash, and [ChangesHash]. Does not include [PublicKey] / [Signature];
// those authenticate the hash, they don't extend it.
//
// Invariant: a momentum's stored Hash always equals ComputeHash().
func (m *Momentum) ComputeHash() types.Hash {
	return types.NewHash(common.JoinBytes(
		common.Uint64ToBytes(m.Version),
		common.Uint64ToBytes(m.ChainIdentifier),
		m.PreviousHash.Bytes(),
		common.Uint64ToBytes(m.Height),
		common.Uint64ToBytes(m.TimestampUnix),
		types.NewHash(m.Data).Bytes(),
		m.Content.Hash().Bytes(),
		m.ChangesHash.Bytes(),
	))
}

// Identifier returns the (Hash, Height) pair locating m on the momentum
// chain. Implements [db.Commit].
func (m *Momentum) Identifier() types.HashHeight {
	return types.HashHeight{
		Height: m.Height,
		Hash:   m.Hash,
	}
}

// Previous returns the predecessor momentum's [HashHeight]. Implements
// [db.Commit].
func (m *Momentum) Previous() types.HashHeight {
	return types.HashHeight{
		Hash:   m.PreviousHash,
		Height: m.Height - 1,
	}
}

// Producer returns the address of the pillar that signed the momentum,
// memoizing the result on first call. Used by the verifier to confirm the
// producer matches the elected pillar for the momentum's tick.
//
// Concurrency: the cache is not synchronized; treat as racy across
// goroutines unless [Momentum.EnsureCache] has been called first.
func (m *Momentum) Producer() types.Address {
	if m.producer == nil {
		producer := types.PubKeyToAddress(m.PublicKey)
		m.producer = &producer
	}
	return *m.producer
}

// EnsureCache populates the lazily-derived fields ([Timestamp] and the
// [Producer] cache) so subsequent reads are race-free. Called by
// [DeProtoMomentum] after deserialization.
func (m *Momentum) EnsureCache() {
	if m.Timestamp == nil {
		timestamp := time.Unix(int64(m.TimestampUnix), 0)
		m.Timestamp = &timestamp
	}
	// don't call producer before publicKey is set
	if !m.PublicKey.Equal(emptyEd25519PublicKey) {
		m.Producer()
	}
}

// Proto wraps m in a [MomentumProto] for protobuf serialization.
func (m *Momentum) Proto() *MomentumProto {
	return &MomentumProto{
		Version:         m.Version,
		ChainIdentifier: m.ChainIdentifier,
		Hash:            m.Hash.Proto(),
		PreviousHash:    m.PreviousHash.Proto(),
		Height:          m.Height,
		Timestamp:       m.TimestampUnix,
		Data:            m.Data,
		Content:         m.Content.Proto(),
		ChangesHash:     m.ChangesHash.Proto(),
		PublicKey:       m.PublicKey,
		Signature:       m.Signature,
	}
}

// DeProtoMomentum decodes a [MomentumProto] back into a [Momentum] and
// primes the lazy caches via [Momentum.EnsureCache].
func DeProtoMomentum(pb *MomentumProto) *Momentum {
	m := &Momentum{
		Version:         pb.Version,
		ChainIdentifier: pb.ChainIdentifier,
		Hash:            *types.DeProtoHash(pb.Hash),
		PreviousHash:    *types.DeProtoHash(pb.PreviousHash),
		Height:          pb.Height,
		TimestampUnix:   pb.Timestamp,
		Data:            pb.Data,
		Content:         DeProtoMomentumContent(pb.Content),
		ChangesHash:     *types.DeProtoHash(pb.ChangesHash),
		PublicKey:       pb.PublicKey,
		Signature:       pb.Signature,
	}
	m.EnsureCache()
	return m
}

// Serialize encodes m as protobuf bytes — the on-disk and on-the-wire
// representation.
func (m *Momentum) Serialize() ([]byte, error) {
	pb := m.Proto()
	buf, err := proto.Marshal(pb)
	if err != nil {
		return nil, err
	}
	return buf, nil
}

// DeserializeMomentum decodes protobuf bytes into a [Momentum]. Returns an
// error if the bytes are not a valid encoded [MomentumProto].
func DeserializeMomentum(data []byte) (*Momentum, error) {
	pb := &MomentumProto{}
	if err := proto.Unmarshal(data, pb); err != nil {
		return nil, err
	}
	return DeProtoMomentum(pb), nil
}
