//go:build !libznn

package app

import (
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/urfave/cli/v2"

	"github.com/zenon-network/go-zenon/node"
)

// Manager is the CLI-build process supervisor: holds the cli
// context (for late flag access) plus the node, exposes Start and
// Stop. The libznn build provides a parallel implementation in
// manager_libznn.go that omits the SIGINT handler and Wait.
type Manager struct {
	ctx  *cli.Context
	node *node.Node
}

// NewNodeManager builds the node config, instantiates a [node.Node]
// (which acquires the data-dir lock), and wraps it in a Manager
// ready for [Manager.Start].
func NewNodeManager(ctx *cli.Context) (*Manager, error) {
	// make config
	nodeConfig, err := MakeConfig(ctx)
	if err != nil {
		return nil, err
	}

	// make node
	newNode, err := node.NewNode(nodeConfig)

	if err != nil {
		log.Error("failed to create the node", "reason", err)
		return nil, err
	}

	return &Manager{
		ctx:  ctx,
		node: newNode,
	}, nil
}

// Start launches the node, prints producer-status detection,
// installs SIGINT/SIGTERM handlers, and blocks on [node.Node.Wait]
// until the node stops.
//
// Signal semantics: the first SIGINT/SIGTERM triggers Stop in a
// background goroutine. The handler then drains up to **ten**
// additional signals before returning — every additional signal in
// that window is logged as a warning ("Please DO NOT interrupt the
// shutdown process, panic may occur."). After ten extra signals the
// drain loop exits but the process keeps running until
// [node.Node.Wait] returns.
//
// Note: signal.Notify is also asked to listen for SIGKILL, but
// SIGKILL is uncatchable on Unix — that entry has no effect and is
// kept only for source-code symmetry with the other names.
func (nodeManager *Manager) Start() error {
	// Start up the node
	log.Info("starting znnd")
	if err := nodeManager.node.Start(); err != nil {
		fmt.Printf("failed to start node; reason:%v\n", err)
		log.Crit("failed to start node", "reason", err)
		os.Exit(1)
	} else {
		fmt.Println("znnd successfully started")
		fmt.Println("*** Node status ***")
		address := nodeManager.node.Zenon().Producer().GetCoinBase()
		if address == nil {
			fmt.Println("* No Pillar configured for current node")
		} else {
			fmt.Printf("* Producer address detected: %v\n", address)
		}
	}

	// Listening event closes the node
	go func() {
		c := make(chan os.Signal, 1)
		signal.Notify(c, syscall.SIGINT, syscall.SIGTERM, syscall.SIGKILL)
		defer signal.Stop(c)
		<-c
		fmt.Println("Shutting down znnd")

		go func() {
			nodeManager.Stop()
		}()

		for i := 10; i > 0; i-- {
			<-c
			if i > 1 {
				log.Warn("Please DO NOT interrupt the shutdown process, panic may occur.", "times", i-1)
			}
		}
	}()

	// Waiting for node to close
	nodeManager.node.Wait()

	return nil
}

// Stop forwards to [node.Node.Stop]. Logs (does not surface)
// teardown errors — the process is exiting anyway.
func (nodeManager *Manager) Stop() error {
	log.Warn("Stopping znnd ...")

	if err := nodeManager.node.Stop(); err != nil {
		log.Error("Failed to stop node", "reason", err)
	}
	return nil
}
