package embedded

import (
	"encoding/json"
	"math/big"
	"sort"

	"github.com/inconshreveable/log15"
	"github.com/pkg/errors"

	"github.com/zenon-network/go-zenon/chain"
	"github.com/zenon-network/go-zenon/chain/nom"
	"github.com/zenon-network/go-zenon/common"
	"github.com/zenon-network/go-zenon/common/types"
	"github.com/zenon-network/go-zenon/consensus"
	"github.com/zenon-network/go-zenon/rpc/api"
	"github.com/zenon-network/go-zenon/vm"
	"github.com/zenon-network/go-zenon/vm/embedded/definition"
	"github.com/zenon-network/go-zenon/zenon"
)

// PlasmaApi is the "embedded.plasma" namespace — read access to
// plasma fusion entries (anti-spam stake-style mechanism).
type PlasmaApi struct {
	chain chain.Chain
	z     zenon.Zenon
	cs    consensus.Consensus
	log   log15.Logger
}

// NewPlasmaApi constructs the plasma namespace handler.
func NewPlasmaApi(z zenon.Zenon) *PlasmaApi {
	return &PlasmaApi{
		chain: z.Chain(),
		z:     z,
		cs:    z.Consensus(),
		log:   common.RPCLogger.New("module", "embedded_plasma_api"),
	}
}

// PlasmaInfo is part of the package's public API; see the surrounding code for usage.
type PlasmaInfo struct {
	CurrentPlasma uint64   `json:"currentPlasma"`
	MaxPlasma     uint64   `json:"maxPlasma"`
	QsrAmount     *big.Int `json:"qsrAmount"`
}

// PlasmaInfoMarshal is part of the package's public API; see the surrounding code for usage.
type PlasmaInfoMarshal struct {
	CurrentPlasma uint64 `json:"currentPlasma"`
	MaxPlasma     uint64 `json:"maxPlasma"`
	QsrAmount     string `json:"qsrAmount"`
}

// ToPlasmaInfoMarshal projects the receiver to its JSON-friendly PlasmaInfoMarshal twin.
func (r *PlasmaInfo) ToPlasmaInfoMarshal() *PlasmaInfoMarshal {
	aux := &PlasmaInfoMarshal{
		CurrentPlasma: r.CurrentPlasma,
		MaxPlasma:     r.MaxPlasma,
		QsrAmount:     r.QsrAmount.String(),
	}

	return aux
}

// MarshalJSON forwards through the Marshal twin so big.Int fields render as decimal strings.
func (r *PlasmaInfo) MarshalJSON() ([]byte, error) {
	return json.Marshal(r.ToPlasmaInfoMarshal())
}

// UnmarshalJSON inflates the JSON wire form back into the in-memory receiver.
func (r *PlasmaInfo) UnmarshalJSON(data []byte) error {
	aux := new(PlasmaInfoMarshal)
	if err := json.Unmarshal(data, aux); err != nil {
		return err
	}
	r.CurrentPlasma = aux.CurrentPlasma
	r.MaxPlasma = aux.MaxPlasma
	r.QsrAmount = common.StringToBigInt(aux.QsrAmount)
	return nil
}

// FusionEntry is part of the package's public API; see the surrounding code for usage.
type FusionEntry struct {
	QsrAmount        *big.Int      `json:"qsrAmount"`
	Beneficiary      types.Address `json:"beneficiary"`
	ExpirationHeight uint64        `json:"expirationHeight"`
	Id               types.Hash    `json:"id"`
}

// FusionEntryMarshal is part of the package's public API; see the surrounding code for usage.
type FusionEntryMarshal struct {
	QsrAmount        string        `json:"qsrAmount"`
	Beneficiary      types.Address `json:"beneficiary"`
	ExpirationHeight uint64        `json:"expirationHeight"`
	Id               types.Hash    `json:"id"`
}

// ToFusionEntryMarshal projects the receiver to its JSON-friendly FusionEntryMarshal twin.
func (r *FusionEntry) ToFusionEntryMarshal() *FusionEntryMarshal {
	aux := &FusionEntryMarshal{
		QsrAmount:        r.QsrAmount.String(),
		Beneficiary:      r.Beneficiary,
		ExpirationHeight: r.ExpirationHeight,
		Id:               r.Id,
	}

	return aux
}

// MarshalJSON forwards through the Marshal twin so big.Int fields render as decimal strings.
func (r *FusionEntry) MarshalJSON() ([]byte, error) {
	return json.Marshal(r.ToFusionEntryMarshal())
}

// UnmarshalJSON inflates the JSON wire form back into the in-memory receiver.
func (r *FusionEntry) UnmarshalJSON(data []byte) error {
	aux := new(FusionEntryMarshal)
	if err := json.Unmarshal(data, aux); err != nil {
		return err
	}
	r.QsrAmount = common.StringToBigInt(aux.QsrAmount)
	r.Beneficiary = aux.Beneficiary
	r.ExpirationHeight = aux.ExpirationHeight
	r.Id = aux.Id
	return nil
}

// FusionEntryList is part of the package's public API; see the surrounding code for usage.
type FusionEntryList struct {
	QsrAmount *big.Int       `json:"qsrAmount"`
	Count     int            `json:"count"`
	Fusions   []*FusionEntry `json:"list"`
}

// FusionEntryListMarshal is part of the package's public API; see the surrounding code for usage.
type FusionEntryListMarshal struct {
	QsrAmount string         `json:"qsrAmount"`
	Count     int            `json:"count"`
	Fusions   []*FusionEntry `json:"list"`
}

// ToFusionEntryListMarshal projects the receiver to its JSON-friendly FusionEntryListMarshal twin.
func (r *FusionEntryList) ToFusionEntryListMarshal() *FusionEntryListMarshal {
	aux := &FusionEntryListMarshal{
		QsrAmount: r.QsrAmount.String(),
		Count:     r.Count,
	}
	aux.Fusions = make([]*FusionEntry, len(r.Fusions))
	for idx, fusion := range r.Fusions {
		aux.Fusions[idx] = fusion
	}

	return aux
}

// MarshalJSON forwards through the Marshal twin so big.Int fields render as decimal strings.
func (r *FusionEntryList) MarshalJSON() ([]byte, error) {
	return json.Marshal(r.ToFusionEntryListMarshal())
}

// UnmarshalJSON inflates the JSON wire form back into the in-memory receiver.
func (r *FusionEntryList) UnmarshalJSON(data []byte) error {
	aux := new(FusionEntryListMarshal)
	if err := json.Unmarshal(data, aux); err != nil {
		return err
	}
	r.QsrAmount = common.StringToBigInt(aux.QsrAmount)
	r.Count = aux.Count
	r.Fusions = make([]*FusionEntry, len(r.Fusions))
	for idx, fusion := range aux.Fusions {
		r.Fusions[idx] = fusion
	}

	return nil
}

// SortFusionEntryByHeight is part of the package's public API; see the surrounding code for usage.
type SortFusionEntryByHeight []*definition.FusionInfo

func (a SortFusionEntryByHeight) Len() int      { return len(a) }
func (a SortFusionEntryByHeight) Swap(i, j int) { a[i], a[j] = a[j], a[i] }
func (a SortFusionEntryByHeight) Less(i, j int) bool {
	if a[i].ExpirationHeight == a[j].ExpirationHeight {
		return a[i].Beneficiary.String() < a[j].Beneficiary.String()
	}
	return a[i].ExpirationHeight < a[j].ExpirationHeight
}

// Get is part of the receiver's public API.
func (a *PlasmaApi) Get(address types.Address) (*PlasmaInfo, error) {
	_, context, err := api.GetFrontierContext(a.chain, address)
	if err != nil {
		return nil, err
	}

	amount, err := a.chain.GetFrontierMomentumStore().GetStakeBeneficialAmount(address)
	if err != nil {
		return nil, err
	}

	available, err := vm.AvailablePlasma(context.MomentumStore(), context)
	if err != nil {
		return nil, err
	}

	return &PlasmaInfo{
		CurrentPlasma: available,
		MaxPlasma:     vm.FussedAmountToPlasma(amount),
		QsrAmount:     amount,
	}, nil
}

// GetEntriesByAddress returns address's plasma fusion entries sorted by
// ascending expiration height (Beneficiary-tiebroken), sliced to
// (pageIndex, pageSize). Composes definition.GetFusionInfoListByOwner with
// [SortFusionEntryByHeight] and projects each entry to wire-form [FusionEntry];
// QsrAmount in the response is the total fused QSR across the full list.
func (a *PlasmaApi) GetEntriesByAddress(address types.Address, pageIndex, pageSize uint32) (*FusionEntryList, error) {
	if pageSize > api.RpcMaxPageSize {
		return nil, api.ErrPageSizeParamTooBig
	}

	_, context, err := api.GetFrontierContext(a.chain, types.PlasmaContract)
	if err != nil {
		return nil, err
	}
	list, amount, err := definition.GetFusionInfoListByOwner(context.Storage(), address)
	if err != nil {
		return nil, err
	}

	sort.Sort(SortFusionEntryByHeight(list))
	listLen := len(list)
	start, end := api.GetRange(pageIndex, pageSize, uint32(listLen))
	entryList := make([]*FusionEntry, end-start)

	for i, info := range list[start:end] {
		entryList[i] = &FusionEntry{
			info.Amount,
			info.Beneficiary,
			info.ExpirationHeight,
			info.Id,
		}
	}
	return &FusionEntryList{amount, listLen, entryList}, nil
}

// GetRequiredParam is the request shape for [PlasmaApi.GetRequiredPoWForAccountBlock]:
// the prospective account block's address, type, recipient, and data, used
// to compute the PoW difficulty needed to cover its plasma cost.
type GetRequiredParam struct {
	SelfAddr  types.Address  `json:"address"`
	BlockType uint64         `json:"blockType"`
	ToAddr    *types.Address `json:"toAddress"`
	Data      []byte         `json:"data"`
}

// GetRequiredResult is the response shape for [PlasmaApi.GetRequiredPoWForAccountBlock]:
// the caller's currently-available plasma, the block's base plasma cost, and
// the PoW difficulty (zero when available plasma already covers the cost).
type GetRequiredResult struct {
	AvailablePlasma    uint64 `json:"availablePlasma"`
	BasePlasma         uint64 `json:"basePlasma"`
	RequiredDifficulty uint64 `json:"requiredDifficulty"`
}

// GetRequiredPoWForAccountBlock returns the PoW difficulty a caller must
// solve to send the prospective account block described by param. Composes
// vm.AvailablePlasma + vm.GetBasePlasmaForAccountBlock; if available plasma
// already covers the base cost the returned RequiredDifficulty is 0,
// otherwise it is computed via vm.GetDifficultyForPlasma on the shortfall.
func (a *PlasmaApi) GetRequiredPoWForAccountBlock(param GetRequiredParam) (*GetRequiredResult, error) {
	_, context, err := api.GetFrontierContext(a.chain, param.SelfAddr)
	frontierMomentum, err := context.GetFrontierMomentum()
	if err != nil {
		return nil, err
	}

	// get required plasma
	block := &nom.AccountBlock{
		BlockType:            param.BlockType,
		Address:              param.SelfAddr,
		Data:                 param.Data,
		MomentumAcknowledged: frontierMomentum.Identifier(),
	}

	if param.ToAddr != nil {
		block.ToAddress = *param.ToAddr
	} else if param.BlockType == nom.BlockTypeUserSend {
		return nil, errors.New("toAddress is nil")
	}

	availablePlasma, err := vm.AvailablePlasma(context.MomentumStore(), context)
	if err != nil {
		return nil, err
	}

	basePlasma, err := vm.GetBasePlasmaForAccountBlock(context, block)
	if err != nil {
		return nil, err
	}

	if availablePlasma > basePlasma {
		return &GetRequiredResult{
			AvailablePlasma:    availablePlasma,
			BasePlasma:         basePlasma,
			RequiredDifficulty: 0,
		}, nil
	} else {
		difficulty, err := vm.GetDifficultyForPlasma(basePlasma - availablePlasma)
		if err != nil {
			return nil, err
		}
		return &GetRequiredResult{
			AvailablePlasma:    availablePlasma,
			BasePlasma:         basePlasma,
			RequiredDifficulty: difficulty,
		}, nil
	}
}
