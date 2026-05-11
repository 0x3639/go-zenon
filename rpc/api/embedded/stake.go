package embedded

import (
	"encoding/json"
	"math/big"
	"sort"

	"github.com/inconshreveable/log15"

	"github.com/zenon-network/go-zenon/chain"
	"github.com/zenon-network/go-zenon/common"
	"github.com/zenon-network/go-zenon/common/types"
	"github.com/zenon-network/go-zenon/consensus"
	"github.com/zenon-network/go-zenon/rpc/api"
	"github.com/zenon-network/go-zenon/vm/embedded/definition"
	"github.com/zenon-network/go-zenon/zenon"
)

// StakeApi serves read RPCs for the (ZNN) stake contract — direct
// stake entries that earn ZNN/QSR rewards over time.
type StakeApi struct {
	chain chain.Chain
	z     zenon.Zenon
	cs    consensus.Consensus
	log   log15.Logger
}

// NewStakeApi returns a StakeApi bound to z's chain and consensus.
// The cs and z fields are stored for symmetry with other handler
// constructors in this package; currently-exposed methods only
// use chain.
func NewStakeApi(z zenon.Zenon) *StakeApi {
	return &StakeApi{
		chain: z.Chain(),
		z:     z,
		cs:    z.Consensus(),
		log:   common.RPCLogger.New("module", "embedded_stake_api"),
	}
}

// === Shared RPCs ===
//
// Note that StakeApi has no GetDepositedQsr — staking uses ZNN,
// not deposited QSR, so the helper does not apply here.

// GetUncollectedReward returns the cumulative uncollected
// ZNN + QSR reward owed to address by the stake contract.
// The definition layer zero-fills the "no entry" case, so the
// result is never (nil, nil); a zero-valued *RewardDeposit
// (Znn = Qsr = 0) represents "nothing owed yet".
func (a *StakeApi) GetUncollectedReward(address types.Address) (*definition.RewardDeposit, error) {
	return getUncollectedReward(a.chain, types.StakeContract, address)
}

// GetFrontierRewardByPage walks epochs descending from the latest
// LastEpochUpdate and returns a paged window of per-epoch rewards
// for address from the stake contract.
func (a *StakeApi) GetFrontierRewardByPage(address types.Address, pageIndex, pageSize uint32) (*RewardHistoryList, error) {
	if pageSize > api.RpcMaxPageSize {
		return nil, api.ErrPageSizeParamTooBig
	}
	return getFrontierRewardByPage(a.chain, types.StakeContract, address, pageIndex, pageSize)
}

// StakeEntry is the RPC view of one stake record. WeightedAmount
// is Amount adjusted for lock duration per the stake contract's
// weighting curve. StartTimestamp and ExpirationTimestamp are Unix
// seconds. Id is the stake's deterministic chain identifier.
type StakeEntry struct {
	Amount              *big.Int      `json:"amount"`
	WeightedAmount      *big.Int      `json:"weightedAmount"`
	StartTimestamp      int64         `json:"startTimestamp"`
	ExpirationTimestamp int64         `json:"expirationTimestamp"`
	Address             types.Address `json:"address"`
	Id                  types.Hash    `json:"id"`
}

// StakeEntryMarshal mirrors StakeEntry with the *big.Int amount
// fields encoded as decimal strings for JSON precision safety.
type StakeEntryMarshal struct {
	Amount              string        `json:"amount"`
	WeightedAmount      string        `json:"weightedAmount"`
	StartTimestamp      int64         `json:"startTimestamp"`
	ExpirationTimestamp int64         `json:"expirationTimestamp"`
	Address             types.Address `json:"address"`
	Id                  types.Hash    `json:"id"`
}

// ToStakeEntryMarshal converts s into its string-amount wire form.
func (s *StakeEntry) ToStakeEntryMarshal() *StakeEntryMarshal {
	aux := &StakeEntryMarshal{
		Amount:              s.Amount.String(),
		WeightedAmount:      s.WeightedAmount.String(),
		StartTimestamp:      s.StartTimestamp,
		ExpirationTimestamp: s.ExpirationTimestamp,
		Address:             s.Address,
		Id:                  s.Id,
	}
	return aux
}

// MarshalJSON renders s through StakeEntryMarshal so amounts are
// emitted as decimal strings.
func (s *StakeEntry) MarshalJSON() ([]byte, error) {
	return json.Marshal(s.ToStakeEntryMarshal())
}

// UnmarshalJSON reads a StakeEntryMarshal payload and rehydrates
// the *big.Int amount fields via common.StringToBigInt.
func (s *StakeEntry) UnmarshalJSON(data []byte) error {
	aux := new(StakeEntryMarshal)
	if err := json.Unmarshal(data, aux); err != nil {
		return err
	}
	s.Amount = common.StringToBigInt(aux.Amount)
	s.WeightedAmount = common.StringToBigInt(aux.WeightedAmount)
	s.StartTimestamp = aux.StartTimestamp
	s.ExpirationTimestamp = aux.ExpirationTimestamp
	s.Address = aux.Address
	s.Id = aux.Id
	return nil
}

// StakeList is the paged response shape for an address's stake
// enumeration. TotalAmount and TotalWeightedAmount are the
// caller's full pre-paging totals; Count is the number of stake
// entries before paging; Entries is the requested page.
type StakeList struct {
	TotalAmount         *big.Int      `json:"totalAmount"`
	TotalWeightedAmount *big.Int      `json:"totalWeightedAmount"`
	Count               int           `json:"count"`
	Entries             []*StakeEntry `json:"list"`
}

// StakeListMarshal mirrors StakeList with the *big.Int totals
// encoded as decimal strings for JSON precision safety. Entries
// elements are reused as-is since StakeEntry already provides its
// own MarshalJSON.
type StakeListMarshal struct {
	TotalAmount         string        `json:"totalAmount"`
	TotalWeightedAmount string        `json:"totalWeightedAmount"`
	Count               int           `json:"count"`
	Entries             []*StakeEntry `json:"list"`
}

// ToStakeEntryMarshal converts s into its string-totals wire form.
// The method is named ToStakeEntryMarshal rather than
// ToStakeListMarshal for historical reasons; the returned type is
// *StakeListMarshal.
func (s *StakeList) ToStakeEntryMarshal() *StakeListMarshal {
	aux := &StakeListMarshal{
		TotalAmount:         s.TotalAmount.String(),
		TotalWeightedAmount: s.TotalWeightedAmount.String(),
		Count:               s.Count,
	}
	aux.Entries = make([]*StakeEntry, len(s.Entries))
	for idx, entry := range s.Entries {
		aux.Entries[idx] = entry
	}
	return aux
}

// MarshalJSON renders s through StakeListMarshal so totals are
// emitted as decimal strings.
func (s *StakeList) MarshalJSON() ([]byte, error) {
	return json.Marshal(s.ToStakeEntryMarshal())
}

// UnmarshalJSON reads a StakeListMarshal payload and rehydrates
// the *big.Int totals via common.StringToBigInt.
func (s *StakeList) UnmarshalJSON(data []byte) error {
	aux := new(StakeListMarshal)
	if err := json.Unmarshal(data, aux); err != nil {
		return err
	}
	s.TotalAmount = common.StringToBigInt(aux.TotalAmount)
	s.TotalWeightedAmount = common.StringToBigInt(aux.TotalWeightedAmount)
	s.Count = aux.Count
	s.Entries = make([]*StakeEntry, len(aux.Entries))
	for idx, entry := range aux.Entries {
		s.Entries[idx] = entry
	}
	return nil
}

// GetEntriesByAddress returns every stake recorded under address,
// sorted by ascending expiration time (via
// definition.StakeByExpirationTime), and a page of the result.
// TotalAmount and TotalWeightedAmount are the un-paged totals
// reported by definition.GetStakeListByAddress. pageSize >
// api.RpcMaxPageSize is rejected with api.ErrPageSizeParamTooBig.
func (a *StakeApi) GetEntriesByAddress(address types.Address, pageIndex, pageSize uint32) (*StakeList, error) {
	if pageSize > api.RpcMaxPageSize {
		return nil, api.ErrPageSizeParamTooBig
	}

	_, context, err := api.GetFrontierContext(a.chain, types.StakeContract)
	if err != nil {
		return nil, err
	}
	list, total, totalWeighted, err := definition.GetStakeListByAddress(context.Storage(), address)
	if err != nil {
		return nil, err
	}

	sort.Sort(definition.StakeByExpirationTime(list))

	listLen := len(list)
	start, end := api.GetRange(pageIndex, pageSize, uint32(listLen))
	entryList := make([]*StakeEntry, end-start)
	for index, info := range list[start:end] {
		entryList[index] = &StakeEntry{
			Amount:              info.Amount,
			WeightedAmount:      info.WeightedAmount,
			StartTimestamp:      info.StartTime,
			ExpirationTimestamp: info.ExpirationTime,
			Address:             info.StakeAddress,
			Id:                  info.Id,
		}
	}

	return &StakeList{
		TotalAmount:         total,
		TotalWeightedAmount: totalWeighted,
		Count:               listLen,
		Entries:             entryList,
	}, nil
}
