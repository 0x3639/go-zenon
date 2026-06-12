package genesis

import "github.com/pkg/errors"

// The Err* sentinels report why a genesis config could not be
// loaded; ReadGenesisConfigFromFile returns the first four and
// MakeEmbeddedGenesisConfig the last. The node treats any of them as
// fatal at startup.
var (
	// ErrInvalidGenesisPath signals that the genesis file could not
	// be opened.
	ErrInvalidGenesisPath = errors.New("can't open genesis file")
	// ErrInvalidGenesisJson signals that the genesis file is not
	// valid JSON for GenesisConfig.
	ErrInvalidGenesisJson = errors.New("malformed genesis json structure")
	// ErrIncompleteGenesisJson signals that the genesis file ended
	// mid-document (decoder reported EOF).
	ErrIncompleteGenesisJson = errors.New("incomplete genesis json")
	// ErrInvalidGenesisConfig signals that the decoded config failed
	// the CheckGenesis consistency checks.
	ErrInvalidGenesisConfig = errors.New("invalid genesis config. Failed to pass tests")

	// ErrNoEmbeddedGenesis signals that this build carries no
	// embedded genesis, so a genesis file must be provided.
	ErrNoEmbeddedGenesis = errors.New("the codebase has no embedded genesis")
)
