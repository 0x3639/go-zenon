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

// Receive-status sentinels stored under [receivedBlockPrefix]. Values
// other than [Received] are treated as "not received yet"; only the
// presence of a tombstone-marker matters at the lookup site.
const (
	// ReceiveStatusUnknown is the implicit value before a send is
	// recorded as received. Equivalent to the key being absent.
	ReceiveStatusUnknown uint64 = iota
	// Received marks a send hash that the account has consumed.
	Received
)
