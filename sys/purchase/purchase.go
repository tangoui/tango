// Package purchase sells in-app products through the platform store.
//
//	purchase.OnUpdate(app, func(t purchase.Transaction) { // before Run
//	    if t.State == purchase.Purchased || t.State == purchase.Restored {
//	        unlock(t.ProductID)
//	        purchase.Finish(app, t)
//	    }
//	})
//	ps, invalid, err := purchase.Products(app, "pro", "coins100")
//	t, err := purchase.Buy(app, "pro")
//
// iOS: StoreKit (SKProductsRequest and SKPaymentQueue — the Objective-C
// API; StoreKit 2 is Swift-only). The transaction observer is installed at
// launch, so transactions left over from an earlier run (an interrupted
// purchase, Ask to Buy approvals, renewals) arrive as updates after Started.
// Purchased and restored transactions stay in the queue until Finish:
// deliver the content first. Failed transactions are finished
// automatically. Products must exist in App Store Connect (or a StoreKit
// configuration in Xcode); otherwise they come back as invalid.
//
// Android: Google Play Billing needs the Play Billing library, which Tango
// does not bundle (the host has no third-party dependencies). Every call
// returns ErrUnsupported; see docs/mobile.md ("应用内购买") for wiring it in.
//
// Calls that wait for the store block; on Android call them from a
// goroutine (see docs/mobile.md). Desktop: ErrUnsupported.
package purchase

import (
	"errors"
	"slices"
	"sync"

	"github.com/tangoui/tango"
	"github.com/tangoui/tango/internal/corehook"
	"github.com/tangoui/tango/internal/platform"
	"github.com/tangoui/tango/sys/internal/capcall"
)

var (
	ErrUnsupported  = platform.ErrUnsupported
	ErrCanceled     = platform.ErrCanceled
	ErrNotAvailable = platform.ErrNotAvailable // payments disabled (parental controls, MDM)
	ErrNotFound     = platform.ErrNotFound     // unknown product / no receipt
)

type (
	Product     = platform.Product
	ProductKind = platform.ProductKind
	Transaction = platform.Transaction
	State       = platform.TransactionState
)

const (
	Consumable    = platform.ProductConsumable
	NonConsumable = platform.ProductNonConsumable
	Subscription  = platform.ProductSubscription

	Purchasing = platform.TxPurchasing
	Purchased  = platform.TxPurchased
	Failed     = platform.TxFailed
	Restored   = platform.TxRestored
	Deferred   = platform.TxDeferred
)

func backend(a *tango.App) (platform.Purchases, error) {
	return capcall.Backend[platform.Purchases](a)
}

// CanMakePayments reports whether the user may buy (false when restricted
// or unsupported).
func CanMakePayments(a *tango.App) bool {
	p, err := backend(a)
	if err != nil {
		return false
	}
	ok, _ := capcall.Sync(a, func() (bool, error) { return p.CanMakePayments(), nil })
	return ok
}

// Products looks products up by id. invalid lists the ids the store does
// not know.
func Products(a *tango.App, ids ...string) (products []Product, invalid []string, err error) {
	p, err := backend(a)
	if err != nil {
		return nil, nil, err
	}
	type res struct {
		ps  []Product
		bad []string
	}
	r, err := capcall.Async(a, func(done func(res, error)) {
		p.QueryProducts(ids, func(ps []Product, bad []string, err error) { done(res{ps, bad}, err) })
	})
	return r.ps, r.bad, err
}

// Buy purchases one unit of a product returned by Products and waits for
// the outcome: Purchased, or Deferred (awaiting approval; the final state
// arrives through OnUpdate). ErrCanceled when the user canceled.
func Buy(a *tango.App, productID string) (Transaction, error) {
	return BuyQuantity(a, productID, 1)
}

// BuyQuantity is Buy for n units of a consumable.
func BuyQuantity(a *tango.App, productID string, n int) (Transaction, error) {
	if productID == "" {
		return Transaction{}, errors.New("purchase: empty product id")
	}
	p, err := backend(a)
	if err != nil {
		return Transaction{}, err
	}
	return capcall.Async(a, func(done func(Transaction, error)) { p.Purchase(productID, max(n, 1), done) })
}

// Restore asks the store to re-deliver completed purchases of
// non-consumables and subscriptions; they arrive through OnUpdate with
// State Restored. It returns when the store is done.
func Restore(a *tango.App) error {
	p, err := backend(a)
	if err != nil {
		return err
	}
	_, err = capcall.Async(a, func(done func(struct{}, error)) {
		p.RestorePurchases(func(err error) { done(struct{}{}, err) })
	})
	return err
}

// Finish tells the store the app delivered t's content. Until then the
// store reports t again on every launch.
func Finish(a *tango.App, t Transaction) error {
	p, err := backend(a)
	if err != nil {
		return err
	}
	_, err = capcall.Sync(a, func() (struct{}, error) { return struct{}{}, p.FinishTransaction(t.ID) })
	return err
}

// Receipt returns the store receipt (iOS: the App Store receipt, PKCS #7)
// to validate purchases on your server.
func Receipt(a *tango.App) ([]byte, error) {
	p, err := backend(a)
	if err != nil {
		return nil, err
	}
	return capcall.Sync(a, p.PurchaseReceipt)
}

// OnUpdate calls fn on the main thread for every transaction update: the
// app's own purchases (each state), restores, and transactions the store
// delivers by itself. Register it before Run. Several functions may be
// registered; each sees every update. It does nothing where the backend
// has no store.
func OnUpdate(a *tango.App, fn func(Transaction)) {
	p, ok := a.Backend().(platform.Purchases)
	if !ok {
		return
	}
	v, loaded := observers.LoadOrStore(a, &observer{})
	o := v.(*observer)
	o.mu.Lock()
	o.fns = append(o.fns, fn)
	o.mu.Unlock()
	if loaded {
		return
	}
	deliver := func() {
		ts := p.TakePurchaseUpdates()
		o.mu.Lock()
		fns := slices.Clone(o.fns)
		o.mu.Unlock()
		for _, t := range ts {
			for _, f := range fns {
				// Each handler on its own: a panic in one does not skip
				// the other handlers or the remaining transactions.
				corehook.Guard(a, "event handler", func() { f(t) })
			}
		}
	}
	a.OnEventKind(platform.AppPurchaseUpdated, func(tango.AppEvent) { deliver() })
	// Backends may start the store lazily (iOS loads StoreKit on first use,
	// which installs the transaction observer): touch it once the app runs
	// so transactions left from an earlier run are delivered. Updates whose
	// event went by unclaimed are still queued in the backend.
	a.Post(func() {
		p.CanMakePayments()
		deliver()
	})
}

type observer struct {
	mu  sync.Mutex
	fns []func(Transaction)
}

var observers sync.Map // *tango.App -> *observer
