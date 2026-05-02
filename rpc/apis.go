package rpc

import (
	"github.com/zenon-network/go-zenon/p2p"
	"github.com/zenon-network/go-zenon/rpc/api"
	"github.com/zenon-network/go-zenon/rpc/api/embedded"
	"github.com/zenon-network/go-zenon/rpc/api/subscribe"
	rpc "github.com/zenon-network/go-zenon/rpc/server"
	"github.com/zenon-network/go-zenon/zenon"
)

// getApi resolves a single config-style module name to the API
// descriptors that should be registered on the JSON-RPC server.
// Unknown module names produce an empty slice (no error) so the node
// can be configured with arbitrary module lists without aborting on
// typos.
func getApi(z zenon.Zenon, p2p *p2p.Server, apiModule string) []rpc.API {
	switch apiModule {
	case "ledger":
		return []rpc.API{
			{
				Namespace: "ledger",
				Version:   "1.0",
				Service:   api.NewLedgerApi(z),
				Public:    true,
			},
		}
	case "ledgerSubscribe":
		return []rpc.API{
			{
				Namespace: "ledger",
				Version:   "1.0",
				Service:   subscribe.GetSubscribeApi(),
				Public:    true,
			},
		}
	case "embedded":
		return []rpc.API{
			{
				Namespace: "embedded.token",
				Version:   "1.0",
				Service:   embedded.NewTokenApi(z),
				Public:    true,
			},
			{
				Namespace: "embedded.sentinel",
				Version:   "1.0",
				Service:   embedded.NewSentinelApi(z),
				Public:    true,
			},
			{
				Namespace: "embedded.pillar",
				Version:   "1.0",
				Service:   embedded.NewPillarApi(z, false),
				Public:    true,
			},
			{
				Namespace: "embedded.plasma",
				Version:   "1.0",
				Service:   embedded.NewPlasmaApi(z),
				Public:    true,
			},
			{
				Namespace: "embedded.stake",
				Version:   "1.0",
				Service:   embedded.NewStakeApi(z),
				Public:    true,
			},
			{
				Namespace: "embedded.swap",
				Version:   "1.0",
				Service:   embedded.NewSwapApi(z),
				Public:    true,
			},
			{
				Namespace: "embedded.spork",
				Version:   "1.0",
				Service:   embedded.NewSporkApi(z),
				Public:    true,
			},
			{
				Namespace: "embedded.accelerator",
				Version:   "1.0",
				Service:   embedded.NewAcceleratorApi(z),
				Public:    true,
			},
			{
				Namespace: "embedded.htlc",
				Version:   "1.0",
				Service:   embedded.NewHtlcApi(z),
				Public:    true,
			},
			{
				Namespace: "embedded.bridge",
				Version:   "1.0",
				Service:   embedded.NewBridgeApi(z),
				Public:    true,
			},
			{
				Namespace: "embedded.liquidity",
				Version:   "1.0",
				Service:   embedded.NewLiquidityApi(z),
				Public:    true,
			},
		}
	case "stats":
		return []rpc.API{
			{
				Namespace: "stats",
				Version:   "1.0",
				Service:   api.NewStatsApi(z, p2p),
				Public:    true,
			},
		}
	default:
		return []rpc.API{}
	}
}

// GetApis flattens a list of module names into the union of API
// descriptors they expose.
func GetApis(z zenon.Zenon, p2p *p2p.Server, apiModule ...string) []rpc.API {
	var apis []rpc.API
	for _, m := range apiModule {
		apis = append(apis, getApi(z, p2p, m)...)
	}
	return apis
}

// GetPublicApis returns every default-on namespace: ledger,
// ledgerSubscribe, embedded, and stats. Used as the canonical
// public-RPC bundle.
func GetPublicApis(z zenon.Zenon, p2p *p2p.Server) []rpc.API {
	return GetApis(z, p2p, "ledger", "ledgerSubscribe", "embedded", "stats")
}
