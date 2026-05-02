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

// SporkApi is the "embedded.spork" namespace — read access to the
// spork registry (active and pending protocol upgrades).
type SporkApi struct {
	chain chain.Chain
	z     zenon.Zenon
	cs    consensus.Consensus
	log   log15.Logger
}

// NewSporkApi constructs the spork namespace handler.
func NewSporkApi(z zenon.Zenon) *SporkApi {
	return &SporkApi{
		chain: z.Chain(),
		z:     z,
		cs:    z.Consensus(),
		log:   common.RPCLogger.New("module", "embedded_spork_api"),
	}
}

// SporkList is the paginated response shape.
type SporkList struct {
	Count uint32              `json:"count"`
	List  []*definition.Spork `json:"list"`
}

// GetAll returns every spork ever registered (active + pending +
// expired), paginated.
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
