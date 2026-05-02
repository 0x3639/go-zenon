package types

import (
	"google.golang.org/protobuf/proto"

	"github.com/zenon-network/go-zenon/common"
)

// AccountHeader uniquely identifies an account block by its owning address
// and its [HashHeight] within that account's chain. Momentums embed a list
// of these headers in their content to commit to the set of account blocks
// they finalize.
type AccountHeader struct {
	Address Address `json:"address"`
	HashHeight
}

// Identifier returns the embedded [HashHeight] portion of the header — the
// account-chain-relative position with no address.
func (abh *AccountHeader) Identifier() HashHeight {
	return abh.HashHeight
}

// Proto wraps abh in an [AccountHeaderProto] for protobuf serialization.
func (abh *AccountHeader) Proto() *AccountHeaderProto {
	return &AccountHeaderProto{
		Address:    abh.Address.Proto(),
		HashHeight: abh.HashHeight.Proto(),
	}
}

// DeProtoAccountHeader decodes an [AccountHeaderProto] back into an
// [AccountHeader].
func DeProtoAccountHeader(pb *AccountHeaderProto) *AccountHeader {
	return &AccountHeader{
		Address:    *DeProtoAddress(pb.Address),
		HashHeight: *DeProtoHashHeight(pb.HashHeight),
	}
}

// Serialize encodes abh as a protobuf byte slice.
func (abh *AccountHeader) Serialize() ([]byte, error) {
	return proto.Marshal(abh.Proto())
}

// DeserializeAccountHeader decodes a protobuf byte slice into an
// [AccountHeader]. Returns an error if the data is not a valid encoded
// [AccountHeaderProto].
func DeserializeAccountHeader(data []byte) (*AccountHeader, error) {
	pb := new(AccountHeaderProto)
	if err := proto.Unmarshal(data, pb); err != nil {
		return nil, err
	}
	return DeProtoAccountHeader(pb), nil
}

// Bytes returns the `address || height || hash` concatenation used as a
// database key in stores indexed by account header.
func (abh *AccountHeader) Bytes() []byte {
	return common.JoinBytes(
		abh.Address.Bytes(),
		common.Uint64ToBytes(abh.Height),
		abh.Hash.Bytes())
}
