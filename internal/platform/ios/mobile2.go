//go:build ios

package ios

/*
#include <stdlib.h>
#include "mobile.h"
#include "mobile2.h"
*/
import "C"

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strings"
	"time"
	"unsafe"

	"github.com/tangoui/tango/internal/platform"
)

// Round-4 mobile capabilities (mobile_ext.go), implemented in mobile2.m.
// StoreKit and BackgroundTasks are loaded at run time (see mobile2.m), so
// the package links no extra framework for them.

var (
	_ platform.BackgroundTasks  = (*backend)(nil)
	_ platform.Purchases        = (*backend)(nil)
	_ platform.StoreReview      = (*backend)(nil)
	_ platform.SettingsOpener   = (*backend)(nil)
	_ platform.ScreenControl    = (*backend)(nil)
	_ platform.StatusBarControl = (*backend)(nil)
	_ platform.Keyboard         = (*backend)(nil)
)

// Updates waiting for TakeBackgroundTasks / TakePurchaseUpdates. Main
// thread only. They also collect what arrives before App.Run (a
// background launch, transactions from an earlier run); the signal events
// are queued like links (emitEvent).
var (
	bgUpdates []platform.BackgroundTask
	txUpdates []platform.Transaction
)

//export tangoIOSBackgroundTask
func tangoIOSBackgroundTask(id C.int64_t, kind C.int, expired C.int) {
	bgUpdates = append(bgUpdates, platform.BackgroundTask{
		ID: int64(id), Kind: platform.BackgroundKind(kind), Expired: expired != 0,
	})
	emitEvent(platform.AppEvent{Kind: platform.AppBackgroundTask})
}

// iosTx is a transaction as mobile2.m encodes it.
type iosTx struct {
	ID         string  `json:"id"`
	OriginalID string  `json:"originalId"`
	ProductID  string  `json:"productId"`
	Quantity   int     `json:"quantity"`
	State      int     `json:"state"`
	Time       float64 `json:"time"` // Unix seconds
	Error      string  `json:"error"`
	Canceled   bool    `json:"canceled"`
}

func decodeTx(s string) (platform.Transaction, bool) {
	var j iosTx
	if s == "" || json.Unmarshal([]byte(s), &j) != nil {
		return platform.Transaction{}, false
	}
	t := platform.Transaction{
		ID: j.ID, OriginalID: j.OriginalID, ProductID: j.ProductID, Quantity: j.Quantity,
		State: platform.TransactionState(j.State), Error: j.Error, Canceled: j.Canceled,
	}
	if j.Time > 0 {
		sec, frac := math.Modf(j.Time)
		t.Time = time.Unix(int64(sec), int64(frac*1e9))
	}
	return t, true
}

//export tangoIOSTransaction
func tangoIOSTransaction(js *C.char) {
	if t, ok := decodeTx(C.GoString(js)); ok {
		txUpdates = append(txUpdates, t)
		emitEvent(platform.AppEvent{Kind: platform.AppPurchaseUpdated})
	}
}

// keyboard is the last height mobile2.m reported (points).
var keyboard float64

//export tangoIOSKeyboard
func tangoIOSKeyboard(h C.double) {
	keyboard = float64(h)
	emitEvent(platform.AppEvent{Kind: platform.AppKeyboardChanged, Insets: platform.Insets{Bottom: keyboard}})
}

// ---- BackgroundTasks ----

func (b *backend) ScheduleBackground(r platform.BackgroundRequest) error {
	var msg *C.char
	b2i := func(v bool) C.int {
		if v {
			return 1
		}
		return 0
	}
	code := C.tgm2_bg_schedule(C.int(r.Kind), C.double(r.Earliest.Seconds()), b2i(r.RequiresNetwork), b2i(r.RequiresCharging), &msg)
	s := ""
	if msg != nil {
		s = C.GoString(msg)
		C.free(unsafe.Pointer(msg))
	}
	return mobileErr(int(code), s)
}

func (b *backend) CancelBackground(k platform.BackgroundKind) error {
	return mobileErr(int(C.tgm2_bg_cancel(C.int(k))), "")
}

func (b *backend) TakeBackgroundTasks() []platform.BackgroundTask {
	u := bgUpdates
	bgUpdates = nil
	return u
}

func (b *backend) FinishBackgroundTask(id int64, success bool) {
	s := C.int(0)
	if success {
		s = 1
	}
	C.tgm2_bg_finish(C.int64_t(id), s)
}

// ---- Purchases ----

func (b *backend) CanMakePayments() bool { return C.tgm2_can_pay() != 0 }

func (b *backend) QueryProducts(ids []string, done func([]platform.Product, []string, error)) {
	cs, free := cstring(strings.Join(ids, "\n"))
	defer free()
	tok := addCall(func(code int, s string) {
		if err := mobileErr(code, s); err != nil {
			done(nil, nil, err)
			return
		}
		var r struct {
			Products []struct {
				ID, Title, Description, Price, Currency string
				PriceValue                              float64
				Kind                                    int
			}
			Invalid []string
		}
		if err := json.Unmarshal([]byte(s), &r); err != nil {
			done(nil, nil, fmt.Errorf("tango/ios: bad products %q", s))
			return
		}
		var ps []platform.Product
		for _, p := range r.Products {
			ps = append(ps, platform.Product{ID: p.ID, Title: p.Title, Description: p.Description, Price: p.Price,
				PriceValue: p.PriceValue, Currency: p.Currency, Kind: platform.ProductKind(p.Kind)})
		}
		done(ps, r.Invalid, nil)
	})
	C.tgm2_products(tok, cs)
}

func (b *backend) Purchase(productID string, quantity int, done func(platform.Transaction, error)) {
	cs, free := cstring(productID)
	defer free()
	tok := addCall(func(code int, s string) {
		t, ok := decodeTx(s)
		if code != C.TGM_OK && ok {
			s = t.Error
		}
		done(t, mobileErr(code, s))
	})
	C.tgm2_buy(tok, cs, C.int(max(quantity, 1)))
}

func (b *backend) RestorePurchases(done func(error)) {
	tok := addCall(func(code int, s string) { done(mobileErr(code, s)) })
	C.tgm2_restore(tok)
}

func (b *backend) TakePurchaseUpdates() []platform.Transaction {
	u := txUpdates
	txUpdates = nil
	return u
}

func (b *backend) FinishTransaction(id string) error {
	cs, free := cstring(id)
	defer free()
	if err := mobileErr(int(C.tgm2_finish_transaction(cs)), ""); err != nil {
		return fmt.Errorf("tango/ios: no unfinished transaction %q: %w", id, err)
	}
	return nil
}

func (b *backend) PurchaseReceipt() ([]byte, error) {
	var p unsafe.Pointer
	var n C.int
	if err := mobileErr(int(C.tgm2_receipt(&p, &n)), ""); err != nil {
		return nil, err
	}
	defer C.free(p)
	return C.GoBytes(p, n), nil
}

// ---- device controls ----

func (b *backend) RequestReview() error {
	C.tgm2_review()
	return nil
}

func (b *backend) OpenAppSettings() error {
	return mobileErr(int(C.tgm2_open_settings()), "")
}

func (b *backend) SetKeepScreenOn(on bool) error {
	v := C.int(0)
	if on {
		v = 1
	}
	C.tgm2_keep_screen_on(v)
	return nil
}

func (b *backend) Brightness() (float64, error) {
	v := float64(C.tgm2_brightness())
	if v < 0 {
		return 0, platform.ErrNotAvailable
	}
	return v, nil
}

func (b *backend) SetBrightness(v float64) error {
	C.tgm2_set_brightness(C.double(v))
	return nil
}

var errBarColor = errors.New("tango/ios: the status bar has no background color on iOS (the page draws under it)")

func (b *backend) SetStatusBar(s platform.StatusBar) error {
	h := C.int(0)
	if s.Hidden {
		h = 1
	}
	C.tgm2_status_bar(C.int(s.Style), h)
	if s.Color != "" && !transparent(s.Color) {
		return fmt.Errorf("%w: %w", platform.ErrUnsupported, errBarColor)
	}
	return nil
}

// transparent reports a fully transparent "#00rrggbb": no color, which is
// what iOS has anyway.
func transparent(c string) bool {
	return len(c) == 9 && strings.HasPrefix(c, "#00")
}

func (b *backend) KeyboardHeight() float64 { return keyboard }

func (b *backend) HideKeyboard() error {
	C.tgm2_hide_keyboard()
	return nil
}
