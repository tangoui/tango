package menu

import (
	"encoding/json"

	"github.com/tangoui/tango"
	"github.com/tangoui/tango/internal/corehook"
	"github.com/tangoui/tango/internal/platform"
)

// Popup shows m as a context menu at (x, y) in s's content coordinates
// (logical px, the page's clientX / clientY at zoom 1) and blocks until the
// menu closes. If the user chose an item, its OnClick has run (on the main
// thread) by the time Popup returns.
//
// Safe from any goroutine. Called on the main thread (from a click or
// message callback) it keeps the UI responsive while waiting, like
// tango.AwaitAsync. The menu is built when Popup is called; later changes
// to m do not affect an open popup.
func Popup(s *tango.Surface, x, y int, m *Menu) error {
	a := s.App()
	pm, ok := a.Backend().(platform.PopupMenu)
	if !ok {
		return ErrUnsupported
	}
	items, onClick := convert(a, m)
	_, err := tango.TryAwaitAsync(a, func(done func(bool)) {
		pm.PopupMenu(s.ID(), x, y, items, onClick, func() { done(true) })
	})
	return err
}

// Context describes a context menu request from a page.
type Context struct {
	Surface *tango.Surface
	Name    string // the declared menu name
	X, Y    int    // click position in the surface's content (CSS px)
	// Data is the value of the data-tango-menu-data attribute nearest to
	// the clicked element ("" if none), e.g. a row id.
	Data string
}

const contextChannel = "ctxmenu"

// contextJS lets pages declare context menus. The innermost element with a
// data-tango-menu attribute wins; otherwise the CSS custom property
// --tango-contextmenu (inherited) of the clicked element. The name "none"
// suppresses the browser menu, "default" keeps it (to opt a subtree out).
const contextJS = `(function () {
  "use strict";
  if (window.__tangoContextMenu) return;
  window.__tangoContextMenu = true;
  var prefix = "\u0000tango-int:` + contextChannel + `:";
  function lookup(target) {
    var name = "", data = "";
    for (var el = target; el && el.nodeType === 1; el = el.parentElement) {
      if (!data && el.hasAttribute("data-tango-menu-data")) data = el.getAttribute("data-tango-menu-data");
      if (!name && el.hasAttribute("data-tango-menu")) name = el.getAttribute("data-tango-menu").trim();
    }
    if (!name && target && target.nodeType === 1) {
      name = getComputedStyle(target).getPropertyValue("--tango-contextmenu").trim();
      if (name.length > 1 && (name[0] === '"' || name[0] === "'")) name = name.slice(1, -1);
    }
    return { name: name, data: data };
  }
  window.addEventListener("contextmenu", function (e) {
    var t = e.target && e.target.nodeType === 1 ? e.target : e.target && e.target.parentElement;
    var r = lookup(t);
    if (!r.name || r.name === "default") return;
    e.preventDefault();
    if (r.name === "none" || !window.__tango) return;
    window.__tango.post(prefix + JSON.stringify({
      name: r.name, x: Math.round(e.clientX), y: Math.round(e.clientY), data: r.data
    }));
  }, true);
})();
`

type contextMsg struct {
	Name string `json:"name"`
	X    int    `json:"x"`
	Y    int    `json:"y"`
	Data string `json:"data"`
}

var contexts = map[*tango.App]map[string]func(Context) *Menu{} // guarded by mu

// RegisterContext makes m the context menu of page elements declaring
// name, with the attribute data-tango-menu="name" or the CSS property
// --tango-contextmenu: name. A nil m unregisters name.
//
// The first registration on an app injects the page script: surfaces
// created afterwards get it on every page load, existing ones only for
// their current page. Register context menus before creating surfaces.
func RegisterContext(a *tango.App, name string, m *Menu) error {
	if m == nil {
		return RegisterContextFunc(a, name, nil)
	}
	return RegisterContextFunc(a, name, func(Context) *Menu { return m })
}

// RegisterContextFunc is RegisterContext with a menu built per request,
// on the main thread, from the click context; returning nil shows nothing.
func RegisterContextFunc(a *tango.App, name string, fn func(Context) *Menu) error {
	if _, ok := a.Backend().(platform.PopupMenu); !ok {
		return ErrUnsupported
	}
	mu.Lock()
	reg := contexts[a]
	first := reg == nil
	if first {
		reg = map[string]func(Context) *Menu{}
		contexts[a] = reg
	}
	if fn == nil {
		delete(reg, name)
	} else {
		reg[name] = fn
	}
	mu.Unlock()
	if first {
		corehook.HandleInternal(a, contextChannel, func(s any, payload string) {
			showContext(a, s.(*tango.Surface), payload)
		})
		a.AddBridgeScript(contextJS)
		for _, s := range a.Surfaces() {
			s.Eval(contextJS)
		}
	}
	return nil
}

// showContext runs on the main thread. It does not wait for the menu to
// close: the message callback returns while the menu is open (where the
// backend's popup is non-blocking).
func showContext(a *tango.App, s *tango.Surface, payload string) {
	var msg contextMsg
	if json.Unmarshal([]byte(payload), &msg) != nil {
		return
	}
	mu.Lock()
	fn := contexts[a][msg.Name]
	mu.Unlock()
	if fn == nil {
		return
	}
	m := fn(Context{Surface: s, Name: msg.Name, X: msg.X, Y: msg.Y, Data: msg.Data})
	if m == nil {
		return
	}
	pm, ok := a.Backend().(platform.PopupMenu)
	if !ok {
		return
	}
	items, onClick := convert(a, m)
	pm.PopupMenu(s.ID(), msg.X, msg.Y, items, onClick, func() {})
}
