package definition

import (
	"encoding/binary"
	"encoding/json"
	"github.com/syndtr/goleveldb/leveldb"
	"github.com/zenon-network/go-zenon/common/crypto"
	"math"
	"math/big"
	"reflect"
	"strconv"
	"strings"

	eabi "github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/zenon-network/go-zenon/common"
	"github.com/zenon-network/go-zenon/common/db"
	"github.com/zenon-network/go-zenon/common/types"
	"github.com/zenon-network/go-zenon/vm/abi"
	"github.com/zenon-network/go-zenon/vm/constants"
)

// jsonBridge is the canonical Solidity-shaped ABI for the Bridge
// contract. The bridge spans many flows:
//
//   - Wrap (outbound): user wrap requests turn ZTS tokens into
//     redeemable claims on a remote network.
//     [WrapTokenMethodName] / [UpdateWrapRequestMethodName].
//   - Unwrap (inbound): TSS-signed unwrap claims credit ZTS
//     tokens to the destination address; users claim them via
//     redeem after the configured delay.
//     [UnwrapTokenMethodName] / [RedeemUnwrapMethodName] /
//     [RevokeUnwrapRequestMethodName].
//   - Network configuration: [SetNetworkMethodName],
//     [RemoveNetworkMethodName], [SetTokenPairMethod],
//     [RemoveTokenPairMethodName], plus metadata setters.
//   - Halt / unhalt and emergency: incident-response hooks
//     ([HaltMethodName], [UnhaltMethodName],
//     [EmergencyMethodName]).
//   - Governance: administrator rotation
//     ([ChangeAdministratorMethodName],
//     [ProposeAdministratorMethodName]), guardian nomination
//     ([NominateGuardiansMethodName]), TSS key rotation
//     ([ChangeTssECDSAPubKeyMethodName]), allow-keygen toggle
//     ([SetAllowKeygenMethodName]), orchestrator info, and
//     metadata.
//
// Storage records: bridgeInfo (singleton), networkInfo (per
// remote network), tokenPair (per network × token), wrapRequest /
// unwrapRequest (per outbound / inbound), securityInfo (guardians /
// delays), orchestratorInfo, redeemHistory.
const (
	jsonBridge = `
	[
		{"type":"function","name":"WrapToken", "inputs":[
			{"name":"networkClass","type":"uint32"},
			{"name":"chainId","type":"uint32"},
			{"name":"toAddress","type":"string"}
		]},

		{"type":"function","name":"UpdateWrapRequest", "inputs":[
			{"name":"id","type":"hash"},
			{"name":"signature","type":"string"}
		]},

		{"type":"function","name":"SetNetwork", "inputs":[
			{"name":"networkClass","type":"uint32"},
			{"name":"chainId","type":"uint32"},
			{"name":"name","type":"string"},
			{"name":"contractAddress","type":"string"},
			{"name":"metadata","type":"string"}
		]},

		{"type":"function","name":"RemoveNetwork", "inputs":[
			{"name":"networkClass","type":"uint32"},
			{"name":"chainId","type":"uint32"}
		]},

		{"type":"function","name":"SetTokenPair","inputs":[
			{"name":"networkClass","type":"uint32"},
			{"name":"chainId","type":"uint32"},
			{"name":"tokenStandard","type":"tokenStandard"},
			{"name":"tokenAddress","type":"string"},
			{"name":"bridgeable","type":"bool"},
			{"name":"redeemable","type":"bool"},
			{"name":"owned","type":"bool"},
			{"name":"minAmount","type":"uint256"},
			{"name":"feePercentage","type":"uint32"},
			{"name":"redeemDelay","type":"uint32"},
			{"name":"metadata","type":"string"}
		]},

		{"type":"function","name":"SetNetworkMetadata","inputs":[
			{"name":"networkClass","type":"uint32"},
			{"name":"chainId","type":"uint32"},
			{"name":"metadata","type":"string"}
		]},

		{"type":"function","name":"RemoveTokenPair","inputs":[
			{"name":"networkClass","type":"uint32"},
			{"name":"chainId","type":"uint32"},
			{"name":"tokenStandard","type":"tokenStandard"},
			{"name":"tokenAddress","type":"string"}
		]},

		{"type":"function","name":"Halt","inputs":[
			{"name":"signature","type":"string"}
		]},

		{"type":"function","name":"Unhalt","inputs":[]},
		{"type":"function","name":"Emergency","inputs":[]},
		
		{"type":"function","name":"ChangeTssECDSAPubKey","inputs":[
			{"name":"pubKey","type":"string"},
			{"name":"oldPubKeySignature","type":"string"},
			{"name":"newPubKeySignature","type":"string"}
		]},

		{"type":"function","name":"ChangeAdministrator","inputs":[
			{"name":"administrator","type":"address"}
		]},
		
		{"type":"function","name":"ProposeAdministrator","inputs":[
			{"name":"address","type":"address"}
		]},

		{"type":"function","name":"SetAllowKeyGen","inputs":[
			{"name":"allowKeyGen","type":"bool"}
		]},

		{"type":"function","name":"SetRedeemDelay","inputs":[
			{"name":"redeemDelay","type":"uint64"}
		]},

		{"type":"function","name":"SetBridgeMetadata","inputs":[
			{"name":"metadata","type":"string"}
		]},

		{"type":"function","name":"UnwrapToken","inputs":[
			{"name":"networkClass","type":"uint32"},
			{"name":"chainId","type":"uint32"},
			{"name":"transactionHash","type":"hash"},
			{"name":"logIndex","type":"uint32"},
			{"name":"toAddress","type":"address"},
			{"name":"tokenAddress","type":"string"},
			{"name":"amount","type":"uint256"},
			{"name":"signature","type":"string"}
		]},

		{"type":"function","name":"RevokeUnwrapRequest","inputs":[
			{"name":"transactionHash","type":"hash"},
			{"name":"logIndex","type":"uint32"}
		]},

		{"type":"function","name":"Redeem","inputs":[
			{"name":"transactionHash","type":"hash"},
			{"name":"logIndex","type":"uint32"}
		]},

		{"type":"function","name":"NominateGuardians","inputs":[
			{"name":"guardians","type":"address[]"}
		]},

		{"type":"function","name":"SetOrchestratorInfo","inputs":[
			{"name":"windowSize","type":"uint64"},
			{"name":"keyGenThreshold","type":"uint32"},
			{"name":"confirmationsToFinality","type":"uint32"},
			{"name":"estimatedMomentumTime","type":"uint32"}
		]},

		{"type":"variable","name":"wrapRequest","inputs":[
			{"name":"networkClass","type":"uint32"},
			{"name":"chainId", "type":"uint32"},
			{"name":"toAddress","type":"string"},
			{"name":"tokenStandard","type":"tokenStandard"},
			{"name":"tokenAddress","type":"string"},
			{"name":"amount","type":"uint256"},
			{"name":"fee","type":"uint256"},
			{"name":"signature","type":"string"},
			{"name":"creationMomentumHeight","type":"uint64"}
		]},

		{"type":"variable","name":"requestPair","inputs":[
			{"name":"creationMomentumHeight","type":"uint64"}
		]},

		{"type":"variable","name":"unwrapRequest","inputs":[
			{"name":"registrationMomentumHeight","type":"uint64"},
			{"name":"networkClass","type":"uint32"},
			{"name":"chainId", "type":"uint32"},
			{"name":"toAddress","type":"address"},
			{"name":"tokenAddress","type":"string"},
			{"name":"tokenStandard","type":"tokenStandard"},
			{"name":"amount","type":"uint256"},
			{"name":"signature","type":"string"},
			{"name":"redeemed","type":"uint8"},
			{"name":"revoked","type":"uint8"}
		]},

		{"type":"variable","name":"bridgeInfo","inputs":[
			{"name":"administrator","type":"address"},
			{"name":"compressedTssECDSAPubKey","type":"string"},
			{"name":"decompressedTssECDSAPubKey","type":"string"},
			{"name":"allowKeyGen","type":"bool"},
			{"name":"halted","type":"bool"},
			{"name":"unhaltedAt","type":"uint64"},
			{"name":"unhaltDurationInMomentums","type":"uint64"},
			{"name":"tssNonce","type":"uint64"},
			{"name":"metadata","type":"string"}
		]},

		{"type":"variable","name":"orchestratorInfo","inputs":[
			{"name":"windowSize","type":"uint64"},
			{"name":"keyGenThreshold","type":"uint32"},
			{"name":"confirmationsToFinality","type":"uint32"},
			{"name":"estimatedMomentumTime","type":"uint32"},
			{"name":"allowKeyGenHeight","type":"uint64"}
		]},

		{"type":"variable","name":"networkInfo","inputs":[
			{"name":"networkClass","type":"uint32"},
			{"name":"id","type":"uint32"},
			{"name":"name","type":"string"},
			{"name":"contractAddress","type":"string"},
			{"name":"metadata","type":"string"},
			{"name":"tokenPairs","type":"bytes[]"}
		]},

		{"type":"variable","name":"tokenPair","inputs":[
			{"name":"tokenStandard","type":"tokenStandard"},
			{"name":"tokenAddress","type":"string"},
			{"name":"bridgeable","type":"bool"},
			{"name":"redeemable","type":"bool"},
			{"name":"owned","type":"bool"},
			{"name":"minAmount","type":"uint256"},
			{"name":"feePercentage","type":"uint32"},
			{"name":"redeemDelay","type":"uint32"},
			{"name":"metadata","type":"string"}
		]},

		{"type":"variable","name":"feeTokenPair","inputs":[
			{"name":"accumulatedFee","type":"uint256"}
		]}
	]`

	// WrapTokenMethodName initiates a wrap (outbound) request.
	WrapTokenMethodName = "WrapToken"
	// UpdateWrapRequestMethodName attaches a TSS signature to a
	// queued wrap request so the orchestrator can execute it on the
	// remote chain.
	UpdateWrapRequestMethodName = "UpdateWrapRequest"
	// UnwrapTokenMethodName admits an inbound unwrap (a TSS-signed
	// claim of a remote-chain transaction) into the contract.
	UnwrapTokenMethodName = "UnwrapToken"
	// RevokeUnwrapRequestMethodName cancels a queued inbound
	// unwrap (administrator-only).
	RevokeUnwrapRequestMethodName = "RevokeUnwrapRequest"
	// RedeemUnwrapMethodName claims tokens for a confirmed inbound
	// unwrap once the redeem-delay window has elapsed.
	RedeemUnwrapMethodName = "Redeem"
	// SetNetworkMethodName registers a remote network.
	SetNetworkMethodName = "SetNetwork"
	// RemoveNetworkMethodName de-registers a remote network.
	RemoveNetworkMethodName = "RemoveNetwork"
	// SetTokenPairMethod registers / configures a (network, token)
	// pair (bridgeability flags, fees, redeem delay).
	SetTokenPairMethod = "SetTokenPair"
	// RemoveTokenPairMethodName removes a (network, token) pair.
	RemoveTokenPairMethodName = "RemoveTokenPair"
	// HaltMethodName halts the bridge (suspends new wraps and
	// redeems).
	HaltMethodName = "Halt"
	// UnhaltMethodName unblocks the bridge after the configured
	// UnhaltDurationInMomentums grace window.
	UnhaltMethodName = "Unhalt"
	// SetAllowKeygenMethodName toggles whether the orchestrator
	// may run a TSS key-generation ceremony.
	SetAllowKeygenMethodName = "SetAllowKeyGen"
	// ChangeTssECDSAPubKeyMethodName rotates the TSS ECDSA key
	// (requires signatures with both the old and new keys).
	ChangeTssECDSAPubKeyMethodName = "ChangeTssECDSAPubKey"
	// SetOrchestratorInfoMethodName updates orchestrator-level
	// configuration (window thresholds, etc.).
	SetOrchestratorInfoMethodName = "SetOrchestratorInfo"

	// SetNetworkMetadataMethodName updates a network's free-form
	// metadata blob.
	SetNetworkMetadataMethodName = "SetNetworkMetadata"
	// SetBridgeMetadataMethodName updates the bridge-level
	// metadata blob.
	SetBridgeMetadataMethodName = "SetBridgeMetadata"

	requestPairVariableName   = "requestPair"
	wrapRequestVariableName   = "wrapRequest"
	unwrapRequestVariableName = "unwrapRequest"
	bridgeInfoVariableName    = "bridgeInfo"

	orchestratorInfoVariableName = "orchestratorInfo"
	networkInfoVariableName      = "networkInfo"
	feeTokenPairVariableName     = "feeTokenPair"
	tokenPairVariableName        = "tokenPair"
)

// ABIBridge is the parsed [abi.ABIContract] for the Bridge
// contract. Per-prefix key namespaces:
// 1=wrapTokenRequest, 2=unwrapTokenRequest, 3=bridgeInfo
// (singleton), 4=orchestratorInfo, 5=networkInfo, 6=tokenPair
// (per network × token), 7=feeTokenPair (per token fee
// accumulator).
//
// Network class discriminators distinguish remote-network families:
// [NoMClass] for sister NoM networks, [EvmClass] for
// Ethereum-compatible chains.
//
// The Uint256Ty / AddressTy / StringTy handles are
// go-ethereum ABI types used to encode unwrap-payload digests for
// signature verification.
var (
	ABIBridge = abi.JSONToABIContract(strings.NewReader(jsonBridge))

	wrapTokenRequestKeyPrefix   = []byte{1}
	unwrapTokenRequestKeyPrefix = []byte{2}
	BridgeInfoKeyPrefix         = []byte{3}
	OrchestratorInfoKeyPrefix   = []byte{4}
	NetworkInfoKeyPrefix        = []byte{5}
	RequestPairKeyPrefix        = []byte{6}
	FeeTokenPairKeyPrefix       = []byte{7}

	// NoMClass is the network-class discriminator for sister Network-of-Momentum networks.
	NoMClass = uint32(1)
	// EvmClass is the network-class discriminator for
	// Ethereum-compatible (EVM) remote networks.
	EvmClass = uint32(2)

	Uint256Ty, _ = eabi.NewType("uint256", "uint256", nil)
	AddressTy, _ = eabi.NewType("address", "address", nil)
	StringTy, _  = eabi.NewType("string", "string", nil)
)

// BridgeInfoVariable is the singleton bridge configuration record:
// administrator authority, the active TSS public key (compressed
// and decompressed forms cached together), the allow-keygen
// toggle, halt state with its grace window, the TSS message
// nonce, and a free-form metadata blob.
type BridgeInfoVariable struct {
	// Administrator address.
	Administrator types.Address `json:"administrator"`
	// ECDSA pub key generated by the orchestrator from key gen
	// ceremony.
	CompressedTssECDSAPubKey   string `json:"compressedTssECDSAPubKey"`
	DecompressedTssECDSAPubKey string `json:"decompressedTssECDSAPubKey"`
	// This specifies whether the orchestrator should key gen or not
	AllowKeyGen bool `json:"allowKeyGen"`
	// This specifies whether the bridge is halted or not
	Halted bool `json:"halted"`
	// Height at which the administrator called unhalt method, UnhaltDurationInMomentums starts from here
	UnhaltedAt uint64 `json:"unhaltedAt"`
	// After we call the unhalt embedded method, the bridge will still be halted for UnhaltDurationInMomentums momentums
	UnhaltDurationInMomentums uint64 `json:"unhaltDurationInMomentums"`
	// An incremental nonce used for signing messages
	TssNonce uint64 `json:"tssNonce"`
	// Additional metadata
	Metadata string `json:"metadata"`
}

// Save persists the receiver under its keyed slot in storage.
// Save serialises b under [BridgeInfoKeyPrefix] in the bridge
// contract's storage.
func (b *BridgeInfoVariable) Save(context db.DB) error {
	data, err := ABIBridge.PackVariable(
		bridgeInfoVariableName,
		b.Administrator,
		b.CompressedTssECDSAPubKey,
		b.DecompressedTssECDSAPubKey,
		b.AllowKeyGen,
		b.Halted,
		b.UnhaltedAt,
		b.UnhaltDurationInMomentums,
		b.TssNonce,
		b.Metadata,
	)
	if err != nil {
		return err
	}
	return context.Put(
		BridgeInfoKeyPrefix,
		data,
	)
}
func parseBridgeInfoVariable(data []byte) (*BridgeInfoVariable, error) {
	if len(data) > 0 {
		bridgeInfo := new(BridgeInfoVariable)
		if err := ABIBridge.UnpackVariable(bridgeInfo, bridgeInfoVariableName, data); err != nil {
			return nil, err
		}
		return bridgeInfo, nil
	} else {
		return &BridgeInfoVariable{
			Administrator:              constants.InitialBridgeAdministrator,
			CompressedTssECDSAPubKey:   "",
			DecompressedTssECDSAPubKey: "",
			AllowKeyGen:                false,
			Halted:                     false,
			UnhaltDurationInMomentums:  constants.MinUnhaltDurationInMomentums,
			TssNonce:                   0,
			Metadata:                   "{}",
		}, nil
		// GetBridgeInfoVariable loads the BridgeInfoVariable record from storage.
	}
}

// GetBridgeInfoVariable loads the global [BridgeInfoVariable] from
// storage, returning the default initial config if no record exists.
func GetBridgeInfoVariable(context db.DB) (*BridgeInfoVariable, error) {
	if data, err := context.Get(BridgeInfoKeyPrefix); err != nil {
		return nil, err
	} else {
		upd, err := parseBridgeInfoVariable(data)
		return upd, err
	}
}

// NetworkInfoVariable is the per-remote-network record. One
// network will always be znn, so we just need the other one — the
// (NetworkClass, Id) pair identifies the remote chain;
// ContractAddress points at the bridge endpoint there;
// TokenPairs is the byte-encoded list of [TokenPair] records.
type NetworkInfoVariable struct {
	NetworkClass    uint32   `json:"networkClass"`
	Id              uint32   `json:"chainId"`
	Name            string   `json:"name"`
	ContractAddress string   `json:"contractAddress"`
	Metadata        string   `json:"metadata"`
	TokenPairs      [][]byte `json:"tokenPairs"`
}

// TokenPair is one (network, token) configuration: bridge / redeem
// flags, fee percentage (basis points; denominator
// [constants.MaximumFee]), per-pair redeem delay in momentums,
// minimum amount, and a free-form metadata blob. Owned indicates
// whether the bridge contract owns the token (mintable on
// inbound) or whether tokens are escrowed on outbound.
type TokenPair struct {
	TokenStandard types.ZenonTokenStandard `json:"tokenStandard"`
	TokenAddress  string                   `json:"tokenAddress"`
	Bridgeable    bool                     `json:"bridgeable"`
	Redeemable    bool                     `json:"redeemable"`
	Owned         bool                     `json:"owned"`
	MinAmount     *big.Int                 `json:"minAmount"`
	FeePercentage uint32                   `json:"feePercentage"`
	RedeemDelay   uint32                   `json:"redeemDelay"`
	Metadata      string                   `json:"metadata"`
}

// TokenPairMarshall is the JSON-friendly twin of [TokenPair]
// (MinAmount as a string for clients without big-integer support).
type TokenPairMarshall struct {
	TokenStandard types.ZenonTokenStandard `json:"tokenStandard"`
	TokenAddress  string                   `json:"tokenAddress"`
	Bridgeable    bool                     `json:"bridgeable"`
	Redeemable    bool                     `json:"redeemable"`
	Owned         bool                     `json:"owned"`
	MinAmount     string                   `json:"minAmount"`
	FeePercentage uint32                   `json:"feePercentage"`
	// ToMarshalJson projects the receiver to its JSON wire form.
	RedeemDelay uint32 `json:"redeemDelay"`
	Metadata    string `json:"metadata"`
}

// ToMarshalJson projects t to its JSON-friendly Marshall twin
// (decimal-string MinAmount).
func (t *TokenPair) ToMarshalJson() *TokenPairMarshall {
	aux := &TokenPairMarshall{
		TokenStandard: t.TokenStandard,
		TokenAddress:  t.TokenAddress,
		Bridgeable:    t.Bridgeable,
		Redeemable:    t.Redeemable,
		Owned:         t.Owned,
		MinAmount:     t.MinAmount.String(),
		FeePercentage: t.FeePercentage,
		// MarshalJSON forwards through the Marshal twin so big.Int fields render as decimal strings.
		RedeemDelay: t.RedeemDelay,
		Metadata:    t.Metadata,
	}
	return aux
	// UnmarshalJSON inflates the JSON wire form back into the in-memory receiver.
}

// MarshalJSON forwards through [TokenPair.ToMarshalJson] so the
// decimal-string Marshall form is what hits the wire.
func (t *TokenPair) MarshalJSON() ([]byte, error) {
	return json.Marshal(t.ToMarshalJson())
}

// UnmarshalJSON inflates the wire form back into native big.Int
// fields.
func (t *TokenPair) UnmarshalJSON(data []byte) error {
	aux := new(TokenPairMarshall)
	if err := json.Unmarshal(data, aux); err != nil {
		return err
	}
	t.TokenStandard = aux.TokenStandard
	t.TokenAddress = aux.TokenAddress
	t.Bridgeable = aux.Bridgeable
	// NetworkInfo is part of the package's public API; see the surrounding code for usage.
	t.Redeemable = aux.Redeemable
	t.Owned = aux.Owned
	t.MinAmount = common.StringToBigInt(aux.MinAmount)
	t.FeePercentage = aux.FeePercentage
	t.RedeemDelay = aux.RedeemDelay
	t.Metadata = aux.Metadata

	return nil
}

// ZtsFeesInfo is part of the package's public API; see the surrounding code for usage.

// NetworkInfo is the inflated form of [NetworkInfoVariable] with
// TokenPairs decoded into structured records — the shape returned
// to RPC callers.
type NetworkInfo struct {
	// ZtsFeesInfoMarshal is part of the package's public API; see the surrounding code for usage.
	NetworkClass    uint32 `json:"networkClass"`
	Id              uint32 `json:"chainId"`
	Name            string `json:"name"`
	ContractAddress string `json:"contractAddress"`
	Metadata        string `json:"metadata"`
	// ToZtsFeesInfoMarshal projects the receiver to its JSON-friendly ZtsFeesInfoMarshal twin.
	TokenPairs []TokenPair `json:"tokenPairs"`
}

// ZtsFeesInfo tracks accumulated bridge fees per token standard.
type ZtsFeesInfo struct {
	TokenStandard  types.ZenonTokenStandard `json:"tokenStandard"`
	AccumulatedFee *big.Int                 `json:"accumulatedFee"`
}

// MarshalJSON forwards through the Marshal twin so big.Int fields render as decimal strings.

// ZtsFeesInfoMarshal is the JSON-friendly twin of [ZtsFeesInfo]
// (decimal-string AccumulatedFee).
type ZtsFeesInfoMarshal struct {
	// UnmarshalJSON inflates the JSON wire form back into the in-memory receiver.
	TokenStandard  types.ZenonTokenStandard `json:"tokenStandard"`
	AccumulatedFee string                   `json:"accumulatedFee"`
}

// ToZtsFeesInfoMarshal projects zfi to its JSON-friendly twin.
func (zfi *ZtsFeesInfo) ToZtsFeesInfoMarshal() *ZtsFeesInfoMarshal {
	aux := &ZtsFeesInfoMarshal{
		TokenStandard:  zfi.TokenStandard,
		AccumulatedFee: zfi.AccumulatedFee.String(),
	}
	// Save persists the receiver under its keyed slot in storage.
	return aux
}

// MarshalJSON forwards through the Marshal twin.
func (zfi *ZtsFeesInfo) MarshalJSON() ([]byte, error) {
	return json.Marshal(zfi.ToZtsFeesInfoMarshal())
}

// UnmarshalJSON inflates the wire form back into native big.Int.
func (zfi *ZtsFeesInfo) UnmarshalJSON(data []byte) error {
	aux := new(ZtsFeesInfoMarshal)
	// Key returns the storage key for the receiver.
	if err := json.Unmarshal(data, aux); err != nil {
		return err
	}
	// Delete removes the receiver's record from storage.
	zfi.TokenStandard = aux.TokenStandard
	zfi.AccumulatedFee = common.StringToBigInt(aux.AccumulatedFee)
	return nil
}

// Save persists zfi under its keyed slot in storage.
func (zfi *ZtsFeesInfo) Save(context db.DB) error {
	data, err := ABIBridge.PackVariable(feeTokenPairVariableName, zfi.AccumulatedFee)
	if err != nil {
		return err
	}
	key, err := zfi.Key()
	if err != nil {
		return err
	}
	return context.Put(key, data)
}

// Key returns the storage key for zfi (token-standard-scoped).
func (zfi *ZtsFeesInfo) Key() ([]byte, error) {
	return common.JoinBytes(FeeTokenPairKeyPrefix, zfi.TokenStandard.Bytes()), nil
}

// Delete removes the receiver's record from storage.
// Delete removes zfi's record from storage.
func (zfi *ZtsFeesInfo) Delete(context db.DB) error {
	key, err := zfi.Key()
	if err != nil {
		return err
	}
	return context.Delete(key)
}
func parseZtsFeesInfoVariable(key []byte, data []byte) (*ZtsFeesInfo, error) {
	if len(data) > 0 {
		feeTokenPair := new(ZtsFeesInfo)
		if err := ABIBridge.UnpackVariable(feeTokenPair, feeTokenPairVariableName, data); err != nil {
			return nil, err
		}
		if err := feeTokenPair.TokenStandard.SetBytes(key[1:]); err != nil {
			return nil, constants.ErrInvalidTokenOrAmount
		}

		return feeTokenPair, nil
	} else {
		// GetNetworkInfoKey loads the NetworkInfoKey record from storage.
		return nil, constants.ErrDataNonExistent
	}
}

// GetZtsFeesInfoVariable loads the [ZtsFeesInfo] for tokenStandard,
// returning a zero record (no error) when none exists.
func GetZtsFeesInfoVariable(context db.DB, tokenStandard types.ZenonTokenStandard) (*ZtsFeesInfo, error) {
	feeTokenPair := &ZtsFeesInfo{
		TokenStandard: tokenStandard,
	}
	// Save persists the receiver under its keyed slot in storage.
	key, err := feeTokenPair.Key()
	if err != nil {
		return nil, err
	}
	if data, err := context.Get(key); err != nil {
		return nil, err
	} else {
		fee, err := parseZtsFeesInfoVariable(key, data)
		if err == constants.ErrDataNonExistent {
			return &ZtsFeesInfo{tokenStandard, big.NewInt(0)}, nil
		} else {
			return fee, err
		}
	}
}

// GetNetworkInfoKey returns the storage key for the
// [NetworkInfoVariable] identified by (networkClass, chainId).
func GetNetworkInfoKey(networkClass uint32, chainId uint32) []byte {
	networkIdBytes := make([]byte, 4)
	binary.BigEndian.PutUint32(networkIdBytes, networkClass)

	chainIdBytes := make([]byte, 4)
	binary.BigEndian.PutUint32(chainIdBytes, chainId)
	return common.JoinBytes(NetworkInfoKeyPrefix, networkIdBytes, chainIdBytes)
}

// Save persists the receiver under its keyed slot in storage.
// Save persists nI under its (NetworkClass, Id) keyed slot.
func (nI *NetworkInfoVariable) Save(context db.DB) error {
	data, err := ABIBridge.PackVariable(
		networkInfoVariableName,
		nI.NetworkClass,
		nI.Id,
		nI.Name,
		nI.ContractAddress,
		nI.Metadata,
		nI.TokenPairs,
	)
	if err != nil {
		return err
	}
	return context.Put(
		nI.Key(),
		data,
	)
}

// Key returns the storage key for nI (network-class + chain-id
// scoped).
func (nI *NetworkInfoVariable) Key() []byte {
	networkClassBytes := make([]byte, 4)
	binary.BigEndian.PutUint32(networkClassBytes, nI.NetworkClass)

	chainIdBytes := make([]byte, 4)
	binary.BigEndian.PutUint32(chainIdBytes, nI.Id)

	return common.JoinBytes(NetworkInfoKeyPrefix, networkClassBytes, chainIdBytes)
}

// Delete removes nI's record from storage.
func (nI *NetworkInfoVariable) Delete(context db.DB) error {
	// EncodeNetworkInfo serialises the input to the package's wire encoding.
	return context.Delete(nI.Key())
}

func parseNetworkInfoVariable(data []byte) (*NetworkInfo, error) {
	if len(data) > 0 {
		networkInfoVariable := new(NetworkInfoVariable)
		if err := ABIBridge.UnpackVariable(networkInfoVariable, networkInfoVariableName, data); err != nil {
			return nil, err
		}
		tokenPairs := make([]TokenPair, 0)
		for _, token := range networkInfoVariable.TokenPairs {
			tokenPair := new(TokenPair)
			if err := ABIBridge.UnpackVariable(tokenPair, tokenPairVariableName, token); err != nil {
				continue
			}
			tokenPairs = append(tokenPairs, *tokenPair)
		}
		networkInfo := &NetworkInfo{
			NetworkClass: networkInfoVariable.NetworkClass,
			// GetNetworkInfoVariable loads the NetworkInfoVariable record from storage.
			Id:              networkInfoVariable.Id,
			Name:            networkInfoVariable.Name,
			ContractAddress: networkInfoVariable.ContractAddress,
			Metadata:        networkInfoVariable.Metadata,
			TokenPairs:      tokenPairs,
		}

		return networkInfo, nil
	} else {
		return nil, constants.ErrDataNonExistent
	}
	// GetNetworkList loads the NetworkList record from storage.
}

// EncodeNetworkInfo packs networkInfo's TokenPairs slice back into
// the byte-encoded form held by [NetworkInfoVariable].
func EncodeNetworkInfo(networkInfo *NetworkInfo) (*NetworkInfoVariable, error) {
	networkInfoVariable := new(NetworkInfoVariable)
	networkInfoVariable.Id = networkInfo.Id
	networkInfoVariable.NetworkClass = networkInfo.NetworkClass
	networkInfoVariable.Name = networkInfo.Name
	networkInfoVariable.ContractAddress = networkInfo.ContractAddress
	networkInfoVariable.Metadata = networkInfo.Metadata
	tokenPairs := make([][]byte, 0)
	for _, token := range networkInfo.TokenPairs {
		if tokenPair, err := ABIBridge.PackVariable(tokenPairVariableName, token.TokenStandard,
			token.TokenAddress, token.Bridgeable, token.Redeemable, token.Owned, token.MinAmount, token.FeePercentage, token.RedeemDelay, token.Metadata); err != nil {
			return nil, err
		} else {
			tokenPairs = append(tokenPairs, tokenPair)
		}
	}
	networkInfoVariable.TokenPairs = tokenPairs
	// GetTokenPairVariable loads the TokenPairVariable record from storage.
	return networkInfoVariable, nil
}

// GetNetworkInfoVariable loads the [NetworkInfo] for
// (networkClass, chainId), returning a zero record (no error) when
// none exists.
func GetNetworkInfoVariable(context db.DB, networkClass uint32, chainId uint32) (*NetworkInfo, error) {
	if data, err := context.Get(GetNetworkInfoKey(networkClass, chainId)); err != nil {
		return nil, err
	} else {
		upd, err := parseNetworkInfoVariable(data)
		if err == constants.ErrDataNonExistent {
			return &NetworkInfo{NetworkClass: 0, Id: 0, Name: "", ContractAddress: "", Metadata: "{}"}, nil
		}
		// RequestPair is part of the package's public API; see the surrounding code for usage.
		return upd, err
	}
}

// GetNetworkList iterates the [NetworkInfoKeyPrefix] range and
// returns every registered network. Skips entries that fail to
// decode.
func GetNetworkList(context db.DB) ([]*NetworkInfo, error) {
	iterator := context.NewIterator(NetworkInfoKeyPrefix)
	defer iterator.Release()
	networkList := make([]*NetworkInfo, 0)

	for {
		if !iterator.Next() {
			common.DealWithErr(iterator.Error())
			// Key returns the storage key for the receiver.
			break
		}
		networkInfo, err := parseNetworkInfoVariable(iterator.Value())
		if err != nil {
			continue
		}
		networkList = append(networkList, networkInfo)
	}

	return networkList, nil
}

// GetTokenPairVariable returns the [TokenPair] for zts on the
// (networkClass, chainId) network, or [leveldb.ErrNotFound] if no
// such pair is configured.
func GetTokenPairVariable(context db.DB, networkClass uint32, chainId uint32, zts types.ZenonTokenStandard) (*TokenPair, error) {
	networkInfo, err := GetNetworkInfoVariable(context, networkClass, chainId)
	if err != nil {
		return nil, err
	}
	// GetRequestPairById loads the RequestPairById record from storage.
	for _, tokenPair := range networkInfo.TokenPairs {
		if reflect.DeepEqual(tokenPair.TokenStandard.Bytes(), zts.Bytes()) {
			return &tokenPair, nil
		}
	}
	return nil, leveldb.ErrNotFound
}

// RequestPair is the (id → creation height) lookup used to find a
// wrap or unwrap request by id without scanning the height-keyed
// primary index.
type RequestPair struct {
	Id                     types.Hash `json:"id"`
	CreationMomentumHeight uint64     `json:"creationMomentumHeight"`
}

// Save persists pair under its keyed slot in storage.
func (pair *RequestPair) Save(context db.DB) error {
	data, err := ABIBridge.PackVariable(
		requestPairVariableName,
		pair.CreationMomentumHeight)
	if err != nil {
		// Save persists the receiver under its keyed slot in storage.
		return err
	}
	return context.Put(getRequestPairKey(pair.Id), data)
}

// Key returns the storage key for pair (id-scoped).
func (pair *RequestPair) Key() []byte {
	return getRequestPairKey(pair.Id)
}
func getRequestPairKey(id types.Hash) []byte {
	return common.JoinBytes(RequestPairKeyPrefix, id[:])
}
func parseRequestPair(data, key []byte) (*RequestPair, error) {
	if len(data) > 0 {
		dataVar := new(RequestPair)
		if err := ABIBridge.UnpackVariable(dataVar, requestPairVariableName, data); err != nil {
			return nil, err
		}
		if err := dataVar.Id.SetBytes(key[1:]); err != nil {
			return nil, err
		}
		return dataVar, nil
	} else {
		return nil, constants.ErrDataNonExistent
	}
}

// GetRequestPairById loads the RequestPairById record from storage.
// GetRequestPairById loads the RequestPair record for Id, or
// [constants.ErrDataNonExistent] if no such request was ever
// recorded.
func GetRequestPairById(context db.DB, Id types.Hash) (*RequestPair, error) {
	key := getRequestPairKey(Id)
	if data, err := context.Get(key); err != nil {
		return nil, err
	} else {
		return parseRequestPair(data, key)
	}
}

// WrapTokenRequest is the on-chain record of an outbound bridge
// transfer (Zenon → remote chain). Indexed by creation-height +
// id; the orchestrator signs Signature once the bridge has
// approved the transfer.
type WrapTokenRequest struct {
	NetworkClass  uint32                   `json:"networkClass"`
	ChainId       uint32                   `json:"chainId"`
	Id            types.Hash               `json:"id"`
	ToAddress     string                   `json:"toAddress"`
	TokenStandard types.ZenonTokenStandard `json:"tokenStandard"`
	// GetWrapTokenRequestById loads the WrapTokenRequestById record from storage.
	TokenAddress           string   `json:"tokenAddress"`
	Amount                 *big.Int `json:"amount"`
	Fee                    *big.Int `json:"fee"`
	Signature              string   `json:"signature"`
	CreationMomentumHeight uint64   `json:"creationMomentumHeight"`
}

// Save writes wrapRequest into both indexes (height-keyed primary
// and id-keyed [RequestPair]).
func (wrapRequest *WrapTokenRequest) Save(context db.DB) error {
	data, err := ABIBridge.PackVariable(
		wrapRequestVariableName,
		wrapRequest.NetworkClass,
		// GetWrapTokenRequests loads the WrapTokenRequests record from storage.
		wrapRequest.ChainId,
		wrapRequest.ToAddress,
		wrapRequest.TokenStandard,
		wrapRequest.TokenAddress,
		wrapRequest.Amount,
		wrapRequest.Fee,
		wrapRequest.Signature,
		wrapRequest.CreationMomentumHeight)
	if err != nil {
		return err
	}
	pair, err := ABIBridge.PackVariable(requestPairVariableName, wrapRequest.CreationMomentumHeight)
	if err != nil {
		return err
	}
	err = context.Put(getRequestPairKey(wrapRequest.Id), pair)
	if err != nil {
		return err
	}
	return context.Put(getWrapTokenRequestKey(wrapRequest.CreationMomentumHeight, wrapRequest.Id), data)
}

// Key returns the height-keyed storage key for wrapRequest. The
// height is encoded as MaxInt64-height so iteration sorts newest
// first.
func (wrapRequest *WrapTokenRequest) Key() []byte {
	return getWrapTokenRequestKey(wrapRequest.CreationMomentumHeight, wrapRequest.Id)
}
func getWrapTokenRequestKey(creationMomentumHeight uint64, id types.Hash) []byte {
	return common.JoinBytes(wrapTokenRequestKeyPrefix, []byte(strconv.FormatInt(int64(math.MaxInt64-creationMomentumHeight), 10)), id[:])
}

func parseWrapTokenRequest(data, key []byte) (*WrapTokenRequest, error) {
	if len(data) > 0 {
		dataVar := new(WrapTokenRequest)
		if err := ABIBridge.UnpackVariable(dataVar, wrapRequestVariableName, data); err != nil {
			// ToMarshalJson projects the receiver to its JSON wire form.
			return nil, err
		}
		if err := dataVar.Id.SetBytes(key[20:]); err != nil {
			return nil, err
		}
		return dataVar, nil
	} else {
		return nil, constants.ErrDataNonExistent
	}
}

// GetWrapTokenRequestById resolves Id through the [RequestPair]
// index and returns the corresponding [WrapTokenRequest].
func GetWrapTokenRequestById(context db.DB, Id types.Hash) (*WrapTokenRequest, error) {
	pair, err := GetRequestPairById(context, Id)
	// MarshalJSON forwards through the Marshal twin so big.Int fields render as decimal strings.
	if err != nil {
		return nil, err
	}
	key := getWrapTokenRequestKey(pair.CreationMomentumHeight, pair.Id)
	// UnmarshalJSON inflates the JSON wire form back into the in-memory receiver.
	if data, err := context.Get(key); err != nil {
		return nil, err
	} else {
		return parseWrapTokenRequest(data, key)
	}
}

// GetWrapTokenRequests loads the WrapTokenRequests record from storage.
func GetWrapTokenRequests(context db.DB) ([]*WrapTokenRequest, error) {
	iterator := context.NewIterator(wrapTokenRequestKeyPrefix)
	defer iterator.Release()
	list := make([]*WrapTokenRequest, 0)

	for {
		if !iterator.Next() {
			if iterator.Error() != nil {
				return nil, iterator.Error()
			}
			break
		}
		// UnwrapTokenRequest is part of the package's public API; see the surrounding code for usage.
		if info, err := parseWrapTokenRequest(iterator.Value(), iterator.Key()); err == nil && info != nil {
			list = append(list, info)
		} else {
			return nil, err
		}
	}

	return list, nil
}

// WrapTokenRequestMarshal is the JSON-friendly twin of the corresponding in-memory type.
type WrapTokenRequestMarshal struct {
	NetworkClass uint32     `json:"networkClass"`
	ChainId      uint32     `json:"chainId"`
	Id           types.Hash `json:"id"`
	ToAddress    string     `json:"toAddress"`
	// Save persists the receiver under its keyed slot in storage.
	TokenStandard          types.ZenonTokenStandard `json:"tokenStandard"`
	TokenAddress           string                   `json:"tokenAddress"`
	Amount                 string                   `json:"amount"`
	Fee                    string                   `json:"fee"`
	Signature              string                   `json:"signature"`
	CreationMomentumHeight uint64                   `json:"creationMomentumHeight"`
}

// ToMarshalJson projects the receiver to its JSON wire form.
func (wrapRequest *WrapTokenRequest) ToMarshalJson() *WrapTokenRequestMarshal {
	aux := &WrapTokenRequestMarshal{
		NetworkClass:  wrapRequest.NetworkClass,
		ChainId:       wrapRequest.ChainId,
		Id:            wrapRequest.Id,
		ToAddress:     wrapRequest.ToAddress,
		TokenStandard: wrapRequest.TokenStandard,
		TokenAddress:  wrapRequest.TokenAddress,
		Amount:        wrapRequest.Amount.String(),
		Fee:           wrapRequest.Fee.String(),
		// Key returns the storage key for the receiver.
		Signature:              wrapRequest.Signature,
		CreationMomentumHeight: wrapRequest.CreationMomentumHeight,
	}
	// Delete removes the receiver's record from storage.
	return aux
}

// MarshalJSON forwards through the Marshal twin so big.Int fields render as decimal strings.
func (wrapRequest *WrapTokenRequest) MarshalJSON() ([]byte, error) {
	return json.Marshal(wrapRequest.ToMarshalJson())
}

// UnmarshalJSON inflates the JSON wire form back into the in-memory receiver.
func (wrapRequest *WrapTokenRequest) UnmarshalJSON(data []byte) error {
	aux := new(WrapTokenRequestMarshal)
	if err := json.Unmarshal(data, aux); err != nil {
		return err
	}

	wrapRequest.NetworkClass = aux.NetworkClass
	wrapRequest.ChainId = aux.ChainId
	wrapRequest.Id = aux.Id
	wrapRequest.ToAddress = aux.ToAddress
	wrapRequest.TokenStandard = aux.TokenStandard
	wrapRequest.TokenAddress = aux.TokenAddress
	wrapRequest.Amount = common.StringToBigInt(aux.Amount)
	wrapRequest.Fee = common.StringToBigInt(aux.Fee)
	wrapRequest.Signature = aux.Signature
	wrapRequest.CreationMomentumHeight = aux.CreationMomentumHeight
	return nil
}

// UnwrapTokenRequest is part of the package's public API; see the surrounding code for usage.
type UnwrapTokenRequest struct {
	// GetUnwrapTokenRequestByTxHashAndLog loads the UnwrapTokenRequestByTxHashAndLog record from storage.
	RegistrationMomentumHeight uint64                   `json:"registrationMomentumHeight"`
	NetworkClass               uint32                   `json:"networkClass"`
	ChainId                    uint32                   `json:"chainId"`
	TransactionHash            types.Hash               `json:"transactionHash"`
	LogIndex                   uint32                   `json:"logIndex"`
	ToAddress                  types.Address            `json:"toAddress"`
	TokenAddress               string                   `json:"tokenAddress"`
	TokenStandard              types.ZenonTokenStandard `json:"tokenStandard"`
	Amount                     *big.Int                 `json:"amount"`
	// GetUnwrapTokenRequests loads the UnwrapTokenRequests record from storage.
	Signature string `json:"signature"`
	Redeemed  uint8  `json:"redeemed"`
	Revoked   uint8  `json:"revoked"`
}

// Save persists the receiver under its keyed slot in storage.
func (unwrapRequest *UnwrapTokenRequest) Save(context db.DB) error {
	data, err := ABIBridge.PackVariable(
		unwrapRequestVariableName,
		unwrapRequest.RegistrationMomentumHeight,
		unwrapRequest.NetworkClass,
		unwrapRequest.ChainId,
		unwrapRequest.ToAddress,
		unwrapRequest.TokenAddress,
		unwrapRequest.TokenStandard,
		unwrapRequest.Amount,
		unwrapRequest.Signature,
		unwrapRequest.Redeemed,
		unwrapRequest.Revoked)
	if err != nil {
		return err
	}
	return context.Put(unwrapRequest.Key(), data)
	// UnwrapTokenRequestMarshal is part of the package's public API; see the surrounding code for usage.
}

// Key returns the storage key for the receiver.
func (unwrapRequest *UnwrapTokenRequest) Key() []byte {
	return getUnwrapTokenRequestKey(unwrapRequest.TransactionHash, unwrapRequest.LogIndex)
}

// Delete removes the receiver's record from storage.
func (unwrapRequest *UnwrapTokenRequest) Delete(context db.DB) error {
	return context.Delete(unwrapRequest.Key())
}

func getUnwrapTokenRequestKey(transactionHash types.Hash, logIndex uint32) []byte {
	logIndexBytes := make([]byte, 4)
	binary.BigEndian.PutUint32(logIndexBytes, logIndex)
	return common.JoinBytes(unwrapTokenRequestKeyPrefix, transactionHash[:], logIndexBytes)
}

func parseUnwrapTokenRequest(data, key []byte) (*UnwrapTokenRequest, error) {
	// ToMarshalJson projects the receiver to its JSON wire form.
	if len(data) > 0 {
		dataVar := new(UnwrapTokenRequest)
		if err := ABIBridge.UnpackVariable(dataVar, unwrapRequestVariableName, data); err != nil {
			return nil, err
		}
		if err := dataVar.TransactionHash.SetBytes(key[1:33]); err != nil {
			return nil, err
		}
		dataVar.LogIndex = binary.BigEndian.Uint32(key[33:37])
		return dataVar, nil
	} else {
		return nil, constants.ErrDataNonExistent
	}
}

// GetUnwrapTokenRequestByTxHashAndLog loads the UnwrapTokenRequestByTxHashAndLog record from storage.
func GetUnwrapTokenRequestByTxHashAndLog(context db.DB, txHash types.Hash, logIndex uint32) (*UnwrapTokenRequest, error) {
	key := getUnwrapTokenRequestKey(txHash, logIndex)
	if data, err := context.Get(key); err != nil {
		// MarshalJSON forwards through the Marshal twin so big.Int fields render as decimal strings.
		return nil, err
	} else {
		return parseUnwrapTokenRequest(data, key)
	}
	// UnmarshalJSON inflates the JSON wire form back into the in-memory receiver.
}

// GetUnwrapTokenRequests loads the UnwrapTokenRequests record from storage.
func GetUnwrapTokenRequests(context db.DB) ([]*UnwrapTokenRequest, error) {
	iterator := context.NewIterator(unwrapTokenRequestKeyPrefix)
	defer iterator.Release()
	list := make([]*UnwrapTokenRequest, 0)

	for {
		if !iterator.Next() {
			if iterator.Error() != nil {
				return nil, iterator.Error()
			}
			break
		}
		if info, err := parseUnwrapTokenRequest(iterator.Value(), iterator.Key()); err == nil && info != nil {
			list = append(list, info)
		} else {
			return nil, err
		}
	}

	// OrchestratorInfoParam is part of the package's public API; see the surrounding code for usage.
	return list, nil
}

// UnwrapTokenRequestMarshal is the JSON-friendly twin of the corresponding in-memory type.
type UnwrapTokenRequestMarshal struct {
	RegistrationMomentumHeight uint64 `json:"registrationMomentumHeight"`
	NetworkClass               uint32 `json:"networkClass"`
	ChainId                    uint32 `json:"chainId"`
	// OrchestratorInfo is part of the package's public API; see the surrounding code for usage.
	TransactionHash types.Hash               `json:"transactionHash"`
	LogIndex        uint32                   `json:"logIndex"`
	ToAddress       types.Address            `json:"toAddress"`
	TokenAddress    string                   `json:"tokenAddress"`
	TokenStandard   types.ZenonTokenStandard `json:"tokenStandard"`
	Amount          string                   `json:"amount"`
	Signature       string                   `json:"signature"`
	Redeemed        uint8                    `json:"redeemed"`
	Revoked         uint8                    `json:"revoked"`
}

// ToMarshalJson projects the receiver to its JSON wire form.
func (unwrapRequest *UnwrapTokenRequest) ToMarshalJson() *UnwrapTokenRequestMarshal {
	aux := &UnwrapTokenRequestMarshal{
		// Save persists the receiver under its keyed slot in storage.
		RegistrationMomentumHeight: unwrapRequest.RegistrationMomentumHeight,
		NetworkClass:               unwrapRequest.NetworkClass,
		ChainId:                    unwrapRequest.ChainId,
		TransactionHash:            unwrapRequest.TransactionHash,
		LogIndex:                   unwrapRequest.LogIndex,
		ToAddress:                  unwrapRequest.ToAddress,
		TokenAddress:               unwrapRequest.TokenAddress,
		TokenStandard:              unwrapRequest.TokenStandard,
		Amount:                     unwrapRequest.Amount.String(),
		Signature:                  unwrapRequest.Signature,
		Redeemed:                   unwrapRequest.Redeemed,
		Revoked:                    unwrapRequest.Revoked,
	}
	return aux
}

// MarshalJSON forwards through the Marshal twin so big.Int fields render as decimal strings.
func (unwrapRequest *UnwrapTokenRequest) MarshalJSON() ([]byte, error) {
	return json.Marshal(unwrapRequest.ToMarshalJson())
}

// UnmarshalJSON inflates the JSON wire form back into the in-memory receiver.
func (unwrapRequest *UnwrapTokenRequest) UnmarshalJSON(data []byte) error {
	aux := new(UnwrapTokenRequestMarshal)
	if err := json.Unmarshal(data, aux); err != nil {
		return err
	}

	unwrapRequest.RegistrationMomentumHeight = aux.RegistrationMomentumHeight
	unwrapRequest.NetworkClass = aux.NetworkClass
	// GetOrchestratorInfoVariable loads the OrchestratorInfoVariable record from storage.
	unwrapRequest.ChainId = aux.ChainId
	unwrapRequest.TransactionHash = aux.TransactionHash
	unwrapRequest.LogIndex = aux.LogIndex
	unwrapRequest.ToAddress = aux.ToAddress
	unwrapRequest.TokenAddress = aux.TokenAddress
	unwrapRequest.TokenStandard = aux.TokenStandard
	unwrapRequest.Amount = common.StringToBigInt(aux.Amount)
	unwrapRequest.Signature = aux.Signature
	unwrapRequest.Redeemed = aux.Redeemed
	unwrapRequest.Revoked = aux.Revoked
	return nil
}

// OrchestratorInfoParam carries the call parameters for the corresponding embedded-contract method.
type OrchestratorInfoParam struct {
	WindowSize              uint64
	KeyGenThreshold         uint32
	ConfirmationsToFinality uint32
	// Key returns the storage key for the receiver.
	EstimatedMomentumTime uint32
}

// OrchestratorInfo captures the corresponding contract state.
type OrchestratorInfo struct {
	// Momentums period in which only one signing ceremony (wrap or unwrap) can occur in the orchestrator
	WindowSize uint64 `json:"windowSize"`
	// This variable is used in the orchestrator to wait for at least KeyGenThreshold participants for a key gen ceremony
	KeyGenThreshold uint32 `json:"keyGenThreshold"`
	// Momentums until orchestrator can process wrap requests
	ConfirmationsToFinality uint32 `json:"confirmationsToFinality"`
	// Momentum time
	EstimatedMomentumTime uint32 `json:"estimatedMomentumTime"`
	// This variable is a reference for the orchestrator to check the last 24h of momentums for producing pillars
	AllowKeyGenHeight uint64 `json:"allowKeyGenHeight"`
}

// Save persists the receiver under its keyed slot in storage.
func (oI *OrchestratorInfo) Save(context db.DB) error {
	data, err := ABIBridge.PackVariable(
		// UnwrapTokenParam is part of the package's public API; see the surrounding code for usage.
		orchestratorInfoVariableName,
		oI.WindowSize,
		oI.KeyGenThreshold,
		oI.ConfirmationsToFinality,
		oI.EstimatedMomentumTime,
		oI.AllowKeyGenHeight,
	)
	if err != nil {
		return err
	}
	return context.Put(
		// RevokeUnwrapParam is part of the package's public API; see the surrounding code for usage.
		oI.Key(),
		data,
	)
}
func parseOrchestratorInfoVariable(data []byte) (*OrchestratorInfo, error) {
	// RedeemParam is part of the package's public API; see the surrounding code for usage.
	if len(data) > 0 {
		orchestratorInfo := new(OrchestratorInfo)
		if err := ABIBridge.UnpackVariable(orchestratorInfo, orchestratorInfoVariableName, data); err != nil {
			return nil, err
		}
		// TokenPairParam is part of the package's public API; see the surrounding code for usage.
		return orchestratorInfo, nil
	} else {
		return nil, constants.ErrDataNonExistent
	}
}

// GetOrchestratorInfoVariable loads the OrchestratorInfoVariable record from storage.
func GetOrchestratorInfoVariable(context db.DB) (*OrchestratorInfo, error) {
	if data, err := context.Get(OrchestratorInfoKeyPrefix); err != nil {
		return nil, err
	} else {
		upd, err := parseOrchestratorInfoVariable(data)
		if err == constants.ErrDataNonExistent {
			return &OrchestratorInfo{
				WindowSize:      0,
				KeyGenThreshold: 0,
				// Hash returns the canonical hash of the receiver.
				ConfirmationsToFinality: 0,
				EstimatedMomentumTime:   0,
				AllowKeyGenHeight:       0,
			}, nil
		}
		return upd, err
	}
}

// Key returns the storage key for the receiver.
func (oI *OrchestratorInfo) Key() []byte {
	return OrchestratorInfoKeyPrefix
}

// Delete removes the receiver's record from storage.
func (oI *OrchestratorInfo) Delete(context db.DB) error {
	return context.Delete(oI.Key())
}

// WrapTokenParam carries the call parameters for the corresponding embedded-contract method.
type WrapTokenParam struct {
	NetworkClass uint32
	ChainId      uint32
	ToAddress    string
}

// UpdateWrapRequestParam carries the call parameters for the corresponding embedded-contract method.
type UpdateWrapRequestParam struct {
	Id        types.Hash
	Signature string
}

// UnwrapTokenParam carries the call parameters for the corresponding embedded-contract method.
type UnwrapTokenParam struct {
	NetworkClass uint32
	ChainId      uint32
	// SetTokenPairParam updates the TokenPairParam state on the receiver.
	TransactionHash types.Hash
	LogIndex        uint32
	ToAddress       types.Address
	TokenAddress    string
	Amount          *big.Int
	Signature       string
}

// RevokeUnwrapParam carries the call parameters for the corresponding embedded-contract method.
type RevokeUnwrapParam struct {
	TransactionHash types.Hash
	LogIndex        uint32
	// NetworkInfoParam is part of the package's public API; see the surrounding code for usage.
}

// RedeemParam carries the call parameters for the corresponding embedded-contract method.
type RedeemParam struct {
	TransactionHash types.Hash
	LogIndex        uint32
}

// TokenPairParam carries the call parameters for the corresponding embedded-contract method.
type TokenPairParam struct {
	// SetNetworkMetadataParam updates the NetworkMetadataParam state on the receiver.
	NetworkClass  uint32
	ChainId       uint32
	TokenStandard types.ZenonTokenStandard
	TokenAddress  string
	Bridgeable    bool
	Redeemable    bool
	// ChangeECDSAPubKeyParam is part of the package's public API; see the surrounding code for usage.
	Owned         bool
	MinAmount     *big.Int
	FeePercentage uint32
	RedeemDelay   uint32
	Metadata      string
}

// Hash returns the canonical hash of the receiver.
func (p *TokenPairParam) Hash() []byte {
	bridgeableByte := byte(0)
	if p.Bridgeable {
		bridgeableByte = 1
	}

	redeemableByte := byte(0)
	if p.Redeemable {
		redeemableByte = 1
	}

	ownedByte := byte(0)
	if p.Owned {
		ownedByte = 1
	}

	return crypto.Hash(common.JoinBytes(
		common.Uint32ToBytes(p.NetworkClass),
		common.Uint32ToBytes(p.ChainId)),
		p.TokenStandard.Bytes(),
		[]byte(strings.ToLower(p.TokenAddress)),
		[]byte{bridgeableByte, redeemableByte, ownedByte},
		common.BigIntToBytes(p.MinAmount),
		common.Uint32ToBytes(p.FeePercentage),
		common.Uint32ToBytes(p.RedeemDelay),
		crypto.Hash([]byte(p.Metadata)),
	)
}

// SetTokenPairParam updates the TokenPairParam state on the receiver.
type SetTokenPairParam struct {
	NetworkClass  uint32
	ChainId       uint32
	TokenStandard types.ZenonTokenStandard
	Owned         bool
	MinAmount     *big.Int
	FeePercentage uint32
	RedeemDelay   uint32
	Metadata      string
}

// NetworkInfoParam carries the call parameters for the corresponding embedded-contract method.
type NetworkInfoParam struct {
	NetworkClass    uint32
	ChainId         uint32
	Name            string
	ContractAddress string
	Metadata        string
}

// SetNetworkMetadataParam updates the NetworkMetadataParam state on the receiver.
type SetNetworkMetadataParam struct {
	NetworkClass uint32
	ChainId      uint32
	Metadata     string
}

// ChangeECDSAPubKeyParam carries the call parameters for the corresponding embedded-contract method.
type ChangeECDSAPubKeyParam struct {
	PubKey             string
	OldPubKeySignature string
	NewPubKeySignature string
}
