package purchase_test

import (
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/tangoui/tango"
	"github.com/tangoui/tango/internal/platform"
	"github.com/tangoui/tango/internal/platform/fake/faketest"
	"github.com/tangoui/tango/sys/purchase"
)

func TestStore(t *testing.T) {
	a, fb := faketest.NewApp(tango.Options{})
	updates := make(chan purchase.Transaction, 8)
	purchase.OnUpdate(a, func(tx purchase.Transaction) { updates <- tx })
	second := make(chan purchase.Transaction, 8)
	purchase.OnUpdate(a, func(tx purchase.Transaction) { second <- tx })
	faketest.Start(t, a, fb)

	fb.SetProducts(platform.Product{ID: "pro", Title: "Pro", Price: "$1.99", PriceValue: 1.99, Currency: "USD", Kind: purchase.NonConsumable})
	if !purchase.CanMakePayments(a) {
		t.Fatal("cannot pay")
	}
	ps, bad, err := purchase.Products(a, "pro", "nope")
	if err != nil || len(ps) != 1 || ps[0].Title != "Pro" || !slices.Equal(bad, []string{"nope"}) {
		t.Fatal(ps, bad, err)
	}
	tx, err := purchase.Buy(a, "pro")
	if err != nil || tx.State != purchase.Purchased || tx.ID == "" || tx.ProductID != "pro" {
		t.Fatalf("%+v %v", tx, err)
	}
	if u := recv(t, updates); u.ID != tx.ID {
		t.Fatalf("%+v", u)
	}
	if u := recv(t, second); u.ID != tx.ID {
		t.Fatalf("%+v", u)
	}
	if err := purchase.Finish(a, tx); err != nil || !slices.Equal(fb.FinishedTransactions(), []string{tx.ID}) {
		t.Fatal(err, fb.FinishedTransactions())
	}

	fb.SetPurchaseResult("pro", platform.Transaction{State: purchase.Failed, Canceled: true})
	if _, err := purchase.Buy(a, "pro"); !errors.Is(err, purchase.ErrCanceled) {
		t.Fatal(err)
	}
	recv(t, updates)
	if _, err := purchase.Buy(a, "nope"); !errors.Is(err, purchase.ErrNotFound) {
		t.Fatal(err)
	}

	fb.SetRestorable(platform.Transaction{ID: "r1", OriginalID: "o1", ProductID: "pro"})
	if err := purchase.Restore(a); err != nil {
		t.Fatal(err)
	}
	if u := recv(t, updates); u.State != purchase.Restored || u.OriginalID != "o1" {
		t.Fatalf("%+v", u)
	}
	fb.StoreTransaction(platform.Transaction{ID: "renew", ProductID: "sub", State: purchase.Purchased})
	if u := recv(t, updates); u.ID != "renew" {
		t.Fatalf("%+v", u)
	}

	if _, err := purchase.Receipt(a); !errors.Is(err, purchase.ErrNotFound) {
		t.Fatal(err)
	}
	fb.SetReceipt([]byte("r"))
	if r, err := purchase.Receipt(a); err != nil || string(r) != "r" {
		t.Fatal(r, err)
	}
}

func TestUnsupported(t *testing.T) {
	m, _ := faketest.RunMinimal(t, tango.Options{})
	if purchase.CanMakePayments(m) {
		t.Fatal("can pay")
	}
	if _, _, err := purchase.Products(m, "x"); !errors.Is(err, purchase.ErrUnsupported) {
		t.Fatal(err)
	}
	if _, err := purchase.Buy(m, "x"); !errors.Is(err, purchase.ErrUnsupported) {
		t.Fatal(err)
	}
}

func recv[T any](t *testing.T, c chan T) T {
	t.Helper()
	select {
	case v := <-c:
		return v
	case <-time.After(faketest.Timeout):
		t.Fatal("update missing")
	}
	panic("unreachable")
}
