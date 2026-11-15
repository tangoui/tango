//go:build android && cgo

package main

import (
	"bytes"
	"os"
	"strconv"
)

// footprint reports the app process's resident set size in bytes (VmRSS of
// /proc/self/status; 0 if unknown). The self-test compares it before and
// after streaming tens of MiB through the asset handler. The WebView's
// renderer is another process; this one holds Go and the WebView's browser
// side.
func footprint() uint64 {
	b, err := os.ReadFile("/proc/self/status")
	if err != nil {
		return 0
	}
	for _, l := range bytes.Split(b, []byte("\n")) {
		v, ok := bytes.CutPrefix(l, []byte("VmRSS:"))
		if !ok {
			continue
		}
		f := bytes.Fields(v) // "123456 kB"
		if len(f) != 2 || string(f[1]) != "kB" {
			return 0
		}
		n, err := strconv.ParseUint(string(f[0]), 10, 64)
		if err != nil {
			return 0
		}
		return n << 10
	}
	return 0
}
