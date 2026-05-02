//go:build !libznn

package main

import (
	"github.com/zenon-network/go-zenon/app"
)

// main is the official command-line entry point. Hands off to
// [github.com/zenon-network/go-zenon/app.Run] which owns flag
// parsing and the boot sequence.
func main() {
	app.Run()
}
