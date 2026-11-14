package ui

import (
	"github.com/tangoui/tango/ui/a11y"
	"strconv"
	"time"

	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/icon"
	"github.com/tangoui/tango/ui/input"
	"github.com/tangoui/tango/ui/internal/flexbox"
	"github.com/tangoui/tango/ui/layout"
	"github.com/tangoui/tango/ui/paint"
	"github.com/tangoui/tango/ui/text"
	"github.com/tangoui/tango/ui/theme"
)

// ---------------------------------------------------------------------------
// Message (global toasts at the top of the window)

type toast struct {
	id        ID
	status    string
	title     string
	start     time.Time
	dur       time.Duration
	env       *env // scope Message was called in (theme, direction, locale)
	placement MessagePlacement
	offset    float32 // distance from the window edge; 0: MessageToken.Top
	closable  bool
	plain     bool

	noPause  bool          // PauseOnHover(false)
	paused   time.Duration // time spent hovered
	pausedAt time.Time     // hovered since (zero: counting)
	row      ID            // toast node last frame (hover check)
	ic       *icon.Icon    // Icon
	onClose  func()
	onClick  func()
	closed   bool // onClose ran
	group    bool // Grouping
	repeat   int  // Grouping counter (EP repeatNum)
	fresh    bool // not rendered yet (Grouping and maxCount apply on the first render)
	maxCount int  // Defaults("Message", maxCount) where it was opened (0: none)
}

// elapsed is the visible time of the message, without hovered time.
func (m *toast) elapsed(now time.Time) time.Duration {
	e := now.Sub(m.start) - m.paused
	if !m.pausedAt.IsZero() {
		e -= now.Sub(m.pausedAt)
	}
	return e
}

// leaving reports a message whose time is up (it fades out).
func (m *toast) leaving(now time.Time) bool { return m.elapsed(now) >= m.dur }

// hover pauses or resumes the countdown of a visible message (antd
// pauseOnHover, default true; EP pauses on mouseenter too).
func (m *toast) hover(c *Context, over bool) {
	if m.noPause || m.leaving(c.now) {
		return
	}
	switch {
	case over && m.pausedAt.IsZero():
		m.pausedAt = c.now
	case !over && !m.pausedAt.IsZero():
		m.paused += c.now.Sub(m.pausedAt)
		m.pausedAt = time.Time{}
	}
}

// close ends the message now (close button, MessageDestroy).
func (m *toast) close(now time.Time) {
	if !m.pausedAt.IsZero() {
		m.paused += now.Sub(m.pausedAt)
		m.pausedAt = time.Time{}
	}
	m.dur = max(0, m.elapsed(now))
}

// MessagePlacement is where Message toasts appear. The start / end
// placements are logical: in RTL the end is the left side. The zero value
// is the design system's default (top center; shadcn/ui: the bottom end
// corner, as Sonner).
type MessagePlacement string

const (
	MessageTop         MessagePlacement = "top"
	MessageTopStart    MessagePlacement = "top-start"
	MessageTopEnd      MessagePlacement = "top-end"
	MessageBottom      MessagePlacement = "bottom"
	MessageBottomStart MessagePlacement = "bottom-start"
	MessageBottomEnd   MessagePlacement = "bottom-end"
)

// bottom reports a placement at the bottom edge.
func (p MessagePlacement) bottom() bool {
	return p == MessageBottom || p == MessageBottomStart || p == MessageBottomEnd
}

// align is the cross-axis alignment of the toasts of the placement.
func (p MessagePlacement) align() layout.Align {
	switch p {
	case MessageTopStart, MessageBottomStart:
		return layout.AlignStart
	case MessageTopEnd, MessageBottomEnd:
		return layout.AlignEnd
	}
	return layout.AlignCenter
}

// sonner maps the placement to the Sonner stack corner ("" is the bottom
// end corner).
func (p MessagePlacement) sonner(rtl bool) NotifyPlacement {
	var n NotifyPlacement
	switch p {
	case MessageTop:
		n = NotifyTop
	case MessageTopStart:
		n = NotifyTopStart
	case MessageTopEnd:
		n = NotifyTopEnd
	case MessageBottom:
		n = NotifyBottom
	case MessageBottomStart:
		n = NotifyBottomStart
	default:
		n = NotifyBottomEnd
	}
	return n.resolve(rtl)
}

type toasts struct {
	msgs []*toast
	seq  int
}

func (c *Context) toasts() *toasts {
	return State[toasts](c, hashID("global-toasts"), nil)
}

// MessageDuration is how long Message toasts stay visible by default.
var MessageDuration = 3 * time.Second

const navForever = 100 * 365 * 24 * time.Hour

// MessageRef configures a message opened by Message. Use it in the same
// call chain; it does not need to be kept.
type MessageRef struct {
	_ [0]func()

	t   *toasts
	m   *toast
	now time.Time
}

// Message shows a global toast at the top of the window and returns a
// reference for further options. status: success, info, warning, error,
// loading.
//
//	ui.Message(c, "loading", "保存中…").Key("save").Duration(0)
//	ui.Message(c, "success", "已保存").Key("save") // replaces the loading one
func Message(c *Context, status, content string) *MessageRef {
	t := c.toasts()
	t.seq++
	m := &toast{id: hashID("msg-key", "auto-"+strconv.Itoa(t.seq)), status: status, title: content, start: c.now, dur: messageDuration(c, "Message", MessageDuration), env: c.env,
		noPause: !Prop(c, "Message", "pauseOnHover", true), fresh: true}
	if n, ok := numProp(c.ConfigProp("Message", "maxCount")); ok && n >= 1 {
		m.maxCount = int(n) // applied when rendered, after Key / Grouping (EP order)
	}
	t.msgs = append(t.msgs, m)
	c.invalid = true
	return &MessageRef{t: t, m: m, now: c.now}
}

// Key names the message; a message with the same key already on screen is
// updated in place (content, status and a restarted timer) instead of
// stacking a new one.
func (r *MessageRef) Key(key string) *MessageRef {
	id := hashID("msg-key", key)
	for _, m := range r.t.msgs {
		if m.id == id && m != r.m {
			m.status, m.title = r.m.status, r.m.title
			m.start, m.dur, m.env = r.now, r.m.dur, r.m.env
			m.placement, m.offset, m.closable, m.plain = r.m.placement, r.m.offset, r.m.closable, r.m.plain
			m.noPause, m.ic, m.onClose, m.onClick, m.group = r.m.noPause, r.m.ic, r.m.onClose, r.m.onClick, r.m.group
			m.paused, m.pausedAt, m.closed = 0, time.Time{}, false
			r.t.remove(r.m)
			r.m = m
			return r
		}
	}
	r.m.id = id
	return r
}

// Duration sets how long the message stays; d <= 0 keeps it until
// MessageDestroy or an update with a positive duration.
func (r *MessageRef) Duration(d time.Duration) *MessageRef {
	if d <= 0 {
		d = navForever
	}
	r.m.dur = d
	return r
}

// Placement selects where the message appears (MessageTop by default;
// shadcn/ui: the bottom end corner). Messages of each placement stack
// separately; bottom placements stack upwards.
func (r *MessageRef) Placement(p MessagePlacement) *MessageRef { r.m.placement = p; return r }

// Offset sets the distance in px from the window's top (or bottom) edge to
// the stack (default MessageToken.Top). The newest message of a placement
// decides it. shadcn/ui's Sonner stack keeps its own inset.
func (r *MessageRef) Offset(px float32) *MessageRef { r.m.offset = px; return r }

// ShowClose adds a close button that dismisses the message.
func (r *MessageRef) ShowClose() *MessageRef { r.m.closable = true; return r }

// Plain draws the message on the neutral popup background with only the
// icon in the status color (Element plain); Ant's message already looks
// this way.
func (r *MessageRef) Plain() *MessageRef { r.m.plain = true; return r }

// PauseOnHover pauses the countdown while the pointer is over the message
// (antd pauseOnHover, default true; Defaults key "pauseOnHover").
func (r *MessageRef) PauseOnHover(on bool) *MessageRef { r.m.noPause = !on; return r }

// Icon replaces the status icon (antd icon).
func (r *MessageRef) Icon(ic *icon.Icon) *MessageRef { r.m.ic = ic; return r }

// OnClose runs fn once when the message has closed (timed out, closed or
// destroyed), as antd onClose / EP onClose.
func (r *MessageRef) OnClose(fn func()) *MessageRef { r.m.onClose = fn; return r }

// OnClick runs fn when the message is clicked (antd onClick).
func (r *MessageRef) OnClick(fn func()) *MessageRef { r.m.onClick = fn; return r }

// Grouping merges this message into a visible one with the same content
// and placement: that one restarts its timer, takes the new options and
// shows a repeat counter (EP grouping / repeatNum). The merge happens when
// the message is first shown, so the options chained after Grouping apply.
func (r *MessageRef) Grouping() *MessageRef { r.m.group = true; return r }

// groupInto merges the fresh message m into an older visible one with the
// same content and placement and reports whether it did.
func (t *toasts) groupInto(c *Context, m *toast) bool {
	for _, o := range t.msgs {
		if o == m || o.fresh || o.title != m.title || o.leaving(c.now) || o.place(c) != m.place(c) {
			continue
		}
		o.repeat = max(1, o.repeat) + 1
		o.status, o.start, o.dur, o.env = m.status, m.start, m.dur, m.env
		o.offset, o.closable, o.plain, o.noPause = m.offset, m.closable, m.plain, m.noPause
		o.maxCount = m.maxCount
		if m.ic != nil {
			o.ic = m.ic
		}
		if m.onClose != nil { // options the repeat does not set stay
			o.onClose = m.onClose
		}
		if m.onClick != nil {
			o.onClick = m.onClick
		}
		o.paused, o.pausedAt, o.group, o.closed = 0, time.Time{}, true, false
		return true
	}
	return false
}

// settleNew applies Grouping and the maxCount default to messages opened
// since the last render: grouped ones merge into their twin, then the
// oldest visible messages beyond the newest message's maxCount close (they
// fade out and run OnClose), as EP does grouping before max.
func (t *toasts) settleNew(c *Context) {
	limit := 0
	keep := t.msgs[:0]
	for _, m := range t.msgs {
		if m.fresh {
			m.fresh = false
			if m.group && t.groupInto(c, m) {
				continue
			}
			limit = m.maxCount
		}
		keep = append(keep, m)
	}
	t.msgs = keep
	if limit < 1 {
		return
	}
	open := 0
	for i := len(t.msgs) - 1; i >= 0; i-- {
		if m := t.msgs[i]; !m.leaving(c.now) {
			if open++; open > limit {
				m.close(c.now)
			}
		}
	}
}

func (t *toasts) remove(m *toast) {
	for i, x := range t.msgs {
		if x == m {
			t.msgs = append(t.msgs[:i], t.msgs[i+1:]...)
			return
		}
	}
}

// MessageDestroy closes the message with key ("" closes all).
func MessageDestroy(c *Context, key string) {
	t := c.toasts()
	id := hashID("msg-key", key)
	for _, m := range t.msgs {
		if key == "" || m.id == id {
			m.close(c.now)
		}
	}
	c.invalid = true
}

func init() { globalLayers = append(globalLayers, renderMessages) }

// Messages and notifications are drawn by global layers after the page,
// outside every ConfigProvider scope; each one keeps the env it was opened
// in and is built inside it (theme, direction and locale of the caller).

// withEnv runs fn with e as the current scope (nil: the current one).
func (c *Context) withEnv(e *env, fn func()) {
	if e == nil || e == c.env {
		fn()
		return
	}
	prev, prevTh := c.enterEnv(e)
	fn()
	c.leaveEnv(prev, prevTh)
}

// liveRole is the live-region role of a message of status: errors and
// warnings interrupt (alert, assertive), the rest is polite (status).
func liveRole(status string) a11y.Role {
	if status == "error" || status == "warning" {
		return a11y.RoleAlert
	}
	return a11y.RoleStatus
}

func renderMessages(c *Context) {
	t := State[toasts](c, hashID("global-toasts"), nil)
	if len(t.msgs) == 0 {
		return
	}
	t.settleNew(c)
	keep := t.msgs[:0]
	for _, m := range t.msgs {
		e := m.elapsed(c.now)
		fade := c.Theme.DurationMid
		if c.noAnim {
			fade = 0 // no leave animation: gone at once (as Notification)
		}
		if e < m.dur+fade {
			keep = append(keep, m)
			if m.dur < navForever && m.pausedAt.IsZero() && e < m.dur {
				c.WakeAt(c.now.Add(m.dur - e))
			}
			continue
		}
		if m.onClose != nil && !m.closed {
			m.closed = true
			m.onClose()
		}
	}
	t.msgs = keep
	if len(t.msgs) == 0 {
		return
	}
	// Each placement is its own stack, following the scope of its newest
	// message; each toast is built in its own scope.
	for _, g := range messageGroups(c, t.msgs) {
		g := g
		newest := g[len(g)-1]
		pl := newest.place(c)
		c.withEnv(newest.env, func() {
			if c.Theme.Flavor == theme.StyleShadcn {
				renderMessagesSonner(c, pl, g)
				return
			}
			renderMessageGroup(c, pl, newest.offset, g)
		})
	}
}

// place returns the message's placement with the default resolved for
// the design system it was opened under.
func (m *toast) place(c *Context) MessagePlacement {
	if m.placement != "" {
		return m.placement
	}
	th := c.Theme
	if m.env != nil && m.env.th != nil {
		th = m.env.th
	}
	if th.Flavor == theme.StyleShadcn {
		return MessageBottomEnd
	}
	return MessageTop
}

// messageGroups splits msgs (oldest first) by placement, in order of each
// placement's first message.
func messageGroups(c *Context, msgs []*toast) [][]*toast {
	if len(msgs) == 0 {
		return nil
	}
	first := msgs[0].place(c)
	same := true
	for _, m := range msgs[1:] {
		if m.place(c) != first {
			same = false
			break
		}
	}
	if same {
		return [][]*toast{msgs}
	}
	var groups [][]*toast
	at := map[MessagePlacement]int{}
	for _, m := range msgs {
		p := m.place(c)
		i, ok := at[p]
		if !ok {
			i = len(groups)
			at[p] = i
			groups = append(groups, nil)
		}
		groups[i] = append(groups[i], m)
	}
	return groups
}

// renderMessageGroup builds the Ant / Element stack of placement pl,
// off px from the window edge (0: MessageToken.Top).
func renderMessageGroup(c *Context, pl MessagePlacement, off float32, g []*toast) {
	tk := &c.ThemeFor(theme.CompMessage).Components.Message
	if off <= 0 {
		off = tk.Top
	}
	if pl == MessageTop {
		ov := Overlay(c, OverlayOptions{Place: ScreenTop, Margin: off}, func() {
			for _, m := range g {
				c.withEnv(m.env, func() { messageRow(c, m, layout.AlignCenter, false) })
			}
		})
		ov.n.hit = false
		return
	}
	bottom := pl.bottom()
	pad := geom.Insets{Top: off, Left: tk.MarginInline, Right: tk.MarginInline}
	just := layout.JustifyStart
	if bottom {
		pad = geom.Insets{Bottom: off, Left: tk.MarginInline, Right: tk.MarginInline}
		just = layout.JustifyEnd
	}
	ov := Overlay(c, OverlayOptions{Place: ScreenFill}, func() {
		// Bottom stacks grow upwards: the oldest stays at the edge.
		for i := range g {
			m := g[i]
			if bottom {
				m = g[len(g)-1-i]
			}
			c.withEnv(m.env, func() { messageRow(c, m, pl.align(), bottom) })
		}
	}, layout.Padding(pad), layout.JustifyContent(just))
	ov.n.hit, ov.n.blocks = false, false
}

// messageRow builds one Ant / Element message toast aligned on the cross
// axis with align; bottom stacks enter from below.
func messageRow(c *Context, m *toast, align layout.Align, bottom bool) {
	th := c.ThemeFor(theme.CompMessage)
	tk := &th.Components.Message
	el := th.Flavor == theme.Element
	pal := th.ColorOf(m.status)
	m.hover(c, c.pointerOver(m.row))
	leaving := m.leaving(c.now)
	target := float32(1)
	if leaving {
		target = 0
	}
	a := c.AnimFrom(m.id, "in", 0, target, th.DurationMid)
	margin := geom.Insets{Top: (a - 1) * 16, Bottom: tk.Gap}
	if bottom {
		margin = geom.Insets{Top: tk.Gap, Bottom: (a - 1) * 16}
	}
	row := Row(c, func() {
		switch {
		case m.ic != nil:
			Icon(c, m.ic).Size(16).Color(pal.Base)
		case m.status == "loading":
			Icon(c, icon.Loading).Spin().Size(16).Color(th.Primary.Base) // antd icon-loading: colorInfo (= colorPrimary)
		default:
			statusIcon(c, m.status, 16).ln.AlignSelf = layout.AlignCenter
		}
		t := Text(c, m.title).NoWrap()
		if tk.ContentFontSize > 0 {
			t.Size(tk.ContentFontSize)
		}
		switch {
		case m.plain:
			t.Color(tk.PlainColor)
		case tk.Color.A > 0:
			t.Color(tk.Color)
		case el:
			t.Color(pal.Base) // Element: text in the status color
		}
		if m.closable {
			if IconButton(c, icon.Close, c.Locale().Close).Size(16, 12).NoHoverBg().Color(tk.CloseColor).Clicked() && !leaving {
				m.close(c.now)
				c.invalid = true
			}
		}
	}, layout.PadVH(tk.PaddingBlock, tk.PaddingInline), layout.Gap(8), layout.Middle(), layout.Margin(margin), layout.Self(align))
	m.row = row.n.id
	row.Semantics(liveRole(m.status), m.title)
	if m.onClick != nil && row.Clicked() {
		m.onClick()
	}
	if m.repeat > 1 {
		messageBadge(c, row, m)
	}
	bg, border, shadow := tk.ContentBg, tk.BorderColor, tk.Shadow
	switch {
	case m.plain:
		bg, border, shadow = tk.PlainBg, tk.PlainBorderColor, tk.PlainShadow
	case el:
		// Element tints the toast with the status palette unless the
		// token sets the colors.
		if bg.A == 0 {
			bg = pal.Bg
		}
		if border.A == 0 {
			border = pal.BgHover
		}
	}
	row.Bg(bg).Radius(tk.Radius).Shadow(shadow)
	if border.A > 0 {
		row.Border(1, border)
	}
	row.Opacity(a)
}

// ---------------------------------------------------------------------------
// Notification: corners, progress bar, pause on hover, stacking.

// NotifyPlacement is the corner a notification appears in. NotifyTopEnd
// (the zero value and default), NotifyTopStart, NotifyBottomEnd and
// NotifyBottomStart are logical: in RTL the end is the left side. The
// Right / Left placements are always that physical side.
//
// Append-only: never reorder or insert; the zero value stays NotifyTopEnd.
type NotifyPlacement uint8

const (
	NotifyTopEnd NotifyPlacement = iota
	NotifyTopStart
	NotifyBottomEnd
	NotifyBottomStart
	NotifyTop
	NotifyBottom
	NotifyTopRight
	NotifyTopLeft
	NotifyBottomRight
	NotifyBottomLeft
)

// resolve maps a logical placement to the physical corner for the given
// direction (NotifyTop / NotifyBottom are centered and stay).
func (p NotifyPlacement) resolve(rtl bool) NotifyPlacement {
	switch p {
	case NotifyTopEnd, NotifyTopStart:
		if (p == NotifyTopEnd) != rtl {
			return NotifyTopRight
		}
		return NotifyTopLeft
	case NotifyBottomEnd, NotifyBottomStart:
		if (p == NotifyBottomEnd) != rtl {
			return NotifyBottomRight
		}
		return NotifyBottomLeft
	}
	return p
}

// notifyCorners are the physical placements, in rendering order.
var notifyCorners = []NotifyPlacement{NotifyTopRight, NotifyTopLeft, NotifyBottomRight, NotifyBottomLeft, NotifyTop, NotifyBottom}

func notifyBottom(pl NotifyPlacement) bool {
	return pl == NotifyBottomRight || pl == NotifyBottomLeft || pl == NotifyBottom
}

func notifyLeft(pl NotifyPlacement) bool { return pl == NotifyTopLeft || pl == NotifyBottomLeft }

// noteConfig describes a notification card.
type noteConfig struct {
	Status       string // success, info, warning, error or "" (no icon)
	Title        string
	Description  string
	Placement    NotifyPlacement
	Duration     time.Duration // 0: 4.5s; <0: until closed
	ShowProgress bool          // countdown bar at the bottom
	NoPause      bool          // keep counting while hovered
	Actions      func()        // buttons under the description
	OnClick      func()
	OnClose      func()
	NoClose      bool       // Closable(false)
	CloseIcon    *icon.Icon // nil: icon.Close
	Icon         *icon.Icon // replaces the status icon
	Role         string     // "alert" | "status" | "" (by status)
}

type navNote struct {
	id       ID
	cfg      noteConfig
	start    time.Time
	paused   time.Duration
	pausedAt time.Time
	closing  time.Time
	loc      *Locale // locale where it was opened (rendered in a global layer)
	env      *env    // scope it was opened in (theme, direction, locale)
	focused  bool    // keyboard focus was inside the card last frame (pauses it)
	closed   bool    // OnClose ran
}

// corner is the physical placement of the card in its scope's direction.
func (n *navNote) corner() NotifyPlacement {
	return n.cfg.Placement.resolve(n.env != nil && n.env.rtl)
}

type navNotes struct {
	list   []*navNote
	seq    int
	groups map[NotifyPlacement]ID
}

var navNotesKey = hashID("nav-notifications")

func (c *Context) navNotes() *navNotes {
	return State[navNotes](c, navNotesKey, func() navNotes { return navNotes{groups: map[NotifyPlacement]ID{}} })
}

// NotifyStackThreshold is the count above which cards collapse into a
// stack until hovered (Ant stack mode). 0 disables stacking.
var NotifyStackThreshold = 3

// NotificationRef configures a notification opened by Notification.
// Use it in the same call chain; it does not need to be kept.
type NotificationRef struct {
	_ [0]func()

	ns  *navNotes
	n   *navNote
	now time.Time
}

// Notification opens a notification card in a window corner (top end by
// default: top right, top left in RTL) and returns a reference for further options:
//
//	ui.Notification(c, "success", "已发布").Description("版本 1.2 已上线").Placement(ui.NotifyBottomRight)
//
// status is success, info, warning, error or "" (no icon). Cards close
// after 4.5s (paused while hovered), collapse into a stack when more than
// NotifyStackThreshold share a corner, and can be updated in place by key.
func Notification(c *Context, status, title string) *NotificationRef {
	ns := c.navNotes()
	ns.seq++
	n := &navNote{id: hashID("note-key", "auto-"+strconv.Itoa(ns.seq)), cfg: noteConfig{Status: status, Title: title}, start: c.now, loc: c.Locale(), env: c.env}
	n.defaults(c) // ConfigProvider.Defaults("Notification", ...): placement, duration, showProgress, pauseOnHover
	ns.list = append(ns.list, n)
	if k, ok := numProp(c.ConfigProp("Notification", "maxCount")); ok && k >= 1 {
		// antd maxCount: the oldest open cards close.
		open := 0
		for i := len(ns.list) - 1; i >= 0; i-- {
			if x := ns.list[i]; x.closing.IsZero() {
				if open++; open > int(k) {
					x.closing = c.now
				}
			}
		}
	}
	c.invalid = true
	return &NotificationRef{ns: ns, n: n, now: c.now}
}

// Key names the card; opening another card with the same key replaces the
// existing one in place (its timer restarts).
func (r *NotificationRef) Key(key string) *NotificationRef {
	id := hashID("note-key", key)
	for _, n := range r.ns.list {
		if n.id == id && n != r.n {
			n.cfg, n.env, n.loc = r.n.cfg, r.n.env, r.n.loc
			n.start, n.paused, n.pausedAt, n.closing = r.now, 0, time.Time{}, time.Time{}
			r.ns.remove(r.n)
			r.n = n
			return r
		}
	}
	r.n.id = id
	return r
}

// Description sets the text under the title.
func (r *NotificationRef) Description(s string) *NotificationRef { r.n.cfg.Description = s; return r }

// Placement selects the corner (NotifyTopEnd default).
func (r *NotificationRef) Placement(p NotifyPlacement) *NotificationRef {
	r.n.cfg.Placement = p
	return r
}

// Duration sets how long the card stays; d <= 0 keeps it until closed.
func (r *NotificationRef) Duration(d time.Duration) *NotificationRef {
	if d <= 0 {
		d = -1
	}
	r.n.cfg.Duration = d
	return r
}

// ShowProgress draws a countdown bar at the bottom.
func (r *NotificationRef) ShowProgress() *NotificationRef { r.n.cfg.ShowProgress = true; return r }

// NoPause keeps counting down while the card is hovered.
func (r *NotificationRef) NoPause() *NotificationRef { r.n.cfg.NoPause = true; return r }

// Actions adds buttons under the description.
func (r *NotificationRef) Actions(fn func()) *NotificationRef { r.n.cfg.Actions = fn; return r }

// OnClick runs fn when the card is clicked.
func (r *NotificationRef) OnClick(fn func()) *NotificationRef { r.n.cfg.OnClick = fn; return r }

// OnClose runs fn once when the card has closed (timed out, closed or
// destroyed), as antd onClose / EP onClose.
func (r *NotificationRef) OnClose(fn func()) *NotificationRef { r.n.cfg.OnClose = fn; return r }

// Closable shows (default) or hides the close button (antd closable, EP
// showClose).
func (r *NotificationRef) Closable(on bool) *NotificationRef { r.n.cfg.NoClose = !on; return r }

// CloseIcon replaces the close icon (antd closeIcon, EP closeIcon).
func (r *NotificationRef) CloseIcon(ic *icon.Icon) *NotificationRef { r.n.cfg.CloseIcon = ic; return r }

// Icon replaces the status icon (antd icon, EP icon).
func (r *NotificationRef) Icon(ic *icon.Icon) *NotificationRef { r.n.cfg.Icon = ic; return r }

// Role sets the live-region role: "alert" (assertive) or "status"
// (polite). By default errors and warnings are alerts (antd role).
func (r *NotificationRef) Role(role string) *NotificationRef { r.n.cfg.Role = role; return r }

func (ns *navNotes) remove(n *navNote) {
	for i, x := range ns.list {
		if x == n {
			ns.list = append(ns.list[:i], ns.list[i+1:]...)
			return
		}
	}
}

// NotificationDestroy closes the card with key ("" closes all).
func NotificationDestroy(c *Context, key string) {
	ns := c.navNotes()
	id := hashID("note-key", key)
	for _, n := range ns.list {
		if (key == "" || n.id == id) && n.closing.IsZero() {
			n.closing = c.now
		}
	}
	c.invalid = true
}

func init() { globalLayers = append(globalLayers, renderNavNotes) }

func (n *navNote) duration() time.Duration {
	d := n.cfg.Duration
	if d == 0 {
		d = 4500 * time.Millisecond
	}
	if d < 0 {
		d = navForever
	}
	return d
}

func (n *navNote) elapsed(now time.Time) time.Duration {
	e := now.Sub(n.start) - n.paused
	if !n.pausedAt.IsZero() {
		e -= now.Sub(n.pausedAt)
	}
	return e
}

func renderNavNotes(c *Context) {
	ns := State[navNotes](c, navNotesKey, func() navNotes { return navNotes{groups: map[NotifyPlacement]ID{}} })
	if len(ns.list) == 0 {
		return
	}
	th := c.Theme
	// Expire and pause.
	keep := ns.list[:0]
	for _, n := range ns.list {
		if !n.closing.IsZero() {
			if c.now.Sub(n.closing) < th.DurationMid && !c.noAnim {
				keep = append(keep, n)
				c.animating = true
			} else if n.cfg.OnClose != nil && !n.closed {
				n.closed = true
				n.cfg.OnClose()
			}
			continue
		}
		// Hover or keyboard focus inside the card pauses the countdown.
		hovered := c.pointerOver(ns.groups[n.corner()]) || n.focused
		if !n.cfg.NoPause {
			if hovered && n.pausedAt.IsZero() {
				n.pausedAt = c.now
			} else if !hovered && !n.pausedAt.IsZero() {
				n.paused += c.now.Sub(n.pausedAt)
				n.pausedAt = time.Time{}
			}
		}
		rem := n.duration() - n.elapsed(c.now)
		if rem <= 0 {
			n.closing = c.now
			keep = append(keep, n)
			continue
		}
		if n.pausedAt.IsZero() {
			c.WakeAt(c.now.Add(rem))
			if n.cfg.ShowProgress && n.cfg.Duration >= 0 && !c.noAnim {
				c.animating = true
			}
		}
		keep = append(keep, n)
	}
	ns.list = keep
	for _, pl := range notifyCorners {
		var group []*navNote
		for _, n := range ns.list {
			if n.corner() == pl {
				group = append(group, n)
			}
		}
		if len(group) == 0 {
			delete(ns.groups, pl)
			continue
		}
		// The group container follows the newest card's scope; every card
		// is built in its own.
		c.withEnv(group[len(group)-1].env, func() { renderNoteGroup(c, ns, pl, group) })
	}
}

// renderNoteGroup builds the cards of the physical corner pl.
func renderNoteGroup(c *Context, ns *navNotes, pl NotifyPlacement, group []*navNote) {
	if c.Theme.Flavor == theme.StyleShadcn {
		renderNotesSonner(c, ns, pl, group)
		return
	}
	el := navEl(c)
	bottom := notifyBottom(pl)
	hovered := c.pointerOver(ns.groups[pl])
	stacked := NotifyStackThreshold > 0 && len(group) > NotifyStackThreshold && !hovered && !el
	// Newest first at the edge: top groups show newest on top, bottom
	// groups newest at the bottom.
	order := make([]*navNote, len(group))
	for i, n := range group {
		if bottom {
			order[i] = n
		} else {
			order[len(group)-1-i] = n
		}
	}
	width := c.ThemeFor(theme.CompNotification).Components.Notification.Width
	rtl := c.IsRTL() // direction of the group container
	var col *node
	holder := Overlay(c, OverlayOptions{Place: ScreenFill}, func() {
		b := Column(c, func() {
			for i, n := range order {
				n := n
				depth := i // distance from the newest card
				if bottom {
					depth = len(order) - 1 - i
				}
				if stacked && depth > 2 {
					continue
				}
				Key(c, n.id, func() {
					c.withEnv(n.env, func() { noteCard(c, n, pl, width, stacked, depth, rtl) })
				})
			}
		})
		b.n.ln.Absolute = true
		b.n.ln.Pos = flexbox.Pos{Left: 0, Top: 0, Right: layout.Unset, Bottom: layout.Unset}
		b.n.ln.Width = width
		b.n.hit, b.n.blocks = true, false
		b.Semantics(a11y.RoleGroup, c.Locale().A11y.Notifications)
		col = b.n
	})
	holder.n.hit, holder.n.blocks = false, false
	ns.groups[pl] = col.id
	margin := c.ThemeFor(theme.CompNotification).Components.Notification.MarginEdge
	holder.n.paint = func(_ *paint.List, r geom.Rect) {
		ln := &col.ln
		switch pl {
		case NotifyTopRight, NotifyBottomRight:
			ln.Rect.X = r.W - ln.Rect.W - margin
		case NotifyTopLeft, NotifyBottomLeft:
			ln.Rect.X = margin
		default:
			ln.Rect.X = (r.W - ln.Rect.W) / 2
		}
		if bottom {
			ln.Rect.Y = r.H - ln.Rect.H - margin + 16
		} else {
			ln.Rect.Y = margin
		}
	}
}

// noteCard builds one Ant / Element card at the physical corner pl;
// rtlParent is the direction of the group column, which decides how its
// logical margins map to the screen.
func noteCard(c *Context, n *navNote, pl NotifyPlacement, width float32, stacked bool, depth int, rtlParent bool) {
	th := c.ThemeFor(theme.CompNotification)
	tk := &th.Components.Notification
	leaving := !n.closing.IsZero()
	a := c.AnimFrom(n.id, "in", 0, b2f(!leaving), th.DurationMid)
	if leaving && c.noAnim {
		a = 0
	}
	cfg := n.cfg
	gap := tk.Gap
	var card *Box
	inner := func() {
		switch {
		case cfg.Icon != nil:
			Icon(c, cfg.Icon).Size(24).Color(th.ColorOf(cfg.Status).Base).n.ln.Margin.Right = tk.IconGap
		case cfg.Status != "":
			statusIcon(c, cfg.Status, 24).ln.Margin.Right = tk.IconGap
		}
		Column(c, func() {
			t := Text(c, cfg.Title).Size(tk.TitleFontSize).Color(tk.TitleColor)
			if tk.TitleLineHeight > 0 {
				t.LineHeight(tk.TitleLineHeight)
			}
			if tk.TitleFontWeight != 0 {
				t.Weight(text.Weight(tk.TitleFontWeight))
			}
			if cfg.Description != "" {
				t.Layout(layout.Margin(geom.Insets{Bottom: tk.TitleMarginBottom}))
				noteDescription(c, tk, cfg.Description)
			}
		}, layout.Grow(1), layout.MinW(0))
		noteClose(c, n, tk, 22, 14, tk.CloseGap, false)
	}
	// Actions are a sibling of the icon / text / close row, floated to the
	// end of the card's content box (rc-notification Notification.js
	// actionsNode; antd .ant-notification-notice-actions float right).
	card = Column(c, func() {
		Row(c, inner, layout.AlignItems(layout.AlignStart))
		if cfg.Actions != nil {
			Row(c, cfg.Actions, layout.Gap(8), layout.End(), layout.Margin(geom.Insets{Top: tk.ActionsMarginTop}))
		}
	}, layout.Padding(geom.Insets{Top: tk.PaddingBlock, Right: tk.Padding, Bottom: tk.PaddingBlock, Left: tk.PaddingInlineStart}))
	card.Bg(tk.Bg).Radius(tk.Radius).Shadow(tk.Shadow)
	if tk.BorderColor.A > 0 {
		card.Border(1, tk.BorderColor)
	}
	noteSem(c, card, n, cfg.Title, cfg.Description, cfg.Status)
	if cfg.OnClick != nil && card.Clicked() {
		cfg.OnClick()
	}
	if cfg.ShowProgress && cfg.Duration >= 0 {
		noteProgress(c, card, n, tk)
	}
	// Entrance: slide in from the card's physical edge; stacking: peek
	// cards behind. Margins are logical in an RTL column, so the sign
	// flips there.
	shift := (1 - a) * (width + 24)
	switch pl {
	case NotifyTop:
		card.n.ln.Margin.Top = -(1 - a) * 40
	case NotifyBottom:
		card.n.ln.Margin.Top = (1 - a) * 40
	default:
		if notifyLeft(pl) != rtlParent {
			shift = -shift
		}
		card.n.ln.Margin.Left = shift
		card.n.ln.Margin.Right = -shift
	}
	bottom := notifyBottom(pl)
	st := c.Anim(n.id, "stack", b2f(stacked && depth > 0), th.DurationMid)
	if st > 0 {
		// Collapsed: keep only an 8px sliver of the card below the newest.
		inset := float32(depth) * 12 * st
		card.n.ln.Margin.Left += inset
		card.n.ln.Margin.Right += inset
		card.n.ln.MaxH = 0
		h := max(8, 200*(1-st))
		card.n.ln.Height = h
		card.Clip()
		if bottom {
			card.n.ln.Margin.Bottom = -h + 8*st
		} else {
			card.n.ln.Margin.Top = -h + 8*st
		}
		card.Opacity(a * (1 - 0.25*float32(depth)))
		for _, k := range card.n.kids {
			k.opacity = 0.0001
		}
	} else {
		card.Opacity(a)
	}
	m := gap * a
	if stacked {
		m = 0 // stacked cards touch
	}
	if bottom {
		card.n.ln.Margin.Top += m
	} else {
		card.n.ln.Margin.Bottom += m
	}
	if leaving {
		card.n.ln.MaxH = 600 * a
		card.Clip()
	}
}

// noteSem marks a notification card as a live region (status, or alert for
// errors and warnings) named by its title and described by its text. A
// card with OnClick is focusable (Enter clicks it); focus inside a card
// pauses its countdown.
func noteSem(c *Context, card *Box, n *navNote, title, desc, status string) {
	role := liveRole(status)
	if n != nil {
		switch n.cfg.Role {
		case "alert":
			role = a11y.RoleAlert
		case "status":
			role = a11y.RoleStatus
		}
	}
	card.Semantics(role, title)
	if desc != "" && c.a11yOn {
		card.A11y(func(s *a11y.Node) { s.Description = desc })
	}
	if n == nil {
		return
	}
	if n.cfg.OnClick != nil {
		ia := card.Focusable(input.CursorPointer)
		if ia.FocusVisible {
			th := c.Theme
			prev := card.n.over
			card.n.over = func(l *paint.List, r geom.Rect) {
				if prev != nil {
					prev(l, r)
				}
				drawFocusRing(l, th, r, geom.Rad(th.RadiusLG))
			}
		}
	}
	n.focused = c.focus != 0 && navHasNode(card.n, c.focus)
}

// ---------------------------------------------------------------------------
// Sonner (shadcn/ui toasts): Message and Notification under the shadcn
// theme are cards in a window corner (bottom right by default). Collapsed,
// up to three cards overlap: the newest in front, older ones peek out
// behind it, each step 14px further and 5% smaller. Hovering the stack
// expands it into a list with 14px gaps.

const (
	sonnerWidth   = 356
	sonnerGap     = 14
	sonnerOffset  = 24
	sonnerVisible = 3
)

type sonnerCard struct {
	id    ID
	env   *env    // scope the toast was opened in
	a     float32 // enter / leave progress
	note  *navNote
	title string // accessible name, description and live role
	desc  string
	stat  string
	body  func()
	click func()
	after func(card *Box) // extra decoration (progress bar)
}

type sonnerState struct {
	col   ID
	front float32 // height of the front card last frame
}

// sonnerStyle is the card look of a Sonner stack: the Message or the
// Notification token.
type sonnerStyle struct {
	bg, border         theme.Color
	radius, pad, width float32
	shadow             []paint.Shadow
}

func sonnerMessageStyle(c *Context) sonnerStyle {
	tk := &c.ThemeFor(theme.CompMessage).Components.Message
	pad := tk.PaddingInline
	return sonnerStyle{bg: tk.ContentBg, border: tk.BorderColor, radius: tk.Radius, pad: pad, width: tk.Width, shadow: tk.Shadow}
}

func sonnerNoteStyle(c *Context) sonnerStyle {
	tk := &c.ThemeFor(theme.CompNotification).Components.Notification
	return sonnerStyle{bg: tk.Bg, border: tk.BorderColor, radius: tk.Radius, pad: tk.Padding, width: tk.Width, shadow: tk.Shadow}
}

// sonnerStack renders cards (oldest first) as one Sonner stack at the
// physical corner pl; each card is built in its own scope and styled by
// style(c) there. Cards enter and stack vertically; the corner (and so the
// side the stack hugs) follows the resolved placement. It returns the
// stack's node ID (for hover checks).
func sonnerStack(c *Context, key ID, pl NotifyPlacement, cards []sonnerCard, style func(c *Context) sonnerStyle) ID {
	st := State[sonnerState](c, key, nil)
	bottom := notifyBottom(pl)
	width := style(c).width
	if width <= 0 {
		width = sonnerWidth
	}
	expanded := c.pointerOver(st.col)
	if len(cards) > sonnerVisible && !expanded {
		cards = cards[len(cards)-sonnerVisible:]
	}
	var cardBody func(c *Context, sc sonnerCard, depth int)
	card := func(sc sonnerCard, depth int) {
		Key(c, sc.id, func() { c.withEnv(sc.env, func() { cardBody(c, sc, depth) }) })
	}
	cardBody = func(c *Context, sc sonnerCard, depth int) {
		sy := style(c)
		b := Row(c, sc.body, layout.Pad(sy.pad), layout.Gap(10), layout.AlignItems(layout.AlignStart))
		b.n.ln.Width = width
		b.Bg(sy.bg).Radius(sy.radius).Shadow(sy.shadow)
		if sy.border.A > 0 {
			b.Border(1, sy.border)
		}
		noteSem(c, b, sc.note, sc.title, sc.desc, sc.stat)
		if sc.click != nil && b.Clicked() {
			sc.click()
		}
		if sc.after != nil {
			sc.after(b)
		}
		enter := (1 - sc.a) * 16
		if !bottom {
			enter = -enter
		}
		if expanded || depth == 0 {
			if depth == 0 && !expanded {
				b.n.ln.AlignSelf = alignEdge(bottom)
			}
			if enter != 0 {
				b.Transform(geom.Translate(0, enter))
			}
			b.Opacity(sc.a)
			return
		}
		// Behind the front card: same height, lifted and shrunk.
		if st.front > 0 {
			b.n.ln.Height = st.front
			b.Clip()
		}
		b.n.ln.AlignSelf = alignEdge(bottom)
		k := 1 - 0.05*float32(depth)
		dy := -float32(depth) * sonnerGap
		if !bottom {
			dy = -dy
		}
		dy += enter
		b.n.xf = func(r geom.Rect) geom.Affine {
			py := r.Bottom()
			if !bottom {
				py = r.Y
			}
			return geom.ScaleAround(k, k, geom.Pt(r.X+r.W/2, py)).Then(geom.Translate(0, dy))
		}
		b.Opacity(sc.a)
		for _, kid := range b.n.kids {
			kid.opacity = 0.0001 // only the card's edge shows
		}
	}
	var col *node
	holder := Overlay(c, OverlayOptions{Place: ScreenFill}, func() {
		var b *Box
		if expanded {
			b = Column(c, func() {
				if bottom {
					for i, sc := range cards {
						card(sc, len(cards)-1-i)
					}
				} else {
					for i := len(cards) - 1; i >= 0; i-- {
						card(cards[i], len(cards)-1-i)
					}
				}
			}, layout.Gap(sonnerGap))
		} else {
			b = StackBox(c, func() {
				for i, sc := range cards { // oldest first so the newest paints on top
					card(sc, len(cards)-1-i)
				}
			})
		}
		b.n.ln.Absolute = true
		b.n.ln.Pos = flexbox.Pos{Left: 0, Top: 0, Right: layout.Unset, Bottom: layout.Unset}
		b.n.ln.Width = width
		b.n.hit, b.n.blocks = true, false
		b.Semantics(a11y.RoleGroup, c.Locale().A11y.Notifications)
		col = b.n
	})
	holder.n.hit, holder.n.blocks = false, false
	st.col = col.id
	if n := len(col.kids); n > 0 && !expanded {
		if r, ok := c.rects[col.kids[n-1].id]; ok {
			st.front = r.H
		}
	}
	holder.n.paint = func(_ *paint.List, r geom.Rect) {
		ln := &col.ln
		switch pl {
		case NotifyTopRight, NotifyBottomRight:
			ln.Rect.X = r.W - ln.Rect.W - sonnerOffset
		case NotifyTopLeft, NotifyBottomLeft:
			ln.Rect.X = sonnerOffset
		default:
			ln.Rect.X = (r.W - ln.Rect.W) / 2
		}
		if bottom {
			ln.Rect.Y = r.H - ln.Rect.H - sonnerOffset
		} else {
			ln.Rect.Y = sonnerOffset
		}
	}
	return col.id
}

func alignEdge(bottom bool) layout.Align {
	if bottom {
		return layout.AlignEnd
	}
	return layout.AlignStart
}

// sonnerIcon draws the leading status icon of a toast (16px).
func sonnerIcon(c *Context, status string) {
	switch status {
	case "":
	case "loading":
		Icon(c, icon.Loading).Spin().Size(16).Color(c.Theme.TextTertiary)
	default:
		statusIcon(c, status, 16).ln.Margin.Top = 2
	}
}

// renderMessagesSonner shows messages as a Sonner stack at the corner of
// placement pl in the current scope (default: bottom right; bottom left in
// RTL).
func renderMessagesSonner(c *Context, pl MessagePlacement, msgs []*toast) {
	th := c.Theme
	key := hashID("sonner-messages")
	if pl != MessageBottomEnd {
		key = hashID("sonner-messages", string(pl))
	}
	// Sonner pauses every toast of a stack while it is hovered.
	over := c.pointerOver(State[sonnerState](c, key, nil).col)
	cards := make([]sonnerCard, 0, len(msgs))
	for _, m := range msgs {
		m := m
		m.hover(c, over)
		leaving := m.leaving(c.now)
		a := c.AnimFrom(m.id, "in", 0, b2f(!leaving), th.DurationMid)
		cards = append(cards, sonnerCard{id: m.id, env: m.env, a: a, title: m.title, stat: m.status, click: m.onClick, body: func() {
			tk := &c.ThemeFor(theme.CompMessage).Components.Message
			if m.ic != nil {
				Icon(c, m.ic).Size(16).Color(c.Theme.Text)
			} else {
				sonnerIcon(c, m.status)
			}
			t := Text(c, m.title).Weight(text.Medium).Layout(layout.Grow(1), layout.MinW(0))
			if tk.ContentFontSize > 0 {
				t.Size(tk.ContentFontSize)
			}
			if m.closable {
				if IconButton(c, icon.Close, c.Locale().Close).Size(20, 14).NoHoverBg().Color(tk.CloseColor).Clicked() && !leaving {
					m.close(c.now)
					c.invalid = true
				}
			}
		}})
	}
	sonnerStack(c, key, pl.sonner(c.IsRTL()), cards, sonnerMessageStyle)
}

func renderNotesSonner(c *Context, ns *navNotes, pl NotifyPlacement, group []*navNote) {
	th := c.Theme
	cards := make([]sonnerCard, 0, len(group))
	for _, n := range group {
		n := n
		leaving := !n.closing.IsZero()
		a := c.AnimFrom(n.id, "in", 0, b2f(!leaving), th.DurationMid)
		if leaving && c.noAnim {
			a = 0
		}
		cfg := n.cfg
		sc := sonnerCard{id: n.id, env: n.env, a: a, click: cfg.OnClick, note: n, title: cfg.Title, desc: cfg.Description, stat: cfg.Status}
		sc.body = func() {
			tk := &c.ThemeFor(theme.CompNotification).Components.Notification
			if cfg.Icon != nil {
				Icon(c, cfg.Icon).Size(16).Color(c.Theme.Text)
			} else {
				sonnerIcon(c, cfg.Status)
			}
			Column(c, func() {
				t := Text(c, cfg.Title).Color(tk.TitleColor)
				if tk.TitleFontSize != c.Theme.FontSize {
					t.Size(tk.TitleFontSize)
				}
				if tk.TitleLineHeight > 0 {
					t.LineHeight(tk.TitleLineHeight)
				}
				if tk.TitleFontWeight != 0 {
					t.Weight(text.Weight(tk.TitleFontWeight))
				}
				if cfg.Description != "" {
					noteDescription(c, tk, cfg.Description)
				}
				if cfg.Actions != nil {
					Row(c, cfg.Actions, layout.Gap(8), layout.Margin(geom.Insets{Top: tk.ActionsMarginTop}))
				}
			}, layout.Grow(1), layout.MinW(0), layout.Gap(tk.TitleMarginBottom))
			noteClose(c, n, tk, 20, 14, 0, true)
		}
		if cfg.ShowProgress && cfg.Duration >= 0 {
			sc.after = func(card *Box) {
				noteProgress(c, card, n, &c.ThemeFor(theme.CompNotification).Components.Notification)
			}
		}
		cards = append(cards, sc)
	}
	ns.groups[pl] = sonnerStack(c, hashID("sonner-notes", strconv.Itoa(int(pl))), pl, cards, sonnerNoteStyle)
}

// messageBadge paints the Grouping repeat counter at the message's top
// end corner (EP message.scss .el-message__badge top -8px right -8px; an
// el-badge of the message type, error as danger: 18px high, padding 0 6px,
// 12px text, 1px border in the background color).
func messageBadge(c *Context, row *Box, m *toast) {
	th := c.Theme
	label := strconv.Itoa(m.repeat)
	st := c.style(th.FontSizeSM, text.Regular)
	lay := text.Shape(label, st, text.Options{NoWrap: true})
	bg := th.ColorOf(m.status).Base
	if m.status == "" || m.status == "loading" {
		bg = th.Info.Base
	}
	fg, ring := th.TextOn(bg, th.TextOnSolid), th.BgContainer
	rtl := c.IsRTL() // the end corner is on the left in RTL
	row.PaintOver(func(l *paint.List, r geom.Rect) {
		w := max(18, lay.Width+12)
		x := r.X + r.W + 8 - w
		if rtl {
			x = r.X - 8
		}
		br := geom.Rect{X: x, Y: r.Y - 8, W: w, H: 18}
		l.Fill(br, geom.Rad(9), bg)
		l.Stroke(br, geom.Rad(9), 1, ring)
		l.Glyphs(geom.Pt(br.X+(br.W-lay.Width)/2, br.Y+(br.H-lay.Height)/2), lay.Glyphs, fg)
	})
}

// noteDescription adds a card's description text.
func noteDescription(c *Context, tk *theme.NotificationToken, s string) {
	d := Text(c, s).Color(tk.DescriptionColor)
	if tk.DescriptionFontSize > 0 {
		d.Size(tk.DescriptionFontSize)
	}
	if tk.DescriptionLineHeight > 0 {
		d.LineHeight(tk.DescriptionLineHeight)
	}
}

// noteClose adds a card's close button (box px, icon px, gap before it)
// unless Closable(false).
func noteClose(c *Context, n *navNote, tk *theme.NotificationToken, box, size, gap float32, noBg bool) {
	if n.cfg.NoClose {
		return
	}
	ic := n.cfg.CloseIcon
	if ic == nil {
		ic = icon.Close
	}
	ib := IconButton(c, ic, n.loc.Close).Size(box, size)
	ib.n.ln.Margin.Left = gap
	if noBg {
		ib.NoHoverBg()
	}
	if !ib.Hovered() {
		ib.Color(tk.CloseColor) // hover: the default icon hover color
	}
	if ib.Clicked() && n.closing.IsZero() {
		n.closing = c.now
		c.invalid = true
	}
}

// noteProgress paints the ShowProgress countdown bar at the bottom of
// card: it shrinks towards the inline start, inset from the rounded
// corners, as a gradient (antd progressBg) or the status color (EP).
func noteProgress(c *Context, card *Box, n *navNote, tk *theme.NotificationToken) {
	th := c.Theme
	left := max(0, min(1, 1-float32(n.elapsed(c.now))/float32(n.duration())))
	from, to := tk.ProgressColorStart, tk.ProgressColor
	if tk.ProgressStatusColor {
		switch n.cfg.Status {
		case "success", "warning", "error":
			from = th.ColorOf(n.cfg.Status).Base
			to = from
		}
	}
	h, inset, rad, rail := max(1, tk.ProgressHeight), tk.ProgressInset, tk.Radius, tk.ProgressRail
	rtl := c.IsRTL()
	card.PaintOver(func(l *paint.List, r geom.Rect) {
		l.Clip(r, geom.Rad(rad))
		track := geom.Rect{X: r.X + inset, Y: r.Bottom() - h, W: max(0, r.W-2*inset), H: h}
		if rail.A > 0 {
			l.Fill(track, geom.Rad(h/2), rail)
		}
		bar := track
		bar.W = track.W * left
		if rtl {
			bar.X = track.Right() - bar.W
		}
		if from == to {
			l.Fill(bar, geom.Rad(h/2), to)
		} else {
			// The gradient spans the whole track, as a CSS progress value.
			p0, p1 := geom.Pt(track.X, 0), geom.Pt(track.Right(), 0)
			if rtl {
				p0, p1 = p1, p0
			}
			l.LinearStops(bar, geom.Rad(h/2), p0, p1, paint.Stop{Offset: 0, Color: from}, paint.Stop{Offset: 1, Color: to})
		}
		l.Pop()
	})
}
