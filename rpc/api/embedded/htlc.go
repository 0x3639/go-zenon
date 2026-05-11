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

// HtlcApi serves read RPCs for the hash-time-locked-contract
// (HTLC) embedded contract.
type HtlcApi struct {
	chain chain.Chain
	z     zenon.Zenon
	cs    consensus.Consensus
	log   log15.Logger
}

// NewHtlcApi returns an HtlcApi bound to z's chain and consensus.
// The cs and z fields are stored for symmetry with other handler
// constructors; the currently-exposed methods only use chain.
func NewHtlcApi(z zenon.Zenon) *HtlcApi {
	return &HtlcApi{
		chain: z.Chain(),
		z:     z,
		cs:    z.Consensus(),
		log:   common.RPCLogger.New("module", "embedded_htlc_api"),
	}
}

// GetById returns the HtlcInfo record for the given htlc id, or the
// underlying error from definition.GetHtlcInfo (typically
// constants.ErrDataNonExistent when the id is unknown).
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

// GetProxyUnlockStatus reports whether address is allowed to call
// the HTLC's proxy-unlock entry point. The boolean reflects the
// recorded permission, defaulting to the contract's documented
// default when no explicit entry exists; see
// implementation.GetHtlcProxyUnlockStatus for the precise rule.
func (a *HtlcApi) GetProxyUnlockStatus(address types.Address) (bool, error) {
	_, context, err := api.GetFrontierContext(a.chain, types.HtlcContract)
	if err != nil {
		return false, err
	}
	return implementation.GetHtlcProxyUnlockStatus(context, address)
}
