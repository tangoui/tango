//go:build linux && !android

package ime

import "testing"

func TestSystemDrawsCandidatesLinux(t *testing.T) {
	if !SystemDrawsCandidates() {
		t.Fatal("Linux engines use the system candidate panel")
	}
}
