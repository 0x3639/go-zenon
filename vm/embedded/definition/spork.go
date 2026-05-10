package definition

import (
	"strings"

	"github.com/zenon-network/go-zenon/common"
	"github.com/zenon-network/go-zenon/common/db"
	"github.com/zenon-network/go-zenon/common/types"
	"github.com/zenon-network/go-zenon/vm/abi"
)

// jsonSpork is the canonical Solidity-shaped ABI for the Spork
// contract: two methods (CreateSpork, ActivateSpork) and one storage
// record shape (sporkInfo).
const (
	jsonSpork = `
	[
		{"type":"function","name":"CreateSpork","inputs":[{"name":"name","type":"string"},{"name":"description","type":"string"}]},
		{"type":"function","name":"ActivateSpork","inputs":[{"name":"id","type":"hash"}]},

		{"type":"variable", "name":"sporkInfo", "inputs":[
			{"name":"id", "type":"hash"},
			{"name":"name", "type":"string"},
			{"name":"description", "type":"string"},
			{"name":"activated", "type": "bool"},
			{"name":"enforcementHeight", "type": "uint64"}
		]}
	]`

	// SporkCreateMethodName names the spork-creation method.
	SporkCreateMethodName = "CreateSpork"
	// SporkActivateMethodName names the spork-activation method.
	SporkActivateMethodName = "ActivateSpork"

	// sporkInfoVariableName is the storage variable name used to
	// (de)encode [Spork] records.
	sporkInfoVariableName = "sporkInfo"
)

// ABISpork is the parsed [abi.ABIContract] for the Spork contract.
//
// CommunitySporkAddressStartHeight / CommunitySporkAddressEndHeight
// bracket the window in which the [types.CommunitySporkAddress]
// transitional spork-controlling address is allowed to operate.
var (
	// ABISpork is the abi definition of the Spork contract.
	ABISpork = abi.JSONToABIContract(strings.NewReader(jsonSpork))

	// CommunitySporkAddressStartHeight is the momentum height at
	// which the community spork address becomes valid. Targeting
	// 2025-04-16 12:00:00 UTC.
	CommunitySporkAddressStartHeight uint64 = 10109240
	// CommunitySporkAddressEndHeight is the momentum height past
	// which the community spork address is no longer accepted.
	// Targeting 2026-04-16 12:00:00 UTC.
	CommunitySporkAddressEndHeight uint64 = 13243712
)

// Single-byte storage prefixes used by the Spork contract. Index 0
// is intentionally skipped (reserved by the storage decorator).
const (
	_ byte = iota
	// sporkInfoPrefix namespaces per-spork records keyed by id.
	sporkInfoPrefix
)

// Spork is the on-chain representation of one protocol upgrade:
// id, human-readable name and description, and the activation
// state. Once activated, EnforcementHeight is set to the activation
// momentum height plus [constants.SporkMinHeightDelay].
type Spork struct {
	Id          types.Hash `json:"id"`
	Name        string     `json:"name"`
	Description string     `json:"description"`

	// If the spork is active, Activated = true and
	// EnforcementHeight = activation momentum height + HeightDelay.
	Activated         bool   `json:"activated"`
	EnforcementHeight uint64 `json:"enforcementHeight"`
}

// Save writes spork into context's storage under its keyspace.
// Panics through [common.DealWithErr] on write failure.
func (spork *Spork) Save(context db.DB) {
	common.DealWithErr(context.Put(spork.Key(), spork.Data()))
}

// Data returns spork ABI-encoded as the `sporkInfo` storage
// variable.
func (spork *Spork) Data() []byte {
	return ABISpork.PackVariablePanic(
		sporkInfoVariableName,
		spork.Id,
		spork.Name,
		spork.Description,
		spork.Activated,
		spork.EnforcementHeight)
}

// Key returns the database key holding spork (`sporkInfoPrefix || id`).
func (spork *Spork) Key() []byte {
	return common.JoinBytes([]byte{sporkInfoPrefix}, spork.Id.Bytes())
}

// parseSporkInfo decodes data (an ABI-encoded sporkInfo record) into
// a [*Spork]. Panics on malformed input.
func parseSporkInfo(data []byte) *Spork {
	spork := new(Spork)
	ABISpork.UnpackVariablePanic(spork, sporkInfoVariableName, data)
	return spork
}

// GetSporkInfoById returns the spork record for id, or nil if no
// such spork is stored. Panics on storage I/O failure.
func GetSporkInfoById(context db.DB, id types.Hash) *Spork {
	spork := new(Spork)
	spork.Id = id
	key := spork.Key()
	data, err := context.Get(key)
	common.DealWithErr(err)
	if len(data) == 0 {
		return nil
	} else {
		return parseSporkInfo(data)
	}
}

// GetAllSporks enumerates every spork record in iteration order.
// Used by the chain layer's spork-enforcement check.
func GetAllSporks(context db.DB) []*Spork {
	iterator := context.NewIterator([]byte{sporkInfoPrefix})
	defer iterator.Release()

	sporks := make([]*Spork, 0)
	for {
		if !iterator.Next() {
			common.DealWithErr(iterator.Error())
			break
		}
		spork := parseSporkInfo(iterator.Value())
		sporks = append(sporks, spork)
	}
	return sporks
}
