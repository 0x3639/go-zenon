package nom

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"google.golang.org/protobuf/encoding/protowire"

	"github.com/zenon-network/go-zenon/common"
)

// nestedDescendantsJSON is a block with a single chain of descendants
// depth levels deep, every level a valid minimal block.
func nestedDescendantsJSON(depth int) string {
	var b strings.Builder
	for i := 0; i < depth; i++ {
		b.WriteString(`{"height":1,"nonce":"0000000000000000","descendantBlocks":[`)
	}
	b.WriteString(`{"height":1,"nonce":"0000000000000000"}`)
	for i := 0; i < depth; i++ {
		b.WriteString(`]}`)
	}
	return b.String()
}

// largestAmountDigits is the decimal length of 2^255-1, the largest
// amount the verifier admits (and constants.TokenMaxSupplyBig).
var largestAmountDigits = len(common.BigP255m1.String())

// The amount string is parsed by big.Int.SetString, whose cost grows
// faster than the string: a string longer than the largest valid
// amount is refused before parsing, at every level of the descendant
// tree, and the longest valid one still decodes.
func TestAccountBlock_UnmarshalJSON_AmountLengthIsBounded(t *testing.T) {
	longest := strings.Repeat("9", largestAmountDigits)
	block, err := decodeBlockJSON(t, `{"height":1,"nonce":"0000000000000000","amount":"`+longest+`"}`)
	common.FailIfErr(t, err)
	if block.Amount.String() != longest {
		t.Fatalf("longest valid amount decoded to %s", block.Amount)
	}

	tooLong := longest + "9"
	_, err = decodeBlockJSON(t, `{"height":1,"nonce":"0000000000000000","amount":"`+tooLong+`"}`)
	if err == nil || !strings.Contains(err.Error(), "amount") {
		t.Fatalf("%d-character amount: err = %v, want an amount error", len(tooLong), err)
	}

	_, err = decodeBlockJSON(t, `{"height":1,"nonce":"0000000000000000","descendantBlocks":[{"height":2,"nonce":"0000000000000000"},{"height":3,"nonce":"0000000000000000","amount":"`+tooLong+`"}]}`)
	if err == nil || !strings.Contains(err.Error(), "index 1") || !strings.Contains(err.Error(), "amount") {
		t.Fatalf("oversized amount in a descendant: err = %v, want an amount error naming index 1", err)
	}

	// Rejection costs a scan of the input, not a parse: 1 MiB of digits
	// took about half a second to parse before the bound existed.
	digits := strings.Repeat("7", 1<<20)
	start := time.Now()
	_, err = decodeBlockJSON(t, `{"height":1,"nonce":"0000000000000000","amount":"`+digits+`"}`)
	elapsed := time.Since(start)
	if err == nil {
		t.Fatal("1 MiB amount decoded without error")
	}
	if elapsed > 200*time.Millisecond {
		t.Fatalf("rejecting a 1 MiB amount took %v, want well under 200ms", elapsed)
	}
}

// A descendant tree is decoded in one pass: before, every level handed
// its raw suffix to a nested decoder, so a chain of n descendants cost
// n parses of a shrinking tail. Eight chains of 4000 took seven seconds
// on the unbounded decoder; the bound is generous so the race detector
// does not trip it. Input nested past what the decoder admits is an
// error, not a crash.
func TestAccountBlock_UnmarshalJSON_DescendantDecodeIsLinear(t *testing.T) {
	chain := nestedDescendantsJSON(4000)
	wide := `{"height":1,"nonce":"0000000000000000","descendantBlocks":[` + strings.Repeat(chain+",", 7) + chain + `]}`
	start := time.Now()
	block, err := decodeBlockJSON(t, wide)
	elapsed := time.Since(start)
	common.FailIfErr(t, err)
	if elapsed > 1500*time.Millisecond {
		t.Fatalf("decoding %d bytes of nested descendants took %v, want well under 1.5s", len(wide), elapsed)
	}
	if len(block.DescendantBlocks) != 8 {
		t.Fatalf("decoded %d top-level descendants, want 8", len(block.DescendantBlocks))
	}
	cur := block.DescendantBlocks[7]
	for level := 1; level < 4000; level++ {
		if len(cur.DescendantBlocks) != 1 {
			t.Fatalf("level %d has %d descendants, want 1", level, len(cur.DescendantBlocks))
		}
		cur = cur.DescendantBlocks[0]
	}
	if len(cur.DescendantBlocks) != 1 || len(cur.DescendantBlocks[0].DescendantBlocks) != 0 {
		t.Fatalf("chain does not end in a leaf at level 4000")
	}

	deeper := nestedDescendantsJSON(6000)
	if _, err := decodeBlockJSON(t, deeper); err == nil {
		t.Fatal("6000 nested descendants decoded without error")
	}
}

// A descendant with an invalid or missing nonce was refused by the
// nested decoder; the one-pass decoder keeps refusing it, naming the
// index, while the root keeps its own nonce error.
func TestAccountBlock_UnmarshalJSON_DescendantNonceIsStillChecked(t *testing.T) {
	_, err := decodeBlockJSON(t, `{"height":1,"nonce":"0000000000000000","descendantBlocks":[{"height":2,"nonce":"zz"}]}`)
	if err == nil || !strings.Contains(err.Error(), "nonce") || !strings.Contains(err.Error(), "index 0") {
		t.Fatalf("invalid descendant nonce: err = %v, want a nonce error naming index 0", err)
	}
	_, err = decodeBlockJSON(t, `{"height":1,"nonce":"0000000000000000","descendantBlocks":[{"height":2,"nonce":"0000000000000000"},{"height":3}]}`)
	if err == nil || !strings.Contains(err.Error(), "nonce") || !strings.Contains(err.Error(), "index 1") {
		t.Fatalf("missing descendant nonce: err = %v, want a nonce error naming index 1", err)
	}
	_, err = decodeBlockJSON(t, `{"height":1,"nonce":"zz"}`)
	if err == nil || !strings.Contains(err.Error(), "nonce") {
		t.Fatalf("invalid root nonce: err = %v, want a nonce error", err)
	}
}

// The one-pass wire form round-trips a nested tree byte for byte.
func TestAccountBlock_MarshalJSON_NestedDescendantsRoundTrip(t *testing.T) {
	text := nestedDescendantsJSON(3)
	block, err := decodeBlockJSON(t, text)
	common.FailIfErr(t, err)
	encoded, err := json.Marshal(block)
	common.FailIfErr(t, err)
	again, err := decodeBlockJSON(t, string(encoded))
	common.FailIfErr(t, err)
	reencoded, err := json.Marshal(again)
	common.FailIfErr(t, err)
	if string(encoded) != string(reencoded) {
		t.Fatalf("round trip differs:\n%s\n%s", encoded, reencoded)
	}
	if again.DescendantBlocks[0].DescendantBlocks[0].DescendantBlocks[0].Height != 1 {
		t.Fatal("nested descendants lost in the round trip")
	}
}

// marshalChain builds, in memory, a wire-form block with depth levels of
// single descendants below it.
func marshalChain(depth int) *AccountBlockMarshal {
	leaf := &AccountBlockMarshal{Nonce: "0000000000000000"}
	cur := leaf
	for i := 0; i < depth; i++ {
		cur = &AccountBlockMarshal{Nonce: "0000000000000000", DescendantBlocks: []*AccountBlockMarshal{cur}}
	}
	return cur
}

// The descendant depth bound is the protobuf decoder's recursion limit,
// so the two wire forms admit the same trees. encoding/json refuses
// deeper input on its own before Check sees it, so the bound is pinned
// on the wire form directly: a chain at the bound passes, one level more
// is refused with the depth named.
func TestAccountBlockMarshal_Check_DescendantDepthIsBounded(t *testing.T) {
	if err := marshalChain(MaxDescendantDepth).Check(); err != nil {
		t.Fatalf("%d levels of descendants rejected: %v", MaxDescendantDepth, err)
	}
	err := marshalChain(MaxDescendantDepth + 1).Check()
	if err == nil {
		t.Fatalf("%d levels of descendants accepted", MaxDescendantDepth+1)
	}
	if !strings.Contains(err.Error(), "nested deeper than") {
		t.Fatalf("error is not the depth bound: %v", err)
	}

	// The bound is the limit the protobuf decoder applies to the wire
	// form, and the digit bound is the length of the largest valid
	// amount; neither is a literal of its own.
	if MaxDescendantDepth != protowire.DefaultRecursionLimit {
		t.Fatalf("MaxDescendantDepth = %d, want protowire.DefaultRecursionLimit (%d)", MaxDescendantDepth, protowire.DefaultRecursionLimit)
	}
	if MaxAmountDigits != len(common.BigP255m1.String()) || MaxAmountDigits != 77 {
		t.Fatalf("MaxAmountDigits = %d, want the %d digits of 2^255-1", MaxAmountDigits, len(common.BigP255m1.String()))
	}
}
