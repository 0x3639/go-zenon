package db

// Single-byte key prefixes used by [SetFrontier] / [GetFrontierIdentifier]
// and the height/hash indexes. Each prefix lives in its own keyspace so
// scans for one don't pick up others.
var (
	// frontierIdentifierKey stores the [HashHeight] of the most recent
	// commit (the "frontier" of the chain).
	frontierIdentifierKey = []byte{0}
	// heightByHashPrefix maps an entry's hash to its height.
	heightByHashPrefix = []byte{1}
	// entryByHeightPrefix maps an entry's height to its serialized bytes.
	entryByHeightPrefix = []byte{2}
)
