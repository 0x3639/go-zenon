package wallet

import (
	"encoding/json"
	"os"

	"github.com/ethereum/go-ethereum/common/hexutil"

	"github.com/zenon-network/go-zenon/common/types"
)

// On-disk format constants. Bumping cryptoStoreVersion or changing
// aesMode / argonName makes existing key files incompatible — readers
// validate these fields on every open.
const (
	// cryptoStoreVersion is the schema version stamped on every key file.
	cryptoStoreVersion = 1
	// aesMode is the AES-GCM variant used for entropy encryption.
	aesMode = "aes-256-gcm"
	// argonName is the password-derivation function name written into
	// every key file.
	argonName = "argon2.IDKey"
)

// KeyFile is the on-disk encrypted representation of a [KeyStore]. The
// BIP-39 entropy is encrypted under an Argon2id-derived key from the
// user's password; the file carries the cipher parameters needed to
// reverse the operation along with the wallet's public base address.
type KeyFile struct {
	Path string

	BaseAddress types.Address `json:"baseAddress"`
	Crypto      cryptoParams  `json:"crypto"`
	Version     int           `json:"version"`
	Timestamp   int64         `json:"timestamp"`
}

// cryptoParams carries the cipher and KDF parameters for a [KeyFile]:
// the cipher name, the KDF identifier, the AES-GCM ciphertext and nonce,
// and the Argon2 parameters used to derive the key.
type cryptoParams struct {
	// Constants
	CipherName string `json:"cipherName"`
	KDF        string `json:"kdf"`
	// Data
	CipherData   hexutil.Bytes `json:"cipherData"`
	AesNonce     hexutil.Bytes `json:"nonce"`
	Argon2Params argon2Params  `json:"argon2Params"`
}

// argon2Params holds the per-keyfile Argon2id salt. Cost parameters are
// hard-coded in [passwordHash.Set] so every keyfile uses the same
// memory/iteration tuning.
type argon2Params struct {
	Salt hexutil.Bytes `json:"salt"`
}

// ReadKeyFile loads and validates a key file from disk. Returns one of
// [ErrKeyFileInvalidVersion], [ErrKeyFileInvalidCipher], or
// [ErrKeyFileInvalidKDF] when the file's format does not match this
// build's expectations.
func ReadKeyFile(path string) (*KeyFile, error) {
	keyFileJson, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	k := &KeyFile{
		Path: path,
	}
	// parse and check entropyJSON params
	if err := json.Unmarshal(keyFileJson, k); err != nil {
		return nil, err
	}
	if k.Version != cryptoStoreVersion {
		return nil, ErrKeyFileInvalidVersion
	}

	// parse and check  cryptoParams params
	if k.Crypto.CipherName != aesMode {
		return nil, ErrKeyFileInvalidCipher
	}
	if k.Crypto.KDF != argonName {
		return nil, ErrKeyFileInvalidKDF
	}
	return k, nil
}

// Write persists kf to its [KeyFile.Path] with restrictive
// (owner-only-read-write) permissions.
func (kf *KeyFile) Write() error {
	keyFileJson, err := json.MarshalIndent(kf, "", "    ")
	if err != nil {
		return err
	}
	return os.WriteFile(kf.Path, keyFileJson, 0700)
}

// Decrypt reverses the AES-GCM encryption with a key derived from
// password and the stored Argon2 salt. Returns [ErrWrongPassword] on any
// authentication-tag mismatch — the GCM failure is converted to a
// password error so callers don't have to distinguish auth failure from
// decryption failure.
func (kf *KeyFile) Decrypt(password string) (*KeyStore, error) {
	derivedKey := new(passwordHash)
	err := derivedKey.SetFromJSON(password, kf.Crypto.Argon2Params)
	if err != nil {
		return nil, err
	}

	entropy, err := aesGCMDecrypt(derivedKey.password[:32], kf.Crypto.CipherData, kf.Crypto.AesNonce)
	if err != nil {
		return nil, ErrWrongPassword
	}

	return keyStoreFromEntropy(entropy)
}
