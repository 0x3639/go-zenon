package storage

import (
	"math/big"

	"google.golang.org/protobuf/proto"

	"github.com/zenon-network/go-zenon/common/types"
)

// ElectionData is the cached output of one election: the ordered slate
// of producer addresses for a tick plus the delegation snapshot they
// were elected against. The tick is keyed by proof-block hash, not by
// tick number — see [DB.GetElectionResultByHash].
type ElectionData struct {
	Producers   []types.Address
	Delegations []*types.PillarDelegation
}

// Marshal encodes d as protobuf bytes via the auto-generated
// [ElectionDataProto] (defined in the .pb.go sibling file).
func (d *ElectionData) Marshal() ([]byte, error) {
	pb := &ElectionDataProto{}
	pb.Delegations = make([]*PillarDelegationProto, 0, len(d.Delegations))
	for _, el := range d.Delegations {
		pb.Delegations = append(pb.Delegations, &PillarDelegationProto{
			Name:             el.Name,
			ProducingAddress: el.Producing.Bytes(),
			Weight:           el.Weight.Bytes()})
	}

	pb.Producers = make([][]byte, 0, len(d.Producers))
	for _, el := range d.Producers {
		pb.Producers = append(pb.Producers, el.Bytes())
	}

	buf, err := proto.Marshal(pb)
	if err != nil {
		return nil, err
	}
	return buf, nil
}

// Unmarshal decodes buf back into d. Returns an error on protobuf
// shape mismatch or on an [types.Address] of the wrong length.
func (d *ElectionData) Unmarshal(buf []byte) error {
	pb := &ElectionDataProto{}
	if err := proto.Unmarshal(buf, pb); err != nil {
		return err
	}

	d.Delegations = make([]*types.PillarDelegation, 0, len(pb.Delegations))
	for _, p := range pb.Delegations {
		addr, err := types.BytesToAddress(p.ProducingAddress)
		if err != nil {
			return err
		}
		d.Delegations = append(d.Delegations, &types.PillarDelegation{
			Weight:    big.NewInt(0).SetBytes(p.Weight),
			Name:      p.Name,
			Producing: addr},
		)
	}

	d.Producers = make([]types.Address, 0, len(pb.Producers))
	for _, p := range pb.Producers {
		addr, err := types.BytesToAddress(p)
		if err != nil {
			return err
		}
		d.Producers = append(d.Producers, addr)
	}

	return nil
}

// GenElectionData builds an [ElectionData] from the supplied
// producer order and delegation snapshot.
func GenElectionData(producers []types.Address, delegations []*types.PillarDelegation) *ElectionData {
	return &ElectionData{
		Producers:   producers,
		Delegations: delegations,
	}
}
