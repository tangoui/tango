//go:build linux && !android

package linux

// Global hotkeys on Wayland through the xdg-desktop-portal GlobalShortcuts
// interface (portal 1.17+; KDE Plasma 6, GNOME 48+, Hyprland). Wayland
// lets no client grab keys, so the app hands the portal its shortcuts with
// a preferred trigger and the desktop binds them — usually after asking the
// user, who may also pick another key. The session is created on the first
// registration; every change rebinds the whole set (BindShortcuts takes
// the full list). Activations arrive as the portal's Activated signal.
//
// Without the portal or the interface RegisterHotkey returns
// ErrUnsupported, as before.

import (
	"fmt"
	"slices"
	"strconv"

	"github.com/tangoui/tango/internal/platform"
)

const (
	portalDesktopBus  = "org.freedesktop.portal.Desktop"
	portalDesktopPath = "/org/freedesktop/portal/desktop"
	portalShortcutsIf = "org.freedesktop.portal.GlobalShortcuts"
	portalRequestIf   = "org.freedesktop.portal.Request"
)

type portalHotkey struct {
	accel   string
	trigger string
	fire    func()
}

var ph struct {
	conn      ptr
	checked   bool
	err       error
	session   string
	creating  bool
	seq       int
	keys      map[uint32]portalHotkey
	activated func()
}

// portalHotkeysAvailable checks once for the GlobalShortcuts interface.
func portalHotkeysAvailable() error {
	if ph.checked {
		return ph.err
	}
	ph.checked = true
	conn, err := bus(gBusTypeSession)
	if err != nil {
		ph.err = fmt.Errorf("%w: %w", platform.ErrUnsupported, err)
		return ph.err
	}
	v, err := dbusGet(conn, portalDesktopBus, portalDesktopPath, portalShortcutsIf, "version")
	if err != nil {
		ph.err = fmt.Errorf("%w: global hotkeys on Wayland need xdg-desktop-portal with the GlobalShortcuts interface "+
			"(KDE Plasma 6, GNOME 48+): %v", platform.ErrUnsupported, err)
		return ph.err
	}
	gVariantUnref(v)
	ph.conn = conn
	ph.keys = map[uint32]portalHotkey{}
	return nil
}

func (b *backend) registerPortalHotkey(id uint32, accelerator string, fire func()) error {
	trigger, err := portalTrigger(accelerator)
	if err != nil {
		return err
	}
	if err := portalHotkeysAvailable(); err != nil {
		return err
	}
	ph.keys[id] = portalHotkey{accel: accelerator, trigger: trigger, fire: fire}
	if ph.session == "" {
		return b.createPortalSession()
	}
	return b.bindPortalHotkeys()
}

// unregisterPortalHotkey reports whether id was a portal hotkey.
func (b *backend) unregisterPortalHotkey(id uint32) bool {
	if _, ok := ph.keys[id]; !ok {
		return false
	}
	delete(ph.keys, id)
	if ph.session != "" {
		_ = b.bindPortalHotkeys()
	}
	return true
}

func (b *backend) portalToken() string {
	ph.seq++
	return "tango_" + strconv.Itoa(ph.seq)
}

// portalRequest calls a portal method whose options carry handle_token
// token and runs done with the Request's Response (code, results) on the
// main loop. The subscription is made before the call, as the
// specification advises, using the predictable request path.
func portalRequest(token, method, args string, done func(code uint32, results ptr)) error {
	path := portalRequestPath(goStr(gd.uniqueName(ph.conn)), token)
	var unsub func()
	unsub = subscribe(ph.conn, portalDesktopBus, portalRequestIf, "Response", path, "", func(_, _, _, _ string, params ptr) {
		unsub()
		cv, rv := gvChild(params, 0), gvChild(params, 1)
		code := gVariantGetUint32(cv)
		gVariantUnref(cv)
		done(code, rv)
		gVariantUnref(rv)
	})
	r, err := dbusCall(ph.conn, portalDesktopBus, portalDesktopPath, portalShortcutsIf, method, args, 5000)
	if err != nil {
		unsub()
		return err
	}
	gVariantUnref(r)
	return nil
}

func (b *backend) createPortalSession() error {
	if ph.creating {
		return nil // the pending session binds every registered key
	}
	ph.creating = true
	token, stoken := b.portalToken(), b.portalToken()
	err := portalRequest(token, "CreateSession", createSessionArgs(token, stoken), func(code uint32, results ptr) {
		ph.creating = false
		if code != 0 {
			return // denied or cancelled; a later registration retries
		}
		s, _ := dictString(results, "session_handle")
		if s == "" {
			return
		}
		ph.session = s
		if ph.activated == nil {
			ph.activated = subscribe(ph.conn, portalDesktopBus, portalShortcutsIf, "Activated", portalDesktopPath, "", func(_, _, _, _ string, params ptr) {
				sv, iv := gvChild(params, 0), gvChild(params, 1)
				session, sid := gvStr(sv), gvStr(iv)
				gVariantUnref(sv)
				gVariantUnref(iv)
				if session != ph.session {
					return
				}
				if id, ok := hotkeyFromShortcutID(sid); ok {
					if k, ok := ph.keys[id]; ok && k.fire != nil {
						k.fire()
					}
				}
			})
		}
		_ = b.bindPortalHotkeys()
	})
	if err != nil {
		ph.creating = false
		return fmt.Errorf("%w: GlobalShortcuts.CreateSession: %v", platform.ErrUnsupported, err)
	}
	return nil
}

func (b *backend) bindPortalHotkeys() error {
	ids := make([]uint32, 0, len(ph.keys))
	for id := range ph.keys {
		ids = append(ids, id)
	}
	slices.Sort(ids)
	list := make([]portalShortcut, len(ids))
	app := b.who()
	for i, id := range ids {
		k := ph.keys[id]
		list[i] = portalShortcut{id: hotkeyShortcutID(id), description: app + ": " + k.accel, trigger: k.trigger}
	}
	token := b.portalToken()
	return portalRequest(token, "BindShortcuts", bindShortcutsArgs(ph.session, list, token), func(uint32, ptr) {})
}
