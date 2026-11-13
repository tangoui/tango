package ui

import (
	"strings"

	"github.com/tangoui/tango/ui/a11y"
	"github.com/tangoui/tango/ui/input"
	"github.com/tangoui/tango/ui/layout"
)

// InputTagW is a text field that turns entries into tags (Enter or comma).
type InputTagW struct {
	c       *Context
	box     *selector
	val     *[]string
	maxTags int
	changed bool
	added   string
	z       *lazy
	name    string // accessible name (A11yLabel)
}

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

func (w *InputTagW) Placeholder(s string) *InputTagW      { w.box.placeholder = s; return w }
func (w *InputTagW) AllowClear() *InputTagW               { w.box.allowClear = true; return w }
func (w *InputTagW) Status(s string) *InputTagW           { w.box.status = s; return w }
func (w *InputTagW) Size(s ControlSize) *InputTagW        { w.box.size = s; return w }
func (w *InputTagW) Width(px float32) *InputTagW          { w.box.n.ln.Width = px; return w }
func (w *InputTagW) Layout(o ...layout.Option) *InputTagW { w.box.n.ln.Style.Apply(o...); return w }
func (w *InputTagW) MaxTagCount(n int) *InputTagW         { w.box.maxCount = n; return w }
func (w *InputTagW) Disabled(on bool) *InputTagW {
	if on {
		w.box.disabled = true
	}
	return w
}

// MaxTags limits how many tags can be entered (0 = unlimited).
func (w *InputTagW) MaxTags(n int) *InputTagW { w.maxTags = n; return w }

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
	if b.disabled {
		return
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
	if b.edit().changed && strings.ContainsAny(b.st.query, ",，") { // i18n:ok separators typed by the user, not interface text
		parts := strings.FieldsFunc(b.st.query, func(r rune) bool { return r == ',' || r == '，' }) // i18n:ok separator
		rest := ""
		if !strings.HasSuffix(b.st.query, ",") && !strings.HasSuffix(b.st.query, "，") && len(parts) > 0 { // i18n:ok separator
			rest = parts[len(parts)-1]
			parts = parts[:len(parts)-1]
		}
		for _, p := range parts {
			w.add(p)
		}
		b.st.query = rest
		n := len([]rune(rest))
		b.st.edit.cur, b.st.edit.anc = n, n
	}
}
