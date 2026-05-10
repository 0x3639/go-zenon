package types

import (
	"bytes"
	"fmt"

	"github.com/btcsuite/btcd/btcutil/bech32"
	"golang.org/x/crypto/sha3"
)

// Bech32 encoding parameters for [Address]. The on-the-wire layout is one
// type byte (UserAddrByte or ContractAddrByte) followed by an
// AddressCoreSize-byte payload, encoded with the AddressPrefix human-readable
// part.
const (
	// AddressPrefix is the bech32 human-readable part for every Zenon address.
	AddressPrefix = "z"
	// AddressSize is the total length of the binary representation in bytes:
	// one type byte plus AddressCoreSize payload bytes.
	AddressSize = 1 + AddressCoreSize
	// AddressCoreSize is the number of payload bytes carried in an address
	// (the truncated SHA3-256 of a public key for users; a deterministic
	// vanity payload for embedded contracts).
	AddressCoreSize = 19
)

// Address-type discriminator stored in byte 0 of every [Address].
const (
	// UserAddrByte marks an externally owned account derived from a public
	// key. See [PubKeyToAddress].
	UserAddrByte = byte(0)
	// ContractAddrByte marks an embedded-contract address. See
	// [IsEmbeddedAddress] and the embedded contract address constants below.
	ContractAddrByte = byte(1)
)

// Canonical addresses of the embedded contracts. Each is a vanity bech32
// string that begins `z1qxemdedded` so it is easy to recognize on-chain.
// These are the *only* contract addresses VM code dispatches to; see
// [github.com/zenon-network/go-zenon/vm/embedded].
var (
	// PillarContract registers and rewards block-producing pillars.
	PillarContract = parseEmbedded("z1qxemdeddedxpyllarxxxxxxxxxxxxxxxsy3fmg")
	// PlasmaContract manages QSR fusing for plasma generation.
	PlasmaContract = parseEmbedded("z1qxemdeddedxplasmaxxxxxxxxxxxxxxxxsctrp")
	// StakeContract holds locked ZNN entitlements that yield QSR.
	StakeContract = parseEmbedded("z1qxemdeddedxstakexxxxxxxxxxxxxxxxjv8v62")
	// SporkContract owns spork lifecycle (create/activate).
	SporkContract = parseEmbedded("z1qxemdeddedxsp0rkxxxxxxxxxxxxxxxx956u48")
	// TokenContract issues ZTS tokens (mint, burn, ownership).
	TokenContract = parseEmbedded("z1qxemdeddedxt0kenxxxxxxxxxxxxxxxxh9amk0")
	// SentinelContract registers and rewards sentinel nodes.
	SentinelContract = parseEmbedded("z1qxemdeddedxsentynelxxxxxxxxxxxxxwy0r2r")
	// SwapContract redeems legacy-chain balances for current-chain tokens.
	SwapContract = parseEmbedded("z1qxemdeddedxswapxxxxxxxxxxxxxxxxxxl4yww")
	// LiquidityContract distributes liquidity-program rewards.
	LiquidityContract = parseEmbedded("z1qxemdeddedxlyquydytyxxxxxxxxxxxxflaaae")
	// AcceleratorContract funds projects via on-chain votes.
	AcceleratorContract = parseEmbedded("z1qxemdeddedxaccelerat0rxxxxxxxxxxp4tk22")
	// HtlcContract implements hashed timelock contracts.
	HtlcContract = parseEmbedded("z1qxemdeddedxhtlcxxxxxxxxxxxxxxxxxygecvw")
	// BridgeContract is the cross-chain bridge endpoint (wrap/unwrap).
	BridgeContract = parseEmbedded("z1qxemdeddedxdrydgexxxxxxxxxxxxxxxmqgr0d")

	// EmbeddedContracts enumerates every embedded-contract address. The
	// dispatcher uses this list to validate that a target address belongs to
	// a known system contract.
	EmbeddedContracts = []Address{PlasmaContract, PillarContract, TokenContract, SentinelContract, SwapContract, StakeContract, SporkContract, LiquidityContract, AcceleratorContract, HtlcContract, BridgeContract}
	// EmbeddedWUpdate enumerates the contracts that participate in the
	// post-update receive flow (those whose state must be advanced as part of
	// every momentum tick).
	EmbeddedWUpdate = []Address{PillarContract, StakeContract, SentinelContract, LiquidityContract, AcceleratorContract}

	// SporkAddress is the live spork-controlling address. It is set during
	// node start-up from configuration and consulted by the spork contract to
	// authorize spork creation and activation.
	SporkAddress *Address

	// CommunitySporkAddress is the temporary spork-controlling address used
	// until an embedded governance contract takes over. The address belongs
	// to the Mariposa01 pillar.
	CommunitySporkAddress = ParseAddressPanic("z1qqvwzz2xq7q5gwk6uhcddgrpxlfcyzc8rsu82s")
)

// IsEmbeddedAddress reports whether addr is the address of an embedded
// contract (i.e., its type byte is [ContractAddrByte]). The check is
// structural — it does not validate that the address is one of the
// recognized [EmbeddedContracts].
func IsEmbeddedAddress(addr Address) bool {
	return addr[0] == ContractAddrByte
}

// Address is a 20-byte Zenon account identifier: one type-byte
// ([UserAddrByte] or [ContractAddrByte]) followed by an AddressCoreSize-byte
// payload. Display form is bech32 with the [AddressPrefix] human-readable
// part.
type Address [AddressSize]byte

// ZeroAddress is the all-zeros address sentinel. Treated as "no address" by
// callers that need to express absence.
var ZeroAddress = Address{}

// SetBytes overwrites addr in place with the contents of b. Returns an error
// if b is not exactly [AddressSize] bytes; the receiver is unchanged in that
// case.
func (addr *Address) SetBytes(b []byte) error {
	if length := len(b); length != AddressSize {
		return fmt.Errorf("error address size  %v", length)
	}
	copy(addr[:], b)
	return nil
}

// Bytes returns a fresh slice over addr's underlying array. Mutating the
// slice mutates the address.
func (addr Address) Bytes() []byte { return addr[:] }

// IsZero reports whether addr equals [ZeroAddress].
func (addr Address) IsZero() bool {
	return bytes.Equal(addr.Bytes(), ZeroAddress.Bytes())
}

// String renders addr in bech32 form. Panics on encoding failure, which
// indicates a corrupt address value (encoding cannot fail for a well-formed
// AddressSize byte array).
func (addr Address) String() string {
	s, err := formatBech32(AddressPrefix, addr[:])
	if err != nil {
		panic(err)
	}
	return s
}

// BytesToAddress builds an [Address] from its raw byte form. Returns the zero
// address and an error if b is not exactly [AddressSize] bytes.
func BytesToAddress(b []byte) (Address, error) {
	var a Address
	err := a.SetBytes(b)
	return a, err
}

// ParseAddress decodes a bech32-encoded address string. Returns an error if
// the string is malformed or if the human-readable part is not
// [AddressPrefix].
func ParseAddress(addrStr string) (Address, error) {
	hrp, b, err := parseBech32(addrStr)
	if err != nil {
		return ZeroAddress, err
	}

	if hrp != AddressPrefix {
		return ZeroAddress, fmt.Errorf("invalid address prefix %v", hrp)
	}

	var addr Address
	err = addr.SetBytes(b)
	return addr, err
}

// ParseAddressPanic is the panicking variant of [ParseAddress]; intended for
// constants and tests where parse failure is a programmer error.
func ParseAddressPanic(addrStr string) Address {
	addr, err := ParseAddress(addrStr)
	if err != nil {
		panic(err)
	}
	return addr
}

// parseEmbedded parses an embedded-contract address constant and panics if
// the string is malformed or not flagged as an embedded address. Used to
// initialize the [EmbeddedContracts] table.
func parseEmbedded(addrStr string) Address {
	a, err := ParseAddress(addrStr)
	if err != nil {
		panic(fmt.Sprintf("Address %v err %v", addrStr, err))
	}
	if !IsEmbeddedAddress(a) {
		panic(fmt.Sprintf("Address %v is not a contract address", addrStr))
	}
	return a
}

// PubKeyToAddress derives the user-account address that owns an Ed25519
// public key. The address is `UserAddrByte || SHA3-256(pubKey)[:AddressCoreSize]`.
func PubKeyToAddress(pubKey []byte) Address {
	hash := sha3.Sum256(pubKey)
	var addr Address
	err := addr.SetBytes(append([]byte{UserAddrByte}, hash[:AddressCoreSize]...))
	if err != nil {
		panic(err)
	}
	return addr
}

// MarshalText emits the bech32 string form. Implements [encoding.TextMarshaler]
// for transparent JSON/text encoding.
func (addr Address) MarshalText() ([]byte, error) {
	return []byte(addr.String()), nil
}

// UnmarshalText parses the bech32 string form. Implements
// [encoding.TextUnmarshaler] for transparent JSON/text decoding.
func (addr *Address) UnmarshalText(input []byte) error {
	addresses, err := ParseAddress(string(input))
	if err != nil {
		return err
	}
	err = addr.SetBytes(addresses.Bytes())
	return err
}

// Proto wraps the address bytes in an [AddressProto] for protobuf
// serialization.
func (addr *Address) Proto() *AddressProto {
	return &AddressProto{
		Address: addr[:],
	}
}

// DeProtoAddress decodes an [AddressProto] back into an [Address]. Panics
// on size mismatch — the protobuf shape is fixed at AddressSize bytes.
func DeProtoAddress(pb *AddressProto) *Address {
	if len(pb.Address) != AddressSize {
		panic(fmt.Sprintf("invalid DeProto - wanted address size %v but got %v", AddressSize, len(pb.Address)))
	}
	addr := new(Address)
	copy(addr[:], pb.Address)
	return addr
}

// parseBech32 takes a bech32 address as input and returns the HRP and data
// section of a bech32 address. Used by [ParseAddress] and [ParseZTS].
func parseBech32(addrStr string) (string, []byte, error) {
	rawHRP, decoded, err := bech32.Decode(addrStr)
	if err != nil {
		return "", nil, err
	}
	addrBytes, err := bech32.ConvertBits(decoded, 5, 8, true)
	if err != nil {
		return "", nil, fmt.Errorf("unable to convert address from 5-bit to 8-bit formatting")
	}
	return rawHRP, addrBytes, nil
}

// formatBech32 takes an address's bytes as input and returns a bech32
// address. Used by [Address.String] and [ZenonTokenStandard.String].
func formatBech32(hrp string, payload []byte) (string, error) {
	fiveBits, err := bech32.ConvertBits(payload, 8, 5, true)
	if err != nil {
		return "", fmt.Errorf("unable to convert address from 8-bit to 5-bit formatting")
	}
	addr, err := bech32.Encode(hrp, fiveBits)
	if err != nil {
		return "", err
	}
	return addr, nil
}
