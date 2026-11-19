// Package shmring implements the single-producer, single-consumer ring for
// the WebView2 shared-memory IPC lanes. Each side serializes its own calls.
//
// The wire format is little-endian. The 64-byte header contains:
//
//	 @0  write          u32  bytes published by the writer, modulo 2^32
//	 @4  read           u32  bytes released by the reader, modulo 2^32
//	 @8  readerSleeping u32  reader-owned flag, 1 requests a doorbell
//	@12  writerWaiting  u32  writer-owned flag, 1 requests a doorbell
//	@16  gen            u64  generation, set once by Init
//	@24  magic          u32  0x31524754 ("TGR1")
//	@28  capacity       u32  data bytes, a power of two >= 64 KiB
//	@32 through @63          reserved, zero
//
// The data starts at byte 64. The position of a cursor is cursor &
// (capacity-1). A record is a u32 payload length, that many payload bytes
// (at most capacity/2), then zero padding to a multiple of four bytes.
// If the tail is too short for the whole record, write 0xffffffff at the
// current position and skip the rest of the tail. The skipped bytes after
// the marker are ignored and retain their previous values. The record then
// starts at data offset zero. An exact fit needs no marker.
//
// The writer publishes write after storing the whole record, and requests a
// doorbell if readerSleeping is 1. At the midpoint, a record larger than half
// the ring cannot fit together with the skipped tail. Publish only the wrap
// marker with writerWaiting set and retry the packet after the reader frees
// that space. The reader releases each record after its callback returns and
// requests a doorbell if writerWaiting is 1. Releasing a marker also frees
// space.
//
// On empty, the reader sets readerSleeping, then rechecks write. If data is
// available, it clears the flag and keeps draining. Otherwise it waits for a
// doorbell and clears the flag before draining. On full, the writer sets
// writerWaiting, then rechecks read. It clears the flag when it can write or
// on receiving a doorbell, then retries. Duplicate doorbells are harmless.
// Go uses 32-bit atomic accesses for the header except for the immutable
// generation. JavaScript on the WebView2 SharedBuffer uses ordinary accesses
// and relies on cross-process doorbell delivery for memory ordering.
package shmring

import (
	"encoding/binary"
	"errors"
	"math/bits"
	"sync/atomic"
	"unsafe"
)

const (
	HeaderSize = 64
	Magic      = uint32(0x31524754)

	writeOffset          = 0
	readOffset           = 4
	readerSleepingOffset = 8
	writerWaitingOffset  = 12
	genOffset            = 16
	magicOffset          = 24
	capacityOffset       = 28
	reservedOffset       = 32

	minCapacity = 64 << 10
	wrapMarker  = uint32(0xffffffff)
)

var (
	nativeLittleEndian = binary.NativeEndian.Uint32([]byte{1, 0, 0, 0}) == 1
	errHeader          = errors.New("shmring: invalid header")
	errCursors         = errors.New("shmring: invalid cursors")
	errLength          = errors.New("shmring: invalid record length")
	errWrap            = errors.New("shmring: invalid wrap marker")
)

// Ring is a view over the caller's shared memory. The mapping must stay
// valid while either endpoint uses it.
type Ring struct {
	mem      []byte
	capacity uint32
}

// Init initializes the header of a fresh, unshared mapping.
func Init(mem []byte, gen uint64) (*Ring, error) {
	r, err := newRing(mem)
	if err != nil {
		return nil, err
	}
	for offset := writeOffset; offset < genOffset; offset += 4 {
		r.store(offset, 0)
	}
	binary.LittleEndian.PutUint64(mem[genOffset:], gen)
	r.store(magicOffset, Magic)
	r.store(capacityOffset, r.capacity)
	for offset := reservedOffset; offset < HeaderSize; offset += 4 {
		r.store(offset, 0)
	}
	return r, nil
}

// Open checks the mapping's format, capacity and generation.
func Open(mem []byte, gen uint64) (*Ring, error) {
	r, err := newRing(mem)
	if err != nil {
		return nil, err
	}
	if !r.matchesFormat() || binary.LittleEndian.Uint64(mem[genOffset:]) != gen {
		return nil, errHeader
	}
	return r, nil
}

func newRing(mem []byte) (*Ring, error) {
	size := len(mem) - HeaderSize
	if size < minCapacity || uint64(size) > 1<<31 || size&(size-1) != 0 {
		return nil, errors.New("shmring: invalid mapping size")
	}
	if uintptr(unsafe.Pointer(&mem[0]))%4 != 0 {
		return nil, errors.New("shmring: unaligned mapping")
	}
	return &Ring{mem: mem, capacity: uint32(size)}, nil
}

func (r *Ring) load(offset int) uint32 {
	value := atomic.LoadUint32((*uint32)(unsafe.Pointer(&r.mem[offset])))
	if !nativeLittleEndian {
		value = bits.ReverseBytes32(value)
	}
	return value
}

func (r *Ring) store(offset int, value uint32) {
	if !nativeLittleEndian {
		value = bits.ReverseBytes32(value)
	}
	atomic.StoreUint32((*uint32)(unsafe.Pointer(&r.mem[offset])), value)
}

func (r *Ring) matchesFormat() bool {
	return r.load(magicOffset) == Magic && r.load(capacityOffset) == r.capacity
}

// MaxPacket returns the largest payload the ring can carry.
func (r *Ring) MaxPacket() int { return int(r.capacity / 2) }

func recordSize(length uint32) uint32 { return (length + 7) &^ 3 }

// TryWrite copies a packet into the ring if there is room. Send the reader
// doorbell when wakeReader is true, even if ok is false. In that case only a
// wrap marker was published and the packet still needs to be retried.
func (r *Ring) TryWrite(p []byte) (ok, wakeReader bool) {
	if len(p) > r.MaxPacket() {
		return false, false
	}
	write := r.load(writeOffset)
	if write&3 != 0 {
		return false, false
	}
	size := recordSize(uint32(len(p)))
	offset := write & (r.capacity - 1)
	tail := r.capacity - offset
	needed := size
	if tail < size {
		needed += tail
	}
	markerOnly := needed > r.capacity
	if markerOnly {
		needed = tail
	}
	if !r.hasSpace(write, needed) {
		r.store(writerWaitingOffset, 1)
		if !r.hasSpace(write, needed) {
			return false, false
		}
	}

	data := r.mem[HeaderSize:]
	if tail < size {
		binary.LittleEndian.PutUint32(data[offset:], wrapMarker)
		write += tail
		offset = 0
		if markerOnly {
			r.store(writerWaitingOffset, 1)
			return false, r.publish(write)
		}
	}
	binary.LittleEndian.PutUint32(data[offset:], uint32(len(p)))
	end := offset + 4 + uint32(len(p))
	copy(data[offset+4:end], p)
	clear(data[end : offset+size])
	r.WriterWoken()
	return true, r.publish(write + size)
}

func (r *Ring) hasSpace(write, needed uint32) bool {
	read := r.load(readOffset)
	used := write - read
	return read&3 == 0 && used <= r.capacity-needed
}

func (r *Ring) publish(write uint32) (wakeReader bool) {
	r.store(writeOffset, write)
	return r.load(readerSleepingOffset) == 1
}

// Read drains the records published when it snapshots write. The payload
// aliases the mapping and is valid until dst returns. A caller that wants
// to keep it must copy it. Only successfully delivered packets count in n.
func (r *Ring) Read(dst func(p []byte)) (n int, wakeWriter bool, err error) {
	if !r.matchesFormat() || r.load(readerSleepingOffset) > 1 || r.load(writerWaitingOffset) > 1 {
		return 0, false, errHeader
	}
	for offset := reservedOffset; offset < HeaderSize; offset += 4 {
		if r.load(offset) != 0 {
			return 0, false, errHeader
		}
	}
	read, write := r.load(readOffset), r.load(writeOffset)
	if (read|write)&3 != 0 || write-read > r.capacity {
		return 0, false, errCursors
	}
	start := read
	defer func() {
		wakeWriter = read != start && r.load(writerWaitingOffset) == 1
	}()

	data := r.mem[HeaderSize:]
	for read != write {
		offset := read & (r.capacity - 1)
		tail, available := r.capacity-offset, write-read
		length := binary.LittleEndian.Uint32(data[offset:])
		if length == wrapMarker {
			if tail > r.capacity/2 || tail > available {
				return n, false, errWrap
			}
			read += tail
		} else {
			if length > r.capacity/2 {
				return n, false, errLength
			}
			size := recordSize(length)
			if size > available || size > tail {
				return n, false, errLength
			}
			end := offset + 4 + length
			dst(data[offset+4 : end : end])
			n++
			read += size
		}
		r.store(readOffset, read)
	}
	return n, false, nil
}

// Sleep arms the reader doorbell and checks for data published since Read.
// If mustDrain is true, keep draining without waiting for a doorbell.
func (r *Ring) Sleep() (mustDrain bool) {
	r.store(readerSleepingOffset, 1)
	if r.load(readOffset) == r.load(writeOffset) {
		return false
	}
	r.Woken()
	return true
}

// Woken clears the reader's doorbell request before draining.
func (r *Ring) Woken() { r.store(readerSleepingOffset, 0) }

// WriterWoken clears the writer's doorbell request before retrying.
func (r *Ring) WriterWoken() { r.store(writerWaitingOffset, 0) }
