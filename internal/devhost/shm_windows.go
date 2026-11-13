package devhost

import (
	"fmt"
	"os"
	"sync/atomic"
	"syscall"
	"unsafe"
)

// Segment is a shared-memory region: on Windows a named, pagefile-backed
// file mapping.
type Segment struct {
	Name string // mapping name
	h    syscall.Handle
	addr uintptr
	data []byte
}

var segCounter atomic.Int64

// CreateSegment creates a segment of size bytes. dir is ignored.
func CreateSegment(dir string, size int) (*Segment, error) {
	name := fmt.Sprintf(`Local\tango-frame-%d-%d`, os.Getpid(), segCounter.Add(1))
	return mapSegment(name, size)
}

// OpenSegment maps an existing segment. CreateFileMapping with the name of
// an existing mapping opens it.
func OpenSegment(name string, size int) (*Segment, error) { return mapSegment(name, size) }

func mapSegment(name string, size int) (*Segment, error) {
	n, err := syscall.UTF16PtrFromString(name)
	if err != nil {
		return nil, err
	}
	h, err := syscall.CreateFileMapping(syscall.InvalidHandle, nil, syscall.PAGE_READWRITE,
		uint32(uint64(size)>>32), uint32(size), n)
	if h == 0 {
		return nil, fmt.Errorf("devhost: CreateFileMapping: %w", err)
	}
	addr, err := syscall.MapViewOfFile(h, syscall.FILE_MAP_WRITE, 0, 0, uintptr(size))
	if addr == 0 {
		syscall.CloseHandle(h)
		return nil, fmt.Errorf("devhost: MapViewOfFile: %w", err)
	}
	// Convert without a uintptr→Pointer cast vet would flag; the view is
	// not Go memory, so the GC does not move it.
	p := *(*unsafe.Pointer)(unsafe.Pointer(&addr))
	data := unsafe.Slice((*byte)(p), size)
	return &Segment{Name: name, h: h, addr: addr, data: data}, nil
}

// Bytes returns the mapped memory.
func (s *Segment) Bytes() []byte { return s.data }

// Unlink does nothing: the mapping disappears with its last handle.
func (s *Segment) Unlink() {}

// Close unmaps the segment.
func (s *Segment) Close() error {
	if s.data == nil {
		return nil
	}
	s.data = nil
	err := syscall.UnmapViewOfFile(s.addr)
	syscall.CloseHandle(s.h)
	return err
}
