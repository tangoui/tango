//go:build !(darwin && !ios) && !(linux && !android) && !windows

package pty

import "testing"

func TestUnsupported(t *testing.T) {
	if _, err := Start(Options{Path: "/bin/sh"}); err != ErrUnsupported {
		t.Errorf("Start error = %v, want ErrUnsupported", err)
	}
	if path, args := DefaultShell(); path != "" || args != nil {
		t.Errorf("DefaultShell() = %q, %q, want \"\", nil", path, args)
	}
}
