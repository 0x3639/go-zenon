package mailbox

// Single-byte key prefixes used by the mailbox store. Each lives in its
// own keyspace so iteration over one cannot pick up entries from another.
//
// Note: prefixes 0–2 are reserved by [github.com/zenon-network/go-zenon/common/db]
// for the frontier identifier and the height/hash indexes; this package
// starts at 4 to leave a small gap for future common-db growth.
var (
	// unreceivedBlockPrefix marks every send hash that has been admitted
	// to this account's mailbox awaiting consumption (the cumulative
	// historical record).
	unreceivedBlockPrefix = []byte{4}
	// pendingBlockPrefix marks every send hash currently still pending
	// consumption (subset of unreceivedBlockPrefix; entries are removed
	// as they are received).
	pendingBlockPrefix = []byte{5}
	// blockWhichReceives maps a send hash to the [types.AccountHeader] of
	// the receive block that consumed it, used to answer
	// [chain/store.Momentum.GetBlockWhichReceives] in O(1).
	blockWhichReceives = []byte{6}
	// sequencerNumInsertedKey holds the running total of sends pushed
	// onto the sequencer queue.
	sequencerNumInsertedKey = []byte{7}
	// sequencerHeaderByHeightPrefix maps a sequencer position (1-based)
	// to the [types.AccountHeader] of the send at that position.
	sequencerHeaderByHeightPrefix = []byte{8}
)
