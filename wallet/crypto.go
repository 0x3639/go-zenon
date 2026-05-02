package wallet

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/ed25519"
	"crypto/rand"
	"io"

	"github.com/pkg/errors"
)

// gcmAdditionData is the AES-GCM additional-data tag bound into every
// keyfile's authenticated ciphertext. Acts as a domain separator so a
// keyfile produced by this package cannot be misinterpreted under a
// different scheme.
const (
	gcmAdditionData = "zenon"
)

// aesGCMEncrypt encrypts inText under key with a fresh 12-byte random
// nonce and the [gcmAdditionData] tag. Returns the sealed ciphertext, the
// nonce, and any error from cipher construction.
func aesGCMEncrypt(key, inText []byte) (outText, nonce []byte, err error) {
	aesBlock, err := aes.NewCipher(key)
	if err != nil {
		return nil, nil, err
	}
	stream, err := cipher.NewGCM(aesBlock)
	if err != nil {
		return nil, nil, err
	}

	nonce = GetEntropyCSPRNG(12)

	outText = stream.Seal(nil, nonce, inText, []byte(gcmAdditionData))
	return outText, nonce, err
}

// aesGCMDecrypt opens cipherText sealed by [aesGCMEncrypt]. Returns the
// AES-GCM open error on tag mismatch — callers map that to a higher-level
// error such as [ErrWrongPassword].
func aesGCMDecrypt(key, cipherText, nonce []byte) ([]byte, error) {
	aesBlock, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}

	stream, err := cipher.NewGCM(aesBlock)
	if err != nil {
		return nil, err
	}

	outText, err := stream.Open(nil, nonce, cipherText, []byte(gcmAdditionData))
	if err != nil {
		return nil, err
	}

	return outText, err
}

// GetEntropyCSPRNG returns n cryptographically-secure random bytes from
// crypto/rand. Panics on read failure — a CSPRNG that returns short is
// considered fatal.
func GetEntropyCSPRNG(n int) []byte {
	mainBuff := make([]byte, n)
	_, err := io.ReadFull(rand.Reader, mainBuff)
	if err != nil {
		panic("reading from crypto/rand failed: " + err.Error())
	}
	return mainBuff
}

// VerifySignature checks an Ed25519 signature. Returns an error if pubkey
// is not the expected length; otherwise returns the bool result of
// [ed25519.Verify] with a nil error.
func VerifySignature(pubkey ed25519.PublicKey, message, sig []byte) (bool, error) {
	if len(pubkey) != ed25519.PublicKeySize {
		return false, errors.Errorf("ed25519: bad public key length; length=%v", len(pubkey))
	}
	return ed25519.Verify(pubkey, message, sig), nil
}
