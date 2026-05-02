package types

import (
	"google.golang.org/protobuf/proto"

	"github.com/zenon-network/go-zenon/common"
)

// HashHeight is the canonical (hash, height) pair used to reference a
// position on either ledger — an account chain or the momentum chain.
// Equality of `HashHeight` between two views means they agree on the
// content at that position.
type HashHeight struct {
	Hash   Hash   `json:"hash"`
	Height uint64 `json:"height"`
}

// ZeroHashHeight is the all-zeros sentinel. Stores use it to express the
// position before genesis.
var ZeroHashHeight = HashHeight{
	Hash:   ZeroHash,
	Height: 0,
}

// IsZero reports whether b equals [ZeroHashHeight].
func (b HashHeight) IsZero() bool {
	return b == ZeroHashHeight
}

// Bytes returns the binary `hash || height` concatenation used as a database
// key in account and momentum stores.
func (b *HashHeight) Bytes() []byte {
	return common.JoinBytes(
		b.Hash.Bytes(),
		common.Uint64ToBytes(b.Height),
	)
}

// Proto wraps b in a [HashHeightProto] for protobuf serialization.
func (b *HashHeight) Proto() *HashHeightProto {
	return &HashHeightProto{
		Hash:   b.Hash.Proto(),
		Height: b.Height,
	}
}

// DeProtoHashHeight decodes a [HashHeightProto] back into a [HashHeight].
func DeProtoHashHeight(pb *HashHeightProto) *HashHeight {
	return &HashHeight{
		Hash:   *DeProtoHash(pb.Hash),
		Height: pb.Height,
	}
}

// Serialize encodes b as a protobuf byte slice. Panics through
// [common.DealWithErr] if marshalling fails — protobuf serialization on the
// fixed shape is expected to succeed.
func (b *HashHeight) Serialize() []byte {
	data, err := proto.Marshal(b.Proto())
	common.DealWithErr(err)
	return data
}

// DeserializeHashHeight decodes a protobuf byte slice into a [HashHeight].
// Returns an error if data is not a valid encoded [HashHeightProto].
func DeserializeHashHeight(data []byte) (*HashHeight, error) {
	pb := &HashHeightProto{}
	if err := proto.Unmarshal(data, pb); err != nil {
		return nil, err
	}
	return DeProtoHashHeight(pb), nil
}
