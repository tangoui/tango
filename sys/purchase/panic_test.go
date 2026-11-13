package purchase_test

import (
	"sync"
	"testing"

	"github.com/tangoui/tango"
	"github.com/tangoui/tango/internal/platform"
	"github.com/tangoui/tango/internal/platform/fake/faketest"
	"github.com/tangoui/tango/sys/purchase"
)

// A transaction update reaches every OnUpdate handler even when one
// panics; the panic goes to OnPanic as "event handler".
func TestUpdatePanic(t *testing.T) {
	var mu sync.Mutex
	var where []string
	a, fb := faketest.NewApp(tango.Options{OnPanic: func(p tango.Panic) bool {
		mu.Lock()
		where = append(where, p.Where)
		mu.Unlock()
		return true
	}})
	purchase.OnUpdate(a, func(purchase.Transaction) { panic("boom") })
	second := make(chan purchase.Transaction, 8)
	purchase.OnUpdate(a, func(tx purchase.Transaction) { second <- tx })
	faketest.Start(t, a, fb)

	fb.SetProducts(platform.Product{ID: "pro", Title: "Pro", Kind: purchase.NonConsumable})
	tx, err := purchase.Buy(a, "pro")
	if err != nil {
		t.Fatal(err)
	}
	if u := recv(t, second); u.ID != tx.ID {
		t.Fatalf("%+v", u)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(where) == 0 {
		t.Fatal("panic not reported")
	}
	for _, w := range where {
		if w != "event handler" {
			t.Fatalf("where %q", where)
		}
	}
}
