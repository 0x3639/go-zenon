package definition

import (
	"encoding/json"
	"github.com/pkg/errors"
	"github.com/zenon-network/go-zenon/common"
	"github.com/zenon-network/go-zenon/common/db"
	"github.com/zenon-network/go-zenon/common/types"
	"github.com/zenon-network/go-zenon/vm/constants"
	"math/big"
	"strings"

	"github.com/zenon-network/go-zenon/vm/abi"
)

// jsonLiquidity is the canonical Solidity-shaped ABI for the
// Liquidity contract: liquidity-program funding (Fund, BurnZnn),
// administered configuration (SetTokenTuple, SetAdditionalReward,
// SetIsHalted), governance (ChangeAdministrator,
// ProposeAdministrator, NominateGuardians, Emergency), staking
// (LiquidityStake, CancelLiquidityStake, UnlockLiquidityStakeEntries),
// and the periodic Update / Donate / CollectReward methods.
//
// Storage records: liquidityInfo (singleton), tokenTuple
// (per-token configuration), liquidityStakeEntry (per-stake),
// securityInfo (guardian / delay configuration).
const (
	jsonLiquidity = `
	[
		{"type":"function","name":"Update", "inputs":[]},
		{"type":"function","name":"Donate", "inputs":[]},
		{"type":"function","name":"Fund", "inputs":[
			{"name":"znnReward","type":"uint256"},
			{"name":"qsrReward","type":"uint256"}
		]},
		{"type":"function","name":"BurnZnn", "inputs":[
			{"name":"burnAmount","type":"uint256"}
		]},
		{"type":"function","name":"SetTokenTuple", "inputs":[
			{"name":"tokenStandards","type":"string[]"},
			{"name":"znnPercentages","type":"uint32[]"},
			{"name":"qsrPercentages","type":"uint32[]"},
			{"name":"minAmounts","type":"uint256[]"}
		]},
		{"type":"variable","name":"liquidityInfo","inputs":[
			{"name":"administrator","type":"address"},
			{"name":"isHalted","type":"bool"},
			{"name":"znnReward","type":"uint256"},
			{"name":"qsrReward","type":"uint256"},
			{"name":"tokenTuples","type":"bytes[]"}
		]},
		{"type":"variable","name":"tokenTuple","inputs":[
			{"name":"tokenStandard","type":"string"},
			{"name":"znnPercentage","type":"uint32"},
			{"name":"qsrPercentage","type":"uint32"},
			{"name":"minAmount","type":"uint256"}
		]},
		{"type":"variable", "name":"liquidityStakeEntry", "inputs":[
			{"name":"amount", "type":"uint256"},
			{"name":"tokenStandard", "type":"tokenStandard"},
			{"name":"weightedAmount", "type":"uint256"},
			{"name":"startTime", "type":"int64"},
			{"name":"revokeTime", "type":"int64"},
			{"name":"expirationTime", "type":"int64"}
		]},
		{"type":"function","name":"NominateGuardians","inputs":[
			{"name":"guardians","type":"address[]"}
		]},
		{"type":"function","name":"ProposeAdministrator","inputs":[
			{"name":"address","type":"address"}
		]},
		{"type":"function","name":"Emergency","inputs":[]},

		{"type":"variable","name":"securityInfo","inputs":[
			{"name":"guardians","type":"address[]"},
			{"name":"guardiansVotes","type":"address[]"},
			{"name":"administratorDelay","type":"uint64"},
			{"name":"softDelay","type":"uint64"}
		]},
		{"type":"function","name":"SetIsHalted","inputs":[
			{"name":"isHalted","type":"bool"}
		]},
		{"type":"function","name":"LiquidityStake","inputs":[
			{"name":"durationInSec", "type":"int64"}
		]},
		{"type":"function","name":"CancelLiquidityStake","inputs":[
			{"name":"id","type":"hash"}
		]},
		{"type":"function","name":"UnlockLiquidityStakeEntries","inputs":[]},
		{"type":"function","name":"SetAdditionalReward","inputs":[
			{"name":"znnReward", "type":"uint256"},
			{"name":"qsrReward", "type":"uint256"}
		]},
		{"type":"function","name":"CollectReward","inputs":[]},
		{"type":"function","name":"ChangeAdministrator","inputs":[
			{"name":"administrator","type":"address"}
		]}
	]`

	// FundMethodName credits the liquidity reward pool from a
	// caller-supplied transfer.
	FundMethodName = "Fund"
	// BurnZnnMethodName burns a portion of the liquidity reward
	// pool via the Token contract.
	BurnZnnMethodName = "BurnZnn"
	// SetTokenTupleMethodName configures the per-token reward
	// shares (administrator-only).
	SetTokenTupleMethodName = "SetTokenTuple"
	// LiquidityStakeMethodName locks tokens for liquidity rewards.
	LiquidityStakeMethodName = "LiquidityStake"
	// CancelLiquidityStakeMethodName begins unlocking a stake
	// (refund follows after the lock window).
	CancelLiquidityStakeMethodName = "CancelLiquidityStake"
	// UnlockLiquidityStakeEntriesMethodName completes the unlock
	// flow once the lock window has elapsed.
	UnlockLiquidityStakeEntriesMethodName = "UnlockLiquidityStakeEntries"
	// SetAdditionalRewardMethodName credits extra rewards into
	// the pool (administrator-only).
	SetAdditionalRewardMethodName = "SetAdditionalReward"
	// SetIsHaltedMethodName toggles the halted flag (no further
	// staking when halted).
	SetIsHaltedMethodName = "SetIsHalted"

	liquidityInfoVariableName       = "liquidityInfo"
	tokenTupleVariableName          = "tokenTuple"
	liquidityStakeEntryVariableName = "liquidityStakeEntry"
)

// ABILiquidity is the parsed [abi.ABIContract] for the Liquidity
// contract. 1=liquidityInfo (singleton config),
// 2=liquidityStakeEntry (per-stake records).
var (
	ABILiquidity = abi.JSONToABIContract(strings.NewReader(jsonLiquidity))

	LiquidityInfoKeyPrefix       = []byte{1}
	LiquidityStakeEntryKeyPrefix = []byte{2}
)

// LiquidityInfoVariable is the on-disk storage shape of the
// liquidity-info singleton: administrator, halted flag,
// accumulated rewards, and the byte-encoded token-tuple list.
// LiquidityInfo (defined just below) is the runtime-decoded
// twin with TokenTuples expanded to typed values.
type LiquidityInfoVariable struct {
	Administrator types.Address `json:"administrator"`
	IsHalted      bool          `json:"isHalted"`
	ZnnReward     *big.Int      `json:"znnReward"`
	QsrReward     *big.Int      `json:"qsrReward"`
	TokenTuples   [][]byte      `json:"tokenTuples"`
}

// LiquidityInfo is part of the package's public API; see the surrounding code for usage.
type LiquidityInfo struct {
	Administrator types.Address `json:"administrator"`
	IsHalted      bool          `json:"isHalted"`
	ZnnReward     *big.Int      `json:"znnReward"`
	QsrReward     *big.Int      `json:"qsrReward"`
	TokenTuples   []TokenTuple  `json:"tokenTuples"`
}

// LiquidityInfoMarshal is part of the package's public API; see the surrounding code for usage.
type LiquidityInfoMarshal struct {
	Administrator types.Address `json:"administrator"`
	IsHalted      bool          `json:"isHalted"`
	ZnnReward     string        `json:"znnReward"`
	QsrReward     string        `json:"qsrReward"`
	TokenTuples   []TokenTuple  `json:"tokenTuples"`
}

// ToLiquidityInfoMarshal projects the receiver to its JSON-friendly LiquidityInfoMarshal twin.
func (l *LiquidityInfo) ToLiquidityInfoMarshal() LiquidityInfoMarshal {
	aux := LiquidityInfoMarshal{
		Administrator: l.Administrator,
		IsHalted:      l.IsHalted,
		ZnnReward:     l.ZnnReward.String(),
		QsrReward:     l.QsrReward.String(),
	}

	aux.TokenTuples = make([]TokenTuple, len(l.TokenTuples))
	for idx, tuple := range l.TokenTuples {
		aux.TokenTuples[idx] = tuple
	}

	return aux
}

// MarshalJSON forwards through the Marshal twin so big.Int fields render as decimal strings.
func (l *LiquidityInfo) MarshalJSON() ([]byte, error) {
	return json.Marshal(l.ToLiquidityInfoMarshal())
}

// UnmarshalJSON inflates the JSON wire form back into the in-memory receiver.
func (l *LiquidityInfo) UnmarshalJSON(data []byte) error {
	aux := new(LiquidityInfoMarshal)
	if err := json.Unmarshal(data, aux); err != nil {
		return err
	}
	l.Administrator = aux.Administrator
	l.IsHalted = aux.IsHalted
	l.ZnnReward = common.StringToBigInt(aux.ZnnReward)
	l.QsrReward = common.StringToBigInt(aux.QsrReward)
	l.TokenTuples = make([]TokenTuple, len(aux.TokenTuples))
	for idx, tuple := range aux.TokenTuples {
		l.TokenTuples[idx] = tuple
	}
	return nil
}

// Save persists the receiver under its keyed slot in storage.
func (liq *LiquidityInfoVariable) Save(context db.DB) error {
	data, err := ABILiquidity.PackVariable(
		liquidityInfoVariableName,
		liq.Administrator,
		liq.IsHalted,
		liq.ZnnReward,
		liq.QsrReward,
		liq.TokenTuples,
	)
	if err != nil {
		return err
	}
	return context.Put(
		LiquidityInfoKeyPrefix,
		data,
	)
}
func parseLiquidityInfo(data []byte) (*LiquidityInfo, error) {
	if len(data) > 0 {
		liquidityInfoVariable := new(LiquidityInfoVariable)
		if err := ABILiquidity.UnpackVariable(liquidityInfoVariable, liquidityInfoVariableName, data); err != nil {
			return nil, err
		}
		tokenTuples := make([]TokenTuple, 0)
		for _, token := range liquidityInfoVariable.TokenTuples {
			tokenTuple := new(TokenTuple)
			if err := ABILiquidity.UnpackVariable(tokenTuple, tokenTupleVariableName, token); err != nil {
				continue
			}
			tokenTuples = append(tokenTuples, *tokenTuple)
		}
		liquidityInfo := &LiquidityInfo{
			Administrator: liquidityInfoVariable.Administrator,
			TokenTuples:   tokenTuples,
			IsHalted:      liquidityInfoVariable.IsHalted,
			ZnnReward:     liquidityInfoVariable.ZnnReward,
			QsrReward:     liquidityInfoVariable.QsrReward,
		}
		return liquidityInfo, nil
	} else {
		return &LiquidityInfo{
			Administrator: constants.InitialBridgeAdministrator,
			TokenTuples:   nil,
			IsHalted:      false,
			ZnnReward:     common.Big0,
			QsrReward:     common.Big0,
		}, nil
	}
}

// GetLiquidityInfo loads the LiquidityInfo record from storage.
func GetLiquidityInfo(context db.DB) (*LiquidityInfo, error) {
	if data, err := context.Get(LiquidityInfoKeyPrefix); err != nil {
		return nil, err
	} else {
		upd, err := parseLiquidityInfo(data)
		return upd, err
	}
}

// EncodeLiquidityInfo serialises the input to the package's wire encoding.
func EncodeLiquidityInfo(liquidityInfo *LiquidityInfo) (*LiquidityInfoVariable, error) {
	liquidityInfoVariable := new(LiquidityInfoVariable)
	if err := liquidityInfoVariable.Administrator.SetBytes(liquidityInfo.Administrator.Bytes()); err != nil {
		return nil, err
	}
	liquidityInfoVariable.IsHalted = liquidityInfo.IsHalted
	liquidityInfoVariable.ZnnReward = liquidityInfo.ZnnReward
	liquidityInfoVariable.QsrReward = liquidityInfo.QsrReward
	tokenTuples := make([][]byte, 0)
	for _, token := range liquidityInfo.TokenTuples {
		if tokenTuple, err := ABILiquidity.PackVariable(tokenTupleVariableName, token.TokenStandard, token.ZnnPercentage, token.QsrPercentage, token.MinAmount); err != nil {
			return nil, err
		} else {
			tokenTuples = append(tokenTuples, tokenTuple)
		}
	}
	liquidityInfoVariable.TokenTuples = tokenTuples
	return liquidityInfoVariable, nil
}

// TokenTuple is part of the package's public API; see the surrounding code for usage.
type TokenTuple struct {
	TokenStandard string   `json:"tokenStandard"`
	ZnnPercentage uint32   `json:"znnPercentage"`
	QsrPercentage uint32   `json:"qsrPercentage"`
	MinAmount     *big.Int `json:"minAmount"`
}

// TokenTupleMarshal projects the receiver to its JSON-friendly kenTupleMarshal twin.
type TokenTupleMarshal struct {
	TokenStandard string `json:"tokenStandard"`
	ZnnPercentage uint32 `json:"znnPercentage"`
	QsrPercentage uint32 `json:"qsrPercentage"`
	MinAmount     string `json:"minAmount"`
}

// ToTokenTupleMarshal projects the receiver to its JSON-friendly TokenTupleMarshal twin.
func (s *TokenTuple) ToTokenTupleMarshal() *TokenTupleMarshal {
	aux := &TokenTupleMarshal{
		TokenStandard: s.TokenStandard,
		ZnnPercentage: s.ZnnPercentage,
		QsrPercentage: s.QsrPercentage,
		MinAmount:     s.MinAmount.String(),
	}
	return aux
}

// MarshalJSON forwards through the Marshal twin so big.Int fields render as decimal strings.
func (s *TokenTuple) MarshalJSON() ([]byte, error) {
	return json.Marshal(s.ToTokenTupleMarshal())
}

// UnmarshalJSON inflates the JSON wire form back into the in-memory receiver.
func (s *TokenTuple) UnmarshalJSON(data []byte) error {
	aux := new(TokenTupleMarshal)
	if err := json.Unmarshal(data, aux); err != nil {
		return err
	}
	s.TokenStandard = aux.TokenStandard
	s.ZnnPercentage = aux.ZnnPercentage
	s.QsrPercentage = aux.QsrPercentage
	s.MinAmount = common.StringToBigInt(aux.MinAmount)

	return nil
}

// FundParam is part of the package's public API; see the surrounding code for usage.
type FundParam struct {
	ZnnReward *big.Int
	QsrReward *big.Int
}

// BurnParam is part of the package's public API; see the surrounding code for usage.
type BurnParam struct {
	BurnAmount *big.Int
}

// TokenTuplesParam is part of the package's public API; see the surrounding code for usage.
type TokenTuplesParam struct {
	TokenStandards []string
	ZnnPercentages []uint32
	QsrPercentages []uint32
	MinAmounts     []*big.Int
}

// SetAdditionalRewardParam updates the AdditionalRewardParam state on the receiver.
type SetAdditionalRewardParam struct {
	ZnnReward *big.Int
	QsrReward *big.Int
}

// LiquidityStakeEntry is part of the package's public API; see the surrounding code for usage.
type LiquidityStakeEntry struct {
	Amount         *big.Int                 `json:"amount"`
	TokenStandard  types.ZenonTokenStandard `json:"tokenStandard"`
	WeightedAmount *big.Int                 `json:"weightedAmount"`
	StartTime      int64                    `json:"startTime"`
	RevokeTime     int64                    `json:"revokeTime"`
	ExpirationTime int64                    `json:"expirationTime"`
	StakeAddress   types.Address            `json:"stakeAddress"`
	Id             types.Hash               `json:"id"`
}

// Save persists the receiver under its keyed slot in storage.
func (stake *LiquidityStakeEntry) Save(context db.DB) error {
	return context.Put(
		getLiquidityStakeEntryKey(stake.Id, stake.StakeAddress),
		ABILiquidity.PackVariablePanic(
			liquidityStakeEntryVariableName,
			stake.Amount,
			stake.TokenStandard,
			stake.WeightedAmount,
			stake.StartTime,
			stake.RevokeTime,
			stake.ExpirationTime,
		))
}

// Delete removes the receiver's record from storage.
func (stake *LiquidityStakeEntry) Delete(context db.DB) error {
	return context.Delete(getLiquidityStakeEntryKey(stake.Id, stake.StakeAddress))
}

func getLiquidityStakeEntryKey(id types.Hash, address types.Address) []byte {
	return append(append(LiquidityStakeEntryKeyPrefix, address.Bytes()...), id.Bytes()...)
}
func isLiquidityStakeEntryKey(key []byte) bool {
	return key[0] == LiquidityStakeEntryKeyPrefix[0]
}
func unmarshalLiquidityStakeEntryKey(key []byte) (*types.Hash, *types.Address, error) {
	if !isLiquidityStakeEntryKey(key) {
		return nil, nil, errors.Errorf("invalid key! Not liquidity stake entry key")
	}
	h := new(types.Hash)
	err := h.SetBytes(key[1+types.AddressSize:])
	if err != nil {
		return nil, nil, err
	}

	addr := new(types.Address)
	err = addr.SetBytes(key[1 : 1+types.AddressSize])
	if err != nil {
		return nil, nil, err
	}

	return h, addr, nil
}
func parseLiquidityStakeEntry(key []byte, data []byte) (*LiquidityStakeEntry, error) {
	if len(data) > 0 {
		entry := new(LiquidityStakeEntry)
		err := ABILiquidity.UnpackVariable(entry, liquidityStakeEntryVariableName, data)
		if err != nil {
			return nil, err
		}

		id, address, err := unmarshalLiquidityStakeEntryKey(key)
		if err != nil {
			return nil, err
		}
		entry.Id = *id
		entry.StakeAddress = *address
		return entry, err
	} else {
		return nil, constants.ErrDataNonExistent
	}
}

// GetLiquidityStakeEntry loads the LiquidityStakeEntry record from storage.
func GetLiquidityStakeEntry(context db.DB, id types.Hash, address types.Address) (*LiquidityStakeEntry, error) {
	key := getLiquidityStakeEntryKey(id, address)
	if data, err := context.Get(key); err != nil {
		return nil, err
	} else {
		return parseLiquidityStakeEntry(key, data)
	}
}

// IterateLiquidityStakeEntries is part of the package's public API.
func IterateLiquidityStakeEntries(context db.DB, f func(entry *LiquidityStakeEntry) error) error {
	iterator := context.NewIterator(LiquidityStakeEntryKeyPrefix)
	defer iterator.Release()

	for {
		if !iterator.Next() {
			if iterator.Error() != nil {
				return iterator.Error()
			}
			break
		}

		if stakeEntry, err := parseLiquidityStakeEntry(iterator.Key(), iterator.Value()); err == nil {
			if err := f(stakeEntry); err != nil {
				return err
			}
		} else if err == constants.ErrDataNonExistent {
		} else {
			return err
		}
	}
	return nil
}

// LiquidityStakeEntryMarshal is the JSON/RPC view of a liquidity-stake entry.
type LiquidityStakeEntryMarshal struct {
	Amount         string                   `json:"amount"`
	TokenStandard  types.ZenonTokenStandard `json:"tokenStandard"`
	WeightedAmount string                   `json:"weightedAmount"`
	StartTime      int64                    `json:"startTime"`
	RevokeTime     int64                    `json:"revokeTime"`
	ExpirationTime int64                    `json:"expirationTime"`
	StakeAddress   types.Address            `json:"stakeAddress"`
	Id             types.Hash               `json:"id"`
}

// ToLiquidityStakeEntry converts the storage record into its JSON/RPC view.
func (stake *LiquidityStakeEntry) ToLiquidityStakeEntry() *LiquidityStakeEntryMarshal {
	aux := &LiquidityStakeEntryMarshal{
		Amount:         stake.Amount.String(),
		TokenStandard:  stake.TokenStandard,
		WeightedAmount: stake.WeightedAmount.String(),
		StartTime:      stake.StartTime,
		RevokeTime:     stake.RevokeTime,
		ExpirationTime: stake.ExpirationTime,
		StakeAddress:   stake.StakeAddress,
		Id:             stake.Id,
	}
	return aux
}

// MarshalJSON forwards through the Marshal twin so big.Int fields render as decimal strings.
func (stake *LiquidityStakeEntry) MarshalJSON() ([]byte, error) {
	return json.Marshal(stake.ToLiquidityStakeEntry())
}

// UnmarshalJSON inflates the JSON wire form back into the in-memory receiver.
func (stake *LiquidityStakeEntry) UnmarshalJSON(data []byte) error {
	aux := new(LiquidityStakeEntryMarshal)
	if err := json.Unmarshal(data, aux); err != nil {
		return err
	}
	stake.Amount = common.StringToBigInt(aux.Amount)
	stake.TokenStandard = aux.TokenStandard
	stake.WeightedAmount = common.StringToBigInt(aux.WeightedAmount)
	stake.StartTime = aux.StartTime
	stake.RevokeTime = aux.RevokeTime
	stake.ExpirationTime = aux.ExpirationTime
	stake.StakeAddress = aux.StakeAddress
	stake.Id = aux.Id
	return nil
}

// GetLiquidityStakeListByAddress loads the LiquidityStakeListByAddress record from storage.
func GetLiquidityStakeListByAddress(context db.DB, address types.Address) ([]*LiquidityStakeEntry, *big.Int, *big.Int, error) {
	total := big.NewInt(0)
	weighted := big.NewInt(0)
	list := make([]*LiquidityStakeEntry, 0)

	err := IterateLiquidityStakeEntries(context, func(stakeEntry *LiquidityStakeEntry) error {
		if stakeEntry.RevokeTime == 0 && stakeEntry.StakeAddress == address {
			list = append(list, stakeEntry)
			total.Add(total, stakeEntry.Amount)
			weighted.Add(weighted, stakeEntry.WeightedAmount)
		}
		return nil
	})
	if err != nil {
		return nil, nil, nil, err
	} else {
		return list, total, weighted, nil
	}
}

// GetAllLiquidityStakeEntries loads the AllLiquidityStakeEntries record from storage.
func GetAllLiquidityStakeEntries(context db.DB) []*LiquidityStakeEntry {
	iterator := context.NewIterator(LiquidityStakeEntryKeyPrefix)
	defer iterator.Release()

	liquidityStakeEntries := make([]*LiquidityStakeEntry, 0)
	for {
		if !iterator.Next() {
			common.DealWithErr(iterator.Error())
			break
		}
		liquidityStakeEntry, err := parseLiquidityStakeEntry(iterator.Key(), iterator.Value())
		if err != nil {
			continue
		}
		liquidityStakeEntries = append(liquidityStakeEntries, liquidityStakeEntry)
	}
	return liquidityStakeEntries
}

// LiquidityStakeByExpirationTime is part of the package's public API; see the surrounding code for usage.
type LiquidityStakeByExpirationTime []*LiquidityStakeEntry

func (a LiquidityStakeByExpirationTime) Len() int      { return len(a) }
func (a LiquidityStakeByExpirationTime) Swap(i, j int) { a[i], a[j] = a[j], a[i] }
func (a LiquidityStakeByExpirationTime) Less(i, j int) bool {
	if a[i].ExpirationTime == a[j].ExpirationTime {
		return a[i].Id.String() < a[j].Id.String()
	}
	return a[i].ExpirationTime < a[j].ExpirationTime
}
