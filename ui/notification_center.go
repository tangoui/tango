package ui

import (
	"time"

	"github.com/tangoui/tango/ui/a11y"
	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/icon"
	"github.com/tangoui/tango/ui/input"
	"github.com/tangoui/tango/ui/layout"
	"github.com/tangoui/tango/ui/paint"
	"github.com/tangoui/tango/ui/text"
	"github.com/tangoui/tango/ui/theme"
)

// NotificationItem is one entry of a NotificationCenter. The app owns the
// slice; the center marks entries read and removes them.
type NotificationItem struct {
	Key   string // stable identity (required for the list to keep state)
	Title string
	Body  string
	// Level picks the leading status icon: "info", "success", "warning",
	// "error" ("" for none).
	Level string
	// Group is the section the entry is listed under. Sections appear in
	// the order their first entry has in the slice ("" = no heading).
	Group   string
	Time    time.Time // shown relative to now ("3 分钟前"); zero hides it
	Read    bool
	Actions []NotificationAction
}

// NotificationAction is a button under a NotificationItem.
type NotificationAction struct {
	Key     string
	Label   string
	Primary bool
}

// NotificationCenterW is the bell button with its notification panel.
type NotificationCenterW struct {
	_ [0]func()

	c       *Context
	th      *theme.Theme
	tk      *theme.NotificationCenterToken
	items   *[]NotificationItem
	open    *bool
	place   string
	width   float32
	title   string
	clicked string
	action  [2]string
	changed bool
	isOpen  bool
	z       *lazy
}

type notifyCenterState struct{ open bool }

// NotificationCenter adds a bell button whose badge counts the unread
// entries of *items; it opens a panel listing them, grouped, newest
// first as the app orders them:
//
//	nc := ui.NotificationCenter(c, &notes)
//	if key, ok := nc.Clicked(); ok { open(key) }
//	if key, act, ok := nc.Action(); ok { run(key, act) }
//
// Activating an entry or one of its actions marks it read and closes the
// panel. The header marks everything read or clears the list; each entry
// can be dismissed on its own. Changed reports any such edit of *items.
// The panel is a Popover: Esc or a click outside closes it.
func NotificationCenter(c *Context, items *[]NotificationItem) *NotificationCenterW {
	w := &NotificationCenterW{c: c, items: items, place: "bottomRight"}
	w.th = c.ThemeFor(theme.CompNotificationCenter)
	w.tk = &w.th.Components.NotificationCenter
	w.width = w.tk.PanelWidth
	n := c.add("notification-center", "")
	n.ln.NoStretch = true
	w.z = c.lazy(n, w.build)
	return w
}

// Placement places the panel like Popover.Placement ("bottomRight" by
// default; "topRight" for a trigger in a status bar).
func (w *NotificationCenterW) Placement(s string) *NotificationCenterW { w.place = s; return w }

// Width sets the panel width.
func (w *NotificationCenterW) Width(px float32) *NotificationCenterW { w.width = px; return w }

// Title replaces the panel heading (Locale.A11y.Notifications).
func (w *NotificationCenterW) Title(s string) *NotificationCenterW { w.title = s; return w }

// Open binds the panel's open state (controlled mode).
func (w *NotificationCenterW) Open(v *bool) *NotificationCenterW { w.open = v; return w }

// Clicked reports the key of the entry activated this frame.
func (w *NotificationCenterW) Clicked() (string, bool) {
	w.z.run()
	return w.clicked, w.clicked != ""
}

// Action reports the entry and action keys of an action button activated
// this frame.
func (w *NotificationCenterW) Action() (key, action string, ok bool) {
	w.z.run()
	return w.action[0], w.action[1], w.action[1] != ""
}

// Changed reports that the center edited *items this frame (marked read,
// dismissed, cleared).
func (w *NotificationCenterW) Changed() bool { w.z.run(); return w.changed }

// IsOpen reports whether the panel is shown.
func (w *NotificationCenterW) IsOpen() bool { w.z.run(); return w.isOpen }

// Unread returns the number of unread entries.
func (w *NotificationCenterW) Unread() int { return notifyUnread(*w.items) }

func notifyUnread(items []NotificationItem) int {
	n := 0
	for i := range items {
		if !items[i].Read {
			n++
		}
	}
	return n
}

func (w *NotificationCenterW) build() {
	c, tk := w.c, w.tk
	loc := c.Locale()
	open := w.open
	if open == nil {
		open = &State[notifyCenterState](c, c.cur.id, nil).open
	}
	unread := notifyUnread(*w.items)
	name := loc.A11y.Notifications
	p := Popover(c).Trigger("click").Open(open).NoPadding().NoArrow().Placement(w.place).Width(w.width).
		Content(w.panel)
	p.Body(func() {
		Badge(c).Count(unread).Small().Offset(-4, 4).Body(func() {
			IconButton(c, icon.Bell, name).Size(tk.TriggerSize, tk.TriggerIconSize)
		})
	})
	w.isOpen = *open
	if w.clicked != "" || w.action[1] != "" {
		*open = false
		c.invalid = true
	}
}

// panel builds the popover content: header, then the grouped list.
func (w *NotificationCenterW) panel() {
	c, th, tk := w.c, w.th, w.tk
	loc := c.Locale()
	items := *w.items
	unread := notifyUnread(items)
	title := w.title
	if title == "" {
		title = loc.A11y.Notifications
	}
	// Header: title, unread count, mark all read, clear all.
	hdr := Row(c, func() {
		t := Text(c, title).Size(tk.HeaderFontSize).Color(tk.HeaderColor)
		if tk.HeaderFontWeight > 0 {
			t.Weight(text.Weight(tk.HeaderFontWeight))
		}
		if unread > 0 {
			Text(c, loc.NotificationCenter.Unread(unread)).Size(tk.TimeFontSize).Color(tk.TimeColor)
		}
		Spacer(c)
		hb := func(ic *icon.Icon, label string, enabled bool) bool {
			clicked := false
			Tooltip(c, label, func() {
				Disabled(c, !enabled, func() {
					clicked = IconButton(c, ic, label).Size(24, 16).Clicked()
				})
			}).Placement("bottom")
			return clicked && enabled
		}
		if hb(icon.CheckAll, loc.NotificationCenter.MarkAllRead, unread > 0) {
			for i := range *w.items {
				(*w.items)[i].Read = true
			}
			w.changed = true
		}
		if hb(icon.Clear, loc.NotificationCenter.ClearAll, len(items) > 0) {
			*w.items = (*w.items)[:0]
			w.changed = true
		}
	}, layout.Middle(), layout.Gap(8), layout.PadVH(tk.HeaderPaddingBlock, tk.HeaderPaddingInline))
	if tk.HeaderBorder.A > 0 {
		hdr.Paint(func(l *paint.List, r geom.Rect) {
			l.Fill(geom.Rect{X: r.X, Y: r.Bottom() - 1, W: r.W, H: 1}, geom.Radii{}, tk.HeaderBorder)
		})
	}
	if len(*w.items) == 0 {
		Empty(c).Simple().Description(loc.NotificationCenter.Empty).Layout(layout.PadVH(24, 16))
		return
	}
	// Sections in first-appearance order.
	var groups []string
	seen := map[string]bool{}
	for _, it := range *w.items {
		if !seen[it.Group] {
			seen[it.Group] = true
			groups = append(groups, it.Group)
		}
	}
	remove := -1
	Scroll(c, func() {
		list := Column(c, func() {
			for _, g := range groups {
				if g != "" {
					Row(c, func() {
						Text(c, g).Size(tk.GroupFontSize).Color(tk.GroupColor)
					}, layout.PadVH(tk.GroupPaddingBlock, tk.ItemPaddingInline))
				}
				for i := range *w.items {
					it := &(*w.items)[i]
					if it.Group != g {
						continue
					}
					Key(c, it.Key, func() {
						if w.entry(it) {
							remove = i
						}
					})
				}
			}
		})
		list.n.setRole(a11y.RoleList).setName(title)
	}, layout.MaxH(tk.PanelMaxHeight))
	if remove >= 0 {
		s := *w.items
		*w.items = append(s[:remove], s[remove+1:]...)
		w.changed = true
		c.invalid = true
	}
	_ = th
}

// entry builds one list entry; it reports whether the entry was dismissed.
func (w *NotificationCenterW) entry(it *NotificationItem) (dismiss bool) {
	c, th, tk := w.c, w.th, w.tk
	loc := c.Locale()
	row := c.box("notification-item", layout.Row, []layout.Option{layout.Gap(tk.ItemGap), layout.PadVH(tk.ItemPaddingBlock, tk.ItemPaddingInline)})
	row.n.setRole(a11y.RoleListItem)
	unread := !it.Read
	rowID := row.n.id
	row.Paint(func(l *paint.List, r geom.Rect) {
		switch {
		case c.pointerOver(rowID) && tk.ItemHoverBg.A > 0:
			l.Fill(r, geom.Radii{}, tk.ItemHoverBg)
		case unread && tk.ItemUnreadBg.A > 0:
			l.Fill(r, geom.Radii{}, tk.ItemUnreadBg)
		}
		if tk.ItemDivider.A > 0 {
			l.Fill(geom.Rect{X: r.X + tk.ItemPaddingInline, Y: r.Bottom() - 1, W: r.W - 2*tk.ItemPaddingInline, H: 1}, geom.Radii{}, tk.ItemDivider)
		}
		if unread {
			d := tk.UnreadDotSize
			s := c.inline(r)
			cy := r.Y + tk.ItemPaddingBlock + th.LineHeightFor(tk.TitleFontSize)/2
			x := s.X((tk.ItemPaddingInline-d)/2, d)
			l.Path(paint.NewPath().Circle(x+d/2, cy, d/2), tk.UnreadDot)
		}
	})
	row.Body(func() {
		if ic, col := notifyLevelIcon(th, it.Level); ic != nil {
			iw := Icon(c, ic).Size(tk.IconSize).Color(col)
			iw.n.ln.AlignSelf = layout.AlignStart
			iw.n.ln.Margin = geom.Insets{Top: (th.LineHeightFor(tk.TitleFontSize) - tk.IconSize) / 2}
		}
		Column(c, func() {
			// The text is one button: activating it opens the entry.
			content := c.box("notification-entry", layout.Column, []layout.Option{layout.Gap(2)})
			content.n.label = it.Title
			ia := c.interact(content.n, true, input.CursorPointer)
			content.n.setRole(a11y.RoleButton).setName(it.Title)
			body, ago := it.Body, ""
			if !it.Time.IsZero() {
				ago = loc.notifyAgo(it.Time, c.now)
			}
			if c.a11yOn {
				desc := body
				if unread {
					desc = joinDesc(loc.NotificationCenter.Unread(1), desc)
				}
				desc = joinDesc(desc, ago)
				content.n.fill(semFunc(func(s *a11y.Node) { addDescription(s, desc) }))
			}
			content.n.over = func(l *paint.List, r geom.Rect) {
				if ia.FocusVisible {
					drawFocusRing(l, th, r, geom.Rad(th.RadiusSM))
				}
			}
			content.Body(func() {
				Row(c, func() {
					t := Text(c, it.Title).Size(tk.TitleFontSize).Color(tk.TitleColor).Ellipsis().
						Layout(layout.Grow(1), layout.MinW(0))
					if unread && tk.UnreadFontWeight > 0 {
						t.Weight(text.Weight(tk.UnreadFontWeight))
					}
					t.n.aHidden = true
					if ago != "" {
						Text(c, ago).Size(tk.TimeFontSize).Color(tk.TimeColor).NoWrap().n.aHidden = true
					}
				}, layout.Gap(8), layout.Middle())
				if body != "" {
					Text(c, body).Color(tk.BodyColor).n.aHidden = true
				}
			})
			if ia.Clicked {
				it.Read = true
				w.clicked = it.Key
				w.changed = true
			}
			if len(it.Actions) > 0 {
				Row(c, func() {
					for _, a := range it.Actions {
						b := Button(c, a.Label).Small()
						if a.Primary {
							b.Primary()
						}
						if b.Clicked() {
							it.Read = true
							w.action = [2]string{it.Key, a.Key}
							w.changed = true
						}
					}
				}, layout.Gap(tk.ActionsGap), layout.Margin(geom.Insets{Top: 8}), layout.WrapItems())
			}
		}, layout.Grow(1), layout.MinW(0))
		dismiss = IconButton(c, icon.Close, loc.NotificationCenter.Dismiss).Size(20, 12).
			A11yLabel(loc.NotificationCenter.Dismiss + " " + it.Title).Clicked()
	})
	return dismiss
}

// notifyLevelIcon returns the status icon of a level and its color.
func notifyLevelIcon(th *theme.Theme, level string) (*icon.Icon, theme.Color) {
	switch level {
	case "success":
		return icon.CheckCircle, th.Success.Base
	case "warning":
		return icon.Exclamation, th.Warning.Base
	case "error":
		return icon.CloseCircle, th.Error.Base
	case "info":
		return icon.Info, th.Info.Base
	}
	return nil, theme.Color{}
}

// joinDesc joins two description parts with a separator screen readers
// pause on.
func joinDesc(a, b string) string {
	switch {
	case a == "":
		return b
	case b == "":
		return a
	}
	return a + " · " + b
}
