package definition

import (
	"math/big"
	"strings"

	"github.com/pkg/errors"

	"github.com/zenon-network/go-zenon/common/db"
	"github.com/zenon-network/go-zenon/common/types"
	"github.com/zenon-network/go-zenon/vm/abi"
	"github.com/zenon-network/go-zenon/vm/constants"
)

// jsonToken is the canonical Solidity-shaped ABI for the Token
// contract: four methods (IssueToken, Mint, Burn, UpdateToken) and
// the per-token storage record (tokenInfo).
const (
	jsonToken = `
	[
		{"type":"function","name":"IssueToken","inputs":[{"name":"tokenName","type":"string"},{"name":"tokenSymbol","type":"string"},{"name":"tokenDomain","type":"string"},{"name":"totalSupply","type":"uint256"},{"name":"maxSupply","type":"uint256"},{"name":"decimals","type":"uint8"},{"name":"isMintable","type":"bool"},{"name":"isBurnable","type":"bool"},{"name":"isUtility","type":"bool"}]},
		{"type":"function","name":"Mint","inputs":[{"name":"tokenStandard","type":"tokenStandard"},{"name":"amount","type":"uint256"},{"name":"receiveAddress","type":"address"}]},
		{"type":"function","name":"Burn","inputs":[]},
		{"type":"function","name":"UpdateToken","inputs":[{"name":"tokenStandard","type":"tokenStandard"},{"name":"owner","type":"address"},{"name":"isMintable","type":"bool"},{"name":"isBurnable","type":"bool"}]},

		{"type":"variable","name":"tokenInfo","inputs":[
			{"name":"owner","type":"address"},
			{"name":"tokenName","type":"string"},
			{"name":"tokenSymbol","type":"string"},
			{"name":"tokenDomain","type":"string"},
			{"name":"totalSupply","type":"uint256"},
			{"name":"maxSupply","type":"uint256"},
			{"name":"decimals","type":"uint8"},
			{"name":"isMintable","type":"bool"},
			{"name":"isBurnable","type":"bool"},
			{"name":"isUtility","type":"bool"}]}
	]`

	// IssueMethodName names the new-token issuance method.
	IssueMethodName = "IssueToken"
	// MintMethodName names the per-token mint method.
	MintMethodName = "Mint"
	// BurnMethodName names the per-token burn method (caller's
	// balance is destroyed).
	BurnMethodName = "Burn"
	// UpdateTokenMethodName names the metadata-update method.
	UpdateTokenMethodName = "UpdateToken"

	tokenInfoVariableName = "tokenInfo"
)

// ABIToken is the parsed [abi.ABIContract] for the token contract.
var (
	ABIToken = abi.JSONToABIContract(strings.NewReader(jsonToken))

	// tokenInfoKeyPrefix namespaces per-token records.
	tokenInfoKeyPrefix = []byte{1}
)

// IssueParam is the call-shape struct for [IssueMethodName].
type IssueParam struct {
	TokenName   string
	TokenSymbol string
	TokenDomain string
	TotalSupply *big.Int
	MaxSupply   *big.Int
	Decimals    uint8
	IsMintable  bool
	IsBurnable  bool
	IsUtility   bool
}

// MintParam is the call-shape struct for [MintMethodName]: the
// target token, the amount, and the recipient address.
type MintParam struct {
	TokenStandard  types.ZenonTokenStandard
	Amount         *big.Int
	ReceiveAddress types.Address
}

// UpdateTokenParam is the call-shape struct for
// [UpdateTokenMethodName]: rotates owner and the burn/mint flags.
type UpdateTokenParam struct {
	TokenStandard types.ZenonTokenStandard
	Owner         types.Address
	IsMintable    bool
	IsBurnable    bool
}

// TokenInfo is the on-chain registration of one ZTS token: human
// metadata (name/symbol/domain/decimals), supply tracking
// (TotalSupply / MaxSupply), capability flags
// (IsMintable/IsBurnable/IsUtility), and the owning address. The
// TokenStandard field is derived from the storage key during
// decoding.
type TokenInfo struct {
	Owner       types.Address `json:"owner"`
	TokenName   string        `json:"tokenName"`
	TokenSymbol string        `json:"tokenSymbol"`
	TokenDomain string        `json:"tokenDomain"`
	TotalSupply *big.Int      `json:"totalSupply"`
	MaxSupply   *big.Int      `json:"maxSupply"`
	Decimals    uint8         `json:"decimals"`
	IsMintable  bool          `json:"isMintable"`
	// IsBurnable = true implies that anyone can burn the token.
	// The Owner can burn the token even if IsBurnable = false.
	IsBurnable bool `json:"isBurnable"`
	IsUtility  bool `json:"isUtility"`

	TokenStandard types.ZenonTokenStandard `json:"tokenStandard"`
}

// Save writes token into context's storage.
func (token *TokenInfo) Save(context db.DB) error {
	data, err := ABIToken.PackVariable(
		tokenInfoVariableName,
		token.Owner,
		token.TokenName,
		token.TokenSymbol,
		token.TokenDomain,
		token.TotalSupply,
		token.MaxSupply,
		token.Decimals,
		token.IsMintable,
		token.IsBurnable,
		token.IsUtility,
	)
	if err != nil {
		return err
	}
	return context.Put(
		getTokenInfoKey(token.TokenStandard),
		data,
	)
}

// getTokenInfoKey composes the storage key for one token record.
func getTokenInfoKey(ts types.ZenonTokenStandard) []byte {
	return append(tokenInfoKeyPrefix, ts.Bytes()...)
}

// isTokenInfoKey reports whether key belongs to the tokenInfo
// keyspace.
func isTokenInfoKey(key []byte) bool {
	return key[0] == tokenInfoKeyPrefix[0]
}

// unmarshalTokenInfoKey extracts the [types.ZenonTokenStandard]
// from a tokenInfo key.
func unmarshalTokenInfoKey(key []byte) (*types.ZenonTokenStandard, error) {
	if !isTokenInfoKey(key) {
		return nil, errors.Errorf("invalid key! Not token info key")
	}
	tokenStandard := new(types.ZenonTokenStandard)
	if err := tokenStandard.SetBytes(key[1:]); err != nil {
		return nil, err
	}
	return tokenStandard, nil
}

// parseTokenInfo decodes a (key, data) pair into a [TokenInfo].
// Returns [constants.ErrDataNonExistent] when data is empty.
func parseTokenInfo(key, data []byte) (*TokenInfo, error) {
	if len(data) > 0 {
		tokenStandard, err := unmarshalTokenInfoKey(key)
		if err != nil {
			return nil, err
		}
		tokenInfo := new(TokenInfo)
		tokenInfo.TokenStandard = *tokenStandard
		if err := ABIToken.UnpackVariable(tokenInfo, tokenInfoVariableName, data); err != nil {
			return nil, err
		}
		return tokenInfo, err
	} else {
		return nil, constants.ErrDataNonExistent
	}
}

// GetTokenInfo returns the token record for ts, or
// [constants.ErrDataNonExistent] if no such token is registered.
func GetTokenInfo(context db.DB, ts types.ZenonTokenStandard) (*TokenInfo, error) {
	key := getTokenInfoKey(ts)
	if data, err := context.Get(key); err != nil {
		return nil, err
	} else {
		return parseTokenInfo(key, data)
	}
}

// GetTokenInfoList enumerates every token record in iteration order.
func GetTokenInfoList(context db.DB) ([]*TokenInfo, error) {
	iterator := context.NewIterator(tokenInfoKeyPrefix)
	defer iterator.Release()
	tokenInfoList := make([]*TokenInfo, 0)
	for {
		if !iterator.Next() {
			if iterator.Error() != nil {
				return nil, iterator.Error()
			}
			break
		}

		if tokenInfo, err := parseTokenInfo(iterator.Key(), iterator.Value()); err == nil {
			tokenInfoList = append(tokenInfoList, tokenInfo)
		} else if err == constants.ErrDataNonExistent {
			continue
		} else {
			return nil, err
		}
	}
	return tokenInfoList, nil
}
