package types

import (
	"encoding/hex"
	"fmt"

	"github.com/zenon-network/go-zenon/common/crypto"
)

// HashSize is the byte length of every [Hash]. Hashes are produced by
// [github.com/zenon-network/go-zenon/common/crypto.Hash], which uses SHA3-256.
const (
	HashSize = 32
)

// Hash is a 32-byte cryptographic digest. Display form is hex; the binary
// form is what is signed and stored on chain.
type Hash [HashSize]byte

// ZeroHash is the all-zeros hash sentinel. Treated as "no hash" by callers
// that need to express absence.
var ZeroHash = Hash{}

// NewHash hashes data with the package's canonical hash function and returns
// the result as a [Hash]. Panic-free: the underlying digest is always
// HashSize bytes.
func NewHash(data []byte) Hash {
	h, _ := BytesToHash(crypto.Hash(data))
	return h
}

// SetBytes overwrites h in place with the contents of b. Returns an error if
// b is not exactly [HashSize] bytes; the receiver is unchanged in that case.
func (h *Hash) SetBytes(b []byte) error {
	if len(b) != HashSize {
		return fmt.Errorf("error hash size %v", len(b))
	}
	copy(h[:], b)
	return nil
}

// Bytes returns a fresh slice over h's underlying array. Mutating the slice
// mutates the hash.
func (h Hash) Bytes() []byte {
	return h[:]
}

// IsZero reports whether h equals [ZeroHash].
func (h Hash) IsZero() bool {
	return h == ZeroHash
}

// String renders h as a lower-case hex string with no `0x` prefix.
func (h Hash) String() string {
	return hex.EncodeToString(h[:])
}

// BytesToHash builds a [Hash] from its raw byte form. Returns the zero hash
// and an error if b is not exactly [HashSize] bytes.
func BytesToHash(b []byte) (Hash, error) {
	var h Hash
	err := h.SetBytes(b)
	return h, err
}

// BytesToHashPanic is the panicking variant of [BytesToHash]; intended for
// constants and tests where size mismatch is a programmer error.
func BytesToHashPanic(b []byte) Hash {
	h, err := BytesToHash(b)
	if err != nil {
		panic(err)
	}
	return h
}

// HexToHash decodes a 64-character lower-case hex string into a [Hash].
// Returns an error on length mismatch or invalid hex.
func HexToHash(hexStr string) (Hash, error) {
	if len(hexStr) != 2*HashSize {
		return Hash{}, fmt.Errorf("error hex hash size %v", len(hexStr))
	}
	b, err := hex.DecodeString(hexStr)
	if err != nil {
		return Hash{}, err
	}
	return BytesToHash(b)
}

// HexToHashPanic is the panicking variant of [HexToHash]; intended for
// constants and tests where parse failure is a programmer error.
func HexToHashPanic(hexStr string) Hash {
	h, err := HexToHash(hexStr)
	if err != nil {
		panic(err)
	}
	return h
}

// UnmarshalText parses the hex string form. Implements
// [encoding.TextUnmarshaler] for transparent JSON/text decoding.
func (h *Hash) UnmarshalText(input []byte) error {
	hash, e := HexToHash(string(input))
	if e != nil {
		return e
	}
	return h.SetBytes(hash.Bytes())
}

// MarshalText emits the hex string form. Implements
// [encoding.TextMarshaler] for transparent JSON/text encoding.
func (h Hash) MarshalText() ([]byte, error) {
	return []byte(h.String()), nil
}

// Proto wraps the hash bytes in a [HashProto] for protobuf serialization.
func (h *Hash) Proto() *HashProto {
	return &HashProto{
		Hash: h[:],
	}
}

// DeProtoHash decodes a [HashProto] back into a [Hash]. Panics on size
// mismatch — the protobuf shape is fixed at [HashSize] bytes.
func DeProtoHash(pb *HashProto) *Hash {
	if len(pb.Hash) != HashSize {
		panic(fmt.Sprintf("invalid DeProto - wanted hash size %v but got %v", HashSize, len(pb.Hash)))
	}
	h := new(Hash)
	copy(h[:], pb.Hash)
	return h
}
