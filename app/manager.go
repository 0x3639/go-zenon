package app

import (
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"gopkg.in/urfave/cli.v1"

	"github.com/zenon-network/go-zenon/node"
)

type Manager struct {
	ctx  *cli.Context
	node *node.Node
}

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
			fmt.Println("* No pillar configured for current node")
		} else {
			fmt.Printf("* Pillar detected! Producing address %v\n", address)
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
func (nodeManager *Manager) Stop() error {
	log.Warn("Stopping znnd ...")

	if err := nodeManager.node.Stop(); err != nil {
		log.Error("Failed to stop node", "reason", err)
	}
	return nil
}
