//go:build darwin && !ios

package darwin

import "testing"

func TestMacISOKeyboardLoads(t *testing.T) {
	_ = macISOKeyboard() // must not crash, whatever keyboard this machine has
}
