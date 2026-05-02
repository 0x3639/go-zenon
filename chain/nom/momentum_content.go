package nom

import (
	"bytes"
	"sort"

	"github.com/zenon-network/go-zenon/common/types"
)

// AccountBlockHeaderRawLen is the byte length of a single
// [types.AccountHeader] when serialized via [types.AccountHeader.Bytes]:
// 20-byte address + 32-byte hash + 8 bytes of height.
const AccountBlockHeaderRawLen = types.AddressSize + types.HashSize + 8 // (+8 from height)

// MomentumContent is the ordered list of [types.AccountHeader]s a
// [Momentum] commits to. The list is sorted by canonical header bytes (see
// [AccountBlockHeaderComparer]) so different producers building a momentum
// from the same account-block set agree on the same hash.
type MomentumContent []*types.AccountHeader

// Proto encodes mc as a slice of [types.AccountHeaderProto] for protobuf
// serialization.
func (mc *MomentumContent) Proto() []*types.AccountHeaderProto {
	arr := ([]*types.AccountHeader)(*mc)
	list := make([]*types.AccountHeaderProto, len(arr))
	for i := range arr {
		list[i] = arr[i].Proto()
	}
	return list
}

// DeProtoMomentumContent decodes a slice of [types.AccountHeaderProto]s
// back into the corresponding [types.AccountHeader] slice. Used by
// [DeProtoMomentum].
func DeProtoMomentumContent(content []*types.AccountHeaderProto) []*types.AccountHeader {
	list := make([]*types.AccountHeader, len(content))
	for i := range content {
		list[i] = types.DeProtoAccountHeader(content[i])
	}
	return list
}

// Bytes returns the concatenation of every header's canonical byte form.
// Used as the input to [MomentumContent.Hash].
func (mc *MomentumContent) Bytes() []byte {
	arr := ([]*types.AccountHeader)(*mc)
	source := make([]byte, 0, len(arr)*AccountBlockHeaderRawLen)
	for _, header := range arr {
		source = append(source, header.Bytes()...)
	}
	return source
}

// Hash returns the digest committed to by [Momentum.ComputeHash] as the
// content commitment.
func (mc *MomentumContent) Hash() types.Hash {
	return types.NewHash(mc.Bytes())
}

// NewMomentumContent builds the [MomentumContent] for the supplied account
// blocks: extracts each block's header and sorts the result by canonical
// bytes so the resulting hash is order-independent.
func NewMomentumContent(blocks []*AccountBlock) MomentumContent {
	content := make([]*types.AccountHeader, len(blocks))
	for i := range blocks {
		header := blocks[i].Header()
		content[i] = &header
	}
	sort.Slice(content, AccountBlockHeaderComparer(content))
	return content
}

// AccountBlockHeaderComparer returns a less function that orders headers
// by their canonical byte form (`address || height || hash`). This is the
// canonical sort used wherever account headers must be hashed in a
// deterministic order across producers.
func AccountBlockHeaderComparer(list []*types.AccountHeader) func(a, b int) bool {
	return func(a, b int) bool {
		return bytes.Compare(list[a].Bytes(), list[b].Bytes()) <= 0
	}
}
