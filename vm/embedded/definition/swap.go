package definition

import (
	"math/big"
	"strings"

	"github.com/zenon-network/go-zenon/common/db"
	"github.com/zenon-network/go-zenon/common/types"
	"github.com/zenon-network/go-zenon/vm/abi"
	"github.com/zenon-network/go-zenon/vm/constants"
)

// jsonSwap is the canonical Solidity-shaped ABI for the Swap
// contract: one method (RetrieveAssets) and one storage record
// shape (swapEntry).
const (
	jsonSwap = `
	[
		{"type":"function","name":"RetrieveAssets", "inputs":[{"name":"publicKey","type":"string"},{"name":"signature","type":"string"}]},
		{"type":"variable","name":"swapEntry", "inputs":[
			{"name":"znn","type":"uint256"}, 
			{"name":"qsr","type":"uint256"}
		]}
	]`

	// RetrieveAssetsMethodName names the legacy-claim retrieval
	// method.
	RetrieveAssetsMethodName = "RetrieveAssets"

	// swapEntryVariableName is the storage variable name used to
	// (de)encode [SwapAssets] records.
	swapEntryVariableName = "swapEntry"
)

// ABISwap is the parsed [abi.ABIContract] for the Swap contract.
var (
	ABISwap = abi.JSONToABIContract(strings.NewReader(jsonSwap))
)

// ParamRetrieveAssets is the call-shape struct for
// [RetrieveAssetsMethodName] — the legacy-chain public key and the
// secp256k1 signature proving its possession.
type ParamRetrieveAssets struct {
	PublicKey string
	Signature string
}

// SwapAssets is the on-chain genesis claim for one legacy-chain
// key id: how much ZNN and QSR the legacy holder may redeem.
// Records are keyed by KeyIdHash (the hash of the legacy key id).
type SwapAssets struct {
	KeyIdHash types.Hash `json:"keyIdHash"`
	Znn       *big.Int   `json:"znn"`
	Qsr       *big.Int   `json:"qsr"`
}

// Save writes assets into context's storage.
func (assets *SwapAssets) Save(context db.DB) error {
	data, err := ABISwap.PackVariable(
		swapEntryVariableName,
		assets.Znn,
		assets.Qsr)
	if err != nil {
		return err
	}
	return context.Put(getSwapAssetsKey(assets.KeyIdHash), data)
}

// getSwapAssetsKey returns the database key holding the swap entry
// for keyIdHash. Note: swap uses a flat key namespace (no prefix
// byte) because it is the only table in the contract's storage.
func getSwapAssetsKey(keyIdHash types.Hash) []byte {
	return keyIdHash[:]
}

// parseSwapAssets decodes data into a [SwapAssets] and pins
// KeyIdHash from key. Returns [constants.ErrDataNonExistent] when
// data is empty.
func parseSwapAssets(data, key []byte) (*SwapAssets, error) {
	if len(data) > 0 {
		dataVar := new(SwapAssets)
		if err := ABISwap.UnpackVariable(dataVar, swapEntryVariableName, data); err != nil {
			return nil, err
		}
		if err := dataVar.KeyIdHash.SetBytes(key); err != nil {
			return nil, err
		}
		return dataVar, nil
	} else {
		return nil, constants.ErrDataNonExistent
	}
}

// GetSwapAssetsByKeyIdHash returns the swap entry registered for
// keyIdHash, or [constants.ErrDataNonExistent] if none is.
func GetSwapAssetsByKeyIdHash(context db.DB, keyIdHash types.Hash) (*SwapAssets, error) {
	key := getSwapAssetsKey(keyIdHash)
	if data, err := context.Get(key); err != nil {
		return nil, err
	} else {
		return parseSwapAssets(data, key)
	}
}

// GetSwapAssets enumerates every swap entry in storage in iteration
// order.
func GetSwapAssets(context db.DB) ([]*SwapAssets, error) {
	iterator := context.NewIterator([]byte{})
	defer iterator.Release()
	list := make([]*SwapAssets, 0)

	for {
		if !iterator.Next() {
			if iterator.Error() != nil {
				return nil, iterator.Error()
			}
			break
		}
		if info, err := parseSwapAssets(iterator.Value(), iterator.Key()); err == nil && info != nil {
			list = append(list, info)
		} else {
			return nil, err
		}
	}

	return list, nil
}
