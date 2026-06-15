package definition

import (
	"strings"

	"github.com/zenon-network/go-zenon/common"
	"github.com/zenon-network/go-zenon/common/db"
	"github.com/zenon-network/go-zenon/common/types"
	"github.com/zenon-network/go-zenon/vm/abi"
	"github.com/zenon-network/go-zenon/vm/constants"
)

const (
	jsonGovernance = `
	[
		{"type":"function","name":"ProposeAction", "inputs":[
			{"name":"name","type":"string"},
			{"name":"description","type":"string"},
			{"name":"url","type":"string"},
			{"name":"destination","type":"address"},
			{"name":"data","type":"string"}
		]},

		{"type":"function","name":"ExecuteAction", "inputs":[
			{"name":"id","type":"hash"}
		]},

		{"type":"function","name":"VoteByName","inputs":[
			{"name":"id","type":"hash"},
			{"name":"name","type":"string"},
			{"name":"vote","type":"uint8"}
		]},
		{"type":"function","name":"VoteByProdAddress","inputs":[
			{"name":"id","type":"hash"},
			{"name":"vote","type":"uint8"}
		]},

		{"type":"variable","name":"action","inputs":[
			{"name":"owner","type":"address"},
			{"name":"name","type":"string"},
			{"name":"description","type":"string"},
			{"name":"url","type":"string"},
			{"name":"destination","type":"address"},
			{"name":"data","type":"string"},
			{"name":"creationTimestamp","type":"int64"},
			{"name":"type","type":"uint8"},
			{"name":"round","type":"uint8"},
			{"name":"currentVoteId","type":"hash"},
			{"name":"roundStartTimestamp","type":"int64"},
			{"name":"status","type":"uint8"},
			{"name":"executed","type":"bool"}
		]}
	]`

	ProposeActionMethodName = "ProposeAction"
	ExecuteActionMethodName = "ExecuteAction"

	actionVariableName = "action"
)

var (
	ABIGovernance = abi.JSONToABIContract(strings.NewReader(jsonGovernance))

	actionKeyPrefix = []byte{0}
)

type ActionVariable struct {
	Id                  types.Hash
	Owner               types.Address
	Name                string
	Description         string
	Url                 string
	Destination         types.Address
	Data                string
	CreationTimestamp   int64
	Type                uint8
	Round               uint8
	CurrentVoteId       types.Hash
	RoundStartTimestamp int64
	Status              uint8
	Executed            bool
}

func (action *ActionVariable) Save(context db.DB) {
	common.DealWithErr(context.Put(action.Key(),
		ABIGovernance.PackVariablePanic(
			actionVariableName,
			action.Owner,
			action.Name,
			action.Description,
			action.Url,
			action.Destination,
			action.Data,
			action.CreationTimestamp,
			action.Type,
			action.Round,
			action.CurrentVoteId,
			action.RoundStartTimestamp,
			action.Status,
			action.Executed,
		)))
}
func (action *ActionVariable) Delete(context db.DB) {
	common.DealWithErr(context.Delete(action.Key()))
}
func (action *ActionVariable) Key() []byte {
	return common.JoinBytes(actionKeyPrefix, action.Id.Bytes())
}

func ActionVoteId(id types.Hash, round uint8) types.Hash {
	if round == 0 {
		return id
	}
	return types.NewHash(common.JoinBytes([]byte("governance-action-round"), id.Bytes(), []byte{round}))
}

func parseAction(data, key []byte) (*ActionVariable, error) {
	if len(data) > 0 {
		dataVar := new(ActionVariable)
		if err := ABIGovernance.UnpackVariable(dataVar, actionVariableName, data); err != nil {
			return nil, err
		}
		if err := dataVar.Id.SetBytes(key[1:33]); err != nil {
			return nil, err
		}
		return dataVar, nil
	} else {
		return nil, constants.ErrDataNonExistent
	}
}

func GetActionById(context db.DB, id types.Hash) (*ActionVariable, error) {
	key := (&ActionVariable{Id: id}).Key()
	data, err := context.Get(key)
	common.DealWithErr(err)
	return parseAction(data, key)
}

func GetActions(context db.DB) ([]*ActionVariable, error) {
	iterator := context.NewIterator(actionKeyPrefix)
	defer iterator.Release()
	list := make([]*ActionVariable, 0)

	for {
		if !iterator.Next() {
			if iterator.Error() != nil {
				return nil, iterator.Error()
			}
			break
		}
		if action, err := parseAction(iterator.Value(), iterator.Key()); err == nil && action != nil {
			list = append(list, action)
		} else {
			return nil, err
		}
	}

	return list, nil
}
