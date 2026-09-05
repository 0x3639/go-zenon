package protocol

import (
	"errors"
	"strings"
	"testing"

	"github.com/ethereum/go-ethereum/rlp"

	"github.com/zenon-network/go-zenon/chain/nom"
	"github.com/zenon-network/go-zenon/common/types"
	"github.com/zenon-network/go-zenon/p2p"
	"github.com/zenon-network/go-zenon/protocol/downloader"
	"github.com/zenon-network/go-zenon/protocol/fetcher"
)

// GetBlocksMsg names blocks by hash, and every named hash costs the receiver a
// store lookup whether or not the block exists. The tests below drive the real
// handleMsg entry point over a message pipe and count those lookups, so the
// per-request work bound is asserted on the receiver's actual behaviour rather
// than on a helper.

// lookupCountingChain is the minimal chainManager the GetBlocksMsg path needs.
// It serves the blocks it was given and counts every GetBlock call.
type lookupCountingChain struct {
	known   map[types.Hash]*nom.DetailedMomentum
	lookups int
}

func (c *lookupCountingChain) GetBlock(hash types.Hash) *nom.DetailedMomentum {
	c.lookups++
	return c.known[hash]
}

func (c *lookupCountingChain) HasBlock(types.Hash) bool { panic("not used by GetBlocksMsg") }
func (c *lookupCountingChain) GetBlockHashesFromHash(types.Hash, uint64) ([]types.Hash, error) {
	panic("not used by GetBlocksMsg")
}
func (c *lookupCountingChain) GetBlockByNumber(uint64) (*nom.Momentum, error) {
	panic("not used by GetBlocksMsg")
}
func (c *lookupCountingChain) CurrentBlock() *nom.Momentum { panic("not used by GetBlocksMsg") }
func (c *lookupCountingChain) Status() (uint64, types.Hash, types.Hash) {
	panic("not used by GetBlocksMsg")
}
func (c *lookupCountingChain) InsertChain([]*nom.DetailedMomentum) (int, error) {
	panic("not used by GetBlocksMsg")
}
func (c *lookupCountingChain) VerifyMomentum(*nom.DetailedMomentum) error {
	panic("not used by GetBlocksMsg")
}

// knownBlocks builds n distinct momentums (heights 2..n+1, so SendBlocks'
// genesis special case stays out of the way) and returns them with their
// hashes in request order.
func knownBlocks(n int) (map[types.Hash]*nom.DetailedMomentum, []types.Hash) {
	known := make(map[types.Hash]*nom.DetailedMomentum, n)
	hashes := make([]types.Hash, 0, n)
	for i := 0; i < n; i++ {
		momentum := &nom.Momentum{Height: uint64(i + 2)}
		momentum.Hash = momentum.ComputeHash()
		known[momentum.Hash] = &nom.DetailedMomentum{Momentum: momentum}
		hashes = append(hashes, momentum.Hash)
	}
	return known, hashes
}

// unknownHashes returns n distinct hashes that no test chain stores.
func unknownHashes(n int) []types.Hash {
	hashes := make([]types.Hash, n)
	for i := range hashes {
		hashes[i] = types.NewHash([]byte{'m', 'i', 's', 's', byte(i >> 24), byte(i >> 16), byte(i >> 8), byte(i)})
	}
	return hashes
}

type getBlocksResult struct {
	blocks   []*nom.DetailedMomentum
	answered bool // whether the handler wrote a BlocksMsg before the pipe closed
}

// serveGetBlocks sends one GetBlocksMsg naming hashes to a handler backed by
// chain, returns the handler's error, and reports whether a BlocksMsg reply
// was written and what it carried.
func serveGetBlocks(t *testing.T, chain *lookupCountingChain, hashes []types.Hash) (getBlocksResult, error) {
	t.Helper()
	return serveGetBlocksPayload(t, chain, hashes)
}

// serveGetBlocksPayload is serveGetBlocks for an arbitrary RLP-encodable
// payload, so a test can put something on the wire that is not a well-formed
// list of hashes.
func serveGetBlocksPayload(t *testing.T, chain *lookupCountingChain, payload interface{}) (getBlocksResult, error) {
	t.Helper()

	app, net := p2p.MsgPipe()
	defer func() { _ = app.Close() }()

	pm := &ProtocolManager{chainman: chain}
	p := &peer{rw: net, id: "test-peer"}

	// The pipe blocks the writer until the reader has consumed the whole
	// payload, and the handler only discards what it did not decode after it
	// has replied. The request writer and the reply reader therefore run on
	// separate goroutines so neither can wait on the other.
	sendErr := make(chan error, 1)
	go func() {
		sendErr <- p2p.Send(app, GetBlocksMsg, payload)
	}()

	done := make(chan getBlocksResult, 1)
	go func() {
		msg, err := app.ReadMsg()
		if err != nil {
			// The pipe was closed without a reply.
			done <- getBlocksResult{}
			return
		}
		result := getBlocksResult{answered: true}
		if msg.Code != BlocksMsg {
			t.Errorf("reply code %d, want BlocksMsg (%d)", msg.Code, BlocksMsg)
		}
		stream := rlp.NewStream(msg.Payload, uint64(msg.Size))
		if err := stream.Decode(&result.blocks); err != nil {
			t.Errorf("decode reply: %v", err)
		}
		done <- result
	}()

	err := pm.handleMsg(p)
	if err != nil {
		// No reply is coming; release the reader.
		_ = app.Close()
	}
	if serr := <-sendErr; serr != nil {
		t.Fatalf("send request: %v", serr)
	}
	return <-done, err
}

func TestMaxBlocksRequest_CoversEveryHonestRequester(t *testing.T) {
	if MaxBlocksRequest < downloader.MaxBlockFetch {
		t.Fatalf("MaxBlocksRequest %d is below the downloader batch of %d", MaxBlocksRequest, downloader.MaxBlockFetch)
	}
	if MaxBlocksRequest < fetcher.HashLimit {
		t.Fatalf("MaxBlocksRequest %d is below the fetcher announce limit of %d", MaxBlocksRequest, fetcher.HashLimit)
	}
}

func TestHandleGetBlocks_ServesKnownBlocks(t *testing.T) {
	known, hashes := knownBlocks(3)
	chain := &lookupCountingChain{known: known}

	result, err := serveGetBlocks(t, chain, hashes)
	if err != nil {
		t.Fatalf("handleMsg: %v", err)
	}
	if !result.answered {
		t.Fatal("no BlocksMsg reply")
	}
	if len(result.blocks) != 3 {
		t.Fatalf("reply carries %d blocks, want 3", len(result.blocks))
	}
	for i, block := range result.blocks {
		if block.Momentum.Hash != hashes[i] {
			t.Fatalf("block %d has hash %v, want %v", i, block.Momentum.Hash, hashes[i])
		}
	}
	if chain.lookups != 3 {
		t.Fatalf("%d lookups, want 3", chain.lookups)
	}
}

// A full-size request whose hashes are all unknown is legitimate (a peer may
// simply be ahead of us) and must be answered with an empty reply after
// exactly one lookup per hash.
func TestHandleGetBlocks_FullRequestOfUnknownHashesIsAnswered(t *testing.T) {
	chain := &lookupCountingChain{}

	result, err := serveGetBlocks(t, chain, unknownHashes(MaxBlocksRequest))
	if err != nil {
		t.Fatalf("handleMsg: %v", err)
	}
	if !result.answered {
		t.Fatal("no BlocksMsg reply")
	}
	if len(result.blocks) != 0 {
		t.Fatalf("reply carries %d blocks, want 0", len(result.blocks))
	}
	if chain.lookups != MaxBlocksRequest {
		t.Fatalf("%d lookups, want %d", chain.lookups, MaxBlocksRequest)
	}
}

// One hash past the limit is a protocol violation: the handler must return an
// error before looking that hash up, and must not answer.
func TestHandleGetBlocks_RejectsOversizedRequestBeforeExtraLookup(t *testing.T) {
	for _, count := range []int{MaxBlocksRequest + 1, 8 * MaxBlocksRequest} {
		chain := &lookupCountingChain{}

		result, err := serveGetBlocks(t, chain, unknownHashes(count))
		if err == nil {
			t.Fatalf("%d hashes: handleMsg accepted the request", count)
		}
		if !strings.Contains(err.Error(), errCode(ErrMsgTooLarge).String()) {
			t.Fatalf("%d hashes: error %q does not report %q", count, err, errCode(ErrMsgTooLarge).String())
		}
		if result.answered {
			t.Fatalf("%d hashes: a rejected request was answered", count)
		}
		if chain.lookups != MaxBlocksRequest {
			t.Fatalf("%d hashes: %d lookups, want exactly %d", count, chain.lookups, MaxBlocksRequest)
		}
	}
}

// Known and unknown hashes count the same: the limit is on the request, not
// on the hits.
func TestHandleGetBlocks_MixedRequestPastLimitIsRejected(t *testing.T) {
	known, knownHashes := knownBlocks(16)
	chain := &lookupCountingChain{known: known}

	hashes := make([]types.Hash, 0, MaxBlocksRequest+1)
	unknown := unknownHashes(MaxBlocksRequest + 1)
	for i := 0; i < MaxBlocksRequest+1; i++ {
		if i%16 == 0 && i/16 < len(knownHashes) {
			hashes = append(hashes, knownHashes[i/16])
		} else {
			hashes = append(hashes, unknown[i])
		}
	}

	result, err := serveGetBlocks(t, chain, hashes)
	if err == nil {
		t.Fatal("handleMsg accepted the request")
	}
	if result.answered {
		t.Fatal("a rejected request was answered")
	}
	if chain.lookups != MaxBlocksRequest {
		t.Fatalf("%d lookups, want exactly %d", chain.lookups, MaxBlocksRequest)
	}
}

// The reply-volume cap is unchanged: a request naming more known blocks than
// MaxBlockFetch is answered with MaxBlockFetch blocks and the remaining hashes
// are never looked up.
func TestHandleGetBlocks_ReplyStillCappedAtMaxBlockFetch(t *testing.T) {
	known, hashes := knownBlocks(MaxBlocksRequest + 4)
	chain := &lookupCountingChain{known: known}

	result, err := serveGetBlocks(t, chain, hashes)
	if err != nil {
		t.Fatalf("handleMsg: %v", err)
	}
	if !result.answered {
		t.Fatal("no BlocksMsg reply")
	}
	if len(result.blocks) != downloader.MaxBlockFetch {
		t.Fatalf("reply carries %d blocks, want %d", len(result.blocks), downloader.MaxBlockFetch)
	}
	if chain.lookups != downloader.MaxBlockFetch {
		t.Fatalf("%d lookups, want %d", chain.lookups, downloader.MaxBlockFetch)
	}
}

func TestHandleGetBlocks_EmptyRequestIsAnswered(t *testing.T) {
	chain := &lookupCountingChain{}

	result, err := serveGetBlocks(t, chain, []types.Hash{})
	if err != nil {
		t.Fatalf("handleMsg: %v", err)
	}
	if !result.answered {
		t.Fatal("no BlocksMsg reply")
	}
	if len(result.blocks) != 0 || chain.lookups != 0 {
		t.Fatalf("reply carries %d blocks after %d lookups, want 0 and 0", len(result.blocks), chain.lookups)
	}
}

// A peer on an earlier release does not split its requests. Its oversized
// request is still answered when the reply cap of MaxBlockFetch found blocks
// is reached within the first MaxBlocksRequest hashes, since that ends the
// loop before the request bound is checked; otherwise the request is dropped.
func TestHandleGetBlocks_UnsplitLegacyRequest(t *testing.T) {
	const legacyBatch = MaxBlocksRequest + 44

	t.Run("enough hits before the bound is answered", func(t *testing.T) {
		known, knownHashes := knownBlocks(downloader.MaxBlockFetch)
		chain := &lookupCountingChain{known: known}
		hashes := append(knownHashes, unknownHashes(legacyBatch-len(knownHashes))...)

		result, err := serveGetBlocks(t, chain, hashes)
		if err != nil {
			t.Fatalf("handleMsg: %v", err)
		}
		if !result.answered || len(result.blocks) != downloader.MaxBlockFetch {
			t.Fatalf("answered=%v with %d blocks, want %d blocks", result.answered, len(result.blocks), downloader.MaxBlockFetch)
		}
		if chain.lookups != downloader.MaxBlockFetch {
			t.Fatalf("%d lookups, want %d", chain.lookups, downloader.MaxBlockFetch)
		}
	})

	t.Run("last allowed hit completes the reply", func(t *testing.T) {
		// The MaxBlockFetch-th hit is the MaxBlocksRequest-th hash: the reply
		// cap ends the loop on the same iteration the request bound would
		// trip on the next one.
		known, knownHashes := knownBlocks(downloader.MaxBlockFetch)
		chain := &lookupCountingChain{known: known}
		hashes := append(unknownHashes(MaxBlocksRequest-downloader.MaxBlockFetch), knownHashes...)
		hashes = append(hashes, unknownHashes(legacyBatch-len(hashes))...)

		result, err := serveGetBlocks(t, chain, hashes)
		if err != nil {
			t.Fatalf("handleMsg: %v", err)
		}
		if !result.answered || len(result.blocks) != downloader.MaxBlockFetch {
			t.Fatalf("answered=%v with %d blocks, want %d blocks", result.answered, len(result.blocks), downloader.MaxBlockFetch)
		}
		if chain.lookups != MaxBlocksRequest {
			t.Fatalf("%d lookups, want exactly %d", chain.lookups, MaxBlocksRequest)
		}
	})

	t.Run("too few hits before the bound is dropped", func(t *testing.T) {
		known, knownHashes := knownBlocks(downloader.MaxBlockFetch - 1)
		chain := &lookupCountingChain{known: known}
		hashes := append(knownHashes, unknownHashes(legacyBatch-len(knownHashes))...)

		result, err := serveGetBlocks(t, chain, hashes)
		if err == nil {
			t.Fatal("handleMsg accepted the request")
		}
		if !strings.Contains(err.Error(), errCode(ErrMsgTooLarge).String()) {
			t.Fatalf("error %q does not report %q", err, errCode(ErrMsgTooLarge).String())
		}
		if result.answered {
			t.Fatal("a rejected request was answered")
		}
		if chain.lookups != MaxBlocksRequest {
			t.Fatalf("%d lookups, want exactly %d", chain.lookups, MaxBlocksRequest)
		}
	})
}

// A payload that is not a list of 32-byte hashes is a decode error: the
// handler stops at the malformed element, looks nothing further up, and does
// not answer.
func TestHandleGetBlocks_MalformedPayloadIsRejected(t *testing.T) {
	valid := unknownHashes(1)[0]
	cases := []struct {
		name          string
		payload       interface{}
		lookupsBefore int  // hashes that decode before the malformed element
		decodeErr     bool // reported as ErrDecode (a bad outer list is a raw rlp error)
	}{
		{"not a list", "not-a-list", 0, false},
		{"short hash", [][]byte{valid[:], valid[:31]}, 1, true},
		{"long hash", [][]byte{valid[:], append(valid[:], 0)}, 1, true},
		{"nested list", []interface{}{valid[:], []interface{}{valid[:]}}, 1, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			chain := &lookupCountingChain{}

			result, err := serveGetBlocksPayload(t, chain, tc.payload)
			if err == nil {
				t.Fatal("handleMsg accepted the payload")
			}
			if tc.decodeErr && !strings.Contains(err.Error(), errCode(ErrDecode).String()) {
				t.Fatalf("error %q does not report %q", err, errCode(ErrDecode).String())
			}
			if result.answered {
				t.Fatal("a rejected request was answered")
			}
			if chain.lookups != tc.lookupsBefore {
				t.Fatalf("%d lookups, want %d", chain.lookups, tc.lookupsBefore)
			}
		})
	}
}

// readGetBlocksRequests reads GetBlocksMsg frames from rw until the pipe
// closes and returns the hash list carried by each.
func readGetBlocksRequests(t *testing.T, rw p2p.MsgReadWriter) <-chan [][]types.Hash {
	t.Helper()
	out := make(chan [][]types.Hash, 1)
	go func() {
		var requests [][]types.Hash
		for {
			msg, err := rw.ReadMsg()
			if err != nil {
				out <- requests
				return
			}
			if msg.Code != GetBlocksMsg {
				t.Errorf("request code %d, want GetBlocksMsg (%d)", msg.Code, GetBlocksMsg)
			}
			var hashes []types.Hash
			if err := rlp.NewStream(msg.Payload, uint64(msg.Size)).Decode(&hashes); err != nil {
				t.Errorf("decode request: %v", err)
			}
			requests = append(requests, hashes)
		}
	}()
	return out
}

// RequestBlocks is the only place this node encodes a GetBlocksMsg. Whatever
// its callers hand it, no single request on the wire may name more than
// MaxBlocksRequest hashes, or the remote side drops us.
func TestRequestBlocks_SplitsBatchesLargerThanMaxBlocksRequest(t *testing.T) {
	for _, count := range []int{0, 1, MaxBlocksRequest, MaxBlocksRequest + 1, 2 * MaxBlocksRequest, 2*MaxBlocksRequest + 5} {
		app, net := p2p.MsgPipe()
		p := &peer{rw: net, id: "test-peer"}
		hashes := unknownHashes(count)
		requests := readGetBlocksRequests(t, app)

		if err := p.RequestBlocks(hashes); err != nil {
			t.Fatalf("%d hashes: RequestBlocks: %v", count, err)
		}
		_ = app.Close()

		frames := <-requests
		// Every frame is full except the last, so the frame count is fixed by
		// the batch size. An empty batch is one empty frame, as before.
		wantFrames := (count + MaxBlocksRequest - 1) / MaxBlocksRequest
		if count == 0 {
			wantFrames = 1
		}
		if len(frames) != wantFrames {
			t.Fatalf("%d hashes: %d frames on the wire, want %d", count, len(frames), wantFrames)
		}
		var sent []types.Hash
		for i, request := range frames {
			if len(request) > MaxBlocksRequest {
				t.Fatalf("%d hashes: request %d names %d hashes, want at most %d", count, i, len(request), MaxBlocksRequest)
			}
			if i < len(frames)-1 && len(request) != MaxBlocksRequest {
				t.Fatalf("%d hashes: request %d names %d hashes, want a full %d", count, i, len(request), MaxBlocksRequest)
			}
			sent = append(sent, request...)
		}
		if len(sent) != len(hashes) {
			t.Fatalf("%d hashes: %d hashes reached the wire", count, len(sent))
		}
		for i := range hashes {
			if sent[i] != hashes[i] {
				t.Fatalf("%d hashes: hash %d differs or is out of order", count, i)
			}
		}
	}
}

// failAfterWriter is a MsgReadWriter whose WriteMsg succeeds a fixed number of
// times and then fails. It counts every attempt, successful or not.
type failAfterWriter struct {
	succeed  int
	attempts int
	err      error
}

func (w *failAfterWriter) ReadMsg() (p2p.Msg, error) { panic("not used by RequestBlocks") }

func (w *failAfterWriter) WriteMsg(msg p2p.Msg) error {
	w.attempts++
	if w.attempts > w.succeed {
		return w.err
	}
	return nil
}

// A write failure on a later chunk is returned to the caller and no further
// chunks are attempted, so the caller sees the same error it would have seen
// from an unsplit send.
func TestRequestBlocks_StopsAtFirstFailedChunk(t *testing.T) {
	wantErr := errors.New("connection reset")
	rw := &failAfterWriter{succeed: 1, err: wantErr}
	p := &peer{rw: rw, id: "test-peer"}

	err := p.RequestBlocks(unknownHashes(3*MaxBlocksRequest + 1))
	if !errors.Is(err, wantErr) {
		t.Fatalf("RequestBlocks returned %v, want %v", err, wantErr)
	}
	// One successful chunk, one failed chunk, and nothing after the failure.
	if rw.attempts != 2 {
		t.Fatalf("%d write attempts, want 2 (one success, one failure)", rw.attempts)
	}
}
