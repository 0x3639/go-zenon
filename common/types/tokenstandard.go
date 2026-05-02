package types

import (
	"fmt"

	"github.com/pkg/errors"

	"github.com/zenon-network/go-zenon/common"
	"github.com/zenon-network/go-zenon/common/crypto"
)

// Bech32 encoding parameters for [ZenonTokenStandard]. ZTS identifiers are
// 10 bytes encoded with the [ZTSPrefix] human-readable part.
const (
	// ZTSPrefix is the bech32 human-readable part for every ZTS token id.
	ZTSPrefix = "zts"
	// ZenonTokenStandardSize is the binary length of a token-standard
	// identifier in bytes.
	ZenonTokenStandardSize = 10
)

// ZenonTokenStandard (ZTS) identifies a Zenon token, analogous to a
// contract address for an ERC-20 on Ethereum. Display form is bech32 with
// the [ZTSPrefix].
type ZenonTokenStandard [ZenonTokenStandardSize]byte

// ZeroTokenStandard is the all-zeros ZTS sentinel. Treated as "no token" by
// callers that need to express absence.
var ZeroTokenStandard = ZenonTokenStandard{}

// ZnnTokenStandard is the canonical ZNN (network token) identifier.
var ZnnTokenStandard = ParseZTSPanic("zts1znnxxxxxxxxxxxxx9z4ulx")

// QsrTokenStandard is the canonical QSR (gas/staking token) identifier.
var QsrTokenStandard = ParseZTSPanic("zts1qsrxxxxxxxxxxxxxmrhjll")

// NewZenonTokenStandard derives a fresh ZTS identifier by hashing the
// supplied data and truncating to [ZenonTokenStandardSize] bytes. The token
// contract uses this to mint stable IDs from issuance parameters.
func NewZenonTokenStandard(data ...[]byte) ZenonTokenStandard {
	zts, _ := BytesToZTS(crypto.Hash(data...)[0:ZenonTokenStandardSize])
	return zts
}

// SetBytes overwrites zts in place with the contents of b. Returns an error
// if b is not exactly [ZenonTokenStandardSize] bytes.
func (zts *ZenonTokenStandard) SetBytes(b []byte) error {
	if length := len(b); length != ZenonTokenStandardSize {
		return fmt.Errorf("invalid ZTS size error %v", length)
	}
	copy(zts[:], b)
	return nil
}

// Bytes returns a fresh slice over zts's underlying array.
func (zts ZenonTokenStandard) Bytes() []byte { return zts[:] }

// String renders zts in bech32 form. Panics on encoding failure, which would
// indicate a corrupt fixed-size array.
func (zts ZenonTokenStandard) String() string {
	s, err := formatBech32(ZTSPrefix, zts[:])
	if err != nil {
		panic(err)
	}
	return s
}

// BytesToZTS builds a [ZenonTokenStandard] from its raw byte form. Returns
// the zero token standard and an error on size mismatch.
func BytesToZTS(b []byte) (ZenonTokenStandard, error) {
	var zts ZenonTokenStandard
	err := zts.SetBytes(b)
	return zts, err
}

// BytesToZTSPanic is the panicking variant of [BytesToZTS]; intended for
// constants and tests where size mismatch is a programmer error.
func BytesToZTSPanic(b []byte) ZenonTokenStandard {
	var zts ZenonTokenStandard
	common.DealWithErr(zts.SetBytes(b))
	return zts
}

// ParseZTS decodes a bech32-encoded ZTS string. Returns an error if the
// string is malformed or if the human-readable part is not [ZTSPrefix].
func ParseZTS(ztsString string) (ZenonTokenStandard, error) {
	hrp, data, err := parseBech32(ztsString)
	if err != nil {
		return ZeroTokenStandard, err
	}

	if hrp != ZTSPrefix {
		return ZeroTokenStandard, fmt.Errorf("invalid ZTS String prefix %v", hrp)
	}

	var zts ZenonTokenStandard
	err = zts.SetBytes(data)
	if err != nil {
		return ZeroTokenStandard, err
	}
	return zts, nil
}

// ParseZTSPanic is the panicking variant of [ParseZTS]; intended for
// constants and tests where parse failure is a programmer error.
func ParseZTSPanic(ztsString string) ZenonTokenStandard {
	zts, err := ParseZTS(ztsString)
	if err != nil {
		panic(errors.Errorf("failed to parse %v; reason %v", ztsString, err))
	}
	return zts
}

// MarshalText emits the bech32 string form. Implements
// [encoding.TextMarshaler] for transparent JSON/text encoding.
func (zts ZenonTokenStandard) MarshalText() ([]byte, error) {
	return []byte(zts.String()), nil
}

// UnmarshalText parses the bech32 string form. Implements
// [encoding.TextUnmarshaler] for transparent JSON/text decoding.
func (zts *ZenonTokenStandard) UnmarshalText(input []byte) error {
	raw, err := ParseZTS(string(input))
	if err != nil {
		return err
	}
	return zts.SetBytes(raw.Bytes())
}
