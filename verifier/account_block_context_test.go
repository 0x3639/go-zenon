package verifier_test

import (
	"testing"

	"github.com/zenon-network/go-zenon/chain/nom"
	"github.com/zenon-network/go-zenon/common"
	"github.com/zenon-network/go-zenon/common/types"
	"github.com/zenon-network/go-zenon/verifier"
	"github.com/zenon-network/go-zenon/zenon/mock"
)

// A block above height 1 for an address that has no account chain at all
// must be rejected the same way as a block whose previous block is missing.
// Before the guard in getContext the frontier lookup for such an address
// returned nil and the verifier dereferenced it.
func TestAccountBlockVerifier_NoAccountChain_ReturnsPreviousMissing(t *testing.T) {
	z := mock.NewMockZenon(t)
	defer z.StopPanic()
	z.InsertMomentumsTo(5)

	address := types.PubKeyToAddress([]byte("address-without-an-account-chain"))
	frontier, err := z.Chain().GetFrontierAccountStore(address).Frontier()
	common.FailIfErr(t, err)
	if frontier != nil {
		t.Fatalf("test address %v unexpectedly has an account chain", address)
	}
	momentum, err := z.Chain().GetFrontierMomentumStore().GetFrontierMomentum()
	common.FailIfErr(t, err)

	v := verifier.NewAccountBlockVerifier(z.Chain(), z.Consensus())
	for _, height := range []uint64{2, 3, 1000} {
		block := &nom.AccountBlock{
			Version:              1,
			BlockType:            nom.BlockTypeUserSend,
			Address:              address,
			ToAddress:            address,
			Height:               height,
			PreviousHash:         types.NewHash([]byte("previous")),
			MomentumAcknowledged: momentum.Identifier(),
			Amount:               common.Big0,
			TokenStandard:        types.ZnnTokenStandard,
		}
		if err := v.AccountBlock(block); err != verifier.ErrABPreviousMissing {
			t.Errorf("height %d: got %v, want %v", height, err, verifier.ErrABPreviousMissing)
		}
	}
}
