package momentum

// Single-byte key prefixes used by the momentum store. Each lives in its
// own keyspace so iteration over one cannot pick up entries from another.
//
// Prefixes 0–2 are reserved by [github.com/zenon-network/go-zenon/common/db]
// for the frontier identifier and the height/hash indexes. Prefixes 6–7
// are intentionally unused and reserved for future indexes.
var (
	// accountStorePrefix namespaces every account's chain storage.
	// Subset(accountStorePrefix||address) yields one account's
	// [github.com/zenon-network/go-zenon/chain/store.Account] view.
	accountStorePrefix = []byte{3}
	// accountMailboxPrefix namespaces every account's mailbox.
	// Subset(accountMailboxPrefix||address) yields one account's
	// [github.com/zenon-network/go-zenon/chain/account/mailbox] view.
	accountMailboxPrefix = []byte{4}
	// blockConfirmationHeightPrefix maps an account-block hash to the
	// momentum height that confirmed it.
	blockConfirmationHeightPrefix = []byte{5}
	// accountZNNBalancePrefix caches the per-account ZNN balance,
	// duplicated from the per-account store so consensus delegation
	// math can read every backer's balance in O(1).
	accountZNNBalancePrefix = []byte{8}
	// accountHeaderByHashPrefix maps an account-block hash to the
	// serialized [types.AccountHeader] (address + height + hash) so
	// callers can look up a block without knowing its account.
	accountHeaderByHashPrefix = []byte{9}
)
