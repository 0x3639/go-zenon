package genesis

import "github.com/pkg/errors"

// Sentinel errors returned by the genesis loader. Callers should branch
// on these rather than parsing error strings.
var (
	// ErrInvalidGenesisPath is returned when the genesis file cannot be
	// opened (missing, unreadable, etc.).
	ErrInvalidGenesisPath = errors.New("can't open genesis file")
	// ErrInvalidGenesisJson is returned when JSON parsing fails for
	// reasons other than truncation.
	ErrInvalidGenesisJson = errors.New("malformed genesis json structure")
	// ErrIncompleteGenesisJson is returned when the file is truncated
	// (decoder reports unexpected EOF / EOF).
	ErrIncompleteGenesisJson = errors.New("incomplete genesis json")
	// ErrInvalidGenesisConfig is returned when [CheckGenesis] rejects
	// the parsed config (balance / fusion / supply mismatch).
	ErrInvalidGenesisConfig = errors.New("invalid genesis config. Failed to pass tests")

	// ErrNoEmbeddedGenesis is returned by [MakeEmbeddedGenesisConfig]
	// when the binary was built without an embedded genesis JSON.
	ErrNoEmbeddedGenesis = errors.New("the codebase has no embedded genesis")
)
