package main

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"golang.org/x/sys/windows"
)

func TestDevPublishKeepsTargetOpenedWithoutDeleteSharing(t *testing.T) {
	dir := t.TempDir()
	src, dst := filepath.Join(dir, "new.exe"), filepath.Join(dir, "app.exe")
	writeDevTestFile(t, src, "new", 0o644)
	writeDevTestFile(t, dst, "old", 0o644)
	name, err := windows.UTF16PtrFromString(dst)
	if err != nil {
		t.Fatal(err)
	}
	h, err := windows.CreateFile(name, windows.GENERIC_READ, windows.FILE_SHARE_READ, nil, windows.OPEN_EXISTING, windows.FILE_ATTRIBUTE_NORMAL, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer windows.CloseHandle(h)
	if err := publishDevOutput(context.Background(), src, dst); err == nil {
		t.Fatal("replaced an executable opened without delete sharing")
	}
	if got, err := os.ReadFile(dst); err != nil || string(got) != "old" {
		t.Fatalf("old executable changed: %q (%v)", got, err)
	}
}
