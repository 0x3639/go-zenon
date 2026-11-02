package api

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/zenon-network/go-zenon/common"
)

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

var largestAmountDigits = len(common.BigP255m1.String())

// ledger.publishRawTransaction decodes its block parameter through this
// type before any verification. Amount and token supply strings longer
// than the largest valid amount are refused before they are parsed, at
// the root, in a descendant, in the paired chain and in a list element;
// the longest valid strings decode.
func TestAccountBlock_UnmarshalJSON_AmountAndSupplyLengthAreBounded(t *testing.T) {
	longest := strings.Repeat("9", largestAmountDigits)
	tooLong := longest + "9"

	var block AccountBlock
	valid := `{"height":1,"nonce":"0000000000000000","amount":"` + longest + `","token":{"totalSupply":"` + longest + `","maxSupply":"` + longest + `"},"pairedAccountBlock":{"height":2,"nonce":"0000000000000000","amount":"` + longest + `","token":{"totalSupply":"` + longest + `","maxSupply":"` + longest + `"}}}`
	if err := json.Unmarshal([]byte(valid), &block); err != nil {
		t.Fatal(err)
	}
	if block.Amount.String() != longest || block.TokenInfo.TotalSupply.String() != longest || block.TokenInfo.MaxSupply.String() != longest {
		t.Fatalf("longest valid strings did not decode: %+v %+v", block.Amount, block.TokenInfo)
	}
	if block.PairedAccountBlock.Amount.String() != longest || block.PairedAccountBlock.TokenInfo.MaxSupply.String() != longest {
		t.Fatalf("longest valid strings in the paired block did not decode: %+v", block.PairedAccountBlock)
	}

	cases := []struct {
		name string
		text string
		want []string
	}{
		{"root amount", `{"height":1,"nonce":"0000000000000000","amount":"` + tooLong + `"}`, []string{"amount"}},
		{"root total supply", `{"height":1,"nonce":"0000000000000000","token":{"totalSupply":"` + tooLong + `"}}`, []string{"totalSupply"}},
		{"root max supply", `{"height":1,"nonce":"0000000000000000","token":{"maxSupply":"` + tooLong + `"}}`, []string{"maxSupply"}},
		{"descendant amount", `{"height":1,"nonce":"0000000000000000","descendantBlocks":[{"height":2,"nonce":"0000000000000000","amount":"` + tooLong + `"}]}`, []string{"amount", "index 0"}},
		{"paired amount", `{"height":1,"nonce":"0000000000000000","pairedAccountBlock":{"height":2,"nonce":"0000000000000000","amount":"` + tooLong + `"}}`, []string{"amount", "depth 1"}},
		{"paired supply", `{"height":1,"nonce":"0000000000000000","pairedAccountBlock":{"height":2,"nonce":"0000000000000000","token":{"maxSupply":"` + tooLong + `"}}}`, []string{"maxSupply", "depth 1"}},
		{"paired descendant amount", `{"height":1,"nonce":"0000000000000000","pairedAccountBlock":{"height":2,"nonce":"0000000000000000","descendantBlocks":[{"height":3,"nonce":"0000000000000000","amount":"` + tooLong + `"}]}}`, []string{"amount", "depth 1", "index 0"}},
	}
	for _, c := range cases {
		var b AccountBlock
		err := json.Unmarshal([]byte(c.text), &b)
		if err == nil {
			t.Errorf("%s: %d-character string decoded without error", c.name, len(tooLong))
			continue
		}
		for _, want := range c.want {
			if !strings.Contains(err.Error(), want) {
				t.Errorf("%s: err = %v, want it to mention %q", c.name, err, want)
			}
		}
	}

	var list AccountBlockList
	err := json.Unmarshal([]byte(`{"list":[null,{"height":1,"nonce":"0000000000000000","token":{"totalSupply":"`+tooLong+`"}}],"count":1,"more":false}`), &list)
	if err == nil || !strings.Contains(err.Error(), "list element 1") || !strings.Contains(err.Error(), "totalSupply") {
		t.Fatalf("oversized supply in a list element: err = %v", err)
	}

	// Rejection costs a scan of the input, not a parse: 1 MiB of digits
	// took about half a second to parse before the bound existed.
	digits := strings.Repeat("7", 1<<20)
	for _, text := range []string{
		`{"height":1,"nonce":"0000000000000000","amount":"` + digits + `"}`,
		`{"height":1,"nonce":"0000000000000000","token":{"totalSupply":"` + digits + `"}}`,
	} {
		var b AccountBlock
		start := time.Now()
		err := json.Unmarshal([]byte(text), &b)
		elapsed := time.Since(start)
		if err == nil {
			t.Fatal("1 MiB digit string decoded without error")
		}
		if elapsed > 200*time.Millisecond {
			t.Fatalf("rejecting a 1 MiB digit string took %v, want well under 200ms", elapsed)
		}
	}
}

// The descendant tree of a submitted block is decoded once, not once per
// level; the decoded block still copies into a ledger block.
func TestAccountBlock_UnmarshalJSON_DescendantDecodeIsLinear(t *testing.T) {
	chain := nestedDescendantsJSON(4000)
	wide := `{"height":1,"nonce":"0000000000000000","descendantBlocks":[` + strings.Repeat(chain+",", 7) + chain + `]}`
	var block AccountBlock
	start := time.Now()
	err := json.Unmarshal([]byte(wide), &block)
	elapsed := time.Since(start)
	if err != nil {
		t.Fatal(err)
	}
	if elapsed > 1500*time.Millisecond {
		t.Fatalf("decoding %d bytes of nested descendants took %v, want well under 1.5s", len(wide), elapsed)
	}
	ledger, err := block.ToLedgerBlock()
	if err != nil {
		t.Fatal(err)
	}
	if len(ledger.DescendantBlocks) != 8 || len(ledger.DescendantBlocks[0].DescendantBlocks) != 1 {
		t.Fatalf("descendants lost in ToLedgerBlock: %d top-level", len(ledger.DescendantBlocks))
	}
	if err := json.Unmarshal([]byte(nestedDescendantsJSON(6000)), &block); err == nil {
		t.Fatal("6000 nested descendants decoded without error")
	}
}
