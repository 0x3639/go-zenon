package verifier

import (
	"github.com/zenon-network/go-zenon/chain"
	"github.com/zenon-network/go-zenon/common"
	"github.com/zenon-network/go-zenon/consensus"
)

// log is the package-level logger; alias of [common.VerifierLogger].
var (
	log = common.VerifierLogger
)

// Verifier is the combined interface every chain insertion goes through:
// account blocks (and their transactions) are validated by
// [AccountBlockVerifier], momentums (and their transactions) by
// [MomentumVerifier]. Validation is split in two passes — see the
// per-interface docs.
type Verifier interface {
	AccountBlockVerifier
	MomentumVerifier
}

// verifier is the canonical [Verifier] implementation, composed of an
// [AccountBlockVerifier] and a [MomentumVerifier] sharing the same chain
// and consensus handles.
type verifier struct {
	AccountBlockVerifier
	MomentumVerifier
}

// NewVerifier constructs a [Verifier] backed by chain reads and the
// consensus producer-validation surface.
func NewVerifier(chain chain.Chain, consensus consensus.Consensus) Verifier {
	return &verifier{
		AccountBlockVerifier: NewAccountBlockVerifier(chain, consensus),
		MomentumVerifier:     NewMomentumVerifier(chain, consensus),
	}
}
