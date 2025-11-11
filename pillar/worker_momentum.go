package pillar

import (
	"github.com/zenon-network/go-zenon/chain/nom"
	"github.com/zenon-network/go-zenon/common"
	"github.com/zenon-network/go-zenon/consensus"
)

func (w *worker) generateMomentum(e consensus.ProducerEvent) (*nom.MomentumTransaction, error) {
	startTime := common.Clock.Now()

	insert := w.chain.AcquireInsert("momentum-generator")
	defer insert.Unlock()

	store := w.chain.GetFrontierMomentumStore()
	peerCount := w.broadcaster.GetPeerCount()
	blocks := w.chain.GetNewMomentumContent(peerCount)

	previousMomentum, err := store.GetFrontierMomentum()
	if err != nil {
		return nil, err
	}

	m := &nom.Momentum{
		ChainIdentifier: w.chain.ChainIdentifier(),
		PreviousHash:    previousMomentum.Hash,
		Height:          previousMomentum.Height + 1,
		TimestampUnix:   uint64(e.StartTime.Unix()),
		Content:         nom.NewMomentumContent(blocks),
		Version:         uint64(1),
	}
	m.EnsureCache()

	momentumTx, err := w.supervisor.GenerateMomentum(&nom.DetailedMomentum{
		Momentum:      m,
		AccountBlocks: blocks,
	}, w.coinbase.Signer)

	productionTimeMs := common.Clock.Now().Sub(startTime).Milliseconds()

	// Diagnostic logging: track momentum production
	if err == nil && momentumTx != nil {
		if diagnosticLogger := common.GetDiagnosticLogger(); diagnosticLogger != nil {
			diagnosticLogger.LogMomentumProduced(
				momentumTx.Momentum.Hash.String(),
				momentumTx.Momentum.Height,
				len(blocks),
				productionTimeMs,
			)
		}
	}

	return momentumTx, err
}
