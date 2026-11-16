//go:build !tango_dev && !tango_selftest

package downq

// Release builds ignore TANGO_IPC_LANE_FAULT (see fault.go).

func loadFault() (int, bool) { return 0, false }

func noteFault(gen, seq uint64) {}
