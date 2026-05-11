package embedded

import (
	"encoding/hex"
	"encoding/json"
	"math/big"

	"github.com/inconshreveable/log15"

	"github.com/zenon-network/go-zenon/chain"
	"github.com/zenon-network/go-zenon/common"
	"github.com/zenon-network/go-zenon/common/types"
	"github.com/zenon-network/go-zenon/consensus"
	"github.com/zenon-network/go-zenon/rpc/api"
	"github.com/zenon-network/go-zenon/vm/constants"
	"github.com/zenon-network/go-zenon/vm/embedded/definition"
	"github.com/zenon-network/go-zenon/vm/embedded/implementation"
	"github.com/zenon-network/go-zenon/zenon"
)

// SwapApi serves read RPCs for the legacy swap contract — the
// mechanism that migrated legacy Zenon balances (keyed by hashed
// public keys) and legacy pillar registrations onto the
// Network of Momentum.
type SwapApi struct {
	chain     chain.Chain
	consensus consensus.Consensus
	log       log15.Logger
}

// NewSwapApi returns a SwapApi bound to z's chain and consensus.
// The consensus reader is used to compute the current epoch when
// applying per-epoch decay to a swap entry's claimable amounts.
func NewSwapApi(z zenon.Zenon) *SwapApi {
	return &SwapApi{
		chain:     z.Chain(),
		consensus: z.Consensus(),
		log:       common.RPCLogger.New("module", "rpc_api/embedded_swap_api"),
	}
}

// SwapAssetEntry is the RPC view of a single swap claim:
// the hashed legacy key id and the post-decay ZNN/QSR amounts the
// claimant can still redeem.
type SwapAssetEntry struct {
	KeyIdHash string   `json:"keyIdHash"`
	Znn       *big.Int `json:"znn"`
	Qsr       *big.Int `json:"qsr"`
}

// SwapAssetEntryMarshal mirrors SwapAssetEntry with the *big.Int
// amounts encoded as decimal strings for JSON precision safety.
type SwapAssetEntryMarshal struct {
	KeyIdHash string `json:"keyIdHash"`
	Znn       string `json:"znn"`
	Qsr       string `json:"qsr"`
}

// ToSwapAssetEntryMarshal converts s into its string-amount wire form.
func (s *SwapAssetEntry) ToSwapAssetEntryMarshal() *SwapAssetEntryMarshal {
	aux := &SwapAssetEntryMarshal{
		KeyIdHash: s.KeyIdHash,
		Znn:       s.Znn.String(),
		Qsr:       s.Qsr.String(),
	}

	return aux
}

// MarshalJSON renders s through SwapAssetEntryMarshal so amounts
// are emitted as decimal strings.
func (s *SwapAssetEntry) MarshalJSON() ([]byte, error) {
	return json.Marshal(s.ToSwapAssetEntryMarshal())
}

// UnmarshalJSON reads a SwapAssetEntryMarshal payload and
// rehydrates the *big.Int amounts via common.StringToBigInt.
func (s *SwapAssetEntry) UnmarshalJSON(data []byte) error {
	aux := new(SwapAssetEntryMarshal)
	if err := json.Unmarshal(data, aux); err != nil {
		return err
	}
	s.KeyIdHash = aux.KeyIdHash
	s.Znn = common.StringToBigInt(aux.Znn)
	s.Qsr = common.StringToBigInt(aux.Qsr)
	return nil
}

// SwapAssetEntrySimple is the RPC view of a swap claim's
// post-decay amounts without the key-id-hash label — used as the
// value type in GetAssets's hash-keyed map.
type SwapAssetEntrySimple struct {
	Znn *big.Int `json:"znn"`
	Qsr *big.Int `json:"qsr"`
}

// SwapAssetEntrySimpleMarshal mirrors SwapAssetEntrySimple with
// *big.Int amounts encoded as decimal strings for JSON precision
// safety.
type SwapAssetEntrySimpleMarshal struct {
	Znn string `json:"znn"`
	Qsr string `json:"qsr"`
}

// ToSwapAssetEntrySimpleMarshal converts s into its string-amount
// wire form.
func (s *SwapAssetEntrySimple) ToSwapAssetEntrySimpleMarshal() *SwapAssetEntrySimpleMarshal {
	aux := &SwapAssetEntrySimpleMarshal{
		Znn: s.Znn.String(),
		Qsr: s.Qsr.String(),
	}

	return aux
}

// MarshalJSON renders s through SwapAssetEntrySimpleMarshal so
// amounts are emitted as decimal strings.
func (s *SwapAssetEntrySimple) MarshalJSON() ([]byte, error) {
	return json.Marshal(s.ToSwapAssetEntrySimpleMarshal())
}

// UnmarshalJSON reads a SwapAssetEntrySimpleMarshal payload and
// rehydrates the *big.Int amounts via common.StringToBigInt.
func (s *SwapAssetEntrySimple) UnmarshalJSON(data []byte) error {
	aux := new(SwapAssetEntrySimpleMarshal)
	if err := json.Unmarshal(data, aux); err != nil {
		return err
	}
	s.Znn = common.StringToBigInt(aux.Znn)
	s.Qsr = common.StringToBigInt(aux.Qsr)
	return nil
}

// SwapLegacyPillarEntry is the RPC view of a legacy pillar swap
// entitlement: a hashed legacy key id (rendered as a hex string)
// and the number of legacy pillars the key holder may still
// migrate.
type SwapLegacyPillarEntry struct {
	KeyIdHash  string `json:"keyIdHash"`
	NumPillars int    `json:"numPillars"`
}

// === Swap Assets ===

// GetAssetsByKeyIdHash returns the swap claim recorded under
// keyIdHash with current-epoch decay applied. When no record
// exists, returns a zero-amount entry (rather than an error) so
// clients can render a placeholder UI for unknown keys.
//
// The current epoch is computed from the frontier momentum
// timestamp via the consensus reader's epoch ticker; decay is
// applied in place by implementation.ApplyDecay.
func (p *SwapApi) GetAssetsByKeyIdHash(keyIdHash types.Hash) (*SwapAssetEntry, error) {
	m, context, err := api.GetFrontierContext(p.chain, types.SwapContract)
	if err != nil {
		return nil, err
	}

	entry, err := definition.GetSwapAssetsByKeyIdHash(context.Storage(), keyIdHash)
	if err == constants.ErrDataNonExistent {
		return &SwapAssetEntry{
			KeyIdHash: keyIdHash.String(),
			Znn:       common.Big0,
			Qsr:       common.Big0,
		}, nil
	}
	if err != nil {
		return nil, err
	}

	currentM, err := context.GetFrontierMomentum()
	common.DealWithErr(err)
	currentEpoch := int(p.consensus.FixedPillarReader(m.Identifier()).EpochTicker().ToTick(*currentM.Timestamp))
	implementation.ApplyDecay(entry, currentEpoch)
	return &SwapAssetEntry{
		KeyIdHash: keyIdHash.String(),
		Znn:       entry.Znn,
		Qsr:       entry.Qsr,
	}, nil
}
// GetAssets returns every recorded swap claim keyed by KeyIdHash,
// with current-epoch decay applied to each entry. Unlike
// GetAssetsByKeyIdHash this is an all-or-nothing read — no
// paging — so callers should expect a result map sized by the
// number of legacy claimants.
func (p *SwapApi) GetAssets() (map[types.Hash]*SwapAssetEntrySimple, error) {
	m, context, err := api.GetFrontierContext(p.chain, types.SwapContract)
	if err != nil {
		return nil, err
	}

	listRaw, err := definition.GetSwapAssets(context.Storage())
	if err != nil {
		return nil, err
	}

	result := make(map[types.Hash]*SwapAssetEntrySimple, len(listRaw))
	currentM, err := context.GetFrontierMomentum()
	common.DealWithErr(err)
	currentEpoch := int(p.consensus.FixedPillarReader(m.Identifier()).EpochTicker().ToTick(*currentM.Timestamp))
	for _, entry := range listRaw {
		implementation.ApplyDecay(entry, currentEpoch)
		result[entry.KeyIdHash] = &SwapAssetEntrySimple{
			Znn: entry.Znn,
			Qsr: entry.Qsr,
		}
	}

	return result, nil
}

// === Swap Legacy Pillars ===

// GetLegacyPillars returns every legacy pillar swap entry —
// pairs of hashed legacy key id (hex-encoded) and the number of
// legacy pillars still claimable under that key. The list comes
// from the pillar contract's storage, not the swap contract's,
// because legacy pillar accounting was kept alongside the active
// pillar registry.
func (p *SwapApi) GetLegacyPillars() ([]*SwapLegacyPillarEntry, error) {
	_, context, err := api.GetFrontierContext(p.chain, types.PillarContract)
	if err != nil {
		return nil, err
	}
	entries, err := definition.GetLegacyPillarList(context.Storage())
	if err != nil {
		return nil, err
	}

	result := make([]*SwapLegacyPillarEntry, len(entries))

	for itr, entry := range entries {
		result[itr] = &SwapLegacyPillarEntry{
			NumPillars: int(entry.PillarCount),
			KeyIdHash:  hex.EncodeToString(entry.KeyIdHash[:]),
		}
	}
	return result, nil
}
