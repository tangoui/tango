package ui

import (
	"runtime"
	"strings"

	"github.com/tangoui/tango/ui/a11y"
	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/icon"
	"github.com/tangoui/tango/ui/input"
	"github.com/tangoui/tango/ui/layout"
	"github.com/tangoui/tango/ui/paint"
	"github.com/tangoui/tango/ui/theme"
)

// shellWindows selects the Windows key name ("Win" rather than "Super").
var shellWindows = runtime.GOOS == "windows"

// HotkeyInputW is the shortcut recorder HotkeyInput returns.
type HotkeyInputW struct {
	_ [0]func()

	c           *Context
	th          *theme.Theme
	tk          *theme.HotkeyInputToken
	val         *string
	wrap        *Box
	frame       *Box
	placeholder string
	size        ControlSize
	status      string
	width       float32
	disabled    bool
	allowBare   bool
	literal     bool
	conflict    func(accel string) string
	name        string
	msg         string
	changed     bool
	ia          Interaction
	z           *lazy
}

type hotkeyState struct {
	recording bool
	focused   bool            // focus last frame
	mods      input.Modifiers // modifiers held while recording
}

// HotkeyInput adds a shortcut recorder bound to *accel, an accelerator in
// the syntax of package sys/hotkey and uihost shortcuts
// ("CmdOrCtrl+Shift+K", "Alt+F4", "F5"):
//
//	ui.HotkeyInput(c, &keys.Palette).Conflict(func(a string) string {
//		if owner := bindings.Owner(a); owner != "" { return "已被“" + owner + "”使用" }
//		return ""
//	})
//
// Focusing it (a click, or Tab and then Enter / Space) starts recording:
// the held modifiers show as key caps until a main key completes the
// combination, which becomes the value. The platform shortcut modifier is
// stored as CmdOrCtrl (Literal stores Cmd / Ctrl as pressed). A main key
// needs a modifier other than Shift, except F1–F12 (AllowBare lifts
// that). Bare Backspace / Delete clear the value, as the clear button
// does.
//
// Esc while recording cancels it and keeps the old value; the Esc is not
// passed on, so an enclosing Modal or Drawer stays open, and a second Esc
// (not recording) reaches it. (Popovers and dropdowns check Esc before
// building their content, so inside one Esc closes the panel.) Tab is not
// recorded: it moves focus as everywhere else.
func HotkeyInput(c *Context, accel *string) *HotkeyInputW {
	w := &HotkeyInputW{c: c, val: accel, status: c.itemStatus, size: c.defaultSize()}
	w.th = c.ThemeFor(theme.CompHotkeyInput)
	w.tk = &w.th.Components.HotkeyInput
	w.width = w.tk.MinWidth
	w.wrap = c.box("hotkey-input", layout.Column, []layout.Option{layout.Gap(4)})
	w.wrap.n.ln.NoStretch = true
	w.wrap.Body(func() {
		w.frame = c.box("hotkey-field", layout.Row, []layout.Option{layout.Middle(), layout.Gap(w.tk.KeyGap)})
	})
	// Esc that cancels recording is taken now, before an enclosing dialog
	// (built around this field) checks for it at the end of its body.
	f := w.frame.n
	if st := State[hotkeyState](c, f.id, nil); st.recording && c.focus == f.id && !f.disabled && c.Escape() {
		st.recording, st.mods = false, 0
		c.invalid = true
	}
	w.z = c.lazy(w.wrap.n, w.build)
	return w
}

// Placeholder replaces the empty-field text (Locale.HotkeyInput.Placeholder).
func (w *HotkeyInputW) Placeholder(s string) *HotkeyInputW { w.placeholder = s; return w }

// Size sets the control size.
func (w *HotkeyInputW) Size(s ControlSize) *HotkeyInputW { w.size = s; return w }

// Status sets the validation status: "error" or "warning" (a conflict
// shows as "warning" unless a status is set).
func (w *HotkeyInputW) Status(s string) *HotkeyInputW { w.status = s; return w }

// Width sets the field width (the token's MinWidth by default).
func (w *HotkeyInputW) Width(px float32) *HotkeyInputW { w.width = px; return w }

// Disabled stops recording and clearing.
func (w *HotkeyInputW) Disabled(on bool) *HotkeyInputW { w.disabled = on; return w }

// AllowBare accepts main keys without a modifier (letters, digits, Enter).
func (w *HotkeyInputW) AllowBare(on bool) *HotkeyInputW { w.allowBare = on; return w }

// Literal stores Cmd (macOS) and Ctrl as pressed instead of CmdOrCtrl.
func (w *HotkeyInputW) Literal() *HotkeyInputW { w.literal = true; return w }

// Conflict checks the value: fn returns a message when the shortcut is
// taken or reserved ("" when it is free). The message shows under the
// field, which turns to the warning status; the value is kept, the app
// decides whether to save it.
func (w *HotkeyInputW) Conflict(fn func(accel string) string) *HotkeyInputW {
	w.conflict = fn
	return w
}

// A11yLabel sets the accessible name (a FormItem label names it too).
func (w *HotkeyInputW) A11yLabel(s string) *HotkeyInputW { w.name = s; return w }

// Changed reports that the value was recorded or cleared this frame.
func (w *HotkeyInputW) Changed() bool { w.z.run(); return w.changed }

// Recording reports whether the field is waiting for a key.
func (w *HotkeyInputW) Recording() bool {
	w.z.run()
	return State[hotkeyState](w.c, w.frame.n.id, nil).recording
}

// ConflictText returns the message of the Conflict check ("" for none).
func (w *HotkeyInputW) ConflictText() string { w.z.run(); return w.msg }

// ID returns the field's ID (the focusable frame).
func (w *HotkeyInputW) ID() ID { w.z.run(); return w.frame.n.id }

func (w *HotkeyInputW) set(v string) {
	if *w.val != v {
		*w.val = v
		w.changed = true
	}
	w.c.invalid = true
}

func (w *HotkeyInputW) build() {
	c, tk := w.c, w.tk
	loc := c.Locale()
	h := c.controlHeight(w.size)
	w.frame.Style(layout.H(h), layout.W(w.width), layout.PadVH(0, tk.PaddingInline))
	f := w.frame.n
	disabled := w.disabled || f.disabled
	f.disabled = disabled
	f.ownKeys = true
	f.setRole(a11y.RoleTextbox).setState(a11y.ReadOnly)
	if w.name != "" {
		f.setName(w.name)
	}
	w.ia = c.interact(f, true, input.CursorPointer)
	st := State[hotkeyState](c, f.id, nil)
	focused := !disabled && c.focus == f.id
	switch {
	case !focused:
		st.recording, st.mods = false, 0
	case !st.focused || (w.ia.PressedNow && !st.recording):
		st.recording, st.mods = true, 0
		c.invalid = true
	}
	st.focused = focused
	if focused {
		w.keys(st)
	}
	if w.conflict != nil && *w.val != "" {
		w.msg = w.conflict(*w.val)
	}
	status := w.status
	if status == "" && w.msg != "" {
		status = "warning"
	}

	// Content: the recorded keys, the held modifiers or a prompt, and the
	// clear button.
	var caps []string
	prompt, promptCol := "", tk.PlaceholderColor
	switch {
	case st.recording && st.mods != 0:
		caps = shortcutCaps(st.mods, "")
	case st.recording:
		prompt, promptCol = loc.HotkeyInput.Recording, tk.RecordingColor
	case *w.val != "":
		caps = ShortcutKeys(*w.val)
	default:
		prompt = w.placeholder
		if prompt == "" {
			prompt = loc.HotkeyInput.Placeholder
		}
	}
	if disabled {
		promptCol = w.th.TextDisabled
		w.frame.Opacity(0.6) // the caps fade with the frame
	}
	w.frame.Body(func() {
		if len(caps) > 0 {
			// Shortcuts read left to right in every language.
			ConfigProvider(c).Direction(LTR).Body(func() {
				Row(c, func() {
					for _, k := range caps {
						Kbd(c, k)
					}
				}, layout.Middle(), layout.Gap(tk.KeyGap))
			})
		}
		if prompt != "" {
			t := Text(c, prompt).Size(c.controlFont(w.size)).Color(promptCol).NoWrap().Ellipsis()
			t.Layout(layout.Shrink(1), layout.MinW(0))
			t.n.aHidden = true
		}
		Spacer(c)
		if *w.val != "" && !disabled && !st.recording {
			s := tk.ClearIconSize
			if IconButton(c, icon.CloseFilled, loc.HotkeyInput.ClearLabel).Size(s+6, s).Color(tk.ClearColor).NoHoverBg().Clicked() {
				w.set("")
				c.SetFocus(f.id)
			}
		}
	})
	fs := frameState{hover: w.ia.Hovered, focus: focused, disabled: disabled, status: status}
	rad := geom.Rad(c.controlRadius(w.size))
	f.paint = func(l *paint.List, r geom.Rect) { c.paintFrame(l, r, rad, f.id, fs) }
	if w.msg != "" {
		Text(c, w.msg).Size(tk.MessageFontSize).Color(tk.MessageColor).Layout(layout.MaxW(w.width))
	}
	recording := st.recording
	if c.a11yOn {
		val, msg := ShortcutLabel(*w.val), w.msg
		ph, fallback := prompt, w.placeholder
		if fallback == "" {
			fallback = loc.HotkeyInput.Placeholder
		}
		f.fill(semFunc(func(s *a11y.Node) {
			s.Value = val
			if val == "" || recording {
				s.Placeholder = ph
			}
			if s.Name == "" {
				s.Name = fallback // outside a FormItem: the placeholder names the field
			}
			addDescription(s, msg)
		}))
	}
}

// keys handles the key presses of the focused field.
func (w *HotkeyInputW) keys(st *hotkeyState) {
	c := w.c
	for _, k := range c.KeysFor(w.frame.n.id) {
		if !st.recording {
			if !k.Down {
				continue
			}
			bare := k.Mods == 0
			switch {
			case bare && (k.Key == input.KeyEnter || k.Key == input.KeySpace):
				st.recording, st.mods = true, 0
				c.invalid = true
			case bare && (k.Key == input.KeyBackspace || k.Key == input.KeyDelete):
				w.set("")
			}
			continue
		}
		if k.Key.Modifier() != 0 {
			st.mods = k.Mods
			c.invalid = true
			continue
		}
		if !k.Down || k.Repeat {
			continue
		}
		if k.Mods == 0 && (k.Key == input.KeyBackspace || k.Key == input.KeyDelete) {
			st.recording, st.mods = false, 0
			w.set("")
			continue
		}
		name, ok := accelKeyName(k)
		if !ok {
			continue
		}
		fkey := k.Key >= input.KeyF1 && k.Key <= input.KeyF12
		if k.Mods&^input.ModShift == 0 && !fkey && !w.allowBare {
			continue // a shortcut needs Ctrl, Alt or Cmd
		}
		st.recording, st.mods = false, 0
		w.set(accelFrom(k.Mods, name, w.literal))
	}
}

// accelKeyName returns the accelerator name of the main key of k.
func accelKeyName(k input.KeyEvent) (string, bool) {
	if k.Code >= input.KeyNumpad0 && k.Code <= input.KeyNumpad9 {
		return "num" + string(rune('0'+k.Code-input.KeyNumpad0)), true
	}
	key := k.Key
	switch {
	case key >= input.KeyF1 && key <= input.KeyF12:
		return key.String(), true
	case key.Rune() >= 'a' && key.Rune() <= 'z':
		return strings.ToUpper(string(key.Rune())), true
	case key.Rune() >= '0' && key.Rune() <= '9':
		return string(key.Rune()), true
	}
	switch key {
	case input.KeyEnter:
		return "Enter", true
	case input.KeyTab:
		return "Tab", true
	case input.KeySpace:
		return "Space", true
	case input.KeyBackspace:
		return "Backspace", true
	case input.KeyDelete:
		return "Delete", true
	case input.KeyInsert:
		return "Insert", true
	case input.KeyEscape:
		return "Esc", true
	case input.KeyUp, input.KeyDown, input.KeyLeft, input.KeyRight,
		input.KeyHome, input.KeyEnd, input.KeyPageUp, input.KeyPageDown:
		return key.String(), true
	case input.KeyMinus, input.KeyEqual, input.KeyBracketLeft, input.KeyBracketRight,
		input.KeyBackslash, input.KeySemicolon, input.KeyQuote, input.KeyComma,
		input.KeyPeriod, input.KeySlash, input.KeyBackquote:
		return string(key.Rune()), true
	}
	return "", false
}

// accelFrom joins modifiers and a key name into an accelerator. The
// platform shortcut modifier becomes CmdOrCtrl unless literal.
func accelFrom(m input.Modifiers, key string, literal bool) string {
	var b strings.Builder
	add := func(s string) { b.WriteString(s); b.WriteByte('+') }
	short := input.ModCtrl
	if shellMac {
		short = input.ModMeta
	}
	if !literal && m.Has(short) {
		add("CmdOrCtrl")
		m &^= short
	}
	if m.Has(input.ModCtrl) {
		add("Ctrl")
	}
	if m.Has(input.ModAlt) {
		add("Alt")
	}
	if m.Has(input.ModShift) {
		add("Shift")
	}
	if m.Has(input.ModMeta) {
		if shellMac {
			add("Cmd")
		} else {
			add("Super")
		}
	}
	b.WriteString(key)
	return b.String()
}

// parseAccel splits an accelerator into modifiers and the key name; ok is
// false when it does not parse.
func parseAccel(accel string) (m input.Modifiers, key string, ok bool) {
	accel = strings.TrimSpace(accel)
	if accel == "" {
		return 0, "", false
	}
	parts := strings.Split(accel, "+")
	if n := len(parts); n >= 2 && strings.TrimSpace(parts[n-1]) == "" && strings.TrimSpace(parts[n-2]) == "" {
		parts = append(parts[:n-2], "+") // "Ctrl++"
	}
	for _, p := range parts[:len(parts)-1] {
		switch strings.ToLower(strings.TrimSpace(p)) {
		case "cmdorctrl", "commandorcontrol":
			if shellMac {
				m |= input.ModMeta
			} else {
				m |= input.ModCtrl
			}
		case "cmd", "command", "super", "meta", "win":
			m |= input.ModMeta
		case "ctrl", "control":
			m |= input.ModCtrl
		case "alt", "option", "opt":
			m |= input.ModAlt
		case "shift":
			m |= input.ModShift
		default:
			return 0, "", false
		}
	}
	key = strings.TrimSpace(parts[len(parts)-1])
	return m, key, key != ""
}

// ShortcutKeys returns the key caps of an accelerator for this platform:
// on macOS the modifier symbols in Apple's order ("⌃", "⌥", "⇧", "⌘")
// then the key, elsewhere "Ctrl", "Alt", "Shift", "Win" / "Super" then the
// key; nil when accel does not parse. Kbd draws one cap.
func ShortcutKeys(accel string) []string {
	m, key, ok := parseAccel(accel)
	if !ok {
		return nil
	}
	return shortcutCaps(m, key)
}

// ShortcutLabel returns an accelerator as one string for this platform:
// "⌘⇧K" on macOS, "Ctrl+Shift+K" elsewhere ("" when it does not parse).
// Use it for MenuItem.Shortcut and similar hints.
func ShortcutLabel(accel string) string {
	caps := ShortcutKeys(accel)
	if shellMac {
		return strings.Join(caps, "")
	}
	return strings.Join(caps, "+")
}

// shortcutCaps returns the caps of modifiers m and key ("" for none).
func shortcutCaps(m input.Modifiers, key string) []string {
	var out []string
	if shellMac {
		for _, x := range [...]struct {
			m input.Modifiers
			s string
		}{{input.ModCtrl, "⌃"}, {input.ModAlt, "⌥"}, {input.ModShift, "⇧"}, {input.ModMeta, "⌘"}} {
			if m.Has(x.m) {
				out = append(out, x.s)
			}
		}
	} else {
		meta := "Super"
		if shellWindows {
			meta = "Win"
		}
		for _, x := range [...]struct {
			m input.Modifiers
			s string
		}{{input.ModCtrl, "Ctrl"}, {input.ModAlt, "Alt"}, {input.ModShift, "Shift"}, {input.ModMeta, meta}} {
			if m.Has(x.m) {
				out = append(out, x.s)
			}
		}
	}
	if key != "" {
		out = append(out, keyCap(key))
	}
	return out
}

// keyCap is the cap text of an accelerator key name.
func keyCap(key string) string {
	lk := strings.ToLower(key)
	if len(lk) == 4 && strings.HasPrefix(lk, "num") {
		return "Num " + lk[3:]
	}
	switch lk {
	case "up":
		return "↑"
	case "down":
		return "↓"
	case "left":
		return "←"
	case "right":
		return "→"
	case "plus":
		return "+"
	}
	if shellMac {
		switch lk {
		case "enter", "return":
			return "↩"
		case "backspace":
			return "⌫"
		case "delete", "del":
			return "⌦"
		case "esc", "escape":
			return "⎋"
		case "tab":
			return "⇥"
		case "pageup":
			return "⇞"
		case "pagedown":
			return "⇟"
		case "home":
			return "↖"
		case "end":
			return "↘"
		}
	}
	switch lk {
	case "esc", "escape":
		return "Esc"
	case "del":
		return "Delete"
	case "return":
		return "Enter"
	}
	if len(key) == 1 {
		return strings.ToUpper(key)
	}
	return key
}
