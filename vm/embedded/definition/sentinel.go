package definition

import (
	"math/big"
	"strings"

	"github.com/zenon-network/go-zenon/common"
	"github.com/zenon-network/go-zenon/common/db"
	"github.com/zenon-network/go-zenon/common/types"
	"github.com/zenon-network/go-zenon/vm/abi"
)

// jsonSentinel is the canonical Solidity-shaped ABI for the
// Sentinel contract. The contract shares the common deposit /
// withdraw / collect-reward methods (defined in common.go) and
// adds Register / Revoke / Update.
const (
	jsonSentinel = `
	[
		{"type":"function","name":"DepositQsr","inputs":[]},
		{"type":"function","name":"WithdrawQsr","inputs":[]},
		{"type":"function","name":"Register","inputs":[]},
		{"type":"function","name":"Revoke","inputs":[]},
		{"type":"function","name":"Update", "inputs":[]},
		{"type":"function","name":"CollectReward","inputs":[]},

		{"type":"variable","name":"sentinelInfo","inputs":[
			{"name":"owner","type":"address"},
			{"name":"registrationTimestamp","type":"int64"},
			{"name":"revokeTimestamp","type":"int64"},
			{"name":"znnAmount","type":"uint256"},
			{"name":"qsrAmount","type":"uint256"}]}
	]`

	// RegisterSentinelMethodName names the sentinel-registration
	// method.
	RegisterSentinelMethodName = "Register"
	// RevokeSentinelMethodName names the sentinel-revocation
	// method.
	RevokeSentinelMethodName = "Revoke"

	sentinelInfoVariableName = "sentinelInfo"
)

// ABISentinel is the parsed [abi.ABIContract] for the sentinel
// contract.
var (
	ABISentinel = abi.JSONToABIContract(strings.NewReader(jsonSentinel))
)

// Storage prefix; index 0 is reserved by the storage decorator.
const (
	_ byte = iota
	sentinelInfoPrefix
)

// SentinelInfoKey carries just the owner field — the key half of a
// [SentinelInfo] record. Used internally to compute the storage
// key without instantiating the full record.
type SentinelInfoKey struct {
	Owner types.Address `json:"owner"`
}

// SentinelInfo is the on-chain registration of one sentinel: the
// owner's address, the wall-clock timestamps marking registration
// and (optional) revocation, and the locked stakes (ZNN + QSR).
// After revocation the amounts are zeroed and the record stays as
// a tombstone until the corresponding ZNN/QSR refunds clear.
type SentinelInfo struct {
	SentinelInfoKey
	RegistrationTimestamp int64    `json:"registrationTimestamp"`
	RevokeTimestamp       int64    `json:"revokeTimestamp"`
	ZnnAmount             *big.Int `json:"znnAmount"`
	QsrAmount             *big.Int `json:"qsrAmount"`
}

// Save writes sentinel into context's storage. Panics on write
// failure.
func (sentinel *SentinelInfo) Save(context db.DB) {
	common.DealWithErr(context.Put(sentinel.Key(), sentinel.Data()))
}

// Delete removes sentinel from context's storage.
func (sentinel *SentinelInfo) Delete(context db.DB) {
	common.DealWithErr(context.Delete(sentinel.Key()))
}

// Data returns the sentinel ABI-encoded for storage.
func (sentinel *SentinelInfo) Data() []byte {
	return ABISentinel.PackVariablePanic(
		sentinelInfoVariableName,
		sentinel.Owner,
		sentinel.RegistrationTimestamp,
		sentinel.RevokeTimestamp,
		sentinel.ZnnAmount,
		sentinel.QsrAmount)
}

// Key returns the database key for this sentinel
// (`sentinelInfoPrefix || owner`).
func (sentinel *SentinelInfoKey) Key() []byte {
	return common.JoinBytes([]byte{sentinelInfoPrefix}, sentinel.Owner.Bytes())
}

// parseSentinelInfo decodes a sentinel record from data. Panics on
// malformed input.
func parseSentinelInfo(data []byte) *SentinelInfo {
	sentinel := new(SentinelInfo)
	ABISentinel.UnpackVariablePanic(sentinel, sentinelInfoVariableName, data)
	return sentinel
}

// GetSentinelInfoByOwner returns the sentinel registered to
// address, or nil if none is.
func GetSentinelInfoByOwner(context db.DB, address types.Address) *SentinelInfo {
	key := (&SentinelInfoKey{Owner: address}).Key()
	data, err := context.Get(key)
	common.DealWithErr(err)
	if len(data) == 0 {
		return nil
	} else {
		return parseSentinelInfo(data)
	}
}

// GetAllSentinelInfo enumerates every sentinel record in iteration
// order.
func GetAllSentinelInfo(context db.DB) []*SentinelInfo {
	iterator := context.NewIterator([]byte{sentinelInfoPrefix})
	defer iterator.Release()

	sentinelInfoList := make([]*SentinelInfo, 0)
	for {
		if !iterator.Next() {
			common.DealWithErr(iterator.Error())
			break
		}
		sentinelInfoList = append(sentinelInfoList, parseSentinelInfo(iterator.Value()))
	}
	return sentinelInfoList
}

// IterateSentinelEntries calls f for every sentinel record,
// stopping early if f returns a non-nil error.
func IterateSentinelEntries(context db.DB, f func(*SentinelInfo) error) error {
	iterator := context.NewIterator([]byte{sentinelInfoPrefix})
	defer iterator.Release()

	for {
		if !iterator.Next() {
			common.DealWithErr(iterator.Error())
			break
		}

		sentinelInfo := parseSentinelInfo(iterator.Value())
		if err := f(sentinelInfo); err != nil {
			return err
		}
	}
	return nil
}
