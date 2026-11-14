package ipc

import (
	"bytes"
	"encoding/binary"
	"testing"
)

func framesEqual(t *testing.T, got, want []PacketFrame) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("got %d frames, want %d", len(got), len(want))
	}
	for i := range want {
		g, w := got[i], want[i]
		if g.Stream != w.Stream || g.Seq != w.Seq || !bytes.Equal(g.Payload, w.Payload) {
			t.Fatalf("frame %d: got {%d %d len %d}, want {%d %d len %d}",
				i, g.Stream, g.Seq, len(g.Payload), w.Stream, w.Seq, len(w.Payload))
		}
	}
}

func TestPacketRoundTrip(t *testing.T) {
	if p := AppendPacket(nil); !bytes.Equal(p, []byte{0, 0}) {
		t.Fatalf("empty packet %v, want [0 0]", p)
	}
	big := make([]byte, 64<<10)
	for i := range big {
		big[i] = byte(i * 7)
	}
	cases := [][]PacketFrame{
		{},
		{{Stream: 1, Seq: 0, Payload: nil}},
		{
			{Stream: 1, Seq: 5, Payload: []byte("abc")},
			{Stream: 1<<63 + 3, Seq: 1<<40 + 9, Payload: big},
			{Stream: 2, Seq: 0, Payload: []byte{}},
		},
	}
	for _, frames := range cases {
		p := AppendPacket(nil, frames...)
		got, err := ParsePacket(p)
		if err != nil {
			t.Fatalf("%d frames: ParsePacket: %v", len(frames), err)
		}
		framesEqual(t, got, frames)
	}

	// Appending keeps dst's prefix; the exact layout is big endian.
	p := AppendPacket([]byte("xy"), PacketFrame{Stream: 0x0102, Seq: 3, Payload: []byte{9}})
	want := []byte("xy")
	want = append(want, 0, 1)
	want = binary.BigEndian.AppendUint64(want, 0x0102)
	want = binary.BigEndian.AppendUint64(want, 3)
	want = binary.BigEndian.AppendUint32(want, 1)
	want = append(want, 9)
	if !bytes.Equal(p, want) {
		t.Fatalf("layout\n got %v\nwant %v", p, want)
	}
}

// header appends a frame header (no payload) to dst.
func header(dst []byte, stream, seq uint64, n uint32) []byte {
	dst = binary.BigEndian.AppendUint64(dst, stream)
	dst = binary.BigEndian.AppendUint64(dst, seq)
	return binary.BigEndian.AppendUint32(dst, n)
}

func TestParsePacketRejects(t *testing.T) {
	valid := AppendPacket(nil, PacketFrame{Stream: 1, Seq: 0, Payload: []byte("abc")})
	cases := map[string][]byte{
		"empty":            {},
		"one byte":         {0},
		"short header":     append([]byte{0, 1}, header(nil, 1, 0, 0)[:19]...),
		"len past end":     append(header([]byte{0, 1}, 1, 0, 4), "abc"...),
		"len over max":     header([]byte{0, 1}, 1, 0, MaxFrameBytes+1),
		"trailing bytes":   append(append([]byte{}, valid...), 0),
		"count too high":   append([]byte{0, 2}, valid[2:]...),
		"empty plus extra": {0, 0, 0},
	}
	for name, p := range cases {
		if fs, err := ParsePacket(p); err == nil {
			t.Errorf("%s: ParsePacket(%v) = %d frames, want error", name, p, len(fs))
		}
	}
}
