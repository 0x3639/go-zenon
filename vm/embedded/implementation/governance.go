package implementation

import (
	"encoding/base64"
	"regexp"

	"github.com/zenon-network/go-zenon/chain/nom"
	"github.com/zenon-network/go-zenon/common"
	"github.com/zenon-network/go-zenon/common/types"
	"github.com/zenon-network/go-zenon/vm/constants"
	"github.com/zenon-network/go-zenon/vm/embedded/definition"
	"github.com/zenon-network/go-zenon/vm/vm_context"
)

var (
	governanceLog        = common.EmbeddedLogger.New("contract", "governance")
	governanceURLPattern = regexp.MustCompile("^([Hh][Tt][Tt][Pp][Ss]?://)?[a-zA-Z0-9]{2,60}\\.[a-zA-Z]{1,63}([-a-zA-Z0-9()@:%_+.~#?&/=]{0,100})$")
)

const (
	actionVotePending uint8 = iota
	actionVoteApproved
	actionVoteRejected
)

type ProposeActionMethod struct {
	MethodName string
}

type governanceActionValidator interface {
	ValidateSendBlock(block *nom.AccountBlock) error
}

var governanceAllowedActionMethods = map[types.Address]map[string]governanceActionValidator{
	types.SporkContract: {
		definition.SporkCreateMethodName:   &CreateSporkMethod{definition.SporkCreateMethodName},
		definition.SporkActivateMethodName: &ActivateSporkMethod{definition.SporkActivateMethodName},
	},
	types.BridgeContract: {
		definition.SetNetworkMethodName:           &SetNetworkMethod{definition.SetNetworkMethodName},
		definition.RemoveNetworkMethodName:        &RemoveNetworkMethod{definition.RemoveNetworkMethodName},
		definition.SetNetworkMetadataMethodName:   &SetNetworkMetadataMethod{definition.SetNetworkMetadataMethodName},
		definition.SetTokenPairMethod:             &SetTokenPairMethod{definition.SetTokenPairMethod},
		definition.RemoveTokenPairMethodName:      &RemoveTokenPairMethod{definition.RemoveTokenPairMethodName},
		definition.HaltMethodName:                 &HaltMethod{definition.HaltMethodName},
		definition.UnhaltMethodName:               &UnhaltMethod{definition.UnhaltMethodName},
		definition.EmergencyMethodName:            &EmergencyMethod{definition.EmergencyMethodName},
		definition.ChangeTssECDSAPubKeyMethodName: &ChangeTssECDSAPubKeyMethod{definition.ChangeTssECDSAPubKeyMethodName},
		definition.ChangeAdministratorMethodName:  &ChangeAdministratorMethod{definition.ChangeAdministratorMethodName},
		definition.SetAllowKeygenMethodName:       &SetAllowKeygenMethod{definition.SetAllowKeygenMethodName},
		definition.SetOrchestratorInfoMethodName:  &SetOrchestratorInfoMethod{definition.SetOrchestratorInfoMethodName},
		definition.SetBridgeMetadataMethodName:    &SetBridgeMetadataMethod{definition.SetBridgeMetadataMethodName},
		definition.RevokeUnwrapRequestMethodName:  &RevokeUnwrapRequestMethod{definition.RevokeUnwrapRequestMethodName},
		definition.NominateGuardiansMethodName:    &NominateGuardiansMethod{definition.NominateGuardiansMethodName},
	},
	types.LiquidityContract: {
		definition.FundMethodName:                        &FundMethod{definition.FundMethodName},
		definition.BurnZnnMethodName:                     &BurnZnnMethod{definition.BurnZnnMethodName},
		definition.SetTokenTupleMethodName:               &SetTokenTupleMethod{definition.SetTokenTupleMethodName},
		definition.SetIsHaltedMethodName:                 &SetIsHalted{definition.SetIsHaltedMethodName},
		definition.UnlockLiquidityStakeEntriesMethodName: &UnlockLiquidityStakeEntries{definition.UnlockLiquidityStakeEntriesMethodName},
		definition.SetAdditionalRewardMethodName:         &SetAdditionalReward{definition.SetAdditionalRewardMethodName},
		definition.ChangeAdministratorMethodName:         &ChangeAdministratorLiquidity{definition.ChangeAdministratorMethodName},
		definition.NominateGuardiansMethodName:           &NominateGuardiansLiquidity{definition.NominateGuardiansMethodName},
		definition.EmergencyMethodName:                   &EmergencyLiquidity{definition.EmergencyMethodName},
	},
}

func governanceActionMethodName(destination types.Address, data []byte) (string, error) {
	var (
		methodName string
		err        error
	)

	switch destination {
	case types.SporkContract:
		method, methodErr := definition.ABISpork.MethodById(data)
		if methodErr == nil {
			methodName = method.Name
		}
		err = methodErr
	case types.BridgeContract:
		method, methodErr := definition.ABIBridge.MethodById(data)
		if methodErr == nil {
			methodName = method.Name
		}
		err = methodErr
	case types.LiquidityContract:
		method, methodErr := definition.ABILiquidity.MethodById(data)
		if methodErr == nil {
			methodName = method.Name
		}
		err = methodErr
	default:
		return "", constants.ErrPermissionDenied
	}

	if err != nil {
		return "", constants.ErrForbiddenParam
	}
	return methodName, nil
}

func checkGovernanceActionDestination(destination types.Address, data []byte) error {
	allowedMethods, ok := governanceAllowedActionMethods[destination]
	if !ok {
		return constants.ErrPermissionDenied
	}

	methodName, err := governanceActionMethodName(destination, data)
	if err != nil {
		return err
	}
	validator, ok := allowedMethods[methodName]
	if !ok {
		return constants.ErrPermissionDenied
	}

	return validator.ValidateSendBlock(&nom.AccountBlock{
		Address:       types.GovernanceContract,
		ToAddress:     destination,
		Amount:        common.Big0,
		TokenStandard: types.ZnnTokenStandard,
		Data:          append([]byte(nil), data...),
	})
}

func checkActionStatic(param *definition.ActionVariable) error {
	if len(param.Name) == 0 ||
		len(param.Name) > constants.ProjectNameLengthMax {
		governanceLog.Debug("governance-check-action-static", "reason", "malformed-name")
		return constants.ErrInvalidName
	}

	if len(param.Description) == 0 || len(param.Description) > constants.ProjectDescriptionLengthMax {
		governanceLog.Debug("governance-check-action-static", "reason", "malformed-description")
		return constants.ErrInvalidDescription
	}

	if !governanceURLPattern.MatchString(param.Url) || len(param.Url) == 0 {
		governanceLog.Debug("governance-check-action-static", "reason", "malformed-url")
		return constants.ErrForbiddenParam
	}

	if param.Destination.String() == types.TokenContract.String() {
		governanceLog.Debug("governance-check-action-static", "reason", "forbidden-destination")
		return constants.ErrPermissionDenied
	}

	if len(param.Data) > base64.StdEncoding.EncodedLen(constants.GovernanceActionDataMaxLength) {
		governanceLog.Debug("governance-check-action-static", "reason", "data-too-large")
		return constants.ErrForbiddenParam
	}

	data, err := base64.StdEncoding.DecodeString(param.Data)
	if err != nil {
		governanceLog.Debug("governance-check-action-static", "reason", "malformed-data")
		return constants.ErrInvalidB64Decode
	}
	if len(data) > constants.GovernanceActionDataMaxLength {
		governanceLog.Debug("governance-check-action-static", "reason", "data-too-large")
		return constants.ErrForbiddenParam
	}
	if err := checkGovernanceActionDestination(param.Destination, data); err != nil {
		if err == constants.ErrPermissionDenied {
			governanceLog.Debug("governance-check-action-static", "reason", "forbidden-destination-or-method")
		} else {
			governanceLog.Debug("governance-check-action-static", "reason", "malformed-action-data")
		}
		return err
	}
	return nil
}

func (p *ProposeActionMethod) GetPlasma(plasmaTable *constants.PlasmaTable) (uint64, error) {
	return plasmaTable.EmbeddedWDoubleWithdraw, nil
}

func (p *ProposeActionMethod) ValidateSendBlock(block *nom.AccountBlock) error {
	var err error
	param := new(definition.ActionVariable)

	if err := definition.ABIGovernance.UnpackMethod(param, p.MethodName, block.Data); err != nil {
		return constants.ErrUnpackError
	}

	if err := checkActionStatic(param); err != nil {
		return err
	}

	if block.TokenStandard != types.ZnnTokenStandard || block.Amount.Cmp(constants.ProjectCreationAmount) != 0 {
		return constants.ErrInvalidTokenOrAmount
	}

	block.Data, err = definition.ABIGovernance.PackMethod(p.MethodName, param.Name, param.Description, param.Url, param.Destination, param.Data)
	return err
}
func (p *ProposeActionMethod) ReceiveBlock(context vm_context.AccountVmContext, sendBlock *nom.AccountBlock) ([]*nom.AccountBlock, error) {
	if err := p.ValidateSendBlock(sendBlock); err != nil {
		return nil, err
	}

	proposedAction := new(definition.ActionVariable)
	err := definition.ABIGovernance.UnpackMethod(proposedAction, p.MethodName, sendBlock.Data)
	common.DealWithErr(err)

	frontierMomentum, err := context.GetFrontierMomentum()
	if err != nil {
		return nil, err
	}

	proposedAction.Id = sendBlock.Hash
	proposedAction.Owner = sendBlock.Address
	proposedAction.CreationTimestamp = frontierMomentum.Timestamp.Unix()
	proposedAction.Round = 0
	proposedAction.CurrentVoteId = definition.ActionVoteId(proposedAction.Id, proposedAction.Round)
	proposedAction.RoundStartTimestamp = proposedAction.CreationTimestamp
	proposedAction.Status = constants.ActionStatusVoting
	proposedAction.Executed = false
	// Only account-blocks to spork are considered type1 for now
	if proposedAction.Destination.String() == types.SporkContract.String() {
		proposedAction.Type = constants.Type1Action
	} else {
		proposedAction.Type = constants.Type2Action
	}

	proposedAction.Save(context.Storage())

	// Add hash to votable hashes
	(&definition.VotableHash{Id: proposedAction.CurrentVoteId}).Save(context.Storage())

	governanceLog.Debug("successfully created action proposal", "action", proposedAction)
	return nil, nil
}

type ExecuteActionMethod struct {
	MethodName string
}

func (p *ExecuteActionMethod) GetPlasma(plasmaTable *constants.PlasmaTable) (uint64, error) {
	return plasmaTable.EmbeddedSimple, nil
}

func (p *ExecuteActionMethod) ValidateSendBlock(block *nom.AccountBlock) error {
	var err error
	id := new(types.Hash)

	if err := definition.ABIGovernance.UnpackMethod(id, p.MethodName, block.Data); err != nil {
		return constants.ErrUnpackError
	}

	if block.TokenStandard != types.ZnnTokenStandard || block.Amount.Sign() != 0 {
		return constants.ErrInvalidTokenOrAmount
	}

	block.Data, err = definition.ABIGovernance.PackMethod(p.MethodName, id)
	return err
}
func (p *ExecuteActionMethod) ReceiveBlock(context vm_context.AccountVmContext, sendBlock *nom.AccountBlock) ([]*nom.AccountBlock, error) {
	if err := p.ValidateSendBlock(sendBlock); err != nil {
		return nil, err
	}

	id := new(types.Hash)
	err := definition.ABIGovernance.UnpackMethod(id, p.MethodName, sendBlock.Data)
	common.DealWithErr(err)

	action, err := definition.GetActionById(context.Storage(), *id)
	if err != nil {
		return nil, err
	}

	frontierMomentum, err := context.GetFrontierMomentum()
	if err != nil {
		return nil, err
	}

	if action.CurrentVoteId.IsZero() {
		action.CurrentVoteId = definition.ActionVoteId(action.Id, action.Round)
	}
	if action.RoundStartTimestamp == 0 {
		action.RoundStartTimestamp = action.CreationTimestamp
	}
	schedule, err := constants.GovernanceActionScheduleForChain(context.MomentumStore().ChainIdentifier(), action.Type, action.Round)
	if err != nil {
		return nil, err
	}

	if action.Executed || action.Status == constants.ActionStatusApproved ||
		action.Status == constants.ActionStatusRejected ||
		action.Status == constants.ActionStatusNoDecision {
		governanceLog.Debug("action-terminal", "executed", action.Executed, "status", action.Status)
		return nil, nil
	}

	pillarList, err := context.MomentumStore().GetActivePillars()
	if err != nil {
		return nil, err
	}
	numPillars := uint32(len(pillarList))
	decision, err := checkActionVotes(context, action.CurrentVoteId, numPillars, action.Type, action.Round)
	if err != nil {
		return nil, err
	}
	expired := action.RoundStartTimestamp+schedule.VotingPeriod < frontierMomentum.Timestamp.Unix()
	if action.Type == constants.Type1Action && !expired && decision != actionVotePending {
		return nil, nil
	}
	if decision == actionVoteRejected {
		action.Status = constants.ActionStatusRejected
		(&definition.VotableHash{Id: action.CurrentVoteId}).Delete(context.Storage())
		action.Save(context.Storage())
		governanceLog.Debug("action rejected by voting", "action-id", action.Id, "round", action.Round)
		return nil, nil
	}
	if decision != actionVoteApproved {
		if !expired {
			return nil, nil
		}

		maxRound, err := constants.GovernanceActionMaxRound(action.Type)
		if err != nil {
			return nil, err
		}
		(&definition.VotableHash{Id: action.CurrentVoteId}).Delete(context.Storage())
		if action.Round >= maxRound {
			action.Status = constants.ActionStatusNoDecision
			action.Save(context.Storage())
			governanceLog.Debug("action closed without decision", "action-id", action.Id, "round", action.Round)
			return nil, nil
		}

		action.Round += 1
		action.CurrentVoteId = definition.ActionVoteId(action.Id, action.Round)
		action.RoundStartTimestamp = frontierMomentum.Timestamp.Unix()
		action.Status = constants.ActionStatusVoting
		action.Save(context.Storage())
		(&definition.VotableHash{Id: action.CurrentVoteId}).Save(context.Storage())
		governanceLog.Debug("action advanced to next voting round", "action-id", action.Id, "round", action.Round, "vote-id", action.CurrentVoteId)
		return nil, nil
	}

	data, err := base64.StdEncoding.DecodeString(action.Data)
	if err != nil {
		governanceLog.Debug("execute-action", "reason", "malformed-data")
		return nil, constants.ErrInvalidB64Decode
	}

	action.Status = constants.ActionStatusApproved
	action.Executed = true
	(&definition.VotableHash{Id: action.CurrentVoteId}).Delete(context.Storage())
	action.Save(context.Storage())

	governanceLog.Debug("action passed voting and is being executed", "action-id", action.Id, "round", action.Round)
	return []*nom.AccountBlock{
		{
			Address:       types.GovernanceContract,
			ToAddress:     action.Destination,
			BlockType:     nom.BlockTypeContractSend,
			Amount:        common.Big0,
			TokenStandard: types.ZnnTokenStandard,
			Data:          data,
		},
	}, nil
}

func checkActionVotes(context vm_context.AccountVmContext, id types.Hash, numPillars uint32, actionType, round uint8) (uint8, error) {
	breakdown := definition.GetVoteBreakdown(context.Storage(), id)
	status, err := checkActionVoteBreakdown(breakdown, numPillars, actionType, round)
	if err != nil {
		return actionVotePending, err
	}

	governanceLog.Debug("check action votes", "votes", breakdown, "round", round, "status", status)
	return status, nil
}

func checkActionVoteBreakdown(breakdown *definition.VoteBreakdown, numPillars uint32, actionType, round uint8) (uint8, error) {
	schedule, err := constants.GovernanceActionSchedule(actionType, round)
	if err != nil {
		return actionVotePending, err
	}

	directionalVotes := breakdown.Yes + breakdown.No
	enoughDirectionalVotes := directionalVotes*100 > numPillars*schedule.ActivePillarThreshold
	approved := enoughDirectionalVotes &&
		breakdown.Yes*100 > directionalVotes*schedule.DirectionalThreshold
	rejected := enoughDirectionalVotes &&
		breakdown.No*100 > directionalVotes*schedule.DirectionalThreshold

	status := actionVotePending
	if approved {
		status = actionVoteApproved
	} else if rejected {
		status = actionVoteRejected
	}

	return status, nil
}
