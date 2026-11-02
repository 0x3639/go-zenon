package libp2p

import (
	"testing"
	"time"

	"github.com/zenon-network/go-zenon/p2p"
)

// The disconnect frame is written under discWriteTimeout, not under
// WriteMsg's frameWriteTimeout: against a stream that accepts no writes,
// a locally requested disconnect ends the peer within about a second.
func TestDisconnectWriteHonoursDiscWriteTimeout(t *testing.T) {
	s := newHeldStream()
	p, done := runPongTestPeer(t, s)

	start := time.Now()
	go p.Disconnect(p2p.DiscQuitting)
	expectEntered(t, s.entered, discMsg)
	expectPeerReasonWithin(t, done, p2p.DiscQuitting, start, discWriteTimeout+time.Second)
}

// A write already stalled on the stream when the peer is dropped holds
// the write lock the disconnect frame needs. The disconnect deadline cuts
// that write short too, so the stalled write and the disconnect frame
// together still end within about discWriteTimeout.
func TestDisconnectDeadlineCutsShortStalledWrite(t *testing.T) {
	s := newHeldStream()
	p, done := runPongTestPeer(t, s)

	s.deliver(t, pingMsg)
	expectEntered(t, s.entered, pongMsg) // held with frameWriteTimeout

	start := time.Now()
	go p.Disconnect(p2p.DiscQuitting)
	expectEntered(t, s.entered, discMsg)
	expectPeerReasonWithin(t, done, p2p.DiscQuitting, start, discWriteTimeout+time.Second)
}

func expectPeerReasonWithin(t *testing.T, done <-chan p2p.DiscReason, want p2p.DiscReason, start time.Time, limit time.Duration) {
	t.Helper()
	select {
	case got := <-done:
		if got != want {
			t.Fatalf("run returned %v, want %v", got, want)
		}
		if elapsed := time.Since(start); elapsed > limit {
			t.Fatalf("run returned after %v, want within %v", elapsed, limit)
		}
	case <-time.After(limit):
		t.Fatalf("run did not return within %v", limit)
	}
}
