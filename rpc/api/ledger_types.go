package api

import (
	"encoding/json"
	"math/big"

	"github.com/zenon-network/go-zenon/chain"
	"github.com/zenon-network/go-zenon/chain/nom"
	"github.com/zenon-network/go-zenon/common"
	"github.com/zenon-network/go-zenon/common/types"
	"github.com/zenon-network/go-zenon/vm/embedded/definition"
)

// DetailedMomentum is a Momentum paired with the full
// AccountBlock view of every account block included in it.
// Returned by LedgerApi.GetDetailedMomentumsByHeight.
type DetailedMomentum struct {
	AccountBlocks []*AccountBlock `json:"blocks"`
	Momentum      *Momentum       `json:"momentum"`
}

// Momentum is the RPC view of a consensus block: an embedded
// *nom.Momentum plus the elected producer's address (derived from
// the momentum's signature via nom.Momentum.Producer()).
type Momentum struct {
	*nom.Momentum
	Producer types.Address `json:"producer"`
}

// MomentumHeader is a momentum identity tuple used for compact
// references in payloads that do not need the full body. Note
// that this type is defined in this package but not emitted by
// any current LedgerApi method.
type MomentumHeader struct {
	Hash      types.Hash `json:"hash"`
	Height    uint64     `json:"height"`
	Timestamp int64      `json:"timestamp"`
}

// AccountBlockConfirmationDetail describes which momentum first
// confirmed an account block and how many momentums have built on
// top of that confirmation. NumConfirmations is computed as
// frontier.Height - MomentumHeight + 1, so an account block in
// the frontier momentum has NumConfirmations == 1.
type AccountBlockConfirmationDetail struct {
	NumConfirmations  uint64     `json:"numConfirmations"`
	MomentumHeight    uint64     `json:"momentumHeight"`
	MomentumHash      types.Hash `json:"momentumHash"`
	MomentumTimestamp int64      `json:"momentumTimestamp"`
}

// AccountBlock is the RPC view of one account block. It embeds the
// chain's nom.AccountBlock and decorates it with three derived
// fields populated by addAllExtraInfo:
//
//   - TokenInfo: the token registry record for the block's
//     TokenStandard (nil for ZeroTokenStandard).
//   - ConfirmationDetail: the momentum that first confirmed this
//     block plus the running confirmation depth (nil until the
//     block is included in a momentum).
//   - PairedAccountBlock: the counterpart send block (for receive
//     blocks) or the matching receive block (for send blocks);
//     nil until paired.
type AccountBlock struct {
	nom.AccountBlock

	TokenInfo          *Token                          `json:"token"`
	ConfirmationDetail *AccountBlockConfirmationDetail `json:"confirmationDetail"`
	PairedAccountBlock *AccountBlock                   `json:"pairedAccountBlock"`
}

// AccountBlockMarshal is the on-the-wire twin of AccountBlock with
// *big.Int amount fields encoded as decimal strings (via the
// embedded nom.AccountBlockMarshal) and the TokenInfo / Paired
// blocks recursively converted to their *Marshal variants.
// MarshalJSON / UnmarshalJSON on AccountBlock round-trip through
// this type.
type AccountBlockMarshal struct {
	nom.AccountBlockMarshal
	TokenInfo          *TokenMarshal                   `json:"token"`
	ConfirmationDetail *AccountBlockConfirmationDetail `json:"confirmationDetail"`
	PairedAccountBlock *AccountBlockMarshal            `json:"pairedAccountBlock"`
}

// ToAccountBlockMarshal converts block into its decimal-string
// wire form. The embedded nom.AccountBlock is delegated to
// nom.AccountBlock.ToNomMarshalJson for amount handling; TokenInfo
// and PairedAccountBlock are recursively converted.
func (block *AccountBlock) ToAccountBlockMarshal() *AccountBlockMarshal {
	aux := &AccountBlockMarshal{
		AccountBlockMarshal: *block.AccountBlock.ToNomMarshalJson(),
		ConfirmationDetail:  block.ConfirmationDetail,
	}
	if block.TokenInfo != nil {
		aux.TokenInfo = block.TokenInfo.ToTokenMarshal()
	}
	if block.PairedAccountBlock != nil {
		aux.PairedAccountBlock = block.PairedAccountBlock.ToAccountBlockMarshal()
	}
	return aux
}

// MarshalJSON renders block through its AccountBlockMarshal twin
// so amounts cross the wire as decimal strings.
func (block *AccountBlock) MarshalJSON() ([]byte, error) {
	return json.Marshal(block.ToAccountBlockMarshal())
}

// UnmarshalJSON reads an AccountBlockMarshal payload and rehydrates
// the embedded nom.AccountBlock via FromNomMarshalJson, plus the
// TokenInfo and PairedAccountBlock recursive *big.Int fields.
func (block *AccountBlock) UnmarshalJSON(data []byte) error {
	aux := new(AccountBlockMarshal)
	if err := json.Unmarshal(data, aux); err != nil {
		return err
	}

	block.AccountBlock = *aux.FromNomMarshalJson()
	if aux.TokenInfo != nil {
		block.TokenInfo = aux.TokenInfo.FromTokenMarshal()
	}
	block.ConfirmationDetail = aux.ConfirmationDetail
	if aux.PairedAccountBlock != nil {
		block.PairedAccountBlock = aux.PairedAccountBlock.FromApiMarshalJson()
	}
	return nil
}

// FromApiMarshalJson converts a back into the *big.Int-bearing
// AccountBlock by rehydrating the embedded nom.AccountBlock and
// recursively converting TokenInfo and PairedAccountBlock. The
// returned *AccountBlock is fully independent of a; mutating
// either does not affect the other.
func (a *AccountBlockMarshal) FromApiMarshalJson() *AccountBlock {
	aux := &AccountBlock{
		ConfirmationDetail: a.ConfirmationDetail,
	}
	block := a.FromNomMarshalJson()
	aux.AccountBlock = *block
	if a.TokenInfo != nil {
		aux.TokenInfo = a.TokenInfo.FromTokenMarshal()
	}
	if a.PairedAccountBlock != nil {
		aux.PairedAccountBlock = a.PairedAccountBlock.FromApiMarshalJson()
	}
	return aux
}

// AccountInfo summarises the chain state of one account: its
// address, the height of its frontier account block, and its
// balance per token standard. AccountHeight is zero for accounts
// that have not produced any blocks yet. BalanceInfoMap omits
// tokens whose registry lookup fails (the unregistered ZTS case);
// it does not include zero-balance entries for known tokens that
// the account never held.
type AccountInfo struct {
	Address        types.Address                             `json:"address"`
	AccountHeight  uint64                                    `json:"accountHeight"`
	BalanceInfoMap map[types.ZenonTokenStandard]*BalanceInfo `json:"balanceInfoMap"`
}

// BalanceInfo pairs a token registry record with one account's
// balance for that token. Balance is *big.Int internally and
// emitted as a decimal string over JSON via BalanceInfoMarshal.
type BalanceInfo struct {
	TokenInfo *Token   `json:"token"`
	Balance   *big.Int `json:"balance"`
}

// BalanceInfoMarshal is the on-the-wire twin of BalanceInfo with
// the *big.Int balance encoded as a decimal string. MarshalJSON /
// UnmarshalJSON on BalanceInfo round-trip through this type.
type BalanceInfoMarshal struct {
	TokenInfo *TokenMarshal `json:"token"`
	Balance   string        `json:"balance"`
}

// ToBalanceInfoMarshal converts b into its string-balance wire
// form. Both nested *big.Int values (Balance and TokenInfo's
// supply totals) are rendered as decimal strings.
func (b *BalanceInfo) ToBalanceInfoMarshal() BalanceInfoMarshal {
	aux := BalanceInfoMarshal{
		TokenInfo: b.TokenInfo.ToTokenMarshal(),
		Balance:   b.Balance.String(),
	}
	return aux
}

// MarshalJSON renders b through its BalanceInfoMarshal twin so
// the *big.Int balance becomes a decimal string.
func (b *BalanceInfo) MarshalJSON() ([]byte, error) {
	return json.Marshal(b.ToBalanceInfoMarshal())
}

// UnmarshalJSON reads a BalanceInfoMarshal payload and rehydrates
// the *big.Int Balance via common.StringToBigInt. TokenInfo is
// converted via TokenMarshal.FromTokenMarshal.
func (b *BalanceInfo) UnmarshalJSON(data []byte) error {
	aux := new(BalanceInfoMarshal)
	if err := json.Unmarshal(data, aux); err != nil {
		return err
	}

	b.TokenInfo = aux.TokenInfo.FromTokenMarshal()
	b.Balance = common.StringToBigInt(aux.Balance)
	return nil
}

// Token is the RPC view of one entry in the embedded token
// registry: identity (name / symbol / domain / ZTS), supply
// (TotalSupply / MaxSupply as *big.Int), and policy flags
// (IsBurnable / IsMintable / IsUtility). The ZeroTokenStandard
// has no Token record; LedgerTokenInfoToRpc returns nil for a
// nil registry entry.
type Token struct {
	TokenName          string                   `json:"name"`
	TokenSymbol        string                   `json:"symbol"`
	TokenDomain        string                   `json:"domain"`
	TotalSupply        *big.Int                 `json:"totalSupply"`
	Decimals           uint8                    `json:"decimals"`
	Owner              types.Address            `json:"owner"`
	ZenonTokenStandard types.ZenonTokenStandard `json:"tokenStandard"`
	MaxSupply          *big.Int                 `json:"maxSupply"`
	IsBurnable         bool                     `json:"isBurnable"`
	IsMintable         bool                     `json:"isMintable"`
	IsUtility          bool                     `json:"isUtility"`
}

// TokenMarshal is the on-the-wire twin of Token with the supply
// totals encoded as decimal strings. MarshalJSON / UnmarshalJSON
// on Token round-trip through this type.
type TokenMarshal struct {
	TokenName          string                   `json:"name"`
	TokenSymbol        string                   `json:"symbol"`
	TokenDomain        string                   `json:"domain"`
	TotalSupply        string                   `json:"totalSupply"`
	Decimals           uint8                    `json:"decimals"`
	Owner              types.Address            `json:"owner"`
	ZenonTokenStandard types.ZenonTokenStandard `json:"tokenStandard"`
	MaxSupply          string                   `json:"maxSupply"`
	IsBurnable         bool                     `json:"isBurnable"`
	IsMintable         bool                     `json:"isMintable"`
	IsUtility          bool                     `json:"isUtility"`
}

// ToTokenMarshal converts t into its string-supply wire form.
// Both TotalSupply and MaxSupply are emitted as decimal strings.
func (t *Token) ToTokenMarshal() *TokenMarshal {
	aux := &TokenMarshal{
		TokenName:          t.TokenName,
		TokenSymbol:        t.TokenSymbol,
		TokenDomain:        t.TokenDomain,
		MaxSupply:          t.MaxSupply.String(),
		Decimals:           t.Decimals,
		Owner:              t.Owner,
		ZenonTokenStandard: t.ZenonTokenStandard,
		TotalSupply:        t.TotalSupply.String(),
		IsBurnable:         t.IsBurnable,
		IsMintable:         t.IsMintable,
		IsUtility:          t.IsUtility,
	}
	return aux
}

// MarshalJSON renders t through its TokenMarshal twin so supply
// totals become decimal strings.
func (t *Token) MarshalJSON() ([]byte, error) {
	return json.Marshal(t.ToTokenMarshal())
}

// UnmarshalJSON reads a TokenMarshal payload and rehydrates the
// *big.Int supply fields via common.StringToBigInt.
func (t *Token) UnmarshalJSON(data []byte) error {
	aux := new(TokenMarshal)
	if err := json.Unmarshal(data, aux); err != nil {
		return err
	}

	t.TokenName = aux.TokenName
	t.TokenSymbol = aux.TokenSymbol
	t.TokenDomain = aux.TokenDomain
	t.MaxSupply = common.StringToBigInt(aux.MaxSupply)
	t.Decimals = aux.Decimals
	t.Owner = aux.Owner
	t.ZenonTokenStandard = aux.ZenonTokenStandard
	t.TotalSupply = common.StringToBigInt(aux.TotalSupply)
	t.IsBurnable = aux.IsBurnable
	t.IsMintable = aux.IsMintable
	t.IsUtility = aux.IsUtility
	return nil
}

// FromTokenMarshal converts t back into the *big.Int-bearing
// Token by rehydrating TotalSupply and MaxSupply via
// common.StringToBigInt. The returned *Token is fully independent
// of t.
func (t *TokenMarshal) FromTokenMarshal() *Token {
	return &Token{
		TokenName:          t.TokenName,
		TokenSymbol:        t.TokenSymbol,
		TokenDomain:        t.TokenDomain,
		TotalSupply:        common.StringToBigInt(t.TotalSupply),
		Decimals:           t.Decimals,
		Owner:              t.Owner,
		ZenonTokenStandard: t.ZenonTokenStandard,
		MaxSupply:          common.StringToBigInt(t.MaxSupply),
		IsBurnable:         t.IsBurnable,
		IsMintable:         t.IsMintable,
		IsUtility:          t.IsUtility,
	}
}

// AccountBlockList is the paged response shape for account-block
// enumeration. Count is the underlying total (whose precise
// meaning depends on the calling method — frontier-height for
// height-paged readers, unreceived-block count for the unreceived
// reader). More is true when the underlying source still has
// records beyond the returned slice, used by the unreceived
// reader to signal that pagination may continue.
type AccountBlockList struct {
	List  []*AccountBlock `json:"list"`
	Count int             `json:"count"`
	More  bool            `json:"more"`
}

// AccountBlockListMarshal is the on-the-wire twin of
// AccountBlockList with each entry rendered as AccountBlockMarshal
// (decimal-string amounts). MarshalJSON / UnmarshalJSON on
// AccountBlockList round-trip through this type.
type AccountBlockListMarshal struct {
	List  []*AccountBlockMarshal `json:"list"`
	Count int                    `json:"count"`
	More  bool                   `json:"more"`
}

// ToAccountBlockListMarshal converts abl into its decimal-string
// wire form, recursing into each list entry's amounts, TokenInfo,
// and PairedAccountBlock.
func (abl *AccountBlockList) ToAccountBlockListMarshal() *AccountBlockListMarshal {
	aux := &AccountBlockListMarshal{
		Count: abl.Count,
		More:  abl.More,
	}
	aux.List = make([]*AccountBlockMarshal, 0)
	for idx, block := range abl.List {
		aux.List = append(aux.List, &AccountBlockMarshal{
			AccountBlockMarshal: *block.ToNomMarshalJson(),
			ConfirmationDetail:  block.ConfirmationDetail,
		})
		if block.TokenInfo != nil {
			aux.List[idx].TokenInfo = block.TokenInfo.ToTokenMarshal()
		}
		if block.PairedAccountBlock != nil {
			aux.List[idx].PairedAccountBlock = block.PairedAccountBlock.ToAccountBlockMarshal()
		}
	}
	return aux
}

// MarshalJSON renders abl through its AccountBlockListMarshal twin
// so the embedded entry amounts cross the wire as decimal strings.
func (abl *AccountBlockList) MarshalJSON() ([]byte, error) {
	return json.Marshal(abl.ToAccountBlockListMarshal())
}

// UnmarshalJSON reads an AccountBlockListMarshal payload and
// rehydrates each entry via AccountBlockMarshal.FromApiMarshalJson.
func (abl *AccountBlockList) UnmarshalJSON(data []byte) error {
	aux := new(AccountBlockListMarshal)
	if err := json.Unmarshal(data, aux); err != nil {
		return err
	}

	abl.List = make([]*AccountBlock, 0)
	for _, accBl := range aux.List {
		block := accBl.FromApiMarshalJson()
		abl.List = append(abl.List, block)
	}
	abl.Count = aux.Count
	abl.More = aux.More
	return nil
}

// MomentumList is the paged response shape for momentum
// enumeration. Count is the chain's frontier momentum height
// (lastEpoch + 1 in effect), not the size of List, so clients can
// compute the global page count.
type MomentumList struct {
	List  []*Momentum `json:"list"`
	Count int         `json:"count"`
}

// DetailedMomentumList is the paged response shape for
// LedgerApi.GetDetailedMomentumsByHeight: a slice of momentums
// each paired with the full AccountBlock view of every account
// block included in that momentum. Count matches the underlying
// MomentumList.Count (chain frontier height).
type DetailedMomentumList struct {
	List  []*DetailedMomentum `json:"list"`
	Count int                 `json:"count"`
}

// ToLedgerBlock returns a defensive deep copy of the embedded
// nom.AccountBlock, stripped of the RPC-only decoration fields.
// Used by LedgerApi.PublishRawTransaction to hand a clean block
// to the VM supervisor. The (error) return is kept for forward
// compatibility; the current implementation always returns a
// non-nil block and a nil error.
func (block *AccountBlock) ToLedgerBlock() (*nom.AccountBlock, error) {
	return block.AccountBlock.Copy(), nil
}

// ComputeHash returns the canonical hash of the embedded
// nom.AccountBlock as computed by nom.AccountBlock.ComputeHash —
// i.e., the hash a producer would sign. Errors from ToLedgerBlock
// (none today) propagate unchanged.
func (block *AccountBlock) ComputeHash() (*types.Hash, error) {
	lAb, err := block.ToLedgerBlock()
	if err != nil {
		return nil, err
	}
	hash := lAb.ComputeHash()
	return &hash, nil
}
func (block *AccountBlock) prefetchToken(chain chain.Chain) error {
	store := chain.GetFrontierMomentumStore()
	if block.TokenStandard != types.ZeroTokenStandard {
		token, err := store.GetTokenInfoByTs(block.TokenStandard)
		if err != nil {
			return err
		}
		block.TokenInfo = LedgerTokenInfoToRpc(token)
	}
	return nil
}
func (block *AccountBlock) prefetchPaired(chain chain.Chain) error {
	store := chain.GetFrontierMomentumStore()
	var err error
	var paired *nom.AccountBlock
	if block.BlockType == nom.BlockTypeGenesisReceive {
		genesis := chain.GetGenesisMomentum()
		frontier, _ := store.GetFrontierMomentum()
		block.PairedAccountBlock = &AccountBlock{
			AccountBlock: nom.AccountBlock{
				BlockType:        nom.BlockTypeContractSend,
				Amount:           common.Big0,
				DescendantBlocks: make([]*nom.AccountBlock, 0),
			},
			ConfirmationDetail: &AccountBlockConfirmationDetail{
				NumConfirmations:  frontier.Height - genesis.Height + 1,
				MomentumHeight:    genesis.Height,
				MomentumHash:      genesis.Hash,
				MomentumTimestamp: genesis.Timestamp.Unix(),
			},
		}
		return nil
	}

	if nom.IsSendBlock(block.BlockType) {
		paired, err = store.GetBlockWhichReceives(block.Hash)
	} else {
		paired, err = store.GetAccountBlockByHash(block.FromBlockHash)
	}
	if err != nil {
		return err
	}
	if paired != nil {
		block.PairedAccountBlock = &AccountBlock{
			AccountBlock: *paired.Copy(),
		}
		if err := block.PairedAccountBlock.prefetchToken(chain); err != nil {
			return err
		}
		if err := block.PairedAccountBlock.addConfirmationInfo(chain); err != nil {
			return err
		}
	}

	return nil
}
func (block *AccountBlock) addConfirmationInfo(chain chain.Chain) error {
	store := chain.GetFrontierMomentumStore()
	frontier, err := store.GetFrontierMomentum()
	confirmationHeight, err := chain.GetFrontierMomentumStore().GetBlockConfirmationHeight(block.Hash)
	if err != nil {
		return err
	}
	confirmedBlock, err := chain.GetFrontierMomentumStore().GetMomentumByHeight(confirmationHeight)
	if err != nil {
		return err
	}
	if confirmedBlock != nil && frontier != nil && confirmedBlock.Height <= frontier.Height {
		block.ConfirmationDetail = &AccountBlockConfirmationDetail{
			NumConfirmations:  frontier.Height - confirmedBlock.Height + 1,
			MomentumHeight:    confirmedBlock.Height,
			MomentumHash:      confirmedBlock.Hash,
			MomentumTimestamp: confirmedBlock.Timestamp.Unix(),
		}
	}

	return nil
}
func (block *AccountBlock) addAllExtraInfo(chain chain.Chain) error {
	if err := block.prefetchPaired(chain); err != nil {
		return err
	}
	if err := block.prefetchToken(chain); err != nil {
		return err
	}
	if err := block.addConfirmationInfo(chain); err != nil {
		return err
	}

	return nil
}

func momentumListToDetailedList(chain chain.Chain, list *MomentumList) (*DetailedMomentumList, error) {
	ans := &DetailedMomentumList{
		Count: list.Count,
		List:  make([]*DetailedMomentum, len(list.List)),
	}
	for index, momentum := range list.List {
		store := chain.GetFrontierMomentumStore()
		m, err := store.PrefetchMomentum(momentum.Momentum)
		if err != nil {
			return nil, err
		}
		accountBlocks, err := ledgerAccountBlocksToRpc(chain, m.AccountBlocks)
		if err != nil {
			return nil, err
		}
		ans.List[index] = &DetailedMomentum{
			Momentum:      momentum,
			AccountBlocks: accountBlocks,
		}
	}

	return ans, nil
}
func ledgerMomentumToRpc(m *nom.Momentum) (*Momentum, error) {
	if m == nil {
		return nil, nil
	}
	rm := &Momentum{
		Momentum: m,
		Producer: m.Producer(),
	}

	// Populate null fields with empty ones
	if rm.Data == nil {
		rm.Data = make([]byte, 0)
	}
	if rm.Content == nil {
		rm.Content = make(nom.MomentumContent, 0)
	}

	return rm, nil
}
func ledgerMomentumsToRpc(list []*nom.Momentum) ([]*Momentum, error) {
	momentums := make([]*Momentum, 0, len(list))
	for _, momentum := range list {
		if momentum == nil {
		} else {
			rpc, err := ledgerMomentumToRpc(momentum)
			if err != nil {
				return nil, err
			}
			momentums = append(momentums, rpc)
		}
	}

	return momentums, nil
}
func ledgerAccountBlockToRpc(chain chain.Chain, lAb *nom.AccountBlock) (*AccountBlock, error) {
	rpcBlock := &AccountBlock{
		AccountBlock: *lAb.Copy(),
	}
	if err := rpcBlock.addAllExtraInfo(chain); err != nil {
		return nil, err
	}

	return rpcBlock, nil
}
func ledgerAccountBlocksToRpc(chain chain.Chain, list []*nom.AccountBlock) ([]*AccountBlock, error) {
	if list == nil {
		return []*AccountBlock{}, nil
	}

	blocks := make([]*AccountBlock, 0, len(list))
	for _, block := range list {
		if block == nil {
		} else {
			rpc, err := ledgerAccountBlockToRpc(chain, block)
			if err != nil {
				return nil, err
			}
			blocks = append(blocks, rpc)
		}
	}
	return blocks, nil
}

// LedgerTokenInfoToRpc converts a definition.TokenInfo from the
// token-contract storage layer into the public Token wire view.
// Returns nil for a nil input. Used by LedgerApi to attach
// TokenInfo to account blocks and balance entries, and exported
// for reuse by the embedded RPC handlers (rpc/api/embedded/bridge.go)
// that surface token records.
func LedgerTokenInfoToRpc(tokenInfo *definition.TokenInfo) *Token {
	var rt *Token = nil
	if tokenInfo != nil {
		rt = &Token{
			TokenName:          tokenInfo.TokenName,
			TokenSymbol:        tokenInfo.TokenSymbol,
			TokenDomain:        tokenInfo.TokenDomain,
			TotalSupply:        nil,
			MaxSupply:          nil,
			Decimals:           tokenInfo.Decimals,
			Owner:              tokenInfo.Owner,
			ZenonTokenStandard: tokenInfo.TokenStandard,
			IsBurnable:         tokenInfo.IsBurnable,
			IsMintable:         tokenInfo.IsMintable,
			IsUtility:          tokenInfo.IsUtility,
		}
		if tokenInfo.TotalSupply != nil {
			rt.TotalSupply = tokenInfo.TotalSupply
		}
		if tokenInfo.MaxSupply != nil {
			rt.MaxSupply = tokenInfo.MaxSupply
		}
	}
	return rt
}

// LedgerTokenInfosToRpc maps LedgerTokenInfoToRpc over a slice.
// A nil entry in list contributes a nil entry in the returned
// slice (rather than being skipped), preserving 1:1 indexing.
func LedgerTokenInfosToRpc(list []*definition.TokenInfo) []*Token {
	tokenInfos := make([]*Token, 0)
	for _, item := range list {
		tokenInfos = append(tokenInfos, LedgerTokenInfoToRpc(item))
	}

	return tokenInfos
}
