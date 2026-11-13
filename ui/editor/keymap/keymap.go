package keymap

import (
	"fmt"
	"strings"
	"time"

	"github.com/tangoui/tango/ui/editor/when"
	"github.com/tangoui/tango/ui/input"
)

// Rule binds key text to a command, VS Code's keybindings.json entry.
//
// A Command starting with "-" is a removal rule (Monaco 0.56's
// KeybindingResolver.handleRemovals): it removes rules for that command
// from the default rule set — the first set passed to New, normally
// DefaultRules — never rules of later sets, wherever the removal appears.
// When Key is set, only defaults whose sequence starts with Key's chords
// are removed ("ctrl+k" removes "ctrl+k ctrl+c"); when When is set, only
// defaults whose when entails it (when.Implies(default, removal): "a"
// removes a default with "a && b", not one with "a || b" or none). As in
// Monaco's whenIsEntirelyIncluded, a when of "true" counts as absent on
// either side: a removal with When "true" removes defaults with any when
// or none, and a default whose when is "true" is removed only by a removal
// without a when (or with "true").
//
// Entailment is decided structurally (see when.Implies): parentheses,
// nesting and term order are ignored, but an expression is not otherwise
// normalized. A when that is not in that flat form, for example
// "!(a || b)" where the default says "!a && !b", is a different term, so
// such a removal can miss its default; write removal whens in the form the
// default uses. A rule with an empty Command does nothing.
type Rule struct {
	Key     string
	Command string
	When    string
	Args    any
}

// Double-tap timing (MinCode's DOUBLE_TAP_MAX_PRESS_MS and
// DOUBLE_TAP_MAX_GAP_MS), applied when the Keymap has a clock (SetClock).
const (
	DoubleTapMaxPress = 300 * time.Millisecond
	DoubleTapMaxGap   = 400 * time.Millisecond
)

// binding is a compiled rule.
type binding struct {
	seq  Sequence
	cmd  string
	when when.Expr // nil: always
	args any
}

// Keymap resolves key events to commands. It holds the chord state (the
// first chord of a two-chord binding) and the double-tap state, so give
// each focus scope its own Keymap (Clone shares the compiled rules). It is
// not safe for concurrent use.
type Keymap struct {
	p    Platform
	bs   []binding // priority order: later wins
	errs []error

	pending []Chord // chords pressed so far of a multi-chord binding

	now     func() time.Time
	tapMod  input.Modifiers // modifier held for a tap, 0 for none
	tapDown time.Time
	tapOK   bool // no other key or repeat since tapMod went down
	second  bool // tapMod is the second tap
	lastMod input.Modifiers
	lastAt  time.Time
}

// New compiles rule sets for platform p: later sets take priority over
// earlier ones (defaults first, then user rules), as do later rules within
// a set. Rules that do not parse are skipped and reported by Errors.
func New(p Platform, rules ...[]Rule) *Keymap {
	k := &Keymap{p: p}
	type removal struct {
		cmd  string
		seq  Sequence
		when when.Expr
	}
	var rms []removal
	var isDefault []bool
	for i, set := range rules {
		for _, r := range set {
			b, ok := k.compile(r)
			if !ok {
				continue
			}
			if cmd, isRm := strings.CutPrefix(b.cmd, "-"); isRm {
				rms = append(rms, removal{cmd, b.seq, b.when})
				continue
			}
			k.bs = append(k.bs, b)
			isDefault = append(isDefault, i == 0)
		}
	}
	if len(rms) > 0 {
		out := k.bs[:0]
		for i, b := range k.bs {
			removed := false
			for _, r := range rms {
				if isDefault[i] && r.cmd == b.cmd && targeted(b, r.seq, r.when) {
					removed = true
					break
				}
			}
			if !removed {
				out = append(out, b)
			}
		}
		k.bs = out
	}
	return k
}

// targeted is Monaco's _isTargetedForRemoval: the removal's chords are a
// prefix of the binding's (a double tap must be the same), and the
// binding's when entails the removal's.
func targeted(b binding, seq Sequence, w when.Expr) bool {
	switch {
	case seq.Double != 0:
		if b.seq.Double != seq.Double {
			return false
		}
	case len(seq.Chords) > 0:
		if len(seq.Chords) > len(b.seq.Chords) || !matchPrefix(b.seq.Chords, seq.Chords) {
			return false
		}
	}
	if w == nil || isTrue(w) {
		return true // no when, or "true": every when is included
	}
	if b.when == nil || isTrue(b.when) || !when.Implies(b.when, w) {
		return false
	}
	return true
}

// isTrue reports whether e is the constant true (Monaco's
// ContextKeyExprType.True), which removals treat as no when.
func isTrue(e when.Expr) bool { return e.String() == "true" }

// compile parses a rule; errors are recorded.
func (k *Keymap) compile(r Rule) (binding, bool) {
	if r.Command == "" {
		return binding{}, false
	}
	var seq Sequence
	if strings.TrimSpace(r.Key) != "" {
		s, err := ParseKey(r.Key, k.p)
		if err != nil {
			k.errs = append(k.errs, fmt.Errorf("rule %q → %s: %w", r.Key, r.Command, err))
			return binding{}, false
		}
		seq = s
	} else if !strings.HasPrefix(r.Command, "-") {
		k.errs = append(k.errs, fmt.Errorf("rule for %s: no key", r.Command))
		return binding{}, false
	}
	var w when.Expr
	if strings.TrimSpace(r.When) != "" {
		e, err := when.Parse(r.When)
		if err != nil {
			k.errs = append(k.errs, fmt.Errorf("rule %q → %s: %w", r.Key, r.Command, err))
			return binding{}, false
		}
		w = e
	}
	return binding{seq: seq, cmd: r.Command, when: w, args: r.Args}, true
}

// Errors returns the rules New skipped, one error each.
func (k *Keymap) Errors() []error { return k.errs }

// Platform returns the Keymap's platform.
func (k *Keymap) Platform() Platform { return k.p }

// Clone returns a Keymap with the same rules and fresh chord and
// double-tap state.
func (k *Keymap) Clone() *Keymap {
	return &Keymap{p: k.p, bs: k.bs, errs: k.errs, now: k.now}
}

// SetClock gives the Keymap a clock for the double-tap timing: each press
// must be shorter than DoubleTapMaxPress and the gap between the taps
// shorter than DoubleTapMaxGap. Without a clock any two clean taps count.
func (k *Keymap) SetClock(now func() time.Time) { k.now = now }

// Pending reports whether the first chord of a multi-chord binding was
// pressed and the next key completes or cancels it.
func (k *Keymap) Pending() bool { return len(k.pending) > 0 }

// ClearPending cancels a pending chord and a double tap in progress (focus
// lost, window deactivated). Chords have no timeout otherwise.
func (k *Keymap) ClearPending() {
	k.pending = k.pending[:0]
	k.resetTap()
}

// ResetTap voids a double tap in progress. Call it when committed text
// arrives without key events (macOS and Linux deliver text-producing keys
// as text only), so Shift held for a capital letter is not a Shift tap.
func (k *Keymap) ResetTap() { k.resetTap() }

func (k *Keymap) resetTap() {
	k.tapMod, k.tapOK, k.second, k.lastMod = 0, false, false, 0
}

// Keys returns the sequences bound to command whose when holds in ctx (a
// nil ctx ignores when), highest priority first: for menus and the command
// palette.
func (k *Keymap) Keys(command string, ctx when.Context) []Sequence {
	var out []Sequence
	for i := len(k.bs) - 1; i >= 0; i-- {
		b := &k.bs[i]
		if b.cmd == command && (ctx == nil || b.when == nil || b.when.Eval(ctx)) {
			out = append(out, b.seq)
		}
	}
	return out
}

// Resolve feeds one key event (presses, repeats and releases; releases
// matter only to double taps) and returns the command it triggers with the
// rule's args. pending is true when ev was the first chord of a two-chord
// binding: the caller swallows the key and the next press completes the
// binding (or cancels it: a press that matches no continuation returns
// no command and clears the state, VS Code's behaviour). Modifier keys
// pressed alone keep a pending chord.
//
// Among the rules whose sequence starts with the chords pressed so far,
// the last one (highest priority) whose when holds in ctx wins; if it is
// longer than the chords pressed, the result is pending.
func (k *Keymap) Resolve(ev input.KeyEvent, ctx when.Context) (cmd string, args any, pending bool) {
	return k.ResolveEnabled(ev, ctx, nil)
}

// ResolveEnabled is Resolve where a rule also needs enabled(command) to
// hold (nil: always), as if the command's precondition were part of the
// rule's when (Monaco registers editor commands' keybindings with when =
// precondition && kbExpr): the key of an unavailable command is not
// consumed and does not shadow lower-priority rules.
func (k *Keymap) ResolveEnabled(ev input.KeyEvent, ctx when.Context, enabled func(command string) bool) (cmd string, args any, pending bool) {
	ok := func(b *binding) bool {
		return (b.when == nil || b.when.Eval(ctx)) && (enabled == nil || enabled(b.cmd))
	}
	if mod := physical(ev).Modifier(); mod != 0 {
		cmd, args := k.modifierEvent(ev, mod, ok)
		return cmd, args, len(k.pending) > 0
	}
	if !ev.Down {
		return "", nil, len(k.pending) > 0
	}
	k.resetTap()
	cands := chordsOf(ev)
	if len(cands) == 0 {
		return "", nil, len(k.pending) > 0
	}
	prefix := k.pending
	n := len(prefix) + 1
	for i := len(k.bs) - 1; i >= 0; i-- {
		b := &k.bs[i]
		if len(b.seq.Chords) < n || !matchPrefix(b.seq.Chords, prefix) || !matchChord(b.seq.Chords[n-1], cands) {
			continue
		}
		if !ok(b) {
			continue
		}
		if len(b.seq.Chords) > n {
			k.pending = append(k.pending, b.seq.Chords[n-1])
			return "", nil, true
		}
		k.pending = k.pending[:0]
		return b.cmd, b.args, false
	}
	k.pending = k.pending[:0]
	return "", nil, false
}

func matchPrefix(chords, prefix []Chord) bool {
	for i, c := range prefix {
		if chords[i] != c {
			return false
		}
	}
	return true
}

func matchChord(c Chord, cands []Chord) bool {
	for _, x := range cands {
		if c == x {
			return true
		}
	}
	return false
}

// modifierEvent tracks double taps: a modifier pressed and released alone,
// twice; the second release fires.
func (k *Keymap) modifierEvent(ev input.KeyEvent, mod input.Modifiers, ok func(*binding) bool) (string, any) {
	var now time.Time
	if k.now != nil {
		now = k.now()
	}
	if ev.Down {
		if ev.Repeat {
			if mod == k.tapMod {
				k.tapOK = false
			}
			return "", nil
		}
		if ev.Mods&modMask&^mod != 0 {
			k.resetTap()
			return "", nil
		}
		if k.lastMod != 0 && (k.lastMod != mod || (k.now != nil && now.Sub(k.lastAt) > DoubleTapMaxGap)) {
			k.lastMod = 0
		}
		k.second = k.lastMod != 0
		k.tapMod, k.tapDown, k.tapOK = mod, now, true
		return "", nil
	}
	if mod != k.tapMod || !k.tapOK || (k.now != nil && now.Sub(k.tapDown) > DoubleTapMaxPress) {
		k.resetTap()
		return "", nil
	}
	k.tapMod, k.tapOK = 0, false
	if !k.second {
		k.lastMod, k.lastAt = mod, now
		return "", nil
	}
	k.resetTap()
	for i := len(k.bs) - 1; i >= 0; i-- {
		b := &k.bs[i]
		if b.seq.Double == mod && ok(b) {
			return b.cmd, b.args
		}
	}
	return "", nil
}
