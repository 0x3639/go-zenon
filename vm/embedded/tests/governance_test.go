package tests

import (
	"encoding/base64"
	"github.com/zenon-network/go-zenon/common"
	"math/big"
	"testing"
	"time"

	g "github.com/zenon-network/go-zenon/chain/genesis/mock"
	"github.com/zenon-network/go-zenon/chain/nom"
	"github.com/zenon-network/go-zenon/common/types"
	"github.com/zenon-network/go-zenon/rpc/api/embedded"
	"github.com/zenon-network/go-zenon/vm/constants"
	"github.com/zenon-network/go-zenon/vm/embedded/definition"
	"github.com/zenon-network/go-zenon/zenon/mock"
)

func overrideGovernanceSporkForTest(t *testing.T, id types.Hash) {
	t.Helper()

	previousSporkId := types.GovernanceSpork.SporkId
	previousImplemented, hadPreviousImplemented := types.ImplementedSporksMap[id]

	types.GovernanceSpork.SporkId = id
	types.ImplementedSporksMap[id] = true

	t.Cleanup(func() {
		types.GovernanceSpork.SporkId = previousSporkId
		if hadPreviousImplemented {
			types.ImplementedSporksMap[id] = previousImplemented
		} else {
			delete(types.ImplementedSporksMap, id)
		}
	})
}

func activateGovernance(t *testing.T, z mock.MockZenon) {
	t.Helper()

	z.InsertSendBlock(&nom.AccountBlock{
		Address:   g.Spork.Address,
		ToAddress: types.SporkContract,
		Data: definition.ABISpork.PackMethodPanic(definition.SporkCreateMethodName,
			"spork-governance",              // name
			"activate spork for governance", // description
		),
	}, nil, mock.SkipVmChanges)
	z.InsertNewMomentum()

	sporkAPI := embedded.NewSporkApi(z)
	sporkList, err := sporkAPI.GetAll(0, 10)
	common.FailIfErr(t, err)
	id := findSpork(t, sporkList, "spork-governance").Id

	z.InsertSendBlock(&nom.AccountBlock{
		Address:   g.Spork.Address,
		ToAddress: types.SporkContract,
		Data: definition.ABISpork.PackMethodPanic(definition.SporkActivateMethodName,
			id, // id
		),
	}, nil, mock.SkipVmChanges)
	z.InsertNewMomentum()
	overrideGovernanceSporkForTest(t, id)
}

// Activate spork
func activateGovernanceStep0(t *testing.T, z mock.MockZenon) {
	activateGovernance(t, z)
	z.InsertMomentumsTo(10)

	governanceApi := embedded.NewGovernanceApi(z)
	actionsList, err := governanceApi.GetAllActions(0, 10)

	common.Json(actionsList, err).Equals(t, `
{
	"count": 0,
	"list": []
}`)

	// Register 4th pillar for voting
	defer z.CallContract(&nom.AccountBlock{
		Address:       g.Pillar4.Address,
		ToAddress:     types.PillarContract,
		Data:          definition.ABIPillars.PackMethodPanic(definition.DepositQsrMethodName),
		TokenStandard: types.QsrTokenStandard,
		Amount:        big.NewInt(150000 * g.Zexp),
	}).Error(t, nil)
	z.InsertNewMomentum()
	// register the first normal pillar
	defer z.CallContract(&nom.AccountBlock{
		Address:       g.Pillar4.Address,
		ToAddress:     types.PillarContract,
		Data:          definition.ABIPillars.PackMethodPanic(definition.RegisterMethodName, g.Pillar4Name, g.Pillar4.Address, g.Pillar4.Address, uint8(0), uint8(100)),
		TokenStandard: types.ZnnTokenStandard,
		Amount:        constants.PillarStakeAmount,
	}).Error(t, nil)
	z.InsertNewMomentum()

	// Register 5th pillar for voting
	defer z.CallContract(&nom.AccountBlock{
		Address:       g.Pillar5.Address,
		ToAddress:     types.PillarContract,
		Data:          definition.ABIPillars.PackMethodPanic(definition.DepositQsrMethodName),
		TokenStandard: types.QsrTokenStandard,
		Amount:        big.NewInt(160000 * g.Zexp),
	}).Error(t, nil)
	z.InsertNewMomentum()
	// register the first normal pillar
	defer z.CallContract(&nom.AccountBlock{
		Address:       g.Pillar5.Address,
		ToAddress:     types.PillarContract,
		Data:          definition.ABIPillars.PackMethodPanic(definition.RegisterMethodName, g.Pillar5Name, g.Pillar5.Address, g.Pillar5.Address, uint8(0), uint8(100)),
		TokenStandard: types.ZnnTokenStandard,
		Amount:        constants.PillarStakeAmount,
	}).Error(t, nil)
	insertMomentums(z, 2)

	// Register 6th pillar for voting
	defer z.CallContract(&nom.AccountBlock{
		Address:       g.Pillar6.Address,
		ToAddress:     types.PillarContract,
		Data:          definition.ABIPillars.PackMethodPanic(definition.DepositQsrMethodName),
		TokenStandard: types.QsrTokenStandard,
		Amount:        big.NewInt(170000 * g.Zexp),
	}).Error(t, nil)
	insertMomentums(z, 2)

	// register the first normal pillar
	defer z.CallContract(&nom.AccountBlock{
		Address:       g.Pillar6.Address,
		ToAddress:     types.PillarContract,
		Data:          definition.ABIPillars.PackMethodPanic(definition.RegisterMethodName, g.Pillar6Name, g.Pillar6.Address, g.Pillar6.Address, uint8(0), uint8(100)),
		TokenStandard: types.ZnnTokenStandard,
		Amount:        constants.PillarStakeAmount,
	}).Error(t, nil)
	z.InsertNewMomentum()
}

func assertGovernanceAction(t *testing.T, action *embedded.Action, name string, actionType, round, status uint8, executed bool, total, yes, no uint32) {
	t.Helper()

	if action.Name != name {
		t.Fatalf("expected action %q, got %q", name, action.Name)
	}
	if action.Type != actionType {
		t.Fatalf("expected action type %v, got %v", actionType, action.Type)
	}
	if action.Round != round {
		t.Fatalf("expected round %v, got %v", round, action.Round)
	}
	expectedVoteId := definition.ActionVoteId(action.Id, round)
	if action.CurrentVoteId != expectedVoteId {
		t.Fatalf("expected current vote id %v, got %v", expectedVoteId, action.CurrentVoteId)
	}
	if action.RoundStartTimestamp == 0 {
		t.Fatalf("expected round start timestamp to be set")
	}
	if action.Status != status {
		t.Fatalf("expected status %v, got %v", status, action.Status)
	}
	if action.Executed != executed {
		t.Fatalf("expected executed %v, got %v", executed, action.Executed)
	}
	schedule, err := constants.GovernanceActionSchedule(actionType, round)
	common.FailIfErr(t, err)
	if action.ActivePillarThreshold != schedule.ActivePillarThreshold {
		t.Fatalf("expected active pillar threshold %v, got %v", schedule.ActivePillarThreshold, action.ActivePillarThreshold)
	}
	if action.DirectionalThreshold != schedule.DirectionalThreshold {
		t.Fatalf("expected directional threshold %v, got %v", schedule.DirectionalThreshold, action.DirectionalThreshold)
	}
	if action.VotingPeriod != schedule.VotingPeriod {
		t.Fatalf("expected voting period %v, got %v", schedule.VotingPeriod, action.VotingPeriod)
	}
	if action.Votes.Id != action.CurrentVoteId {
		t.Fatalf("expected vote breakdown id %v, got %v", action.CurrentVoteId, action.Votes.Id)
	}
	if action.Votes.Total != total || action.Votes.Yes != yes || action.Votes.No != no {
		t.Fatalf("expected votes total=%v yes=%v no=%v, got total=%v yes=%v no=%v", total, yes, no, action.Votes.Total, action.Votes.Yes, action.Votes.No)
	}
}

func findSpork(t *testing.T, sporks *embedded.SporkList, name string) *definition.Spork {
	t.Helper()

	for _, spork := range sporks.List {
		if spork.Name == name {
			return spork
		}
	}

	t.Fatalf("expected spork %q to exist", name)
	return nil
}

func assertSpork(t *testing.T, sporks *embedded.SporkList, name, description string, activated bool, enforcementHeight uint64) {
	t.Helper()

	spork := findSpork(t, sporks, name)
	if spork.Description != description {
		t.Fatalf("expected spork %q description %q, got %q", name, description, spork.Description)
	}
	if spork.Activated != activated {
		t.Fatalf("expected spork %q activated=%v, got %v", name, activated, spork.Activated)
	}
	if spork.EnforcementHeight != enforcementHeight {
		t.Fatalf("expected spork %q enforcement height %v, got %v", name, enforcementHeight, spork.EnforcementHeight)
	}
}

func findGovernanceAction(t *testing.T, actions *embedded.ActionList, name string) *embedded.Action {
	t.Helper()

	for _, action := range actions.List {
		if action.Name == name {
			return action
		}
	}

	t.Fatalf("expected governance action %q to exist", name)
	return nil
}

func findTimeChallenge(t *testing.T, challenges *embedded.TimeChallengesList, methodName string) *definition.TimeChallengeInfo {
	t.Helper()

	for _, challenge := range challenges.List {
		if challenge.MethodName == methodName {
			return challenge
		}
	}

	t.Fatalf("expected time challenge %q to exist", methodName)
	return nil
}

func overrideType1GovernanceVotingPeriods(t *testing.T, periods []int64) {
	t.Helper()

	previous := append([]int64(nil), constants.Type1ActionVotingPeriods...)
	constants.Type1ActionVotingPeriods = append([]int64(nil), periods...)

	t.Cleanup(func() {
		constants.Type1ActionVotingPeriods = previous
	})
}

func callContractAndInsert(t *testing.T, z mock.MockZenon, block *nom.AccountBlock, expected error) {
	t.Helper()

	expect := z.CallContract(block)
	insertMomentums(z, 2)
	expect.Error(t, expected)
}

func proposeVoteAndExecuteGovernanceAction(t *testing.T, z mock.MockZenon, name, description string, destination types.Address, data []byte) *embedded.Action {
	t.Helper()

	dataString := base64.StdEncoding.EncodeToString(data)
	callContractAndInsert(t, z, proposeAction(g.User1.Address, name, description, "https://qwerty.com", destination, dataString), nil)

	governanceApi := embedded.NewGovernanceApi(z)
	actions, err := governanceApi.GetAllActions(0, 100)
	common.FailIfErr(t, err)
	action := findGovernanceAction(t, actions, name)

	voters := []struct {
		address types.Address
		name    string
	}{
		{g.Pillar1.Address, g.Pillar1Name},
		{g.Pillar2.Address, g.Pillar2Name},
		{g.Pillar3.Address, g.Pillar3Name},
		{g.Pillar4.Address, g.Pillar4Name},
	}
	for _, voter := range voters {
		callContractAndInsert(t, z, voteByName(voter.address, voter.name, action.CurrentVoteId, definition.VoteYes), nil)
	}

	callContractAndInsert(t, z, executeAction(g.User1.Address, action.Id), nil)
	action, err = governanceApi.GetActionById(action.Id)
	common.FailIfErr(t, err)
	return action
}

func assertBridgeAdministrator(t *testing.T, z mock.MockZenon, expected types.Address) {
	t.Helper()

	bridgeAPI := embedded.NewBridgeApi(z)
	bridgeInfo, err := bridgeAPI.GetBridgeInfo()
	common.FailIfErr(t, err)
	if bridgeInfo.Administrator != expected {
		t.Fatalf("expected bridge administrator %v, got %v", expected, bridgeInfo.Administrator)
	}
}

// Activate spork
// Propose action to create a spork
func activateGovernanceStep1(t *testing.T, z mock.MockZenon) {
	activateGovernanceStep0(t, z)
	insertMomentums(z, 10)

	name := "create btc-bridge spork"
	description := "this spork will implement bitcoin bridge logic"
	url := "https://qwerty.com"

	sporkName := "btc-bridge"
	sporkDescription := "btc-bridge logic"
	data, err := definition.ABISpork.PackMethod(definition.SporkCreateMethodName, sporkName, sporkDescription)
	common.FailIfErr(t, err)
	dataString := base64.StdEncoding.EncodeToString(data)

	defer z.CallContract(proposeAction(g.User1.Address, name, description, url, types.SporkContract, dataString)).
		Error(t, nil)
	insertMomentums(z, 2)

	governanceApi := embedded.NewGovernanceApi(z)
	actionsList, err := governanceApi.GetAllActions(0, 10)
	common.FailIfErr(t, err)
	if actionsList.Count != 1 || len(actionsList.List) != 1 {
		t.Fatalf("expected one governance action, got count=%v len=%v", actionsList.Count, len(actionsList.List))
	}
	assertGovernanceAction(t, actionsList.List[0], name, constants.Type1Action, 0, constants.ActionStatusVoting, false, 0, 0, 0)
}

// Activate spork
// Propose action to create a spork
// Vote action
func activateGovernanceStep2(t *testing.T, z mock.MockZenon) {
	activateGovernanceStep1(t, z)
	insertMomentums(z, 10)

	governanceApi := embedded.NewGovernanceApi(z)
	actionsList, err := governanceApi.GetAllActions(0, 10)
	common.FailIfErr(t, err)
	id := actionsList.List[0].Id

	defer z.CallContract(voteByName(g.Pillar1.Address, g.Pillar1Name, id, definition.VoteYes)).Error(t, nil)
	insertMomentums(z, 2)
	defer z.CallContract(voteByName(g.Pillar2.Address, g.Pillar2Name, id, definition.VoteYes)).Error(t, nil)
	insertMomentums(z, 2)
	defer z.CallContract(voteByName(g.Pillar3.Address, g.Pillar3Name, id, definition.VoteYes)).Error(t, nil)
	insertMomentums(z, 2)
	defer z.CallContract(voteByName(g.Pillar4.Address, g.Pillar4Name, id, definition.VoteNo)).Error(t, nil)
	insertMomentums(z, 2)
	defer z.CallContract(voteByName(g.Pillar5.Address, g.Pillar5Name, id, definition.VoteNo)).Error(t, nil)
	insertMomentums(z, 2)
	defer z.CallContract(voteByName(g.Pillar6.Address, g.Pillar6Name, id, definition.VoteYes)).Error(t, nil)
	insertMomentums(z, 2)

	actionsList, err = governanceApi.GetAllActions(0, 10)
	common.FailIfErr(t, err)
	if actionsList.Count != 1 || len(actionsList.List) != 1 {
		t.Fatalf("expected one governance action, got count=%v len=%v", actionsList.Count, len(actionsList.List))
	}
	assertGovernanceAction(t, actionsList.List[0], "create btc-bridge spork", constants.Type1Action, 0, constants.ActionStatusVoting, false, 6, 4, 2)
}

// Activate spork
// Propose action to create a spork
// Vote action
// Execute action and check that the spork is created
func activateGovernanceStep3(t *testing.T, z mock.MockZenon) {
	activateGovernanceStep2(t, z)
	insertMomentums(z, 10)

	governanceApi := embedded.NewGovernanceApi(z)
	actionsList, err := governanceApi.GetAllActions(0, 10)
	common.FailIfErr(t, err)
	id := actionsList.List[0].Id

	defer z.CallContract(executeAction(g.User1.Address, id)).Error(t, nil)
	insertMomentums(z, 2)

	// Action should be executed
	action, err := governanceApi.GetActionById(id)
	common.FailIfErr(t, err)
	assertGovernanceAction(t, action, "create btc-bridge spork", constants.Type1Action, 0, constants.ActionStatusApproved, true, 6, 4, 2)

	// The spork should be created
	sporkApi := embedded.NewSporkApi(z)
	allSporks, err := sporkApi.GetAll(0, 10)
	common.FailIfErr(t, err)
	if allSporks.Count != 2 || len(allSporks.List) != 2 {
		t.Fatalf("expected two sporks, got count=%v len=%v", allSporks.Count, len(allSporks.List))
	}
	assertSpork(t, allSporks, "btc-bridge", "btc-bridge logic", false, 0)
	assertSpork(t, allSporks, "spork-governance", "activate spork for governance", true, 9)
}

// Activate spork
// Propose action to create a spork
// Vote action
// Execute action and check that the spork is created
// Propose action to activate spork
func activateGovernanceStep4(t *testing.T, z mock.MockZenon) {
	activateGovernanceStep3(t, z)
	insertMomentums(z, 10)

	sporkName := "btc-bridge"
	sporkId := types.ZeroHash
	sporkApi := embedded.NewSporkApi(z)
	allSporks, err := sporkApi.GetAll(0, 10)
	common.FailIfErr(t, err)
	for _, spork := range allSporks.List {
		if spork.Name == sporkName {
			sporkId = spork.Id
		}
	}

	name := "activate btc-bridge spork"
	description := "this action will activate the btc-spork"
	url := "https://qwerty.com"

	data, err := definition.ABISpork.PackMethod(definition.SporkActivateMethodName, sporkId)
	common.FailIfErr(t, err)
	dataString := base64.StdEncoding.EncodeToString(data)

	defer z.CallContract(proposeAction(g.User1.Address, name, description, url, types.SporkContract, dataString)).
		Error(t, nil)
	insertMomentums(z, 2)

	governanceApi := embedded.NewGovernanceApi(z)
	actionsList, err := governanceApi.GetAllActions(0, 10)
	common.FailIfErr(t, err)
	if actionsList.Count != 2 || len(actionsList.List) != 2 {
		t.Fatalf("expected two governance actions, got count=%v len=%v", actionsList.Count, len(actionsList.List))
	}
	assertGovernanceAction(t, actionsList.List[0], "create btc-bridge spork", constants.Type1Action, 0, constants.ActionStatusApproved, true, 6, 4, 2)
	assertGovernanceAction(t, actionsList.List[1], name, constants.Type1Action, 0, constants.ActionStatusVoting, false, 0, 0, 0)
}

// Activate spork
// Propose action to create a spork
// Vote action
// Execute action and check that the spork is created
// Propose action to activate spork
// Vote action
func activateGovernanceStep5(t *testing.T, z mock.MockZenon) {
	activateGovernanceStep4(t, z)
	insertMomentums(z, 10)

	actionName := "activate btc-bridge spork"
	actionId := types.ZeroHash
	governanceApi := embedded.NewGovernanceApi(z)
	actionsList, err := governanceApi.GetAllActions(0, 10)
	common.FailIfErr(t, err)
	for _, action := range actionsList.List {
		if action.Name == actionName {
			actionId = action.Id
		}
	}

	defer z.CallContract(voteByName(g.Pillar1.Address, g.Pillar1Name, actionId, definition.VoteYes)).Error(t, nil)
	insertMomentums(z, 2)
	defer z.CallContract(voteByName(g.Pillar2.Address, g.Pillar2Name, actionId, definition.VoteYes)).Error(t, nil)
	insertMomentums(z, 2)
	defer z.CallContract(voteByName(g.Pillar3.Address, g.Pillar3Name, actionId, definition.VoteYes)).Error(t, nil)
	insertMomentums(z, 2)
	defer z.CallContract(voteByName(g.Pillar4.Address, g.Pillar4Name, actionId, definition.VoteNo)).Error(t, nil)
	insertMomentums(z, 2)
	defer z.CallContract(voteByName(g.Pillar5.Address, g.Pillar5Name, actionId, definition.VoteYes)).Error(t, nil)
	insertMomentums(z, 2)
	defer z.CallContract(voteByName(g.Pillar6.Address, g.Pillar6Name, actionId, definition.VoteYes)).Error(t, nil)
	insertMomentums(z, 2)

	actionsList, err = governanceApi.GetAllActions(0, 10)
	common.FailIfErr(t, err)
	if actionsList.Count != 2 || len(actionsList.List) != 2 {
		t.Fatalf("expected two governance actions, got count=%v len=%v", actionsList.Count, len(actionsList.List))
	}
	assertGovernanceAction(t, actionsList.List[0], "create btc-bridge spork", constants.Type1Action, 0, constants.ActionStatusApproved, true, 6, 4, 2)
	assertGovernanceAction(t, actionsList.List[1], actionName, constants.Type1Action, 0, constants.ActionStatusVoting, false, 6, 5, 1)
}

// Activate spork
// Propose action to create a spork
// Vote action
// Execute action and check that the spork is created
// Propose action to activate spork
// Vote action
// Execute action and check that the spork is active
func activateGovernanceStep6(t *testing.T, z mock.MockZenon) {
	activateGovernanceStep5(t, z)
	insertMomentums(z, 10)

	actionName := "activate btc-bridge spork"
	actionId := types.ZeroHash
	governanceApi := embedded.NewGovernanceApi(z)
	actionList, err := governanceApi.GetAllActions(0, 10)
	common.FailIfErr(t, err)
	for _, action := range actionList.List {
		if action.Name == actionName {
			actionId = action.Id
		}
	}

	defer z.CallContract(executeAction(g.User1.Address, actionId)).Error(t, nil)
	insertMomentums(z, 2)

	// Action should be executed
	action, err := governanceApi.GetActionById(actionId)
	common.FailIfErr(t, err)
	assertGovernanceAction(t, action, actionName, constants.Type1Action, 0, constants.ActionStatusApproved, true, 6, 5, 1)

	// The spork should be created
	sporkApi := embedded.NewSporkApi(z)
	allSporks, err := sporkApi.GetAll(0, 10)
	common.FailIfErr(t, err)
	if allSporks.Count != 2 || len(allSporks.List) != 2 {
		t.Fatalf("expected two sporks, got count=%v len=%v", allSporks.Count, len(allSporks.List))
	}
	assertSpork(t, allSporks, "btc-bridge", "btc-bridge logic", true, 116)
	assertSpork(t, allSporks, "spork-governance", "activate spork for governance", true, 9)
}

func TestGovernance(t *testing.T) {
	overrideType1GovernanceVotingPeriods(t, []int64{30, 30, 30, 30})

	z := mock.NewMockZenonWithCustomEpochDuration(t, time.Hour)
	defer z.StopPanic()

	activateGovernanceStep6(t, z)
}

func TestGovernanceRatchetAdvancesUnderVotedAction(t *testing.T) {
	overrideType1GovernanceVotingPeriods(t, []int64{30, 30, 30, 30})

	z := mock.NewMockZenonWithCustomEpochDuration(t, time.Hour)
	defer z.StopPanic()

	activateGovernanceStep1(t, z)

	governanceApi := embedded.NewGovernanceApi(z)
	actionsList, err := governanceApi.GetAllActions(0, 10)
	common.FailIfErr(t, err)
	if actionsList.Count != 1 || len(actionsList.List) != 1 {
		t.Fatalf("expected one governance action, got count=%v len=%v", actionsList.Count, len(actionsList.List))
	}

	action := actionsList.List[0]
	round0VoteId := action.CurrentVoteId

	insertMomentums(z, 2)
	expect := z.CallContract(executeAction(g.User1.Address, action.Id))
	insertMomentums(z, 2)
	expect.Error(t, nil)

	action, err = governanceApi.GetActionById(action.Id)
	common.FailIfErr(t, err)
	if action.CurrentVoteId == round0VoteId {
		t.Fatalf("expected vote id to change after advancing round")
	}
	assertGovernanceAction(t, action, "create btc-bridge spork", constants.Type1Action, 1, constants.ActionStatusVoting, false, 0, 0, 0)
}

func TestGovernanceBridgeTimeChallengeRequiresSecondAction(t *testing.T) {
	z := mock.NewMockZenonWithCustomEpochDuration(t, time.Hour)
	defer z.StopPanic()

	activateBridgeStep2(t, z)
	activateGovernanceStep0(t, z)

	bridgeAPI := embedded.NewBridgeApi(z)
	securityInfo, err := bridgeAPI.GetSecurityInfo()
	common.FailIfErr(t, err)
	newAdministrator := g.User4.Address
	data := definition.ABIBridge.PackMethodPanic(definition.ChangeAdministratorMethodName, newAdministrator)

	firstAction := proposeVoteAndExecuteGovernanceAction(t, z, "start bridge admin change", "start bridge administrator time challenge", types.BridgeContract, data)
	assertGovernanceAction(t, firstAction, "start bridge admin change", constants.Type2Action, 0, constants.ActionStatusApproved, true, 4, 4, 0)
	assertBridgeAdministrator(t, z, g.User5.Address)

	timeChallenges, err := bridgeAPI.GetTimeChallengesInfo()
	common.FailIfErr(t, err)
	challenge := findTimeChallenge(t, timeChallenges, definition.ChangeAdministratorMethodName)
	if challenge.ParamsHash.IsZero() {
		t.Fatalf("expected first governance action to start bridge administrator time challenge")
	}

	frontierMomentum, err := z.Chain().GetFrontierMomentumStore().GetFrontierMomentum()
	common.FailIfErr(t, err)
	z.InsertMomentumsTo(frontierMomentum.Height + securityInfo.AdministratorDelay + 2)

	callContractAndInsert(t, z, executeAction(g.User1.Address, firstAction.Id), nil)
	assertBridgeAdministrator(t, z, g.User5.Address)

	secondAction := proposeVoteAndExecuteGovernanceAction(t, z, "complete bridge admin change", "complete bridge administrator time challenge", types.BridgeContract, data)
	assertGovernanceAction(t, secondAction, "complete bridge admin change", constants.Type2Action, 0, constants.ActionStatusApproved, true, 4, 4, 0)
	assertBridgeAdministrator(t, z, newAdministrator)

	timeChallenges, err = bridgeAPI.GetTimeChallengesInfo()
	common.FailIfErr(t, err)
	challenge = findTimeChallenge(t, timeChallenges, definition.ChangeAdministratorMethodName)
	if !challenge.ParamsHash.IsZero() {
		t.Fatalf("expected second governance action to complete bridge administrator time challenge")
	}
}

func proposeAction(user types.Address, name, description, url string, destination types.Address, data string) *nom.AccountBlock {
	return &nom.AccountBlock{
		Address:       user,
		ToAddress:     types.GovernanceContract,
		TokenStandard: types.ZnnTokenStandard,
		Amount:        big.NewInt(1 * constants.Decimals),
		Data: definition.ABIGovernance.PackMethodPanic(definition.ProposeActionMethodName,
			name, description, url, destination, data),
	}
}

func executeAction(user types.Address, id types.Hash) *nom.AccountBlock {
	return &nom.AccountBlock{
		Address:       user,
		ToAddress:     types.GovernanceContract,
		TokenStandard: types.ZnnTokenStandard,
		Amount:        big.NewInt(0),
		Data:          definition.ABIGovernance.PackMethodPanic(definition.ExecuteActionMethodName, id),
	}
}

func voteByName(pillarAddress types.Address, pillarName string, id types.Hash, vote uint8) *nom.AccountBlock {
	return &nom.AccountBlock{
		Address:   pillarAddress,
		ToAddress: types.GovernanceContract,
		Data: definition.ABIGovernance.PackMethodPanic(definition.VoteByNameMethodName,
			id,
			pillarName,
			vote,
		),
	}
}
