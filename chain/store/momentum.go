package store

import (
	"math/big"
	"time"

	"github.com/zenon-network/go-zenon/chain/nom"
	"github.com/zenon-network/go-zenon/common/db"
	"github.com/zenon-network/go-zenon/common/types"
	"github.com/zenon-network/go-zenon/vm/embedded/definition"
)

// Momentum is the read/write surface for the global momentum chain at a
// specific point in time. It is the unifying view consumers use: any
// chain-state question (which momentum, which account block, what's the
// active spork set, who are the active pillars, what's the balance of X)
// is answered through a [Momentum].
//
// Embeds [Genesis] so the chain identifier and genesis constants are
// always reachable from a momentum view.
//
// Concurrency: snapshots taken via [Momentum.Snapshot] are not
// goroutine-safe; derive a fresh snapshot per goroutine.
type Momentum interface {
	Genesis

	// Identifier returns the (Hash, Height) of the frontier momentum in
	// this view.
	Identifier() types.HashHeight
	// GetFrontierMomentum returns the frontier momentum in this view.
	GetFrontierMomentum() (*nom.Momentum, error)
	// GetMomentumByHash looks up a momentum by hash.
	GetMomentumByHash(types.Hash) (*nom.Momentum, error)
	// GetMomentumByHeight looks up a momentum by height.
	GetMomentumByHeight(uint64) (*nom.Momentum, error)

	// GetAccountBlock looks up the block named by header (address +
	// hash + height).
	GetAccountBlock(types.AccountHeader) (*nom.AccountBlock, error)
	// GetFrontierAccountBlock returns the frontier block of the
	// supplied account chain.
	GetFrontierAccountBlock(types.Address) (*nom.AccountBlock, error)
	// GetAccountBlockByHash looks up an account block by hash without
	// needing the address.
	GetAccountBlockByHash(types.Hash) (*nom.AccountBlock, error)
	// GetAccountBlockByHeight looks up the block at height in the
	// supplied account chain.
	GetAccountBlockByHeight(types.Address, uint64) (*nom.AccountBlock, error)

	// Range queries

	// GetAccountBlocksByHeight returns up to count blocks starting at
	// height in the supplied account chain, in ascending order.
	GetAccountBlocksByHeight(address types.Address, height, count uint64) ([]*nom.AccountBlock, error)
	// GetMomentumsByHash returns up to count momentums starting at the
	// momentum identified by blockHash, walking forward (higher == true)
	// or backward.
	GetMomentumsByHash(blockHash types.Hash, higher bool, count uint64) ([]*nom.Momentum, error)
	// GetMomentumsByHeight returns up to count momentums starting at
	// height, walking forward or backward by `higher`.
	GetMomentumsByHeight(height uint64, higher bool, count uint64) ([]*nom.Momentum, error)
	// GetMomentumBeforeTime returns the most recent momentum whose
	// timestamp is before timestamp; useful for time-anchored queries.
	GetMomentumBeforeTime(timestamp *time.Time) (*nom.Momentum, error)
	// PrefetchMomentum bundles momentum together with the full account
	// blocks it commits to, returning the [nom.DetailedMomentum] form
	// the verifier consumes.
	PrefetchMomentum(momentum *nom.Momentum) (*nom.DetailedMomentum, error)

	// Unreceived

	// GetBlockWhichReceives returns the receive block that consumed the
	// send identified by hash, or nil if the send is still pending.
	GetBlockWhichReceives(hash types.Hash) (*nom.AccountBlock, error)

	// Confirmed

	// GetBlockConfirmationHeight returns the momentum height at which
	// hash was confirmed (the height the verifier uses to validate
	// MomentumAcknowledged for auto-generated blocks).
	GetBlockConfirmationHeight(hash types.Hash) (uint64, error)

	// Embedded

	// GetAllDefinedSporks returns every spork record (active and pending)
	// recorded by the spork contract.
	GetAllDefinedSporks() ([]*definition.Spork, error)
	// GetActivePillars returns the registered pillar set whose
	// registration is still active.
	GetActivePillars() ([]*definition.PillarInfo, error)
	// IsSporkActive reports whether the supplied spork has been
	// activated at this view's height.
	IsSporkActive(*types.ImplementedSpork) (bool, error)
	// GetStakeBeneficialAmount returns the total stake amount addr is
	// the beneficial owner of (the input to per-account staking rewards).
	GetStakeBeneficialAmount(addr types.Address) (*big.Int, error)
	// GetTokenInfoByTs returns the issuance metadata for the token
	// identified by ts.
	GetTokenInfoByTs(ts types.ZenonTokenStandard) (*definition.TokenInfo, error)
	// ComputePillarDelegations re-derives every pillar's aggregated
	// delegation weight (and the per-backer breakdown) from the current
	// stake and vote records. Used by the consensus layer when
	// constructing election snapshots.
	ComputePillarDelegations() ([]*types.PillarDelegationDetail, error)

	// GetAccountStore returns the [Account] view for address pinned at
	// this momentum's identifier.
	GetAccountStore(address types.Address) Account
	// GetAccountDB returns the raw [db.DB] for address — used by code
	// that reads contract storage directly (e.g., RPC range queries).
	GetAccountDB(address types.Address) db.DB
	// GetAccountMailbox returns the mailbox for address pinned at this
	// view.
	GetAccountMailbox(address types.Address) AccountMailbox

	// Snapshot returns an isolated copy of this view.
	Snapshot() Momentum
	// Changes returns the patch capturing every write made to this view
	// since it was created.
	Changes() (db.Patch, error)

	// AddAccountBlockTransaction admits the (header, patch) pair into
	// this view, marking the named account block as part of the
	// in-progress momentum's content.
	AddAccountBlockTransaction(header types.AccountHeader, patch db.Patch) error
}
