package protocol

import (
	"fmt"

	"github.com/zenon-network/go-zenon/chain"
	"github.com/zenon-network/go-zenon/chain/nom"
	"github.com/zenon-network/go-zenon/common"
)

type broadcaster struct {
	log      common.Logger
	chain    chain.Chain
	protocol *ProtocolManager
}

func NewBroadcaster(chain chain.Chain, protocol *ProtocolManager) Broadcaster {
	return &broadcaster{
		log:      common.ProtocolLogger.New("submodule", "broadcaster"),
		chain:    chain,
		protocol: protocol,
	}
}

func (b *broadcaster) SyncInfo() *SyncInfo {
	return b.protocol.SyncInfo()
}

func (b *broadcaster) GetPeerCount() int {
	return b.protocol.peers.Len()
}

// CreateMomentum is called when our node created a momentum.
// The momentum will be inserted in the chain and broadcasted.
func (b *broadcaster) CreateMomentum(momentumTransaction *nom.MomentumTransaction) {
	b.log.Info("creating own momentum", "identifier", momentumTransaction.Momentum.Identifier())
	insert := b.chain.AcquireInsert(fmt.Sprintf("zenon - create momentum %v", momentumTransaction.Momentum.Identifier()))
	err := b.chain.AddMomentumTransaction(insert, momentumTransaction)
	insert.Unlock()
	if err != nil {
		b.log.Error("failed to insert own momentum", "reason", err)
		return
	}

	store := b.chain.GetFrontierMomentumStore()
	detailed, err := store.PrefetchMomentum(momentumTransaction.Momentum)
	if err != nil {
		b.log.Error("failed to insert own momentum", "reason", err)
		return
	}

	b.log.Info("broadcasting own momentum", "identifier", momentumTransaction.Momentum.Identifier())
	b.protocol.BroadcastMomentum(detailed, true)
}

// CreateAccountBlock is called when our node created an account block.
// The account-block will be inserted in the chain and broadcasted.
func (b *broadcaster) CreateAccountBlock(accountBlockTransaction *nom.AccountBlockTransaction) {
	insert := b.chain.AcquireInsert(fmt.Sprintf("zenon - create account-block %v", accountBlockTransaction.Block.Header()))
	err := b.chain.AddAccountBlockTransaction(insert, accountBlockTransaction)
	insert.Unlock()
	if err != nil {
		b.log.Error("failed to insert own account-block", "reason", err)
		return
	}

	// Diagnostic logging: track transaction creation
	if diagnosticLogger := common.GetDiagnosticLogger(); diagnosticLogger != nil {
		peerCount := b.protocol.peers.Len()
		diagnosticLogger.LogAccountBlockAdded(
			accountBlockTransaction.Block.Hash.String(),
			accountBlockTransaction.Block.Address.String(),
			accountBlockTransaction.Block.Height,
			"local_create",
		)
		// Broadcast will be logged in handler.go with peer details
		b.log.Debug("diagnostic: created account block", "hash", accountBlockTransaction.Block.Hash, "peer-count", peerCount)
	}

	b.protocol.BroadcastAccountBlock(accountBlockTransaction.Block)
}
