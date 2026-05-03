package definition

import (
	"encoding/json"
	"math/big"
	"strings"

	"github.com/zenon-network/go-zenon/common"
	"github.com/zenon-network/go-zenon/common/db"
	"github.com/zenon-network/go-zenon/common/types"
	"github.com/zenon-network/go-zenon/vm/abi"
	"github.com/zenon-network/go-zenon/vm/constants"
)

// Project / phase status codes used by the Accelerator state
// machine. Values progress: Voting → Active → Paid → Completed,
// or to Closed if voting / payment fails. The discriminator is
// stored in the project / phase records.
//
// jsonAccelerator is the canonical Solidity-shaped ABI for the
// Accelerator contract: project lifecycle (CreateProject, AddPhase,
// UpdatePhase, Update), pillar voting (VoteByName, VoteByProdAddress),
// donations (Donate), and the per-project / per-phase storage
// records.
const (
	// VotingStatus marks an entry that has been created and is in
	// its voting window.
	VotingStatus uint8 = iota
	// ActiveStatus marks a project / phase whose vote passed; it
	// is awaiting payout.
	ActiveStatus
	// PaidStatus marks a phase that has been funded.
	PaidStatus
	// ClosedStatus marks an entry that did not pass voting.
	ClosedStatus
	// CompletedStatus marks a project whose every phase has been
	// paid.
	CompletedStatus

	jsonAccelerator = `
	[
		{"type":"function","name":"Update", "inputs":[]},
		{"type":"function","name":"Donate", "inputs":[]},
		
		{"type":"function","name":"CreateProject", "inputs":[
			{"name":"name","type":"string"},
			{"name":"description","type":"string"},
			{"name":"url","type":"string"},
			{"name":"znnFundsNeeded","type":"uint256"},
			{"name":"qsrFundsNeeded","type":"uint256"}
		]},
		{"type":"function","name":"AddPhase", "inputs":[
			{"name":"id","type":"hash"},
			{"name":"name","type":"string"},
			{"name":"description","type":"string"},
			{"name":"url","type":"string"},
			{"name":"znnFundsNeeded","type":"uint256"},
			{"name":"qsrFundsNeeded","type":"uint256"}
		]},
		{"type":"function","name":"UpdatePhase", "inputs":[
			{"name":"id","type":"hash"},
			{"name":"name","type":"string"},
			{"name":"description","type":"string"},
			{"name":"url","type":"string"},
			{"name":"znnFundsNeeded","type":"uint256"},
			{"name":"qsrFundsNeeded","type":"uint256"}
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

		{"type":"variable","name":"project","inputs":[
			{"name":"id", "type":"hash"},
			{"name":"owner","type":"address"},
			{"name":"name","type":"string"},
			{"name":"description","type":"string"},
			{"name":"url","type":"string"},
			{"name":"znnFundsNeeded","type":"uint256"},
			{"name":"qsrFundsNeeded","type":"uint256"},
			{"name":"creationTimestamp","type":"int64"},
			{"name":"lastUpdateTimestamp","type":"int64"},
			{"name":"status","type":"uint8"},
			{"name":"phaseIds","type":"hash[]"}
		]},

		{"type":"variable","name":"phase","inputs":[
			{"name":"id", "type":"hash"},
			{"name":"projectId", "type":"hash"},
			{"name":"name","type":"string"},
			{"name":"description","type":"string"},
			{"name":"url","type":"string"},
			{"name":"znnFundsNeeded","type":"uint256"},
			{"name":"qsrFundsNeeded","type":"uint256"},
			{"name":"creationTimestamp","type":"int64"},
			{"name":"acceptedTimestamp","type":"int64"},
			{"name":"status","type":"uint8"}
		]}
	]`

	// CreateProjectMethodName names the project-creation method
	// (caller pays [constants.ProjectCreationAmount] ZNN).
	CreateProjectMethodName = "CreateProject"
	// AddPhaseMethodName names the add-phase-to-project method.
	AddPhaseMethodName = "AddPhase"
	// UpdatePhaseMethodName names the phase-metadata-update method.
	UpdatePhaseMethodName = "UpdatePhase"

	ProjectVariableName = "project"
	PhaseVariableName   = "phase"

	_ byte = iota
	projectKeyPrefix
	phaseKeyPrefix
)

var (
	// ABIAccelerator is part of the package's public API.
	ABIAccelerator = abi.JSONToABIContract(strings.NewReader(jsonAccelerator))
)

// Project is part of the package's public API; see the surrounding code for usage.
type Project struct {
	Id                  types.Hash    `json:"id"`
	Owner               types.Address `json:"owner"`
	Name                string        `json:"name"`
	Description         string        `json:"description"`
	Url                 string        `json:"url"`
	ZnnFundsNeeded      *big.Int      `json:"znnFundsNeeded"`
	QsrFundsNeeded      *big.Int      `json:"qsrFundsNeeded"`
	CreationTimestamp   int64         `json:"creationTimestamp"`
	LastUpdateTimestamp int64         `json:"lastUpdateTimestamp"`
	Status              uint8         `json:"status"`
	PhaseIds            []types.Hash
}

// AcceleratorParam is part of the package's public API; see the surrounding code for usage.
type AcceleratorParam struct {
	Id             types.Hash
	Name           string
	Description    string
	Url            string
	ZnnFundsNeeded *big.Int
	QsrFundsNeeded *big.Int
}

// Save persists the receiver under its keyed slot in storage.
func (project *Project) Save(context db.DB) {
	common.DealWithErr(context.Put(project.Key(), project.Data()))
}

// Delete removes the receiver's record from storage.
func (project *Project) Delete(context db.DB) {
	common.DealWithErr(context.Delete(project.Key()))
}

// Key returns the storage key for the receiver.
func (project *Project) Key() []byte {
	return common.JoinBytes([]byte{projectKeyPrefix}, project.Id.Bytes())
}

// Data ABI-encodes the project record for storage.
func (project *Project) Data() []byte {
	return ABIAccelerator.PackVariablePanic(
		ProjectVariableName,
		project.Id,
		project.Owner,
		project.Name,
		project.Description,
		project.Url,
		project.ZnnFundsNeeded,
		project.QsrFundsNeeded,
		project.CreationTimestamp,
		project.LastUpdateTimestamp,
		project.Status,
		project.PhaseIds,
	)
}

// GetCurrentPhase loads the CurrentPhase record from storage.
func (project *Project) GetCurrentPhase(context db.DB) (*Phase, error) {
	if len(project.PhaseIds) > 0 {
		currentActivePhaseId := project.PhaseIds[len(project.PhaseIds)-1]
		return GetPhaseEntry(context, currentActivePhaseId)
	} else {
		return nil, constants.ErrDataNonExistent
	}
}

func parseProject(data []byte) *Project {
	project := new(Project)
	ABIAccelerator.UnpackVariablePanic(project, ProjectVariableName, data)
	return project
}

// Phase is part of the package's public API; see the surrounding code for usage.
type Phase struct {
	Id                types.Hash `json:"id"`
	ProjectId         types.Hash `json:"projectID"`
	Name              string     `json:"name"`
	Description       string     `json:"description"`
	Url               string     `json:"url"`
	ZnnFundsNeeded    *big.Int   `json:"znnFundsNeeded"`
	QsrFundsNeeded    *big.Int   `json:"qsrFundsNeeded"`
	CreationTimestamp int64      `json:"creationTimestamp"`
	AcceptedTimestamp int64      `json:"acceptedTimestamp"`
	Status            uint8      `json:"status"`
}

// PhaseMarshal is part of the package's public API; see the surrounding code for usage.
type PhaseMarshal struct {
	Id                types.Hash `json:"id"`
	ProjectId         types.Hash `json:"projectID"`
	Name              string     `json:"name"`
	Description       string     `json:"description"`
	Url               string     `json:"url"`
	ZnnFundsNeeded    string     `json:"znnFundsNeeded"`
	QsrFundsNeeded    string     `json:"qsrFundsNeeded"`
	CreationTimestamp int64      `json:"creationTimestamp"`
	AcceptedTimestamp int64      `json:"acceptedTimestamp"`
	Status            uint8      `json:"status"`
}

// ToProjectMarshal projects the receiver to its JSON-friendly ProjectMarshal twin.
func (phase *Phase) ToProjectMarshal() *PhaseMarshal {
	aux := &PhaseMarshal{
		Id:                phase.Id,
		ProjectId:         phase.ProjectId,
		Name:              phase.Name,
		Description:       phase.Description,
		Url:               phase.Url,
		ZnnFundsNeeded:    phase.ZnnFundsNeeded.String(),
		QsrFundsNeeded:    phase.QsrFundsNeeded.String(),
		CreationTimestamp: phase.CreationTimestamp,
		AcceptedTimestamp: phase.AcceptedTimestamp,
		Status:            phase.Status,
	}
	return aux
}

// MarshalJSON forwards through the Marshal twin so big.Int fields render as decimal strings.
func (phase *Phase) MarshalJSON() ([]byte, error) {
	return json.Marshal(phase.ToProjectMarshal())
}

// UnmarshalJSON inflates the JSON wire form back into the in-memory receiver.
func (phase *Phase) UnmarshalJSON(data []byte) error {
	aux := new(PhaseMarshal)
	if err := json.Unmarshal(data, aux); err != nil {
		return err
	}
	phase.Id = aux.Id
	phase.ProjectId = aux.ProjectId
	phase.Name = aux.Name
	phase.Description = aux.Description
	phase.Url = aux.Url
	phase.ZnnFundsNeeded = common.StringToBigInt(aux.ZnnFundsNeeded)
	phase.QsrFundsNeeded = common.StringToBigInt(aux.QsrFundsNeeded)
	phase.CreationTimestamp = aux.CreationTimestamp
	phase.AcceptedTimestamp = aux.AcceptedTimestamp
	phase.Status = aux.Status
	return nil
}

// Save persists the receiver under its keyed slot in storage.
func (phase *Phase) Save(context db.DB) {
	common.DealWithErr(context.Put(phase.Key(), phase.Data()))
}

// Delete removes the receiver's record from storage.
func (phase *Phase) Delete(context db.DB) {
	common.DealWithErr(context.Delete(phase.Key()))
}

// Key returns the storage key for the receiver.
func (phase *Phase) Key() []byte {
	return common.JoinBytes([]byte{phaseKeyPrefix}, phase.Id.Bytes())
}

// Data ABI-encodes the phase record for storage.
func (phase *Phase) Data() []byte {
	return ABIAccelerator.PackVariablePanic(
		PhaseVariableName,
		phase.Id,
		phase.ProjectId,
		phase.Name,
		phase.Description,
		phase.Url,
		phase.ZnnFundsNeeded,
		phase.QsrFundsNeeded,
		phase.CreationTimestamp,
		phase.AcceptedTimestamp,
		phase.Status,
	)
}

func parsePhase(data []byte) *Phase {
	phase := new(Phase)
	ABIAccelerator.UnpackVariablePanic(phase, PhaseVariableName, data)
	return phase
}

// GetProjectList loads the ProjectList record from storage.
func GetProjectList(context db.DB) ([]*Project, error) {
	iterator := context.NewIterator([]byte{projectKeyPrefix})
	defer iterator.Release()
	projectList := make([]*Project, 0)

	for {
		if !iterator.Next() {
			common.DealWithErr(iterator.Error())
			break
		}
		projectList = append(projectList, parseProject(iterator.Value()))
	}

	return projectList, nil
}

// GetProjectEntry loads the ProjectEntry record from storage.
func GetProjectEntry(context db.DB, id types.Hash) (*Project, error) {
	key := (&Project{Id: id}).Key()
	data, err := context.Get(key)
	common.DealWithErr(err)
	if len(data) == 0 {
		return nil, constants.ErrDataNonExistent
	} else {
		return parseProject(data), nil
	}
}

// GetPhaseEntry loads the PhaseEntry record from storage.
func GetPhaseEntry(context db.DB, id types.Hash) (*Phase, error) {
	key := (&Phase{Id: id}).Key()
	data, err := context.Get(key)
	common.DealWithErr(err)
	if len(data) == 0 {
		return nil, constants.ErrDataNonExistent
	} else {
		return parsePhase(data), nil
	}
}
