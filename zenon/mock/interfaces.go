package mock

import (
	"github.com/zenon-network/go-zenon/chain/nom"
	"github.com/zenon-network/go-zenon/common"
	"github.com/zenon-network/go-zenon/common/types"
	"github.com/zenon-network/go-zenon/zenon"
)

// MockZenon is the test-harness facade. Extends [zenon.Zenon] with
// time-stepping (Insert* momentum helpers), block-insertion helpers
// with expected-error / expected-VM-changes assertions, contract
// call helpers, and per-test cleanup (StopPanic).
type MockZenon interface {
	zenon.Zenon
	StopPanic()

	InsertNewMomentum()
	InsertMomentumsTo(targetHeight uint64)

	CallContract(template *nom.AccountBlock) *common.Expecter
	InsertSendBlock(template *nom.AccountBlock, expectedError error, expectedVmChanges string) *nom.AccountBlock
	InsertReceiveBlock(fromHeader types.AccountHeader, template *nom.AccountBlock, expectedError error, expectedVmChanges string) *nom.AccountBlock

	SaveLogs(logger common.Logger) *common.Expecter
	ExpectBalance(address types.Address, standard types.ZenonTokenStandard, expected int64)
}
