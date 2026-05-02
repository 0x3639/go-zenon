package genesis

import (
	"encoding/json"

	"github.com/zenon-network/go-zenon/common"
)

// init parses the embedded genesis JSON string (when this build carries
// one) into the package-level [embeddedGenesis] handle. Panics through
// [common.DealWithErr] on parse failure — a malformed embedded genesis
// is a build defect.
func init() {
	if embeddedGenesis == nil && len(embeddedGenesisStr) != 0 {
		embeddedGenesis = new(GenesisConfig)
		common.DealWithErr(json.Unmarshal([]byte(embeddedGenesisStr), embeddedGenesis))
	}
}

// embeddedGenesis holds the parsed alphanet [GenesisConfig] for builds
// that compile in [embeddedGenesisStr]. Nil for builds without an
// embedded genesis (in which case [MakeEmbeddedGenesisConfig] returns
// [ErrNoEmbeddedGenesis]).
var (
	embeddedGenesis *GenesisConfig
)
