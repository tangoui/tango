//go:build unix

package devhost

import (
	"fmt"
	"os"
	"syscall"
)

// Segment is a shared-memory region: on unix a temporary file mapped
// MAP_SHARED by both processes.
type Segment struct {
	Name string // file path
	data []byte
}

// CreateSegment creates a segment of size bytes in dir ("" = os.TempDir).
func CreateSegment(dir string, size int) (*Segment, error) {
	f, err := os.CreateTemp(dir, "tango-frame-*")
	if err != nil {
		return nil, err
	}
	defer f.Close()
	if err := f.Truncate(int64(size)); err != nil {
		os.Remove(f.Name())
		return nil, err
	}
	data, err := syscall.Mmap(int(f.Fd()), 0, size, syscall.PROT_READ|syscall.PROT_WRITE, syscall.MAP_SHARED)
	if err != nil {
		os.Remove(f.Name())
		return nil, fmt.Errorf("devhost: mmap: %w", err)
	}
	return &Segment{Name: f.Name(), data: data}, nil
}

// OpenSegment maps an existing segment.
func OpenSegment(name string, size int) (*Segment, error) {
	f, err := os.OpenFile(name, os.O_RDWR, 0)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil {
		return nil, err
	}
	if st.Size() < int64(size) {
		return nil, fmt.Errorf("devhost: segment %s is %d bytes, want %d", name, st.Size(), size)
	}
	data, err := syscall.Mmap(int(f.Fd()), 0, size, syscall.PROT_READ|syscall.PROT_WRITE, syscall.MAP_SHARED)
	if err != nil {
		return nil, fmt.Errorf("devhost: mmap: %w", err)
	}
	return &Segment{Name: name, data: data}, nil
}

// Bytes returns the mapped memory.
func (s *Segment) Bytes() []byte { return s.data }

// Unlink removes the segment's name; existing mappings stay valid.
func (s *Segment) Unlink() { _ = os.Remove(s.Name) }

// Close unmaps the segment.
func (s *Segment) Close() error {
	if s.data == nil {
		return nil
	}
	err := syscall.Munmap(s.data)
	s.data = nil
	return err
}
