package fake

import (
	"slices"
	"sync"
	"time"

	"github.com/tangoui/tango/internal/platform"
)

// Round-4 mobile capabilities of the fake backend (mobile_ext.go):
// BackgroundTasks, Purchases, StoreReview, SettingsOpener, ScreenControl,
// StatusBarControl, Keyboard and Widgets. Like mobile.go, the state lives
// beside Backend; helpers simulate the OS (a task starting, a transaction
// arriving, the keyboard showing).

type mobile2State struct {
	mu sync.Mutex

	scheduled map[platform.BackgroundKind]platform.BackgroundRequest
	bgQueue   []platform.BackgroundTask
	finished  map[int64]bool // id -> success
	nextTask  int64

	canPay    bool
	products  map[string]platform.Product
	txQueue   []platform.Transaction
	txFinish  []string
	buyResult map[string]platform.Transaction // product -> answer; default purchased
	restore   []platform.Transaction
	receipt   []byte
	nextTx    int

	reviews, settings int
	keepOn            bool
	brightness        float64
	statusBars        []platform.StatusBar
	keyboard          float64
	widget            *platform.WidgetContent
}

var mobile2States sync.Map // *Backend -> *mobile2State

func (b *Backend) mobile2() *mobile2State {
	if v, ok := mobile2States.Load(b); ok {
		return v.(*mobile2State)
	}
	v, _ := mobile2States.LoadOrStore(b, &mobile2State{
		scheduled:  map[platform.BackgroundKind]platform.BackgroundRequest{},
		finished:   map[int64]bool{},
		canPay:     true,
		products:   map[string]platform.Product{},
		buyResult:  map[string]platform.Transaction{},
		brightness: 0.5,
	})
	return v.(*mobile2State)
}

func (b *Backend) with2(fn func(m *mobile2State)) {
	m := b.mobile2()
	m.mu.Lock()
	defer m.mu.Unlock()
	fn(m)
}

var (
	_ platform.BackgroundTasks  = (*Backend)(nil)
	_ platform.Purchases        = (*Backend)(nil)
	_ platform.StoreReview      = (*Backend)(nil)
	_ platform.SettingsOpener   = (*Backend)(nil)
	_ platform.ScreenControl    = (*Backend)(nil)
	_ platform.StatusBarControl = (*Backend)(nil)
	_ platform.Keyboard         = (*Backend)(nil)
	_ platform.Widgets          = (*Backend)(nil)
)

// ---- BackgroundTasks ----

func (b *Backend) ScheduleBackground(r platform.BackgroundRequest) error {
	b.mustMain("ScheduleBackground")
	if r.Kind != platform.BackgroundRefresh && r.Kind != platform.BackgroundProcessing {
		return platform.ErrUnsupported
	}
	b.with2(func(m *mobile2State) { m.scheduled[r.Kind] = r })
	return nil
}

func (b *Backend) CancelBackground(k platform.BackgroundKind) error {
	b.mustMain("CancelBackground")
	b.with2(func(m *mobile2State) { delete(m.scheduled, k) })
	return nil
}

// Scheduled returns the pending request of kind k.
func (b *Backend) Scheduled(k platform.BackgroundKind) (r platform.BackgroundRequest, ok bool) {
	b.with2(func(m *mobile2State) { r, ok = m.scheduled[k] })
	return
}

func (b *Backend) TakeBackgroundTasks() (out []platform.BackgroundTask) {
	b.mustMain("TakeBackgroundTasks")
	b.with2(func(m *mobile2State) { out, m.bgQueue = m.bgQueue, nil })
	return
}

func (b *Backend) FinishBackgroundTask(id int64, success bool) {
	b.mustMain("FinishBackgroundTask")
	b.with2(func(m *mobile2State) {
		if _, done := m.finished[id]; !done {
			m.finished[id] = success
		}
	})
}

// RunBackgroundTask simulates the system starting the pending task of kind
// k (which it unschedules) with the given time budget. It returns the
// task id, or 0 when the loop stopped.
func (b *Backend) RunBackgroundTask(k platform.BackgroundKind, budget time.Duration) int64 {
	var id int64
	b.with2(func(m *mobile2State) {
		m.nextTask++
		id = m.nextTask
		delete(m.scheduled, k)
		m.bgQueue = append(m.bgQueue, platform.BackgroundTask{ID: id, Kind: k, Deadline: time.Now().Add(budget)})
	})
	if !b.AppEvent(platform.AppEvent{Kind: platform.AppBackgroundTask}) {
		return 0
	}
	return id
}

// ExpireBackgroundTask simulates the system asking task id to stop.
func (b *Backend) ExpireBackgroundTask(id int64, k platform.BackgroundKind) bool {
	b.with2(func(m *mobile2State) {
		m.bgQueue = append(m.bgQueue, platform.BackgroundTask{ID: id, Kind: k, Expired: true})
	})
	return b.AppEvent(platform.AppEvent{Kind: platform.AppBackgroundTask})
}

// BackgroundFinished reports whether task id was finished, and its result.
func (b *Backend) BackgroundFinished(id int64) (success, done bool) {
	b.with2(func(m *mobile2State) { success, done = m.finished[id] })
	return
}

// ---- Purchases ----

// SetProducts sets what the store knows; SetCanPay what CanMakePayments says.
func (b *Backend) SetProducts(ps ...platform.Product) {
	b.with2(func(m *mobile2State) {
		for _, p := range ps {
			m.products[p.ID] = p
		}
	})
}

func (b *Backend) SetCanPay(ok bool) { b.with2(func(m *mobile2State) { m.canPay = ok }) }

// SetPurchaseResult makes Purchase(product) end with t (State, Error,
// Canceled); the default is TxPurchased.
func (b *Backend) SetPurchaseResult(product string, t platform.Transaction) {
	b.with2(func(m *mobile2State) { m.buyResult[product] = t })
}

// SetRestorable sets the transactions RestorePurchases re-delivers.
func (b *Backend) SetRestorable(ts ...platform.Transaction) {
	b.with2(func(m *mobile2State) { m.restore = ts })
}

func (b *Backend) SetReceipt(r []byte) { b.with2(func(m *mobile2State) { m.receipt = r }) }

// FinishedTransactions returns the ids passed to FinishTransaction.
func (b *Backend) FinishedTransactions() (out []string) {
	b.with2(func(m *mobile2State) { out = slices.Clone(m.txFinish) })
	return
}

func (b *Backend) CanMakePayments() (ok bool) {
	b.mustMain("CanMakePayments")
	b.with2(func(m *mobile2State) { ok = m.canPay })
	return
}

func (b *Backend) QueryProducts(ids []string, done func([]platform.Product, []string, error)) {
	b.mustMain("QueryProducts")
	var ps []platform.Product
	var bad []string
	b.with2(func(m *mobile2State) {
		for _, id := range ids {
			if p, ok := m.products[id]; ok {
				ps = append(ps, p)
			} else {
				bad = append(bad, id)
			}
		}
	})
	b.enqueue(func() { done(ps, bad, nil) })
}

func (b *Backend) Purchase(productID string, quantity int, done func(platform.Transaction, error)) {
	b.mustMain("Purchase")
	var t platform.Transaction
	var err error
	b.with2(func(m *mobile2State) {
		if _, ok := m.products[productID]; !ok {
			err = platform.ErrNotFound
			return
		}
		if !m.canPay {
			err = platform.ErrNotAvailable
			return
		}
		t = platform.Transaction{State: platform.TxPurchased}
		if r, ok := m.buyResult[productID]; ok {
			t = r
		}
		m.nextTx++
		t.ProductID, t.Quantity, t.Time = productID, max(quantity, 1), time.Now()
		if t.State == platform.TxPurchased && t.ID == "" {
			t.ID = "tx" + itoa(m.nextTx)
		}
		m.txQueue = append(m.txQueue, t)
		if t.Canceled {
			err = platform.ErrCanceled
		}
	})
	if err != nil && t.State == 0 {
		b.enqueue(func() { done(platform.Transaction{}, err) })
		return
	}
	b.enqueue(func() {
		if hx, ok := b.hostEx(); ok {
			hx.OnAppEvent(platform.AppEvent{Kind: platform.AppPurchaseUpdated})
		}
		done(t, err)
	})
}

func (b *Backend) RestorePurchases(done func(error)) {
	b.mustMain("RestorePurchases")
	b.with2(func(m *mobile2State) {
		for _, t := range m.restore {
			t.State = platform.TxRestored
			m.txQueue = append(m.txQueue, t)
		}
	})
	b.enqueue(func() {
		if hx, ok := b.hostEx(); ok {
			hx.OnAppEvent(platform.AppEvent{Kind: platform.AppPurchaseUpdated})
		}
		done(nil)
	})
}

// StoreTransaction simulates the store reporting t on its own (a renewal,
// an interrupted purchase completing).
func (b *Backend) StoreTransaction(t platform.Transaction) bool {
	b.with2(func(m *mobile2State) { m.txQueue = append(m.txQueue, t) })
	return b.AppEvent(platform.AppEvent{Kind: platform.AppPurchaseUpdated})
}

func (b *Backend) TakePurchaseUpdates() (out []platform.Transaction) {
	b.mustMain("TakePurchaseUpdates")
	b.with2(func(m *mobile2State) { out, m.txQueue = m.txQueue, nil })
	return
}

func (b *Backend) FinishTransaction(id string) error {
	b.mustMain("FinishTransaction")
	b.with2(func(m *mobile2State) { m.txFinish = append(m.txFinish, id) })
	return nil
}

func (b *Backend) PurchaseReceipt() (r []byte, err error) {
	b.mustMain("PurchaseReceipt")
	b.with2(func(m *mobile2State) { r = slices.Clone(m.receipt) })
	if r == nil {
		err = platform.ErrNotFound
	}
	return
}

// ---- device controls ----

func (b *Backend) RequestReview() error {
	b.mustMain("RequestReview")
	b.with2(func(m *mobile2State) { m.reviews++ })
	return nil
}

func (b *Backend) OpenAppSettings() error {
	b.mustMain("OpenAppSettings")
	b.with2(func(m *mobile2State) { m.settings++ })
	return nil
}

// DeviceCalls counts RequestReview and OpenAppSettings calls.
func (b *Backend) DeviceCalls() (reviews, settings int) {
	b.with2(func(m *mobile2State) { reviews, settings = m.reviews, m.settings })
	return
}

func (b *Backend) SetKeepScreenOn(on bool) error {
	b.mustMain("SetKeepScreenOn")
	b.with2(func(m *mobile2State) { m.keepOn = on })
	return nil
}

func (b *Backend) KeepScreenOn() (on bool) {
	b.with2(func(m *mobile2State) { on = m.keepOn })
	return
}

func (b *Backend) Brightness() (v float64, err error) {
	b.mustMain("Brightness")
	b.with2(func(m *mobile2State) { v = m.brightness })
	return
}

func (b *Backend) SetBrightness(v float64) error {
	b.mustMain("SetBrightness")
	if v > 1 {
		v = 1
	}
	if v < 0 {
		v = 0.5 // "system" brightness
	}
	b.with2(func(m *mobile2State) { m.brightness = v })
	return nil
}

func (b *Backend) SetStatusBar(s platform.StatusBar) error {
	b.mustMain("SetStatusBar")
	b.with2(func(m *mobile2State) { m.statusBars = append(m.statusBars, s) })
	return nil
}

// StatusBars returns every SetStatusBar call, in order.
func (b *Backend) StatusBars() (out []platform.StatusBar) {
	b.with2(func(m *mobile2State) { out = slices.Clone(m.statusBars) })
	return
}

func (b *Backend) KeyboardHeight() (h float64) {
	b.mustMain("KeyboardHeight")
	b.with2(func(m *mobile2State) { h = m.keyboard })
	return
}

func (b *Backend) HideKeyboard() error {
	b.mustMain("HideKeyboard")
	var was float64
	b.with2(func(m *mobile2State) { was, m.keyboard = m.keyboard, 0 })
	if was != 0 {
		b.enqueue(func() {
			if hx, ok := b.hostEx(); ok {
				hx.OnAppEvent(platform.AppEvent{Kind: platform.AppKeyboardChanged})
			}
		})
	}
	return nil
}

// ShowKeyboard simulates the keyboard appearing with height h (0 hides it).
func (b *Backend) ShowKeyboard(h float64) bool {
	b.with2(func(m *mobile2State) { m.keyboard = h })
	return b.AppEvent(platform.AppEvent{Kind: platform.AppKeyboardChanged, Insets: platform.Insets{Bottom: h}})
}

// ---- Widgets ----

func (b *Backend) UpdateWidget(c platform.WidgetContent) error {
	b.mustMain("UpdateWidget")
	b.with2(func(m *mobile2State) { m.widget = &c })
	return nil
}

// Widget returns the last widget content.
func (b *Backend) Widget() (c platform.WidgetContent, ok bool) {
	b.with2(func(m *mobile2State) {
		if m.widget != nil {
			c, ok = *m.widget, true
		}
	})
	return
}

func itoa(i int) string {
	if i == 0 {
		return "0"
	}
	var s []byte
	for ; i > 0; i /= 10 {
		s = append([]byte{byte('0' + i%10)}, s...)
	}
	return string(s)
}
