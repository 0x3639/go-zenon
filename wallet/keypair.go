package wallet

import (
	"crypto/ed25519"

	"github.com/zenon-network/go-zenon/common/types"
)

// KeyPair bundles an Ed25519 keypair with its derived [types.Address].
// Returned by every derivation entry point ([DeriveForPath],
// [KeyStore.DeriveForIndexPath]) and consumed by the pillar producer and
// the RPC signer.
type KeyPair struct {
	Public  ed25519.PublicKey
	Private ed25519.PrivateKey
	Address types.Address
}

// Sign produces an Ed25519 signature over message using the keypair's
// private key.
func (kp *KeyPair) Sign(message []byte) []byte {
	return ed25519.Sign(kp.Private, message)
}

// Signer is the multi-return shape consumed by the pillar / RPC signing
// path: it returns the signed bytes, the signer's address, the public
// key, and any error. The trailing nil error keeps the signature
// compatible with signers that may fail (e.g., hardware wallets).
func (kp *KeyPair) Signer(data []byte) (signedData []byte, address *types.Address, pubkey []byte, err error) {
	return kp.Sign(data), &kp.Address, kp.Public, nil
}
