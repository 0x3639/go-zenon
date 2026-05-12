package api

import (
	"time"

	"github.com/inconshreveable/log15"
	"github.com/pkg/errors"

	"github.com/zenon-network/go-zenon/chain"
	"github.com/zenon-network/go-zenon/chain/nom"
	"github.com/zenon-network/go-zenon/common"
	"github.com/zenon-network/go-zenon/common/types"
	"github.com/zenon-network/go-zenon/vm"
	"github.com/zenon-network/go-zenon/zenon"
)

// NewLedgerApi returns a LedgerApi bound to z's chain handle and a
// log15.Logger tagged with module "ledger_api". The full zenon
// handle is kept so PublishRawTransaction can reach the broadcaster.
func NewLedgerApi(z zenon.Zenon) *LedgerApi {
	api := &LedgerApi{
		z:     z,
		chain: z.Chain(),
		log:   common.RPCLogger.New("module", "ledger_api"),
	}

	return api
}

// LedgerApi serves the "ledger" RPC namespace: account-block /
// momentum reads against the frontier plus the
// PublishRawTransaction submission path. All reads go through
// chain.Chain's frontier stores; there is no historical-snapshot
// reader on this type.
type LedgerApi struct {
	z     zenon.Zenon
	chain chain.Chain
	log   log15.Logger
}

// Per-method caps for GetUnreceivedBlocksByAddress. The reader
// over-fetches by querying unreceivedQuerySize hashes (page-index
// ceiling times page-size ceiling = 500) so the per-page filter
// step has enough material to fill the requested page even when
// some hashes have already been received locally.
const (
	unreceivedMaxPageIndex = 10
	unreceivedMaxPageSize  = 50
	unreceivedQuerySize    = unreceivedMaxPageIndex * unreceivedMaxPageSize
)

// String returns the literal "LedgerApi" so the RPC server's
// service-registration code can identify the handler in logs.
func (l LedgerApi) String() string {
	return "LedgerApi"
}

// PublishRawTransaction validates and submits a client-signed
// account block to the local node's broadcaster. The pipeline is:
//
//  1. Reject nil with ErrParamIsNull.
//  2. Reject a block whose ChainIdentifier is set to a different
//     network than the local node (a zero ChainIdentifier passes
//     through unchecked).
//  3. Strip the RPC-decoration fields via ToLedgerBlock.
//  4. Validate the token standard via checkTokenIdValid against
//     the frontier momentum store.
//  5. Confirm a frontier momentum exists.
//  6. Apply the block through vm.Supervisor; if the VM accepts it,
//     hand the resulting transaction to the broadcaster for
//     network propagation.
//
// VM rejection errors propagate unchanged to the caller; broadcast
// is fire-and-forget once ApplyBlock returns. RecoverStack guards
// against panics in the VM apply path.
func (l *LedgerApi) PublishRawTransaction(block *AccountBlock) error {
	defer common.RecoverStack()
	if block == nil {
		return ErrParamIsNull
	}

	if block.ChainIdentifier != 0 && block.ChainIdentifier != l.chain.ChainIdentifier() {
		return errors.Errorf("the block has a different network Id (%d) from the node (%d)", block.ChainIdentifier, l.chain.ChainIdentifier())
	}

	lb, err := block.ToLedgerBlock()
	if err != nil {
		return err
	}
	if err := checkTokenIdValid(l.chain, &lb.TokenStandard); err != nil {
		return err
	}
	m, err := l.chain.GetFrontierMomentumStore().GetFrontierMomentum()
	if m == nil {
		return errors.New("failed to get latest momentum")
	}

	supervisor := vm.NewSupervisor(l.z.Chain(), l.z.Consensus())
	transaction, err := supervisor.ApplyBlock(lb)

	if err != nil {
		return err
	}

	l.z.Broadcaster().CreateAccountBlock(transaction)
	return nil
}

// GetUnconfirmedBlocksByAddress returns a page of address's
// uncommitted account blocks — submitted to this node but not yet
// included in a momentum. Count is the full uncommitted total
// before paging; More is always false (uncommitted blocks live in
// memory and the full set is enumerated by the chain on every
// call). pageSize > RpcMaxPageSize is rejected with
// ErrPageSizeParamTooBig before the chain read.
func (l *LedgerApi) GetUnconfirmedBlocksByAddress(address types.Address, pageIndex, pageSize uint32) (*AccountBlockList, error) {
	if pageSize > RpcMaxPageSize {
		return nil, ErrPageSizeParamTooBig
	}

	unreceived := l.chain.GetUncommittedAccountBlocksByAddress(address)
	start, end := GetRange(pageIndex, pageSize, uint32(len(unreceived)))
	a, err := ledgerAccountBlocksToRpc(l.chain, unreceived[start:end])

	if err != nil {
		return nil, err
	}

	return &AccountBlockList{
		List:  a,
		Count: len(unreceived),
		More:  false,
	}, nil
}

// GetFrontierAccountBlock returns address's most recent committed
// account block (i.e. the one with the highest Height in address's
// account chain), or (nil, nil) when no block has been recorded
// for address. The returned block carries the full AccountBlock
// decoration (token, confirmation detail, paired block).
func (l *LedgerApi) GetFrontierAccountBlock(address types.Address) (*AccountBlock, error) {
	accountStore := l.chain.GetFrontierAccountStore(address)
	block, err := accountStore.Frontier()
	if err != nil {
		return nil, err
	}
	if block == nil {
		return nil, nil
	}
	return ledgerAccountBlockToRpc(l.chain, block)
}

// GetAccountBlockByHash returns the committed account block with
// the given hash, or (nil, nil) when no such block is recorded.
// Storage errors propagate unchanged (logged at error level
// before returning).
func (l *LedgerApi) GetAccountBlockByHash(blockHash types.Hash) (*AccountBlock, error) {
	momentumStore := l.chain.GetFrontierMomentumStore()
	block, err := momentumStore.GetAccountBlockByHash(blockHash)
	if err != nil {
		l.log.Error("GetAccountBlockByHash failed", "reason", err, "method-called", "momentumStore.GetAccountBlockByHash")
		return nil, err
	}
	if block == nil {
		return nil, nil
	}

	return ledgerAccountBlockToRpc(l.chain, block)
}

// GetAccountBlocksByHeight returns up to count of address's
// committed account blocks starting at height, in ascending order
// of block height. height must be strictly positive (account-block
// heights are 1-indexed); height == 0 returns ErrHeightParamIsZero.
// count > RpcMaxCountSize returns ErrCountParamTooBig. If address
// has no account chain the result is an empty list with Count = 0
// rather than an error. The returned Count is address's frontier
// height (not the size of List), so clients can detect "this is
// the whole chain to here".
func (l *LedgerApi) GetAccountBlocksByHeight(address types.Address, height, count uint64) (*AccountBlockList, error) {
	if height == 0 {
		return nil, ErrHeightParamIsZero
	}
	if count > RpcMaxCountSize {
		return nil, ErrCountParamTooBig
	}

	accountStore := l.chain.GetFrontierAccountStore(address)
	frontier, err := accountStore.Frontier()
	if err != nil {
		l.log.Error("GetAccountBlocksByHeight failed", "reason", err, "method-called", "accountStore.Frontier")
		return nil, err
	}
	if frontier == nil {
		return &AccountBlockList{
			List:  make([]*AccountBlock, 0),
			Count: 0,
		}, nil
	}

	accountBlocks, err := accountStore.MoreByHeight(height, count)
	if err != nil {
		l.log.Error("GetAccountBlocksByHeight failed", "reason", err, "method-called", "GetAccountBlocksByHeight")
		return nil, err
	}

	list, err := ledgerAccountBlocksToRpc(l.chain, accountBlocks)
	if err != nil {
		l.log.Error("GetAccountBlocksByHeight failed", "reason", err, "method-called", "ledgerAccountBlocksToRpc")
		return nil, err
	}

	return &AccountBlockList{
		List:  list,
		Count: int(frontier.Height),
	}, nil
}

// GetAccountBlocksByPage returns one page of address's committed
// account blocks in descending order of block height (newest
// first). Pagination walks backwards from the account's frontier:
// page 0 is the most recent pageSize blocks, page 1 the pageSize
// before those, and so on. Pages near the genesis end are
// truncated rather than empty when the requested range crosses
// height 1. pageSize > RpcMaxPageSize returns ErrPageSizeParamTooBig.
// If the page falls entirely before the start of the chain
// (pageIndex too high), the result is an empty list with Count
// set to the frontier height.
//
// Internally this delegates to GetAccountBlocksByHeight (which
// returns ascending) and reverses the slice in place.
func (l *LedgerApi) GetAccountBlocksByPage(address types.Address, pageIndex, pageSize uint32) (*AccountBlockList, error) {
	if pageSize > RpcMaxPageSize {
		return nil, ErrPageSizeParamTooBig
	}

	accountStore := l.chain.GetFrontierAccountStore(address)
	frontier, err := accountStore.Frontier()
	if err != nil {
		l.log.Error("GetAccountBlocksByHeight failed", "reason", err, "method-called", "accountStore.Frontier")
		return nil, err
	}
	if frontier == nil {
		return &AccountBlockList{
			List:  make([]*AccountBlock, 0),
			Count: 0,
		}, nil
	}

	startHeight := int64(frontier.Height) - int64(pageIndex+1)*int64(pageSize) + 1
	count := int64(pageSize)
	tooMuch := 1 - startHeight
	if tooMuch > 0 {
		startHeight = 1
		count -= tooMuch
	}
	if count < 1 {
		return &AccountBlockList{
			Count: int(frontier.Height),
			More:  false,
			List:  []*AccountBlock{},
		}, nil
	}

	ans, err := l.GetAccountBlocksByHeight(address, uint64(startHeight), uint64(count))
	if err != nil {
		return nil, err
	}

	for i, j := 0, len(ans.List)-1; i < j; i, j = i+1, j-1 {
		ans.List[i], ans.List[j] = ans.List[j], ans.List[i]
	}
	return ans, nil
}

// GetAccountInfoByAddress returns address's account-chain height
// and per-token balance map at the frontier. Tokens whose registry
// lookup fails (e.g. a balance entry for a no-longer-registered
// ZTS) are silently dropped from the result; the map does not
// include zero-balance entries for tokens the account never held.
// An address with no account chain returns AccountHeight = 0 and
// the live balance map (which may still be non-empty if the
// account has received but not yet processed transfers).
func (l *LedgerApi) GetAccountInfoByAddress(address types.Address) (*AccountInfo, error) {
	l.log.Info("GetAccountInfoByAddress")

	momentumStore := l.chain.GetFrontierMomentumStore()
	accountStore := l.chain.GetFrontierAccountStore(address)
	frontierAccountBlock, err := accountStore.Frontier()
	if err != nil {
		l.log.Error("GetFrontierAccountBlock failed, error is "+err.Error(), "method", "GetAccountInfoByAddress")
		return nil, err
	}

	totalNum := uint64(0)
	if frontierAccountBlock != nil {
		totalNum = frontierAccountBlock.Height
	}

	balanceMap, err := accountStore.GetBalanceMap()
	if err != nil {
		l.log.Error("GetAccountBalance failed, error is "+err.Error(), "method", "GetAccountInfoByAddress")
		return nil, err
	}

	balanceInfoMap := make(map[types.ZenonTokenStandard]*BalanceInfo)

	for zts, balance := range balanceMap {
		tokenInfo, _ := momentumStore.GetTokenInfoByTs(zts)
		if tokenInfo == nil {
			continue
		}

		balanceInfoMap[zts] = &BalanceInfo{
			TokenInfo: LedgerTokenInfoToRpc(tokenInfo),
			Balance:   balance,
		}
	}

	return &AccountInfo{
		Address:        address,
		AccountHeight:  totalNum,
		BalanceInfoMap: balanceInfoMap,
	}, nil
}

// GetUnreceivedBlocksByAddress returns a page of send blocks
// destined for address that the address has not yet acknowledged
// with a receive block.
//
// Bounds: pageSize > unreceivedMaxPageSize (50) returns
// ErrPageSizeParamTooBig; pageIndex >= unreceivedMaxPageIndex
// (10) returns ErrPageIndexParamTooBig. Together these cap the
// effective query window at 500 blocks per address per call.
//
// Implementation: the chain mailbox is queried for up to
// unreceivedQuerySize (500) candidate hashes. Each hash is
// dropped if address's local account store records it as already
// received (the chain mailbox can lag the local receive record
// during sync). The filtered list is paged. More is set to true
// when the underlying mailbox returned the full 500 hashes,
// signalling that there may be additional records past the
// current window.
func (l *LedgerApi) GetUnreceivedBlocksByAddress(address types.Address, pageIndex, pageSize uint32) (*AccountBlockList, error) {
	l.log.Info("GetUnreceivedBlocksByAddress", "address", address, "page", pageIndex, "size", pageSize)
	if pageSize > unreceivedMaxPageSize {
		return nil, ErrPageSizeParamTooBig
	}
	if pageIndex >= unreceivedMaxPageIndex {
		return nil, ErrPageIndexParamTooBig
	}

	accountStore := l.chain.GetFrontierAccountStore(address)
	hashList, err := l.chain.GetFrontierMomentumStore().GetAccountMailbox(address).GetUnreceivedAccountBlockHashes(unreceivedQuerySize)
	if err != nil {
		return nil, err
	}

	ledgerFrontier := l.chain.GetFrontierMomentumStore()
	blockList := make([]*nom.AccountBlock, 0, len(hashList))
	for _, hash := range hashList {
		if accountStore.IsReceived(hash) {
			continue
		}
		block, err := ledgerFrontier.GetAccountBlockByHash(hash)

		if err != nil {
			return nil, err
		}
		blockList = append(blockList, block)
	}

	// Check if there are 100% more blocks that could've been returned
	isMore := false
	if len(hashList) == unreceivedQuerySize {
		isMore = true
	}

	start, end := GetRange(pageIndex, pageSize, uint32(len(blockList)))
	a, err := ledgerAccountBlocksToRpc(l.chain, blockList[start:end])

	if err != nil {
		return nil, err
	}

	return &AccountBlockList{
		List:  a,
		Count: len(blockList),
		More:  isMore,
	}, nil
}

// GetFrontierMomentum returns the chain's most recent committed
// momentum decorated with its producer address. Storage errors
// propagate unchanged.
func (l *LedgerApi) GetFrontierMomentum() (*Momentum, error) {
	momentum, err := l.chain.GetFrontierMomentumStore().GetFrontierMomentum()
	if err != nil {
		return nil, err
	}
	return ledgerMomentumToRpc(momentum)
}

// GetMomentumBeforeTime returns the most recent momentum whose
// timestamp is strictly less than the supplied unix timestamp,
// or (nil, nil) when no such momentum exists (timestamp before
// genesis). The lookup delegates to MomentumStore.GetMomentumBeforeTime;
// errors propagate unchanged.
func (l *LedgerApi) GetMomentumBeforeTime(timestamp int64) (*Momentum, error) {
	currentTime := time.Unix(timestamp, 0)
	momentum, err := l.chain.GetFrontierMomentumStore().GetMomentumBeforeTime(&currentTime)
	if err != nil || momentum == nil {
		return nil, err
	}

	return ledgerMomentumToRpc(momentum)
}

// GetMomentumByHash returns the momentum with the given hash, or
// (nil, nil) when no such momentum is recorded. Storage errors
// are logged at error level and propagated unchanged.
func (l *LedgerApi) GetMomentumByHash(hash types.Hash) (*Momentum, error) {
	block, err := l.chain.GetFrontierMomentumStore().GetMomentumByHash(hash)
	if err != nil {
		l.log.Error("GetMomentumByHash failed, error is "+err.Error(), "method", "GetMomentumByHash")
		return nil, err
	}
	return ledgerMomentumToRpc(block)
}

// GetMomentumsByHeight returns up to count committed momentums
// starting at height, ascending order. Bounds: height == 0
// returns ErrHeightParamIsZero (momentum heights are 1-indexed);
// count > RpcMaxCountSize returns ErrCountParamTooBig.
//
// Count in the returned MomentumList is the chain frontier height,
// not the size of List, so clients can detect "this is the tail".
func (l *LedgerApi) GetMomentumsByHeight(height, count uint64) (*MomentumList, error) {
	if height == 0 {
		return nil, ErrHeightParamIsZero
	}
	if count > RpcMaxCountSize {
		return nil, ErrCountParamTooBig
	}

	momentumStore := l.chain.GetFrontierMomentumStore()
	frontier, err := momentumStore.GetFrontierMomentum()
	if err != nil {
		l.log.Error("GetMomentumsByHeight failed", "reason", err, "method-called", "momentumStore.GetFrontierMomentum")
		return nil, err
	}

	momentums, err := momentumStore.GetMomentumsByHeight(height, true, count)
	if err != nil {
		l.log.Error("GetMomentumsByHeight failed", "reason", err, "method-called", "momentumStore.GetMomentumsByHeight")
		return nil, err
	}

	list, err := ledgerMomentumsToRpc(momentums)
	if err != nil {
		l.log.Error("GetMomentumsByHeight failed", "reason", err, "method-called", "ledgerMomentumsToRpc")
		return nil, err
	}

	return &MomentumList{
		List:  list,
		Count: int(frontier.Height),
	}, nil
}

// GetMomentumsByPage returns one page of committed momentums in
// descending order of height (newest first). Pagination walks
// backwards from the chain frontier; pages near genesis are
// truncated rather than empty when the requested range crosses
// height 1. pageSize > RpcMaxPageSize returns ErrPageSizeParamTooBig.
// If pageIndex is high enough that the page falls entirely before
// genesis, the result is an empty list with Count set to the
// frontier height.
//
// Internally delegates to GetMomentumsByHeight (which returns
// ascending) and reverses the slice in place.
func (l *LedgerApi) GetMomentumsByPage(pageIndex, pageSize uint32) (*MomentumList, error) {
	if pageSize > RpcMaxPageSize {
		return nil, ErrPageSizeParamTooBig
	}

	momentumStore := l.chain.GetFrontierMomentumStore()
	frontier, err := momentumStore.GetFrontierMomentum()
	if err != nil {
		l.log.Error("GetMomentumsByPage failed", "reason", err, "method-called", "momentumStore.GetFrontierMomentum")
		return nil, err
	}

	startHeight := int64(frontier.Height) - int64(pageIndex+1)*int64(pageSize) + 1
	count := int64(pageSize)
	tooMuch := 1 - startHeight
	if tooMuch > 0 {
		startHeight = 1
		count -= tooMuch
	}
	if count < 1 {
		return &MomentumList{
			Count: int(frontier.Height),
			List:  []*Momentum{},
		}, nil
	}

	ans, err := l.GetMomentumsByHeight(uint64(startHeight), uint64(count))
	if err != nil {
		return nil, err
	}

	for i, j := 0, len(ans.List)-1; i < j; i, j = i+1, j-1 {
		ans.List[i], ans.List[j] = ans.List[j], ans.List[i]
	}
	return ans, nil
}

// GetDetailedMomentumsByHeight returns up to count momentums
// starting at height (ascending), each paired with the full
// AccountBlock view of every account block included in that
// momentum. Unlike GetMomentumsByHeight, this method has no
// height-zero guard at the RPC layer — height == 0 is rejected
// downstream by GetMomentumsByHeight. count > RpcMaxCountSize
// returns ErrCountParamTooBig.
//
// This is the heavier of the two height-based momentum readers
// because it eagerly prefetches every block in each momentum;
// callers that only need the momentum headers should prefer
// GetMomentumsByHeight.
func (l *LedgerApi) GetDetailedMomentumsByHeight(height, count uint64) (*DetailedMomentumList, error) {
	l.log.Info("GetDetailedMomentumsByHeight", "height", height, "count", count)
	if count > RpcMaxCountSize {
		return nil, ErrCountParamTooBig
	}

	ans, err := l.GetMomentumsByHeight(height, count)
	if err != nil {
		return nil, err
	}
	return momentumListToDetailedList(l.chain, ans)
}
