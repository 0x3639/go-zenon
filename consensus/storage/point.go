package storage

import (
	"encoding/json"
	"math/big"

	"github.com/pkg/errors"
	"google.golang.org/protobuf/proto"

	"github.com/zenon-network/go-zenon/common/types"
)

// ProducerDetail is one pillar's per-tick performance: how many blocks
// it was expected to produce, how many it actually produced, and its
// weight at the tick. Aggregated across ticks via [ProducerDetail.Merge]
// to roll period points up into epoch points.
type ProducerDetail struct {
	ExpectedNum uint32
	FactualNum  uint32
	Weight      *big.Int
}

// Copy returns a deep copy of detail; weight is independently
// allocated so subsequent arithmetic on the copy does not affect the
// original.
func (detail ProducerDetail) Copy() *ProducerDetail {
	return &ProducerDetail{
		ExpectedNum: detail.ExpectedNum,
		FactualNum:  detail.FactualNum,
		Weight:      new(big.Int).Set(detail.Weight),
	}
}

// Merge folds c into detail in place: counts are summed and the
// weight is added. Used by [Point.LeftAppend] to combine per-period
// records into epoch records.
func (detail *ProducerDetail) Merge(c *ProducerDetail) {
	detail.ExpectedNum = detail.ExpectedNum + c.ExpectedNum
	detail.FactualNum = detail.FactualNum + c.FactualNum
	detail.Weight.Add(detail.Weight, c.Weight)
}

// AddNum adjusts only the produced/expected counts on detail without
// touching weight. Used while building a per-period [Point] from the
// election result and the actual block content.
func (detail *ProducerDetail) AddNum(ExpectedNum uint32, FactualNum uint32) {
	detail.ExpectedNum = detail.ExpectedNum + ExpectedNum
	detail.FactualNum = detail.FactualNum + FactualNum
}

// Point is the per-tick performance snapshot: the (prev, end) hash
// pair bracketing the chain range it covers, every pillar's
// [ProducerDetail], and the cumulative weight of the elected slate.
type Point struct {
	// PrevHash is the last hash that is not in this point — the
	// momentum immediately before the point's window.
	PrevHash types.Hash
	// EndHash is the last hash that is in this point — the momentum
	// at the end of the window.
	EndHash     types.Hash
	Pillars     map[string]*ProducerDetail
	TotalWeight *big.Int
}

// Json renders p as JSON for ad-hoc inspection. Errors silently
// produce an empty string.
func (p *Point) Json() string {
	bytes, _ := json.Marshal(p)
	return string(bytes)
}

// Marshal encodes p as protobuf bytes for persistence.
func (p *Point) Marshal() ([]byte, error) {
	pb := &ConsensusPointProto{}
	pb.EndHash = p.EndHash.Bytes()
	pb.PrevHash = p.PrevHash.Bytes()
	pb.TotalWeight = p.TotalWeight.Bytes()
	if len(p.Pillars) > 0 {
		pb.Content = make([]*ProducerDetailProto, 0, len(p.Pillars))
		for k, v := range p.Pillars {
			c := &ProducerDetailProto{}
			c.Name = k
			c.ExpectedNum = v.ExpectedNum
			c.FactualNum = v.FactualNum
			c.Weight = v.Weight.Bytes()
			pb.Content = append(pb.Content, c)
		}
	}
	buf, err := proto.Marshal(pb)
	if err != nil {
		return nil, err
	}
	return buf, nil
}

// Unmarshal decodes buf back into p.
func (p *Point) Unmarshal(buf []byte) error {
	pb := &ConsensusPointProto{}

	if unmarshalErr := proto.Unmarshal(buf, pb); unmarshalErr != nil {
		return unmarshalErr
	}

	if len(pb.EndHash) > 0 {
		if err := p.EndHash.SetBytes(pb.EndHash); err != nil {
			return err
		}
	}

	if len(pb.PrevHash) > 0 {
		if err := p.PrevHash.SetBytes(pb.PrevHash); err != nil {
			return err
		}
	}

	p.TotalWeight = big.NewInt(0).SetBytes(pb.TotalWeight)
	p.Pillars = make(map[string]*ProducerDetail, len(pb.Content))
	for _, v := range pb.Content {
		p.Pillars[v.Name] = &ProducerDetail{ExpectedNum: v.ExpectedNum, FactualNum: v.FactualNum, Weight: big.NewInt(0).SetBytes(v.Weight)}
	}
	return nil
}

// LeftAppend extends p backwards to also cover left's window.
// Requires `left.EndHash == p.PrevHash` (the windows must abut);
// returns an error otherwise. After the merge p.PrevHash points at
// left.PrevHash and pillar counts/weights are summed.
func (p *Point) LeftAppend(left *Point) error {
	if left.EndHash != p.PrevHash {
		return errors.Errorf("failed to merge consensus points. LeftPoint is [%v,%v) and RightPoint is [%v,%v)", left.PrevHash, left.EndHash, p.PrevHash, p.EndHash)
	}

	p.PrevHash = left.PrevHash
	p.TotalWeight.Add(p.TotalWeight, left.TotalWeight)

	for k, v := range left.Pillars {
		c, ok := p.Pillars[k]
		if !ok {
			p.Pillars[k] = v.Copy()
		} else {
			c.Merge(v)
		}
	}

	return nil
}

// IsEmpty reports whether p covers an empty chain range
// (PrevHash == EndHash, e.g., a tick with no blocks).
func (p *Point) IsEmpty() bool {
	return p.EndHash == p.PrevHash
}

// NewEmptyPoint returns a fresh point pinned at proofHash on both
// sides — used as the seed to which per-block contributions are
// appended while building a per-period or per-epoch point.
func NewEmptyPoint(proofHash types.Hash) *Point {
	return &Point{
		PrevHash:    proofHash,
		EndHash:     proofHash,
		Pillars:     make(map[string]*ProducerDetail),
		TotalWeight: big.NewInt(0),
	}
}
