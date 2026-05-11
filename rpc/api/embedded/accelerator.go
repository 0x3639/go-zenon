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

// AcceleratorApi serves read RPCs for the accelerator contract —
// the on-chain project-funding mechanism where pillars vote on
// project phases for ZNN/QSR grants.
type AcceleratorApi struct {
	chain chain.Chain
	log   log15.Logger
}

// NewAcceleratorApi returns an AcceleratorApi bound to z's chain.
// Accelerator reads do not need a consensus handle.
func NewAcceleratorApi(z zenon.Zenon) *AcceleratorApi {
	return &AcceleratorApi{
		chain: z.Chain(),
		log:   common.RPCLogger.New("module", "embedded_accelerator_api"),
	}
}

// toProject hydrates an on-chain definition.Project into the RPC
// Project view: it copies the static fields, attaches a fresh
// VoteBreakdown for the project itself, and walks PhaseIds to
// build the Phase slice (each with its own VoteBreakdown).
// definition.GetPhaseEntry failures for individual phase ids are
// swallowed and the corresponding Phase slot is left nil.
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

// Phase is the RPC view of one project phase: the on-chain phase
// record paired with the current vote breakdown across pillars.
type Phase struct {
	Phase *definition.Phase         `json:"phase"`
	Votes *definition.VoteBreakdown `json:"votes"`
}

// Project is the RPC view of one accelerator project. Funding
// amounts are *big.Int for in-process precision; the JSON form
// encodes them as decimal strings via ProjectMarshal. Votes
// counts the pillar votes on the project itself, while each entry
// in Phases carries its own VoteBreakdown for that phase.
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

// ProjectMarshal mirrors Project with the *big.Int funding amounts
// encoded as decimal strings for JSON precision safety.
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

// ToProjectMarshal converts p into its string-amount wire form.
// The PhaseIds and Phases slices are copied (not aliased) so
// callers can mutate the marshal value without affecting p.
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

// MarshalJSON renders p through ProjectMarshal so funding amounts
// are emitted as decimal strings.
func (p *Project) MarshalJSON() ([]byte, error) {
	return json.Marshal(p.ToProjectMarshal())
}

// UnmarshalJSON reads a ProjectMarshal payload and rehydrates the
// *big.Int funding fields via common.StringToBigInt.
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

// ProjectList is the paged response shape for project enumeration.
// Count is the full pre-paging total so clients can compute the
// global page count.
type ProjectList struct {
	Count int        `json:"count"`
	List  []*Project `json:"list"`
}

// === Getters for projects ===

// GetAll returns one page of every accelerator project, sorted
// stably by descending LastUpdateTimestamp (sort.SliceStable so
// projects sharing a timestamp retain their definition-order
// position). Each Project entry is fully hydrated with vote
// breakdowns and phase data via toProject.
//
// GetAll does not pre-validate page bounds beyond what
// api.GetRange enforces — there is no api.ErrPageSizeParamTooBig
// guard here. Callers using untrusted page sizes should validate
// upstream.
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

// GetProjectById looks up one accelerator project by its id and
// returns the fully-hydrated RPC view (project record + vote
// breakdown + phases each with their own vote breakdown). Storage
// errors propagate unchanged.
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

// GetPhaseById looks up one phase by its id and returns the
// phase record plus its current pillar VoteBreakdown.
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

// GetVoteBreakdown returns the pillar vote breakdown for the
// given project or phase id, or constants.ErrDataNonExistent
// when the underlying definition lookup returns no breakdown.
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

// GetPillarVotes returns the individual votes the named pillar
// has cast on each of the supplied project/phase hashes, returned
// in the same order as the input. A nil slot in the result means
// the pillar has not voted on that hash
// (constants.ErrDataNonExistent from the lookup is mapped to nil).
// Other storage errors abort the scan and propagate unchanged.
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
