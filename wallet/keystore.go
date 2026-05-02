package wallet

import (
	"fmt"
	"time"

	"github.com/tyler-smith/go-bip39"

	"github.com/zenon-network/go-zenon/common/types"
)

// maxSearchIndex caps how many derivation indexes [KeyStore.FindAddress]
// will scan before giving up.
const (
	maxSearchIndex = 128
)

// KeyStore is the in-memory, decrypted view of a wallet: the BIP-39
// entropy, the derived seed, the human-readable mnemonic, and the
// [types.Address] of the index-0 account (the wallet's "base address",
// used as the wallet's identity in logs and RPC).
//
// A [KeyStore] is produced either fresh from entropy via
// [keyStoreFromEntropy] or by decrypting a [KeyFile] with the user's
// password. [KeyStore.Zero] should be called when the keystore is no
// longer needed; the [Manager] does this for every decrypted keystore at
// shutdown.
type KeyStore struct {
	Entropy  []byte
	Seed     []byte
	Mnemonic string

	BaseAddress types.Address
}

// keyStoreFromEntropy builds a fresh [KeyStore] from the supplied entropy:
// derives the BIP-39 mnemonic and seed, and computes the index-0 address.
func keyStoreFromEntropy(entropy []byte) (*KeyStore, error) {
	mnemonic, err := bip39.NewMnemonic(entropy)
	if err != nil {
		return nil, err
	}

	ks := &KeyStore{
		Entropy:  entropy,
		Seed:     bip39.NewSeed(mnemonic, ""),
		Mnemonic: mnemonic,
	}

	// setup base address
	if _, kp, err := ks.DeriveForIndexPath(0); err == nil {
		ks.BaseAddress = kp.Address
	} else {
		return nil, err
	}

	return ks, nil
}

// Zero clears every secret-bearing field on ks. Called when the keystore
// is locked or the manager is stopped so secret material does not linger
// in memory.
func (ks *KeyStore) Zero() {
	ks.Entropy = nil
	ks.Seed = nil
	ks.Mnemonic = ""
	ks.BaseAddress = types.ZeroAddress
}

// DeriveForFullPath derives the keypair for the supplied derivation path.
// The first return value is the path that was used and is currently
// always empty — historical artifact preserved for backwards compatibility.
func (ks *KeyStore) DeriveForFullPath(ipath string) (path string, key *KeyPair, err error) {
	key, err = DeriveForPath(ipath, ks.Seed)
	if err != nil {
		return "", nil, err
	}
	return path, key, nil
}

// DeriveForIndexPath is the convenience over [KeyStore.DeriveForFullPath]
// that builds the canonical Zenon account path for the supplied index.
func (ks *KeyStore) DeriveForIndexPath(index uint32) (path string, key *KeyPair, err error) {
	return ks.DeriveForFullPath(fmt.Sprintf(ZenonAccountPathFormat, index))
}

// FindAddress walks derivation indexes 0..maxSearchIndex looking for one
// that derives to address. Returns [ErrAddressNotFound] if no such index
// is reached.
func (ks *KeyStore) FindAddress(address types.Address) (key *KeyPair, index uint32, err error) {
	for index = uint32(0); index < maxSearchIndex; index++ {
		_, key, err = ks.DeriveForIndexPath(index)
		if err != nil {
			return nil, 0, err
		}
		if address == key.Address {
			return
		}
	}
	return nil, 0, ErrAddressNotFound
}

// Encrypt produces a [KeyFile] from ks: derives an Argon2id key from
// password, AES-GCM-encrypts the entropy, and packages the result with
// the cipher parameters needed to decrypt it later.
func (ks *KeyStore) Encrypt(password string) (*KeyFile, error) {
	derivedKey := new(passwordHash)
	err := derivedKey.Set(password)
	if err != nil {
		return nil, err
	}

	cipherData, nonce, err := aesGCMEncrypt(derivedKey.password[:], ks.Entropy)
	if err != nil {
		return nil, err
	}

	return &KeyFile{
		BaseAddress: ks.BaseAddress,
		Crypto: cryptoParams{
			CipherName: aesMode,
			KDF:        argonName,
			CipherData: cipherData,
			AesNonce:   nonce,
			Argon2Params: argon2Params{
				Salt: derivedKey.salt,
			},
		},
		Version:   cryptoStoreVersion,
		Timestamp: time.Now().UTC().Unix(),
	}, nil
}
