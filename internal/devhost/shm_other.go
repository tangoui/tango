//go:build !unix && !windows

package devhost

import "errors"

// Segment is unavailable on this platform.
type Segment struct{ Name string }

var errNoShm = errors.New("devhost: shared memory is not supported on this platform")

func CreateSegment(dir string, size int) (*Segment, error) { return nil, errNoShm }
func OpenSegment(name string, size int) (*Segment, error)  { return nil, errNoShm }
func (s *Segment) Bytes() []byte                           { return nil }
func (s *Segment) Unlink()                                 {}
func (s *Segment) Close() error                            { return nil }
