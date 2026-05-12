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

// StatsApi serves the "stats" RPC namespace: node-introspection
// methods that expose host OS information, the running binary's
// version, the live p2p peer set, and consensus sync state. The
// methods are read-only and do not require any chain access — they
// reflect the running process rather than chain state.
type StatsApi struct {
	z   zenon.Zenon
	p2p *p2p.Server
	log log15.Logger
}

// NewStatsApi returns a StatsApi bound to z and the live p2p
// server. The logger module tag is "net_api" (the package's
// historical name before the rename to stats); it remains "net_api"
// in logs so existing log filters keep matching.
func NewStatsApi(z zenon.Zenon, p2p *p2p.Server) *StatsApi {
	return &StatsApi{
		z:   z,
		p2p: p2p,
		log: common.RPCLogger.New("module", "net_api"),
	}
}

// OsInfoResponse is the wire shape returned by StatsApi.OsInfo. It
// merges host-level OS identifiers (from gopsutil/host.Info) with
// runtime memory (gopsutil/mem.VirtualMemory) and Go-runtime
// counters (runtime.NumCPU, runtime.NumGoroutine). Fields sourced
// from gopsutil are zero-valued if the underlying probe fails.
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

// OsInfo returns the host OS snapshot described by OsInfoResponse.
// gopsutil probe failures are swallowed: Memory* and Platform*
// fields stay zero-valued rather than propagating the underlying
// error, so OsInfo never returns a non-nil error in current code.
// The (error) return is kept for forward compatibility.
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
// StatsApi.ProcessInfo: the build-time identifiers baked into the
// running binary via the metadata package.
type ProcessInfoResponse struct {
	Version string `json:"version"`
	Commit  string `json:"commit"`
}

// ProcessInfo returns the running binary's version (metadata.Version,
// the release tag at build time) and short commit hash
// (metadata.GitCommit, set via -ldflags by the Makefile). The
// (error) return is kept for forward compatibility; the current
// implementation never returns a non-nil error.
func (api *StatsApi) ProcessInfo() (*ProcessInfoResponse, error) {
	return &ProcessInfoResponse{
		Version: metadata.Version,
		Commit:  metadata.GitCommit,
	}, nil
}

// Peer is the wire view of one P2P peer used in NetworkInfoResponse.
// PublicKey is the node ID (hex-encoded), IP is the remote address
// stripped of its port, and Name is the announced peer name. The
// Self peer is synthesised with IP "127.0.0.1" and Name "*self*".
type Peer struct {
	PublicKey string `json:"publicKey"`
	IP        string `json:"ip"`
	Name      string `json:"name"`
}

// NetworkInfoResponse is the wire shape returned by
// StatsApi.NetworkInfo: the live peer set plus the node's own
// identity. NumPeers reflects p2p.Server.PeerCount at call time,
// which may differ from len(Peers) if a peer was added or removed
// between the two reads.
type NetworkInfoResponse struct {
	NumPeers int     `json:"numPeers"`
	Peers    []*Peer `json:"peers"`
	Self     *Peer   `json:"self"`
}

func p2pPeerToPeer(peer *p2p.Peer) (*Peer, error) {
	ip := peer.RemoteAddr().String()
	splits := strings.Split(ip, ":")
	return &Peer{
		PublicKey: peer.ID().String(),
		IP:        splits[0],
		Name:      peer.Name(),
	}, nil
}
func selfToPeer(node *discover.Node) *Peer {
	return &Peer{
		PublicKey: node.ID.String(),
		IP:        "127.0.0.1",
		Name:      "*self*",
	}
}

// NetworkInfo enumerates the current p2p peer set and returns it
// wrapped in NetworkInfoResponse. A failed per-peer address
// conversion aborts the whole call rather than skipping that peer,
// so the result is all-or-nothing.
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

// SyncInfo returns the consensus-broadcaster's view of how far the
// local node is from the network frontier. The (error) return is
// kept for forward compatibility; the current implementation
// always returns a non-nil *SyncInfo and a nil error.
func (api *StatsApi) SyncInfo() (*protocol.SyncInfo, error) {
	return api.z.Broadcaster().SyncInfo(), nil
}
