package genesis

import (
	"time"

	"github.com/zenon-network/go-zenon/chain"
	"github.com/zenon-network/go-zenon/chain/nom"
	"github.com/zenon-network/go-zenon/common"
	"github.com/zenon-network/go-zenon/vm"
)

// newGenesisMomentum constructs the genesis [nom.MomentumTransaction]
// from genesisConfig and the seeded account pool: assembles a
// height-1 momentum referencing every seeded account block, then runs
// the VM's [vm.Supervisor.GenerateGenesisMomentum] to derive the matching
// state patch. The genesis momentum is the only momentum that does not
// flow through the verifier — its content is fixed by the embedded
// configuration.
//
// Panics on supervisor failure — a malformed genesis is unrecoverable.
func newGenesisMomentum(genesisConfig *GenesisConfig, pool chain.AccountPool) *nom.MomentumTransaction {
	timestamp := time.Unix(genesisConfig.GenesisTimestampSec, 0)
	blocks := pool.GetAllUncommittedAccountBlocks()

	supervisor := vm.NewSupervisor(nil, nil)
	// genesis momentum does not go through the verifier
	m := &nom.Momentum{
		Version:         1,
		ChainIdentifier: genesisConfig.ChainIdentifier,
		Height:          1, // height
		TimestampUnix:   uint64(timestamp.Unix()),
		Data:            []byte(genesisConfig.ExtraData),
		Content:         nom.NewMomentumContent(blocks),
	}
	m.EnsureCache()
	transaction, err := supervisor.GenerateGenesisMomentum(m, pool)
	common.DealWithErr(err)

	return transaction
}
