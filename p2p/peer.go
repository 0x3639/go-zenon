// Copyright 2014 The go-ethereum Authors
// This file is part of the go-ethereum library.
//
// The go-ethereum library is free software: you can redistribute it and/or modify
// it under the terms of the GNU Lesser General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// The go-ethereum library is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
// GNU Lesser General Public License for more details.
//
// You should have received a copy of the GNU Lesser General Public License
// along with the go-ethereum library. If not, see <http://www.gnu.org/licenses/>.

package p2p

import (
	"errors"
	"fmt"
	"io"
	"net"
	"sort"
	"sync"
	"time"

	"github.com/ethereum/go-ethereum/rlp"

	"github.com/zenon-network/go-zenon/common"
	"github.com/zenon-network/go-zenon/p2p/discover"
)

const (
	// baseProtocolVersion is the devp2p framing version this node
	// speaks. Negotiated in protoHandshake; mismatch produces
	// DiscIncompatibleVersion.
	baseProtocolVersion = 4
	// baseProtocolLength reserves message-code space for base-protocol
	// messages (0..15). Subprotocol message codes are offset past this.
	baseProtocolLength = uint64(16)
	// baseProtocolMaxMsgSize bounds the size of base-protocol frames
	// (handshake, ping/pong, disconnect). Subprotocols set their own
	// limits.
	baseProtocolMaxMsgSize = 2 * 1024

	// pingInterval is the heartbeat cadence for keeping idle
	// connections alive (must be < frameReadTimeout).
	pingInterval = 15 * time.Second
)

const (
	// handshakeMsg carries the protoHandshake frame exchanged once at
	// connection start.
	handshakeMsg = 0x00
	// discMsg notifies the remote of an impending close with a single
	// DiscReason.
	discMsg = 0x01
	// pingMsg requests a pongMsg reply, used as a keepalive.
	pingMsg = 0x02
	// pongMsg is the reply to pingMsg.
	pongMsg = 0x03
	// getPeersMsg / peersMsg are legacy devp2p peer-exchange codes;
	// they are accepted for compatibility but not actively used.
	getPeersMsg = 0x04
	peersMsg    = 0x05
)

// protoHandshake is the RLP structure exchanged once per connection
// after the encryption handshake completes. Version must match
// baseProtocolVersion; Caps advertises the subprotocols this node
// supports; ID is the node's secp256k1 public key.
type protoHandshake struct {
	Version    uint64
	Name       string
	Caps       []Cap
	ListenPort uint64
	ID         discover.NodeID
}

// Peer represents a connected remote node.
type Peer struct {
	rw      *conn
	running map[string]*protoRW

	wg       sync.WaitGroup
	protoErr chan error
	closed   chan struct{}
	disc     chan DiscReason
}

// NewPeer returns a peer for testing purposes.
func NewPeer(id discover.NodeID, name string, caps []Cap) *Peer {
	pipe, _ := net.Pipe()
	conn := &conn{fd: pipe, transport: nil, id: id, caps: caps, name: name}
	peer := newPeer(conn, nil)
	close(peer.closed) // ensures Disconnect doesn't block
	return peer
}

// ID returns the node's public key.
func (p *Peer) ID() discover.NodeID {
	return p.rw.id
}

// Name returns the node name that the remote node advertised.
func (p *Peer) Name() string {
	return p.rw.name
}

// Caps returns the capabilities (supported subprotocols) of the remote peer.
func (p *Peer) Caps() []Cap {
	// TODO: maybe return copy
	return p.rw.caps
}

// RemoteAddr returns the remote address of the network connection.
func (p *Peer) RemoteAddr() net.Addr {
	return p.rw.fd.RemoteAddr()
}

// LocalAddr returns the local address of the network connection.
func (p *Peer) LocalAddr() net.Addr {
	return p.rw.fd.LocalAddr()
}

// Disconnect terminates the peer connection with the given reason.
// It returns immediately and does not wait until the connection is closed.
func (p *Peer) Disconnect(reason DiscReason) {
	select {
	case p.disc <- reason:
	case <-p.closed:
		return
	}
}

// String implements fmt.Stringer.
func (p *Peer) String() string {
	return fmt.Sprintf("Peer %x %v", p.rw.id[:8], p.RemoteAddr())
}

// newPeer wires up a Peer over an already-handshaked conn, matching
// the supplied subprotocols against the remote's advertised caps.
func newPeer(conn *conn, protocols []Protocol) *Peer {
	protomap := matchProtocols(protocols, conn.caps, conn)
	p := &Peer{
		rw:       conn,
		running:  protomap,
		disc:     make(chan DiscReason),
		protoErr: make(chan error, len(protomap)+1), // protocols + pingLoop
		closed:   make(chan struct{}),
	}
	return p
}

// run drives the peer's lifetime: launches readLoop / pingLoop /
// per-protocol handlers, blocks until any of them errors or a
// disconnect is requested, then closes the connection. Returns the
// DiscReason recorded for the disconnect.
func (p *Peer) run() DiscReason {
	var (
		writeStart = make(chan struct{}, 1)
		writeErr   = make(chan error, 1)
		readErr    = make(chan error, 1)
		reason     DiscReason
		requested  bool
	)

	p.wg.Add(1)
	go func() {
		p.readLoop(readErr)
		p.wg.Done()
	}()

	p.wg.Add(1)
	go func() {
		p.pingLoop()
		p.wg.Done()
	}()

	// Start all protocol handlers.
	writeStart <- struct{}{}
	p.startProtocols(writeStart, writeErr)

	// Wait for an error or disconnect.
loop:
	for {
		select {
		case err := <-writeErr:
			// A write finished. Allow the next write to start if
			// there was no error.
			if err != nil {
				common.P2PLogger.Debug(fmt.Sprintf("%v: write error: %v\n", p, err))
				reason = DiscNetworkError
				break loop
			}
			writeStart <- struct{}{}
		case err := <-readErr:
			if r, ok := err.(DiscReason); ok {
				common.P2PLogger.Debug(fmt.Sprintf("%v: remote requested disconnect: %v\n", p, r))
				requested = true
				reason = r
			} else {
				common.P2PLogger.Debug(fmt.Sprintf("%v: read error: %v\n", p, err))
				reason = DiscNetworkError
			}
			break loop
		case err := <-p.protoErr:
			reason = discReasonForError(err)
			common.P2PLogger.Debug(fmt.Sprintf("%v: protocol error: %v (%v)\n", p, err, reason))
			break loop
		case reason = <-p.disc:
			common.P2PLogger.Debug(fmt.Sprintf("%v: locally requested disconnect: %v\n", p, reason))
			break loop
		}
	}

	p.rw.close(reason)
	close(p.closed)
	// Historical: p.wg.Wait() used to block here until every
	// subprotocol goroutine returned. The Wait was disabled to avoid
	// hanging shutdowns when a misbehaving protocol never returned;
	// the surrounding Debug logs were left in place to mark where the
	// barrier used to be. Closing p.closed above is what now signals
	// the protocols to stop; protocol goroutines return at their own
	// pace.
	common.P2PLogger.Debug("wg.Wait() peer.run() (skipped — see comment)")
	// p.wg.Wait()
	common.P2PLogger.Debug("wg.Wait() peer.run() finished (skipped — see comment)")
	if requested {
		reason = DiscRequested
	}
	return reason
}

// pingLoop emits a pingMsg every pingInterval and exits when the peer
// is closed. A failed write surfaces as a protoErr that triggers
// shutdown.
func (p *Peer) pingLoop() {
	ping := time.NewTicker(pingInterval)
	defer ping.Stop()
	for {
		select {
		case <-ping.C:
			if err := SendItems(p.rw, pingMsg); err != nil {
				p.protoErr <- err
				return
			}
		case <-p.closed:
			return
		}
	}
}

// readLoop is the single reader of the underlying transport. Each
// decoded message is timestamped and dispatched by handle; the first
// read or handle error terminates the loop.
func (p *Peer) readLoop(errc chan<- error) {
	for {
		msg, err := p.rw.ReadMsg()
		if err != nil {
			errc <- err
			return
		}
		msg.ReceivedAt = time.Now()
		if err = p.handle(msg); err != nil {
			errc <- err
			return
		}
	}
}

// handle dispatches one inbound message: replies to ping, returns the
// reason on disc, hands off subprotocol messages to the matching
// protoRW, and silently discards unknown base-protocol codes.
func (p *Peer) handle(msg Msg) error {
	switch {
	case msg.Code == pingMsg:
		msg.Discard()
		p.wg.Add(1)
		go func() {
			SendItems(p.rw, pongMsg)
			p.wg.Done()
		}()
	case msg.Code == discMsg:
		var reason [1]DiscReason
		// This is the last message. We don't need to discard or
		// check errors because, the connection will be closed after it.
		rlp.Decode(msg.Payload, &reason)
		return reason[0]
	case msg.Code < baseProtocolLength:
		// ignore other base protocol messages
		return msg.Discard()
	default:
		// it's a subprotocol message
		proto, err := p.getProto(msg.Code)
		if err != nil {
			return fmt.Errorf("msg code out of range: %v", msg.Code)
		}
		select {
		case proto.in <- msg:
			return nil
		case <-p.closed:
			return io.EOF
		}
	}
	return nil
}

// countMatchingProtocols counts how many entries in caps name a
// protocol the local node supports. Used by Server.run to drop peers
// with no usable subprotocol overlap (DiscUselessPeer).
func countMatchingProtocols(protocols []Protocol, caps []Cap) int {
	n := 0
	for _, cap := range caps {
		for _, proto := range protocols {
			if proto.Name == cap.Name && proto.Version == cap.Version {
				n++
			}
		}
	}
	return n
}

// matchProtocols creates structures for matching named subprotocols.
func matchProtocols(protocols []Protocol, caps []Cap, rw MsgReadWriter) map[string]*protoRW {
	sort.Sort(capsByNameAndVersion(caps))
	offset := baseProtocolLength
	result := make(map[string]*protoRW)

outer:
	for _, cap := range caps {
		for _, proto := range protocols {
			if proto.Name == cap.Name && proto.Version == cap.Version {
				// If an old protocol version matched, revert it
				if old := result[cap.Name]; old != nil {
					offset -= old.Length
				}
				// Assign the new match
				result[cap.Name] = &protoRW{Protocol: proto, offset: offset, in: make(chan Msg), w: rw}
				offset += proto.Length

				continue outer
			}
		}
	}
	return result
}

// startProtocols launches each matched subprotocol handler in its own
// goroutine, sharing the writeStart token so writes serialize across
// protocols on the same connection.
func (p *Peer) startProtocols(writeStart <-chan struct{}, writeErr chan<- error) {
	p.wg.Add(len(p.running))
	for _, proto := range p.running {
		proto := proto
		proto.closed = p.closed
		proto.wstart = writeStart
		proto.werr = writeErr
		common.P2PLogger.Debug(fmt.Sprintf("%v: Starting protocol %s/%d\n", p, proto.Name, proto.Version))
		go func() {
			err := proto.Run(p, proto)
			p.wg.Done()
			if err == nil {
				common.P2PLogger.Debug(fmt.Sprintf("%v: Protocol %s/%d returned\n", p, proto.Name, proto.Version))
				err = errors.New("protocol returned")
			} else if err != io.EOF {
				common.P2PLogger.Debug(fmt.Sprintf("%v: Protocol %s/%d error: %v\n", p, proto.Name, proto.Version, err))
			}
			p.protoErr <- err
		}()
	}
}

// getProto finds the protocol responsible for handling
// the given message code.
func (p *Peer) getProto(code uint64) (*protoRW, error) {
	for _, proto := range p.running {
		if code >= proto.offset && code < proto.offset+proto.Length {
			return proto, nil
		}
	}
	return nil, newPeerError(errInvalidMsgCode, "%d", code)
}

// protoRW is the per-subprotocol MsgReadWriter handed to a Protocol's
// Run callback. It applies the subprotocol's offset when reading /
// writing so each subprotocol sees its codes starting at zero, and
// gates writes through the wstart token to serialize with other
// protocols on the same Peer.
type protoRW struct {
	Protocol
	in     chan Msg        // receices read messages
	closed <-chan struct{} // receives when peer is shutting down
	wstart <-chan struct{} // receives when write may start
	werr   chan<- error    // for write results
	offset uint64
	w      MsgWriter
}

// WriteMsg is part of the receiver's public API.
func (rw *protoRW) WriteMsg(msg Msg) (err error) {
	if msg.Code >= rw.Length {
		return newPeerError(errInvalidMsgCode, "not handled")
	}
	msg.Code += rw.offset
	select {
	case <-rw.wstart:
		err = rw.w.WriteMsg(msg)
		// Report write status back to Peer.run. It will initiate
		// shutdown if the error is non-nil and unblock the next write
		// otherwise. The calling protocol code should exit for errors
		// as well but we don't want to rely on that.
		rw.werr <- err
	case <-rw.closed:
		err = fmt.Errorf("shutting down")
	}
	return err
}

// ReadMsg is part of the receiver's public API.
func (rw *protoRW) ReadMsg() (Msg, error) {
	select {
	case msg := <-rw.in:
		msg.Code -= rw.offset
		return msg, nil
	case <-rw.closed:
		return Msg{}, io.EOF
	}
}
