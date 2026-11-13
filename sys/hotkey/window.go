package hotkey

import (
	"encoding/json"
	"errors"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/tangoui/tango"
	"github.com/tangoui/tango/internal/corehook"
)

// Window shortcuts: a keydown listener (capture phase, so the page cannot
// swallow the key first) injected into the surface's page posts an
// internal message on the "hotkey" channel with the binding id. The
// listener's table is re-sent after every page load.

const channel = "hotkey"

// Binding is a shortcut bound to one surface's page.
type Binding struct {
	s     *tango.Surface
	id    uint32
	accel string
	spec  accel
	fn    func()
	live  atomic.Bool
}

type surfaceBinds struct {
	binds map[uint32]*Binding
}

var win = struct {
	mu       sync.Mutex
	apps     map[*tango.App]bool
	surfaces map[*tango.Surface]*surfaceBinds
	byID     map[uint32]*Binding
}{
	apps:     map[*tango.App]bool{},
	surfaces: map[*tango.Surface]*surfaceBinds{},
	byID:     map[uint32]*Binding{},
}

// Bind makes accelerator call fn (on the main thread) while s's page has
// the keyboard focus. The key event does not reach the page. Bindings
// survive navigation and end when the surface closes or with Unbind. If
// several bindings of s share an accelerator, the first bound fires.
func Bind(s *tango.Surface, accelerator string, fn func()) (*Binding, error) {
	spec, err := parse(accelerator)
	if err != nil {
		return nil, err
	}
	if corehook.HandleInternal == nil {
		return nil, errors.New("hotkey: core internal messages unavailable")
	}
	app := s.App()
	b := &Binding{s: s, id: nextID.Add(1), accel: accelerator, spec: spec, fn: fn}
	b.live.Store(true)

	win.mu.Lock()
	hookApp := !win.apps[app]
	win.apps[app] = true
	sb := win.surfaces[s]
	hookSurface := sb == nil
	if hookSurface {
		sb = &surfaceBinds{binds: map[uint32]*Binding{}}
		win.surfaces[s] = sb
	}
	sb.binds[b.id] = b
	win.byID[b.id] = b
	win.mu.Unlock()

	if hookApp {
		corehook.HandleInternal(app, channel, func(surface any, payload string) {
			fire(surface, payload)
		})
	}
	if hookSurface {
		s.OnLoaded(func() { s.Eval(script(s)) })
		s.OnClosed(func() { dropSurface(s) })
	}
	s.Eval(script(s))
	return b, nil
}

// Accelerator returns the accelerator b was bound with.
func (b *Binding) Accelerator() string { return b.accel }

// Unbind removes the binding; the key reaches the page again. Unbinding
// twice does nothing.
func (b *Binding) Unbind() {
	if !b.live.CompareAndSwap(true, false) {
		return
	}
	win.mu.Lock()
	delete(win.byID, b.id)
	if sb := win.surfaces[b.s]; sb != nil {
		delete(sb.binds, b.id)
	}
	win.mu.Unlock()
	b.s.Eval(script(b.s))
}

func dropSurface(s *tango.Surface) {
	win.mu.Lock()
	defer win.mu.Unlock()
	if sb := win.surfaces[s]; sb != nil {
		for id, b := range sb.binds {
			b.live.Store(false)
			delete(win.byID, id)
		}
		// Keep the entry (with no bindings) so a closed surface is not
		// hooked again by a late Bind.
		sb.binds = map[uint32]*Binding{}
	}
}

// fire runs on the main thread for an internal "hotkey" message.
func fire(surface any, payload string) {
	id, err := strconv.ParseUint(payload, 10, 32)
	if err != nil {
		return
	}
	win.mu.Lock()
	b := win.byID[uint32(id)]
	win.mu.Unlock()
	if b == nil || !b.live.Load() || any(b.s) != surface {
		return
	}
	b.fn()
}

type jsBinding struct {
	C    bool   `json:"c"`
	A    bool   `json:"a"`
	S    bool   `json:"s"`
	M    bool   `json:"m"`
	Code string `json:"code"`
	Key  string `json:"key,omitempty"`
}

// bindingsJSON returns s's live bindings keyed by id, ordered by id
// (encoding/json sorts map keys; ids are compared numerically in JS).
func bindingsJSON(s *tango.Surface) string {
	win.mu.Lock()
	m := map[string]jsBinding{}
	if sb := win.surfaces[s]; sb != nil {
		for id, b := range sb.binds {
			sp := b.spec
			m[strconv.FormatUint(uint64(id), 10)] = jsBinding{sp.Ctrl, sp.Alt, sp.Shift, sp.Meta, sp.Code, sp.Key}
		}
	}
	win.mu.Unlock()
	out, _ := json.Marshal(m)
	return string(out)
}

var prefixJS = func() string {
	b, _ := json.Marshal(corehook.InternalPrefix + channel + ":")
	return string(b)
}()

// listenerJS installs the keydown listener once per page and replaces its
// table with __BINDINGS__.
const listenerJS = `(function () {
  var w = window, T = w.__tangoKeys;
  if (!T) {
    T = w.__tangoKeys = { b: {} };
    w.addEventListener("keydown", function (e) {
      if (e.isComposing || !w.__tango) return;
      var ids = Object.keys(T.b).sort(function (x, y) { return x - y; });
      var key = typeof e.key === "string" ? e.key.toLowerCase() : "";
      for (var i = 0; i < ids.length; i++) {
        var k = T.b[ids[i]];
        if (e.ctrlKey !== k.c || e.altKey !== k.a || e.shiftKey !== k.s || e.metaKey !== k.m) continue;
        if (e.code !== k.code && !(k.key && key === k.key)) continue;
        e.preventDefault();
        e.stopImmediatePropagation();
        if (!e.repeat) w.__tango.post(__PREFIX__ + ids[i]);
        return;
      }
    }, true);
  }
  T.b = __BINDINGS__;
})();`

func script(s *tango.Surface) string {
	return strings.NewReplacer("__PREFIX__", prefixJS, "__BINDINGS__", bindingsJSON(s)).Replace(listenerJS)
}
