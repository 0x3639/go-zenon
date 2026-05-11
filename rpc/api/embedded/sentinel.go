package embedded

import (
	"github.com/inconshreveable/log15"

	"github.com/zenon-network/go-zenon/chain"
	"github.com/zenon-network/go-zenon/common"
	"github.com/zenon-network/go-zenon/common/types"
	rpcapi "github.com/zenon-network/go-zenon/rpc/api"
	"github.com/zenon-network/go-zenon/vm/embedded/definition"
	"github.com/zenon-network/go-zenon/vm/embedded/implementation"
	"github.com/zenon-network/go-zenon/zenon"
)

// SentinelApi serves read RPCs for the sentinel contract — full
// nodes that backstop the network in exchange for a periodic reward.
type SentinelApi struct {
	chain chain.Chain
	log   log15.Logger
}

// SentinelInfo is the RPC view of a single sentinel registration.
//
// CanBeRevoked and RevokeCooldown come from
// implementation.GetSentinelRevokeStatus and reflect the time
// remaining (in seconds) before the owner may revoke the
// registration. Active is true when the registration has not yet
// been revoked — equivalently, when the underlying
// definition.SentinelInfo's RevokeTimestamp is zero.
type SentinelInfo struct {
	Owner                 types.Address `json:"owner"`
	RegistrationTimestamp int64         `json:"registrationTimestamp"`
	CanBeRevoked          bool          `json:"isRevocable"`
	RevokeCooldown        int64         `json:"revokeCooldown"`
	Active                bool          `json:"active"`
}

// SentinelInfoList is the paged response shape for active-sentinel
// enumeration: Count is the number of active sentinels (post-filter)
// before paging, List is the requested page.
type SentinelInfoList struct {
	Count int             `json:"count"`
	List  []*SentinelInfo `json:"list"`
}

// NewSentinelApi returns a SentinelApi bound to z's chain. Sentinel
// reads never need a consensus handle, so unlike other API
// constructors in this package only chain is stored.
func NewSentinelApi(z zenon.Zenon) *SentinelApi {
	return &SentinelApi{
		chain: z.Chain(),
		log:   common.RPCLogger.New("module", "embedded_sentinel_api"),
	}
}

func (api *SentinelApi) toSentinelInfo(sentinel *definition.SentinelInfo) *SentinelInfo {
	m, _, err := rpcapi.GetFrontierContext(api.chain, types.SentinelContract)
	if err != nil {
		return nil
	}

	canBeRevoked, revokeCooldown := implementation.GetSentinelRevokeStatus(sentinel.RegistrationTimestamp, m)
	return &SentinelInfo{
		Owner:                 sentinel.Owner,
		RegistrationTimestamp: sentinel.RegistrationTimestamp,
		CanBeRevoked:          canBeRevoked,
		RevokeCooldown:        revokeCooldown,
		Active:                sentinel.RevokeTimestamp == 0,
	}
}

// GetByOwner returns the sentinel registered to owner, including
// computed revocation status, or (nil, nil) when no registration is
// recorded for that address. Errors from the storage read are
// propagated unchanged.
func (api *SentinelApi) GetByOwner(owner types.Address) (*SentinelInfo, error) {
	_, context, err := rpcapi.GetFrontierContext(api.chain, types.SentinelContract)
	if err != nil {
		return nil, err
	}
	sentinel := definition.GetSentinelInfoByOwner(context.Storage(), owner)
	if sentinel != nil {
		return api.toSentinelInfo(sentinel), nil
	} else {
		return nil, nil
	}
}

// GetAllActive iterates every sentinel record, keeps only those
// whose RevokeTimestamp is zero, and returns a page of the result.
// Count reflects the number of active sentinels after filtering, not
// the underlying record count. pageSize > api.RpcMaxPageSize is
// rejected with api.ErrPageSizeParamTooBig.
func (api *SentinelApi) GetAllActive(pageIndex, pageSize uint32) (*SentinelInfoList, error) {
	if pageSize > rpcapi.RpcMaxPageSize {
		return nil, rpcapi.ErrPageSizeParamTooBig
	}
	_, context, err := rpcapi.GetFrontierContext(api.chain, types.SentinelContract)
	if err != nil {
		return nil, err
	}

	rawList := definition.GetAllSentinelInfo(context.Storage())

	list := make([]*SentinelInfo, 0, len(rawList))
	for _, raw := range rawList {
		if raw.RevokeTimestamp == 0 {
			list = append(list, api.toSentinelInfo(raw))
		}
	}
	start, end := rpcapi.GetRange(pageIndex, pageSize, uint32(len(list)))

	return &SentinelInfoList{
		Count: len(list),
		List:  list[start:end],
	}, nil
}

// === Shared RPCs ===
//
// These three methods exist on every reward-bearing API
// (PillarApi, SentinelApi, StakeApi, LiquidityApi) and forward to
// the helpers in shared.go scoped to the SentinelContract address.

// GetDepositedQsr returns the QSR amount the address has deposited
// to the sentinel contract, formatted as a decimal string.
func (api *SentinelApi) GetDepositedQsr(address types.Address) (string, error) {
	depositedQsr, err := getDepositedQsr(api.chain, types.SentinelContract, address)
	return depositedQsr.String(), err
}

// GetUncollectedReward returns the cumulative uncollected
// ZNN + QSR reward owed to address by the sentinel contract.
// The definition layer zero-fills the "no entry" case, so the
// result is never (nil, nil); a zero-valued *RewardDeposit
// (Znn = Qsr = 0) represents "nothing owed yet".
func (api *SentinelApi) GetUncollectedReward(address types.Address) (*definition.RewardDeposit, error) {
	return getUncollectedReward(api.chain, types.SentinelContract, address)
}

// GetFrontierRewardByPage walks epochs descending from the latest
// LastEpochUpdate and returns a paged window of per-epoch rewards
// for the address from the sentinel contract.
func (api *SentinelApi) GetFrontierRewardByPage(address types.Address, pageIndex, pageSize uint32) (*RewardHistoryList, error) {
	if pageSize > rpcapi.RpcMaxPageSize {
		return nil, rpcapi.ErrPageSizeParamTooBig
	}
	return getFrontierRewardByPage(api.chain, types.SentinelContract, address, pageIndex, pageSize)
}
