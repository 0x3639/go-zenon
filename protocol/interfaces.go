package protocol

import (
	"github.com/zenon-network/go-zenon/chain/nom"
	"github.com/zenon-network/go-zenon/common"
	"github.com/zenon-network/go-zenon/common/types"
)

// SyncState reports where a node sits in the catch-up cycle.
// Surfaced through [SyncInfo] for RPC consumers and used by the
// protocol manager to gate which peer messages it processes.
type SyncState int

// log is the package-level protocol logger.
var (
	log = common.ProtocolLogger
)

// Sync state discriminators.
const (
	// Unknown is the initial state before sync has been attempted.
	Unknown SyncState = iota
	// Syncing means the node is actively pulling blocks from peers.
	Syncing
	// SyncDone means the node's frontier matches the network.
	SyncDone
	// NotEnoughPeers means the node cannot start syncing because
	// it lacks the minimum peer count.
	NotEnoughPeers
)

// SyncInfo is the RPC-friendly view of the protocol manager's
// sync progress: current state plus the (current, target) height
// pair so clients can show a percentage.
type SyncInfo struct {
	State         SyncState `json:"state"`
	CurrentHeight uint64    `json:"currentHeight"`
	TargetHeight  uint64    `json:"targetHeight"`
}

// txPool is the protocol layer's view of the unconfirmed-account-block
// pool: it can admit incoming blocks from peers and surface pending
// blocks to be relayed.
type txPool interface {
	// AddAccountBlocks admits blocks into the local chain pool.
	AddAccountBlocks([]*nom.AccountBlock) error

	// GetTransactions should return pending transactions.
	// The slice should be modifiable by the caller.
	GetTransactions() []*nom.AccountBlock
}

// chainManager is the protocol layer's view of the momentum chain.
// It exposes the read paths the downloader/fetcher need plus the
// mutation entry-point ([chainManager.InsertChain]) the bulk-sync
// uses to commit a batch of momentums.
type chainManager interface {
	// HasBlock reports whether hash is known.
	HasBlock(hash types.Hash) bool
	// GetBlockHashesFromHash returns up to amount predecessor
	// hashes ending at hash; used by sync to walk back from a peer
	// announcement to a common ancestor.
	GetBlockHashesFromHash(hash types.Hash, amount uint64) ([]types.Hash, error)
	// GetBlock returns the [nom.DetailedMomentum] for hash.
	GetBlock(hash types.Hash) (block *nom.DetailedMomentum)
	// GetBlockByNumber returns the momentum at height num.
	GetBlockByNumber(num uint64) (*nom.Momentum, error)
	// CurrentBlock returns the local frontier momentum.
	CurrentBlock() *nom.Momentum
	// Status returns the (height, frontier hash, genesis hash)
	// triple the protocol's status handshake exchanges with peers.
	Status() (td uint64, currentBlock types.Hash, genesisBlock types.Hash)

	// InsertChain admits a contiguous batch of momentums (with
	// their account blocks). Returns the number of momentums
	// successfully inserted before any error.
	InsertChain(chain []*nom.DetailedMomentum) (int, error)
}

// ChainBridge is the protocol layer's full view of the chain:
// transaction pool + chain manager. Implemented by [chainBridge]
// and consumed by [ProtocolManager] / [downloader] / [fetcher].
type ChainBridge interface {
	txPool
	chainManager
}

// Broadcaster is the local-side surface for self-produced blocks:
// the pillar / RPC submit blocks here, and the broadcaster commits
// them to the chain and announces them to peers.
type Broadcaster interface {
	// SyncInfo returns the current sync status (proxied from the
	// protocol manager).
	SyncInfo() *SyncInfo
	// CreateMomentum is called when our node created a momentum.
	// The momentum will be inserted in the chain and broadcasted.
	CreateMomentum(*nom.MomentumTransaction)
	// CreateAccountBlock is called when our node created an
	// account block. The block will be inserted in the chain and
	// broadcasted.
	CreateAccountBlock(*nom.AccountBlockTransaction)
}
