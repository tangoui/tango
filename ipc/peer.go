package ipc

import (
	"encoding/base64"
	"encoding/binary"
)

// frame is one stream data frame travelling between a Stream and its peer.
//
// The payload form of byte frames depends on the direction:
//   - Inbound (peer → Stream.onFrame): always raw bytes. A transport that
//     carries bytes encoded (runtime-js: base64 in "bin") decodes them
//     before calling onFrame.
//   - Outbound (Stream → peer.sendFrame): whatever prepareBytes produced
//     for that peer: for wirePeer, laneRoom bytes of room followed by the
//     raw bytes (wirePeer base64-encodes them itself unless the frame goes
//     on a fast lane); a raw copy otherwise.
//
// JSON frames always carry the JSON text.
type frame struct {
	seq     uint64
	cost    int    // charged size (the wire "c"): payload length + FrameOverhead
	bin     bool   // true: byte frame (payload form: see above); false: JSON frame
	payload []byte // private to the frame: the receiver may keep it
	// noLane (outbound): the frame must not take a fast lane. Set while
	// this end opened the stream and the peer's accept has not arrived: a
	// lane packet could overtake the open message.
	noLane bool
}

// ctl is a stream control message travelling from a Stream to its peer.
type ctl struct {
	op  string // "accept" | "ack" | "end" | "close" | "error" | "cancel"
	seq uint64 // end/close/error: frames sent in this direction
	// hasSeq (error only): seq is set. An error with seq is ordered like
	// end/close (frames before it are delivered first); one without seq
	// (a protocol-level failure) aborts at once.
	hasSeq bool
	b      int    // ack: credit returned, in bytes
	in     bool   // accept: whether this end receives data
	err    *Error // error
}

// peer hands this end's frames and control messages to the other end.
// Implementations must not block and must not call back into methods of the
// sending Stream that take its locks. (A peer may deliver to the other end
// synchronously; the other end's onCtl only takes its state mutex, never its
// write lock, so an ack delivered while that side is writing is safe.)
type peer interface {
	sendFrame(id uint64, f frame)
	sendCtl(id uint64, c ctl)
}

// bytesPreparer is optionally implemented by a peer that wants byte payloads
// in a transport-specific form. Stream.WriteBytes calls it before taking the
// send lock; its result must not alias p (the caller may reuse p once
// WriteBytes returns) and becomes frame.payload for that peer's sendFrame.
// Peers that do not implement it receive a private copy of the raw bytes.
type bytesPreparer interface {
	prepareBytes(p []byte) []byte
}

// prepareBytes returns the payload p should travel as on peer pr.
func prepareBytes(pr peer, p []byte) []byte {
	if bp, ok := pr.(bytesPreparer); ok {
		return bp.prepareBytes(p)
	}
	return append([]byte{}, p...)
}

// wirePeer encodes frames and control messages as v2 JSON messages for
// runtime-js and sends them through the connection. A byte frame of at
// least BinaryMinSize bytes goes on the downstream fast lane instead when
// the connection is a BinaryConn whose lane accepts it. The choice is made
// per frame (a lane may come and go between frames); seq and cost are
// assigned before it, so credit and ordering do not depend on the path.
type wirePeer struct{ cs *connState }

// laneRoom is the room wirePeer.prepareBytes leaves before the raw bytes:
// the header of a one-frame lane packet, so the lane path sends the buffer
// as is, without a second copy.
const laneRoom = packetHeaderBytes + frameHeaderBytes

// prepareBytes copies p after laneRoom bytes of room, outside the send
// lock. The payload of byte frames passed to wirePeer.sendFrame is
// therefore that buffer (room, then the raw bytes); base64 encoding waits
// until sendFrame knows the frame goes as a message.
func (w wirePeer) prepareBytes(p []byte) []byte {
	buf := make([]byte, laneRoom+len(p))
	copy(buf[laneRoom:], p)
	return buf
}

// sendFrame runs on the writing goroutine holding only the stream's send
// lock (never its state mutex), so the base64 encoding here stays off the
// path onFrame/onCtl contend on.
func (w wirePeer) sendFrame(id uint64, f frame) {
	m := streamData{Tango: "stream", ID: id, Op: "data", Seq: f.seq, C: f.cost}
	if f.bin {
		if !f.noLane && w.sendLane(id, f) {
			return
		}
		b64 := base64.StdEncoding.EncodeToString(f.payload[laneRoom:])
		m.Bin = &b64
	} else {
		m.Data = f.payload
	}
	w.cs.send(encode(m))
}

// sendLane sends byte frame f as a one-frame packet on the downstream lane
// and reports whether it was handled. f.payload is prepareBytes' buffer;
// the packet header is written into its room.
func (w wirePeer) sendLane(id uint64, f frame) bool {
	bc := w.cs.binary
	if bc == nil {
		return false
	}
	n := len(f.payload) - laneRoom
	if min := bc.BinaryMinSize(); min < 0 || n < min {
		return false
	}
	if w.cs.ctx.Err() != nil {
		return true // connection dropped: send nothing, as cs.send does
	}
	pkt := f.payload
	binary.BigEndian.PutUint16(pkt, 1)
	binary.BigEndian.PutUint64(pkt[packetHeaderBytes:], id)
	binary.BigEndian.PutUint64(pkt[packetHeaderBytes+8:], f.seq)
	binary.BigEndian.PutUint32(pkt[packetHeaderBytes+16:], uint32(n))
	if !bc.SendBinary(pkt) {
		return false
	}
	w.sendCtl(id, ctl{op: "boundary", seq: f.seq + 1})
	return true
}

func (w wirePeer) sendCtl(id uint64, c ctl) {
	m := streamCtl{Tango: "stream", ID: id, Op: c.op}
	switch c.op {
	case "accept":
		in := c.in
		m.In = &in
	case "ack":
		m.B = c.b
	case "boundary", "end", "close":
		seq := c.seq
		m.Seq = &seq
	case "error":
		seq := c.seq
		m.Seq = &seq
		m.Error = c.err
	}
	w.cs.send(encode(m))
}
