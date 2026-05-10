package embedded

import (
	"encoding/json"
	"github.com/inconshreveable/log15"
	"github.com/zenon-network/go-zenon/chain"
	"github.com/zenon-network/go-zenon/common"
	"github.com/zenon-network/go-zenon/common/types"
	"github.com/zenon-network/go-zenon/rpc/api"
	"github.com/zenon-network/go-zenon/vm/embedded/definition"
	"github.com/zenon-network/go-zenon/zenon"
	"math/big"
	"sort"
)

// LiquidityApi is the "embedded.liquidity" namespace — read access
// to liquidity-mining state (stake entries, reward accounting).
type LiquidityApi struct {
	chain chain.Chain
	log   log15.Logger
}

// NewLiquidityApi constructs the liquidity namespace handler.
func NewLiquidityApi(z zenon.Zenon) *LiquidityApi {
	return &LiquidityApi{
		chain: z.Chain(),
		log:   common.RPCLogger.New("module", "embedded_liquidity_api"),
	}
}

// GetLiquidityInfo returns the global liquidity configuration
// (administrator, ZNN/QSR reward percentages, accepted token list).
func (a *LiquidityApi) GetLiquidityInfo() (*definition.LiquidityInfo, error) {
	_, context, err := api.GetFrontierContext(a.chain, types.LiquidityContract)
	if err != nil {
		return nil, err
	}

	liquidityInfo, err := definition.GetLiquidityInfo(context.Storage())
	if err != nil {
		return nil, err
	}

	return liquidityInfo, nil
}

// GetSecurityInfo returns the liquidity contract's security
// parameters (administrator addresses, time-challenges).
func (a *LiquidityApi) GetSecurityInfo() (*definition.SecurityInfoVariable, error) {
	_, context, err := api.GetFrontierContext(a.chain, types.LiquidityContract)
	if err != nil {
		return nil, err
	}

	security, err := definition.GetSecurityInfoVariable(context.Storage())
	if err != nil {
		return nil, err
	}

	return security, nil
}

// LiquidityStakeList is part of the package's public API; see the surrounding code for usage.
type LiquidityStakeList struct {
	TotalAmount         *big.Int                          `json:"totalAmount"`
	TotalWeightedAmount *big.Int                          `json:"totalWeightedAmount"`
	Count               int                               `json:"count"`
	Entries             []*definition.LiquidityStakeEntry `json:"list"`
}

// LiquidityStakeListMarshal is part of the package's public API; see the surrounding code for usage.
type LiquidityStakeListMarshal struct {
	TotalAmount         string                            `json:"totalAmount"`
	TotalWeightedAmount string                            `json:"totalWeightedAmount"`
	Count               int                               `json:"count"`
	Entries             []*definition.LiquidityStakeEntry `json:"list"`
}

// ToLiquidityStakeListMarshal projects the receiver to its JSON-friendly LiquidityStakeListMarshal twin.
func (stake *LiquidityStakeList) ToLiquidityStakeListMarshal() *LiquidityStakeListMarshal {
	aux := &LiquidityStakeListMarshal{
		TotalAmount:         stake.TotalAmount.String(),
		TotalWeightedAmount: stake.TotalWeightedAmount.String(),
		Count:               stake.Count,
	}
	aux.Entries = make([]*definition.LiquidityStakeEntry, len(stake.Entries))
	for idx, entry := range stake.Entries {
		aux.Entries[idx] = entry
	}
	return aux
}

// MarshalJSON forwards through the Marshal twin so big.Int fields render as decimal strings.
func (stake *LiquidityStakeList) MarshalJSON() ([]byte, error) {
	return json.Marshal(stake.ToLiquidityStakeListMarshal())
}

// UnmarshalJSON inflates the JSON wire form back into the in-memory receiver.
func (stake *LiquidityStakeList) UnmarshalJSON(data []byte) error {
	aux := new(LiquidityStakeListMarshal)
	if err := json.Unmarshal(data, aux); err != nil {
		return err
	}
	stake.TotalAmount = common.StringToBigInt(aux.TotalAmount)
	stake.TotalWeightedAmount = common.StringToBigInt(aux.TotalWeightedAmount)
	stake.Count = aux.Count
	stake.Entries = make([]*definition.LiquidityStakeEntry, len(aux.Entries))
	for idx, entry := range aux.Entries {
		stake.Entries[idx] = entry
	}
	return nil
}

// GetLiquidityStakeEntriesByAddress returns address's liquidity-stake
// entries sorted by descending expiration time, sliced to (pageIndex, pageSize).
// Composes definition.GetLiquidityStakeListByAddress with sort + pagination;
// TotalAmount / TotalWeightedAmount are computed across the full unpaged list.
func (a *LiquidityApi) GetLiquidityStakeEntriesByAddress(address types.Address, pageIndex, pageSize uint32) (*LiquidityStakeList, error) {
	if pageSize > api.RpcMaxPageSize {
		return nil, api.ErrPageSizeParamTooBig
	}

	_, context, err := api.GetFrontierContext(a.chain, types.LiquidityContract)
	if err != nil {
		return nil, err
	}
	list, total, totalWeighted, err := definition.GetLiquidityStakeListByAddress(context.Storage(), address)
	if err != nil {
		return nil, err
	}

	sort.Sort(definition.LiquidityStakeByExpirationTime(list))

	listLen := len(list)
	start, end := api.GetRange(pageIndex, pageSize, uint32(listLen))

	return &LiquidityStakeList{
		TotalAmount:         total,
		TotalWeightedAmount: totalWeighted,
		Count:               listLen,
		Entries:             list[start:end],
	}, nil
}

// GetUncollectedReward returns the unclaimed liquidity-mining reward accrued
// for address. Thin wrapper over the shared [getUncollectedReward] helper,
// scoped to the liquidity contract.
func (a *LiquidityApi) GetUncollectedReward(address types.Address) (*definition.RewardDeposit, error) {
	return getUncollectedReward(a.chain, types.LiquidityContract, address)
}

// GetFrontierRewardByPage returns address's liquidity-mining reward history
// walking backwards from the frontier momentum, sliced to (pageIndex, pageSize).
// Thin wrapper over the shared [getFrontierRewardByPage] helper.
func (a *LiquidityApi) GetFrontierRewardByPage(address types.Address, pageIndex, pageSize uint32) (*RewardHistoryList, error) {
	if pageSize > api.RpcMaxPageSize {
		return nil, api.ErrPageSizeParamTooBig
	}
	return getFrontierRewardByPage(a.chain, types.LiquidityContract, address, pageIndex, pageSize)
}

// GetTimeChallengesInfo returns the active time-challenge records for the
// four governance-gated liquidity methods (NominateGuardians, SetTokenTuple,
// ChangeAdministrator, SetAdditionalReward). Composes per-method
// definition.GetTimeChallengeInfoVariable calls; nil entries are filtered out.
func (a *LiquidityApi) GetTimeChallengesInfo() (*TimeChallengesList, error) {
	_, context, err := api.GetFrontierContext(a.chain, types.LiquidityContract)
	if err != nil {
		return nil, err
	}

	ans := make([]*definition.TimeChallengeInfo, 0)
	methods := []string{"NominateGuardians", "SetTokenTuple", "ChangeAdministrator", "SetAdditionalReward"}

	for _, m := range methods {
		timeC, err := definition.GetTimeChallengeInfoVariable(context.Storage(), m)
		if err != nil {
			return nil, err
		}
		if timeC != nil {
			ans = append(ans, timeC)
		}
	}

	return &TimeChallengesList{
		Count: len(ans),
		List:  ans,
	}, nil
}
