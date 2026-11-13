// Package safearea reports the parts of a page covered by system UI (status
// bar, notch / Dynamic Island, home indicator, navigation bar) and follows
// their changes, e.g. on rotation.
//
//	in, err := safearea.Get(a)
//	safearea.OnChange(a, func(in safearea.Insets) { ... })
//
// SyncPage mirrors the insets into every page as CSS variables on <html>:
//
//	--tango-safe-top, --tango-safe-bottom, --tango-safe-left, --tango-safe-right
//
// for pages that cannot rely on env(safe-area-inset-*) (Android WebView,
// which reports 0 there, older engines, values needed in JS). Use them with
// env() as the fallback:
//
//	padding-top: var(--tango-safe-top, env(safe-area-inset-top, 0px));
//
// Insets are in CSS pixels relative to the page's web view. On iOS a page
// fills the screen; the values matter once the page opts into
// viewport-fit=cover (otherwise WKWebView insets the content itself, and
// padding by them again would double it). The Android host keeps pages
// clear of system bars, so they are normally zero there.
//
// Get returns ErrUnsupported on desktop and tango.ErrNotRunning after the
// app quit.
package safearea

import (
	"fmt"

	"github.com/tangoui/tango"
	"github.com/tangoui/tango/internal/corehook"
	"github.com/tangoui/tango/internal/platform"
	"github.com/tangoui/tango/sys/internal/capcall"
)

var ErrUnsupported = platform.ErrUnsupported

// Insets in CSS pixels.
type Insets = platform.Insets

// Get returns the current insets.
func Get(a *tango.App) (Insets, error) {
	s, err := capcall.Backend[platform.SafeArea](a)
	if err != nil {
		return Insets{}, err
	}
	return capcall.Sync(a, func() (Insets, error) { return s.SafeArea(), nil })
}

// OnChange calls fn on the main thread with the new insets whenever they
// change. It does nothing where the backend has no safe area.
func OnChange(a *tango.App, fn func(Insets)) {
	if _, ok := a.Backend().(platform.SafeArea); !ok {
		return
	}
	a.OnEventKind(tango.EventSafeAreaChanged, func(ev tango.AppEvent) { fn(ev.Insets) })
}

// SyncPage keeps the --tango-safe-* CSS variables of every page current.
// Pages can also listen for the "tangosafearea" event on window, whose
// detail is {top, bottom, left, right}. Call it before creating surfaces
// (it adds a bridge script). It returns ErrUnsupported (and installs
// nothing) where the backend has no safe area.
func SyncPage(a *tango.App) error {
	s, ok := a.Backend().(platform.SafeArea)
	if !ok {
		return ErrUnsupported
	}
	a.AddBridgeScript(pageScript)
	// A page asks for the values when its document starts.
	corehook.HandleInternal(a, channel, func(surface any, _ string) {
		if sf, ok := surface.(*tango.Surface); ok {
			sf.Eval(applyCall(s.SafeArea()))
		}
	})
	OnChange(a, func(in Insets) {
		js := applyCall(in)
		for _, sf := range a.Surfaces() {
			sf.Eval(js)
		}
	})
	return nil
}

const channel = "safearea"

func applyCall(in Insets) string {
	return fmt.Sprintf("window.__tangoSafeArea&&window.__tangoSafeArea(%g,%g,%g,%g);", in.Top, in.Bottom, in.Left, in.Right)
}

// pageScript runs at document start in every page, after the core shim.
const pageScript = `(function(){
if (window.__tangoSafeArea) return;
var v = null;
function apply(){
  var el = document.documentElement;
  if (!el || !v) return;
  for (var k in v) el.style.setProperty("--tango-safe-" + k, v[k] + "px");
  try { window.dispatchEvent(new CustomEvent("tangosafearea", {detail: v})); } catch (e) {}
}
window.__tangoSafeArea = function(t, b, l, r){ v = {top: t, bottom: b, left: l, right: r}; apply(); };
if (!document.documentElement) document.addEventListener("readystatechange", apply, {once: true});
try { window.__tango.post("\u0000tango-int:` + channel + `:get"); } catch (e) {}
})();`
