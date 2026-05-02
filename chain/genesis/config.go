package genesis

import (
	"encoding/json"
	"math/big"
	"os"

	"github.com/zenon-network/go-zenon/chain/store"
	"github.com/zenon-network/go-zenon/common"
	"github.com/zenon-network/go-zenon/common/types"
	"github.com/zenon-network/go-zenon/vm/embedded/definition"
)

// log is the per-submodule logger used by the genesis loader.
var (
	log = common.ChainLogger.New("submodule", "genesis")
)

// MakeEmbeddedGenesisConfig builds a [store.Genesis] from the
// alphanet-genesis JSON embedded in this binary at build time. Returns
// [ErrNoEmbeddedGenesis] when the binary was built without an embedded
// genesis (e.g., a stripped libznn build).
func MakeEmbeddedGenesisConfig() (store.Genesis, error) {
	if embeddedGenesis == nil {
		return nil, ErrNoEmbeddedGenesis
	}
	return NewGenesis(embeddedGenesis), nil
}

// ReadGenesisConfigFromFile loads a JSON-encoded [GenesisConfig] from
// genesisFile, validates it via [CheckGenesis], and returns the
// resulting [store.Genesis]. An empty path returns (nil, nil) — the
// caller should fall back to the embedded genesis.
//
// On any failure the function logs at `Crit` level and returns one of
// [ErrInvalidGenesisPath], [ErrIncompleteGenesisJson],
// [ErrInvalidGenesisJson], or [ErrInvalidGenesisConfig].
func ReadGenesisConfigFromFile(genesisFile string) (store.Genesis, error) {
	defer func() {
		if err := recover(); err != nil {
			log.Crit("invalid genesis file", "method", "readGenesis", "genesisFile", genesisFile)
		}
	}()

	var config *GenesisConfig

	if len(genesisFile) > 0 {
		file, err := os.Open(genesisFile)
		if err != nil {
			log.Crit("invalid genesis file", "method", "readGenesis", "reason", err, "genesisFile", genesisFile)
			return nil, ErrInvalidGenesisPath
		}
		defer file.Close()

		config = new(GenesisConfig)
		if err := json.NewDecoder(file).Decode(config); err != nil {
			log.Crit("invalid genesis file", "method", "readGenesis", "reason", err, "genesisFile", genesisFile)
			if err.Error() == "unexpected EOF" || err.Error() == "EOF" {
				return nil, ErrIncompleteGenesisJson
			} else {
				return nil, ErrInvalidGenesisJson
			}
		}

		if err := CheckGenesis(config); err != nil {
			log.Crit("invalid genesis file", "method", "readGenesis", "reason", err, "genesisFile", genesisFile)
			return nil, ErrInvalidGenesisConfig
		}
		return NewGenesis(config), nil
	} else {
		return nil, nil
	}
}

// GenesisConfig is the JSON-shaped genesis description: the chain
// identifier, opaque ExtraData, the genesis timestamp, the
// spork-controlling address, the per-embedded-contract configs, and the
// catalog of genesis-receive blocks that seed user balances.
type GenesisConfig struct {
	ChainIdentifier     uint64
	ExtraData           string
	GenesisTimestampSec int64
	SporkAddress        *types.Address

	PillarConfig *PillarContractConfig
	TokenConfig  *TokenContractConfig
	PlasmaConfig *PlasmaContractConfig
	SwapConfig   *SwapContractConfig
	SporkConfig  *SporkConfig

	GenesisBlocks *GenesisBlocksConfig
}

// PillarContractConfig is the genesis seed for the pillar contract:
// initial pillar registrations, their delegations, and any legacy
// (chain-migration) pillar entries.
type PillarContractConfig struct {
	Pillars       []*definition.PillarInfo
	Delegations   []*definition.DelegationInfo
	LegacyEntries []*definition.LegacyPillarEntry
}

// TokenContractConfig is the genesis seed for the token contract:
// initial token issuances.
type TokenContractConfig struct {
	Tokens []*definition.TokenInfo
}

// PlasmaContractConfig is the genesis seed for the plasma contract:
// initial fusion entries.
type PlasmaContractConfig struct {
	Fusions []*definition.FusionInfo
}

// SwapContractConfig is the genesis seed for the swap contract: the
// legacy-chain redemption entries available at launch.
type SwapContractConfig struct {
	Entries []*definition.SwapAssets
}

// GenesisBlocksConfig is the catalog of genesis-receive blocks that
// seed user balances at chain birth. Each entry produces one
// [nom.BlockTypeGenesisReceive] block on the supplied address.
type GenesisBlocksConfig struct {
	Blocks []*GenesisBlockConfig
}

// GenesisBlockConfig describes one genesis-receive: the recipient
// address and its initial balance per ZTS.
type GenesisBlockConfig struct {
	Address     types.Address
	BalanceList map[types.ZenonTokenStandard]*big.Int
}

// SporkConfig is the genesis seed for the spork contract — used by
// chains that launch with one or more sporks already defined.
type SporkConfig struct {
	Sporks []*definition.Spork
}
