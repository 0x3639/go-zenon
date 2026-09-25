package protocol_test

import (
	"math/big"
	"sync"
	"testing"
	"time"

	"github.com/pkg/errors"

	"github.com/zenon-network/go-zenon/chain"
	"github.com/zenon-network/go-zenon/chain/cache/storage"
	g "github.com/zenon-network/go-zenon/chain/genesis/mock"
	"github.com/zenon-network/go-zenon/chain/nom"
	"github.com/zenon-network/go-zenon/common"
	"github.com/zenon-network/go-zenon/common/types"
	"github.com/zenon-network/go-zenon/protocol"
	"github.com/zenon-network/go-zenon/vm"
	"github.com/zenon-network/go-zenon/zenon/mock"
)

// rollbackCountingChain records how often the bridge rolls the canonical
// chain back.
type rollbackCountingChain struct {
	chain.Chain
	rollbacks int
	// failCommitOf, when set, makes the commit of the momentum with this hash
	// fail after its cache update has already gone through, the way a store
	// error would.
	failCommitOf types.Hash
}

func (c *rollbackCountingChain) RollbackTo(insertLocker sync.Locker, identifier types.HashHeight) error {
	c.rollbacks++
	return c.Chain.RollbackTo(insertLocker, identifier)
}

func (c *rollbackCountingChain) AddMomentumTransaction(insertLocker sync.Locker, transaction *nom.MomentumTransaction) error {
	if !c.failCommitOf.IsZero() && transaction.Momentum.Hash == c.failCommitOf {
		return errors.Errorf("injected commit failure for %v", transaction.Momentum.Identifier())
	}
	return c.Chain.AddMomentumTransaction(insertLocker, transaction)
}

// chainSnapshot is everything a failed side-chain insert must leave alone.
type chainSnapshot struct {
	frontier      types.HashHeight
	cacheFrontier types.HashHeight
	hashes        map[uint64]types.Hash
	balances      map[types.Address]string
}

func snapshotChain(t *testing.T, c chain.Chain) chainSnapshot {
	t.Helper()
	store := c.GetFrontierMomentumStore()
	frontier, err := store.GetFrontierMomentum()
	common.FailIfErr(t, err)
	snapshot := chainSnapshot{
		frontier:      frontier.Identifier(),
		cacheFrontier: c.GetFrontierCacheStore().Identifier(),
		hashes:        make(map[uint64]types.Hash, frontier.Height),
	}
	for height := uint64(1); height <= frontier.Height; height++ {
		momentum, err := store.GetMomentumByHeight(height)
		common.FailIfErr(t, err)
		snapshot.hashes[height] = momentum.Hash
	}
	common.Expect(t, snapshot.cacheFrontier, snapshot.frontier)
	snapshot.balances = make(map[types.Address]string)
	for _, address := range []types.Address{g.User1.Address, g.User2.Address, g.Pillar1.Address, types.PillarContract} {
		balance, err := store.GetAccountStore(address).GetBalance(types.ZnnTokenStandard)
		common.FailIfErr(t, err)
		snapshot.balances[address] = balance.String()
	}
	return snapshot
}

func expectChainEquals(t *testing.T, c chain.Chain, expected chainSnapshot) {
	t.Helper()
	actual := snapshotChain(t, c)
	common.Expect(t, actual.frontier, expected.frontier)
	common.Expect(t, actual.cacheFrontier, expected.cacheFrontier)
	common.Expect(t, len(actual.hashes), len(expected.hashes))
	for height, hash := range expected.hashes {
		common.Expect(t, actual.hashes[height], hash)
	}
	for address, balance := range expected.balances {
		common.Expect(t, actual.balances[address], balance)
	}
}

// attackerMomentum builds a momentum on top of previous that is internally
// consistent (hash recomputed, signed by a key that is not an elected
// producer) but carries a changes-hash the node cannot reproduce. It passes
// every check that needs no chain state and fails the first one that does.
func attackerMomentum(previous *nom.Momentum, height uint64) *nom.Momentum {
	timestamp := time.Unix(int64(previous.TimestampUnix+10), 0)
	momentum := &nom.Momentum{
		Version:         previous.Version,
		ChainIdentifier: previous.ChainIdentifier,
		PreviousHash:    previous.Hash,
		Height:          height,
		TimestampUnix:   previous.TimestampUnix + 10,
		Timestamp:       &timestamp,
		Content:         nom.NewMomentumContent(nil),
		ChangesHash:     types.NewHash([]byte("not the state this momentum produces")),
	}
	momentum.Hash = momentum.ComputeHash()
	momentum.PublicKey = g.User1.Public
	momentum.Signature = g.User1.Sign(momentum.Hash.Bytes())
	return momentum
}

func detailedOf(momentums ...*nom.Momentum) []*nom.DetailedMomentum {
	detailed := make([]*nom.DetailedMomentum, len(momentums))
	for i, momentum := range momentums {
		detailed[i] = &nom.DetailedMomentum{Momentum: momentum, AccountBlocks: []*nom.AccountBlock{}}
	}
	return detailed
}

// invalidTail chains attacker momentums on top of previous up to and
// including height top. Each passes the stateless checks and fails the first
// state-dependent one.
func invalidTail(previous *nom.Momentum, top uint64) []*nom.DetailedMomentum {
	tail := make([]*nom.Momentum, 0, top-previous.Height)
	for height := previous.Height + 1; height <= top; height++ {
		previous = attackerMomentum(previous, height)
		tail = append(tail, previous)
	}
	return detailedOf(tail...)
}

func momentumAt(t *testing.T, c chain.Chain, height uint64) *nom.Momentum {
	t.Helper()
	momentum, err := c.GetFrontierMomentumStore().GetMomentumByHeight(height)
	common.FailIfErr(t, err)
	if momentum == nil {
		t.Fatalf("no momentum at height %d", height)
	}
	return momentum
}

// expectRolledBack asserts the canonical chain was rolled back at least once;
// a restore rolls the failed candidates back again, so the count is not one.
func expectRolledBack(t *testing.T, c *rollbackCountingChain) {
	t.Helper()
	if c.rollbacks == 0 {
		t.Fatal("expected the side chain to have triggered a rollback")
	}
}

func newSideChainBridge(z mock.MockZenon) (*rollbackCountingChain, protocol.ChainBridge) {
	counting := &rollbackCountingChain{Chain: z.Chain()}
	supervisor := vm.NewSupervisor(z.Chain(), z.Consensus())
	return counting, protocol.NewChainBridge(counting, z.Consensus(), z.Verifier(), supervisor)
}

func TestInsertChain_SideChainRestoredWhenCandidateFailsAfterRollback(t *testing.T) {
	z := mock.NewMockZenon(t)
	defer z.StopPanic()
	z.InsertMomentumsTo(6)
	counting, bridge := newSideChainBridge(z)
	before := snapshotChain(t, z.Chain())

	// Side chain forking one below the frontier, longer than ours.
	target := momentumAt(t, z.Chain(), 5)
	head := attackerMomentum(target, 6)
	tail := attackerMomentum(head, 7)

	_, err := bridge.InsertChain(detailedOf(head, tail))
	if err == nil {
		t.Fatal("expected the side chain to be rejected")
	}
	// The candidate passes every stateless check, so the rollback happens;
	// the failure comes from the state-dependent checks after it.
	expectRolledBack(t, counting)
	expectChainEquals(t, z.Chain(), before)
}

func TestInsertChain_SideChainWithInvalidSignatureIsRejectedBeforeRollback(t *testing.T) {
	z := mock.NewMockZenon(t)
	defer z.StopPanic()
	z.InsertMomentumsTo(6)
	counting, bridge := newSideChainBridge(z)
	before := snapshotChain(t, z.Chain())

	target := momentumAt(t, z.Chain(), 5)
	head := attackerMomentum(target, 6)
	head.Signature[0] ^= 0xff
	tail := attackerMomentum(head, 7)

	_, err := bridge.InsertChain(detailedOf(head, tail))
	if err == nil {
		t.Fatal("expected the side chain to be rejected")
	}
	common.Expect(t, counting.rollbacks, 0)
	expectChainEquals(t, z.Chain(), before)
}

func TestInsertChain_SideChainWithTamperedHashIsRejectedBeforeRollback(t *testing.T) {
	z := mock.NewMockZenon(t)
	defer z.StopPanic()
	z.InsertMomentumsTo(6)
	counting, bridge := newSideChainBridge(z)
	before := snapshotChain(t, z.Chain())

	target := momentumAt(t, z.Chain(), 5)
	head := attackerMomentum(target, 6)
	head.Hash = types.NewHash([]byte("advertised hash that does not match the content"))
	tail := attackerMomentum(head, 7)

	_, err := bridge.InsertChain(detailedOf(head, tail))
	if err == nil {
		t.Fatal("expected the side chain to be rejected")
	}
	common.Expect(t, counting.rollbacks, 0)
	expectChainEquals(t, z.Chain(), before)
}

// forkFixture holds a node at an original branch plus a genuine, valid
// alternative branch forking one below the original branch's lowest momentum.
type forkFixture struct {
	z        mock.MockZenon
	original chainSnapshot
	// removed is how many momentums the original branch has above the fork
	// point.
	removed int
	fork    []*nom.DetailedMomentum
}

// newForkFixture builds the original branch on one node and the fork on a
// second node so that neither branch has to be rolled back to reach the fork
// point; the cache only keeps 100 rollback steps, which a long fork exceeds.
// Both nodes share genesis and a deterministic clock, so the momentums below
// the fork point are byte-identical on both. The original branch spans heights
// forkBase+1..originalTop, the fork spans forkBase+1..forkTop.
func newForkFixture(t *testing.T, originalTop, forkTop uint64) *forkFixture {
	const forkBase = uint64(5)
	z := mock.NewMockZenon(t)
	z.InsertMomentumsTo(originalTop)

	other := mock.NewMockZenon(t)
	defer other.StopPanic()
	other.InsertMomentumsTo(forkBase)
	common.Expect(t, momentumAt(t, other.Chain(), forkBase).Hash, momentumAt(t, z.Chain(), forkBase).Hash)
	// Produce a different branch from the fork point: add a transaction so
	// the content differs, then let the pillars build the rest.
	other.InsertSendBlock(&nom.AccountBlock{
		Address:       g.User1.Address,
		ToAddress:     g.User2.Address,
		TokenStandard: types.ZnnTokenStandard,
		Amount:        big.NewInt(1),
	}, nil, mock.SkipVmChanges)
	other.InsertMomentumsTo(forkTop)
	fork := make([]*nom.DetailedMomentum, 0, forkTop-forkBase)
	for height := forkBase + 1; height <= forkTop; height++ {
		detailed, err := other.Chain().GetFrontierMomentumStore().PrefetchMomentum(momentumAt(t, other.Chain(), height))
		common.FailIfErr(t, err)
		fork = append(fork, detailed)
	}
	if fork[0].Momentum.Hash == momentumAt(t, z.Chain(), forkBase+1).Hash {
		t.Fatal("fork did not diverge from the original branch")
	}

	return &forkFixture{
		z:        z,
		original: snapshotChain(t, z.Chain()),
		removed:  int(originalTop - forkBase),
		fork:     fork,
	}
}

// expectFrontierAt asserts both the canonical and the cache frontier sit at
// the given momentum.
func expectFrontierAt(t *testing.T, c chain.Chain, momentum *nom.Momentum) {
	t.Helper()
	common.Expect(t, c.GetFrontierMomentumStore().Identifier(), momentum.Identifier())
	common.Expect(t, c.GetFrontierCacheStore().Identifier(), momentum.Identifier())
}

func TestInsertChain_ValidLongerSideChainReplacesBranch(t *testing.T) {
	f := newForkFixture(t, 6, 7)
	defer f.z.StopPanic()
	counting, bridge := newSideChainBridge(f.z)

	_, err := bridge.InsertChain(f.fork)
	common.FailIfErr(t, err)
	common.Expect(t, counting.rollbacks, 1)
	common.Expect(t, f.z.Chain().GetFrontierMomentumStore().Identifier(), f.fork[1].Momentum.Identifier())
	common.Expect(t, f.z.Chain().GetFrontierCacheStore().Identifier(), f.fork[1].Momentum.Identifier())
}

func TestInsertChain_SideChainRestoredWhenLaterCandidateFails(t *testing.T) {
	f := newForkFixture(t, 6, 7)
	defer f.z.StopPanic()
	counting, bridge := newSideChainBridge(f.z)

	// A genuine first candidate that replaces our branch, followed by one
	// that fails only after it has been applied.
	bad := attackerMomentum(f.fork[0].Momentum, 7)
	_, err := bridge.InsertChain([]*nom.DetailedMomentum{f.fork[0], detailedOf(bad)[0]})
	if err == nil {
		t.Fatal("expected the side chain to be rejected")
	}
	expectRolledBack(t, counting)
	expectChainEquals(t, f.z.Chain(), f.original)

	// The restored state is real state: the genuine fork still replaces the
	// branch afterwards, which requires its changes-hash to be reproducible
	// from what was put back.
	_, err = bridge.InsertChain(f.fork)
	common.FailIfErr(t, err)
	common.Expect(t, f.z.Chain().GetFrontierMomentumStore().Identifier(), f.fork[1].Momentum.Identifier())
}

// TestInsertChain_RestoreBoundaryAtRemovedBranchLength pins the rule that
// decides between restoring the original branch and keeping the committed
// replacement prefix: the original comes back only while the prefix is no
// longer than it. A longer prefix is a valid chain longer than the one the
// node had, so it stays, exactly as it would have before restores existed.
func TestInsertChain_RestoreBoundaryAtRemovedBranchLength(t *testing.T) {
	cases := []struct {
		name      string
		committed int
		restored  bool
	}{
		{"prefix shorter than removed branch is restored", 1, true},
		{"prefix as long as removed branch is restored", 2, true},
		{"prefix longer than removed branch is kept", 3, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := newForkFixture(t, 7, 9)
			defer f.z.StopPanic()
			common.Expect(t, f.removed, 2)
			counting, bridge := newSideChainBridge(f.z)

			// The genuine prefix, then invalid candidates up to a height that
			// keeps the side chain taller than our frontier.
			candidates := append([]*nom.DetailedMomentum{}, f.fork[:tc.committed]...)
			candidates = append(candidates, invalidTail(f.fork[tc.committed-1].Momentum, 10)...)

			index, err := bridge.InsertChain(candidates)
			if err == nil {
				t.Fatal("expected the side chain to be rejected")
			}
			common.Expect(t, index, tc.committed)
			if tc.restored {
				common.Expect(t, counting.rollbacks, 2)
				expectChainEquals(t, f.z.Chain(), f.original)
			} else {
				common.Expect(t, counting.rollbacks, 1)
				expectFrontierAt(t, f.z.Chain(), f.fork[tc.committed-1].Momentum)
			}

			// Whatever was kept is real state: the rest of the genuine fork
			// still inserts on top of it.
			_, err = bridge.InsertChain(f.fork)
			common.FailIfErr(t, err)
			expectFrontierAt(t, f.z.Chain(), f.fork[len(f.fork)-1].Momentum)
		})
	}
}

// TestInsertChain_ReplacementPrefixBeyondCacheWindowIsKept commits more
// replacement momentums than the cache keeps rollback steps for, then fails
// the next candidate. A restore would have to roll the cache back further
// than it can go, after the canonical chain has already been rolled back,
// and the node would exit with a cache it cannot reconcile on restart. The
// committed prefix is longer than the removed branch, so it stays instead.
func TestInsertChain_ReplacementPrefixBeyondCacheWindowIsKept(t *testing.T) {
	committed := storage.GetRollbackCacheSize() + 1
	f := newForkFixture(t, 6, uint64(5+committed+1))
	defer f.z.StopPanic()
	counting, bridge := newSideChainBridge(f.z)

	candidates := append([]*nom.DetailedMomentum{}, f.fork[:committed]...)
	last := f.fork[committed-1].Momentum
	candidates = append(candidates, invalidTail(last, last.Height+1)...)

	index, err := bridge.InsertChain(candidates)
	if err == nil {
		t.Fatal("expected the side chain to be rejected")
	}
	common.Expect(t, index, committed)
	common.Expect(t, counting.rollbacks, 1)
	expectFrontierAt(t, f.z.Chain(), last)

	// The node carries on from the kept prefix.
	_, err = bridge.InsertChain(f.fork[committed:])
	common.FailIfErr(t, err)
	expectFrontierAt(t, f.z.Chain(), f.fork[len(f.fork)-1].Momentum)
}

func TestInsertChain_SideChainDepthBoundary(t *testing.T) {
	z := mock.NewMockZenon(t)
	defer z.StopPanic()
	z.InsertMomentumsTo(40)
	counting, bridge := newSideChainBridge(z)
	before := snapshotChain(t, z.Chain())

	// Depth 31: rejected before anything is touched.
	tooDeep := momentumAt(t, z.Chain(), 9)
	head := attackerMomentum(tooDeep, 10)
	tail := attackerMomentum(head, 41)
	_, err := bridge.InsertChain(detailedOf(head, tail))
	if err == nil {
		t.Fatal("expected a 31-deep side chain to be rejected")
	}
	common.Expect(t, counting.rollbacks, 0)
	expectChainEquals(t, z.Chain(), before)

	// Depth 30: allowed, rolls back, fails after, and every removed momentum
	// comes back.
	deepest := momentumAt(t, z.Chain(), 10)
	head = attackerMomentum(deepest, 11)
	tail = attackerMomentum(head, 41)
	_, err = bridge.InsertChain(detailedOf(head, tail))
	if err == nil {
		t.Fatal("expected the side chain to be rejected")
	}
	expectRolledBack(t, counting)
	expectChainEquals(t, z.Chain(), before)
}

// storedPatchDumps returns the serialized state patch of every momentum above
// identifier exactly as the momentum store holds it, frontier writes included.
// It deliberately does not go through CaptureBranchAbove, which strips those
// writes and would hide the very growth this is used to detect.
func storedPatchDumps(t *testing.T, c chain.Chain, identifier types.HashHeight) [][]byte {
	t.Helper()
	frontier := c.GetFrontierMomentumStore().Identifier()
	dumps := make([][]byte, 0, frontier.Height-identifier.Height)
	for height := identifier.Height + 1; height <= frontier.Height; height++ {
		patch := c.GetMomentumPatch(momentumAt(t, c, height).Identifier())
		if patch == nil {
			t.Fatalf("no stored patch for height %d", height)
		}
		dumps = append(dumps, patch.Dump())
	}
	return dumps
}

func TestInsertChain_RepeatedRestoresKeepStoredPatchesIdentical(t *testing.T) {
	z := mock.NewMockZenon(t)
	defer z.StopPanic()
	z.InsertMomentumsTo(6)
	_, bridge := newSideChainBridge(z)
	before := snapshotChain(t, z.Chain())
	target := momentumAt(t, z.Chain(), 4).Identifier()
	patchesBefore := storedPatchDumps(t, z.Chain(), target)

	for attempt := 0; attempt < 3; attempt++ {
		head := attackerMomentum(momentumAt(t, z.Chain(), 4), 5)
		tail := attackerMomentum(head, 7)
		_, err := bridge.InsertChain(detailedOf(head, tail))
		if err == nil {
			t.Fatalf("attempt %d: expected the side chain to be rejected", attempt)
		}
		expectChainEquals(t, z.Chain(), before)

		patchesAfter := storedPatchDumps(t, z.Chain(), target)
		common.Expect(t, len(patchesAfter), len(patchesBefore))
		for i := range patchesBefore {
			if string(patchesAfter[i]) != string(patchesBefore[i]) {
				t.Fatalf("attempt %d: stored patch for height %d changed: %d bytes before, %d bytes after", attempt, target.Height+uint64(i)+1, len(patchesBefore[i]), len(patchesAfter[i]))
			}
		}
	}
}

// TestInsertChain_RestoreBoundaryAtDepthLimit runs the restore/keep decision
// at the deepest fork the depth check admits: a removed branch of 30. A
// committed prefix of 30 is restored, which is the longest restore there is,
// and a prefix of 31 is kept.
func TestInsertChain_RestoreBoundaryAtDepthLimit(t *testing.T) {
	cases := []struct {
		name      string
		committed int
		restored  bool
	}{
		{"prefix as long as the 30-deep removed branch is restored", 30, true},
		{"prefix longer than the 30-deep removed branch is kept", 31, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := newForkFixture(t, 35, 37)
			defer f.z.StopPanic()
			common.Expect(t, f.removed, 30)
			counting, bridge := newSideChainBridge(f.z)

			candidates := append([]*nom.DetailedMomentum{}, f.fork[:tc.committed]...)
			last := f.fork[tc.committed-1].Momentum
			candidates = append(candidates, invalidTail(last, last.Height+1)...)

			index, err := bridge.InsertChain(candidates)
			if err == nil {
				t.Fatal("expected the side chain to be rejected")
			}
			common.Expect(t, index, tc.committed)
			if tc.restored {
				common.Expect(t, counting.rollbacks, 2)
				expectChainEquals(t, f.z.Chain(), f.original)
			} else {
				common.Expect(t, counting.rollbacks, 1)
				expectFrontierAt(t, f.z.Chain(), last)
			}

			_, err = bridge.InsertChain(f.fork)
			common.FailIfErr(t, err)
			expectFrontierAt(t, f.z.Chain(), f.fork[len(f.fork)-1].Momentum)
		})
	}
}

// TestInsertChain_KnownPrefixOffsetsReturnedIndex feeds momentums the node
// already has ahead of the side chain, as a downloader batch that starts
// below the fork point does. The known prefix is skipped, the restore/keep
// decision counts only the committed candidates, and the returned index still
// points at the failing candidate's position in the batch as given.
func TestInsertChain_KnownPrefixOffsetsReturnedIndex(t *testing.T) {
	f := newForkFixture(t, 7, 9)
	defer f.z.StopPanic()
	common.Expect(t, f.removed, 2)
	counting, bridge := newSideChainBridge(f.z)

	store := f.z.Chain().GetFrontierMomentumStore()
	known := make([]*nom.DetailedMomentum, 0, 2)
	for height := uint64(4); height <= 5; height++ {
		detailed, err := store.PrefetchMomentum(momentumAt(t, f.z.Chain(), height))
		common.FailIfErr(t, err)
		known = append(known, detailed)
	}

	// Two known momentums, two genuine candidates that commit, then an
	// invalid one: as many committed as removed, so the branch is restored.
	candidates := append([]*nom.DetailedMomentum{}, known...)
	candidates = append(candidates, f.fork[:2]...)
	candidates = append(candidates, invalidTail(f.fork[1].Momentum, 10)...)

	index, err := bridge.InsertChain(candidates)
	if err == nil {
		t.Fatal("expected the side chain to be rejected")
	}
	common.Expect(t, index, len(known)+2)
	common.Expect(t, counting.rollbacks, 2)
	expectChainEquals(t, f.z.Chain(), f.original)

	// The same known prefix ahead of the genuine fork still replaces the
	// branch.
	candidates = append(append([]*nom.DetailedMomentum{}, known...), f.fork...)
	_, err = bridge.InsertChain(candidates)
	common.FailIfErr(t, err)
	expectFrontierAt(t, f.z.Chain(), f.fork[len(f.fork)-1].Momentum)
}

// TestInsertChain_RestoreAfterCommitFailsPastCacheUpdate fails a candidate at
// the last step, after its cache entry is written and before its momentum is
// committed. The insert path compensates by rolling the cache back one step;
// the restore then has to start from a chain and cache that agree, and put
// the original branch back on both.
func TestInsertChain_RestoreAfterCommitFailsPastCacheUpdate(t *testing.T) {
	f := newForkFixture(t, 7, 9)
	defer f.z.StopPanic()
	common.Expect(t, f.removed, 2)
	counting, bridge := newSideChainBridge(f.z)

	// The first candidate commits, the second passes every check and fails
	// only at the commit.
	counting.failCommitOf = f.fork[1].Momentum.Hash
	index, err := bridge.InsertChain(f.fork)
	if err == nil {
		t.Fatal("expected the side chain to be rejected")
	}
	common.Expect(t, index, 1)
	common.Expect(t, counting.rollbacks, 2)
	expectChainEquals(t, f.z.Chain(), f.original)

	// With the fault gone the same fork replaces the branch.
	counting.failCommitOf = types.Hash{}
	_, err = bridge.InsertChain(f.fork)
	common.FailIfErr(t, err)
	expectFrontierAt(t, f.z.Chain(), f.fork[len(f.fork)-1].Momentum)
}

// momentumEvent is one notification a chain listener received.
type momentumEvent struct {
	kind       string
	identifier types.HashHeight
}

// recordingListener keeps every momentum event in the order it was delivered.
type recordingListener struct {
	events []momentumEvent
}

func (l *recordingListener) InsertMomentum(detailed *nom.DetailedMomentum) {
	l.events = append(l.events, momentumEvent{"insert", detailed.Momentum.Identifier()})
}
func (l *recordingListener) DeleteMomentum(detailed *nom.DetailedMomentum) {
	l.events = append(l.events, momentumEvent{"delete", detailed.Momentum.Identifier()})
}

// TestInsertChain_ListenersSeeDeleteThenInsertOnRestore pins the event
// sequence a restore produces: the removed branch is announced as deleted,
// the committed prefix as inserted then deleted, and the original momentums
// as inserted again. Nothing is suppressed or replayed out of order.
func TestInsertChain_ListenersSeeDeleteThenInsertOnRestore(t *testing.T) {
	f := newForkFixture(t, 7, 9)
	defer f.z.StopPanic()
	common.Expect(t, f.removed, 2)
	_, bridge := newSideChainBridge(f.z)
	original6 := momentumAt(t, f.z.Chain(), 6).Identifier()
	original7 := momentumAt(t, f.z.Chain(), 7).Identifier()

	listener := &recordingListener{}
	f.z.Chain().Register(listener)
	defer f.z.Chain().UnRegister(listener)

	// One genuine candidate commits, the next fails: the original two come
	// back.
	candidates := append([]*nom.DetailedMomentum{}, f.fork[:1]...)
	candidates = append(candidates, invalidTail(f.fork[0].Momentum, 8)...)
	_, err := bridge.InsertChain(candidates)
	if err == nil {
		t.Fatal("expected the side chain to be rejected")
	}
	expectChainEquals(t, f.z.Chain(), f.original)

	expected := []momentumEvent{
		{"delete", original7},
		{"delete", original6},
		{"insert", f.fork[0].Momentum.Identifier()},
		{"delete", f.fork[0].Momentum.Identifier()},
		{"insert", original6},
		{"insert", original7},
	}
	common.Expect(t, len(listener.events), len(expected))
	for i := range expected {
		common.Expect(t, listener.events[i], expected[i])
	}
}

// TestInsertChain_PendingPoolAfterRestore pins what a restore does to the
// account pool, which it deliberately does not restore. Blocks that were
// pending before the side chain arrived are dropped by the rollback, exactly
// as any rollback drops them. A valid block carried by the failing candidate
// was force-inserted before the candidate failed and stays pending, as if a
// peer had relayed it on its own.
func TestInsertChain_PendingPoolAfterRestore(t *testing.T) {
	f := newForkFixture(t, 6, 7)
	defer f.z.StopPanic()
	_, bridge := newSideChainBridge(f.z)

	pending := f.z.InsertSendBlock(&nom.AccountBlock{
		Address:       g.User2.Address,
		ToAddress:     g.User1.Address,
		TokenStandard: types.ZnnTokenStandard,
		Amount:        big.NewInt(1),
	}, nil, mock.SkipVmChanges)
	common.Expect(t, len(f.z.Chain().GetAllUncommittedAccountBlocks()), 1)

	// A candidate carrying the fork's genuine account block, but signed by a
	// key that is not the elected producer and advertising a changes-hash the
	// node can't reproduce: the block applies, the momentum fails.
	target := momentumAt(t, f.z.Chain(), 5)
	head := attackerMomentum(target, 6)
	head.Content = f.fork[0].Momentum.Content
	head.Hash = head.ComputeHash()
	head.Signature = g.User1.Sign(head.Hash.Bytes())
	carried := f.fork[0].AccountBlocks
	if len(carried) == 0 {
		t.Fatal("fork head carries no account block")
	}
	candidates := []*nom.DetailedMomentum{
		{Momentum: head, AccountBlocks: carried},
		detailedOf(attackerMomentum(head, 7))[0],
	}

	_, err := bridge.InsertChain(candidates)
	if err == nil {
		t.Fatal("expected the side chain to be rejected")
	}
	expectChainEquals(t, f.z.Chain(), f.original)

	uncommitted := f.z.Chain().GetAllUncommittedAccountBlocks()
	common.Expect(t, len(uncommitted), len(carried))
	for i, block := range carried {
		common.Expect(t, uncommitted[i].Hash, block.Hash)
	}
	for _, block := range uncommitted {
		if block.Hash == pending.Hash {
			t.Fatal("block pending before the side chain survived the rollback")
		}
	}
}
