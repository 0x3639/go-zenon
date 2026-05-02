package crypto

import (
	"crypto/sha256"

	"golang.org/x/crypto/sha3"
)

// Hash returns the canonical hash of the receiver.
func Hash(data ...[]byte) []byte {
	d := sha3.New256()
	for _, item := range data {
		d.Write(item)
	}
	return d.Sum(nil)
}

// HashSHA256 is part of the package's public API.
func HashSHA256(data ...[]byte) []byte {
	d := sha256.New()
	for _, item := range data {
		d.Write(item)
	}
	return d.Sum(nil)
}

// Keccak256 is part of the package's public API.
func Keccak256(data ...[]byte) []byte {
	d := sha3.NewLegacyKeccak256()
	for _, item := range data {
		d.Write(item)
	}

	return d.Sum(nil)
}
