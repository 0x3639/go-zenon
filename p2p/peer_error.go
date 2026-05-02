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
	"fmt"
)

const (
	// errInvalidMsgCode is returned when a peer sends a message code
	// outside the negotiated subprotocol's reserved range.
	errInvalidMsgCode = iota
	// errInvalidMsg is returned when a message's RLP payload fails to
	// decode into the expected structure.
	errInvalidMsg
)

// errorToString maps each peerError code to its human-readable
// description. newPeerError panics if it is given a code missing from
// this table — every constant in the err* group above must be listed.
var errorToString = map[int]string{
	errInvalidMsgCode: "invalid message code",
	errInvalidMsg:     "invalid message",
}

// peerError is the internal error type for misbehaving-peer cases.
// It carries a code so discReasonForError can map it to a wire-level
// DiscReason.
type peerError struct {
	code    int
	message string
}

// newPeerError builds a peerError whose message starts with the
// canonical description for code, optionally followed by formatted
// detail. Panics if code is unknown — the constants and errorToString
// must stay in sync.
func newPeerError(code int, format string, v ...interface{}) *peerError {
	desc, ok := errorToString[code]
	if !ok {
		panic("invalid error code")
	}
	err := &peerError{code, desc}
	if format != "" {
		err.message += ": " + fmt.Sprintf(format, v...)
	}
	return err
}

func (self *peerError) Error() string {
	return self.message
}

// DiscReason is the wire-level enum sent in discMsg explaining why a
// connection is being closed. Values mirror the devp2p disconnect
// reasons; new entries must be appended to keep wire compatibility.
type DiscReason uint

const (
	// DiscRequested — local code or remote peer voluntarily closed.
	DiscRequested DiscReason = iota
	// DiscNetworkError — TCP-level read/write error.
	DiscNetworkError
	// DiscProtocolError — base-protocol violation (bad message code,
	// malformed RLP).
	DiscProtocolError
	// DiscUselessPeer — no overlapping subprotocols.
	DiscUselessPeer
	// DiscTooManyPeers — admission cap exceeded.
	DiscTooManyPeers
	// DiscAlreadyConnected — duplicate connection from same node ID.
	DiscAlreadyConnected
	// DiscIncompatibleVersion — base-protocol version mismatch.
	DiscIncompatibleVersion
	// DiscInvalidIdentity — empty or malformed node ID in handshake.
	DiscInvalidIdentity
	// DiscQuitting — local node is shutting down.
	DiscQuitting
	// DiscUnexpectedIdentity — dialed peer's ID didn't match the
	// expected enode ID.
	DiscUnexpectedIdentity
	// DiscSelf — connected to ourselves (loopback or NAT echo).
	DiscSelf
	// DiscReadTimeout — connection was idle past frameReadTimeout.
	DiscReadTimeout
	// DiscSubprotocolError — error originating in a registered
	// subprotocol's Run callback.
	DiscSubprotocolError
)

var discReasonToString = [...]string{
	DiscRequested:           "Disconnect requested",
	DiscNetworkError:        "Network error",
	DiscProtocolError:       "Breach of protocol",
	DiscUselessPeer:         "Useless peer",
	DiscTooManyPeers:        "Too many peers",
	DiscAlreadyConnected:    "Already connected",
	DiscIncompatibleVersion: "Incompatible P2P protocol version",
	DiscInvalidIdentity:     "Invalid node identity",
	DiscQuitting:            "Client quitting",
	DiscUnexpectedIdentity:  "Unexpected identity",
	DiscSelf:                "Connected to self",
	DiscReadTimeout:         "Read timeout",
	DiscSubprotocolError:    "Subprotocol error",
}

func (d DiscReason) String() string {
	if len(discReasonToString) < int(d) {
		return fmt.Sprintf("Unknown Reason(%d)", d)
	}
	return discReasonToString[d]
}

func (d DiscReason) Error() string {
	return d.String()
}

// discReasonForError maps an arbitrary error to the DiscReason that
// should be sent on the wire: pass through if it's already a
// DiscReason, fold base-protocol errors to DiscProtocolError, and
// fall back to DiscSubprotocolError otherwise.
func discReasonForError(err error) DiscReason {
	if reason, ok := err.(DiscReason); ok {
		return reason
	}
	peerError, ok := err.(*peerError)
	if ok {
		switch peerError.code {
		case errInvalidMsgCode, errInvalidMsg:
			return DiscProtocolError
		default:
			return DiscSubprotocolError
		}
	}
	return DiscSubprotocolError
}
