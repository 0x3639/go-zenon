package node

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"github.com/pkg/errors"
	"github.com/prometheus/tsdb/fileutil"

	"github.com/zenon-network/go-zenon/common"
	"github.com/zenon-network/go-zenon/p2p"
	api "github.com/zenon-network/go-zenon/rpc"
	rpc "github.com/zenon-network/go-zenon/rpc/server"
	"github.com/zenon-network/go-zenon/wallet"
	"github.com/zenon-network/go-zenon/zenon"
)

var (
	log = common.NodeLogger
)

// Node is the long-running process container. Owns the wallet,
// p2p server, RPC stack, and a [zenon.Zenon] core; coordinates
// their start / stop order and holds the data-dir lock.
type Node struct {
	config *Config

	walletManager *wallet.Manager
	server        *p2p.Server

	z zenon.Zenon

	rpcAPIs []rpc.API   // List of APIs currently provided by the node
	http    *httpServer //
	ws      *httpServer //

	// Channel to wait for termination notifications
	stop        chan struct{}
	lock        sync.RWMutex
	dataDirLock fileutil.Releaser // prevents concurrent use of instance directory
}

// NewNode performs setup-only work: open + lock the data dir, start
// the wallet, build a [zenon.Config] from conf, instantiate the
// [zenon.Zenon] core, and assemble the [p2p.Server] descriptor. The
// returned Node is not yet started; call [Node.Start].
func NewNode(conf *Config) (*Node, error) {
	var err error

	node := &Node{
		config:        conf,
		stop:          make(chan struct{}),
		walletManager: wallet.New(conf.makeWalletConfig()),
		http:          newHTTPServer(rpc.DefaultHTTPTimeouts),
		ws:            newHTTPServer(rpc.DefaultHTTPTimeouts),
	}

	// prepare node
	log.Info("preparing node ... ")
	if err = node.openDataDir(); err != nil {
		return nil, err
	}

	// start wallet
	if err = node.startWallet(); err != nil {
		log.Error("failed to start wallet", "reason", err)
		return nil, err
	}

	// Initialize the zenon rpc
	zenonConfig, err := node.config.makeZenonConfig(node.walletManager)
	if err != nil {
		return nil, err
	}
	node.z, err = zenon.NewZenon(zenonConfig)
	if err != nil {
		log.Error("failed to create zenon", "reason", err)
		return nil, err
	}

	netConfig := conf.makeNetConfig()
	nodes, err := netConfig.Nodes()
	if err != nil {
		return nil, errors.Errorf("Unable to parse seeders. Reason: %v", err)
	}

	node.server = &p2p.Server{
		PrivateKey:        netConfig.PrivateKey(),
		Name:              netConfig.Name,
		MaxPeers:          netConfig.MaxPeers,
		MinConnectedPeers: netConfig.MinConnectedPeers,
		MaxPendingPeers:   netConfig.MaxPendingPeers,
		Discovery:         true,
		NoDial:            false,
		StaticNodes:       nil,
		BootstrapNodes:    nodes,
		TrustedNodes:      nil,
		NodeDatabase:      netConfig.NodeDatabase,
		ListenAddr:        fmt.Sprintf("%v:%v", netConfig.ListenAddr, netConfig.ListenPort),
		Protocols:         node.z.Protocol().SubProtocols,
	}
	return node, nil
}

// Start runs the boot sequence: zenon Init+Start, p2p server, then
// the RPC stack. Returns the first error encountered. Holds the
// node lock for the duration so concurrent Stop calls block until
// startup completes (or fails).
func (node *Node) Start() error {
	node.lock.Lock()
	defer node.lock.Unlock()

	if err := node.startZenon(); err != nil {
		return err
	}
	if err := node.server.Start(); err != nil {
		return err
	}
	node.rpcAPIs = api.GetPublicApis(node.z, node.server)
	if err := node.startRPC(); err != nil {
		log.Error("failed to start rpc", "reason", err)
		return err
	}

	return nil
}

// Stop unwinds the subsystems in reverse-dependency order: p2p →
// wallet → zenon → RPC stack → release the data-dir lock. Closes
// the stop channel so [Node.Wait] returns. Returns the first
// non-nil teardown error.
func (node *Node) Stop() error {
	node.lock.Lock()
	defer node.lock.Unlock()
	defer close(node.stop)

	log.Info("stopping p2p server ...")
	node.server.Stop()

	if err := node.stopWallet(); err != nil {
		log.Error("failed to stop wallet", "reason", err)
		return err
	}
	if err := node.stopZenon(); err != nil {
		log.Error("failed to stop zenon", "reason", err)
		return err
	}
	node.stopRPC()

	// Release instance directory lock.
	node.closeDataDir()

	return nil
}

// Wait blocks until [Node.Stop] is called. The CLI driver uses this
// to keep the process alive after a successful Start.
func (node *Node) Wait() {
	<-node.stop
}

// Zenon returns the running [zenon.Zenon] core.
func (node *Node) Zenon() zenon.Zenon {
	return node.z
}

// Config returns the original Config used to construct this Node.
// Treat as read-only.
func (node *Node) Config() *Config {
	return node.config
}

// WalletManager returns the started wallet manager — used by the
// producer keypair lookup and by tests / utilities that need wallet
// access without a full restart.
func (node *Node) WalletManager() *wallet.Manager {
	return node.walletManager
}

// startWallet begins the wallet manager so producer keypairs can be
// unlocked while building the zenon config.
func (node *Node) startWallet() error {
	if err := node.walletManager.Start(); err != nil {
		return err
	}
	return nil
}

// startZenon runs zenon Init and Start in sequence, returning the
// first error encountered.
func (node *Node) startZenon() error {
	if err := node.z.Init(); err != nil {
		log.Error("failed to init zenon", "reason", err)
		return err
	}
	if err := node.z.Start(); err != nil {
		log.Error("failed to start zenon", "reason", err)
		return err
	}
	return nil
}

// stopWallet shuts down the wallet manager. Returns ErrNodeStopped
// if the manager was never started — callers should treat that as
// a no-op rather than fatal.
func (node *Node) stopWallet() error {
	if node.walletManager == nil {
		return ErrNodeStopped
	}
	node.walletManager.Stop()
	return nil
}

// stopZenon shuts down the zenon core. Returns ErrNodeStopped if
// the core was never started.
func (node *Node) stopZenon() error {
	if node.z == nil {
		return ErrNodeStopped
	}
	return node.z.Stop()
}

// openDataDir mkdir's DataPath and acquires the exclusive `.lock`
// flock that guarantees single-process ownership of the chain
// database. Returns [ErrDataDirUsed] if another znnd holds the lock.
func (node *Node) openDataDir() error {
	if node.config.DataPath == "" {
		return nil
	}

	if err := os.MkdirAll(node.config.DataPath, 0700); err != nil {
		return err
	}
	log.Info("successfully ensured DataPath exists", "data-path", node.config.DataPath)

	// Lock the instance directory to prevent concurrent use by another instance as well as
	// accidental use of the instance directory as a database.
	if fileLock, _, err := fileutil.Flock(filepath.Join(node.config.DataPath, ".lock")); err != nil {
		log.Info("unable to acquire file-lock", "reason", err)
		return convertFileLockError(err)
	} else {
		node.dataDirLock = fileLock
	}

	log.Info("successfully locked dataDir")
	return nil
}

// closeDataDir releases the `.lock` flock acquired by openDataDir.
// Best-effort: a release failure is logged but not surfaced.
func (node *Node) closeDataDir() {
	log.Info("releasing dataDir lock ... ")
	// Release instance directory lock.
	if node.dataDirLock != nil {
		if err := node.dataDirLock.Release(); err != nil {
			log.Error("can't release dataDir lock", "reason", err)
		}
		node.dataDirLock = nil
	}
}
