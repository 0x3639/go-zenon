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

// getDepositedQsr returns the QSR deposit recorded for address
// against contract. Called by the per-contract GetDepositedQsr
// methods (PillarApi, SentinelApi) — the contract address is the
// only thing that varies between callers.
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

// getUncollectedReward returns the cumulative uncollected
// ZNN + QSR reward owed to address by contract. The definition
// layer (definition.GetRewardDeposit) synthesises a zero-valued
// *RewardDeposit (Znn = Qsr = 0) when no entry exists, so callers
// always receive a non-nil result for the "no rewards yet" case;
// (nil, err) is returned only on a real storage error. Used by
// every reward-bearing API (Pillar/Sentinel/Stake/Liquidity).
func getUncollectedReward(chain chain.Chain, contract types.Address, address types.Address) (*definition.RewardDeposit, error) {
	_, context, err := api.GetFrontierContext(chain, contract)
	if err != nil {
		return nil, err
	}
	return definition.GetRewardDeposit(context.Storage(), &address)
}

// RewardHistoryEntry is one row in the per-epoch reward history
// surfaced by Pillar/Sentinel/Stake/Liquidity APIs:
// the epoch number plus the ZNN and QSR amounts credited that
// epoch.
type RewardHistoryEntry struct {
	Epoch int64    `json:"epoch"`
	Znn   *big.Int `json:"znnAmount"`
	Qsr   *big.Int `json:"qsrAmount"`
}

// RewardHistoryEntryMarshal mirrors RewardHistoryEntry with
// string-encoded amounts so the JSON wire format does not lose
// precision on very large *big.Int values. Round-trip via
// MarshalJSON / UnmarshalJSON on RewardHistoryEntry.
type RewardHistoryEntryMarshal struct {
	Epoch int64  `json:"epoch"`
	Znn   string `json:"znnAmount"`
	Qsr   string `json:"qsrAmount"`
}

// ToRewardDepositMarshal converts r into its string-amount wire
// form. The method name predates this file's other To<Type>Marshal
// methods and references the underlying definition.RewardDeposit
// shape rather than RewardHistoryEntry itself; it is preserved so
// existing callers do not break.
func (r *RewardHistoryEntry) ToRewardDepositMarshal() *RewardHistoryEntryMarshal {
	aux := &RewardHistoryEntryMarshal{
		Epoch: r.Epoch,
		Znn:   r.Znn.String(),
		Qsr:   r.Qsr.String(),
	}

	return aux
}

// MarshalJSON renders r through its RewardHistoryEntryMarshal twin
// so *big.Int amounts become decimal strings on the wire.
func (r *RewardHistoryEntry) MarshalJSON() ([]byte, error) {
	return json.Marshal(r.ToRewardDepositMarshal())
}

// UnmarshalJSON reads a RewardHistoryEntryMarshal payload and
// rehydrates the *big.Int Znn/Qsr fields from their decimal-string
// counterparts via common.StringToBigInt.
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

// RewardHistoryList is the paged response shape for per-epoch
// reward history. Count is the total number of epochs covered
// (lastEpoch + 1, i.e. epochs 0..lastEpoch inclusive) rather than
// the size of List, so clients can compute the global page count.
type RewardHistoryList struct {
	Count int64                 `json:"count"`
	List  []*RewardHistoryEntry `json:"list"`
}

// getFrontierRewardByPage walks epochs descending starting from
// the latest LastEpochUpdate and returns a paged window. The
// definition layer (definition.GetRewardDepositHistory)
// zero-fills missing epochs, so absent epochs surface as
// RewardHistoryEntry rows with Znn = Qsr = 0 rather than being
// skipped. The loop stops when either pageSize entries have been
// collected or the epoch counter falls below zero, so the page
// contains exactly min(pageSize, lastEpoch + 1 - pageIndex*pageSize)
// entries.
//
// pageSize > api.RpcMaxPageSize is rejected with
// api.ErrPageSizeParamTooBig before the chain read.
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
