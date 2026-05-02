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

// jsonStake is the canonical Solidity-shaped ABI for the Stake
// contract: Stake/Cancel/CollectReward/Update plus the per-stake
// storage record (stakeInfo).
const (
	jsonStake = `
	[
		{"type":"function","name":"Stake","inputs":[{"name":"durationInSec", "type":"int64"}]},
		{"type":"function","name":"Cancel","inputs":[{"name":"id","type":"hash"}]},
		{"type":"function","name":"CollectReward","inputs":[]},
		{"type":"function","name":"Update", "inputs":[]},

		{"type":"variable", "name":"stakeInfo", "inputs":[
			{"name":"amount", "type":"uint256"},
			{"name":"weightedAmount", "type":"uint256"},
			{"name":"startTime", "type":"int64"},
			{"name":"revokeTime", "type":"int64"},
			{"name":"expirationTime", "type":"int64"}
		]}
	]`

	// StakeMethodName names the lock-and-stake method.
	StakeMethodName = "Stake"
	// CancelStakeMethodName names the cancel-and-refund method.
	CancelStakeMethodName = "Cancel"

	stakeInfoVariableName = "stakeInfo"
)

// ABIStake is the parsed [abi.ABIContract] for the stake contract.
var (
	ABIStake = abi.JSONToABIContract(strings.NewReader(jsonStake))

	// stakeInfoPrefix namespaces per-stake records.
	stakeInfoPrefix = []byte{1}
)

// StakeInfo is one stake record: how much ZNN was locked, the
// weight derived from the chosen duration, the wall-clock window
// the stake covers, the optional revoke timestamp, and the address
// + id derived from the storage key during decoding. WeightedAmount
// is the input to staking-reward distribution; longer-duration
// stakes weigh more per ZNN.
type StakeInfo struct {
	Amount         *big.Int      `json:"amount"`
	WeightedAmount *big.Int      `json:"weightedAmount"`
	StartTime      int64         `json:"startTime"`
	RevokeTime     int64         `json:"revokeTime"`
	ExpirationTime int64         `json:"expirationTime"`
	StakeAddress   types.Address `json:"stakeAddress"`
	Id             types.Hash    `json:"id"`
}

// Save writes stake into context's storage.
func (stake *StakeInfo) Save(context db.DB) error {
	return context.Put(
		getStakeInfoKey(stake.Id, stake.StakeAddress),
		ABIStake.PackVariablePanic(
			stakeInfoVariableName,
			stake.Amount,
			stake.WeightedAmount,
			stake.StartTime,
			stake.RevokeTime,
			stake.ExpirationTime,
		))
}

// Delete removes stake from context's storage.
func (stake *StakeInfo) Delete(context db.DB) error {
	return context.Delete(getStakeInfoKey(stake.Id, stake.StakeAddress))
}

// getStakeInfoKey composes the storage key
// (`stakeInfoPrefix || address || id`).
func getStakeInfoKey(id types.Hash, address types.Address) []byte {
	return append(append(stakeInfoPrefix, address.Bytes()...), id.Bytes()...)
}

// isStakeInfoKey reports whether key belongs to the stakeInfo
// keyspace.
func isStakeInfoKey(key []byte) bool {
	return key[0] == stakeInfoPrefix[0]
}

// unmarshalStakeInfoKey extracts (id, address) from a stakeInfo
// key.
func unmarshalStakeInfoKey(key []byte) (*types.Hash, *types.Address, error) {
	if !isStakeInfoKey(key) {
		return nil, nil, errors.Errorf("invalid key! Not stake info key")
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

// parseStakeInfo decodes a (key, data) pair into a [StakeInfo].
// Returns [constants.ErrDataNonExistent] when data is empty.
func parseStakeInfo(key []byte, data []byte) (*StakeInfo, error) {
	if len(data) > 0 {
		entry := new(StakeInfo)
		err := ABIStake.UnpackVariable(entry, stakeInfoVariableName, data)
		if err != nil {
			return nil, err
		}

		id, address, err := unmarshalStakeInfoKey(key)
		if err != nil {
			return nil, err
		}
		entry.Id = *id
		entry.StakeAddress = *address
		return entry, err
	} else {
		return nil, constants.ErrDataNonExistent
	}
}

// GetStakeInfo returns the stake record for (id, address).
func GetStakeInfo(context db.DB, id types.Hash, address types.Address) (*StakeInfo, error) {
	key := getStakeInfoKey(id, address)
	if data, err := context.Get(key); err != nil {
		return nil, err
	} else {
		return parseStakeInfo(key, data)
	}
}

// IterateStakeEntries calls f for every stake record. f returning a
// non-nil error stops iteration; data-non-existent entries are
// silently skipped.
func IterateStakeEntries(context db.DB, f func(*StakeInfo) error) error {
	iterator := context.NewIterator(stakeInfoPrefix)
	defer iterator.Release()

	for {
		if !iterator.Next() {
			if iterator.Error() != nil {
				return iterator.Error()
			}
			break
		}

		if stakeInfo, err := parseStakeInfo(iterator.Key(), iterator.Value()); err == nil {
			if err := f(stakeInfo); err != nil {
				return err
			}
		} else if err == constants.ErrDataNonExistent {
		} else {
			return err
		}
	}
	return nil
}

// GetStakeListByAddress returns all *active* stake entries for an
// address (RevokeTime == 0), the summed Amount, and the summed
// WeightedAmount. Used by the staking-reward distribution.
func GetStakeListByAddress(context db.DB, address types.Address) ([]*StakeInfo, *big.Int, *big.Int, error) {
	total := big.NewInt(0)
	weighted := big.NewInt(0)
	list := make([]*StakeInfo, 0)

	err := IterateStakeEntries(context, func(stakeInfo *StakeInfo) error {
		if stakeInfo.RevokeTime == 0 && stakeInfo.StakeAddress == address {
			list = append(list, stakeInfo)
			total.Add(total, stakeInfo.Amount)
			weighted.Add(weighted, stakeInfo.WeightedAmount)
		}
		return nil
	})
	if err != nil {
		return nil, nil, nil, err
	} else {
		return list, total, weighted, nil
	}
}

// StakeByExpirationTime sorts a slice of [StakeInfo] by ascending
// ExpirationTime, breaking ties on Id. Implements [sort.Interface].
type StakeByExpirationTime []*StakeInfo

// Len returns the number of stake entries in a.
func (a StakeByExpirationTime) Len() int { return len(a) }

// Swap exchanges entries i and j.
func (a StakeByExpirationTime) Swap(i, j int) { a[i], a[j] = a[j], a[i] }

// Less reports whether entry i sorts before entry j.
func (a StakeByExpirationTime) Less(i, j int) bool {
	if a[i].ExpirationTime == a[j].ExpirationTime {
		return a[i].Id.String() < a[j].Id.String()
	}
	return a[i].ExpirationTime < a[j].ExpirationTime
}
