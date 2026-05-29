package embedded

import (
	"testing"

	"github.com/zenon-network/go-zenon/rpc/api"
)

func TestGovernanceGetAllActionsRejectsLargePage(t *testing.T) {
	_, err := (&GovernanceApi{}).GetAllActions(0, api.RpcMaxPageSize+1)
	if err != api.ErrPageSizeParamTooBig {
		t.Fatalf("expected ErrPageSizeParamTooBig, got %v", err)
	}
}
