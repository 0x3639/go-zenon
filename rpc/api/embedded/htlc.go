package embedded

import (
	"github.com/inconshreveable/log15"

	"github.com/zenon-network/go-zenon/chain"
	"github.com/zenon-network/go-zenon/common"
	"github.com/zenon-network/go-zenon/common/types"
	"github.com/zenon-network/go-zenon/consensus"
	"github.com/zenon-network/go-zenon/rpc/api"
	"github.com/zenon-network/go-zenon/vm/embedded/definition"
	"github.com/zenon-network/go-zenon/vm/embedded/implementation"
	"github.com/zenon-network/go-zenon/zenon"
)

// HtlcApi is the "embedded.htlc" namespace — read access to
// hash-time-locked contract entries.
type HtlcApi struct {
	chain chain.Chain
	z     zenon.Zenon
	cs    consensus.Consensus
	log   log15.Logger
}

// NewHtlcApi constructs the HTLC namespace handler.
func NewHtlcApi(z zenon.Zenon) *HtlcApi {
	return &HtlcApi{
		chain: z.Chain(),
		z:     z,
		cs:    z.Consensus(),
		log:   common.RPCLogger.New("module", "embedded_htlc_api"),
	}
}

// GetById returns the HTLC entry with the given id, or
// ErrDataNonExistent if no such entry exists.
func (a *HtlcApi) GetById(id types.Hash) (*definition.HtlcInfo, error) {

	_, context, err := api.GetFrontierContext(a.chain, types.HtlcContract)
	if err != nil {
		return nil, err
	}

	htlcInfo, err := definition.GetHtlcInfo(context.Storage(), id)
	if err != nil {
		return nil, err
	}

	return htlcInfo, nil
}

// GetProxyUnlockStatus reports whether address is currently allowed
// to act as an HTLC proxy unlocker.
func (a *HtlcApi) GetProxyUnlockStatus(address types.Address) (bool, error) {
	_, context, err := api.GetFrontierContext(a.chain, types.HtlcContract)
	if err != nil {
		return false, err
	}
	return implementation.GetHtlcProxyUnlockStatus(context, address)
}
