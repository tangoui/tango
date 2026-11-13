//go:build darwin && !ios

package darwin

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/tangoui/tango/internal/platform"
)

func TestSecureStoreKeychain(t *testing.T) {
	if os.Getenv("CI") != "" {
		t.Skip("no login keychain in CI")
	}
	load()
	b := &backend{opts: platform.AppOptions{ID: "dev.tango.test-securestore"}}
	key := fmt.Sprintf("k-%d", time.Now().UnixNano())
	t.Cleanup(func() { _ = b.SecureDelete(key) })

	if _, err := b.SecureGet(key); !errors.Is(err, platform.ErrNotFound) {
		if err != nil && !errors.Is(err, platform.ErrNotFound) {
			t.Skipf("keychain unusable here: %v", err)
		}
		t.Fatalf("get missing: %v", err)
	}
	if err := b.SecureSet(key, []byte("one")); err != nil {
		t.Skipf("keychain unusable here: %v", err)
	}
	if v, err := b.SecureGet(key); err != nil || string(v) != "one" {
		t.Fatalf("get: %q %v", v, err)
	}
	val := []byte("two \x00 bytes 🔑")
	if err := b.SecureSet(key, val); err != nil {
		t.Fatalf("update: %v", err)
	}
	if v, err := b.SecureGet(key); err != nil || !bytes.Equal(v, val) {
		t.Fatalf("get after update: %q %v", v, err)
	}
	// Any goroutine.
	done := make(chan error)
	go func() { _, err := b.SecureGet(key); done <- err }()
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if err := b.SecureDelete(key); err != nil {
		t.Fatal(err)
	}
	if _, err := b.SecureGet(key); !errors.Is(err, platform.ErrNotFound) {
		t.Fatalf("after delete: %v", err)
	}
	if err := b.SecureDelete(key); err != nil {
		t.Fatalf("delete missing: %v", err)
	}
}

func TestBiometryKindNoPanic(t *testing.T) {
	load()
	b := &backend{}
	k, err := b.BiometryKind()
	t.Logf("biometry kind=%d err=%v", k, err)
	if k == platform.BiometryNone && err == nil {
		t.Fatal("BiometryNone without a reason")
	}
}
