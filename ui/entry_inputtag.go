package ui

import (
	"strings"

	"github.com/tangoui/tango/ui/a11y"
	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/input"
	"github.com/tangoui/tango/ui/internal/flexbox"
	"github.com/tangoui/tango/ui/layout"
	"github.com/tangoui/tango/ui/paint"
	"github.com/tangoui/tango/ui/theme"
)

// InputTagW is a text field that turns entries into tags (Enter or a
// separator, by default a comma).
type InputTagW struct {
	_ [0]func()

	c          *Context
	box        *selector
	val        *[]string
	maxTags    int
	changed    bool
	added      string
	z          *lazy
	name       string   // accessible name (A11yLabel)
	seps       []string // separators (nil: the defaults)
	sepsSet    bool
	draggable  bool
	saveOnBlur bool
}

// inputTagState is the per-widget state kept across frames.
type inputTagState struct {
	focused bool // the editor had focus last frame (SaveOnBlur)
	// Drag reordering (Draggable).
	drag     bool
	src      ID  // the dragged tag's node
	from, to int // dragged index and insertion index (0..len)
}

// defaultTagSeparators are the separators of a tag input that does not set
// Separators: the ASCII and the full-width comma.
var defaultTagSeparators = []string{",", "，"} // i18n:ok separators typed by the user, not interface text

// InputTag adds a tag input bound to *val.
func InputTag(c *Context, val *[]string) *InputTagW {
	w := &InputTagW{c: c, val: val}
	w.box = newSelector(c, "inputtag")
	w.box.arrow = false
	w.box.search = true
	// No popup: the frame is a group of tag remove buttons and the text
	// field (the editor) is exposed itself.
	w.box.n.setRole(a11y.RoleGroup)
	w.box.n.aState, w.box.n.aFill = 0, nil
	w.box.shadDefaults("InputTag")
	w.z = c.lazy(w.box.n, w.finish)
	return w
}

// A11yLabel sets the accessible name of the text field (inside a FormItem
// the item label is used; the placeholder is the last resort).
func (w *InputTagW) A11yLabel(s string) *InputTagW { w.name = s; return w }

func (w *InputTagW) semantics(s *a11y.Node) {
	s.Value, s.Placeholder = w.box.st.query, w.box.placeholder
	if s.Name == "" {
		s.Name = w.box.placeholder
	}
	if w.box.status == "error" {
		s.States |= a11y.Invalid
	}
}

func (w *InputTagW) Placeholder(s string) *InputTagW { w.box.placeholder = s; return w }
func (w *InputTagW) AllowClear() *InputTagW          { w.box.allowClear = true; return w }
func (w *InputTagW) Status(s string) *InputTagW      { w.box.status = s; return w }
func (w *InputTagW) Size(s ControlSize) *InputTagW   { w.box.size = s; return w }
func (w *InputTagW) Width(px float32) *InputTagW     { w.box.n.ln.Width = px; return w }
func (w *InputTagW) Layout(o ...layout.Option) *InputTagW {
	flexbox.Apply(&w.box.n.ln.Style, o...)
	return w
}
func (w *InputTagW) MaxTagCount(n int) *InputTagW { w.box.maxCount = n; return w }
func (w *InputTagW) Disabled(on bool) *InputTagW {
	if on {
		w.box.disabled = true
	}
	return w
}

// MaxTags limits how many tags can be entered (0 = unlimited).
func (w *InputTagW) MaxTags(n int) *InputTagW { w.maxTags = n; return w }

// Separators sets the strings that end a tag while typing or pasting
// (Element delimiter), e.g. Separators(";", " "); the default is the ASCII
// and the full-width comma. Separators() with no argument leaves only
// Enter. Empty strings are ignored.
func (w *InputTagW) Separators(seps ...string) *InputTagW {
	w.seps, w.sepsSet = nil, true
	for _, s := range seps {
		if s != "" {
			w.seps = append(w.seps, s)
		}
	}
	return w
}

// Draggable lets the user reorder the tags by dragging them; a drop
// reports Changed.
func (w *InputTagW) Draggable() *InputTagW { w.draggable = true; return w }

// SaveOnBlur turns the typed text into a tag when the field loses focus
// (by default the text stays in the field).
func (w *InputTagW) SaveOnBlur() *InputTagW { w.saveOnBlur = true; return w }

func (w *InputTagW) separators() []string {
	if w.sepsSet {
		return w.seps
	}
	return defaultTagSeparators
}

// splitTags splits s at any of seps. The last part is returned as rest
// (text still being typed) unless s ends with a separator.
func splitTags(s string, seps []string) (parts []string, rest string) {
	for {
		at, n := -1, 0
		for _, sep := range seps {
			if i := strings.Index(s, sep); i >= 0 && (at < 0 || i < at || (i == at && len(sep) > n)) {
				at, n = i, len(sep)
			}
		}
		if at < 0 {
			return parts, s
		}
		if p := s[:at]; strings.TrimSpace(p) != "" {
			parts = append(parts, p)
		}
		s = s[at+n:]
	}
}

// Changed reports an added or removed tag this frame.
func (w *InputTagW) Changed() bool { w.z.run(); return w.changed }

// Added returns the tag added this frame ("" if none).
func (w *InputTagW) Added() string { w.z.run(); return w.added }

func (w *InputTagW) add(s string) bool {
	s = strings.TrimSpace(s)
	if s == "" || contains(*w.val, s) || (w.maxTags > 0 && len(*w.val) >= w.maxTags) {
		return false
	}
	*w.val = append(append([]string(nil), *w.val...), s)
	w.changed, w.added = true, s
	w.c.invalid = true
	return true
}

func (w *InputTagW) finish() {
	c, b := w.c, w.box
	b.build(*w.val)
	b.ed.aHidden = false
	b.ed.setRole(a11y.RoleTextbox).fill(w)
	if w.name != "" {
		b.ed.setName(w.name)
	}
	st := State[inputTagState](c, hashID(b.n.id, "itag"), nil)
	if b.disabled {
		st.drag, st.focused = false, false
		return
	}
	if w.draggable && w.dragTags(st) {
		b.removed = -1 // a drop released on a tag's close icon is no click
	}
	if b.removed >= 0 && b.removed < len(*w.val) {
		*w.val = msWithout(*w.val, (*w.val)[b.removed])
		w.changed = true
	} else if b.clearClicked() {
		*w.val = nil
		b.st.query = ""
		w.changed = true
	}
	if b.clicked() {
		c.SetFocus(b.ed.id)
	}
	keys := b.takeKeys(func(k input.KeyEvent) bool {
		return k.Key == input.KeyEnter || (k.Key == input.KeyBackspace && b.st.query == "")
	})
	for _, k := range keys {
		switch k.Key {
		case input.KeyEnter:
			if w.add(b.st.query) || strings.TrimSpace(b.st.query) == "" || contains(*w.val, strings.TrimSpace(b.st.query)) {
				b.st.query = ""
			}
		case input.KeyBackspace:
			if n := len(*w.val); n > 0 {
				*w.val = append([]string(nil), (*w.val)[:n-1]...)
				w.changed = true
			}
		}
	}
	if seps := w.separators(); b.edit().changed && len(seps) > 0 {
		if parts, rest := splitTags(b.st.query, seps); rest != b.st.query {
			for _, p := range parts {
				w.add(p)
			}
			b.st.query = rest
			n := len([]rune(rest))
			b.st.edit.cur, b.st.edit.anc = n, n
		}
	}
	focused := b.focused()
	if w.saveOnBlur && st.focused && !focused && strings.TrimSpace(b.st.query) != "" {
		w.add(b.st.query)
		b.st.query = ""
		b.st.edit.cur, b.st.edit.anc = 0, 0
	}
	st.focused = focused
}

// dragTags runs the drag reordering of the visible tags: a press on a tag
// that moves past a few pixels starts a drag, the insertion point follows
// the pointer, and the release moves the tag. It reports a drop this frame.
func (w *InputTagW) dragTags(st *inputTagState) bool {
	c, b := w.c, w.box
	shown := len(*w.val)
	if b.maxCount > 0 && shown > b.maxCount {
		shown = b.maxCount
	}
	tags := make([]*node, 0, shown)
	for _, k := range b.n.kids {
		if len(tags) == shown || k == b.ed {
			break
		}
		tags = append(tags, k)
	}
	dropped := false
	if st.drag {
		if c.active != st.src {
			// Released (or cancelled).
			if c.released[st.src] && st.from < len(*w.val) {
				to := st.to
				if to > st.from {
					to--
				}
				if to != st.from {
					*w.val = moveTag(*w.val, st.from, to)
					w.changed = true
					c.invalid = true
				}
				dropped = true
			}
			st.drag = false
		}
	} else if c.active != 0 {
		for i, n := range tags {
			if n.id != c.active {
				continue
			}
			dx, dy := c.pointer.X-c.pressPos.X, c.pointer.Y-c.pressPos.Y
			if dx*dx+dy*dy >= 16 {
				st.drag, st.src, st.from, st.to = true, n.id, i, i
			}
			break
		}
	}
	if !st.drag {
		return dropped
	}
	c.invalid = true // follow the pointer
	// The insertion point: before the tag under (or nearest to) the
	// pointer on its row, after it past its middle.
	best, bestD := -1, float32(0)
	for i, n := range tags {
		r, ok := c.rects[n.id]
		if !ok {
			continue
		}
		dy := float32(0)
		if c.pointer.Y < r.Y {
			dy = r.Y - c.pointer.Y
		} else if c.pointer.Y > r.Bottom() {
			dy = c.pointer.Y - r.Bottom()
		}
		dx := float32(0)
		if c.pointer.X < r.X {
			dx = r.X - c.pointer.X
		} else if c.pointer.X > r.Right() {
			dx = c.pointer.X - r.Right()
		}
		if d := dy*1000 + dx; best < 0 || d < bestD {
			best, bestD = i, d
		}
	}
	if best >= 0 {
		r := c.rects[tags[best].id]
		st.to = best
		if c.inline(r).Off(c.pointer.X) > r.W/2 {
			st.to = best + 1
		}
	}
	// Fade the dragged tag and mark the insertion point.
	b.n.over = func(l *paint.List, _ geom.Rect) {
		th := c.ThemeFor(theme.CompSelect)
		if st.from < len(tags) {
			src := tags[st.from].abs
			l.Fill(src, geom.Rad(th.RadiusSM), theme.Fade(th.BgContainer, 0.6))
		}
		if len(tags) == 0 {
			return
		}
		// A 2px bar in the gap before tag st.to (after the last tag).
		var bar geom.Rect
		if st.to < len(tags) {
			ref := tags[st.to].abs
			bar = c.inline(ref).Rect(-3, ref.Y, 2, ref.H)
		} else {
			ref := tags[len(tags)-1].abs
			bar = c.inline(ref).Rect(ref.W+1, ref.Y, 2, ref.H)
		}
		l.Fill(bar, geom.Rad(1), th.Primary.Base)
	}
	return dropped
}

// moveTag returns tags with the element at from moved to index to.
func moveTag(tags []string, from, to int) []string {
	out := append([]string(nil), tags...)
	v := out[from]
	out = append(out[:from], out[from+1:]...)
	out = append(out[:to], append([]string{v}, out[to:]...)...)
	return out
}
