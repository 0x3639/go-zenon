package discover

import (
	"net"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/crypto"
)

func newTestNodeID(t *testing.T) NodeID {
	t.Helper()
	priv, err := crypto.GenerateKey()
	if err != nil {
		t.Fatal(err)
	}
	return PubkeyID(&priv.PublicKey)
}

// hasEntries reports whether any key is stored for id.
func (db *nodeDB) hasEntries(id NodeID) bool {
	it := db.lvl.NewIterator(nil, nil)
	defer it.Release()
	for it.Next() {
		if kid, _ := splitKey(it.Key()); kid == id {
			return true
		}
	}
	return false
}

// A ping that is never answered leaves timing metadata without a node
// record. Expiration must reclaim such entries the same way it reclaims
// stale node records, while leaving recently seen nodes alone.
func TestExpireNodesReclaimsEntriesWithoutNodeRecord(t *testing.T) {
	self := newTestNodeID(t)
	db, err := newNodeDB("", Version, self)
	if err != nil {
		t.Fatal(err)
	}
	defer db.close()

	stale := time.Now().Add(-2 * nodeDBNodeExpiration)
	fresh := time.Now()

	unanswered := newTestNodeID(t)
	if err := db.updateLastPing(unanswered, stale); err != nil {
		t.Fatal(err)
	}
	bonded := newTestNodeID(t)
	if err := db.updateNode(newNode(bonded, net.IPv4(10, 0, 0, 2), 30303, 30303)); err != nil {
		t.Fatal(err)
	}
	if err := db.updateLastPong(bonded, fresh); err != nil {
		t.Fatal(err)
	}
	gone := newTestNodeID(t)
	if err := db.updateNode(newNode(gone, net.IPv4(10, 0, 0, 3), 30303, 30303)); err != nil {
		t.Fatal(err)
	}
	if err := db.updateLastPong(gone, stale); err != nil {
		t.Fatal(err)
	}

	if err := db.expireNodes(); err != nil {
		t.Fatal(err)
	}
	if db.hasEntries(unanswered) {
		t.Error("entries for the unanswered identity were not reclaimed")
	}
	if !db.hasEntries(bonded) {
		t.Error("entries for the recently seen node were removed")
	}
	if db.hasEntries(gone) {
		t.Error("entries for the stale node were not reclaimed")
	}
}
