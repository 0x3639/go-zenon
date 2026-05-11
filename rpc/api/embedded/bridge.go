package embedded

import (
	"encoding/json"
	"github.com/inconshreveable/log15"
	"github.com/pkg/errors"
	"github.com/zenon-network/go-zenon/chain/nom"
	"github.com/zenon-network/go-zenon/vm/constants"
	"github.com/zenon-network/go-zenon/vm/embedded/implementation"
	"reflect"
	"sort"

	"github.com/zenon-network/go-zenon/chain"
	"github.com/zenon-network/go-zenon/common"
	"github.com/zenon-network/go-zenon/common/types"
	"github.com/zenon-network/go-zenon/rpc/api"
	"github.com/zenon-network/go-zenon/vm/embedded/definition"
	"github.com/zenon-network/go-zenon/vm/vm_context"
	"github.com/zenon-network/go-zenon/zenon"
)

// BridgeApi serves read RPCs for the cross-chain bridge contract:
// wrap/unwrap request enumeration, network configuration, security
// info, and administrator time-challenges.
type BridgeApi struct {
	chain chain.Chain
	log   log15.Logger
}

// NewBridgeApi returns a BridgeApi bound to z's chain. Bridge
// reads do not need a consensus handle.
func NewBridgeApi(z zenon.Zenon) *BridgeApi {
	return &BridgeApi{
		chain: z.Chain(),
		log:   common.RPCLogger.New("module", "embedded_bridge_api"),
	}
}

// GetBridgeInfo returns the BridgeInfoVariable singleton: the
// administrator address, the orchestrator's TSS ECDSA public-key
// pair (compressed + decompressed), the AllowKeyGen and Halted
// flags, the unhalt countdown (UnhaltedAt +
// UnhaltDurationInMomentums), the current TssNonce, and a free-form
// Metadata string (JSON-validated on writes by the implementation
// layer). The orchestrator's own networking/finality settings live
// separately in OrchestratorInfo (see GetOrchestratorInfo). Errors
// from the storage read propagate unchanged.
func (a *BridgeApi) GetBridgeInfo() (*definition.BridgeInfoVariable, error) {
	_, context, err := api.GetFrontierContext(a.chain, types.BridgeContract)
	if err != nil {
		return nil, err
	}

	bridgeInfo, err := definition.GetBridgeInfoVariable(context.Storage())
	if err != nil {
		return nil, err
	}

	return bridgeInfo, nil
}

// GetSecurityInfo returns the SecurityInfoVariable singleton:
// the guardian set, the soft-delay, and other governance
// parameters that gate administrator actions on this contract.
// Structurally identical to LiquidityApi.GetSecurityInfo but reads
// the bridge contract's own storage.
func (a *BridgeApi) GetSecurityInfo() (*definition.SecurityInfoVariable, error) {
	_, context, err := api.GetFrontierContext(a.chain, types.BridgeContract)
	if err != nil {
		return nil, err
	}

	security, err := definition.GetSecurityInfoVariable(context.Storage())
	if err != nil {
		return nil, err
	}

	return security, nil
}

// GetOrchestratorInfo returns the OrchestratorInfo singleton —
// the off-chain orchestrator's signing-key state and the
// confirmations-to-finality threshold used by wrap-request
// hydration.
func (a *BridgeApi) GetOrchestratorInfo() (*definition.OrchestratorInfo, error) {
	_, context, err := api.GetFrontierContext(a.chain, types.BridgeContract)
	if err != nil {
		return nil, err
	}

	orchestratorInfo, err := definition.GetOrchestratorInfoVariable(context.Storage())
	if err != nil {
		return nil, err
	}

	return orchestratorInfo, nil
}

// TimeChallengesList is the response shape for time-challenge
// enumeration on the bridge or liquidity contracts: each entry
// is one outstanding administrator-action challenge. Methods
// without an outstanding challenge are omitted, so Count reflects
// only the in-flight subset.
//
// The type is declared here because the bridge contract was the
// first user of the time-challenge pattern; LiquidityApi reuses
// it verbatim for its own four method names.
type TimeChallengesList struct {
	Count int                             `json:"count"`
	List  []*definition.TimeChallengeInfo `json:"list"`
}

// GetTimeChallengesInfo returns the time-challenge record (if any)
// for each of the bridge contract's four administrator-gated
// methods: NominateGuardians, ChangeTssECDSAPubKey,
// ChangeAdministrator, SetTokenPair. Methods without an
// outstanding challenge are silently omitted.
func (a *BridgeApi) GetTimeChallengesInfo() (*TimeChallengesList, error) {
	_, context, err := api.GetFrontierContext(a.chain, types.BridgeContract)
	if err != nil {
		return nil, err
	}

	ans := make([]*definition.TimeChallengeInfo, 0)
	methods := []string{"NominateGuardians", "ChangeTssECDSAPubKey", "ChangeAdministrator", "SetTokenPair"}

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

// GetNetworkInfo returns the NetworkInfo for one (networkClass,
// chainId) pair — the off-chain network's enabled state, contract
// address, and registered token pairs. Returns the underlying
// storage error when the network is unknown.
func (a *BridgeApi) GetNetworkInfo(networkClass uint32, chainId uint32) (*definition.NetworkInfo, error) {
	_, context, err := api.GetFrontierContext(a.chain, types.BridgeContract)
	if err != nil {
		return nil, err
	}

	networkInfo, err := definition.GetNetworkInfoVariable(context.Storage(), networkClass, chainId)
	if err != nil {
		return nil, err
	}

	return networkInfo, nil
}

// GetAllNetworks returns one page of every registered network,
// in the order returned by definition.GetNetworkList (no
// post-sort). pageSize > api.RpcMaxPageSize is rejected with
// api.ErrPageSizeParamTooBig.
func (a *BridgeApi) GetAllNetworks(pageIndex, pageSize uint32) (*NetworkInfoList, error) {
	if pageSize > api.RpcMaxPageSize {
		return nil, api.ErrPageSizeParamTooBig
	}

	_, context, err := api.GetFrontierContext(a.chain, types.BridgeContract)
	if err != nil {
		return nil, err
	}
	networkList, err := definition.GetNetworkList(context.Storage())
	if err != nil {
		return nil, err
	}
	start, end := api.GetRange(pageIndex, pageSize, uint32(len(networkList)))
	list := networkList[start:end]

	result := &NetworkInfoList{
		Count: len(networkList),
		List:  list,
	}
	return result, nil
}

// NetworkInfoList is the paged response shape for network
// enumeration. Count is the full pre-paging total.
type NetworkInfoList struct {
	Count int                       `json:"count"`
	List  []*definition.NetworkInfo `json:"list"`
}

// toRequest hydrates an outbound (wrap) request with the matching
// foreign-chain TokenAddress, taken from the network's token-pair
// table. Returns nil when no matching pair is registered for the
// request's TokenStandard on its (NetworkClass, ChainId) — the
// caller is expected to skip such requests rather than surface a
// half-populated record.
func (a *BridgeApi) toRequest(context vm_context.AccountVmContext, abiRequest *definition.WrapTokenRequest) *definition.WrapTokenRequest {
	if abiRequest == nil {
		return nil
	}
	networkInfoVariable, err := definition.GetNetworkInfoVariable(context.Storage(), abiRequest.NetworkClass, abiRequest.ChainId)
	if err != nil {
		return nil
	}
	tokenAddress := ""
	for i := 0; i < len(networkInfoVariable.TokenPairs); i++ {
		if reflect.DeepEqual(networkInfoVariable.TokenPairs[i].TokenStandard.Bytes(), abiRequest.TokenStandard.Bytes()) {
			tokenAddress = networkInfoVariable.TokenPairs[i].TokenAddress
		}
	}
	if tokenAddress == "" {
		return nil
	}
	request := &definition.WrapTokenRequest{
		NetworkClass: abiRequest.NetworkClass,
		ChainId:      abiRequest.ChainId,
		Id:           abiRequest.Id,
		ToAddress:    abiRequest.ToAddress,
		TokenAddress: tokenAddress,
		Amount:       abiRequest.Amount,
		Signature:    abiRequest.Signature,
	}
	return request
}

// WrapTokenRequest is the RPC view of an outbound (NoM-to-foreign)
// bridge request: the on-chain definition.WrapTokenRequest record
// plus the resolved token metadata and a countdown of remaining
// momentums until the request reaches the configured
// confirmations-to-finality threshold (0 once finalised).
type WrapTokenRequest struct {
	*definition.WrapTokenRequest
	TokenInfo               *api.Token `json:"token"`
	ConfirmationsToFinality uint64     `json:"confirmationsToFinality"`
}

// MarshalJSON renders w as a flat JSON object combining the
// embedded definition.WrapTokenRequestMarshal fields, the
// TokenMarshal token info, and ConfirmationsToFinality. The
// embedded marshal handles the *big.Int → string conversion for
// amount fields on the wrap-request itself.
func (w *WrapTokenRequest) MarshalJSON() ([]byte, error) {
	aux := struct {
		*definition.WrapTokenRequestMarshal
		TokenInfo               *api.TokenMarshal `json:"token"`
		ConfirmationsToFinality uint64            `json:"confirmationsToFinality"`
	}{
		WrapTokenRequestMarshal: w.WrapTokenRequest.ToMarshalJson(),
		ConfirmationsToFinality: w.ConfirmationsToFinality,
	}
	if w.TokenInfo != nil {
		aux.TokenInfo = w.TokenInfo.ToTokenMarshal()
	}

	return json.Marshal(aux)
}

// UnmarshalJSON reverses MarshalJSON: it reads the flat
// definition.WrapTokenRequestMarshal + TokenMarshal +
// ConfirmationsToFinality payload and rehydrates the *big.Int
// amount and fee fields via common.StringToBigInt.
func (w *WrapTokenRequest) UnmarshalJSON(data []byte) error {
	aux := &struct {
		*definition.WrapTokenRequestMarshal
		TokenInfo               *api.TokenMarshal `json:"token"`
		ConfirmationsToFinality uint64            `json:"confirmationsToFinality"`
	}{}
	if err := json.Unmarshal(data, aux); err != nil {
		return err
	}

	w.WrapTokenRequest = &definition.WrapTokenRequest{
		NetworkClass:           aux.WrapTokenRequestMarshal.NetworkClass,
		ChainId:                aux.ChainId,
		Id:                     aux.Id,
		ToAddress:              aux.ToAddress,
		TokenStandard:          aux.TokenStandard,
		TokenAddress:           aux.TokenAddress,
		Amount:                 common.StringToBigInt(aux.Amount),
		Fee:                    common.StringToBigInt(aux.Fee),
		Signature:              aux.Signature,
		CreationMomentumHeight: aux.CreationMomentumHeight,
	}
	if aux.TokenInfo != nil {
		w.TokenInfo = aux.TokenInfo.FromTokenMarshal()
	}
	w.ConfirmationsToFinality = aux.ConfirmationsToFinality
	return nil
}

// getToken fetches the on-chain token registry record for zts
// and converts it to the RPC api.Token view. Returns (nil, nil)
// when no token is registered (the underlying
// constants.ErrDataNonExistent is swallowed) and (nil, err) on
// any other storage failure. Note that enumerator callers
// (GetAllWrapTokenRequests, GetAllUnwrapTokenRequests, etc.)
// only `continue` on err != nil, so an unknown ZTS produces an
// entry with TokenInfo == nil rather than a skipped slot.
func (a *BridgeApi) getToken(zts types.ZenonTokenStandard) (*api.Token, error) {
	_, context, err := api.GetFrontierContext(a.chain, types.TokenContract)
	if err != nil {
		return nil, err
	}
	tokenInfo, err := definition.GetTokenInfo(context.Storage(), zts)
	if err == constants.ErrDataNonExistent {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if tokenInfo != nil {
		return api.LedgerTokenInfoToRpc(tokenInfo), nil
	}
	return nil, nil
}

// getRedeemableIn returns how many momentums are still required
// before unwrapTokenRequest may be redeemed under tokenPair's
// RedeemDelay policy. Returns 0 once the delay has elapsed.
func (a *BridgeApi) getRedeemableIn(unwrapTokenRequest definition.UnwrapTokenRequest, tokenPair definition.TokenPair, momentum nom.Momentum) uint64 {
	var redeemableIn uint64
	if momentum.Height-unwrapTokenRequest.RegistrationMomentumHeight >= uint64(tokenPair.RedeemDelay) {
		redeemableIn = 0
	} else {
		redeemableIn = unwrapTokenRequest.RegistrationMomentumHeight + uint64(tokenPair.RedeemDelay) - momentum.Height
	}
	return redeemableIn
}

// getConfirmationsToFinality returns how many momentums are still
// required before wrapTokenRequest reaches the configured
// finality threshold (orchestrator-side this is when the
// off-chain bridge will sign the request). Returns 0 once the
// threshold has elapsed. Despite the (uint64, error) signature
// the implementation cannot return a non-nil error today; the
// error return is retained for forward compatibility.
func (a *BridgeApi) getConfirmationsToFinality(wrapTokenRequest definition.WrapTokenRequest, confirmationsToFinality uint32, momentum nom.Momentum) (uint64, error) {
	var actualConfirmationsToFinality uint64
	if momentum.Height-wrapTokenRequest.CreationMomentumHeight >= uint64(confirmationsToFinality) {
		actualConfirmationsToFinality = 0
	} else {
		actualConfirmationsToFinality = wrapTokenRequest.CreationMomentumHeight + uint64(confirmationsToFinality) - momentum.Height
	}
	return actualConfirmationsToFinality, nil
}

// GetWrapTokenRequestById returns one wrap request by id with
// token metadata and confirmations-to-finality countdown attached.
// Errors from any of the storage/momentum/orchestrator reads
// propagate unchanged.
func (a *BridgeApi) GetWrapTokenRequestById(id types.Hash) (*WrapTokenRequest, error) {
	_, context, err := api.GetFrontierContext(a.chain, types.BridgeContract)
	if err != nil {
		return nil, err
	}

	wrapTokenRequest, err := definition.GetWrapTokenRequestById(context.Storage(), id)
	if err != nil {
		return nil, err
	}

	token, err := a.getToken(wrapTokenRequest.TokenStandard)
	if err != nil {
		return nil, err
	}

	momentum, err := context.GetFrontierMomentum()
	if err != nil {
		return nil, err
	}
	orchestratorInfo, err := definition.GetOrchestratorInfoVariable(context.Storage())
	if err != nil {
		return nil, err
	}
	confirmationsToFinality, err := a.getConfirmationsToFinality(*wrapTokenRequest, orchestratorInfo.ConfirmationsToFinality, *momentum)
	if err != nil {
		return nil, err
	}

	return &WrapTokenRequest{wrapTokenRequest, token, confirmationsToFinality}, nil
}

// WrapTokenRequestList is the paged response shape for wrap-request
// enumeration. Count reflects different things depending on the
// caller — see each Get*WrapTokenRequests* method for the precise
// definition (raw total vs filtered total vs unsigned-only total).
type WrapTokenRequestList struct {
	Count int                 `json:"count"`
	List  []*WrapTokenRequest `json:"list"`
}

// GetAllWrapTokenRequests returns one page of every recorded wrap
// request, in the order returned by definition.GetWrapTokenRequests.
// Count is the raw underlying total before paging. Per-entry token
// lookups go through getToken, which returns (nil, nil) for
// unknown ZTSes — those entries still appear in List with
// TokenInfo == nil. Entries are only skipped when getToken or
// getConfirmationsToFinality returns a non-nil error.
func (a *BridgeApi) GetAllWrapTokenRequests(pageIndex, pageSize uint32) (*WrapTokenRequestList, error) {
	_, context, err := api.GetFrontierContext(a.chain, types.BridgeContract)
	if err != nil {
		return nil, err
	}

	requests, err := definition.GetWrapTokenRequests(context.Storage())
	if err != nil {
		return nil, err
	}

	result := &WrapTokenRequestList{
		Count: len(requests),
		List:  make([]*WrapTokenRequest, 0),
	}

	momentum, err := context.GetFrontierMomentum()
	if err != nil {
		return nil, err
	}
	orchestratorInfo, err := definition.GetOrchestratorInfoVariable(context.Storage())
	if err != nil {
		return nil, err
	}

	start, end := api.GetRange(pageIndex, pageSize, uint32(len(requests)))
	for i := start; i < end; i++ {
		token, err := a.getToken(requests[i].TokenStandard)
		if err != nil {
			continue
		}
		confirmationsToFinality, err := a.getConfirmationsToFinality(*requests[i], orchestratorInfo.ConfirmationsToFinality, *momentum)
		if err != nil {
			continue
		}
		wrapReqest := &WrapTokenRequest{requests[i], token, confirmationsToFinality}
		result.List = append(result.List, wrapReqest)
	}
	return result, nil
}

// GetAllWrapTokenRequestsByToAddress returns one page of wrap
// requests filtered by ToAddress. Passing toAddress == "" yields
// the unfiltered set (equivalent to GetAllWrapTokenRequests but
// without the in-loop skip behaviour). Count reflects the
// filtered total prior to paging.
func (a *BridgeApi) GetAllWrapTokenRequestsByToAddress(toAddress string, pageIndex, pageSize uint32) (*WrapTokenRequestList, error) {
	_, context, err := api.GetFrontierContext(a.chain, types.BridgeContract)
	if err != nil {
		return nil, err
	}

	requests, err := definition.GetWrapTokenRequests(context.Storage())
	if err != nil {
		return nil, err
	}

	result := &WrapTokenRequestList{
		List: make([]*WrapTokenRequest, 0),
	}

	specificRequests := make([]*definition.WrapTokenRequest, 0)
	if toAddress == "" {
		specificRequests = append(specificRequests, requests...)
	} else {
		for _, request := range requests {
			if request.ToAddress == toAddress {
				specificRequests = append(specificRequests, request)
			}
		}
	}
	result.Count = len(specificRequests)
	start, end := api.GetRange(pageIndex, pageSize, uint32(len(specificRequests)))

	momentum, err := context.GetFrontierMomentum()
	if err != nil {
		return nil, err
	}
	orchestratorInfo, err := definition.GetOrchestratorInfoVariable(context.Storage())
	if err != nil {
		return nil, err
	}
	for i := start; i < end; i++ {
		token, err := a.getToken(specificRequests[i].TokenStandard)
		if err != nil {
			continue
		}
		confirmationsToFinality, err := a.getConfirmationsToFinality(*specificRequests[i], orchestratorInfo.ConfirmationsToFinality, *momentum)
		if err != nil {
			continue
		}
		wrapRequest := &WrapTokenRequest{specificRequests[i], token, confirmationsToFinality}
		result.List = append(result.List, wrapRequest)
	}
	return result, nil
}

// GetAllWrapTokenRequestsByToAddressNetworkClassAndChainId returns
// one page of wrap requests filtered by NetworkClass + ChainId
// and (optionally) ToAddress. Passing toAddress == "" disables
// the address filter. Count reflects the filtered total prior to
// paging.
func (a *BridgeApi) GetAllWrapTokenRequestsByToAddressNetworkClassAndChainId(toAddress string, networkClass, chainId uint32, pageIndex, pageSize uint32) (*WrapTokenRequestList, error) {
	_, context, err := api.GetFrontierContext(a.chain, types.BridgeContract)
	if err != nil {
		return nil, err
	}

	requests, err := definition.GetWrapTokenRequests(context.Storage())
	if err != nil {
		return nil, err
	}

	result := &WrapTokenRequestList{
		List: make([]*WrapTokenRequest, 0),
	}

	specificRequests := make([]*definition.WrapTokenRequest, 0)
	for _, request := range requests {
		if request.NetworkClass == networkClass && request.ChainId == chainId && (toAddress == "" || request.ToAddress == toAddress) {
			specificRequests = append(specificRequests, request)
		}
	}
	result.Count = len(specificRequests)
	start, end := api.GetRange(pageIndex, pageSize, uint32(len(specificRequests)))

	momentum, err := context.GetFrontierMomentum()
	if err != nil {
		return nil, err
	}
	orchestratorInfo, err := definition.GetOrchestratorInfoVariable(context.Storage())
	if err != nil {
		return nil, err
	}

	for i := start; i < end; i++ {
		token, err := a.getToken(specificRequests[i].TokenStandard)
		if err != nil {
			continue
		}
		confirmationsToFinality, err := a.getConfirmationsToFinality(*specificRequests[i], orchestratorInfo.ConfirmationsToFinality, *momentum)
		if err != nil {
			continue
		}
		wrapRequest := &WrapTokenRequest{specificRequests[i], token, confirmationsToFinality}
		result.List = append(result.List, wrapRequest)
	}
	return result, nil
}

// GetAllUnsignedWrapTokenRequests returns one page of wrap
// requests whose Signature is still empty (i.e. waiting on the
// off-chain orchestrator). The full filtered list is reversed
// before paging so callers see the oldest unsigned request first.
// Count reflects the unsigned-only total prior to paging.
func (a *BridgeApi) GetAllUnsignedWrapTokenRequests(pageIndex, pageSize uint32) (*WrapTokenRequestList, error) {
	_, context, err := api.GetFrontierContext(a.chain, types.BridgeContract)
	if err != nil {
		return nil, err
	}

	requests, err := definition.GetWrapTokenRequests(context.Storage())
	if err != nil {
		return nil, err
	}
	var unsignedRequests []*WrapTokenRequest

	momentum, err := context.GetFrontierMomentum()
	if err != nil {
		return nil, err
	}
	orchestratorInfo, err := definition.GetOrchestratorInfoVariable(context.Storage())
	if err != nil {
		return nil, err
	}

	for _, request := range requests {
		if request.Signature == "" {
			token, err := a.getToken(request.TokenStandard)
			if err != nil {
				continue
			}
			confirmationsToFinality, err := a.getConfirmationsToFinality(*request, orchestratorInfo.ConfirmationsToFinality, *momentum)
			if err != nil {
				continue
			}
			wrapRequest := &WrapTokenRequest{request, token, confirmationsToFinality}
			unsignedRequests = append(unsignedRequests, wrapRequest)
		}
	}

	for i, j := 0, len(unsignedRequests)-1; i < j; i, j = i+1, j-1 {
		unsignedRequests[i], unsignedRequests[j] = unsignedRequests[j], unsignedRequests[i]
	}

	result := &WrapTokenRequestList{
		Count: len(unsignedRequests),
		List:  make([]*WrapTokenRequest, len(unsignedRequests)),
	}

	start, end := api.GetRange(pageIndex, pageSize, uint32(len(unsignedRequests)))
	result.List = unsignedRequests[start:end]
	return result, nil
}

// UnwrapTokenRequest is the RPC view of an inbound
// (foreign-to-NoM) bridge request: the on-chain
// definition.UnwrapTokenRequest record plus token metadata and a
// countdown of momentums until the request becomes redeemable
// per the token pair's RedeemDelay (0 once redeemable).
type UnwrapTokenRequest struct {
	*definition.UnwrapTokenRequest
	TokenInfo    *api.Token `json:"token"`
	RedeemableIn uint64     `json:"redeemableIn"`
}

// MarshalJSON renders u as a flat JSON object combining the
// embedded definition.UnwrapTokenRequestMarshal fields, the
// TokenMarshal token info, and RedeemableIn. The embedded marshal
// handles the *big.Int → string conversion for the request's
// Amount field.
func (u *UnwrapTokenRequest) MarshalJSON() ([]byte, error) {
	aux := struct {
		*definition.UnwrapTokenRequestMarshal
		TokenInfo    *api.TokenMarshal `json:"token"`
		RedeemableIn uint64            `json:"redeemableIn"`
	}{
		UnwrapTokenRequestMarshal: u.UnwrapTokenRequest.ToMarshalJson(),
		RedeemableIn:              u.RedeemableIn,
	}
	if u.TokenInfo != nil {
		aux.TokenInfo = u.TokenInfo.ToTokenMarshal()
	}
	return json.Marshal(aux)
}

// UnmarshalJSON reverses MarshalJSON: it reads the flat
// definition.UnwrapTokenRequestMarshal + TokenMarshal +
// RedeemableIn payload and rehydrates the embedded
// definition.UnwrapTokenRequest with its *big.Int Amount
// reconstituted via common.StringToBigInt.
func (u *UnwrapTokenRequest) UnmarshalJSON(data []byte) error {
	aux := &struct {
		*definition.UnwrapTokenRequestMarshal
		TokenInfo    *api.TokenMarshal `json:"token"`
		RedeemableIn uint64            `json:"redeemableIn"`
	}{}
	if err := json.Unmarshal(data, aux); err != nil {
		return err
	}

	u.UnwrapTokenRequest = &definition.UnwrapTokenRequest{
		RegistrationMomentumHeight: aux.RegistrationMomentumHeight,
		NetworkClass:               aux.NetworkClass,
		ChainId:                    aux.ChainId,
		TransactionHash:            aux.TransactionHash,
		LogIndex:                   aux.LogIndex,
		ToAddress:                  aux.ToAddress,
		TokenAddress:               aux.TokenAddress,
		TokenStandard:              aux.TokenStandard,
		Amount:                     common.StringToBigInt(aux.Amount),
		Signature:                  aux.Signature,
		Redeemed:                   aux.Redeemed,
		Revoked:                    aux.Revoked,
	}

	if aux.TokenInfo != nil {
		u.TokenInfo = aux.TokenInfo.FromTokenMarshal()
	}
	u.RedeemableIn = aux.RedeemableIn
	return nil
}

// UnwrapTokenRequestList is the paged response shape for
// unwrap-request enumeration. Count is the underlying total.
type UnwrapTokenRequestList struct {
	Count int                   `json:"count"`
	List  []*UnwrapTokenRequest `json:"list"`
}

// GetUnwrapTokenRequestByHashAndLog returns one unwrap request
// keyed by the foreign-chain transaction hash + log index that
// produced it, hydrated with token metadata and the
// RedeemableIn countdown.
//
// Errors:
//   - returns "token pair not found" (an errors.New value) when
//     the request's network/chain/token-address triple has no
//     registered TokenPair on the bridge — typically a stale
//     request whose pair has since been unregistered.
//   - propagates storage and momentum-read errors unchanged.
func (a *BridgeApi) GetUnwrapTokenRequestByHashAndLog(txHash types.Hash, logIndex uint32) (*UnwrapTokenRequest, error) {
	_, context, err := api.GetFrontierContext(a.chain, types.BridgeContract)
	if err != nil {
		return nil, err
	}
	request, err := definition.GetUnwrapTokenRequestByTxHashAndLog(context.Storage(), txHash, logIndex)
	if err != nil {
		return nil, err
	}
	token, err := a.getToken(request.TokenStandard)
	if err != nil {
		return nil, err
	}
	momentum, err := context.GetFrontierMomentum()
	if err != nil {
		return nil, err
	}
	tokenPair, err := implementation.CheckNetworkAndPairExist(context, request.NetworkClass, request.ChainId, request.TokenAddress)
	if err != nil {
		return nil, err
	}
	if tokenPair == nil {
		return nil, errors.New("token pair not found")
	}
	redeemableIn := a.getRedeemableIn(*request, *tokenPair, *momentum)
	unwrapRequest := &UnwrapTokenRequest{request, token, redeemableIn}

	return unwrapRequest, nil
}

// GetAllUnwrapTokenRequests returns one page of every recorded
// unwrap request, in the order returned by
// definition.GetUnwrapTokenRequests. Count is the raw underlying
// total before paging. Per-entry token lookups go through
// getToken: an unknown ZTS yields (nil, nil) and the entry is
// appended with TokenInfo == nil rather than skipped (the loop
// only `continue`s when getToken returns a non-nil error).
// CheckNetworkAndPairExist failures, by contrast, abort the
// whole call with errors.New("token pair not found") — they do
// not skip a single entry.
func (a *BridgeApi) GetAllUnwrapTokenRequests(pageIndex, pageSize uint32) (*UnwrapTokenRequestList, error) {
	_, context, err := api.GetFrontierContext(a.chain, types.BridgeContract)
	if err != nil {
		return nil, err
	}

	requests, err := definition.GetUnwrapTokenRequests(context.Storage())
	if err != nil {
		return nil, err
	}

	result := &UnwrapTokenRequestList{
		Count: len(requests),
		List:  make([]*UnwrapTokenRequest, 0),
	}

	start, end := api.GetRange(pageIndex, pageSize, uint32(len(requests)))
	momentum, err := context.GetFrontierMomentum()
	if err != nil {
		return nil, err
	}
	for i := start; i < end; i++ {
		token, err := a.getToken(requests[i].TokenStandard)
		if err != nil {
			continue
		}
		tokenPair, err := implementation.CheckNetworkAndPairExist(context, requests[i].NetworkClass, requests[i].ChainId, requests[i].TokenAddress)
		if err != nil {
			return nil, err
		}
		if tokenPair == nil {
			return nil, errors.New("token pair not found")
		}
		redeemableIn := a.getRedeemableIn(*requests[i], *tokenPair, *momentum)
		result.List = append(result.List, &UnwrapTokenRequest{requests[i], token, redeemableIn})
	}
	return result, nil
}

// GetAllUnwrapTokenRequestsByToAddress returns one page of unwrap
// requests filtered by ToAddress (string match against
// UnwrapTokenRequest.ToAddress.String()). Passing
// toAddress == "" yields the unfiltered set; when filtered, the
// matching subset is sorted by descending RegistrationMomentumHeight
// (newest first) before paging. Count is the filtered total.
func (a *BridgeApi) GetAllUnwrapTokenRequestsByToAddress(toAddress string, pageIndex, pageSize uint32) (*UnwrapTokenRequestList, error) {
	_, context, err := api.GetFrontierContext(a.chain, types.BridgeContract)
	if err != nil {
		return nil, err
	}

	requests, err := definition.GetUnwrapTokenRequests(context.Storage())
	if err != nil {
		return nil, err
	}

	result := &UnwrapTokenRequestList{
		List: make([]*UnwrapTokenRequest, 0),
	}
	specificRequests := make([]*definition.UnwrapTokenRequest, 0)
	if toAddress == "" {
		specificRequests = append(specificRequests, requests...)
	} else {
		for _, request := range requests {
			if request.ToAddress.String() == toAddress {
				specificRequests = append(specificRequests, request)
			}
		}
		sort.Slice(specificRequests[:], func(i, j int) bool {
			return specificRequests[i].RegistrationMomentumHeight > specificRequests[j].RegistrationMomentumHeight
		})

	}
	result.Count = len(specificRequests)
	start, end := api.GetRange(pageIndex, pageSize, uint32(len(specificRequests)))
	momentum, err := context.GetFrontierMomentum()
	if err != nil {
		return nil, err
	}
	for i := start; i < end; i++ {
		token, err := a.getToken(specificRequests[i].TokenStandard)
		if err != nil {
			continue
		}
		tokenPair, err := implementation.CheckNetworkAndPairExist(context, specificRequests[i].NetworkClass, specificRequests[i].ChainId, specificRequests[i].TokenAddress)
		if err != nil {
			return nil, err
		}
		if tokenPair == nil {
			return nil, errors.New("token pair not found")
		}
		redeemableIn := a.getRedeemableIn(*specificRequests[i], *tokenPair, *momentum)
		result.List = append(result.List, &UnwrapTokenRequest{specificRequests[i], token, redeemableIn})
	}
	return result, nil
}

// GetFeeTokenPair returns the ZtsFeesInfo record for the given
// token standard — the bridge's per-token fee schedule. Errors
// from the storage read propagate unchanged.
func (a *BridgeApi) GetFeeTokenPair(zts types.ZenonTokenStandard) (*definition.ZtsFeesInfo, error) {
	_, context, err := api.GetFrontierContext(a.chain, types.BridgeContract)
	if err != nil {
		return nil, err
	}
	return definition.GetZtsFeesInfoVariable(context.Storage(), zts)
}
