//go:build !tango_dev && !tango_selftest

package lane

// Release builds ignore TANGO_IPC_LANE_FAULT (see fault.go).

func loadFault() (string, int, bool) { return "", 0, false }
