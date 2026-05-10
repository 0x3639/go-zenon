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

// BridgeApi is the "embedded.bridge" namespace — read access to
// cross-chain bridge state (orchestrator, networks, wraps, unwraps,
// security parameters, time-challenges).
type BridgeApi struct {
	chain chain.Chain
	log   log15.Logger
}

// NewBridgeApi constructs the bridge namespace handler.
func NewBridgeApi(z zenon.Zenon) *BridgeApi {
	return &BridgeApi{
		chain: z.Chain(),
		log:   common.RPCLogger.New("module", "embedded_bridge_api"),
	}
}

// GetBridgeInfo returns the global bridge configuration variable
// (orchestrator address, halted flag, etc.).
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

// GetSecurityInfo returns the bridge's security parameters
// (administrator addresses, soft / hard delays, time-challenges).
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

// GetOrchestratorInfo returns the bridge's orchestrator parameters
// (key-sign threshold, confirmations-to-finality, etc.). Thin wrapper over
// definition.GetOrchestratorInfoVariable.
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

// TimeChallengesList is part of the package's public API; see the surrounding code for usage.
type TimeChallengesList struct {
	Count int                             `json:"count"`
	List  []*definition.TimeChallengeInfo `json:"list"`
}

// GetTimeChallengesInfo returns the active time-challenge records for the
// four governance-gated bridge methods (NominateGuardians,
// ChangeTssECDSAPubKey, ChangeAdministrator, SetTokenPair). Composes
// per-method definition.GetTimeChallengeInfoVariable calls; nil entries
// are filtered out.
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

// GetNetworkInfo returns the configuration record for the bridge network
// identified by (networkClass, chainId), including its registered token
// pairs. Thin wrapper over definition.GetNetworkInfoVariable.
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

// GetAllNetworks returns every registered bridge network sliced to
// (pageIndex, pageSize). Composes definition.GetNetworkList with pagination;
// preserves the underlying storage iteration order (no explicit sort).
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

// NetworkInfoList is part of the package's public API; see the surrounding code for usage.
type NetworkInfoList struct {
	Count int                       `json:"count"`
	List  []*definition.NetworkInfo `json:"list"`
}

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

// WrapTokenRequest is part of the package's public API; see the surrounding code for usage.
type WrapTokenRequest struct {
	*definition.WrapTokenRequest
	TokenInfo               *api.Token `json:"token"`
	ConfirmationsToFinality uint64     `json:"confirmationsToFinality"`
}

// MarshalJSON forwards through the Marshal twin so big.Int fields render as decimal strings.
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

// UnmarshalJSON inflates the JSON wire form back into the in-memory receiver.
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

func (a *BridgeApi) getRedeemableIn(unwrapTokenRequest definition.UnwrapTokenRequest, tokenPair definition.TokenPair, momentum nom.Momentum) uint64 {
	var redeemableIn uint64
	if momentum.Height-unwrapTokenRequest.RegistrationMomentumHeight >= uint64(tokenPair.RedeemDelay) {
		redeemableIn = 0
	} else {
		redeemableIn = unwrapTokenRequest.RegistrationMomentumHeight + uint64(tokenPair.RedeemDelay) - momentum.Height
	}
	return redeemableIn
}

func (a *BridgeApi) getConfirmationsToFinality(wrapTokenRequest definition.WrapTokenRequest, confirmationsToFinality uint32, momentum nom.Momentum) (uint64, error) {
	var actualConfirmationsToFinality uint64
	if momentum.Height-wrapTokenRequest.CreationMomentumHeight >= uint64(confirmationsToFinality) {
		actualConfirmationsToFinality = 0
	} else {
		actualConfirmationsToFinality = wrapTokenRequest.CreationMomentumHeight + uint64(confirmationsToFinality) - momentum.Height
	}
	return actualConfirmationsToFinality, nil
}

// GetWrapTokenRequestById returns the wrap-request id, enriched with its
// resolved token info and remaining momentum confirmations to finality.
// Composes definition.GetWrapTokenRequestById +
// definition.GetOrchestratorInfoVariable + [getToken] +
// [getConfirmationsToFinality].
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

// WrapTokenRequestList is part of the package's public API; see the surrounding code for usage.
type WrapTokenRequestList struct {
	Count int                 `json:"count"`
	List  []*WrapTokenRequest `json:"list"`
}

// GetAllWrapTokenRequests returns every wrap request sliced to
// (pageIndex, pageSize), each enriched with its resolved token info and
// remaining momentum confirmations to finality. Composes
// definition.GetWrapTokenRequests with pagination + per-entry [getToken] +
// [getConfirmationsToFinality]; entries whose token or confirmation lookup
// fails are silently skipped (Count still reflects the unfiltered total).
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

// GetAllWrapTokenRequestsByToAddress returns wrap requests destined for
// toAddress sliced to (pageIndex, pageSize), each enriched as in
// [BridgeApi.GetAllWrapTokenRequests]. An empty toAddress short-circuits
// the filter and yields the unfiltered list. Composes
// definition.GetWrapTokenRequests with an in-memory ToAddress filter +
// per-entry [getToken] + [getConfirmationsToFinality]; failed token or
// confirmation lookups skip the entry silently.
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

// GetAllWrapTokenRequestsByToAddressNetworkClassAndChainId returns wrap
// requests filtered by (networkClass, chainId) and optionally toAddress
// (empty string disables that part of the filter), sliced to
// (pageIndex, pageSize) and enriched as in
// [BridgeApi.GetAllWrapTokenRequests]. Composes
// definition.GetWrapTokenRequests with an in-memory tri-key filter +
// per-entry [getToken] + [getConfirmationsToFinality].
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

// GetAllUnsignedWrapTokenRequests returns wrap requests still missing an
// orchestrator signature, reversed (newest-first by storage iteration order)
// and sliced to (pageIndex, pageSize). Composes
// definition.GetWrapTokenRequests with a Signature == "" filter +
// per-entry [getToken] + [getConfirmationsToFinality]; failed token or
// confirmation lookups skip the entry silently.
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

// UnwrapTokenRequest is part of the package's public API; see the surrounding code for usage.
type UnwrapTokenRequest struct {
	*definition.UnwrapTokenRequest
	TokenInfo    *api.Token `json:"token"`
	RedeemableIn uint64     `json:"redeemableIn"`
}

// MarshalJSON forwards through the Marshal twin so big.Int fields render as decimal strings.
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

// UnmarshalJSON inflates the JSON wire form back into the in-memory receiver.
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

// UnwrapTokenRequestList is part of the package's public API; see the surrounding code for usage.
type UnwrapTokenRequestList struct {
	Count int                   `json:"count"`
	List  []*UnwrapTokenRequest `json:"list"`
}

// GetUnwrapTokenRequestByHashAndLog returns the unwrap request keyed by
// (txHash, logIndex), enriched with its resolved token info and the
// remaining momentum delay until it becomes redeemable. Composes
// definition.GetUnwrapTokenRequestByTxHashAndLog +
// implementation.CheckNetworkAndPairExist + [getToken] + [getRedeemableIn];
// errors out (rather than returning nil) when the network/token pair is
// missing.
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

// GetAllUnwrapTokenRequests returns every unwrap request sliced to
// (pageIndex, pageSize), each enriched with token info and remaining
// redemption delay. Composes definition.GetUnwrapTokenRequests with
// pagination + per-entry [getToken] + implementation.CheckNetworkAndPairExist
// + [getRedeemableIn]; failed token lookups skip the entry, but a missing
// network/token pair errors the entire call.
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

// GetAllUnwrapTokenRequestsByToAddress returns unwrap requests destined for
// toAddress sliced to (pageIndex, pageSize), each enriched as in
// [BridgeApi.GetAllUnwrapTokenRequests]. An empty toAddress short-circuits
// the filter; otherwise matches are sorted by descending
// RegistrationMomentumHeight before pagination. Composes
// definition.GetUnwrapTokenRequests with the filter + sort +
// implementation.CheckNetworkAndPairExist + [getToken] + [getRedeemableIn].
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

// GetFeeTokenPair returns the accumulated bridge fees recorded for the
// given ZTS. Thin wrapper over definition.GetZtsFeesInfoVariable.
func (a *BridgeApi) GetFeeTokenPair(zts types.ZenonTokenStandard) (*definition.ZtsFeesInfo, error) {
	_, context, err := api.GetFrontierContext(a.chain, types.BridgeContract)
	if err != nil {
		return nil, err
	}
	return definition.GetZtsFeesInfoVariable(context.Storage(), zts)
}
