package app

import (
	"fmt"
	"net/http"
	_ "net/http/pprof"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"time"

	"github.com/urfave/cli/v2"

	"github.com/zenon-network/go-zenon/common"
	"github.com/zenon-network/go-zenon/metadata"
)

var (
	log = common.ZenonLogger.New()
	// app is the urfave/cli App configured in init() with flags,
	// commands, and the boot/teardown action callbacks.
	app = cli.NewApp()
	// nodeManager holds the running node so the libznn-exported
	// [Stop] can reach it. Assigned by the [action] callback before
	// it calls [Manager.Start] (see action() below); the assignment
	// is therefore live for the entire duration of Start, not only
	// after Start returns.
	nodeManager *Manager
)

// Run is the CLI entry point. Dispatches the urfave/cli App against
// os.Args and exits non-zero on error. Called by both znnd's main
// and the libznn-exported RunNode shim.
func Run() {
	err := app.Run(os.Args)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

// Stop tears down the running node. Exported for the libznn build —
// CLI builds reach the same code path through SIGINT/SIGTERM
// handlers in [Manager.Start]. Panics via [common.DealWithErr] on
// teardown failure.
func Stop() {
	err := nodeManager.Stop()
	common.DealWithErr(err)
	fmt.Println("znnd successfully stopped")
}

func init() {
	app.Name = filepath.Base(os.Args[0])
	app.HideVersion = false
	app.Version = metadata.Version
	app.Compiled = time.Now()
	app.Authors = []*cli.Author{
		{
			Name:  "The Zenon Developers",
			Email: "portal@zenon.network",
		},
	}
	app.Copyright = "Copyright 2021, Zenon"
	app.Usage = "znnd Node"

	//Import: Please add the New command here
	app.Commands = []*cli.Command{
		versionCommand,
		licenseCommand,
	}
	sort.Sort(cli.CommandsByName(app.Commands))

	app.Flags = AllFlags

	app.Before = beforeAction
	app.Action = action
	app.After = afterAction
}

// beforeAction is the urfave/cli pre-action hook: print the version
// banner, pin GOMAXPROCS to all cores, and (if `--pprof` is set)
// start the pprof HTTP server in a background goroutine.
func beforeAction(ctx *cli.Context) error {

	max := runtime.NumCPU()
	fmt.Printf("Starting znnd.\n")
	fmt.Printf("current time is %v\n", time.Now().Format("2006-01-02 15:04:05"))
	fmt.Printf("version: %v\n", metadata.Version)
	fmt.Printf("git-commit-hash: %v\n", metadata.GitCommit)
	fmt.Printf("znnd will use at most %v cpu-cores\n", max)
	runtime.GOMAXPROCS(max)

	// pprof server
	if ctx.IsSet(PprofFlag.Name) {
		listenHost := ctx.String(PprofAddrFlag.Name)

		port := ctx.Int(PprofPortFlag.Name)

		address := fmt.Sprintf("%s:%d", listenHost, port)

		log.Info("Starting pprof server", "addr", fmt.Sprintf("http://%s/debug/pprof", address))
		go func() {
			if err := http.ListenAndServe(address, nil); err != nil {
				log.Error("Failure in running pprof server", "err", err)
			}
		}()
	}

	return nil
}

// action is the default urfave/cli action — invoked when no
// sub-command is given. Rejects positional args, builds a [Manager]
// from the parsed flags, and starts it.
func action(ctx *cli.Context) error {
	//Make sure No subCommands were entered,Only the flags
	if args := ctx.Args(); args.Len() > 0 {
		return fmt.Errorf("invalid command: %q", args.Get(0))
	}
	var err error
	nodeManager, err = NewNodeManager(ctx)
	if err != nil {
		return err
	}

	return nodeManager.Start()
}

// afterAction is the urfave/cli post-action hook. Currently a
// no-op; exists so future cleanup logic has a stable insertion
// point.
func afterAction(*cli.Context) error {
	return nil
}
