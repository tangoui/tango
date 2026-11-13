package securestore_test

import (
	"errors"
	"sync"
	"testing"

	"github.com/tangoui/tango"
	"github.com/tangoui/tango/internal/platform/fake/faketest"
	"github.com/tangoui/tango/sys/securestore"
)

func TestStore(t *testing.T) {
	a, fb := faketest.Run(t, tango.Options{})
	if _, err := securestore.Get(a, "k"); !errors.Is(err, securestore.ErrNotFound) {
		t.Fatal(err)
	}
	if err := securestore.SetString(a, "k", "secret"); err != nil {
		t.Fatal(err)
	}
	if v, err := securestore.GetString(a, "k"); v != "secret" || err != nil {
		t.Fatal(v, err)
	}
	if err := securestore.Set(a, "empty", nil); err != nil {
		t.Fatal(err)
	}
	if v, err := securestore.Get(a, "empty"); err != nil || v == nil || len(v) != 0 {
		t.Fatal(v, err)
	}
	if err := securestore.Delete(a, "k"); err != nil {
		t.Fatal(err)
	}
	if err := securestore.Delete(a, "k"); err != nil {
		t.Fatal("deleting a missing key:", err)
	}
	if _, err := securestore.Get(a, "k"); !errors.Is(err, securestore.ErrNotFound) {
		t.Fatal(err)
	}
	if err := securestore.Set(a, "", []byte("x")); err == nil {
		t.Fatal("empty key accepted")
	}
	// Any goroutine.
	var wg sync.WaitGroup
	for i := range 8 {
		wg.Go(func() {
			key := string(rune('a' + i))
			if err := securestore.SetString(a, key, key); err != nil {
				t.Error(err)
			}
			if v, _ := securestore.GetString(a, key); v != key {
				t.Error(v)
			}
		})
	}
	wg.Wait()
	boom := errors.New("keychain locked")
	fb.SetSecureErr(boom)
	if err := securestore.SetString(a, "k", "v"); err != boom {
		t.Fatal(err)
	}
	m, _ := faketest.RunMinimal(t, tango.Options{})
	if _, err := securestore.Get(m, "k"); !errors.Is(err, securestore.ErrUnsupported) {
		t.Fatal(err)
	}
}
