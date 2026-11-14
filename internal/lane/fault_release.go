//go:build !tango_dev

package lane

// Release builds ignore TANGO_IPC_LANE_FAULT (see fault.go).

func loadFault() (string, int, bool) { return "", 0, false }
