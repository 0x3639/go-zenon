package implementation

import (
	"encoding/base64"
	"math/big"
	"testing"

	"github.com/zenon-network/go-zenon/common/types"
	"github.com/zenon-network/go-zenon/vm/constants"
	"github.com/zenon-network/go-zenon/vm/embedded/definition"
)

func governanceActionForTest(destination types.Address, data []byte) *definition.ActionVariable {
	return &definition.ActionVariable{
		Name:        "governance action",
		Description: "governance action description",
		Url:         "https://zenon.network",
		Destination: destination,
		Data:        base64.StdEncoding.EncodeToString(data),
	}
}

func ensureSporkAddressForTest(t *testing.T) {
	t.Helper()
	if types.SporkAddress != nil {
		return
	}

	sporkAddress := types.SporkContract
	types.SporkAddress = &sporkAddress
	t.Cleanup(func() {
		types.SporkAddress = nil
	})
}

func TestCheckActionStaticAllowsGovernanceMethods(t *testing.T) {
	ensureSporkAddressForTest(t)

	tests := []struct {
		name        string
		destination types.Address
		data        []byte
	}{
		{
			name:        "spork create",
			destination: types.SporkContract,
			data: definition.ABISpork.PackMethodPanic(
				definition.SporkCreateMethodName,
				"spork-governance",
				"activate governance",
			),
		},
		{
			name:        "bridge unhalt",
			destination: types.BridgeContract,
			data:        definition.ABIBridge.PackMethodPanic(definition.UnhaltMethodName),
		},
		{
			name:        "liquidity fund",
			destination: types.LiquidityContract,
			data: definition.ABILiquidity.PackMethodPanic(
				definition.FundMethodName,
				big.NewInt(1),
				big.NewInt(2),
			),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := checkActionStatic(governanceActionForTest(tt.destination, tt.data)); err != nil {
				t.Fatalf("expected action to be allowed, got %v", err)
			}
		})
	}
}

func TestCheckActionStaticRejectsUnsupportedDestination(t *testing.T) {
	ensureSporkAddressForTest(t)

	action := governanceActionForTest(
		types.AcceleratorContract,
		definition.ABICommon.PackMethodPanic(definition.DonateMethodName),
	)

	if err := checkActionStatic(action); err != constants.ErrPermissionDenied {
		t.Fatalf("expected ErrPermissionDenied, got %v", err)
	}
}

func TestCheckActionStaticAllowsModernTLDs(t *testing.T) {
	ensureSporkAddressForTest(t)

	action := governanceActionForTest(
		types.SporkContract,
		definition.ABISpork.PackMethodPanic(
			definition.SporkCreateMethodName,
			"spork-governance",
			"activate governance",
		),
	)
	action.Url = "https://zenon.network"

	if err := checkActionStatic(action); err != nil {
		t.Fatalf("expected modern TLD to be allowed, got %v", err)
	}
}

func TestCheckActionStaticRejectsUnsupportedMethod(t *testing.T) {
	ensureSporkAddressForTest(t)

	action := governanceActionForTest(
		types.BridgeContract,
		definition.ABIBridge.PackMethodPanic(definition.WrapTokenMethodName, uint32(1), uint32(1), "address"),
	)

	if err := checkActionStatic(action); err != constants.ErrPermissionDenied {
		t.Fatalf("expected ErrPermissionDenied, got %v", err)
	}
}

func TestCheckActionStaticRejectsMalformedAllowedMethodPayload(t *testing.T) {
	ensureSporkAddressForTest(t)

	data := append(definition.ABIBridge.PackMethodPanic(definition.UnhaltMethodName), 0)
	action := governanceActionForTest(types.BridgeContract, data)

	if err := checkActionStatic(action); err != constants.ErrUnpackError {
		t.Fatalf("expected ErrUnpackError, got %v", err)
	}
}

func TestCheckActionStaticRejectsMalformedActionData(t *testing.T) {
	ensureSporkAddressForTest(t)

	action := governanceActionForTest(types.SporkContract, []byte{1, 2, 3})

	if err := checkActionStatic(action); err != constants.ErrForbiddenParam {
		t.Fatalf("expected ErrForbiddenParam, got %v", err)
	}
}

func TestCheckActionStaticRejectsOversizedActionData(t *testing.T) {
	ensureSporkAddressForTest(t)

	action := governanceActionForTest(types.SporkContract, make([]byte, constants.GovernanceActionDataMaxLength+1))

	if err := checkActionStatic(action); err != constants.ErrForbiddenParam {
		t.Fatalf("expected ErrForbiddenParam, got %v", err)
	}
}
