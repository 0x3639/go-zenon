package libp2p

import (
	"encoding/binary"
	"fmt"
	"io"
	"sync"
	"time"

	"github.com/ethereum/go-ethereum/rlp"
	"github.com/libp2p/go-libp2p/core/network"

	"github.com/zenon-network/go-zenon/p2p"
)

// maxMessageSize is the maximum payload size allowed in a single message.
// This prevents a malicious peer from causing an OOM by declaring a huge payload.
const maxMessageSize = 10 * 1024 * 1024 // 10MB, matching ProtocolMaxMsgSize

// initialPayloadBuffer is the most readMsg allocates for a payload before
// any of it has arrived. The buffer doubles as it fills, up to the declared
// size, so a header alone cannot make the reader reserve a full
// maxMessageSize buffer for the length of the read deadline.
const initialPayloadBuffer = 64 * 1024

// StreamRW wraps a libp2p bidirectional stream to implement p2p.MsgReadWriter.
//
// Wire format (replaces RLPX framing):
//
//	[4 bytes: message code, big-endian uint32]
//	[4 bytes: payload length, big-endian uint32]
//	[N bytes: RLP payload]
//
// No encryption at this layer — libp2p Noise handles transport security.
type StreamRW struct {
	stream network.Stream
	rmu    sync.Mutex
	wmu    sync.Mutex
}

// NewStreamRW creates a new StreamRW wrapping the given libp2p stream.
func NewStreamRW(s network.Stream) *StreamRW {
	return &StreamRW{stream: s}
}

// ReadMsg reads a message from the stream, rejecting a frame whose
// declared size exceeds maxMessageSize before allocating a payload
// buffer for it.
func (rw *StreamRW) ReadMsg() (p2p.Msg, error) {
	return rw.readMsg(maxMessageSize, frameReadTimeout)
}

// ReadMsgLimitedTimeout reads a message from the stream, rejecting a
// frame whose declared size exceeds maxSize before allocating a payload
// buffer for it, under an explicit read deadline rather than the
// steady-state frameReadTimeout. Used for the pre-adoption handshake
// read, which should time out much sooner than a live peer's frame reads
// so a stalled, unauthenticated remote can't hold a pending-handshake
// slot for the full steady-state timeout.
func (rw *StreamRW) ReadMsgLimitedTimeout(maxSize uint32, timeout time.Duration) (p2p.Msg, error) {
	return rw.readMsg(maxSize, timeout)
}

func (rw *StreamRW) readMsg(maxSize uint32, timeout time.Duration) (p2p.Msg, error) {
	rw.rmu.Lock()
	defer rw.rmu.Unlock()

	rw.stream.SetReadDeadline(time.Now().Add(timeout))
	defer rw.stream.SetReadDeadline(time.Time{})

	// Read 8-byte header
	var header [8]byte
	if _, err := io.ReadFull(rw.stream, header[:]); err != nil {
		return p2p.Msg{}, err
	}

	code := uint64(binary.BigEndian.Uint32(header[0:4]))
	size := binary.BigEndian.Uint32(header[4:8])

	// Reject messages exceeding the maximum size before allocating memory
	if size > maxSize {
		return p2p.Msg{}, fmt.Errorf("message too large: %d bytes (max %d)", size, maxSize)
	}

	payload, err := readPayload(rw.stream, int(size))
	if err != nil {
		return p2p.Msg{}, err
	}

	return p2p.Msg{
		Code:    code,
		Size:    size,
		Payload: &payloadReader{data: payload},
	}, nil
}

// readPayload reads exactly size bytes from r. The buffer starts at
// initialPayloadBuffer and doubles each time it fills, so its size stays
// within twice what the remote has actually sent (or initialPayloadBuffer,
// if larger), and the final buffer is exactly size bytes. A stream that ends
// before the first payload byte returns io.EOF and one that ends later
// returns io.ErrUnexpectedEOF, as io.ReadFull does.
func readPayload(r io.Reader, size int) ([]byte, error) {
	buf := make([]byte, min(size, initialPayloadBuffer))
	read := 0
	for {
		n, err := io.ReadFull(r, buf[read:])
		read += n
		if err != nil {
			if err == io.EOF && read > 0 {
				err = io.ErrUnexpectedEOF
			}
			return nil, err
		}
		if read == size {
			return buf, nil
		}
		grown := make([]byte, min(2*len(buf), size))
		copy(grown, buf[:read])
		buf = grown
	}
}

// WriteMsg writes a message to the stream.
func (rw *StreamRW) WriteMsg(msg p2p.Msg) error {
	rw.wmu.Lock()
	defer rw.wmu.Unlock()

	rw.stream.SetWriteDeadline(time.Now().Add(frameWriteTimeout))
	defer rw.stream.SetWriteDeadline(time.Time{})

	// Encode payload. Bound the read at maxMessageSize+1 so an oversized
	// or unbounded Payload reader can't force an unbounded allocation
	// here — ReadMsg enforces the same cap on the receiving side.
	payload, err := io.ReadAll(io.LimitReader(msg.Payload, int64(maxMessageSize)+1))
	if err != nil {
		return fmt.Errorf("read payload: %w", err)
	}
	if len(payload) > maxMessageSize {
		return fmt.Errorf("message too large: payload exceeds %d bytes", maxMessageSize)
	}
	if msg.Size != uint32(len(payload)) {
		return fmt.Errorf("declared message size %d does not match payload length %d", msg.Size, len(payload))
	}

	// Build 8-byte header
	var header [8]byte
	binary.BigEndian.PutUint32(header[0:4], uint32(msg.Code))
	binary.BigEndian.PutUint32(header[4:8], uint32(len(payload)))

	// Write header + payload, looping to handle short writes
	if err := writeFull(rw.stream, header[:]); err != nil {
		return err
	}
	if len(payload) > 0 {
		if err := writeFull(rw.stream, payload); err != nil {
			return err
		}
	}
	return nil
}

// writeFull writes all bytes from buf to w, looping on short writes.
func writeFull(w io.Writer, buf []byte) error {
	for len(buf) > 0 {
		n, err := w.Write(buf)
		if err != nil {
			return err
		}
		buf = buf[n:]
	}
	return nil
}

// Close closes the underlying stream.
func (rw *StreamRW) Close() error {
	return rw.stream.Close()
}

// payloadReader is a simple io.Reader over a byte slice.
type payloadReader struct {
	data []byte
	pos  int
}

func (r *payloadReader) Read(buf []byte) (int, error) {
	if r.pos >= len(r.data) {
		return 0, io.EOF
	}
	n := copy(buf, r.data[r.pos:])
	r.pos += n
	return n, nil
}

// RlpEncode encodes data as RLP and returns the bytes and size.
func RlpEncode(data interface{}) ([]byte, uint32, error) {
	size, r, err := rlp.EncodeToReader(data)
	if err != nil {
		return nil, 0, err
	}
	payload, err := io.ReadAll(r)
	if err != nil {
		return nil, 0, err
	}
	return payload, uint32(size), nil
}
