package wallet

import (
	"bytes"
	"crypto/ed25519"
	"crypto/hmac"
	"crypto/sha512"
	"encoding/binary"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/zenon-network/go-zenon/common"
	"github.com/zenon-network/go-zenon/common/types"
)

// BIP-32-style derivation parameters used to turn a BIP-39 seed into an
// Ed25519 keypair.
const (
	// ZenonAccountPathFormat is the canonical BIP-44 derivation path for
	// Zenon accounts. Coin type 73404 is allocated to ZNN; the placeholder
	// is the account index.
	ZenonAccountPathFormat = "m/44'/73404'/%d'"
	// FirstHardenedIndex is the BIP-32 hardened-derivation offset. Every
	// segment of a Zenon path is hardened; non-hardened (public)
	// derivation is not defined for Ed25519.
	FirstHardenedIndex = uint32(0x80000000)
	// seedModifier is the HMAC key used to derive the master key from a
	// BIP-39 seed (the canonical SLIP-0010 ed25519 modifier).
	seedModifier = "ed25519 seed"
)

// pathRegex validates that a derivation path is a `/`-separated sequence
// of hardened segments rooted at `m`.
var (
	pathRegex = regexp.MustCompile("^m(\\/[0-9]+')+$")
)

// key is an intermediate derivation node holding the 32-byte private key
// and the 32-byte chain code. Each derivation step produces a new key.
type key struct {
	Key       []byte
	ChainCode []byte
}

// toKeyPair turns the raw 32-byte private-key seed into a fully-formed
// [KeyPair] (Ed25519 keypair + derived address).
func (k key) toKeyPair() (*KeyPair, error) {
	public, private, err := ed25519.GenerateKey(bytes.NewReader(k.Key))
	if err != nil {
		return nil, err
	}
	address := types.PubKeyToAddress(public)
	return &KeyPair{
		Public:  public,
		Private: private,
		Address: address,
	}, nil
}

// DeriveForPath derives the keypair for the given BIP-44 path against the
// supplied seed. Ed25519 derivation operates on hardened keys only; an
// invalid or non-hardened path returns [ErrInvalidPath] or
// [ErrNoPublicDerivation].
func DeriveForPath(path string, seed []byte) (*KeyPair, error) {
	if !isValidPath(path) {
		return nil, ErrInvalidPath
	}

	key, err := newMasterKey(seed)
	if err != nil {
		return nil, err
	}

	segments := strings.Split(path, "/")
	for _, segment := range segments[1:] {
		i64, err := strconv.ParseUint(strings.TrimRight(segment, "'"), 10, 32)
		if err != nil {
			return nil, err
		}

		i := uint32(i64) + FirstHardenedIndex
		key, err = key.derive(i)
		if err != nil {
			return nil, err
		}
	}

	return key.toKeyPair()
}

// DeriveWithIndex is a convenience over [DeriveForPath] that builds the
// canonical Zenon account path for the supplied index.
func DeriveWithIndex(i uint32, seed []byte) (*KeyPair, error) {
	path := fmt.Sprintf(ZenonAccountPathFormat, i)
	return DeriveForPath(path, seed)
}

// newMasterKey runs HMAC-SHA512 over the seed using [seedModifier] as the
// key and splits the result into the master private key and chain code.
func newMasterKey(seed []byte) (*key, error) {
	newHmac := hmac.New(sha512.New, []byte(seedModifier))
	_, err := newHmac.Write(seed)
	if err != nil {
		return nil, err
	}
	sum := newHmac.Sum(nil)
	key := &key{
		Key:       sum[:32],
		ChainCode: sum[32:],
	}
	return key, nil
}

// derive performs one hardened SLIP-0010 derivation step. Returns
// [ErrNoPublicDerivation] for any non-hardened index (Ed25519 has no
// public-derivation form).
func (k *key) derive(i uint32) (*key, error) {
	// no public derivation for ed25519
	if i < FirstHardenedIndex {
		return nil, ErrNoPublicDerivation
	}

	iBytes := make([]byte, 4)
	binary.BigEndian.PutUint32(iBytes, i)
	data := common.JoinBytes([]byte{0x0}, k.Key, iBytes)

	newHmac := hmac.New(sha512.New, k.ChainCode)
	_, err := newHmac.Write(data)
	if err != nil {
		return nil, err
	}
	sum := newHmac.Sum(nil)
	return &key{
		Key:       sum[:32],
		ChainCode: sum[32:],
	}, nil
}

// isValidPath reports whether path matches [pathRegex] and every segment
// fits in a uint32 once the trailing `'` is stripped.
func isValidPath(path string) bool {
	if !pathRegex.MatchString(path) {
		return false
	}

	// Check for overflows
	segments := strings.Split(path, "/")
	for _, segment := range segments[1:] {
		_, err := strconv.ParseUint(strings.TrimRight(segment, "'"), 10, 32)
		if err != nil {
			return false
		}
	}

	return true
}
