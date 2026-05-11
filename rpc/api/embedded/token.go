package embedded

import (
	"github.com/inconshreveable/log15"

	"github.com/zenon-network/go-zenon/chain"
	"github.com/zenon-network/go-zenon/common"
	"github.com/zenon-network/go-zenon/common/types"
	"github.com/zenon-network/go-zenon/consensus"
	"github.com/zenon-network/go-zenon/rpc/api"
	"github.com/zenon-network/go-zenon/vm/constants"
	"github.com/zenon-network/go-zenon/vm/embedded/definition"
	"github.com/zenon-network/go-zenon/zenon"
)

// TokenAPI serves read RPCs for the token registry — every
// ZTS-issued token's static metadata (name, symbol, supply, owner)
// and current totals. The type is named TokenAPI (uppercase API)
// for historical reasons; the constructor follows the
// New<Name>Api convention.
type TokenAPI struct {
	chain chain.Chain
	z     zenon.Zenon
	cs    consensus.Consensus
	log   log15.Logger
}

// NewTokenApi returns a TokenAPI bound to z's chain and consensus.
// The cs and z fields are stored for symmetry with sibling
// constructors in this package; the currently-exposed methods
// only use chain.
func NewTokenApi(z zenon.Zenon) *TokenAPI {
	return &TokenAPI{
		chain: z.Chain(),
		z:     z,
		cs:    z.Consensus(),
		log:   common.RPCLogger.New("module", "embedded_token_api"),
	}
}

// TokenList is the paged response shape for token enumeration.
// Count is the size of the full underlying list before paging.
type TokenList struct {
	Count int          `json:"count"`
	List  []*api.Token `json:"list"`
}

// GetAll returns one page of every registered token, converted from
// the on-chain definition.TokenInfo records into the RPC api.Token
// view (which stringifies *big.Int totals for JSON safety). Order
// is whatever definition.GetTokenInfoList returns — no additional
// sort is applied here. pageSize > api.RpcMaxPageSize is rejected
// with api.ErrPageSizeParamTooBig.
func (a *TokenAPI) GetAll(pageIndex, pageSize uint32) (*TokenList, error) {
	if pageSize > api.RpcMaxPageSize {
		return nil, api.ErrPageSizeParamTooBig
	}

	_, context, err := api.GetFrontierContext(a.chain, types.TokenContract)
	if err != nil {
		return nil, err
	}
	tokenListRaw, err := definition.GetTokenInfoList(context.Storage())
	if err != nil {
		return nil, err
	}
	tokenList := api.LedgerTokenInfosToRpc(tokenListRaw)
	start, end := api.GetRange(pageIndex, pageSize, uint32(len(tokenList)))
	return &TokenList{
		Count: len(tokenList),
		List:  tokenList[start:end],
	}, nil
}

// GetByOwner returns the page-sliced tokens whose Owner matches
// the supplied address. Filtering happens after the full registry
// read, so Count reflects the filtered total (not the registry
// size). pageSize > api.RpcMaxPageSize is rejected with
// api.ErrPageSizeParamTooBig.
func (a *TokenAPI) GetByOwner(owner types.Address, pageIndex, pageSize uint32) (*TokenList, error) {
	if pageSize > api.RpcMaxPageSize {
		return nil, api.ErrPageSizeParamTooBig
	}

	_, context, err := api.GetFrontierContext(a.chain, types.TokenContract)
	if err != nil {
		return nil, err
	}
	tokenListRaw, err := definition.GetTokenInfoList(context.Storage())
	if err != nil {
		return nil, err
	}
	tokenListUnfiltered := api.LedgerTokenInfosToRpc(tokenListRaw)

	tokenList := make([]*api.Token, 0)
	for _, tokenInfo := range tokenListUnfiltered {
		if tokenInfo.Owner == owner {
			tokenList = append(tokenList, tokenInfo)
		}
	}

	start, end := api.GetRange(pageIndex, pageSize, uint32(len(tokenList)))
	return &TokenList{
		Count: len(tokenList),
		List:  tokenList[start:end],
	}, nil
}

// GetByZts looks up a single token by its ZenonTokenStandard
// identifier. Returns (nil, nil) when no token is registered under
// zts — constants.ErrDataNonExistent from the storage read is
// mapped to a nil result so RPC callers can branch on existence
// without inspecting an error code.
func (a *TokenAPI) GetByZts(zts types.ZenonTokenStandard) (*api.Token, error) {
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
