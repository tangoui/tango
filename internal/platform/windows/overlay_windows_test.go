//go:build windows && (amd64 || arm64)

package windows

import "testing"

func TestNativeBackendSupportsOverlay(t *testing.T) {
	if !(&Backend{}).SupportsOverlay() {
		t.Fatal("Windows backend does not declare overlay support")
	}
}
