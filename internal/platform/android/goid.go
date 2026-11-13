package android

import (
	"bytes"
	"runtime"
	"strconv"
)

// goroutineID is the calling goroutine's id, parsed from its stack header.
func goroutineID() uint64 {
	var buf [64]byte
	n := runtime.Stack(buf[:], false)
	s := bytes.TrimPrefix(buf[:n], []byte("goroutine "))
	if i := bytes.IndexByte(s, ' '); i >= 0 {
		s = s[:i]
	}
	id, _ := strconv.ParseUint(string(s), 10, 64)
	return id
}
