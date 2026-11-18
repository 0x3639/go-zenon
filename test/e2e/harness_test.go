//go:build e2e

package e2e

import (
	"fmt"
	"math/big"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/zenon-network/go-zenon/chain/nom"
	"github.com/zenon-network/go-zenon/common/types"
	"github.com/zenon-network/go-zenon/vm/constants"
	"github.com/zenon-network/go-zenon/vm/embedded/definition"
	"github.com/zenon-network/go-zenon/wallet"
)

// wait is how long a case waits for a block to be in a momentum. Momentums
// are 10 seconds apart; a contract call needs two.
const wait = 2 * time.Minute

var (
	net     *Net
	runBase uint32 // first derivation index of this run's fresh accounts
	runTag  string // unique per run on one chain; used in names
	cov     = &coverage{rows: map[string]bool{}}

	// fast is true when driver and node are the test build with short
	// epochs and locks; then only the TestFast groups run.
	fast bool

	// live says which sporks are enforced on this chain when the run starts.
	live = map[string]bool{}

	znn = types.ZnnTokenStandard
	qsr = types.QsrTokenStandard
)

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func TestMain(m *testing.M) {
	urls := strings.Split(envOr("E2E_RPC", "http://localhost:35997,http://localhost:35991"), ",")
	chainID, _ := strconv.ParseUint(envOr("E2E_CHAIN_ID", "69"), 10, 64)
	minHeight, _ := strconv.ParseUint(os.Getenv("E2E_MIN_HEIGHT"), 10, 64)

	var err error
	if net, err = Dial(urls, chainID); err != nil {
		fmt.Fprintln(os.Stderr, "e2e:", err)
		os.Exit(2)
	}
	sporks, last, err := readSporks()
	if err != nil {
		fmt.Fprintln(os.Stderr, "e2e: spork list:", err)
		os.Exit(2)
	}
	if minHeight == 0 {
		// Start after the last scheduled spork, so the cases see one fixed state.
		minHeight = last + 3
	}
	if err = net.WaitHeight(minHeight, 15*time.Minute); err != nil {
		fmt.Fprintln(os.Stderr, "e2e:", err)
		os.Exit(2)
	}
	f, err := net.Frontier()
	if err != nil {
		fmt.Fprintln(os.Stderr, "e2e:", err)
		os.Exit(2)
	}
	// Fresh accounts and names for every run, so a run can be repeated on
	// the same chain without meeting the state of an earlier run.
	runBase = 1000 + uint32(f.Height)*64
	runTag = strconv.FormatUint(f.Height, 10)
	var names []string
	for name, height := range sporks {
		if height <= f.Height {
			live[name] = true
			names = append(names, name)
		}
	}
	sort.Strings(names)
	fmt.Printf("e2e: endpoints %v, chain %d, start height %d, account indexes from %d\n", urls, chainID, f.Height, runBase)
	fmt.Printf("e2e: sporks enforced: %v\n", names)

	// The driver and the node must be built from the same constants: either
	// both stock, or both with the fast test overlay (docs/TESTING.md, P11).
	var proc struct {
		Version string `json:"version"`
	}
	if err = net.Call(&proc, "stats.processInfo"); err != nil {
		fmt.Fprintln(os.Stderr, "e2e: process info:", err)
		os.Exit(2)
	}
	fast = constants.MomentumsPerEpoch != 8640
	if nodeFast := strings.HasSuffix(proc.Version, "+fast-constants"); nodeFast != fast {
		fmt.Fprintf(os.Stderr, "e2e: node version %q and driver disagree on the constants (driver fast=%v); build both the same way\n", proc.Version, fast)
		os.Exit(2)
	}
	fmt.Printf("e2e: node %s, fast constants: %v\n", proc.Version, fast)

	if err = ensureFunderPlasma(); err != nil {
		fmt.Fprintln(os.Stderr, "e2e: funder setup:", err)
		os.Exit(2)
	}

	code := m.Run()
	if path := os.Getenv("E2E_COVERAGE"); path != "" {
		if err := cov.write(path); err != nil {
			fmt.Fprintln(os.Stderr, "e2e: coverage file:", err)
			code = 2
		}
	}
	os.Exit(code)
}

// Spork names used in live, by the id compiled into znnd.
var sporkNames = map[types.Hash]string{
	types.AcceleratorSpork.SporkId:        "accelerator",
	types.HtlcSpork.SporkId:               "htlc",
	types.BridgeAndLiquiditySpork.SporkId: "bridge",
	types.PtlcSpork.SporkId:               "ptlc",
	types.Libp2pSpork.SporkId:             "libp2p",
	types.DynamicPlasmaSpork.SporkId:      "dynamic-plasma",
	types.EmbeddedRefundSpork.SporkId:     "embedded-refund",
	types.EpochRewardTermsSpork.SporkId:   "epoch-reward-terms",
}

// readSporks returns the enforcement height of every activated spork this
// binary implements, and the highest of them.
func readSporks() (map[string]uint64, uint64, error) {
	var list struct {
		List []struct {
			Id                types.Hash `json:"id"`
			Activated         bool       `json:"activated"`
			EnforcementHeight uint64     `json:"enforcementHeight"`
		} `json:"list"`
	}
	if err := net.Call(&list, "embedded.spork.getAll", 0, 50); err != nil {
		return nil, 0, err
	}
	out, last := map[string]uint64{}, uint64(0)
	for _, s := range list.List {
		name, known := sporkNames[s.Id]
		if !known || !s.Activated {
			continue
		}
		out[name] = s.EnforcementHeight
		if s.EnforcementHeight > last {
			last = s.EnforcementHeight
		}
	}
	return out, last, nil
}

// contractLive applies the spork ladder (vm/embedded, sporkVariantKey):
// HTLC implies Bridge and Liquidity, and those imply Accelerator.
func contractLive(name string) bool {
	switch name {
	case "htlc":
		return live["htlc"]
	case "bridge", "liquidity":
		return live["bridge"] || live["htlc"]
	case "accelerator":
		return live["accelerator"] || live["bridge"] || live["htlc"]
	case "ptlc":
		return live["ptlc"]
	}
	return true
}

// needs skips a group whose contract is not live on this chain. The spork
// ladder group checks that such a contract refuses every method.
func needs(t *testing.T, contract string) {
	t.Helper()
	if !contractLive(contract) {
		t.Skipf("the %s contract is not live on this chain", contract)
	}
}

// ensureFunderPlasma gives the funder account fused plasma from pillar
// owner 1, which holds fused QSR at genesis, so that no setup step needs
// proof of work.
func ensureFunderPlasma() error {
	funder := net.Key(IdxFunder)
	var info struct {
		CurrentPlasma uint64 `json:"currentPlasma"`
	}
	if err := net.Call(&info, "embedded.plasma.get", funder.Address); err != nil {
		return err
	}
	if info.CurrentPlasma >= 5_000_000 {
		return nil
	}
	res, err := net.CallContract(net.Key(IdxOwner1), types.PlasmaContract,
		definition.ABIPlasma.PackMethodPanic(definition.FuseMethodName, funder.Address), qsr, coins(5000), wait)
	if err != nil {
		return err
	}
	if !res.OK() {
		return fmt.Errorf("fuse for the funder failed at the contract")
	}
	return nil
}

// group starts a normal case group: it runs in parallel with the others
// and is skipped in a destructive run, which must be alone on the chain.
func group(t *testing.T) {
	t.Helper()
	if os.Getenv("E2E_DESTRUCTIVE") == "1" {
		t.Skip("skipped in a destructive run")
	}
	if fast {
		t.Skip("skipped on the fast test build; its windows are too short for these cases")
	}
	t.Parallel()
}

// fastGroup starts a group that needs the test build with fast constants.
func fastGroup(t *testing.T) {
	t.Helper()
	if os.Getenv("E2E_DESTRUCTIVE") == "1" || !fast {
		t.Skip("needs the test build with fast constants (docs/TESTING.md, P11)")
	}
	t.Parallel()
}

// eventually polls cond every five seconds until it holds, and fails the
// case if it does not within the timeout.
func eventually(t *testing.T, timeout time.Duration, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatalf("not reached in %s: %s", timeout, what)
		}
		time.Sleep(5 * time.Second)
	}
}

// fundFrom is newAccount with another funding account. Devnet account 8 is
// kept for the cases that need large amounts.
func fundFrom(t *testing.T, funder *wallet.KeyPair, slot uint32, znnCoins, qsrCoins int64) *wallet.KeyPair {
	t.Helper()
	kp := net.Key(runBase + slot)
	callOK(t, net.Key(IdxFunder), types.PlasmaContract, definition.ABIPlasma.PackMethodPanic(definition.FuseMethodName, kp.Address), qsr, coins(2000))
	pending := 0
	if znnCoins > 0 {
		transfer(t, funder, kp.Address, znn, coins(znnCoins), nil)
		pending++
	}
	if qsrCoins > 0 {
		transfer(t, funder, kp.Address, qsr, coins(qsrCoins), nil)
		pending++
	}
	receiveAll(t, kp, pending)
	return kp
}

// coins converts whole coins of an 8-decimal token to base units.
func coins(n int64) *big.Int {
	return new(big.Int).Mul(big.NewInt(n), big.NewInt(100000000))
}

func units(n int64) *big.Int { return big.NewInt(n) }

// coverage collects "kind, item, class:case" rows from the cases that
// passed. test/inventory/record.py merges the file into coverage.tsv.
type coverage struct {
	mu   sync.Mutex
	rows map[string]bool
}

func (c *coverage) add(kind, item, token string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.rows[kind+"\t"+item+"\t"+token] = true
}

func (c *coverage) write(path string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	rows := make([]string, 0, len(c.rows))
	for r := range c.rows {
		rows = append(rows, r)
	}
	sort.Strings(rows)
	return os.WriteFile(path, []byte(strings.Join(rows, "\n")+"\n"), 0o644)
}

// caseID is the id in front of the first underscore of the subtest name:
// "TestPlasma/PL-1_fuse" gives "PL-1".
func caseID(t *testing.T) string {
	name := t.Name()
	if i := strings.LastIndex(name, "/"); i >= 0 {
		name = name[i+1:]
	}
	if i := strings.Index(name, "_"); i >= 0 {
		name = name[:i]
	}
	return name
}

// cover records that this case exercises an inventory item in an edge
// class (docs/TESTING.md section 1). The row is kept only if the case
// passes.
func cover(t *testing.T, class, kind string, items ...string) {
	t.Helper()
	id := caseID(t)
	t.Cleanup(func() {
		if t.Failed() || t.Skipped() {
			return
		}
		for _, item := range items {
			cov.add(kind, item, class+":"+id)
		}
	})
}

var apiTypes = map[string]string{
	"ledger": "LedgerApi", "stats": "StatsApi",
	"embedded.accelerator": "AcceleratorApi", "embedded.bridge": "BridgeApi", "embedded.htlc": "HtlcApi",
	"embedded.liquidity": "LiquidityApi", "embedded.pillar": "PillarApi", "embedded.plasma": "PlasmaApi",
	"embedded.ptlc": "PtlcApi", "embedded.sentinel": "SentinelApi", "embedded.spork": "SporkApi",
	"embedded.stake": "StakeApi", "embedded.swap": "SwapApi", "embedded.token": "TokenAPI",
}

// rpcItem maps "embedded.plasma.get" to the inventory name "PlasmaApi.Get".
func rpcItem(method string) string {
	i := strings.LastIndex(method, ".")
	ns, name := method[:i], method[i+1:]
	return apiTypes[ns] + "." + strings.ToUpper(name[:1]) + name[1:]
}

// get calls a read-only RPC method, fails the case on an error and records
// the method as exercised.
func get(t *testing.T, result interface{}, method string, args ...interface{}) {
	t.Helper()
	if err := net.Call(result, method, args...); err != nil {
		t.Fatalf("%s: %v", method, err)
	}
	cover(t, "E1", "rpc-method", rpcItem(method))
}

func bal(t *testing.T, addr types.Address, zts types.ZenonTokenStandard) *big.Int {
	t.Helper()
	b, err := net.Balance(addr, zts)
	if err != nil {
		t.Fatalf("balance of %s: %v", addr, err)
	}
	return b
}

func wantBig(t *testing.T, what string, got, want *big.Int) {
	t.Helper()
	if got.Cmp(want) != 0 {
		t.Errorf("%s: got %s, want %s", what, got, want)
	}
}

// call sends a contract call and waits for the contract's receive.
func call(kp *wallet.KeyPair, to types.Address, data []byte, zts types.ZenonTokenStandard, amount *big.Int) (*Result, error) {
	return net.CallContract(kp, to, data, zts, amount, wait)
}

// callOK, callFail and callReject send a contract call and require one of
// the three outcomes: executed, refused by the contract, refused at publish.
func callOK(t *testing.T, kp *wallet.KeyPair, to types.Address, data []byte, zts types.ZenonTokenStandard, amount *big.Int) *Result {
	t.Helper()
	res, err := call(kp, to, data, zts, amount)
	return mustOK(t, res, err)
}

func callFail(t *testing.T, kp *wallet.KeyPair, to types.Address, data []byte, zts types.ZenonTokenStandard, amount *big.Int) *Result {
	t.Helper()
	res, err := call(kp, to, data, zts, amount)
	return mustFail(t, res, err)
}

func callReject(t *testing.T, want string, kp *wallet.KeyPair, to types.Address, data []byte, zts types.ZenonTokenStandard, amount *big.Int) {
	t.Helper()
	_, err := net.Send(kp, &nom.AccountBlock{BlockType: nom.BlockTypeUserSend, ToAddress: to, Data: data, TokenStandard: zts, Amount: amount}, &Mutator{})
	mustReject(t, err, want)
}

// mustOK requires that the call was accepted and the contract executed it.
func mustOK(t *testing.T, res *Result, err error) *Result {
	t.Helper()
	if err != nil {
		t.Fatalf("call refused: %v", err)
	}
	if !res.OK() {
		t.Fatalf("contract returned status %d for send %s, want success", res.Status, res.Send.Hash)
	}
	return res
}

// mustReject requires that the node refused the block at publish with a
// message containing want.
func mustReject(t *testing.T, err error, want string) {
	t.Helper()
	if err == nil {
		t.Fatalf("block was accepted, want refusal containing %q", want)
	}
	if !strings.Contains(err.Error(), want) {
		t.Fatalf("refused with %q, want a message containing %q", err.Error(), want)
	}
}

// mustFail requires that the block was accepted and the contract then
// refused it: a receive block with the fail status. The reason is not on
// chain.
func mustFail(t *testing.T, res *Result, err error) *Result {
	t.Helper()
	if err != nil {
		t.Fatalf("call refused at publish (%v), want a failed contract receive", err)
	}
	if res.Status != StatusFail {
		t.Fatalf("contract returned status %d for send %s, want fail (%d)", res.Status, res.Send.Hash, StatusFail)
	}
	return res
}

// transfer sends tokens to a user address and returns the published block.
func transfer(t *testing.T, from *wallet.KeyPair, to types.Address, zts types.ZenonTokenStandard, amount *big.Int, data []byte) *nom.AccountBlock {
	t.Helper()
	b, err := net.Send(from, &nom.AccountBlock{BlockType: nom.BlockTypeUserSend, ToAddress: to, TokenStandard: zts, Amount: amount, Data: data}, nil)
	if err != nil {
		t.Fatalf("send from %s: %v", from.Address, err)
	}
	return b
}

// tryTransfer publishes a send built with a mutator and returns the error.
func tryTransfer(from *wallet.KeyPair, to types.Address, zts types.ZenonTokenStandard, amount *big.Int, mut *Mutator) error {
	_, err := net.Send(from, &nom.AccountBlock{BlockType: nom.BlockTypeUserSend, ToAddress: to, TokenStandard: zts, Amount: amount}, mut)
	return err
}

// receiveAll receives at least want pending sends and fails the case
// otherwise.
func receiveAll(t *testing.T, kp *wallet.KeyPair, want int) {
	t.Helper()
	if _, err := net.ReceiveAll(kp, want, 2*wait); err != nil {
		t.Fatalf("receive for %s: %v", kp.Address, err)
	}
}

// newAccount returns a fresh account of this run. The funder sends it the
// given whole-coin amounts and fuses fuseQsr QSR for it; with fuseQsr 0 the
// account has no plasma and pays for its blocks with proof of work.
func newAccount(t *testing.T, slot uint32, znnCoins, qsrCoins, fuseQsr int64) *wallet.KeyPair {
	t.Helper()
	kp := net.Key(runBase + slot)
	funder := net.Key(IdxFunder)
	pending := 0
	if fuseQsr > 0 {
		callOK(t, funder, types.PlasmaContract, definition.ABIPlasma.PackMethodPanic(definition.FuseMethodName, kp.Address), qsr, coins(fuseQsr))
	}
	if znnCoins > 0 {
		transfer(t, funder, kp.Address, znn, coins(znnCoins), nil)
		pending++
	}
	if qsrCoins > 0 {
		transfer(t, funder, kp.Address, qsr, coins(qsrCoins), nil)
		pending++
	}
	if pending > 0 {
		receiveAll(t, kp, pending)
	}
	return kp
}
