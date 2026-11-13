package automation

import (
	"fmt"
	"runtime"
	"strings"

	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/input"
)

var keyByName = func() map[string]input.Key {
	m := map[string]input.Key{}
	for k := input.Key(1); k < 1024; k++ {
		s := k.String()
		if strings.HasPrefix(s, "Key(") {
			continue
		}
		m[strings.ToLower(s)] = k
	}
	for k := input.Key(1); k < 1024; k++ {
		if r := k.Rune(); r > ' ' && r < 0x7f {
			if _, ok := m[string(r)]; !ok {
				m[string(r)] = k
			}
		}
	}
	for alias, name := range map[string]string{
		"return": "enter", "esc": "escape", "del": "delete", "arrowleft": "left", "arrowright": "right",
		"arrowup": "up", "arrowdown": "down", "pgup": "pageup", "pgdn": "pagedown", " ": "space",
		"shift": "shiftleft", "ctrl": "ctrlleft", "alt": "altleft", "meta": "metaleft",
	} {
		if k, ok := m[name]; ok {
			m[alias] = k
		}
	}
	return m
}()

// ParseMod returns the modifier named s: shift, ctrl/control, alt/option,
// meta/cmd/command/super/win, cmdorctrl (Meta on macOS, Ctrl elsewhere).
func ParseMod(s string) (input.Modifiers, bool) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "shift":
		return input.ModShift, true
	case "ctrl", "control":
		return input.ModCtrl, true
	case "alt", "option", "opt":
		return input.ModAlt, true
	case "meta", "cmd", "command", "super", "win":
		return input.ModMeta, true
	case "cmdorctrl", "commandorcontrol":
		if runtime.GOOS == "darwin" || runtime.GOOS == "ios" {
			return input.ModMeta, true
		}
		return input.ModCtrl, true
	}
	return 0, false
}

// ParseKeys parses an accelerator: modifiers and a key joined by "+"
// ("Enter", "a", "Cmd+N", "CmdOrCtrl+Shift+Z"). Key names are those of
// input.Key.String, case-insensitive, plus single characters and a few
// aliases (Return, Esc, ArrowLeft...). The plus key itself is "Shift+Equal".
func ParseKeys(s string) (input.Key, input.Modifiers, error) {
	if strings.TrimSpace(s) == "" {
		return 0, 0, fmt.Errorf("automation: empty key")
	}
	parts := strings.Split(s, "+")
	var mods input.Modifiers
	for _, p := range parts[:len(parts)-1] {
		m, ok := ParseMod(p)
		if !ok {
			return 0, 0, fmt.Errorf("automation: unknown modifier %q in %q", p, s)
		}
		mods |= m
	}
	last := parts[len(parts)-1]
	k, ok := keyByName[strings.ToLower(last)]
	if !ok {
		k, ok = keyByName[strings.ToLower(strings.TrimSpace(last))]
	}
	if !ok {
		return 0, 0, fmt.Errorf("automation: unknown key %q in %q", last, s)
	}
	return k, mods, nil
}

func parseButton(s string) (input.Button, error) {
	switch strings.ToLower(s) {
	case "", "left":
		return input.ButtonLeft, nil
	case "right":
		return input.ButtonRight, nil
	case "middle":
		return input.ButtonMiddle, nil
	}
	return 0, fmt.Errorf("automation: unknown button %q", s)
}

// Events turns in into batches of input events: each batch is delivered
// at once, and a frame should run between batches (drag steps).
func Events(in *Input) ([][]input.Event, error) {
	if in == nil {
		return nil, fmt.Errorf("automation: missing input")
	}
	var mods input.Modifiers
	for _, s := range in.Mods {
		m, ok := ParseMod(s)
		if !ok {
			return nil, fmt.Errorf("automation: unknown modifier %q", s)
		}
		mods |= m
	}
	btn, err := parseButton(in.Button)
	if err != nil {
		return nil, err
	}
	p := geom.Pt(in.X, in.Y)
	ptr := func(k input.PointerKind, at geom.Point, clicks int) input.Event {
		return input.Pointer{Kind: k, Pos: at, Button: btn, Mods: mods, Clicks: clicks}
	}
	switch in.Kind {
	case "move":
		return [][]input.Event{{ptr(input.Move, p, 0)}}, nil
	case "down":
		return [][]input.Event{{ptr(input.Move, p, 0), ptr(input.Press, p, 1)}}, nil
	case "up":
		return [][]input.Event{{ptr(input.Release, p, 1)}}, nil
	case "click":
		n := in.Clicks
		if n <= 0 {
			n = 1
		}
		evs := []input.Event{ptr(input.Move, p, 0)}
		for i := 1; i <= n; i++ {
			evs = append(evs, ptr(input.Press, p, i), ptr(input.Release, p, i))
		}
		return [][]input.Event{evs}, nil
	case "drag":
		steps := in.Steps
		if steps <= 0 {
			steps = 8
		}
		to := geom.Pt(in.X2, in.Y2)
		out := [][]input.Event{{ptr(input.Move, p, 0), ptr(input.Press, p, 1)}}
		for i := 1; i <= steps; i++ {
			f := float32(i) / float32(steps)
			out = append(out, []input.Event{ptr(input.Move, geom.Pt(p.X+(to.X-p.X)*f, p.Y+(to.Y-p.Y)*f), 0)})
		}
		return append(out, []input.Event{ptr(input.Release, to, 1)}), nil
	case "scroll":
		return [][]input.Event{{ptr(input.Move, p, 0), input.Scroll{Pos: p, DX: in.DX, DY: in.DY, Mods: mods}}}, nil
	case "key":
		k, km, err := ParseKeys(in.Keys)
		if err != nil {
			return nil, err
		}
		km |= mods
		return [][]input.Event{{
			input.KeyEvent{Key: k, Code: k, Down: true, Mods: km},
			input.KeyEvent{Key: k, Code: k, Mods: km},
		}}, nil
	case "text":
		if in.Text == "" {
			return nil, nil
		}
		return [][]input.Event{{input.TextInput{Text: in.Text}}}, nil
	}
	return nil, fmt.Errorf("automation: unknown input kind %q", in.Kind)
}
