package kv

import (
	"os"
	"path/filepath"
	"syscall"
	"testing"
	"time"
)

// holdWithoutDelete opens path the way a scanner might: shared for reading
// and writing but not for deletion, so it cannot be replaced while open.
func holdWithoutDelete(t *testing.T, path string) syscall.Handle {
	t.Helper()
	p, err := syscall.UTF16PtrFromString(path)
	if err != nil {
		t.Fatal(err)
	}
	h, err := syscall.CreateFile(p, syscall.GENERIC_READ, syscall.FILE_SHARE_READ|syscall.FILE_SHARE_WRITE, nil, syscall.OPEN_EXISTING, syscall.FILE_ATTRIBUTE_NORMAL, 0)
	if err != nil {
		t.Fatal(err)
	}
	return h
}

// TestSetWhileHeld: a Set that lands while another process briefly holds
// the store file open waits for it and is not lost.
func TestSetWhileHeld(t *testing.T) {
	path := filepath.Join(t.TempDir(), "s.json")
	s, err := New(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Set("a", 1); err != nil {
		t.Fatal(err)
	}
	h := holdWithoutDelete(t, path)

	// The hold does block a plain rename, so the retry is what is tested.
	tmp := filepath.Join(filepath.Dir(path), "plain.tmp")
	if err := os.WriteFile(tmp, []byte("{}"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(tmp, path); err == nil || !replaceHeld(err) {
		t.Fatalf("rename over a held file: %v, want a sharing error", err)
	}

	go func() {
		time.Sleep(100 * time.Millisecond)
		syscall.CloseHandle(h)
	}()
	if err := s.Set("b", 2); err != nil {
		t.Fatalf("Set while held: %v", err)
	}
	re, err := New(path)
	if err != nil {
		t.Fatal(err)
	}
	if !re.Has("a") || !re.Has("b") {
		t.Fatalf("reloaded keys %v, want a and b", re.Keys())
	}
}

// TestSetHeldTooLong: a hold that outlasts the retries fails the Set with
// the sharing error instead of hanging, and leaves the store unchanged.
func TestSetHeldTooLong(t *testing.T) {
	path := filepath.Join(t.TempDir(), "s.json")
	s, err := New(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Set("a", 1); err != nil {
		t.Fatal(err)
	}
	h := holdWithoutDelete(t, path)
	defer syscall.CloseHandle(h)
	start := time.Now()
	if err := s.Set("b", 2); err == nil || !replaceHeld(err) {
		t.Fatalf("Set held for good: %v, want a sharing error", err)
	}
	if d := time.Since(start); d < replaceTimeout || d > 5*replaceTimeout {
		t.Errorf("gave up after %v, want about %v", d, replaceTimeout)
	}
	if s.Has("b") {
		t.Error("failed Set changed the store")
	}
}
