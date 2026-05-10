//go:build libznn && !detached

package main

import (
	"github.com/zenon-network/go-zenon/app"
)

import "C"

// RunNode is the C-exported entry point. Called by embedders to
// boot a node; equivalent to running the znnd binary. Returns
// after [app.Run] returns — under the libznn build tag, app.Run
// invokes the libznn manager's Start which itself returns
// immediately after node startup (no node.Wait), so embedders
// must keep their host process alive themselves and call
// [StopNode] for graceful teardown.
//
//export RunNode
func RunNode() {
	app.Run()
}

// StopNode is the C-exported teardown hook. Triggers a graceful
// node shutdown.
//
//export StopNode
func StopNode() {
	app.Stop()
}

// main is required by `go build -buildmode=c-shared` but is never
// invoked — embedders link against [RunNode] / [StopNode] only.
func main() {}
