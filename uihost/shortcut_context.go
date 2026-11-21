package uihost

import (
	"errors"
	"fmt"
	"runtime"
	"slices"
	"strings"
	"sync/atomic"
	"time"

	"github.com/tangoui/tango"
	"github.com/tangoui/tango/internal/corehook"
	"github.com/tangoui/tango/sys/hotkey"
	"github.com/tangoui/tango/ui/input"
)

// ShortcutContext describes the physical key event and the latest input state
// of the self-drawn window when a contextual shortcut is matched.
type ShortcutContext struct {
	// Event carries the key, modifiers and phase. Down is false on physical
	// release, and Repeat is true on auto-repeated presses.
	Event input.KeyEvent
	// Composing reports an active IME pre-edit.
	Composing bool
	// TextInput reports that the focused control enabled text input.
	TextInput bool
	// Prefix reports an intermediate stroke of a multi-stroke shortcut.
	Prefix bool
}

// ShortcutContextOptions refines ShortcutWithContext.
type ShortcutContextOptions struct {
	// Timeout is the maximum gap between consecutive chord presses. For a
	// double tap it is the maximum duration of each tap and the gap between
	// taps. Zero selects 1500 ms for chords and 300 ms for double taps.
	Timeout time.Duration
}

// ShortcutWithContext registers a shortcut on a window opened with Open.
// Use the Shortcut accelerator syntax for each stroke, for example
// "CmdOrCtrl+K CmdOrCtrl+S"; use "Shift Shift", "Ctrl Ctrl", "Alt Alt" or
// "Meta Meta" for a double tap of either side of a bare modifier. Each tap
// uses the same physical key for its press and release. "Mod" is an alias
// for "CmdOrCtrl".
//
// The callback runs on the app's main thread and may consume each matched
// event by returning true. A false result lets a later binding or the
// focused input path handle the event. Return true on a prefix press to
// arm the next stroke. A mismatch or timeout discards the pending sequence;
// consumed events stay consumed. Auto-repeat and physical release are
// offered for matched presses, even if modifiers changed. A double tap
// invokes the callback only on the second physical release. Native menu
// shortcuts and shortcut recording retain priority. In tan dev the callback
// runs after native translation and can only consume forwarded key events.
//
// Pending continuations have priority over new bindings. For the same
// stroke bindings are tried in registration order, and prefixes sharing a
// stroke may arm together. Contextual bindings run before ShortcutWith.
// The returned remove function is idempotent and can be called from any
// goroutine, as can registration. Window close ends all bindings.
func ShortcutWithContext(s *tango.Surface, sequence string, opts ShortcutContextOptions, fn func(ShortcutContext) bool) (remove func(), err error) {
	b, err := parseShortcutSequence(sequence)
	if err != nil {
		return nil, err
	}
	if fn == nil {
		return nil, errors.New("uihost: nil shortcut function")
	}
	if opts.Timeout < 0 {
		return nil, errors.New("uihost: negative shortcut timeout")
	}
	if opts.Timeout > 0 {
		b.timeout = opts.Timeout
	}
	t := shortcutTable(s)
	if t == nil {
		return nil, ErrNotUI
	}
	b.fn = fn
	t.mu.Lock()
	t.contextual = append(t.contextual, b)
	t.mu.Unlock()
	return func() {
		if b.removed.Swap(true) {
			return
		}
		t.mu.Lock()
		if i := slices.Index(t.contextual, b); i >= 0 {
			t.contextual = append(t.contextual[:i:i], t.contextual[i+1:]...)
		}
		t.mu.Unlock()
	}, nil
}

type contextShortcut struct {
	strokes  []*shortcut
	modifier input.Modifiers
	timeout  time.Duration
	fn       func(ShortcutContext) bool
	removed  atomic.Bool

	// Main thread only.
	next     int
	deadline time.Time
	held     map[input.Key]bool // matched presses, with their Prefix state
	tapKey   input.Key
	tapStart time.Time
}

func parseShortcutSequence(sequence string) (*contextShortcut, error) {
	parts := strings.Fields(sequence)
	if len(parts) == 0 {
		return nil, fmt.Errorf("%w: empty shortcut sequence", hotkey.ErrBadAccelerator)
	}
	if len(parts) == 2 {
		first, second := shortcutModifier(parts[0]), shortcutModifier(parts[1])
		if first != 0 && first == second {
			return &contextShortcut{modifier: first, timeout: 300 * time.Millisecond}, nil
		}
	}
	b := &contextShortcut{timeout: 1500 * time.Millisecond}
	for _, part := range parts {
		tokens := strings.Split(part, "+")
		for i := 0; i < len(tokens)-1; i++ {
			if strings.EqualFold(tokens[i], "mod") {
				tokens[i] = "CmdOrCtrl"
			}
		}
		stroke, err := parseShortcut(strings.Join(tokens, "+"))
		if err != nil {
			return nil, err
		}
		b.strokes = append(b.strokes, stroke)
	}
	return b, nil
}

func shortcutModifier(s string) input.Modifiers {
	switch strings.ToLower(s) {
	case "shift":
		return input.ModShift
	case "ctrl", "control":
		return input.ModCtrl
	case "alt", "option", "opt":
		return input.ModAlt
	case "meta", "cmd", "command", "win", "super":
		return input.ModMeta
	case "mod", "cmdorctrl", "commandorcontrol":
		if runtime.GOOS == "darwin" || runtime.GOOS == "ios" {
			return input.ModMeta
		}
		return input.ModCtrl
	}
	return 0
}

func shortcutPhysicalKey(ev input.KeyEvent) input.Key {
	if ev.Code != 0 {
		return ev.Code
	}
	return ev.Key
}

func (t *shortcuts) contextBindings() []*contextShortcut {
	t.mu.Lock()
	bindings := t.contextual
	t.mu.Unlock()
	return bindings
}

func (t *shortcuts) resetContextKeys() {
	t.contextEpoch++
	clear(t.contextHeld)
	for _, b := range t.contextBindings() {
		b.cancelSequence()
		clear(b.held)
	}
}

func (b *contextShortcut) cancelSequence() {
	b.next, b.tapKey, b.deadline = 0, 0, time.Time{}
}

func (t *shortcuts) callContext(b *contextShortcut, ctx ShortcutContext) bool {
	if b.removed.Load() {
		return false
	}
	var consumed bool
	corehook.Guard(t.app, "event handler", func() { consumed = b.fn(ctx) })
	return consumed
}

func (t *shortcuts) contextKey(ev input.KeyEvent, capture, textOn bool, now time.Time) bool {
	bindings := t.contextBindings()
	physical := shortcutPhysicalKey(ev)
	if !ev.Down || !ev.Repeat {
		delete(t.contextHeld, physical)
	}
	if len(bindings) == 0 {
		return false
	}
	// Preserve recorder ownership and the press/release pairing of legacy
	// bindings that already took the press.
	if capture || t.captured[ev.Key] || (t.held[ev.Key] && (!ev.Down || ev.Repeat)) {
		if capture {
			t.resetContextKeys()
		}
		return false
	}
	ctx := ShortcutContext{Event: ev, TextInput: textOn, Composing: t.contextComposing.Load()}
	epoch := t.contextEpoch
	consumed, sharedPrefix := false, false

	// Advance every double-tap tracker, including on mismatches. Pair
	// repeats and releases with the stroke that matched the original press.
	if ev.Key.Modifier() != 0 || !ev.Down || ev.Repeat {
		for _, b := range bindings {
			matched := false
			if b.modifier != 0 {
				matched = b.doubleTap(ev, now)
				ctx.Prefix = false
			} else {
				ctx.Prefix, matched = b.held[physical]
				if !ev.Down {
					delete(b.held, physical)
				}
			}
			if matched && !consumed && t.contextEpoch == epoch {
				consumed = t.callContext(b, ctx)
			}
		}
		return consumed
	}

	// Match pending sequences first. A mismatch resets the sequence, but
	// the same press can start a new one.
	var attempted []*contextShortcut
	for _, b := range bindings {
		if b.modifier != 0 {
			b.cancelSequence()
			continue
		}
		next := b.next
		b.next = 0
		if next > 0 && now.Before(b.deadline) && b.strokes[next].matches(ev) && t.contextEpoch == epoch {
			prefix := next+1 < len(b.strokes)
			if !consumed || sharedPrefix && prefix {
				attempted = append(attempted, b)
				if t.contextStroke(b, next, ctx, physical, now, epoch) {
					consumed, sharedPrefix = true, prefix
				}
			}
		}
	}
	if consumed || t.contextEpoch != epoch {
		return consumed
	}
	for _, b := range bindings {
		if b.modifier == 0 && b.strokes[0].matches(ev) && t.contextEpoch == epoch && !slices.Contains(attempted, b) {
			prefix := len(b.strokes) > 1
			if (!consumed || sharedPrefix && prefix) && t.contextStroke(b, 0, ctx, physical, now, epoch) {
				consumed, sharedPrefix = true, prefix
			}
		}
	}
	return consumed
}

func (t *shortcuts) contextStroke(b *contextShortcut, index int, ctx ShortcutContext, key input.Key, now time.Time, epoch uint64) bool {
	ctx.Prefix = index+1 < len(b.strokes)
	if b.removed.Load() {
		return false
	}
	if b.held == nil {
		b.held = make(map[input.Key]bool)
	}
	b.held[key] = ctx.Prefix
	if !t.callContext(b, ctx) {
		return false
	}
	if t.contextEpoch == epoch {
		if t.contextHeld == nil {
			t.contextHeld = make(map[input.Key]bool)
		}
		t.contextHeld[key] = true
		if ctx.Prefix {
			b.next = index + 1
			b.deadline = now.Add(b.timeout)
		}
	}
	return true
}

func (b *contextShortcut) doubleTap(ev input.KeyEvent, now time.Time) bool {
	if ev.Repeat {
		return false
	}
	if ev.Down {
		if ev.Key.Modifier() != b.modifier || ev.Mods != b.modifier || b.tapKey != 0 {
			b.deadline, b.tapKey = time.Time{}, 0
		} else {
			b.tapKey, b.tapStart = ev.Key, now
		}
		return false
	}
	if ev.Key != b.tapKey {
		return false
	}
	b.tapKey = 0
	if ev.Mods != 0 || now.Sub(b.tapStart) > b.timeout {
		b.deadline = time.Time{}
		return false
	}
	if !b.deadline.IsZero() && b.tapStart.Before(b.deadline) {
		b.deadline = time.Time{}
		return true
	}
	b.deadline = now.Add(b.timeout)
	return false
}
