package ipc

import (
	"encoding/binary"
	"errors"
	"fmt"
)

// Fast lane packet layout (big endian; see the package comment):
//
//	packet := count:u16 | frame*count
//	frame  := streamID:u64 | seq:u64 | len:u32 | payload[len]
const (
	packetHeaderBytes = 2
	frameHeaderBytes  = 8 + 8 + 4
	maxPacketFrames   = 1<<16 - 1
)

// PacketFrame is one byte frame of a fast lane packet.
type PacketFrame struct {
	Stream  uint64
	Seq     uint64
	Payload []byte // after ParsePacket: points into the input; copy it to keep it
}

// AppendPacket appends the packet carrying frames to dst and returns the
// extended slice. With no frames it appends the empty packet {0, 0}.
//
// It panics if there are more than 65535 frames or a payload is larger than
// MaxFrameBytes: such a packet would never parse, so the caller has a bug.
func AppendPacket(dst []byte, frames ...PacketFrame) []byte {
	if len(frames) > maxPacketFrames {
		panic("ipc: AppendPacket: more than 65535 frames")
	}
	n := packetHeaderBytes
	for _, f := range frames {
		if len(f.Payload) > MaxFrameBytes {
			panic("ipc: AppendPacket: payload exceeds MaxFrameBytes")
		}
		n += frameHeaderBytes + len(f.Payload)
	}
	if cap(dst)-len(dst) < n {
		grown := make([]byte, len(dst), len(dst)+n)
		copy(grown, dst)
		dst = grown
	}
	dst = binary.BigEndian.AppendUint16(dst, uint16(len(frames)))
	for _, f := range frames {
		dst = binary.BigEndian.AppendUint64(dst, f.Stream)
		dst = binary.BigEndian.AppendUint64(dst, f.Seq)
		dst = binary.BigEndian.AppendUint32(dst, uint32(len(f.Payload)))
		dst = append(dst, f.Payload...)
	}
	return dst
}

var errShortPacket = errors.New("ipc: lane packet shorter than its header")

// ParsePacket decodes a fast lane packet. It fails unless p holds exactly
// the frames its count announces, each with a payload of at most
// MaxFrameBytes, and nothing after them. The returned payloads point into
// p (an empty payload is a non-nil empty slice).
func ParsePacket(p []byte) ([]PacketFrame, error) {
	if len(p) < packetHeaderBytes {
		return nil, errShortPacket
	}
	count := int(binary.BigEndian.Uint16(p))
	rest := p[packetHeaderBytes:]
	// Every frame needs at least its header: reject a count the packet
	// cannot hold before allocating for it.
	if count > len(rest)/frameHeaderBytes {
		return nil, fmt.Errorf("ipc: lane packet announces %d frames in %d bytes", count, len(rest))
	}
	frames := make([]PacketFrame, count)
	for i := range frames {
		if len(rest) < frameHeaderBytes {
			return nil, fmt.Errorf("ipc: lane packet frame %d: truncated header", i)
		}
		n := binary.BigEndian.Uint32(rest[16:])
		if n > MaxFrameBytes {
			return nil, fmt.Errorf("ipc: lane packet frame %d: %d bytes exceeds MaxFrameBytes", i, n)
		}
		body := rest[frameHeaderBytes:]
		if uint64(n) > uint64(len(body)) {
			return nil, fmt.Errorf("ipc: lane packet frame %d: %d bytes past the end of the packet", i, n)
		}
		frames[i] = PacketFrame{
			Stream:  binary.BigEndian.Uint64(rest),
			Seq:     binary.BigEndian.Uint64(rest[8:]),
			Payload: body[:n:n],
		}
		rest = body[n:]
	}
	if len(rest) != 0 {
		return nil, fmt.Errorf("ipc: lane packet has %d trailing bytes", len(rest))
	}
	return frames, nil
}
