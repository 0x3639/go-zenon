package account

// Single-byte key prefixes used by the account store. Each lives in its
// own keyspace so iteration over one cannot pick up entries from another.
//
// Note: prefixes 0–2 are reserved by [github.com/zenon-network/go-zenon/common/db]
// for the frontier identifier and the height/hash indexes; this package
// starts at 3.
var (
	// balanceKeyPrefix namespaces the per-token balance cache.
	balanceKeyPrefix = []byte{3}
	// storageKeyPrefix namespaces the embedded-contract storage area
	// surfaced through [accountStore.Storage].
	storageKeyPrefix = []byte{4}
	// chainPlasmaKey holds the per-account chain-plasma counter.
	chainPlasmaKey = []byte{5}
	// receivedBlockPrefix marks send hashes the account has already
	// consumed (used to reject double-receives).
	receivedBlockPrefix = []byte{6}
	// sequencerLastReceivedKey holds the height of the last sequencer
	// entry consumed by an embedded-contract receive.
	sequencerLastReceivedKey = []byte{7}
)

// Receive-status sentinels stored under [receivedBlockPrefix]. The
// stored value is informational only — [accountStore.IsReceived]
// (received.go:26-33) decides "received" purely by key presence: any
// successful Get returns true, leveldb.ErrNotFound returns false.
// Other values are never written by [accountStore.MarkAsReceived].
const (
	// ReceiveStatusUnknown is the implicit value before a send is
	// recorded as received. Equivalent to the key being absent.
	ReceiveStatusUnknown uint64 = iota
	// Received is the sentinel value MarkAsReceived writes; the
	// IsReceived check ignores it and only inspects key presence.
	Received
)
