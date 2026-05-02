package api

import (
	"runtime"
	"strings"

	"github.com/inconshreveable/log15"
	"github.com/shirou/gopsutil/host"
	"github.com/shirou/gopsutil/mem"

	"github.com/zenon-network/go-zenon/common"
	"github.com/zenon-network/go-zenon/metadata"
	"github.com/zenon-network/go-zenon/p2p"
	"github.com/zenon-network/go-zenon/p2p/discover"
	"github.com/zenon-network/go-zenon/protocol"
	"github.com/zenon-network/go-zenon/zenon"
)

// StatsApi is the "stats" JSON-RPC namespace. Exposes process / OS
// / network / sync introspection used by operators and dashboards.
// Stateless after construction; safe for concurrent use.
type StatsApi struct {
	z   zenon.Zenon
	p2p *p2p.Server
	log log15.Logger
}

// NewStatsApi constructs the "stats" namespace handler. The p2p
// pointer is required for the network-info endpoints; pass the same
// server that was registered with the node.
func NewStatsApi(z zenon.Zenon, p2p *p2p.Server) *StatsApi {
	return &StatsApi{
		z:   z,
		p2p: p2p,
		log: common.RPCLogger.New("module", "net_api"),
	}
}

// OsInfoResponse is the wire shape returned by [StatsApi.OsInfo].
// All fields default to zero values when the underlying gopsutil
// probe fails; callers should treat absent values as "unknown"
// rather than retrying.
type OsInfoResponse struct {
	Os              string `json:"os"`
	Platform        string `json:"platform"`
	PlatformFamily  string `json:"platformFamily"`
	PlatformVersion string `json:"platformVersion"`
	KernelVersion   string `json:"kernelVersion"`
	MemoryTotal     uint64 `json:"memoryTotal"`
	MemoryFree      uint64 `json:"memoryFree"`
	NumCPU          int    `json:"numCPU"`
	NumGoroutine    int    `json:"numGoroutine"`
}

// OsInfo reports host OS, memory, and Go runtime statistics.
// gopsutil errors are silently ignored — partial data is preferred
// over failing the request.
func (api *StatsApi) OsInfo() (*OsInfoResponse, error) {
	result := &OsInfoResponse{}
	stat, e := host.Info()
	if e == nil {
		result.Os = stat.OS
		result.Platform = stat.Platform
		result.PlatformFamily = stat.PlatformFamily
		result.PlatformVersion = stat.PlatformVersion
		result.KernelVersion = stat.KernelVersion
	}

	memO, e := mem.VirtualMemory()
	if e == nil {
		result.MemoryFree = memO.Available
		result.MemoryTotal = memO.Total
	}

	result.NumCPU = runtime.NumCPU()
	result.NumGoroutine = runtime.NumGoroutine()
	return result, nil
}

// ProcessInfoResponse is the wire shape returned by
// [StatsApi.ProcessInfo]. Version and Commit are baked into the
// binary at build time via [github.com/zenon-network/go-zenon/metadata].
type ProcessInfoResponse struct {
	Version string `json:"version"`
	Commit  string `json:"commit"`
}

// ProcessInfo returns the build version and git commit baked into
// this binary.
func (api *StatsApi) ProcessInfo() (*ProcessInfoResponse, error) {
	return &ProcessInfoResponse{
		Version: metadata.Version,
		Commit:  metadata.GitCommit,
	}, nil
}

// Peer is the wire-form summary of a connected p2p peer.
type Peer struct {
	PublicKey string `json:"publicKey"`
	IP        string `json:"ip"`
	Name      string `json:"name"`
}

// NetworkInfoResponse is returned by [StatsApi.NetworkInfo] —
// reports the local node identity (Self) and connected peer set.
type NetworkInfoResponse struct {
	NumPeers int     `json:"numPeers"`
	Peers    []*Peer `json:"peers"`
	Self     *Peer   `json:"self"`
}

// p2pPeerToPeer projects a [p2p.Peer] into the wire-form [Peer].
// Strips the port suffix from RemoteAddr to produce a bare IP.
func p2pPeerToPeer(peer *p2p.Peer) (*Peer, error) {
	ip := peer.RemoteAddr().String()
	splits := strings.Split(ip, ":")
	return &Peer{
		PublicKey: peer.ID().String(),
		IP:        splits[0],
		Name:      peer.Name(),
	}, nil
}

// selfToPeer renders the local node as a Peer — IP is hard-coded to
// loopback because the local listening interface is not meaningful
// to remote callers.
func selfToPeer(node *discover.Node) *Peer {
	return &Peer{
		PublicKey: node.ID.String(),
		IP:        "127.0.0.1",
		Name:      "*self*",
	}
}

// NetworkInfo returns the connected-peer set and the local node's
// identity.
func (api *StatsApi) NetworkInfo() (*NetworkInfoResponse, error) {
	peersRaw := api.p2p.Peers()
	peers := make([]*Peer, 0, len(peersRaw))
	for _, raw := range peersRaw {
		peer, err := p2pPeerToPeer(raw)
		if err != nil {
			return nil, err
		}
		peers = append(peers, peer)
	}

	return &NetworkInfoResponse{
		NumPeers: api.p2p.PeerCount(),
		Peers:    peers,
		Self:     selfToPeer(api.p2p.Self()),
	}, nil
}

// SyncInfo reports the broadcaster's view of catch-up progress
// (Unknown / Syncing / SyncDone / NotEnoughPeers + heights).
func (api *StatsApi) SyncInfo() (*protocol.SyncInfo, error) {
	return api.z.Broadcaster().SyncInfo(), nil
}
