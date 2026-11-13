package tray

// SetGOOS pretends to run on os until the returned function is called.
func SetGOOS(os string) (restore func()) {
	old := goos
	goos = os
	return func() { goos = old }
}
