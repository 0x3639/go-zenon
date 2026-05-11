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

// PlasmaApi serves read RPCs for the plasma contract — Zenon's
// anti-spam fee replacement, where fused QSR earns the right to
// publish account blocks.
type PlasmaApi struct {
	chain chain.Chain
	z     zenon.Zenon
	cs    consensus.Consensus
	log   log15.Logger
}

// NewPlasmaApi returns a PlasmaApi bound to z's chain and
// consensus. The cs and z fields are stored for symmetry with
// other handler constructors; currently-exposed methods only use
// chain.
func NewPlasmaApi(z zenon.Zenon) *PlasmaApi {
	return &PlasmaApi{
		chain: z.Chain(),
		z:     z,
		cs:    z.Consensus(),
		log:   common.RPCLogger.New("module", "embedded_plasma_api"),
	}
}

// PlasmaInfo summarises one address's plasma situation:
// CurrentPlasma is the amount currently available (consuming any
// pending account-block reservations), MaxPlasma is what the
// fused QsrAmount entitles the address to per
// vm.FussedAmountToPlasma, and QsrAmount is the total fused QSR
// backing them both.
type PlasmaInfo struct {
	CurrentPlasma uint64   `json:"currentPlasma"`
	MaxPlasma     uint64   `json:"maxPlasma"`
	QsrAmount     *big.Int `json:"qsrAmount"`
}

// PlasmaInfoMarshal mirrors PlasmaInfo with QsrAmount encoded as
// a decimal string for JSON precision safety. The uint64 plasma
// fields cross the wire unchanged.
type PlasmaInfoMarshal struct {
	CurrentPlasma uint64 `json:"currentPlasma"`
	MaxPlasma     uint64 `json:"maxPlasma"`
	QsrAmount     string `json:"qsrAmount"`
}

// ToPlasmaInfoMarshal converts r into its string-QsrAmount wire form.
func (r *PlasmaInfo) ToPlasmaInfoMarshal() *PlasmaInfoMarshal {
	aux := &PlasmaInfoMarshal{
		CurrentPlasma: r.CurrentPlasma,
		MaxPlasma:     r.MaxPlasma,
		QsrAmount:     r.QsrAmount.String(),
	}

	return aux
}

// MarshalJSON renders r through PlasmaInfoMarshal so QsrAmount is
// emitted as a decimal string.
func (r *PlasmaInfo) MarshalJSON() ([]byte, error) {
	return json.Marshal(r.ToPlasmaInfoMarshal())
}

// UnmarshalJSON reads a PlasmaInfoMarshal payload and rehydrates
// the *big.Int QsrAmount via common.StringToBigInt.
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

// FusionEntry is one fusion record: a beneficiary address with a
// committed QsrAmount, an ExpirationHeight (momentum height) after
// which the fusion can be unwound, and a deterministic Id.
type FusionEntry struct {
	QsrAmount        *big.Int      `json:"qsrAmount"`
	Beneficiary      types.Address `json:"beneficiary"`
	ExpirationHeight uint64        `json:"expirationHeight"`
	Id               types.Hash    `json:"id"`
}

// FusionEntryMarshal mirrors FusionEntry with QsrAmount encoded as
// a decimal string for JSON precision safety.
type FusionEntryMarshal struct {
	QsrAmount        string        `json:"qsrAmount"`
	Beneficiary      types.Address `json:"beneficiary"`
	ExpirationHeight uint64        `json:"expirationHeight"`
	Id               types.Hash    `json:"id"`
}

// ToFusionEntryMarshal converts r into its string-amount wire form.
func (r *FusionEntry) ToFusionEntryMarshal() *FusionEntryMarshal {
	aux := &FusionEntryMarshal{
		QsrAmount:        r.QsrAmount.String(),
		Beneficiary:      r.Beneficiary,
		ExpirationHeight: r.ExpirationHeight,
		Id:               r.Id,
	}

	return aux
}

// MarshalJSON renders r through FusionEntryMarshal so QsrAmount is
// emitted as a decimal string.
func (r *FusionEntry) MarshalJSON() ([]byte, error) {
	return json.Marshal(r.ToFusionEntryMarshal())
}

// UnmarshalJSON reads a FusionEntryMarshal payload and rehydrates
// the *big.Int QsrAmount via common.StringToBigInt.
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

// FusionEntryList is the paged response shape for one owner's
// fusion enumeration. QsrAmount is the un-paged total reported by
// definition.GetFusionInfoListByOwner; Count is the pre-paging
// entry count.
type FusionEntryList struct {
	QsrAmount *big.Int       `json:"qsrAmount"`
	Count     int            `json:"count"`
	Fusions   []*FusionEntry `json:"list"`
}

// FusionEntryListMarshal mirrors FusionEntryList with QsrAmount
// encoded as a decimal string for JSON precision safety. Fusions
// entries are reused as-is because FusionEntry has its own
// MarshalJSON.
type FusionEntryListMarshal struct {
	QsrAmount string         `json:"qsrAmount"`
	Count     int            `json:"count"`
	Fusions   []*FusionEntry `json:"list"`
}

// ToFusionEntryListMarshal converts r into its string-QsrAmount
// wire form.
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

// MarshalJSON renders r through FusionEntryListMarshal so
// QsrAmount is emitted as a decimal string.
func (r *FusionEntryList) MarshalJSON() ([]byte, error) {
	return json.Marshal(r.ToFusionEntryListMarshal())
}

// UnmarshalJSON reads a FusionEntryListMarshal payload and
// rehydrates the *big.Int QsrAmount via common.StringToBigInt.
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

// SortFusionEntryByHeight implements sort.Interface so a list of
// definition.FusionInfo records can be ordered by ascending
// ExpirationHeight, with ascending Beneficiary.String() as a
// tiebreaker for fusions sharing the same height.
type SortFusionEntryByHeight []*definition.FusionInfo

func (a SortFusionEntryByHeight) Len() int      { return len(a) }
func (a SortFusionEntryByHeight) Swap(i, j int) { a[i], a[j] = a[j], a[i] }

// Less reports whether the i'th entry sorts before the j'th.
// Equal-height fusions resolve to ascending Beneficiary.String();
// otherwise the one with the smaller ExpirationHeight comes first.
func (a SortFusionEntryByHeight) Less(i, j int) bool {
	if a[i].ExpirationHeight == a[j].ExpirationHeight {
		return a[i].Beneficiary.String() < a[j].Beneficiary.String()
	}
	return a[i].ExpirationHeight < a[j].ExpirationHeight
}

// Get returns the plasma snapshot for address: current available
// plasma (via vm.AvailablePlasma against the frontier momentum),
// the maximum plasma the fused QSR amount entitles the address to
// (via vm.FussedAmountToPlasma), and that fused QSR amount itself.
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

// GetEntriesByAddress returns every fusion entry recorded for
// owner-address, sorted by ascending expiration height (via
// SortFusionEntryByHeight), and a page of the result. QsrAmount in
// the response is the un-paged total reported by
// definition.GetFusionInfoListByOwner. pageSize >
// api.RpcMaxPageSize is rejected with api.ErrPageSizeParamTooBig.
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

// GetRequiredParam is the request payload for
// GetRequiredPoWForAccountBlock: a self-address that will publish
// the block, the BlockType (nom.BlockType*), an optional ToAddr
// (required for BlockTypeUserSend) and the block Data payload.
type GetRequiredParam struct {
	SelfAddr  types.Address  `json:"address"`
	BlockType uint64         `json:"blockType"`
	ToAddr    *types.Address `json:"toAddress"`
	Data      []byte         `json:"data"`
}

// GetRequiredResult is the response from
// GetRequiredPoWForAccountBlock: the address's current available
// plasma, the base plasma cost of the proposed block, and the
// proof-of-work difficulty required to make up the shortfall.
// RequiredDifficulty is zero when AvailablePlasma already covers
// BasePlasma.
type GetRequiredResult struct {
	AvailablePlasma    uint64 `json:"availablePlasma"`
	BasePlasma         uint64 `json:"basePlasma"`
	RequiredDifficulty uint64 `json:"requiredDifficulty"`
}

// GetRequiredPoWForAccountBlock computes the proof-of-work
// difficulty needed for SelfAddr to publish a block of the given
// shape at the current frontier. If AvailablePlasma exceeds
// BasePlasma, the returned RequiredDifficulty is zero. Otherwise
// vm.GetDifficultyForPlasma maps the missing plasma to a PoW
// difficulty target.
//
// Errors:
//   - returns "toAddress is nil" when BlockType is
//     nom.BlockTypeUserSend and ToAddr is nil.
//   - propagates errors from api.GetFrontierContext,
//     vm.AvailablePlasma, vm.GetBasePlasmaForAccountBlock, and
//     vm.GetDifficultyForPlasma.
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
