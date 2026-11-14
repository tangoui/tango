package ui

import (
	"strings"
	"time"

	"github.com/tangoui/tango/ui/a11y"
	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/icon"
	"github.com/tangoui/tango/ui/input"
	"github.com/tangoui/tango/ui/layout"
	"github.com/tangoui/tango/ui/paint"
	"github.com/tangoui/tango/ui/theme"
)

// ---------------------------------------------------------------------------
// Conversations (Ant Design X Conversations)

// Conversation is one entry of a Conversations list.
type Conversation struct {
	_ [0]func()

	Key      string
	Title    string     // "" shows Locale.Chat.Untitled
	Time     time.Time  // last activity: groups the list by day unless Group is set
	Group    string     // explicit group label
	Icon     *icon.Icon // leading icon (nil: none)
	Running  bool       // a reply is being generated (spinner)
	Disabled bool
}

type convState struct {
	renaming string // key being renamed
	draft    string
	focusIn  bool   // focus the rename input next frame
	refocus  string // give focus back to this row after a rename
}

// ConversationsW is a list of conversations grouped by time (today,
// yesterday, previous 7 / 30 days, earlier) with selection and hover
// actions to rename (F2) and delete (Delete).
type ConversationsW struct {
	_ [0]func()

	c        *Context
	b        *Box
	st       *convState
	z        *lazy
	loc      *Locale
	items    []Conversation
	active   *string
	noGroups bool
	noRename bool
	noDelete bool

	selected, renamedKey, renamedTitle, deleted string
}

// Conversations adds a conversation list; *active holds the selected key.
// Put it in a Scroll for long lists.
//
//	cv := ui.Conversations(c, convs, &cur)
//	if k, t, ok := cv.Renamed(); ok { rename(k, t) }
//	if k, ok := cv.Deleted(); ok { remove(k) }
func Conversations(c *Context, items []Conversation, active *string) *ConversationsW {
	w := &ConversationsW{c: c, items: items, active: active, loc: c.Locale()}
	w.b = c.box("conversations", layout.Column, nil)
	w.b.n.ln.Gap = 2
	w.b.n.setRole(a11y.RoleListbox).setName(w.loc.Chat.Conversations)
	w.st = State[convState](c, w.b.n.id, nil)
	w.z = c.lazy(w.b.n, w.build)
	return w
}

// NoGroups lists the conversations without group headers.
func (w *ConversationsW) NoGroups() *ConversationsW { w.noGroups = true; return w }

// NoRename hides the rename action.
func (w *ConversationsW) NoRename() *ConversationsW { w.noRename = true; return w }

// NoDelete hides the delete action.
func (w *ConversationsW) NoDelete() *ConversationsW { w.noDelete = true; return w }

// A11yLabel sets the accessible name (default Locale.Chat.Conversations).
func (w *ConversationsW) A11yLabel(s string) *ConversationsW { w.b.n.setName(s); return w }

// Layout applies layout options to the list.
func (w *ConversationsW) Layout(opts ...layout.Option) *ConversationsW {
	w.b.Style(opts...)
	return w
}

// Selected returns the key selected this frame.
func (w *ConversationsW) Selected() (string, bool) { w.z.run(); return w.selected, w.selected != "" }

// Renamed returns the key and new title of a rename committed this frame.
func (w *ConversationsW) Renamed() (key, title string, ok bool) {
	w.z.run()
	return w.renamedKey, w.renamedTitle, w.renamedKey != ""
}

// Deleted returns the key whose delete action was used this frame.
func (w *ConversationsW) Deleted() (string, bool) { w.z.run(); return w.deleted, w.deleted != "" }

// convGroup returns the time group label of t relative to now.
func convGroup(l *Locale, now, t time.Time) string {
	if t.IsZero() {
		return ""
	}
	y, m, d := now.Date()
	today := time.Date(y, m, d, 0, 0, 0, 0, now.Location())
	t = t.In(now.Location())
	switch {
	case !t.Before(today):
		return l.Chat.Today
	case !t.Before(today.AddDate(0, 0, -1)):
		return l.Chat.Yesterday
	case !t.Before(today.AddDate(0, 0, -7)):
		return l.Chat.Last7Days
	case !t.Before(today.AddDate(0, 0, -30)):
		return l.Chat.Last30Days
	}
	return l.Chat.Earlier
}

func (w *ConversationsW) build() {
	c := w.c
	th := c.ThemeFor(theme.CompChat)
	tk := &th.Components.Chat
	var nodes []*node
	var ids []ID
	cur := -1
	group := ""
	for i := range w.items {
		it := w.items[i]
		if !w.noGroups {
			g := it.Group
			if g == "" {
				g = convGroup(w.loc, c.now, it.Time)
			}
			if g != group && g != "" {
				group = g
				Key(c, "g:"+g, func() {
					in := geom.Insets{Left: 12, Right: 12, Top: 12, Bottom: 4}
					if i == 0 {
						in.Top = 0
					}
					Row(c, func() {
						Text(c, g).Size(th.FontSizeSM).Color(tk.ConvGroupColor).NoWrap()
					}, layout.Padding(in))
				})
			}
		}
		Key(c, it.Key, func() {
			n := w.item(th, tk, it)
			if n != nil {
				nodes = append(nodes, n)
				ids = append(ids, n.id)
				if w.active != nil && *w.active == it.Key {
					cur = len(nodes) - 1
				}
			}
		})
	}
	rovingTabStop(nodes, cur)
	if to := rovingStep(c, ids, func(i int) bool { return !nodes[i].disabled }, rovingVertical); to >= 0 {
		navFocus(c, ids[to])
	}
}

// item builds one row and returns its node (nil while renaming).
func (w *ConversationsW) item(th *theme.Theme, tk *theme.ChatToken, it Conversation) *node {
	c, st := w.c, w.st
	loc := w.loc
	title := it.Title
	if title == "" {
		title = loc.Chat.Untitled
	}
	if st.renaming == it.Key && !w.noRename {
		w.renameRow(th, tk, it)
		return nil
	}
	row := Row(c, nil, layout.Gap(8), layout.Middle(), layout.H(tk.ConvItemHeight), layout.PadVH(0, 12))
	n := row.n
	sel := w.active != nil && *w.active == it.Key
	n.label = title
	n.setRole(a11y.RoleOption).setName(title).setState(a11y.Selectable).stateIf(sel, a11y.Selected)
	if it.Disabled {
		n.disabled = true
	}
	ia := c.interact(n, true, input.CursorPointer)
	if st.refocus == it.Key {
		st.refocus = ""
		if c.focus == 0 {
			navFocus(c, n.id)
		}
	}
	hover := ia.Hovered || c.pointerOver(n.id)
	if sel {
		row.Bg(tk.ConvActiveBg)
	} else if hover && !it.Disabled {
		row.Bg(tk.ConvHoverBg)
	}
	row.Radius(tk.ConvItemRadius)
	n.over = func(l *paint.List, r geom.Rect) {
		if ia.FocusVisible {
			drawFocusRing(l, th, r, geom.Rad(tk.ConvItemRadius))
		}
	}
	if ia.Clicked && !it.Disabled && !sel {
		if w.active != nil {
			*w.active = it.Key
		}
		w.selected = it.Key
		c.invalid = true
	}
	// Keyboard actions on the focused row.
	for _, k := range msTakeKeys(c, n.id, func(k input.KeyEvent) bool {
		return (k.Key == input.KeyF2 && !w.noRename) || ((k.Key == input.KeyDelete || k.Key == input.KeyBackspace) && !w.noDelete && k.Mods == 0)
	}) {
		if k.Key == input.KeyF2 {
			w.startRename(it.Key, it.Title)
		} else {
			w.deleted = it.Key
		}
	}
	col := th.Text
	switch {
	case it.Disabled:
		col = th.TextDisabled
	case sel:
		col = tk.ConvActiveColor
	}
	c.body(n, func() {
		if it.Running || it.Icon != nil {
			ic := c.add("conv-icon", "")
			ic.aHidden = true
			ic.ln.Width, ic.ln.Height = 16, 16
			running, glyph := it.Running, it.Icon
			ic.paint = func(l *paint.List, r geom.Rect) {
				if running {
					c.paintIcon(l, icon.Loading, r, th.Primary.Base, spinAngle(c))
					return
				}
				c.paintIcon(l, glyph, r, col, 0)
			}
			if running {
				n.setState(a11y.Busy)
			}
		}
		t := Text(c, title).Color(col).Ellipsis().Layout(layout.Grow(1), layout.Basis(0), layout.MinW(0))
		t.n.aHidden = true
		show := (hover || c.focusWithin(n)) && !it.Disabled
		if show && (!w.noRename || !w.noDelete) {
			act := func(ic *icon.Icon, label string) bool {
				var clicked bool
				Tooltip(c, label, func() {
					b := IconButton(c, ic, label).Size(24, 14)
					b.n.noTab, b.n.aKbd = true, true // F2 / Delete on the row
					clicked = b.Clicked()
				})
				return clicked
			}
			if !w.noRename && act(icon.Edit, loc.Chat.Rename) {
				w.startRename(it.Key, it.Title)
			}
			if !w.noDelete && act(icon.Delete, loc.Delete) {
				w.deleted = it.Key
			}
		}
	})
	return n
}

func (w *ConversationsW) startRename(key, title string) {
	w.st.renaming, w.st.draft, w.st.focusIn = key, title, true
	w.c.invalid = true
}

func (w *ConversationsW) renameRow(th *theme.Theme, tk *theme.ChatToken, it Conversation) {
	c, st := w.c, w.st
	in := Input(c, &st.draft).Size(Small).A11yLabel(w.loc.Chat.Rename).
		Layout(layout.H(tk.ConvItemHeight-4), layout.Margin(geom.Insets{Top: 2, Bottom: 2}))
	id := in.ID()
	if st.focusIn {
		st.focusIn = false
		c.SetFocus(id)
		return
	}
	commit := in.Submitted() || c.focus != id
	if c.focus == id && c.Escape() {
		st.renaming, st.refocus = "", it.Key
		c.SetFocus(0)
		return
	}
	if commit {
		t := strings.TrimSpace(st.draft)
		if t != "" && t != it.Title {
			w.renamedKey, w.renamedTitle = it.Key, t
		}
		if c.focus == id {
			st.refocus = it.Key
			c.SetFocus(0)
		}
		st.renaming = ""
		c.invalid = true
	}
}
