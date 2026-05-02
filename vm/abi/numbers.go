package abi

import (
	"math/big"
	"reflect"

	"github.com/zenon-network/go-zenon/common"
	"github.com/zenon-network/go-zenon/common/types"
)

// Word-size constants used by the ABI codec. WordSize matches
// Solidity's 32-byte word; WordBits and WordBytes describe the
// host machine's `big.Word` (used by the [PaddedBigBytes] fast path).
const (
	// WordBits is the number of bits in a [big.Word] on the host
	// architecture.
	WordBits = 32 << (uint64(^big.Word(0)) >> 63)

	// WordBytes is the number of bytes in a [big.Word].
	WordBytes = WordBits / 8

	// WordSize is the number of bytes in one ABI word — always 32.
	WordSize = 32
)

// Cached reflection types for the supported Go-side argument types.
// The codec compares against these to avoid repeated reflect.TypeOf
// allocations on hot paths.
var (
	bigT           = reflect.TypeOf(&big.Int{})
	derefbigT      = reflect.TypeOf(big.Int{})
	uint8T         = reflect.TypeOf(uint8(0))
	uint16T        = reflect.TypeOf(uint16(0))
	uint32T        = reflect.TypeOf(uint32(0))
	uint64T        = reflect.TypeOf(uint64(0))
	int8T          = reflect.TypeOf(int8(0))
	int16T         = reflect.TypeOf(int16(0))
	int32T         = reflect.TypeOf(int32(0))
	int64T         = reflect.TypeOf(int64(0))
	addressT       = reflect.TypeOf(types.Address{})
	tokenStandardT = reflect.TypeOf(types.ZenonTokenStandard{})
	hashT          = reflect.TypeOf(types.Hash{})
)

// U256 converts a big Int into a 256bit VM number: takes n mod 2^256
// and pads to a 32-byte big-endian slice.
func U256(n *big.Int) []byte {
	return PaddedBigBytes(n.And(n, common.BigP256m1), WordSize)
}

// PaddedBigBytes returns the big-endian byte representation of bigint
// padded to n bytes. Used to encode integer arguments at fixed
// 32-byte word size.
func PaddedBigBytes(bigint *big.Int, n int) []byte {
	if bigint.BitLen()/8 >= n {
		return bigint.Bytes()
	}
	ret := make([]byte, n)
	i := len(ret)
	for _, d := range bigint.Bits() {
		for j := 0; j < WordBytes && i > 0; j++ {
			i--
			ret[i] = byte(d)
			d >>= 8
		}
	}
	return ret
}
