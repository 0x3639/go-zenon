package embedded

import (
	"github.com/inconshreveable/log15"

	"github.com/zenon-network/go-zenon/chain"
	"github.com/zenon-network/go-zenon/common"
	"github.com/zenon-network/go-zenon/common/types"
	"github.com/zenon-network/go-zenon/consensus"
	"github.com/zenon-network/go-zenon/rpc/api"
	"github.com/zenon-network/go-zenon/vm/embedded/definition"
	"github.com/zenon-network/go-zenon/zenon"
)

// SporkApi serves read RPCs for the spork contract — the protocol
// feature-flag registry that gates progressive upgrades.
type SporkApi struct {
	chain chain.Chain
	z     zenon.Zenon
	cs    consensus.Consensus
	log   log15.Logger
}

// NewSporkApi returns a SporkApi bound to z's chain and consensus
// handles. The cs and z fields are stored for future read paths;
// the methods currently exposed only need chain.
func NewSporkApi(z zenon.Zenon) *SporkApi {
	return &SporkApi{
		chain: z.Chain(),
		z:     z,
		cs:    z.Consensus(),
		log:   common.RPCLogger.New("module", "embedded_spork_api"),
	}
}

// SporkList is the paged response shape for spork enumeration:
// Count is the total before paging, List is the requested page.
type SporkList struct {
	Count uint32              `json:"count"`
	List  []*definition.Spork `json:"list"`
}

// GetAll returns one page of every spork registered on chain,
// ordered as returned by definition.GetAllSporks: that helper
// iterates the LevelDB key prefix for spork records, so the
// result is sorted by ascending key bytes (sporkInfoPrefix ||
// sporkId) — effectively by sporkId, not by registration order.
// pageSize larger than api.RpcMaxPageSize is rejected with
// api.ErrPageSizeParamTooBig before the chain read.
func (a *SporkApi) GetAll(pageIndex, pageSize uint32) (*SporkList, error) {
	if pageSize > api.RpcMaxPageSize {
		return nil, api.ErrPageSizeParamTooBig
	}

	_, context, err := api.GetFrontierContext(a.chain, types.SporkContract)
	if err != nil {
		return nil, err
	}

	sporks := definition.GetAllSporks(context.Storage())

	listLen := uint32(len(sporks))
	start, end := api.GetRange(pageIndex, pageSize, listLen)
	return &SporkList{
		Count: listLen,
		List:  sporks[start:end],
	}, nil
}
