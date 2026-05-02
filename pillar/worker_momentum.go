package pillar

import (
	"github.com/zenon-network/go-zenon/chain/nom"
	"github.com/zenon-network/go-zenon/consensus"
)

// generateMomentum acquires the chain insert lock, snapshots the
// pending-block content, and asks the supervisor to produce and
// sign a momentum at the current frontier+1 with timestamp =
// slot StartTime. Holds the insert lock for the full call to
// avoid racing with concurrent inserts.
func (w *worker) generateMomentum(e consensus.ProducerEvent) (*nom.MomentumTransaction, error) {
	insert := w.chain.AcquireInsert("momentum-generator")
	defer insert.Unlock()

	store := w.chain.GetFrontierMomentumStore()
	blocks := w.chain.GetNewMomentumContent()

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
	return w.supervisor.GenerateMomentum(&nom.DetailedMomentum{
		Momentum:      m,
		AccountBlocks: blocks,
	}, w.coinbase.Signer)
}
