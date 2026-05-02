//go:build libznn

package app

import (
	"fmt"
	"os"

	"github.com/urfave/cli/v2"

	"github.com/zenon-network/go-zenon/node"
)

// Manager is the libznn-build counterpart of the CLI Manager.
// Identical fields; lifecycle behaviour differs ([Manager.Start]
// returns immediately rather than blocking on Wait, since the
// embedder owns the process loop).
type Manager struct {
	ctx  *cli.Context
	node *node.Node
}

// NewNodeManager builds a node from cli flags and wraps it in a
// libznn-build Manager. Mirrors the CLI build's constructor.
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

// Start launches the node and returns immediately. The libznn
// embedder is expected to retain control of the process and call
// [app.Stop] to tear down — there is no SIGINT handler installed.
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

	return nil
}

// Stop forwards to [node.Node.Stop] and then blocks on
// [node.Node.Wait] so the embedder's call returns only after
// teardown is complete.
func (nodeManager *Manager) Stop() error {
	log.Warn("Stopping znnd ...")

	if err := nodeManager.node.Stop(); err != nil {
		log.Error("Failed to stop node", "reason", err)
	}

	// Waiting for node to close
	nodeManager.node.Wait()

	return nil
}
