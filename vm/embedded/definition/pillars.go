package definition

import (
	"encoding/binary"
	"encoding/json"
	"math/big"
	"strings"

	"github.com/pkg/errors"

	"github.com/zenon-network/go-zenon/common"
	"github.com/zenon-network/go-zenon/common/db"
	"github.com/zenon-network/go-zenon/common/types"
	"github.com/zenon-network/go-zenon/vm/abi"
	"github.com/zenon-network/go-zenon/vm/constants"
)

// jsonPillars is the canonical Solidity-shaped ABI for the Pillar
// contract: registration (Register, RegisterLegacy), updates
// (UpdatePillar, Update), revocation (Revoke), delegation
// (Delegate, Undelegate), QSR deposit/withdraw, reward collection,
// plus the storage record shapes (pillarInfo, producingPillarName,
// LegacyPillarEntry, delegationInfo, pillarEpochHistory).
const (
	jsonPillars = `
	[
{"type":"function","name":"Update", "inputs":[]},
		{"type":"function","name":"Register", "inputs":[
			{"name":"name","type":"string"},
			{"name":"producerAddress","type":"address"},
			{"name":"rewardAddress","type":"address"},
			{"name":"giveBlockRewardPercentage","type":"uint8"},
			{"name":"giveDelegateRewardPercentage","type":"uint8"}
		]},
		{"type":"function","name":"RegisterLegacy", "inputs":[
			{"name":"name","type":"string"},
			{"name":"producerAddress","type":"address"},
			{"name":"rewardAddress","type":"address"},
			{"name":"giveBlockRewardPercentage","type":"uint8"},
			{"name":"giveDelegateRewardPercentage","type":"uint8"}, 
			{"name":"publicKey", "type":"string"}, 
			{"name":"signature","type":"string"}
		]},
		{"type":"function","name":"UpdatePillar", "inputs":[
			{"name":"name","type":"string"},
			{"name":"producerAddress","type":"address"},
			{"name":"rewardAddress","type":"address"},
			{"name":"giveBlockRewardPercentage","type":"uint8"},
			{"name":"giveDelegateRewardPercentage","type":"uint8"}
		]},
		{"type":"function","name":"DepositQsr", "inputs":[]},
		{"type":"function","name":"WithdrawQsr", "inputs":[]},
		{"type":"function","name":"Revoke","inputs":[{"name":"name","type":"string"}]},
		{"type":"function","name":"Delegate", "inputs":[{"name":"name","type":"string"}]},
		{"type":"function","name":"Undelegate","inputs":[]},
		{"type":"function","name":"CollectReward","inputs":[]},

		{"type":"variable","name":"pillarInfo","inputs":[
			{"name":"name","type":"string"},
			{"name":"blockProducingAddress","type":"address"},
			{"name":"rewardWithdrawAddress","type":"address"},
			{"name":"stakeAddress","type":"address"},
			{"name":"amount","type":"uint256"},
			{"name":"registrationTime","type":"int64"},
			{"name":"revokeTime","type":"int64"},
			{"name":"giveBlockRewardPercentage","type":"uint8"},
			{"name":"giveDelegateRewardPercentage","type":"uint8"},
			{"name":"pillarType","type":"uint8"}
		]},
		{"type":"variable","name":"producingPillarName","inputs":[
			{"name":"name","type":"string"}
		]},
		{"type":"variable","name":"LegacyPillarEntry","inputs":[
			{"name":"pillarCount", "type":"uint8"}
		]},
		{"type":"variable","name":"delegationInfo","inputs":[
			{"name":"name","type":"string"}
		]},
		{"type":"variable","name":"pillarEpochHistory","inputs":[
			{"name":"giveBlockRewardPercentage","type":"uint8"},
			{"name":"giveDelegateRewardPercentage","type":"uint8"},
			{"name":"producedBlockNum","type":"int32"},
			{"name":"expectedBlockNum","type":"int32"},
			{"name":"weight","type":"uint256"}
		]}
	]`

	// RegisterMethodName names the standard pillar-registration
	// method.
	RegisterMethodName = "Register"
	// LegacyRegisterMethodName names the legacy-claim-backed
	// registration method (consumes a legacy-pillar slot via a
	// secp256k1 signature).
	LegacyRegisterMethodName = "RegisterLegacy"

	// UpdatePillarMethodName names the pillar-metadata-update method.
	UpdatePillarMethodName = "UpdatePillar"
	// RevokeMethodName names the pillar-revocation method.
	RevokeMethodName = "Revoke"
	// DelegateMethodName names the delegate-to-pillar method.
	DelegateMethodName = "Delegate"
	// UndelegateMethodName names the un-delegate method.
	UndelegateMethodName = "Undelegate"

	pillarInfoVariableName          = "pillarInfo"
	producingPillarNameVariableName = "producingPillarName"
	legacyPillarEntryVariableName   = "LegacyPillarEntry"
	delegationInfoVariableName      = "delegationInfo"
	pillarEpochHistoryVariableName  = "pillarEpochHistory"
)

// ABIPillars is the parsed [abi.ABIContract] for the Pillar
// contract. The per-prefix key namespaces follow:
// 1=pillarInfo, 2=producingPillarName (reverse index by producer),
// 3=LegacyPillarEntry, 4=delegationInfo, 5=pillarEpochHistory.
//
// Pillar-type discriminators tag each pillar with how it was
// registered ([LegacyPillarType] consumed a legacy slot;
// [NormalPillarType] paid the QSR-burn price). [AnyPillarType] is
// the wildcard for queries.
var (
	// ABIPillars is abi definition of pillar contract.
	ABIPillars = abi.JSONToABIContract(strings.NewReader(jsonPillars))

	pillarInfoKeyPrefix          = []byte{1}
	producingPillarNameKeyPrefix = []byte{2}
	legacyPillarEntryKeyPrefix   = []byte{3}
	delegationInfoKeyPrefix      = []byte{4}
	pillarEpochHistoryKeyPrefix  = []byte{5}

	// AnyPillarType is the wildcard used by queries that should
	// match either pillar kind.
	AnyPillarType = uint8(0)
	// LegacyPillarType marks pillars registered via
	// [LegacyRegisterMethodName] (consumed a legacy claim slot).
	LegacyPillarType = uint8(1)
	// NormalPillarType marks pillars registered via the standard
	// [RegisterMethodName] (paid the QSR-burn price).
	NormalPillarType = uint8(2)
)

// RegisterParam is the call-shape for [RegisterMethodName] — the
// pillar's display name, its producing-key (block authoring)
// address, the reward-withdrawal address, and the two split
// percentages (block reward vs delegation reward).
type RegisterParam struct {
	Name                         string
	ProducerAddress              types.Address
	RewardAddress                types.Address
	GiveBlockRewardPercentage    uint8
	GiveDelegateRewardPercentage uint8
}

// LegacyRegisterParam is the call-shape for
// [LegacyRegisterMethodName] — adds the legacy-chain public key and
// secp256k1 signature proving the legacy holder's authority.
type LegacyRegisterParam struct {
	RegisterParam
	PublicKey string
	Signature string
}

// PillarInfo is the on-chain registration of one pillar:
// identifying name, the producing-key address used to sign
// momentums, the address rewards withdraw to, the staking
// (registration) address, the locked ZNN amount, the
// registration / revoke timestamps, the reward split
// percentages, and the [LegacyPillarType] / [NormalPillarType]
// discriminator.
type PillarInfo struct {
	Name                         string
	BlockProducingAddress        types.Address
	RewardWithdrawAddress        types.Address
	StakeAddress                 types.Address
	Amount                       *big.Int
	RegistrationTime             int64
	RevokeTime                   int64
	GiveBlockRewardPercentage    uint8
	GiveDelegateRewardPercentage uint8
	PillarType                   uint8
}

// IsActive reports whether the pillar is still operational
// (i.e., RevokeTime has not been set).
func (pillar *PillarInfo) IsActive() bool {
	return pillar.RevokeTime == 0
}

// Save persists the receiver under its keyed slot in storage.
func (pillar *PillarInfo) Save(context db.DB) error {
	data, err := ABIPillars.PackVariable(
		pillarInfoVariableName,
		pillar.Name,
		pillar.BlockProducingAddress,
		pillar.RewardWithdrawAddress,
		pillar.StakeAddress,
		pillar.Amount,
		pillar.RegistrationTime,
		pillar.RevokeTime,
		pillar.GiveBlockRewardPercentage,
		pillar.GiveDelegateRewardPercentage,
		pillar.PillarType,
	)
	if err != nil {
		return err
	}
	return context.Put(GetPillarInfoKey(pillar.Name), data)
}

// GetPillarInfoKey builds the storage key for the [PillarInfo]
// entry of the pillar with the given display name:
// pillarInfoKeyPrefix || hash(name).
func GetPillarInfoKey(name string) []byte {
	return common.JoinBytes(pillarInfoKeyPrefix, types.NewHash([]byte(name)).Bytes())
}
func parsePillarInfo(data []byte) (*PillarInfo, error) {
	if len(data) > 0 {
		pillar := new(PillarInfo)
		if err := ABIPillars.UnpackVariable(pillar, pillarInfoVariableName, data); err != nil {
			return nil, err
		}
		return pillar, nil
	} else {
		return nil, constants.ErrDataNonExistent
	}
}

// GetPillarInfo loads the PillarInfo record from storage.
func GetPillarInfo(context db.DB, name string) (*PillarInfo, error) {
	key := GetPillarInfoKey(name)
	if data, err := context.Get(key); err != nil {
		return nil, err
	} else {
		return parsePillarInfo(data)
	}
}

// GetPillarsList iterates the [PillarInfo] prefix range and returns
// every registered pillar matching the filters: onlyActive limits
// to entries with no RevokeTime; pillarType filters by
// [LegacyPillarType] / [NormalPillarType] (use [AnyPillarType] to
// match either). Decode failures abort the iteration with the
// underlying error.
func GetPillarsList(context db.DB, onlyActive bool, pillarType uint8) ([]*PillarInfo, error) {
	iterator := context.NewIterator(pillarInfoKeyPrefix)
	defer iterator.Release()
	list := make([]*PillarInfo, 0)
	for {
		if !iterator.Next() {
			if iterator.Error() != nil {
				return nil, iterator.Error()
			}
			break
		}

		if pillar, err := parsePillarInfo(iterator.Value()); err == nil {
			if (!onlyActive || pillar.RevokeTime == 0) && (pillarType == AnyPillarType || pillarType == pillar.PillarType) {
				list = append(list, pillar)
			}
		} else if err == constants.ErrDataNonExistent {
			continue
		} else {
			return nil, err
		}
	}
	return list, nil
}

// ProducingPillar is the reverse-index record from
// producing-key address to pillar name. Lets the consensus layer
// resolve a momentum's signer to a registered pillar in O(1).
type ProducingPillar struct {
	Producing *types.Address
	Name      string
}

// Save persists the receiver under its keyed slot in storage.
func (ppName *ProducingPillar) Save(context db.DB) error {
	data, err := ABIPillars.PackVariable(
		producingPillarNameVariableName,
		ppName.Name,
	)
	if err != nil {
		return err
	}
	return context.Put(GetProducingPillarKey(*ppName.Producing), data)
}

// GetProducingPillarKey builds the storage key for the
// [ProducingPillar] reverse-index entry keyed by the producing
// address: producingPillarNameKeyPrefix || producing.Bytes().
func GetProducingPillarKey(producing types.Address) []byte {
	return common.JoinBytes(producingPillarNameKeyPrefix, producing.Bytes())
}
func isProducingPillarKey(key []byte) bool {
	return key[0] == producingPillarNameKeyPrefix[0]
}
func unmarshalProducingPillarKey(key []byte) (*types.Address, error) {
	if !isProducingPillarKey(key) {
		return nil, errors.Errorf("invalid key! Not producing pillar key")
	}
	addr := new(types.Address)
	if err := addr.SetBytes(key[1:]); err != nil {
		return nil, err
	}
	return addr, nil
}
func parseProducingPillar(key []byte, data []byte) (*ProducingPillar, error) {
	if len(data) > 0 {
		entry := new(ProducingPillar)
		if err := ABIPillars.UnpackVariable(entry, producingPillarNameVariableName, data); err != nil {
			return nil, err
		}

		producing, err := unmarshalProducingPillarKey(key)
		if err != nil {
			return nil, err
		}
		entry.Producing = producing
		return entry, nil
	} else {
		return nil, constants.ErrDataNonExistent
	}
}

// GetProducingPillarName loads the ProducingPillarName record from storage.
func GetProducingPillarName(context db.DB, address types.Address) (*ProducingPillar, error) {
	key := GetProducingPillarKey(address)
	if data, err := context.Get(key); err != nil {
		return nil, err
	} else {
		return parseProducingPillar(key, data)
	}
}

// DelegationInfo is one delegation record: a backer address
// directing its weight to a named pillar. One record per backer
// (delegations are exclusive — a backer cannot split across
// pillars).
type DelegationInfo struct {
	Backer types.Address
	Name   string
}

// Save persists the receiver under its keyed slot in storage.
func (delegation *DelegationInfo) Save(context db.DB) error {
	data, err := ABIPillars.PackVariable(
		delegationInfoVariableName,
		delegation.Name,
	)
	if err != nil {
		return err
	}
	return context.Put(getDelegationInfoKey(delegation.Backer), data)
}

// Delete removes the receiver's record from storage.
func (delegation *DelegationInfo) Delete(context db.DB) error {
	return context.Delete(getDelegationInfoKey(delegation.Backer))
}

func getDelegationInfoKey(addr types.Address) []byte {
	return common.JoinBytes(delegationInfoKeyPrefix, addr.Bytes())
}
func isDelegationInfoKey(key []byte) bool {
	return key[0] == delegationInfoKeyPrefix[0]
}
func unmarshalDelegationInfo(key []byte) (*types.Address, error) {
	if !isDelegationInfoKey(key) {
		return nil, errors.Errorf("invalid key! Not delegation info key")
	}
	addr := new(types.Address)
	if err := addr.SetBytes(key[1:]); err != nil {
		return nil, err
	}
	return addr, nil
}
func parseDelegationInfo(key, data []byte) (*DelegationInfo, error) {
	if len(data) > 0 {
		entry := new(DelegationInfo)
		if err := ABIPillars.UnpackVariable(entry, delegationInfoVariableName, data); err != nil {
			return nil, err
		}

		address, err := unmarshalDelegationInfo(key)
		if err != nil {
			return nil, err
		}
		entry.Backer = *address
		return entry, nil
	} else {
		return nil, constants.ErrDataNonExistent
	}
}

// GetDelegationInfo loads the DelegationInfo record from storage.
func GetDelegationInfo(context db.DB, address types.Address) (*DelegationInfo, error) {
	key := getDelegationInfoKey(address)
	if data, err := context.Get(key); err != nil {
		return nil, err
	} else {
		return parseDelegationInfo(key, data)
	}
}

// GetDelegationsList iterates the [DelegationInfo] prefix range
// and returns every recorded delegation. Each backer contributes
// at most one entry (delegations are exclusive). Decode failures
// abort the iteration with the underlying error.
func GetDelegationsList(context db.DB) ([]*DelegationInfo, error) {
	iterator := context.NewIterator(delegationInfoKeyPrefix)
	defer iterator.Release()
	list := make([]*DelegationInfo, 0)
	for {
		if !iterator.Next() {
			if iterator.Error() != nil {
				return nil, iterator.Error()
			}
			break
		}

		if delegationInfo, err := parseDelegationInfo(iterator.Key(), iterator.Value()); err == nil {
			list = append(list, delegationInfo)
		} else if err == constants.ErrDataNonExistent {
			continue
		} else {
			return nil, err
		}
	}
	return list, nil
}

// LegacyPillarEntry is the on-chain claim of one legacy holder's
// pillar slot pool. PillarCount is decremented every time a
// legacy registration consumes a slot; the entry is deleted when
// it reaches zero.
type LegacyPillarEntry struct {
	PillarCount uint8      `json:"pillarCount"`
	KeyIdHash   types.Hash `json:"keyIdHash"`
}

// Save persists the receiver under its keyed slot in storage.
func (legacy *LegacyPillarEntry) Save(context db.DB) error {
	data, err := ABIPillars.PackVariable(
		legacyPillarEntryVariableName,
		legacy.PillarCount)
	if err != nil {
		return err
	}
	return context.Put(getLegacyPillarEntryKey(legacy.KeyIdHash), data)
}

// Delete removes the receiver's record from storage.
func (legacy *LegacyPillarEntry) Delete(context db.DB) error {
	return context.Delete(getLegacyPillarEntryKey(legacy.KeyIdHash))
}

func getLegacyPillarEntryKey(keyIdHash types.Hash) []byte {
	return common.JoinBytes(legacyPillarEntryKeyPrefix, keyIdHash[:])
}
func isLegacyPillarEntryKey(key []byte) bool {
	return key[0] == legacyPillarEntryKeyPrefix[0]
}
func unmarshalLegacyPillarEntryKey(key []byte) (*types.Hash, error) {
	if !isLegacyPillarEntryKey(key) {
		return nil, errors.Errorf("invalid key! Not legacy pillar key")
	}
	h := new(types.Hash)
	if err := h.SetBytes(key[1:]); err != nil {
		return nil, err
	}
	return h, nil
}
func parseLegacyPillarEntry(key, data []byte) (*LegacyPillarEntry, error) {
	if len(data) > 0 {
		dataVar := new(LegacyPillarEntry)
		if err := ABIPillars.UnpackVariable(dataVar, legacyPillarEntryVariableName, data); err != nil {
			return nil, err
		}
		if keyIdHash, err := unmarshalLegacyPillarEntryKey(key); err == nil {
			dataVar.KeyIdHash = *keyIdHash
		} else {
			return nil, err
		}
		return dataVar, nil
	} else {
		return nil, constants.ErrDataNonExistent
	}
}

// GetLegacyPillarEntry loads the LegacyPillarEntry record from storage.
func GetLegacyPillarEntry(context db.DB, keyIdHash types.Hash) (*LegacyPillarEntry, error) {
	key := getLegacyPillarEntryKey(keyIdHash)
	if data, err := context.Get(key); err != nil {
		return nil, err
	} else {
		return parseLegacyPillarEntry(key, data)
	}
}

// GetLegacyPillarList iterates the [LegacyPillarEntry] prefix
// range and returns every legacy-claim entry that still has slots
// remaining. Empty value entries are skipped; decode failures
// abort the iteration with the underlying error.
func GetLegacyPillarList(context db.DB) ([]*LegacyPillarEntry, error) {
	iterator := context.NewIterator(legacyPillarEntryKeyPrefix)
	defer iterator.Release()
	list := make([]*LegacyPillarEntry, 0)

	for {
		if !iterator.Next() {
			if iterator.Error() != nil {
				return nil, iterator.Error()
			}
			break
		}
		if len(iterator.Value()) == 0 {
			continue
		}
		if entry, err := parseLegacyPillarEntry(iterator.Key(), iterator.Value()); err == nil && entry != nil {
			list = append(list, entry)
		} else {
			return nil, err
		}
	}

	return list, nil
}

// PillarEpochHistory is the per-(pillar, epoch) historical
// record: the reward-split percentages active at the time of the
// epoch, the produced/expected block counts, and the weighted
// stake. Persisted at reward-distribution time so that
// after-the-fact rewards calculations remain stable even when
// pillar metadata changes later.
type PillarEpochHistory struct {
	Name                         string   `json:"name"`
	Epoch                        uint64   `json:"epoch"`
	GiveBlockRewardPercentage    uint8    `json:"giveBlockRewardPercentage"`
	GiveDelegateRewardPercentage uint8    `json:"giveDelegateRewardPercentage"`
	ProducedBlockNum             int32    `json:"producedBlockNum"`
	ExpectedBlockNum             int32    `json:"expectedBlockNum"`
	Weight                       *big.Int `json:"weight"`
}

// Save persists the receiver under its keyed slot in storage.
func (peh *PillarEpochHistory) Save(context db.DB) error {
	data, err := ABIPillars.PackVariable(
		pillarEpochHistoryVariableName,
		peh.GiveBlockRewardPercentage,
		peh.GiveDelegateRewardPercentage,
		peh.ProducedBlockNum,
		peh.ExpectedBlockNum,
		peh.Weight)
	if err != nil {
		return err
	}
	return context.Put(getPillarEpochHistoryEntryKey(peh.Epoch, peh.Name), data)
}

func getPillarEpochHistoryPrefixKey(epoch uint64) []byte {
	epochBytes := make([]byte, 8)
	binary.LittleEndian.PutUint64(epochBytes, epoch)
	return common.JoinBytes(pillarEpochHistoryKeyPrefix, epochBytes)
}
func getPillarEpochHistoryEntryKey(epoch uint64, name string) []byte {
	return common.JoinBytes(getPillarEpochHistoryPrefixKey(epoch), []byte(name))
}
func isPillarEpochHistoryEntryKey(key []byte) bool {
	return key[0] == pillarEpochHistoryKeyPrefix[0]
}
func unmarshalPillarEpochHistoryEntryKey(key []byte) (uint64, string, error) {
	if !isPillarEpochHistoryEntryKey(key) {
		return 0, "", errors.Errorf("invalid key! Not PillarEpochHistory key")
	}
	epoch := binary.LittleEndian.Uint64(key[1:9])
	name := string(key[9:])
	return epoch, name, nil
}
func parsePillarEpochHistoryEntry(key, data []byte) (*PillarEpochHistory, error) {
	if len(data) > 0 {
		entry := new(PillarEpochHistory)
		if err := ABIPillars.UnpackVariable(entry, pillarEpochHistoryVariableName, data); err != nil {
			return nil, err
		}
		if epoch, name, err := unmarshalPillarEpochHistoryEntryKey(key); err == nil {
			entry.Epoch = epoch
			entry.Name = name
		} else {
			return nil, err
		}
		return entry, nil
	} else {
		return nil, constants.ErrDataNonExistent
	}
}

// GetPillarEpochHistoryList iterates the per-epoch
// [PillarEpochHistory] prefix range and returns every entry
// recorded for the given epoch. Decode failures abort the
// iteration with the underlying error.
func GetPillarEpochHistoryList(context db.DB, epoch uint64) ([]*PillarEpochHistory, error) {
	iterator := context.NewIterator(getPillarEpochHistoryPrefixKey(epoch))
	defer iterator.Release()
	list := make([]*PillarEpochHistory, 0)

	for {
		if !iterator.Next() {
			if iterator.Error() != nil {
				return nil, iterator.Error()
			}
			break
		}
		if entry, err := parsePillarEpochHistoryEntry(iterator.Key(), iterator.Value()); err == nil && entry != nil {
			list = append(list, entry)
		} else {
			return nil, err
		}
	}

	return list, nil
}

// PillarEpochHistoryMarshal is the JSON-friendly twin of
// [PillarEpochHistory] (Weight as a string so it round-trips
// through clients without big-integer support).
type PillarEpochHistoryMarshal struct {
	Name                         string `json:"name"`
	Epoch                        uint64 `json:"epoch"`
	GiveBlockRewardPercentage    uint8  `json:"giveBlockRewardPercentage"`
	GiveDelegateRewardPercentage uint8  `json:"giveDelegateRewardPercentage"`
	ProducedBlockNum             int32  `json:"producedBlockNum"`
	ExpectedBlockNum             int32  `json:"expectedBlockNum"`
	Weight                       string `json:"weight"`
}

// ToPillarEpochHistoryMarshal projects the receiver to its JSON-friendly PillarEpochHistoryMarshal twin.
func (g *PillarEpochHistory) ToPillarEpochHistoryMarshal() *PillarEpochHistoryMarshal {
	aux := &PillarEpochHistoryMarshal{
		Name:                         g.Name,
		Epoch:                        g.Epoch,
		GiveBlockRewardPercentage:    g.GiveBlockRewardPercentage,
		GiveDelegateRewardPercentage: g.GiveDelegateRewardPercentage,
		ProducedBlockNum:             g.ProducedBlockNum,
		ExpectedBlockNum:             g.ExpectedBlockNum,
		Weight:                       g.Weight.String(),
	}
	return aux
}

// MarshalJSON forwards through the Marshal twin so big.Int fields render as decimal strings.
func (g *PillarEpochHistory) MarshalJSON() ([]byte, error) {
	return json.Marshal(g.ToPillarEpochHistoryMarshal())
}

// UnmarshalJSON inflates the JSON wire form back into the in-memory receiver.
func (g *PillarEpochHistory) UnmarshalJSON(data []byte) error {
	aux := new(PillarEpochHistoryMarshal)
	if err := json.Unmarshal(data, aux); err != nil {
		return err
	}
	g.Name = aux.Name
	g.Epoch = aux.Epoch
	g.GiveBlockRewardPercentage = aux.GiveBlockRewardPercentage
	g.GiveDelegateRewardPercentage = aux.GiveDelegateRewardPercentage
	g.ProducedBlockNum = aux.ProducedBlockNum
	g.ExpectedBlockNum = aux.ExpectedBlockNum
	g.Weight = common.StringToBigInt(aux.Weight)
	return nil
}
