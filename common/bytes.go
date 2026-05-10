package common

import (
	"encoding/binary"
	"math/big"

	"github.com/ethereum/go-ethereum/common"
)

// JoinBytes concatenates data using a growing append loop (so the
// allocation count is amortized, not strictly one). Used everywhere
// the codebase composes canonical key forms or hash inputs from
// multiple fixed-size pieces; the inputs are typically small enough
// that the amortized cost is negligible.
func JoinBytes(data ...[]byte) []byte {
	var newData []byte
	for _, d := range data {
		newData = append(newData, d...)
	}
	return newData
}

// Uint32ToBytes encodes x as 4 big-endian bytes. Big-endian is canonical
// throughout the chain so that lexicographic LevelDB key order matches
// numeric height order.
func Uint32ToBytes(x uint32) []byte {
	bytes := make([]byte, 4)
	binary.BigEndian.PutUint32(bytes, x)
	return bytes
}

// Uint64ToBytes encodes height as 8 big-endian bytes. The argument name is
// `height` because the overwhelming use case is encoding a chain height
// for a database key.
func Uint64ToBytes(height uint64) []byte {
	bytes := make([]byte, 8)
	binary.BigEndian.PutUint64(bytes, height)
	return bytes
}

// BytesToUint64 decodes 8 big-endian bytes back into a uint64. Panics if
// bytes is shorter than 8 bytes (the underlying binary package contract).
func BytesToUint64(bytes []byte) uint64 {
	return binary.BigEndian.Uint64(bytes)
}

// BigIntToBytes encodes int as a 32-byte big-endian unsigned integer with
// left-zero padding. A nil int encodes as 32 zero bytes. Matches the
// Solidity ABI representation used by the embedded contracts.
func BigIntToBytes(int *big.Int) []byte {
	if int == nil {
		return common.LeftPadBytes(Big0.Bytes(), 32)
	} else {
		return common.LeftPadBytes(int.Bytes(), 32)
	}
}

// BytesToBigInt decodes a big-endian unsigned integer from bytes. An empty
// slice decodes as zero, matching the inverse of [BigIntToBytes].
func BytesToBigInt(bytes []byte) *big.Int {
	if len(bytes) == 0 {
		return big.NewInt(0)
	} else {
		return new(big.Int).SetBytes(bytes)
	}
}

// IsHexCharacter returns bool of c being a valid hexadecimal.
func IsHexCharacter(c byte) bool {
	return ('0' <= c && c <= '9') || ('a' <= c && c <= 'f') || ('A' <= c && c <= 'F')
}

// IsHex validates whether each byte is valid hexadecimal string.
func IsHex(str string) bool {
	if len(str)%2 != 0 {
		return false
	}
	for _, c := range []byte(str) {
		if !IsHexCharacter(c) {
			return false
		}
	}
	return true
}

// StringToBigInt parses a base-10 [big.Int] string. Returns 0 on parse
// failure or empty input — callers that need to distinguish parse failure
// from "0" should use [big.Int.SetString] directly.
func StringToBigInt(str string) *big.Int {
	x := new(big.Int)
	_, ok := x.SetString(str, 10)
	if !ok {
		x.SetInt64(0)
	}
	return x
}
