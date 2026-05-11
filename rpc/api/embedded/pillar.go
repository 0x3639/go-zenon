package embedded

import (
	"encoding/json"
	"math/big"
	"sort"

	"github.com/inconshreveable/log15"

	"github.com/zenon-network/go-zenon/chain"
	"github.com/zenon-network/go-zenon/common"
	"github.com/zenon-network/go-zenon/common/types"
	"github.com/zenon-network/go-zenon/rpc/api"
	"github.com/zenon-network/go-zenon/vm/constants"
	"github.com/zenon-network/go-zenon/vm/embedded/definition"
	"github.com/zenon-network/go-zenon/vm/embedded/implementation"
	"github.com/zenon-network/go-zenon/zenon"
)

// PillarApi serves read RPCs for the pillar contract — registered
// block producers, their delegation balances, and their per-epoch
// reward history.
type PillarApi struct {
	log            log15.Logger
	chain          chain.Chain
	consensusCache ConsensusCache
}

// NewPillarApi returns a PillarApi backed by a fresh ConsensusCache
// for z. The testing flag is forwarded verbatim to NewConsensusCache
// — see that constructor for the refresh-strategy difference
// between testing == true (synchronous) and testing == false
// (background, 5-minute cadence).
func NewPillarApi(z zenon.Zenon, testing bool) *PillarApi {
	return &PillarApi{
		log:            common.RPCLogger.New("module", "embedded_pillar_api"),
		chain:          z.Chain(),
		consensusCache: NewConsensusCache(z, testing),
	}
}

// PillarActive and PillarInActive are the two values reported in
// the NodeStatus field of GetDelegatedPillarResponse: 1 when the
// delegate's pillar has not been revoked (RevokeTime == 0), 2
// otherwise.
//
// They are declared as `var` rather than `const` for historical
// reasons; treat them as constants.
var (
	PillarActive   uint8 = 1
	PillarInActive uint8 = 2
)

// === Shared RPCs ===
//
// These forward to shared.go helpers scoped to the PillarContract
// address. Each is documented on SentinelApi too; the pillar
// variants are identical except for the target contract.

// GetDepositedQsr returns the QSR amount the address has deposited
// to the pillar contract, formatted as a decimal string.
func (a *PillarApi) GetDepositedQsr(address types.Address) (string, error) {
	depositedQsr, err := getDepositedQsr(a.chain, types.PillarContract, address)
	return depositedQsr.String(), err
}

// GetUncollectedReward returns the cumulative uncollected
// ZNN + QSR reward owed to address by the pillar contract, or
// (nil, nil) when no entry exists.
func (a *PillarApi) GetUncollectedReward(address types.Address) (*definition.RewardDeposit, error) {
	return getUncollectedReward(a.chain, types.PillarContract, address)
}

// GetFrontierRewardByPage walks epochs descending from the latest
// LastEpochUpdate and returns a paged window of per-epoch rewards
// for the address from the pillar contract.
func (a *PillarApi) GetFrontierRewardByPage(address types.Address, pageIndex, pageSize uint32) (*RewardHistoryList, error) {
	if pageSize > api.RpcMaxPageSize {
		return nil, api.ErrPageSizeParamTooBig
	}
	return getFrontierRewardByPage(a.chain, types.PillarContract, address, pageIndex, pageSize)
}

// GetQsrRegistrationCost returns the QSR amount required to
// register the next pillar, formatted as a decimal string. The
// cost is computed by implementation.GetQsrCostForNextPillar
// against the current frontier context, so it reflects any
// pillar-count-based adjustments active at the frontier momentum.
func (a *PillarApi) GetQsrRegistrationCost() (string, error) {
	_, context, err := api.GetFrontierContext(a.chain, types.PillarContract)
	if err != nil {
		return "", err
	}

	currentQsrCost, err := implementation.GetQsrCostForNextPillar(context)
	if err != nil {
		return "", err
	}

	return currentQsrCost.String(), nil
}

// PillarInfo is the RPC view of a single pillar entry, merging the
// chain-stored registration record (name, addresses, revocation
// status, reward percentages) with consensus-derived metrics
// (Weight, CurrentStats) populated from the ConsensusCache.
//
// CanBeRevoked and RevokeCooldown come from
// implementation.PillarGetRevokeStatus and reflect the time
// remaining before the stake owner may revoke the registration.
// RevokeTime mirrors the on-chain RevokeTime field; it is zero
// for active pillars.
type PillarInfo struct {
	Name string `json:"name"`
	Rank int    `json:"rank"`
	Type uint8  `json:"type"`

	StakeAddress          types.Address `json:"ownerAddress"`
	BlockProducingAddress types.Address `json:"producerAddress"`
	RewardWithdrawAddress types.Address `json:"withdrawAddress"`

	CanBeRevoked   bool  `json:"isRevocable"`
	RevokeCooldown int64 `json:"revokeCooldown"`
	RevokeTime     int64 `json:"revokeTimestamp"`

	GiveMomentumRewardPercentage uint8 `json:"giveMomentumRewardPercentage"`
	GiveDelegateRewardPercentage uint8 `json:"giveDelegateRewardPercentage"`

	CurrentStats *PillarStats `json:"currentStats"`
	Weight       *big.Int     `json:"weight"`
}

// PillarInfoMarshal mirrors PillarInfo with Weight encoded as a
// decimal string so the JSON wire format does not lose precision
// on large *big.Int weights. PillarInfo.MarshalJSON and
// UnmarshalJSON use this twin internally.
type PillarInfoMarshal struct {
	Name string `json:"name"`
	Rank int    `json:"rank"`
	Type uint8  `json:"type"`

	StakeAddress          types.Address `json:"ownerAddress"`
	BlockProducingAddress types.Address `json:"producerAddress"`
	RewardWithdrawAddress types.Address `json:"withdrawAddress"`

	CanBeRevoked   bool  `json:"isRevocable"`
	RevokeCooldown int64 `json:"revokeCooldown"`
	RevokeTime     int64 `json:"revokeTimestamp"`

	GiveMomentumRewardPercentage uint8 `json:"giveMomentumRewardPercentage"`
	GiveDelegateRewardPercentage uint8 `json:"giveDelegateRewardPercentage"`

	CurrentStats *PillarStats `json:"currentStats"`
	Weight       string       `json:"weight"`
}

// ToPillarInfoMarshal converts p into its string-Weight wire form.
func (p *PillarInfo) ToPillarInfoMarshal() *PillarInfoMarshal {
	aux := &PillarInfoMarshal{
		Name:                         p.Name,
		Rank:                         p.Rank,
		Type:                         p.Type,
		StakeAddress:                 p.StakeAddress,
		BlockProducingAddress:        p.BlockProducingAddress,
		RewardWithdrawAddress:        p.RewardWithdrawAddress,
		CanBeRevoked:                 p.CanBeRevoked,
		RevokeCooldown:               p.RevokeCooldown,
		RevokeTime:                   p.RevokeTime,
		GiveMomentumRewardPercentage: p.GiveMomentumRewardPercentage,
		GiveDelegateRewardPercentage: p.GiveDelegateRewardPercentage,
		CurrentStats:                 p.CurrentStats,
		Weight:                       p.Weight.String(),
	}

	return aux
}

// MarshalJSON renders p through PillarInfoMarshal so Weight is
// emitted as a decimal string.
func (p *PillarInfo) MarshalJSON() ([]byte, error) {
	return json.Marshal(p.ToPillarInfoMarshal())
}

// UnmarshalJSON reads a PillarInfoMarshal payload and rehydrates
// the *big.Int Weight via common.StringToBigInt.
func (p *PillarInfo) UnmarshalJSON(data []byte) error {
	aux := new(PillarInfoMarshal)
	if err := json.Unmarshal(data, aux); err != nil {
		return err
	}
	p.Name = aux.Name
	p.Rank = aux.Rank
	p.Type = aux.Type
	p.StakeAddress = aux.StakeAddress
	p.BlockProducingAddress = aux.BlockProducingAddress
	p.RewardWithdrawAddress = aux.RewardWithdrawAddress
	p.CanBeRevoked = aux.CanBeRevoked
	p.RevokeCooldown = aux.RevokeCooldown
	p.RevokeTime = aux.RevokeTime
	p.GiveMomentumRewardPercentage = aux.GiveMomentumRewardPercentage
	p.GiveDelegateRewardPercentage = aux.GiveDelegateRewardPercentage
	p.CurrentStats = aux.CurrentStats
	p.Weight = common.StringToBigInt(aux.Weight)
	return nil
}

// PillarInfoList is the paged response shape for pillar enumeration.
// Count is the full pre-paging total so clients can compute the
// global page count.
type PillarInfoList struct {
	Count uint32        `json:"count"`
	List  []*PillarInfo `json:"list"`
}

// PillarStats is the per-pillar performance pair surfaced inside
// PillarInfo.CurrentStats: the number of momentums the pillar
// actually produced this epoch and the number it was scheduled to
// produce.
type PillarStats struct {
	ProducedMomentums uint64 `json:"producedMomentums"`
	ExpectedMomentums uint64 `json:"expectedMomentums"`
}

// PillarInfoByWeight implements sort.Interface so a list of
// PillarInfo entries can be ordered by descending weight, with
// alphabetical Name as a tiebreaker for equal weights. GetAll uses
// this ordering before assigning Rank.
type PillarInfoByWeight []*PillarInfo

func (a PillarInfoByWeight) Len() int      { return len(a) }
func (a PillarInfoByWeight) Swap(i, j int) { a[i], a[j] = a[j], a[i] }

// Less reports whether the i'th entry sorts before the j'th. Equal
// weights resolve to ascending Name; otherwise the entry with the
// larger Weight comes first (descending weight order).
func (a PillarInfoByWeight) Less(i, j int) bool {
	r := a[j].Weight.Cmp(a[i].Weight)
	if r == 0 {
		return a[i].Name < a[j].Name
	} else {
		return r < 0
	}
}

// GetAll returns one page of every registered pillar, sorted by
// descending weight (alphabetical Name as tiebreaker) with Rank
// assigned post-sort. Weights and CurrentStats come from the
// ConsensusCache snapshot, so under production refresh policy
// they may lag the current epoch by up to five minutes; under
// testing == true they are recomputed on every call.
//
// pageSize > api.RpcMaxPageSize is rejected with
// api.ErrPageSizeParamTooBig.
func (a *PillarApi) GetAll(pageIndex, pageSize uint32) (*PillarInfoList, error) {
	if pageSize > api.RpcMaxPageSize {
		return nil, api.ErrPageSizeParamTooBig
	}

	m, context, err := api.GetFrontierContext(a.chain, types.PillarContract)
	if err != nil {
		return nil, err
	}

	// pillars
	candidateList, err := definition.GetPillarsList(context.Storage(), true, definition.AnyPillarType)
	if err != nil {
		return nil, err
	}

	targetList := make([]*PillarInfo, len(candidateList))

	for index, pillar := range candidateList {
		// canBeRevoked
		canBeRevoked, revokeCooldown := implementation.PillarGetRevokeStatus(pillar, m)

		targetList[index] = &PillarInfo{
			Name:                         pillar.Name,
			Type:                         pillar.PillarType,
			StakeAddress:                 pillar.StakeAddress,
			BlockProducingAddress:        pillar.BlockProducingAddress,
			RewardWithdrawAddress:        pillar.RewardWithdrawAddress,
			RevokeTime:                   pillar.RevokeTime,
			GiveMomentumRewardPercentage: pillar.GiveBlockRewardPercentage,
			GiveDelegateRewardPercentage: pillar.GiveDelegateRewardPercentage,
			CanBeRevoked:                 canBeRevoked,
			RevokeCooldown:               revokeCooldown,
			CurrentStats: &PillarStats{
				ProducedMomentums: 0,
				ExpectedMomentums: 0,
			},
			Weight: common.Big0,
		}
	}

	// feed information from rpc consensus cache
	weights, stats := a.consensusCache.Get()
	if weights != nil {
		for _, pillar := range targetList {
			weight, ok := weights[pillar.Name]
			if !ok {
				pillar.Weight = big.NewInt(0)
			} else {
				pillar.Weight = (&big.Int{}).Set(weight)
			}
		}
	}

	if stats != nil {
		for _, pillar := range targetList {
			pillarStat, ok := stats.Pillars[pillar.Name]
			if ok {
				pillar.CurrentStats.ProducedMomentums = pillarStat.BlockNum
				pillar.CurrentStats.ExpectedMomentums = pillarStat.ExceptedBlockNum
			}
		}
	}

	sort.Sort(PillarInfoByWeight(targetList))
	for i := range targetList {
		targetList[i].Rank = i
	}

	start, end := api.GetRange(pageIndex, pageSize, uint32(len(targetList)))

	return &PillarInfoList{
		Count: uint32(len(targetList)),
		List:  targetList[start:end],
	}, nil
}
// GetByOwner returns every pillar whose StakeAddress matches the
// supplied address. Implemented in terms of GetAll, so the same
// consensus-cache freshness and sort order apply. An empty slice
// is returned when the owner has no registered pillar.
func (a *PillarApi) GetByOwner(stakeAddress types.Address) ([]*PillarInfo, error) {
	list, err := a.GetAll(0, api.RpcMaxPageSize)
	if err != nil {
		return nil, err
	}
	targetList := make([]*PillarInfo, 0)
	for _, pillar := range list.List {
		if pillar.StakeAddress == stakeAddress {
			targetList = append(targetList, pillar)
		}
	}

	return targetList, nil
}
// GetByName returns the pillar with the matching Name, or
// (nil, nil) when no such pillar is registered. Implemented as a
// linear scan over GetAll's full result, so cost grows with the
// total pillar count.
func (a *PillarApi) GetByName(name string) (*PillarInfo, error) {
	list, err := a.GetAll(0, api.RpcMaxPageSize)
	if err != nil {
		return nil, err
	}
	for _, pillar := range list.List {
		if pillar.Name == name {
			return pillar, nil
		}
	}

	return nil, nil
}

// CheckNameAvailability reports whether name is free to use for a
// new pillar registration. Returns true when no registered pillar
// (including revoked ones — the underlying call passes false for
// the active-only filter) currently holds the name.
func (a *PillarApi) CheckNameAvailability(name string) (bool, error) {
	_, context, err := api.GetFrontierContext(a.chain, types.PillarContract)
	if err != nil {
		return false, err
	}

	// pillars
	pillars, err := definition.GetPillarsList(context.Storage(), false, definition.AnyPillarType)
	if err != nil {
		return false, err
	}

	for _, pillar := range pillars {
		if pillar.Name == name {
			return false, nil
		}
	}
	return true, nil
}

// GetDelegatedPillarResponse describes the pillar a particular
// address has delegated to: the pillar's name, its current
// PillarActive / PillarInActive status, and the delegator's ZNN
// balance (returned under the JSON key "weight" for historical
// reasons, even though it is a balance rather than a consensus
// weight).
type GetDelegatedPillarResponse struct {
	Name       string   `json:"name"`
	NodeStatus uint8    `json:"status"`
	Balance    *big.Int `json:"weight"`
}

// GetDelegatedPillarResponseMarshal mirrors GetDelegatedPillarResponse
// with Balance encoded as a decimal string for JSON safety.
type GetDelegatedPillarResponseMarshal struct {
	Name       string `json:"name"`
	NodeStatus uint8  `json:"status"`
	Balance    string `json:"weight"`
}

// ToGetDelegatedPillarResponse converts g into its
// string-Balance wire form. The method name predates the
// To<TypeName>Marshal convention used elsewhere in this package;
// it returns *GetDelegatedPillarResponseMarshal despite the name.
func (g *GetDelegatedPillarResponse) ToGetDelegatedPillarResponse() *GetDelegatedPillarResponseMarshal {
	aux := &GetDelegatedPillarResponseMarshal{
		Name:       g.Name,
		NodeStatus: g.NodeStatus,
		Balance:    g.Balance.String(),
	}
	return aux
}

// MarshalJSON renders g through GetDelegatedPillarResponseMarshal
// so Balance is emitted as a decimal string.
func (g *GetDelegatedPillarResponse) MarshalJSON() ([]byte, error) {
	return json.Marshal(g.ToGetDelegatedPillarResponse())
}

// UnmarshalJSON reads a GetDelegatedPillarResponseMarshal payload
// and rehydrates the *big.Int Balance via common.StringToBigInt.
func (g *GetDelegatedPillarResponse) UnmarshalJSON(data []byte) error {
	aux := new(GetDelegatedPillarResponseMarshal)
	if err := json.Unmarshal(data, aux); err != nil {
		return err
	}
	g.Name = aux.Name
	g.NodeStatus = aux.NodeStatus
	g.Balance = common.StringToBigInt(aux.Balance)
	return nil
}

// GetDelegatedPillar returns the pillar that addr currently
// delegates to, the delegator's ZNN balance at the frontier
// momentum, and a status code (PillarActive when the delegate's
// pillar still has RevokeTime == 0, PillarInActive otherwise).
//
// Returns (nil, nil) when addr has no recorded delegation
// (constants.ErrDataNonExistent from the storage read is mapped
// to nil so callers can branch on existence without inspecting
// an error). Other storage errors are propagated unchanged.
func (a *PillarApi) GetDelegatedPillar(addr types.Address) (*GetDelegatedPillarResponse, error) {
	_, context, err := api.GetFrontierContext(a.chain, types.PillarContract)
	if err != nil {
		return nil, err
	}
	delegationInfo, err := definition.GetDelegationInfo(context.Storage(), addr)
	if err == constants.ErrDataNonExistent {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if delegationInfo != nil {
		balance, err := a.chain.GetFrontierMomentumStore().GetAccountStore(addr).GetBalance(types.ZnnTokenStandard)
		if err != nil {
			return nil, err
		}
		status := PillarInActive
		if pillar, err := definition.GetPillarInfo(context.Storage(), delegationInfo.Name); err == constants.ErrDataNonExistent {
		} else if err == nil {
			if pillar.RevokeTime == 0 {
				status = PillarActive
			}
		} else {
			return nil, err
		}

		return &GetDelegatedPillarResponse{
			Name:       delegationInfo.Name,
			NodeStatus: status,
			Balance:    balance}, nil

	}
	return nil, nil
}

// PillarEpochHistoryList is the paged response shape for per-epoch
// pillar history. Count is the global epoch count (lastEpoch + 1)
// rather than the size of List, mirroring RewardHistoryList.
type PillarEpochHistoryList struct {
	Count int64                            `json:"count"`
	List  []*definition.PillarEpochHistory `json:"list"`
}

// GetPillarEpochHistory returns a paged window of per-epoch
// history for one pillar, walking epochs descending from the
// latest LastEpochUpdate. Epochs for which the pillar has no
// recorded entry are filled with a zero-valued PillarEpochHistory
// carrying just Name and Epoch — so List always has exactly the
// pageSize length (subject to the epoch-zero floor), making
// client-side rendering straightforward.
//
// pageSize > api.RpcMaxPageSize is rejected with
// api.ErrPageSizeParamTooBig.
func (a *PillarApi) GetPillarEpochHistory(pillarName string, pageIndex, pageSize uint32) (*PillarEpochHistoryList, error) {
	if pageSize > api.RpcMaxPageSize {
		return nil, api.ErrPageSizeParamTooBig
	}

	_, context, err := api.GetFrontierContext(a.chain, types.PillarContract)
	if err != nil {
		return nil, err
	}

	// get latest epoch
	lastEpoch, err := definition.GetLastEpochUpdate(context.Storage())
	if err != nil {
		return nil, err
	}

	epoch := lastEpoch.LastEpoch - int64(pageIndex*pageSize)

	result := &PillarEpochHistoryList{
		Count: lastEpoch.LastEpoch + 1,
		List:  make([]*definition.PillarEpochHistory, 0, pageSize),
	}
	for i := 0; i < int(pageSize); i += 1 {
		if epoch < 0 {
			break
		}
		if pillars, err := definition.GetPillarEpochHistoryList(context.Storage(), uint64(epoch)); err == nil {
			found := false
			for _, pillar := range pillars {
				if pillar.Name == pillarName {
					result.List = append(result.List, pillar)
					found = true
					break
				}
			}
			if !found {
				result.List = append(result.List, &definition.PillarEpochHistory{
					Name:                         pillarName,
					Epoch:                        uint64(epoch),
					GiveDelegateRewardPercentage: 0,
					GiveBlockRewardPercentage:    0,
					ProducedBlockNum:             0,
					ExpectedBlockNum:             0,
					Weight:                       common.Big0,
				})
			}
		} else {
			return nil, err
		}
		epoch -= 1
	}

	return result, err
}

// GetPillarsHistoryByEpoch returns a paged slice of every pillar's
// recorded history entry for the given epoch — the cross-section
// inverse of GetPillarEpochHistory. Count is the number of pillars
// with a recorded entry at that epoch (not the global pillar
// count). pageSize > api.RpcMaxPageSize is rejected with
// api.ErrPageSizeParamTooBig.
func (a *PillarApi) GetPillarsHistoryByEpoch(epoch uint64, pageIndex, pageSize uint32) (*PillarEpochHistoryList, error) {
	if pageSize > api.RpcMaxPageSize {
		return nil, api.ErrPageSizeParamTooBig
	}

	_, context, err := api.GetFrontierContext(a.chain, types.PillarContract)
	if err != nil {
		return nil, err
	}

	pillars, err := definition.GetPillarEpochHistoryList(context.Storage(), epoch)
	if err != nil {
		return nil, err
	}

	start, end := api.GetRange(pageIndex, pageSize, uint32(len(pillars)))

	return &PillarEpochHistoryList{
		Count: int64(len(pillars)),
		List:  pillars[start:end],
	}, nil
}
