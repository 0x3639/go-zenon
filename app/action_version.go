package app

import (
	"fmt"
	"os"
	"runtime"

	"github.com/urfave/cli/v2"

	"github.com/zenon-network/go-zenon/metadata"
)

// versionCommand registers the `znnd version` sub-command, which
// prints build / runtime metadata for diagnostics.
var (
	versionCommand = &cli.Command{
		Action:    versionAction,
		Name:      "version",
		Usage:     "Print version numbers",
		ArgsUsage: " ",
		Category:  "MISCELLANEOUS COMMANDS",
	}
)

// versionAction is the handler for `znnd version` — prints the
// binary version, git commit, Go version, GOOS/GOARCH, and
// GOPATH/GOROOT.
func versionAction(*cli.Context) error {
	fmt.Printf(`znnd
Version:%v
Architecture:%v
Go Version:%v
Operating System:%v
GOPATH:%v
GOROOT:%v
Commit hash:%v
`, metadata.Version, runtime.GOARCH, runtime.Version(), runtime.GOOS, os.Getenv("GOPATH"), runtime.GOROOT(), metadata.GitCommit)
	return nil
}
