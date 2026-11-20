package shmring

import (
	"bytes"
	"context"
	"encoding/binary"
	"flag"
	"os"
	"testing"
	"time"
)

const testCapacity = 64 << 10

var updateGolden = flag.Bool("update", false, "rewrite the shared-memory ring golden file")

func newTestRing(t testing.TB, gen uint64) ([]byte, *Ring) {
	t.Helper()
	mem := make([]byte, HeaderSize+testCapacity)
	r, err := Init(mem, gen)
	if err != nil {
		t.Fatal(err)
	}
	return mem, r
}

func put32(mem []byte, offset int, value uint32) {
	binary.LittleEndian.PutUint32(mem[offset:], value)
}

func get32(mem []byte, offset int) uint32 {
	return binary.LittleEndian.Uint32(mem[offset:])
}

func writePacket(t *testing.T, r *Ring, p []byte) {
	t.Helper()
	if ok, _ := r.TryWrite(p); !ok {
		t.Fatalf("TryWrite refused a %d-byte packet", len(p))
	}
}

func checkPackets(t *testing.T, r *Ring, packets ...[]byte) {
	t.Helper()
	i := 0
	n, _, err := r.Read(func(p []byte) {
		if i >= len(packets) || !bytes.Equal(p, packets[i]) {
			t.Fatalf("packet %d: unexpected %d-byte payload", i, len(p))
		}
		i++
	})
	if err != nil || n != len(packets) || i != n {
		t.Fatalf("Read delivered %d, returned %d, %v; want %d packets", i, n, err, len(packets))
	}
}

func TestRingRoundTrip(t *testing.T) {
	mem, writer := newTestRing(t, 5)
	reader, err := Open(mem, 5)
	if err != nil {
		t.Fatal(err)
	}
	if got := writer.MaxPacket(); got != 32768 {
		t.Fatalf("MaxPacket = %d, want 32768", got)
	}
	// Start near the end of the uint32 counter range to exercise rollover.
	put32(mem, 0, 0xfffffff8)
	put32(mem, 4, 0xfffffff8)
	for _, size := range []int{1, 4096, writer.MaxPacket()} {
		p := make([]byte, size)
		for i := range 1000 {
			for j := range p {
				p[j] = byte(i + j)
			}
			writePacket(t, writer, p)
			checkPackets(t, reader, p)
		}
	}
	writePacket(t, writer, nil)
	checkPackets(t, reader, nil)
	checkPackets(t, reader)
	if ok, wake := writer.TryWrite(make([]byte, writer.MaxPacket()+1)); ok || wake || get32(mem, 12) != 0 {
		t.Fatal("oversized packet was accepted or set a doorbell flag")
	}
}

func TestRingWrapMarker(t *testing.T) {
	for _, tt := range []struct {
		name string
		tail uint32
		want uint32
		end  uint32
	}{
		{"crosses end", 8, 0xffffffff, testCapacity + 12},
		{"ends at boundary", 12, 5, testCapacity},
		{"four bytes left", 4, 0xffffffff, testCapacity + 12},
	} {
		t.Run(tt.name, func(t *testing.T) {
			mem, r := newTestRing(t, 1)
			start := testCapacity - tt.tail
			put32(mem, 0, start)
			put32(mem, 4, start)
			p := []byte{1, 2, 3, 4, 5}
			writePacket(t, r, p)
			if got := get32(mem, HeaderSize+int(start)); got != tt.want {
				t.Fatalf("tail prefix = %#x, want %#x", got, tt.want)
			}
			if got := get32(mem, 0); got != tt.end {
				t.Fatalf("write = %d, want %d", got, tt.end)
			}
			checkPackets(t, r, p)
			if got := get32(mem, 4); got != tt.end {
				t.Fatalf("read = %d, want %d", got, tt.end)
			}
		})
	}

	t.Run("max packet at midpoint", func(t *testing.T) {
		mem, r := newTestRing(t, 1)
		put32(mem, 0, testCapacity/2)
		put32(mem, 4, testCapacity/2)
		if r.Sleep() {
			t.Fatal("empty ring should sleep")
		}
		p := bytes.Repeat([]byte{42}, r.MaxPacket())
		if ok, wake := r.TryWrite(p); ok || !wake {
			t.Fatalf("TryWrite = %t, %t; want marker-only publication and a reader wake", ok, wake)
		}
		r.Woken()
		n, wake, err := r.Read(func([]byte) { t.Fatal("wrap marker delivered as a packet") })
		if n != 0 || !wake || err != nil || get32(mem, 4) != testCapacity {
			t.Fatalf("marker Read = %d, %t, %v; read = %d", n, wake, err, get32(mem, 4))
		}
		r.WriterWoken()
		writePacket(t, r, p)
		checkPackets(t, r, p)
	})
}

func TestRingFullSetsWriterWaiting(t *testing.T) {
	mem, r := newTestRing(t, 1)
	p := make([]byte, testCapacity/2-4)
	for range 2 {
		writePacket(t, r, p)
	}
	if ok, wake := r.TryWrite([]byte{1}); ok || wake || get32(mem, 12) != 1 || get32(mem, 0) != testCapacity {
		t.Fatalf("full TryWrite = %t, %t; writerWaiting = %d, write = %d", ok, wake, get32(mem, 12), get32(mem, 0))
	}
	var calls uint32
	n, wake, err := r.Read(func(got []byte) {
		if !bytes.Equal(got, p) {
			t.Fatal("full ring yielded a different packet")
		}
		if want := calls * (testCapacity / 2); get32(mem, 4) != want {
			t.Fatalf("read during callback = %d, want %d", get32(mem, 4), want)
		}
		calls++
	})
	if n != 2 || calls != 2 || !wake || err != nil || get32(mem, 4) != testCapacity {
		t.Fatalf("Read = %d, %t, %v; calls = %d, read = %d", n, wake, err, calls, get32(mem, 4))
	}
	r.WriterWoken()
	if get32(mem, 12) != 0 {
		t.Fatal("WriterWoken left writerWaiting set")
	}
	writePacket(t, r, []byte{1})
	checkPackets(t, r, []byte{1})
}

func TestRingSleepRecheck(t *testing.T) {
	mem, r := newTestRing(t, 1)
	checkPackets(t, r)
	p := []byte{1}
	if ok, wake := r.TryWrite(p); !ok || wake {
		t.Fatalf("TryWrite before Sleep = %t, %t; want true, false", ok, wake)
	}
	if !r.Sleep() || get32(mem, 8) != 0 {
		t.Fatal("Sleep missed a write after the empty read")
	}
	checkPackets(t, r, p)
	if r.Sleep() || get32(mem, 8) != 1 {
		t.Fatal("Sleep on an empty ring did not arm the reader doorbell")
	}
	if ok, wake := r.TryWrite(p); !ok || !wake {
		t.Fatalf("TryWrite after Sleep = %t, %t; want true, true", ok, wake)
	}
	r.Woken()
	if get32(mem, 8) != 0 {
		t.Fatal("Woken left readerSleeping set")
	}
	checkPackets(t, r, p)
}

func TestRingConcurrentDoorbells(t *testing.T) {
	mem, writer := newTestRing(t, 5)
	reader, err := Open(mem, 5)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	readerBell, writerBell := make(chan struct{}, 1), make(chan struct{}, 1)
	ringBell := func(bell chan struct{}) {
		select {
		case bell <- struct{}{}:
		default:
		}
	}
	const count, size = 128, 8193
	writerDone := make(chan struct{})
	go func() {
		defer close(writerDone)
		for i := range count {
			p := bytes.Repeat([]byte{byte(i)}, size)
			for {
				ok, wake := writer.TryWrite(p)
				if wake {
					ringBell(readerBell)
				}
				if ok {
					break
				}
				select {
				case <-writerBell:
					writer.WriterWoken()
				case <-ctx.Done():
					return
				}
			}
		}
	}()
	defer func() {
		cancel()
		<-writerDone
	}()
	for i := 0; i < count; {
		_, wake, err := reader.Read(func(p []byte) {
			if i >= count || !bytes.Equal(p, bytes.Repeat([]byte{byte(i)}, size)) {
				t.Fatalf("unexpected payload at packet %d", i)
			}
			i++
		})
		if err != nil {
			t.Fatal(err)
		}
		if wake {
			ringBell(writerBell)
		}
		if i == count || reader.Sleep() {
			continue
		}
		select {
		case <-readerBell:
			reader.Woken()
		case <-ctx.Done():
			t.Fatalf("doorbell stalled after %d packets", i)
		}
	}
}

func TestRingRejectsBadHeader(t *testing.T) {
	for _, tt := range []struct {
		name   string
		offset int
		value  uint32
	}{
		{"magic", 24, 0},
		{"non power of two capacity", 28, testCapacity + 4},
		{"capacity mismatch", 28, testCapacity * 2},
		{"generation", 20, 0},
	} {
		t.Run(tt.name, func(t *testing.T) {
			const gen = 0x123456789abcdef
			mem, _ := newTestRing(t, gen)
			put32(mem, tt.offset, tt.value)
			before := bytes.Clone(mem)
			if _, err := Open(mem, gen); err == nil {
				t.Fatal("Open accepted a bad header")
			}
			if !bytes.Equal(mem, before) {
				t.Fatal("Open modified the mapping")
			}
		})
	}
	for _, mem := range [][]byte{nil, make([]byte, HeaderSize+32<<10), make([]byte, HeaderSize+testCapacity+4), make([]byte, HeaderSize+testCapacity+1)[1:]} {
		if _, err := Init(mem, 1); err == nil {
			t.Fatalf("Init accepted invalid mapping of length %d", len(mem))
		}
		if _, err := Open(mem, 1); err == nil {
			t.Fatalf("Open accepted invalid mapping of length %d", len(mem))
		}
	}
}

func TestRingRejectsBadData(t *testing.T) {
	for _, tt := range []struct {
		name  string
		edits map[int]uint32
	}{
		{"bad magic", map[int]uint32{24: 0}},
		{"changed capacity", map[int]uint32{28: testCapacity * 2}},
		{"reserved word", map[int]uint32{60: 1}},
		{"bad flag", map[int]uint32{12: 2}},
		{"misaligned read", map[int]uint32{4: 1}},
		{"misaligned write", map[int]uint32{0: 3}},
		{"too many bytes", map[int]uint32{0: testCapacity + 4}},
		{"length above packet limit", map[int]uint32{0: testCapacity, HeaderSize: testCapacity/2 + 1}},
		{"payload exceeds published bytes", map[int]uint32{0: 8, HeaderSize: 5}},
		{"invalid large prefix", map[int]uint32{0: 4, HeaderSize: 0xfffffffe}},
		{"wrap marker at start", map[int]uint32{0: testCapacity, HeaderSize: 0xffffffff}},
		{"early wrap marker", map[int]uint32{0: testCapacity, 4: 4, HeaderSize + 4: 0xffffffff}},
		{"truncated wrap marker", map[int]uint32{0: testCapacity - 4, 4: testCapacity - 8, HeaderSize + testCapacity - 8: 0xffffffff}},
		{"crosses end without marker", map[int]uint32{0: testCapacity + 8, 4: testCapacity - 4, HeaderSize + testCapacity - 4: 5}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			mem, r := newTestRing(t, 1)
			for offset, value := range tt.edits {
				put32(mem, offset, value)
			}
			before := get32(mem, 4)
			n, _, err := r.Read(func([]byte) { t.Fatal("delivered a malformed packet") })
			if err == nil || n != 0 || get32(mem, 4) != before {
				t.Fatalf("Read = %d, %v; read = %d, want %d", n, err, get32(mem, 4), before)
			}
		})
	}
}

// Golden replay starts with a zeroed mapping, capacity 65536 and generation
// 0x0102030405060708. Write then drain 32760 bytes of 0xa1, then write and
// drain 32760 bytes of 0xb2. Both counters are now 65528. Write the three
// bytes 00 7f ff (the record fits exactly at the end), then drain it.
// Write then drain 32760 bytes of 0xc3, then write and drain 32760 bytes of
// 0xd4. Both counters are now 131064. Write the five bytes 10 20 30 40 50
// (0xffffffff at data offset 65528, then the record at offset 0), then
// write ab at data offset 12. Capture the entire mapping before draining
// those last two packets. The final counters are write=131092, read=131064.
func TestRingGolden(t *testing.T) {
	mem, r := newTestRing(t, 0x0102030405060708)
	for _, b := range []byte{0xa1, 0xb2} {
		p := bytes.Repeat([]byte{b}, 32760)
		writePacket(t, r, p)
		checkPackets(t, r, p)
	}
	p := []byte{0, 0x7f, 0xff}
	writePacket(t, r, p)
	checkPackets(t, r, p)
	for _, b := range []byte{0xc3, 0xd4} {
		p := bytes.Repeat([]byte{b}, 32760)
		writePacket(t, r, p)
		checkPackets(t, r, p)
	}
	first, second := []byte{0x10, 0x20, 0x30, 0x40, 0x50}, []byte{0xab}
	writePacket(t, r, first)
	writePacket(t, r, second)
	if get32(mem, 0) != 131092 || get32(mem, 4) != 131064 || get32(mem, HeaderSize+65528) != 0xffffffff {
		t.Fatal("golden replay did not put the expected wrap marker and cursors in the mapping")
	}
	const name = "testdata/golden.bin"
	if *updateGolden {
		if err := os.WriteFile(name, mem, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	want, err := os.ReadFile(name)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(mem, want) {
		t.Fatal("mapping differs from testdata/golden.bin")
	}
	golden, err := Open(want, 0x0102030405060708)
	if err != nil {
		t.Fatal(err)
	}
	checkPackets(t, golden, first, second)
}

func FuzzReader(f *testing.F) {
	f.Add([]byte{}, []byte{})
	// Preserve the static header in some inputs so mutations reach the parser.
	cursors := make([]byte, 16)
	put32(cursors, 0, 8)
	f.Add(cursors, []byte{3, 0, 0, 0, 1, 2, 3, 0})
	f.Add(cursors, []byte{5, 0, 0, 0})
	f.Add(cursors, []byte{1, 128, 0, 0})
	f.Add(cursors, []byte{254, 255, 255, 255})
	f.Fuzz(func(t *testing.T, header, data []byte) {
		mem, r := newTestRing(t, 1)
		copy(mem[:HeaderSize], header)
		copy(mem[HeaderSize:], data)
		write, read := get32(mem, 0), get32(mem, 4)
		available := write - read
		wantError := (write|read)&3 != 0 || available > testCapacity ||
			get32(mem, 24) != 0x31524754 || get32(mem, 28) != testCapacity ||
			get32(mem, 8) > 1 || get32(mem, 12) > 1
		if available >= 4 && available <= testCapacity && read&3 == 0 {
			offset := read & (testCapacity - 1)
			length := get32(mem, HeaderSize+int(offset))
			if length != 0xffffffff && (length > testCapacity/2 || length > available-4) {
				wantError = true
			}
		}
		var total, calls int
		n, _, err := r.Read(func(p []byte) {
			total += len(p)
			calls++
			if len(p) > testCapacity/2 || total > testCapacity || calls > testCapacity/4 {
				t.Fatalf("invalid callback output: %d bytes in packet, %d bytes in %d calls", len(p), total, calls)
			}
		})
		if n != calls || (wantError && err == nil) {
			t.Fatalf("Read = %d, %v; callbacks = %d, expected error = %t", n, err, calls, wantError)
		}
	})
}
