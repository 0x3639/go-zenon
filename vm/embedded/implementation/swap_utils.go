package implementation

import (
	"crypto/sha256"
	"encoding/base64"
	"math/big"
	"reflect"

	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/crypto/secp256k1"
	"golang.org/x/crypto/ripemd160"

	"github.com/zenon-network/go-zenon/common"
	"github.com/zenon-network/go-zenon/common/types"
	"github.com/zenon-network/go-zenon/vm/constants"
)

// swapUtilsLog is the per-helper logger.
var swapUtilsLog = common.EmbeddedLogger.New("contract", "swap-utils-log")

// Canonical pre-image strings for the secp256k1 messages signed
// during legacy-asset retrieval and legacy-pillar registration.
// The on-chain verifier rebuilds the message exactly from these
// constants plus the caller's public key and Zenon address.
const (
	hashHeader          = "Zenon secp256k1 signature:"
	assetsMessage       = "ZNN swap retrieve assets"
	legacyPillarMessage = "ZNN swap retrieve legacy pillar"

	// SwapRetrieveAssets selects the assets-retrieval message.
	SwapRetrieveAssets = 1
	// SwapRetrieveLegacyPillar selects the legacy-pillar
	// registration message.
	SwapRetrieveLegacyPillar = 2
)

// toOldSignature converts a go-ethereum-style 65-byte signature
// (r || s || v) into the legacy ZNN-style format (header byte first,
// where header = v + 31), base64-encoded.
func toOldSignature(signature []byte) string {
	// transform signature in old znn-style signature
	header := signature[64]
	header += 31
	signature = append([]byte{header}, signature[0:64]...)
	return base64.StdEncoding.EncodeToString(signature)
}

// PubKeyToKeyId derives the legacy key id for an uncompressed
// secp256k1 public key: RIPEMD160(SHA256(compressedPubKey)).
// Mirrors the legacy chain's address-derivation rule.
func PubKeyToKeyId(pubKey []byte) []byte {
	A := new(big.Int).SetBytes(pubKey[1:33])
	B := new(big.Int).SetBytes(pubKey[33:])
	compressed := secp256k1.CompressPubkey(A, B)
	sha := sha256.New()
	sha.Write(compressed)
	ripe := ripemd160.New()
	ripe.Write(sha.Sum(nil))
	return ripe.Sum(nil)
}

// PubKeyToKeyIdHash hashes the key id to a [types.Hash] suitable
// as a swap-entry key.
func PubKeyToKeyIdHash(pubKey []byte) types.Hash {
	keyId := PubKeyToKeyId(pubKey)
	sha := sha256.New()
	sha.Write(keyId)
	return types.BytesToHashPanic(sha.Sum(nil))
}

// SignRetrieveAssetsMessage signs the canonical assets-retrieval
// message for tests. Production signers (clients) build the same
// message and sign with the legacy private key.
func SignRetrieveAssetsMessage(address types.Address, prv []byte, pub string) (string, error) {
	// config message & verify against expected message
	message := GetSwapMessage(assetsMessage, pub, address)

	// sign message
	signature, err := secp256k1.Sign(message, prv)
	if err != nil {
		return "", err
	}
	return toOldSignature(signature), nil
}

// SignLegacyPillarMessage signs the canonical legacy-pillar
// message for tests.
func SignLegacyPillarMessage(address types.Address, prv []byte, pub string) (string, error) {
	// config message & verify against expected message
	message := GetSwapMessage(legacyPillarMessage, pub, address)

	// sign message
	signature, err := secp256k1.Sign(message, prv)
	if err != nil {
		return "", err
	}
	return toOldSignature(signature), nil
}

// serializeString prepends the byte length to txt — a length-prefix
// encoding compatible with the legacy chain's message-signing
// format.
func serializeString(txt string) []byte {
	y := append([]byte(""), byte(len(txt)))
	return append(y, []byte(txt)...)
}

// GetSwapMessage builds the canonical message bytes that the
// legacy private key must sign. Format:
//
//	double-SHA256(serialize(hashHeader) || serialize(operation || pub || addr))
func GetSwapMessage(operationMessage string, pubKey string, addr types.Address) []byte {
	var data []byte
	data = append(data, serializeString(hashHeader)...)
	data = append(data, serializeString(operationMessage+" "+pubKey+" "+addr.String())...)
	a := sha256.Sum256(data)
	b := sha256.Sum256(a[:])
	return b[:]
}

// CheckSwapSignature recovers the secp256k1 public key from
// signatureStr and confirms it matches pubKeyStr. messageType
// selects the operation pre-image. Returns one of
// [constants.ErrInvalidB64Decode], [constants.ErrInvalidSwapCode],
// or [constants.ErrInvalidSignature] on failure.
func CheckSwapSignature(messageType int, addr types.Address, pubKeyStr string, signatureStr string) (bool, error) {
	pubKey, err := base64.StdEncoding.DecodeString(pubKeyStr)
	if err != nil {
		swapUtilsLog.Debug("swap-utils-error", "reason", "malformed-pubKey")
		return false, constants.ErrInvalidB64Decode
	}
	if len(pubKey) != 65 {
		swapUtilsLog.Debug("swap-utils-error", "reason", "invalid-pubKey-length")
		return false, constants.ErrInvalidB64Decode
	}

	sig, err := base64.StdEncoding.DecodeString(signatureStr)
	if err != nil {
		swapUtilsLog.Debug("swap-utils-error", "reason", "malformed-signature")
		return false, constants.ErrInvalidB64Decode
	}
	if len(sig) != 65 {
		swapUtilsLog.Debug("swap-utils-error", "reason", "invalid-signature-length")
		return false, constants.ErrInvalidSignature
	}

	var operationMessage string
	if messageType == SwapRetrieveAssets {
		operationMessage = assetsMessage
	} else if messageType == SwapRetrieveLegacyPillar {
		operationMessage = legacyPillarMessage
	} else {
		swapUtilsLog.Debug("swap-utils-error", "reason", "invalid-operation")
		return false, constants.ErrInvalidSwapCode
	}

	message := GetSwapMessage(operationMessage, pubKeyStr, addr)
	swapUtilsLog.Debug("swap-utils-log", "expected-message", hexutil.Encode(message))

	// Transform signature from Old Znn-style to go secp256k1 signature
	header := sig[0]
	header -= 31
	sig = append(sig, header)
	sig = sig[1:]

	recoveredPubKey, err := secp256k1.RecoverPubkey(message, sig)
	if err != nil {
		swapUtilsLog.Debug("swap-utils-error", "reason", err)
		return false, constants.ErrInvalidSignature
	}
	if !reflect.DeepEqual(pubKey, recoveredPubKey) {
		swapUtilsLog.Debug("swap-utils-error", "reason", "invalid-signature")
		return false, constants.ErrInvalidSignature
	}

	return true, nil
}
