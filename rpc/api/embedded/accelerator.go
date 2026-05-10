package embedded

import (
	"encoding/json"
	"math/big"
	"sort"

	"github.com/inconshreveable/log15"

	"github.com/zenon-network/go-zenon/chain"
	"github.com/zenon-network/go-zenon/common"
	"github.com/zenon-network/go-zenon/common/types"
	"github.com/zenon-network/go-zenon/rpc/api"
	"github.com/zenon-network/go-zenon/vm/constants"
	"github.com/zenon-network/go-zenon/vm/embedded/definition"
	"github.com/zenon-network/go-zenon/vm/vm_context"
	"github.com/zenon-network/go-zenon/zenon"
)

// AcceleratorApi is the "embedded.accelerator" namespace — read
// access to project / phase funding queries on the accelerator
// contract.
type AcceleratorApi struct {
	chain chain.Chain
	log   log15.Logger
}

// NewAcceleratorApi constructs the accelerator namespace handler.
func NewAcceleratorApi(z zenon.Zenon) *AcceleratorApi {
	return &AcceleratorApi{
		chain: z.Chain(),
		log:   common.RPCLogger.New("module", "embedded_accelerator_api"),
	}
}

// toProject expands a storage-layer Project into the wire-form
// [Project], eagerly resolving phase entries and vote breakdowns.
// Skips phases whose entries fail to load (logged at the storage
// layer).
func (a *AcceleratorApi) toProject(context vm_context.AccountVmContext, abiProject *definition.Project) *Project {
	project := &Project{
		Id:                  abiProject.Id,
		Owner:               abiProject.Owner,
		Name:                abiProject.Name,
		Description:         abiProject.Description,
		Url:                 abiProject.Url,
		ZnnFundsNeeded:      abiProject.ZnnFundsNeeded,
		QsrFundsNeeded:      abiProject.QsrFundsNeeded,
		CreationTimestamp:   abiProject.CreationTimestamp,
		LastUpdateTimestamp: abiProject.LastUpdateTimestamp,
		Status:              abiProject.Status,
		PhaseIds:            abiProject.PhaseIds,
		Votes:               definition.GetVoteBreakdown(context.Storage(), abiProject.Id),
		Phases:              make([]*Phase, len(abiProject.PhaseIds)),
	}

	for index, id := range abiProject.PhaseIds {
		phase, err := definition.GetPhaseEntry(context.Storage(), id)
		if err != nil {
			continue
		}
		project.Phases[index] = &Phase{
			Phase: phase,
			Votes: definition.GetVoteBreakdown(context.Storage(), phase.Id),
		}
	}

	return project
}

// Phase is part of the package's public API; see the surrounding code for usage.
type Phase struct {
	Phase *definition.Phase         `json:"phase"`
	Votes *definition.VoteBreakdown `json:"votes"`
}

// Project is part of the package's public API; see the surrounding code for usage.
type Project struct {
	Id                  types.Hash                `json:"id"`
	Owner               types.Address             `json:"owner"`
	Name                string                    `json:"name"`
	Description         string                    `json:"description"`
	Url                 string                    `json:"url"`
	ZnnFundsNeeded      *big.Int                  `json:"znnFundsNeeded"`
	QsrFundsNeeded      *big.Int                  `json:"qsrFundsNeeded"`
	CreationTimestamp   int64                     `json:"creationTimestamp"`
	LastUpdateTimestamp int64                     `json:"lastUpdateTimestamp"`
	Status              uint8                     `json:"status"`
	PhaseIds            []types.Hash              `json:"phaseIds"`
	Votes               *definition.VoteBreakdown `json:"votes"`
	Phases              []*Phase                  `json:"phases"`
}

// ProjectMarshal is part of the package's public API; see the surrounding code for usage.
type ProjectMarshal struct {
	Id                  types.Hash                `json:"id"`
	Owner               types.Address             `json:"owner"`
	Name                string                    `json:"name"`
	Description         string                    `json:"description"`
	Url                 string                    `json:"url"`
	ZnnFundsNeeded      string                    `json:"znnFundsNeeded"`
	QsrFundsNeeded      string                    `json:"qsrFundsNeeded"`
	CreationTimestamp   int64                     `json:"creationTimestamp"`
	LastUpdateTimestamp int64                     `json:"lastUpdateTimestamp"`
	Status              uint8                     `json:"status"`
	PhaseIds            []types.Hash              `json:"phaseIds"`
	Votes               *definition.VoteBreakdown `json:"votes"`
	Phases              []*Phase                  `json:"phases"`
}

// ToProjectMarshal projects the receiver to its JSON-friendly ProjectMarshal twin.
func (p *Project) ToProjectMarshal() *ProjectMarshal {
	aux := &ProjectMarshal{
		Id:                  p.Id,
		Owner:               p.Owner,
		Name:                p.Name,
		Description:         p.Description,
		Url:                 p.Url,
		ZnnFundsNeeded:      p.ZnnFundsNeeded.String(),
		QsrFundsNeeded:      p.QsrFundsNeeded.String(),
		CreationTimestamp:   p.CreationTimestamp,
		LastUpdateTimestamp: p.LastUpdateTimestamp,
		Status:              p.Status,
		PhaseIds:            nil,
		Votes:               p.Votes,
		Phases:              nil,
	}
	aux.PhaseIds = make([]types.Hash, len(p.PhaseIds))
	for idx, phaseId := range p.PhaseIds {
		aux.PhaseIds[idx] = phaseId
	}

	aux.Phases = make([]*Phase, len(p.Phases))
	for idx, phase := range p.Phases {
		aux.Phases[idx] = phase
	}
	return aux
}

// MarshalJSON forwards through the Marshal twin so big.Int fields render as decimal strings.
func (p *Project) MarshalJSON() ([]byte, error) {
	return json.Marshal(p.ToProjectMarshal())
}

// UnmarshalJSON inflates the JSON wire form back into the in-memory receiver.
func (p *Project) UnmarshalJSON(data []byte) error {
	aux := new(ProjectMarshal)
	if err := json.Unmarshal(data, aux); err != nil {
		return err
	}
	p.Id = aux.Id
	p.Owner = aux.Owner
	p.Name = aux.Name
	p.Description = aux.Description
	p.Url = aux.Url
	p.ZnnFundsNeeded = common.StringToBigInt(aux.ZnnFundsNeeded)
	p.QsrFundsNeeded = common.StringToBigInt(aux.QsrFundsNeeded)
	p.CreationTimestamp = aux.CreationTimestamp
	p.LastUpdateTimestamp = aux.LastUpdateTimestamp
	p.Status = aux.Status
	p.PhaseIds = make([]types.Hash, len(aux.PhaseIds))
	for idx, phaseId := range aux.PhaseIds {
		p.PhaseIds[idx] = phaseId
	}
	p.Votes = aux.Votes
	p.Phases = make([]*Phase, len(p.Phases))
	for idx, phase := range aux.Phases {
		p.Phases[idx] = phase
	}
	return nil
}

// ProjectList is part of the package's public API; see the surrounding code for usage.
type ProjectList struct {
	Count int        `json:"count"`
	List  []*Project `json:"list"`
}

// === Getters for projects ===

// GetAll returns the full project list, sorted by descending
// LastUpdateTimestamp and sliced to the requested page. Composes
// definition.GetProjectList plus per-project [toProject] expansion;
// pagination is applied after the full list materializes.
func (a *AcceleratorApi) GetAll(pageIndex, pageSize uint32) (*ProjectList, error) {
	_, context, err := api.GetFrontierContext(a.chain, types.AcceleratorContract)
	if err != nil {
		return nil, err
	}

	projects, err := definition.GetProjectList(context.Storage())
	if err != nil {
		return nil, err
	}

	sort.SliceStable(projects, func(i, j int) bool {
		return projects[i].LastUpdateTimestamp > projects[j].LastUpdateTimestamp
	})

	result := &ProjectList{
		Count: len(projects),
		List:  make([]*Project, len(projects)),
	}

	for index, project := range projects {
		result.List[index] = a.toProject(context, project)
	}

	start, end := api.GetRange(pageIndex, pageSize, uint32(len(result.List)))
	result.List = result.List[start:end]

	return result, nil
}

// GetProjectById returns the project with the given id, expanded
// through [toProject] so the response carries phase entries and
// vote breakdowns inline.
func (a *AcceleratorApi) GetProjectById(id types.Hash) (*Project, error) {
	_, context, err := api.GetFrontierContext(a.chain, types.AcceleratorContract)
	if err != nil {
		return nil, err
	}

	project, err := definition.GetProjectEntry(context.Storage(), id)
	if err != nil {
		return nil, err
	}
	return a.toProject(context, project), nil
}

// GetPhaseById returns one phase by id, paired with its vote
// breakdown. Composes definition.GetPhaseEntry +
// definition.GetVoteBreakdown.
func (a *AcceleratorApi) GetPhaseById(id types.Hash) (*Phase, error) {
	_, context, err := api.GetFrontierContext(a.chain, types.AcceleratorContract)
	if err != nil {
		return nil, err
	}

	phase, err := definition.GetPhaseEntry(context.Storage(), id)
	if err != nil {
		return nil, err
	}
	return &Phase{
		Phase: phase,
		Votes: definition.GetVoteBreakdown(context.Storage(), phase.Id),
	}, nil
}

// GetVoteBreakdown returns the aggregated yes/no/abstain counts for a
// project or phase. Returns [constants.ErrDataNonExistent] when no
// vote record exists.
func (a *AcceleratorApi) GetVoteBreakdown(id types.Hash) (*definition.VoteBreakdown, error) {
	_, context, err := api.GetFrontierContext(a.chain, types.AcceleratorContract)
	if err != nil {
		return nil, err
	}
	voteBreakdown := definition.GetVoteBreakdown(context.Storage(), id)
	if voteBreakdown == nil {
		return nil, constants.ErrDataNonExistent
	}
	return voteBreakdown, nil
}

// GetPillarVotes returns the vote cast by pillar `name` on each
// project/phase id in `hashes`. The result slice is index-aligned with
// `hashes`; positions where the pillar has not voted are nil rather
// than an error.
func (a *AcceleratorApi) GetPillarVotes(name string, hashes []types.Hash) ([]*definition.PillarVote, error) {
	_, context, err := api.GetFrontierContext(a.chain, types.AcceleratorContract)
	if err != nil {
		return nil, err
	}
	result := make([]*definition.PillarVote, len(hashes))
	for index := range hashes {
		vote, err := definition.GetPillarVote(context.Storage(), hashes[index], name)
		if err == constants.ErrDataNonExistent {
			result[index] = nil
		} else if err != nil {
			return nil, err
		} else {
			result[index] = vote
		}
	}
	return result, nil
}
