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

// SwapApi is the "embedded.swap" namespace — read access to the
// pillar→coinbase swap accounting maintained by the swap contract.
type SwapApi struct {
	chain     chain.Chain
	consensus consensus.Consensus
	log       log15.Logger
}

// NewSwapApi constructs the swap namespace handler.
func NewSwapApi(z zenon.Zenon) *SwapApi {
	return &SwapApi{
		chain:     z.Chain(),
		consensus: z.Consensus(),
		log:       common.RPCLogger.New("module", "rpc_api/embedded_swap_api"),
	}
}

// SwapAssetEntry is one (legacy-key, ZNN, QSR) row of swap-asset
// data, returned by [SwapApi.GetAssetsByKeyIdHash] /
// [SwapApi.GetAssets].
type SwapAssetEntry struct {
	KeyIdHash string   `json:"keyIdHash"`
	Znn       *big.Int `json:"znn"`
	Qsr       *big.Int `json:"qsr"`
}

// SwapAssetEntryMarshal is the JSON-friendly twin with decimal-string
// amounts.
type SwapAssetEntryMarshal struct {
	KeyIdHash string `json:"keyIdHash"`
	Znn       string `json:"znn"`
	Qsr       string `json:"qsr"`
}

// ToSwapAssetEntryMarshal projects the receiver to its JSON-friendly SwapAssetEntryMarshal twin.
func (s *SwapAssetEntry) ToSwapAssetEntryMarshal() *SwapAssetEntryMarshal {
	aux := &SwapAssetEntryMarshal{
		KeyIdHash: s.KeyIdHash,
		Znn:       s.Znn.String(),
		Qsr:       s.Qsr.String(),
	}

	return aux
}

// MarshalJSON forwards through the Marshal twin so big.Int fields render as decimal strings.
func (s *SwapAssetEntry) MarshalJSON() ([]byte, error) {
	return json.Marshal(s.ToSwapAssetEntryMarshal())
}

// UnmarshalJSON inflates the JSON wire form back into the in-memory receiver.
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

// SwapAssetEntrySimple is part of the package's public API; see the surrounding code for usage.
type SwapAssetEntrySimple struct {
	Znn *big.Int `json:"znn"`
	Qsr *big.Int `json:"qsr"`
}

// SwapAssetEntrySimpleMarshal is part of the package's public API; see the surrounding code for usage.
type SwapAssetEntrySimpleMarshal struct {
	Znn string `json:"znn"`
	Qsr string `json:"qsr"`
}

// ToSwapAssetEntrySimpleMarshal projects the receiver to its JSON-friendly SwapAssetEntrySimpleMarshal twin.
func (s *SwapAssetEntrySimple) ToSwapAssetEntrySimpleMarshal() *SwapAssetEntrySimpleMarshal {
	aux := &SwapAssetEntrySimpleMarshal{
		Znn: s.Znn.String(),
		Qsr: s.Qsr.String(),
	}

	return aux
}

// MarshalJSON forwards through the Marshal twin so big.Int fields render as decimal strings.
func (s *SwapAssetEntrySimple) MarshalJSON() ([]byte, error) {
	return json.Marshal(s.ToSwapAssetEntrySimpleMarshal())
}

// UnmarshalJSON inflates the JSON wire form back into the in-memory receiver.
func (s *SwapAssetEntrySimple) UnmarshalJSON(data []byte) error {
	aux := new(SwapAssetEntrySimpleMarshal)
	if err := json.Unmarshal(data, aux); err != nil {
		return err
	}
	s.Znn = common.StringToBigInt(aux.Znn)
	s.Qsr = common.StringToBigInt(aux.Qsr)
	return nil
}

// SwapLegacyPillarEntry is part of the package's public API; see the surrounding code for usage.
type SwapLegacyPillarEntry struct {
	KeyIdHash  string `json:"keyIdHash"`
	NumPillars int    `json:"numPillars"`
}

// === Swap Assets ===

// GetAssetsByKeyIdHash returns the post-decay swap balance for keyIdHash.
// Composes definition.GetSwapAssetsByKeyIdHash with implementation.ApplyDecay
// at the current epoch; missing entries are returned as a zero-amount stub
// rather than ErrDataNonExistent.
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

// GetAssets returns every swap entry keyed by legacy KeyIdHash, with
// implementation.ApplyDecay applied to each at the current epoch.
// Composes definition.GetSwapAssets and projects entries to wire-form
// [SwapAssetEntrySimple] (no key field, key lives in the map).
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

// GetLegacyPillars returns the unswapped legacy pillar registry, projecting
// each entry to wire-form [SwapLegacyPillarEntry] with a hex-encoded
// KeyIdHash. Composes definition.GetLegacyPillarList; reads from the pillar
// contract's storage rather than the swap contract.
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
