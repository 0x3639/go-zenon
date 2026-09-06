package discover

import (
	"crypto/ecdsa"
	"io"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/crypto"
)

// fakeConn is a datagram socket that never delivers anything: reads block
// until Close and writes are recorded by destination and packet type. Test
// packets are fed to the real decoder through udp.handlePacket instead.
//
// When release is set, outbound pings park in WriteToUDP until it is closed.
// Only bonding processes send pings, so this holds every admitted bond at a
// known point, with its slots and permits taken, until the test lets go.
type fakeConn struct {
	mu        sync.Mutex
	closed    chan struct{}
	closeOnce sync.Once
	sent      map[string][]byte // destination -> packet types written
	release   chan struct{}     // nil when pings are not held
}

func newFakeConn(holdPings bool) *fakeConn {
	c := &fakeConn{closed: make(chan struct{}), sent: make(map[string][]byte)}
	if holdPings {
		c.release = make(chan struct{})
	}
	return c
}

func (c *fakeConn) ReadFromUDP(b []byte) (int, *net.UDPAddr, error) {
	<-c.closed
	return 0, nil, io.EOF
}

func (c *fakeConn) WriteToUDP(b []byte, addr *net.UDPAddr) (int, error) {
	if b[headSize] == pingPacket && c.release != nil {
		select {
		case <-c.release:
		case <-c.closed:
			return 0, io.ErrClosedPipe
		}
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.sent[addr.String()] = append(c.sent[addr.String()], b[headSize])
	return len(b), nil
}

func (c *fakeConn) Close() error {
	c.closeOnce.Do(func() { close(c.closed) })
	return nil
}

func (c *fakeConn) LocalAddr() net.Addr {
	return &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 30303}
}

// count returns how many packets of the given type were written to addr.
func (c *fakeConn) count(addr *net.UDPAddr, ptype byte) int {
	c.mu.Lock()
	defer c.mu.Unlock()
	n := 0
	for _, p := range c.sent[addr.String()] {
		if p == ptype {
			n++
		}
	}
	return n
}

func (c *fakeConn) countAll(ptype byte) int {
	c.mu.Lock()
	defer c.mu.Unlock()
	n := 0
	for _, ps := range c.sent {
		for _, p := range ps {
			if p == ptype {
				n++
			}
		}
	}
	return n
}

type testUDP struct {
	tab  *Table
	udp  *udp
	conn *fakeConn
}

// newTestUDP starts a transport on a fake socket. With holdPings set,
// outbound pings are parked until u.release is called.
func newTestUDP(t *testing.T, holdPings bool) *testUDP {
	t.Helper()
	priv, err := crypto.GenerateKey()
	if err != nil {
		t.Fatal(err)
	}
	conn := newFakeConn(holdPings)
	tab, u := newUDP(priv, conn, nil, "")
	return &testUDP{tab: tab, udp: u, conn: conn}
}

// release lets parked outbound pings through.
func (u *testUDP) release() {
	close(u.conn.release)
}

// admitted returns how many inbound bonds currently hold a permit.
func (u *testUDP) admitted() int {
	u.tab.bondmu.Lock()
	defer u.tab.bondmu.Unlock()
	return len(u.tab.inbound)
}

// freeSlots returns how many bonding slots are not held by a process.
func (u *testUDP) freeSlots() int {
	return len(u.tab.bondslots)
}

// waitFor polls cond until it holds or the deadline passes. It expresses
// conditions that are reached eventually rather than at a fixed delay.
func (u *testUDP) waitFor(t *testing.T, timeout time.Duration, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatalf("%s: not reached within %v", what, timeout)
		}
		time.Sleep(2 * time.Millisecond)
	}
}

// sender is a remote discovery identity with its own key and address.
type sender struct {
	priv *ecdsa.PrivateKey
	addr *net.UDPAddr
}

func newSender(t *testing.T, port int) *sender {
	t.Helper()
	priv, err := crypto.GenerateKey()
	if err != nil {
		t.Fatal(err)
	}
	return &sender{priv: priv, addr: &net.UDPAddr{IP: net.IPv4(10, 0, 0, 1), Port: port}}
}

func (s *sender) id() NodeID {
	return PubkeyID(&s.priv.PublicKey)
}

// deliver signs a packet from s and feeds it to the real decoder.
func (s *sender) deliver(t *testing.T, u *testUDP, ptype byte, req interface{}) {
	t.Helper()
	packet, err := encodePacket(s.priv, ptype, req)
	if err != nil {
		t.Fatal(err)
	}
	if err := u.udp.handlePacket(s.addr, packet); err != nil {
		t.Fatalf("packet type %d from %v rejected: %v", ptype, s.addr, err)
	}
}

// ping delivers a valid, unexpired, correctly versioned ping from s.
func (s *sender) ping(t *testing.T, u *testUDP) {
	t.Helper()
	s.deliver(t, u, pingPacket, &ping{
		Version:    Version,
		From:       makeEndpoint(s.addr, uint16(s.addr.Port)),
		To:         makeEndpoint(u.conn.LocalAddr().(*net.UDPAddr), 30303),
		Expiration: uint64(time.Now().Add(expiration).Unix()),
	})
}

// answer waits for the ping our bonding process sends to s and delivers
// the pong that completes the exchange.
func (s *sender) answer(t *testing.T, u *testUDP) {
	t.Helper()
	u.waitFor(t, 5*time.Second, "ping sent to "+s.addr.String(), func() bool {
		return u.conn.count(s.addr, pingPacket) > 0
	})
	s.deliver(t, u, pongPacket, &pong{
		To:         makeEndpoint(u.conn.LocalAddr().(*net.UDPAddr), 30303),
		Expiration: uint64(time.Now().Add(expiration).Unix()),
	})
}

func (u *testUDP) bondingLen() int {
	u.tab.bondmu.Lock()
	defer u.tab.bondmu.Unlock()
	return len(u.tab.bonding)
}

// waitDone waits for every goroutine the transport started, or fails.
func (u *testUDP) waitDone(t *testing.T, timeout time.Duration) {
	t.Helper()
	done := make(chan struct{})
	go func() {
		u.udp.wg.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(timeout):
		t.Fatalf("transport goroutines still running %v after close", timeout)
	}
}

// Every valid ping is answered with a pong, but bonding work is only started
// for as many identities as the inbound budget allows. Admission happens in
// the handler itself, so the boundary is exact: the permit count is checked
// right after the pings are delivered, before any bonding process has run.
func TestInboundBondingIsBounded(t *testing.T) {
	u := newTestUDP(t, true)
	defer u.tab.Close()

	const extra = 50
	senders := make([]*sender, maxInboundBonds+extra)
	for i := range senders {
		senders[i] = newSender(t, 40000+i)
	}
	for i, s := range senders {
		s.ping(t, u)
		if want := i + 1; want <= maxInboundBonds && u.admitted() != want {
			t.Fatalf("%d permits held after %d pings", u.admitted(), want)
		}
	}
	if got := u.conn.countAll(pongPacket); got != len(senders) {
		t.Fatalf("%d pongs sent for %d pings", got, len(senders))
	}
	if got := u.admitted(); got != maxInboundBonds {
		t.Fatalf("%d permits held, limit is %d", got, maxInboundBonds)
	}
	// Every admitted process registers itself and then parks on the held
	// ping or on a slot. Processes beyond the budget were never started.
	u.waitFor(t, 5*time.Second, "admitted bonds registered", func() bool {
		return u.bondingLen() == maxInboundBonds
	})
	for _, s := range senders[maxInboundBonds:] {
		if u.conn.count(s.addr, pingPacket) != 0 {
			t.Fatalf("rejected sender %v was bonded", s.addr)
		}
	}
}

// Repeated pings from one identity coalesce onto the bond already in flight
// for it, so they take one permit rather than the whole budget: a different
// identity that pings while the noisy one is still bonding is admitted and
// bonded as usual.
func TestRepeatedIdentityDoesNotStarveOthers(t *testing.T) {
	u := newTestUDP(t, true)
	defer u.tab.Close()

	noisy := newSender(t, 40000)
	for i := 0; i < maxInboundBonds*3; i++ {
		noisy.ping(t, u)
	}
	if got := u.admitted(); got != 1 {
		t.Fatalf("%d permits held for one identity", got)
	}
	other := newSender(t, 40001)
	other.ping(t, u)
	if got := u.admitted(); got != 2 {
		t.Fatalf("%d permits held for two identities", got)
	}
	u.waitFor(t, 5*time.Second, "one bonding process per identity", func() bool {
		return u.bondingLen() == 2
	})

	u.release()
	other.answer(t, u)
	u.waitFor(t, 5*time.Second, "other identity bonded", func() bool {
		return u.tab.db.node(other.id()) != nil
	})
}

// Permits and slots are returned whether an exchange succeeds or times out,
// so a full budget drains completely and a later ping is served again.
func TestInboundBondingBudgetIsReturned(t *testing.T) {
	u := newTestUDP(t, true)
	defer u.tab.Close()

	senders := make([]*sender, maxInboundBonds)
	for i := range senders {
		senders[i] = newSender(t, 40000+i)
		senders[i].ping(t, u)
	}
	u.release()
	// Even senders answer and bond; odd senders stay silent and time out.
	for i := 0; i < len(senders); i += 2 {
		senders[i].answer(t, u)
	}
	u.waitFor(t, 10*time.Second, "all permits returned", func() bool {
		return u.admitted() == 0
	})
	u.waitFor(t, 5*time.Second, "all slots returned", func() bool {
		return u.freeSlots() == maxBondingPingPongs
	})
	for i, s := range senders {
		if bonded := u.tab.db.node(s.id()) != nil; bonded != (i%2 == 0) {
			t.Fatalf("sender %d bonded=%v", i, bonded)
		}
	}

	late := newSender(t, 50000)
	late.ping(t, u)
	if got := u.admitted(); got != 1 {
		t.Fatalf("%d permits held after the budget drained", got)
	}
	late.answer(t, u)
	u.waitFor(t, 5*time.Second, "late sender bonded", func() bool {
		return u.tab.db.node(late.id()) != nil
	})
}

// Outbound bonding shares the bonding slots but not the inbound budget: with
// the budget full and every slot held by an inbound exchange, an outbound
// bond still completes once slots turn over.
func TestOutboundBondProgressesUnderInboundSaturation(t *testing.T) {
	u := newTestUDP(t, true)
	defer u.tab.Close()

	senders := make([]*sender, maxInboundBonds)
	for i := range senders {
		senders[i] = newSender(t, 40000+i)
		senders[i].ping(t, u)
	}
	u.waitFor(t, 5*time.Second, "every slot held", func() bool {
		return u.freeSlots() == 0
	})

	peer := newSender(t, 50000)
	type outcome struct {
		n   *Node
		err error
	}
	done := make(chan outcome, 1)
	go func() {
		n, err := u.tab.bond(false, peer.id(), peer.addr, uint16(peer.addr.Port))
		done <- outcome{n, err}
	}()
	if got := u.admitted(); got != maxInboundBonds {
		t.Fatalf("%d permits held, outbound bonding must not take one", got)
	}

	// The inbound remotes stay silent, so slots turn over as their
	// exchanges time out and the outbound bond takes one in its turn.
	u.release()
	peer.answer(t, u)
	// An outbound bond then waits for the peer to ping us back.
	peer.ping(t, u)
	select {
	case o := <-done:
		if o.err != nil || o.n == nil {
			t.Fatalf("outbound bond failed: node=%v err=%v", o.n, o.err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("outbound bond did not complete")
	}
}

// Closing the table must release bonds waiting for a slot; they may not stay
// parked forever behind slots that will never be returned. Close does not
// join the workers itself, so this checks that they all exit afterwards.
func TestQueuedBondsExitOnClose(t *testing.T) {
	u := newTestUDP(t, true)

	for i := 0; i < maxInboundBonds; i++ {
		newSender(t, 40000+i).ping(t, u)
	}
	// Every process has started: each holds a slot at the held ping or is
	// waiting for one.
	u.waitFor(t, 5*time.Second, "every bond parked", func() bool {
		return u.bondingLen() == maxInboundBonds && u.freeSlots() == 0
	})
	u.tab.Close()
	u.waitDone(t, 5*time.Second)
}
