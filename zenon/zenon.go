package zenon

import (
	"github.com/syndtr/goleveldb/leveldb"

	"github.com/zenon-network/go-zenon/chain"
	"github.com/zenon-network/go-zenon/consensus"
	"github.com/zenon-network/go-zenon/pillar"
	"github.com/zenon-network/go-zenon/protocol"
	"github.com/zenon-network/go-zenon/rpc/api/subscribe"
	"github.com/zenon-network/go-zenon/verifier"
	"github.com/zenon-network/go-zenon/vm"
)

// zenon is the production [Zenon] implementation. Holds one handle
// per managed subsystem plus the consensus leveldb so [zenon.Stop]
// can close it. Field order mirrors construction order in [NewZenon].
type zenon struct {
	config *Config

	protocol    *protocol.ProtocolManager
	subscribe   *subscribe.Server
	verifier    verifier.Verifier
	chain       chain.Chain
	pillar      pillar.Manager
	consensus   consensus.Consensus
	evPrinter   EventPrinter
	broadcaster protocol.Broadcaster
	levelDb     *leveldb.DB
}

// NewZenon constructs a fully-wired Zenon facade ready for [Init] /
// [Start]. Sets the pillar coinbase if cfg.ProducingKeyPair is
// non-nil; nil leaves the node in non-producing mode.
func NewZenon(cfg *Config) (Zenon, error) {
	z := &zenon{
		config: cfg,
	}

	z.chain = chain.NewChain(cfg.NewDBManager("nom"), cfg.GenesisConfig)
	db, levelDb := cfg.NewLevelDB("consensus")
	z.consensus = consensus.NewConsensus(db, z.chain, false)
	z.verifier = verifier.NewVerifier(z.chain, z.consensus)
	z.levelDb = levelDb

	chainBridge := protocol.NewChainBridge(z.chain, z.consensus, z.verifier, vm.NewSupervisor(z.chain, z.consensus))
	z.protocol = protocol.NewProtocolManager(cfg.MinPeers, z.chain.ChainIdentifier(), chainBridge)
	z.broadcaster = protocol.NewBroadcaster(z.chain, z.protocol)

	z.evPrinter = NewEventPrinter(z.chain, z.broadcaster)
	z.subscribe = subscribe.GetSubscribeServer(z.chain)
	z.pillar = pillar.NewPillar(z.chain, z.consensus, z.broadcaster)

	if cfg.ProducingKeyPair != nil {
		z.pillar.SetCoinBase(cfg.ProducingKeyPair)
	}

	return z, nil
}

// Init walks every subsystem's Init in fixed order: chain →
// consensus → event printer → subscription → pillar. Returns the
// first error encountered. Protocol Init is intentionally skipped
// — protocol has no Init step.
func (z *zenon) Init() error {
	if err := z.chain.Init(); err != nil {
		return err
	}
	if err := z.consensus.Init(); err != nil {
		return err
	}
	if err := z.evPrinter.Init(); err != nil {
		return err
	}
	if err := z.subscribe.Init(); err != nil {
		return err
	}
	//z.protocol.Init()
	if err := z.pillar.Init(); err != nil {
		return err
	}

	return nil
}

// Start launches each subsystem in the same fixed order as Init,
// with protocol started last so it cannot request blocks before
// the verifier and chain are live.
func (z *zenon) Start() error {
	if err := z.chain.Start(); err != nil {
		return err
	}
	if err := z.consensus.Start(); err != nil {
		return err
	}
	if err := z.evPrinter.Start(); err != nil {
		return err
	}
	if err := z.subscribe.Start(); err != nil {
		return err
	}
	if err := z.pillar.Start(); err != nil {
		return err
	}
	z.protocol.Start()

	return nil
}

// Stop unwinds the subsystems in reverse-dependency order:
// protocol → pillar → subscribe → printer → consensus → chain →
// leveldb. Returns the first error encountered.
func (z *zenon) Stop() error {
	z.protocol.Stop()
	if err := z.pillar.Stop(); err != nil {
		return err
	}
	if err := z.subscribe.Stop(); err != nil {
		return err
	}
	if err := z.evPrinter.Stop(); err != nil {
		return err
	}
	if err := z.consensus.Stop(); err != nil {
		return err
	}
	if err := z.chain.Stop(); err != nil {
		return err
	}
	if err := z.levelDb.Close(); err != nil {
		return err
	}

	return nil
}

// Chain returns the chain subsystem.
func (z *zenon) Chain() chain.Chain {
	return z.chain
}

// Producer returns the pillar (block-producer) manager.
func (z *zenon) Producer() pillar.Manager {
	return z.pillar
}

// Consensus returns the consensus subsystem.
func (z *zenon) Consensus() consensus.Consensus {
	return z.consensus
}

// Verifier returns the block-verifier.
func (z *zenon) Verifier() verifier.Verifier {
	return z.verifier
}

// Protocol returns the wire-protocol manager.
func (z *zenon) Protocol() *protocol.ProtocolManager {
	return z.protocol
}

// Config returns the original Config used to construct this Zenon.
// Treat as read-only — mutating it after construction has no
// effect.
func (z *zenon) Config() *Config {
	return z.config
}

// Broadcaster returns the local-block broadcaster used by the RPC
// publish path.
func (z *zenon) Broadcaster() protocol.Broadcaster {
	return z.broadcaster
}
