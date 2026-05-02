package embedded

import (
	"encoding/json"
	"github.com/zenon-network/go-zenon/common"
	"math/big"

	"github.com/zenon-network/go-zenon/chain"
	"github.com/zenon-network/go-zenon/common/types"
	"github.com/zenon-network/go-zenon/rpc/api"
	"github.com/zenon-network/go-zenon/vm/embedded/definition"
)

// getDepositedQsr returns the QSR balance fused on contract for
// address, or nil if no deposit exists. Used by APIs that surface
// per-user fused-QSR figures (sentinel, stake, etc.).
func getDepositedQsr(chain chain.Chain, contract types.Address, address types.Address) (*big.Int, error) {
	_, context, err := api.GetFrontierContext(chain, contract)
	if err != nil {
		return nil, err
	}
	qsrDeposit, err := definition.GetQsrDeposit(context.Storage(), &address)
	if err != nil {
		return nil, err
	} else {
		return qsrDeposit.Qsr, nil
	}
}

// getUncollectedReward returns address's pending reward balance
// (ZNN + QSR) on contract — i.e., rewards already credited but not
// yet withdrawn.
func getUncollectedReward(chain chain.Chain, contract types.Address, address types.Address) (*definition.RewardDeposit, error) {
	_, context, err := api.GetFrontierContext(chain, contract)
	if err != nil {
		return nil, err
	}
	return definition.GetRewardDeposit(context.Storage(), &address)
}

// RewardHistoryEntry is one (epoch, ZNN, QSR) row of historical
// reward data — returned by GetFrontierRewardByPage on each
// reward-bearing namespace (pillar, sentinel, stake).
type RewardHistoryEntry struct {
	Epoch int64    `json:"epoch"`
	Znn   *big.Int `json:"znnAmount"`
	Qsr   *big.Int `json:"qsrAmount"`
}

// RewardHistoryEntryMarshal is the JSON-friendly twin of
// [RewardHistoryEntry] with decimal-string amounts.
type RewardHistoryEntryMarshal struct {
	Epoch int64  `json:"epoch"`
	Znn   string `json:"znnAmount"`
	Qsr   string `json:"qsrAmount"`
}

// ToRewardDepositMarshal projects the receiver to its JSON-friendly RewardDepositMarshal twin.
func (r *RewardHistoryEntry) ToRewardDepositMarshal() *RewardHistoryEntryMarshal {
	aux := &RewardHistoryEntryMarshal{
		Epoch: r.Epoch,
		Znn:   r.Znn.String(),
		Qsr:   r.Qsr.String(),
	}

	return aux
}

// MarshalJSON forwards through the Marshal twin so big.Int fields render as decimal strings.
func (r *RewardHistoryEntry) MarshalJSON() ([]byte, error) {
	return json.Marshal(r.ToRewardDepositMarshal())
}

// UnmarshalJSON inflates the JSON wire form back into the in-memory receiver.
func (r *RewardHistoryEntry) UnmarshalJSON(data []byte) error {
	aux := new(RewardHistoryEntryMarshal)
	if err := json.Unmarshal(data, aux); err != nil {
		return err
	}
	r.Epoch = aux.Epoch
	r.Znn = common.StringToBigInt(aux.Znn)
	r.Qsr = common.StringToBigInt(aux.Qsr)
	return nil
}

// RewardHistoryList is the paginated response shape for
// reward-history queries.
type RewardHistoryList struct {
	Count int64                 `json:"count"`
	List  []*RewardHistoryEntry `json:"list"`
}

// getFrontierRewardByPage walks the reward-deposit history backwards
// from the latest epoch, returning up to pageSize entries per page.
// Empty epochs (no reward deposit for address) are skipped silently.
// Shared by every reward-bearing namespace.
func getFrontierRewardByPage(chain chain.Chain, contract types.Address, address types.Address, pageIndex, pageSize uint32) (*RewardHistoryList, error) {
	if pageSize > api.RpcMaxPageSize {
		return nil, api.ErrPageSizeParamTooBig
	}

	_, context, err := api.GetFrontierContext(chain, contract)
	if err != nil {
		return nil, err
	}

	// get latest epoch
	lastEpoch, err := definition.GetLastEpochUpdate(context.Storage())
	if err != nil {
		return nil, err
	}

	epoch := lastEpoch.LastEpoch - int64(pageIndex*pageSize)

	result := &RewardHistoryList{
		Count: lastEpoch.LastEpoch + 1,
		List:  make([]*RewardHistoryEntry, 0, pageSize),
	}
	for i := 0; i < int(pageSize); i += 1 {
		if epoch < 0 {
			break
		}
		if d, err := definition.GetRewardDepositHistory(context.Storage(), uint64(epoch), &address); err == nil {
			result.List = append(result.List, &RewardHistoryEntry{
				Epoch: epoch,
				Znn:   (new(big.Int)).Set(d.Znn),
				Qsr:   (new(big.Int)).Set(d.Qsr),
			})
		} else {
			return nil, err
		}
		epoch -= 1
	}

	return result, err
}
