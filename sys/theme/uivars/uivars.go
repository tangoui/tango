// Package uivars injects the design tokens of Tango's self-drawn UI
// (ui/theme) into every WebView page as CSS custom properties, and keeps
// them in step with the system's light/dark appearance, so the pages of an
// app and its self-drawn windows look the same.
//
// The variables are those of (*ui/theme.Theme).CSSVars, whatever the
// theme exports, in one rule
//
//	:root { --tango-color-primary: #1677ff; ... }
//
// held by <style id="tango-ui-vars"> placed first in the document, so the
// page's own stylesheets can still override them. Pages read them with
// var(--tango-color-primary) and may listen for the "tangouivars" event on
// window (detail: the name → value map); window.__tangoUIVarsState holds
// the current map. runtime-js exposes both as Theme.vars and
// Theme.onVarsChange.
//
// This lives outside sys/theme so that WebView-only apps using sys/theme
// do not link package ui.
package uivars

import (
	"encoding/json"
	"sync"

	"github.com/tangoui/tango"
	"github.com/tangoui/tango/internal/platform"
	systheme "github.com/tangoui/tango/sys/theme"
	uitheme "github.com/tangoui/tango/ui/theme"
)

// Gen returns the theme to export while the system appearance is sys; nil
// removes the variables.
type Gen func(sys systheme.Theme) *uitheme.Theme

// Default is the Gen used for a nil one: theme.AntDesign, dark when the
// system is.
func Default(sys systheme.Theme) *uitheme.Theme {
	if sys.Dark {
		return uitheme.AntDesign(uitheme.Dark())
	}
	return uitheme.AntDesign()
}

// state is one app's sync. Main thread, except mu-guarded registration.
type state struct {
	app     *tango.App
	gen     Gen
	st      platform.SystemTheme // nil: the backend cannot read the theme
	current systheme.Theme
	known   bool
}

var (
	mu   sync.Mutex
	apps = map[*tango.App]*state{}
)

// SyncPage makes every page of a get the variables of gen's theme
// (Default when gen is nil) and updates them whenever the system theme
// changes. Call it once, before creating surfaces: it adds a bridge script
// that sets the variables at document start — the light or dark variant
// by prefers-color-scheme, built with gen at the time of the call — so
// pages do not flash unstyled; once a page has loaded it gets the exact
// variables for the current system theme (accent color included).
//
// With an app that cannot read the system theme it returns
// systheme.ErrUnsupported; pages then keep the document-start variables,
// which still follow prefers-color-scheme. A second call for the same app
// only replaces gen.
//
// To keep pages in step with a self-drawn window, pass the window's
// generator, e.g. uihost.Options.ThemeFor.
func SyncPage(a *tango.App, gen Gen) error {
	if gen == nil {
		gen = Default
	}
	mu.Lock()
	if s := apps[a]; s != nil {
		s.gen = gen
		mu.Unlock()
		return nil
	}
	s := &state{app: a, gen: gen}
	apps[a] = s
	mu.Unlock()

	a.AddBridgeScript(bridgeScript(gen))
	st, ok := a.Backend().(platform.SystemTheme)
	if !ok {
		return systheme.ErrUnsupported
	}
	s.st = st
	systheme.OnChange(a, func(t systheme.Theme) {
		s.current, s.known = t, true
		s.pushAll()
	})
	a.OnReady(func() {
		if !s.known {
			s.current, s.known = st.SystemTheme(), true
			s.pushAll()
		}
	})
	a.OnSurfaceCreated(func(sf *tango.Surface) {
		sf.OnLoaded(func() {
			if !s.known {
				s.current, s.known = st.SystemTheme(), true
			}
			sf.Eval(s.call())
		})
	})
	return nil
}

// Refresh regenerates the variables with the generator given to SyncPage
// and pushes them to every page, for a generator whose answer depends on
// app state (the app switched its theme by hand). Safe from any
// goroutine; it does nothing before SyncPage or when the backend cannot
// read the system theme.
func Refresh(a *tango.App) {
	mu.Lock()
	s := apps[a]
	mu.Unlock()
	if s == nil || s.st == nil {
		return
	}
	a.Post(func() {
		if !s.known {
			s.current, s.known = s.st.SystemTheme(), true
		}
		s.pushAll()
	})
}

// call is the JS applying the current variables. Main thread.
func (s *state) call() string {
	mu.Lock()
	gen := s.gen
	mu.Unlock()
	return applyCall(gen(s.current))
}

// pushAll sends the current variables to every page. Main thread.
func (s *state) pushAll() {
	js := s.call()
	for _, sf := range s.app.Surfaces() {
		sf.Eval(js)
	}
}

// Vars returns the variables uivars injects for th: th.CSSVars(), or an
// empty map for nil.
func Vars(th *uitheme.Theme) map[string]string {
	if th == nil {
		return map[string]string{}
	}
	return th.CSSVars()
}

// applyCall is the JS that applies th's variables to a page.
func applyCall(th *uitheme.Theme) string {
	return "window.__tangoUIVars&&window.__tangoUIVars(" + jsonVars(Vars(th)) + ");"
}

func jsonVars(v map[string]string) string {
	b, _ := json.Marshal(v) // map[string]string always encodes
	return string(b)
}

// bridgeScript installs window.__tangoUIVars and applies the light or dark
// variant of gen at document start.
func bridgeScript(gen Gen) string {
	light := jsonVars(Vars(gen(systheme.Theme{})))
	dark := jsonVars(Vars(gen(systheme.Theme{Dark: true})))
	return `(function(){
if (window.__tangoUIVars) return;
var pushed = false;
function rule(vars){
  var names = Object.keys(vars).sort(), out = ":root {\n";
  for (var i = 0; i < names.length; i++) out += "  " + names[i] + ": " + vars[names[i]] + ";\n";
  return out + "}\n";
}
function apply(vars){
  window.__tangoUIVarsState = vars;
  var el = document.documentElement;
  if (el) {
    var st = document.getElementById("tango-ui-vars");
    if (!st) {
      st = document.createElement("style");
      st.id = "tango-ui-vars";
      var head = document.head;
      if (head) head.insertBefore(st, head.firstChild);
      else el.insertBefore(st, el.firstChild);
    }
    st.textContent = rule(vars);
  }
  try { window.dispatchEvent(new CustomEvent("tangouivars", {detail: vars})); } catch (e) {}
}
window.__tangoUIVars = function(vars){ pushed = true; apply(vars || {}); };
var mq = window.matchMedia ? window.matchMedia("(prefers-color-scheme: dark)") : null;
var light = ` + light + `, dark = ` + dark + `;
function initial(){ if (!pushed) apply(mq && mq.matches ? dark : light); }
if (mq && mq.addEventListener) mq.addEventListener("change", initial);
if (document.documentElement) initial();
else document.addEventListener("readystatechange", initial, {once: true});
})();`
}
