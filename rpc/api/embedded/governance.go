package embedded

import (
	"github.com/inconshreveable/log15"
	"github.com/zenon-network/go-zenon/chain"
	"github.com/zenon-network/go-zenon/common"
	"github.com/zenon-network/go-zenon/common/db"
	"github.com/zenon-network/go-zenon/common/types"
	"github.com/zenon-network/go-zenon/rpc/api"
	"github.com/zenon-network/go-zenon/vm/constants"
	"github.com/zenon-network/go-zenon/vm/embedded/definition"
	"github.com/zenon-network/go-zenon/zenon"
)

type GovernanceApi struct {
	chain chain.Chain
	log   log15.Logger
}

func NewGovernanceApi(z zenon.Zenon) *GovernanceApi {
	return &GovernanceApi{
		chain: z.Chain(),
		log:   common.RPCLogger.New("module", "embedded_governance_api"),
	}
}

type Action struct {
	*definition.ActionVariable
	Expired               bool                      `json:"Expired"`
	ActivePillarThreshold uint32                    `json:"ActivePillarThreshold"`
	DirectionalThreshold  uint32                    `json:"DirectionalThreshold"`
	VotingPeriod          int64                     `json:"VotingPeriod"`
	Votes                 *definition.VoteBreakdown `json:"Votes"`
}

func (a *GovernanceApi) GetActionById(id types.Hash) (*Action, error) {
	_, context, err := api.GetFrontierContext(a.chain, types.GovernanceContract)
	if err != nil {
		return nil, err
	}

	actionVariable, err := definition.GetActionById(context.Storage(), id)
	if err != nil {
		return nil, err
	}

	momentum, err := context.GetFrontierMomentum()
	if err != nil {
		return nil, err
	}
	return newGovernanceAction(context.Storage(), actionVariable, momentum.Timestamp.Unix())
}

type ActionList struct {
	Count int       `json:"count"`
	List  []*Action `json:"list"`
}

func (a *GovernanceApi) GetAllActions(pageIndex, pageSize uint32) (*ActionList, error) {
	if pageSize > api.RpcMaxPageSize {
		return nil, api.ErrPageSizeParamTooBig
	}

	_, context, err := api.GetFrontierContext(a.chain, types.GovernanceContract)
	if err != nil {
		return nil, err
	}

	actions, err := definition.GetActions(context.Storage())
	if err != nil {
		return nil, err
	}

	result := &ActionList{
		Count: len(actions),
		List:  make([]*Action, 0),
	}

	start, end := api.GetRange(pageIndex, pageSize, uint32(len(actions)))
	momentum, err := context.GetFrontierMomentum()
	if err != nil {
		return nil, err
	}
	for i := start; i < end; i++ {
		action, err := newGovernanceAction(context.Storage(), actions[i], momentum.Timestamp.Unix())
		if err != nil {
			return nil, err
		}

		result.List = append(result.List, action)
	}

	return result, nil
}

func newGovernanceAction(storage db.DB, actionVariable *definition.ActionVariable, now int64) (*Action, error) {
	schedule, err := constants.GovernanceActionSchedule(actionVariable.Type, actionVariable.Round)
	if err != nil {
		return nil, err
	}

	voteId := actionVariable.CurrentVoteId
	if voteId.IsZero() {
		voteId = definition.ActionVoteId(actionVariable.Id, actionVariable.Round)
		actionVariable.CurrentVoteId = voteId
	}
	roundStart := actionVariable.RoundStartTimestamp
	if roundStart == 0 {
		roundStart = actionVariable.CreationTimestamp
		actionVariable.RoundStartTimestamp = roundStart
	}

	return &Action{
		ActionVariable:        actionVariable,
		Expired:               roundStart+schedule.VotingPeriod < now,
		ActivePillarThreshold: schedule.ActivePillarThreshold,
		DirectionalThreshold:  schedule.DirectionalThreshold,
		VotingPeriod:          schedule.VotingPeriod,
		Votes:                 definition.GetVoteBreakdown(storage, voteId),
	}, nil
}
