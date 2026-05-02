//go:build libznn && !detached

package main

import (
	"github.com/zenon-network/go-zenon/app"
)

import "C"

// RunNode is the C-exported entry point. Called by embedders to
// boot a node; equivalent to running the znnd binary. Blocks until
// StopNode is called (or a SIGINT is delivered).
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
