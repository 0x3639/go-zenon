package definition

import (
	"math/big"
	"strings"

	"github.com/pkg/errors"

	"github.com/zenon-network/go-zenon/common"
	"github.com/zenon-network/go-zenon/common/db"
	"github.com/zenon-network/go-zenon/common/types"
	"github.com/zenon-network/go-zenon/vm/abi"
	"github.com/zenon-network/go-zenon/vm/constants"
)

// jsonPlasma is the canonical Solidity-shaped ABI for the Plasma
// contract: two methods (Fuse, CancelFuse) and two storage record
// shapes (fusionInfo per-fusion, fusedAmount per-beneficiary
// summary).
const (
	jsonPlasma = `
	[
		{"type":"function","name":"Fuse", "inputs":[
			{"name":"address","type":"address"}
		]},
		{"type":"function","name":"CancelFuse","inputs":[
			{"name":"id","type":"hash"}
		]},

		{"type":"variable","name":"fusionInfo","inputs":[
			{"name":"amount","type":"uint256"},
			{"name":"expirationHeight","type":"uint64"},
			{"name":"beneficiary","type":"address"}
		]},
		{"type":"variable","name":"fusedAmount","inputs":[
			{"name":"amount","type":"uint256"}
		]}
	]`

	// FuseMethodName names the QSR-fusing method.
	FuseMethodName = "Fuse"
	// CancelFuseMethodName names the un-fuse / withdraw method.
	CancelFuseMethodName = "CancelFuse"

	variableNameFusionInfo  = "fusionInfo"
	variableNameFusedAmount = "fusedAmount"
)

// ABIPlasma is the parsed [abi.ABIContract] for the plasma contract.
var (
	ABIPlasma = abi.JSONToABIContract(strings.NewReader(jsonPlasma))

	// fusionInfoKeyPrefix namespaces per-fusion records keyed by
	// (owner, id).
	fusionInfoKeyPrefix = []byte{1}
	// fusedAmountKeyPrefix namespaces per-beneficiary cumulative
	// fused amounts.
	fusedAmountKeyPrefix = []byte{2}
)

// FusionInfo is one fusion record: an owner has locked Amount QSR
// to Beneficiary (granting plasma to that address) until
// ExpirationHeight. Id distinguishes multiple fusions from the
// same owner.
type FusionInfo struct {
	Owner            types.Address `json:"owner"`
	Id               types.Hash    `json:"id"`
	Amount           *big.Int      `json:"amount"`
	ExpirationHeight uint64        `json:"withdrawHeight"`
	Beneficiary      types.Address `json:"beneficiaryAddress"`
}

// Save writes entry into context's storage under
// (owner, id).
func (entry *FusionInfo) Save(context db.DB) error {
	data, err := ABIPlasma.PackVariable(
		variableNameFusionInfo,
		entry.Amount,
		entry.ExpirationHeight,
		entry.Beneficiary,
	)
	if err != nil {
		return err
	}
	return context.Put(getFusionInfoKey(entry.Owner, entry.Id), data)
}

// Delete removes entry from context's storage.
func (entry *FusionInfo) Delete(context db.DB) error {
	return context.Delete(getFusionInfoKey(entry.Owner, entry.Id))
}

// getFusionInfoKey composes the storage key for one fusion record.
func getFusionInfoKey(addr types.Address, hash types.Hash) []byte {
	return common.JoinBytes(fusionInfoKeyPrefix, addr.Bytes(), hash.Bytes())
}

// isFusionInfoKey reports whether key belongs to the fusionInfo
// keyspace.
func isFusionInfoKey(key []byte) bool {
	return key[0] == fusionInfoKeyPrefix[0]
}

// unmarshalFusionInfoKey extracts (id, owner) from a fusionInfo
// key. Returns an error when key is not a fusionInfo key.
func unmarshalFusionInfoKey(key []byte) (*types.Hash, *types.Address, error) {
	if !isFusionInfoKey(key) {
		return nil, nil, errors.Errorf("invalid key! Not fusion info key")
	}
	h := new(types.Hash)
	err := h.SetBytes(key[1+types.AddressSize:])
	if err != nil {
		return nil, nil, err
	}

	addr := new(types.Address)
	err = addr.SetBytes(key[1 : 1+types.AddressSize])
	if err != nil {
		return nil, nil, err
	}

	return h, addr, nil
}

// parseFusionInfo decodes a (key, data) pair into a [FusionInfo].
// Returns [constants.ErrDataNonExistent] when data is empty.
func parseFusionInfo(key, data []byte) (*FusionInfo, error) {
	if len(data) > 0 {
		info := new(FusionInfo)
		if err := ABIPlasma.UnpackVariable(info, variableNameFusionInfo, data); err != nil {
			return nil, err
		}
		id, owner, err := unmarshalFusionInfoKey(key)
		if err != nil {
			return nil, err
		}
		info.Owner = *owner
		info.Id = *id
		return info, nil
	} else {
		return nil, constants.ErrDataNonExistent
	}
}

// GetFusionInfo returns the fusion record for (owner, id).
func GetFusionInfo(context db.DB, owner types.Address, id types.Hash) (*FusionInfo, error) {
	key := getFusionInfoKey(owner, id)
	if data, err := context.Get(key); err != nil {
		return nil, err
	} else {
		return parseFusionInfo(key, data)
	}
}

// GetFusionInfoListByOwner returns every fusion record for owner
// plus the summed Amount across them.
func GetFusionInfoListByOwner(context db.DB, owner types.Address) ([]*FusionInfo, *big.Int, error) {
	fusedAmount := big.NewInt(0)
	iterator := context.NewIterator(common.JoinBytes(fusionInfoKeyPrefix, owner.Bytes()))
	defer iterator.Release()
	list := make([]*FusionInfo, 0)
	for {
		if !iterator.Next() {
			if iterator.Error() != nil {
				return nil, nil, iterator.Error()
			}
			break
		}

		if fusionInfo, err := parseFusionInfo(iterator.Key(), iterator.Value()); err == nil {
			list = append(list, fusionInfo)
			fusedAmount.Add(fusedAmount, fusionInfo.Amount)
		} else if err == constants.ErrDataNonExistent {
			continue
		} else {
			return nil, nil, err
		}
	}
	return list, fusedAmount, nil
}

// FusedAmount is the per-beneficiary cumulative fused-QSR record:
// the total a particular address has been granted across all
// fusions. The plasma layer reads this directly to compute
// available plasma without iterating fusions.
type FusedAmount struct {
	Beneficiary types.Address
	Amount      *big.Int
}

// Save writes entry into context's storage.
func (entry *FusedAmount) Save(context db.DB) error {
	data, err := ABIPlasma.PackVariable(
		variableNameFusedAmount,
		entry.Amount,
	)
	if err != nil {
		return err
	}
	return context.Put(getFusedAmountKey(entry.Beneficiary), data)
}

// Delete removes entry from context's storage.
func (entry *FusedAmount) Delete(context db.DB) error {
	return context.Delete(getFusedAmountKey(entry.Beneficiary))
}

// getFusedAmountKey composes the storage key for a per-beneficiary
// cumulative record.
func getFusedAmountKey(beneficiary types.Address) []byte {
	return common.JoinBytes(fusedAmountKeyPrefix, beneficiary.Bytes())
}

// isFusedAmountKey reports whether key belongs to the fusedAmount
// keyspace.
func isFusedAmountKey(key []byte) bool {
	return key[0] == fusedAmountKeyPrefix[0]
}

// unmarshalFusedAmountKey extracts beneficiary from a fusedAmount
// key.
func unmarshalFusedAmountKey(key []byte) (*types.Address, error) {
	if !isFusedAmountKey(key) {
		return nil, errors.Errorf("invalid key! Not fused amount key")
	}
	addr := new(types.Address)
	if err := addr.SetBytes(key[1:]); err != nil {
		return nil, err
	}

	return addr, nil
}

// parseFusedAmount decodes a (key, data) pair into a [FusedAmount].
// Returns [constants.ErrDataNonExistent] when data is empty.
func parseFusedAmount(key, data []byte) (*FusedAmount, error) {
	if len(data) > 0 {
		info := new(FusedAmount)
		if err := ABIPlasma.UnpackVariable(info, variableNameFusedAmount, data); err != nil {
			return nil, err
		}
		beneficiary, err := unmarshalFusedAmountKey(key)
		if err != nil {
			return nil, err
		}
		info.Beneficiary = *beneficiary
		return info, nil
	} else {
		return nil, constants.ErrDataNonExistent
	}
}

// GetFusedAmount returns the cumulative fused amount for
// beneficiary, or zero when no fusions have ever named it (the
// caller does not need to special-case absence).
func GetFusedAmount(context db.DB, beneficiary types.Address) (*FusedAmount, error) {
	key := getFusedAmountKey(beneficiary)
	if data, err := context.Get(key); err != nil {
		return nil, err
	} else {
		amount, err := parseFusedAmount(key, data)
		if err == constants.ErrDataNonExistent {
			return &FusedAmount{
				Beneficiary: beneficiary,
				Amount:      big.NewInt(0),
			}, nil
		}
		return amount, err
	}
}
