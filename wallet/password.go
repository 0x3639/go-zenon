package wallet

import (
	"github.com/ethereum/go-ethereum/common/hexutil"
	"golang.org/x/crypto/argon2"
)

// passwordHash holds the 32-byte Argon2id-derived key used to AES-GCM-encrypt
// the wallet entropy, plus the per-keyfile salt used to derive it.
type passwordHash struct {
	password [32]byte
	salt     hexutil.Bytes
}

// Set derives a fresh keyfile-encryption key from password using a new
// 16-byte random salt. Argon2id parameters: time=1, memory=64 MiB,
// parallelism=4, output=32 bytes.
func (h *passwordHash) Set(password string) error {
	h.salt = GetEntropyCSPRNG(16)
	// pw is the salted, hashed password
	pw := argon2.IDKey([]byte(password), h.salt, 1, 64*1024, 4, 32)
	copy(h.password[:], pw[:32])
	return nil
}

// SetFromJSON re-derives the keyfile-encryption key from password using
// the salt persisted in params (the inverse of [passwordHash.Set]).
// Argon2id parameters match [passwordHash.Set].
func (h *passwordHash) SetFromJSON(password string, params argon2Params) error {
	h.salt = params.Salt
	// pw is the salted, hashed password
	pw := argon2.IDKey([]byte(password), h.salt, 1, 64*1024, 4, 32)
	copy(h.password[:], pw[:32])
	return nil
}
