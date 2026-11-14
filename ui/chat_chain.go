package ui

import (
	"strconv"
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
// ThoughtChain and ToolCall (Ant Design X ThoughtChain)

// ChainStatus is the state of a step or tool call.
type ChainStatus uint8

const (
	ChainPending ChainStatus = iota // not started
	ChainRunning                    // in progress (spinner, live duration)
	ChainSuccess
	ChainError
)

func (s ChainStatus) label(l *Locale) string {
	switch s {
	case ChainRunning:
		return l.Chat.Running
	case ChainSuccess:
		return l.Chat.Done
	case ChainError:
		return l.Chat.Error
	}
	return l.Chat.Pending
}

// ChainItem is one step of a ThoughtChain.
type ChainItem struct {
	_ [0]func()

	Key         string // stable key (default: the index)
	Title       string
	Description string        // secondary line under the title
	Status      ChainStatus   //
	Duration    time.Duration // shown at the end (0: hidden)
	Started     time.Time     // a running step shows the time elapsed since then
	Detail      string        // collapsible Markdown detail
	Content     func()        // collapsible custom detail (instead of Detail)
}

type chainState struct {
	init      bool
	collapsed bool
	open      map[string]bool
}

// ThoughtChainW is a vertical chain of steps with status icons, durations
// and collapsible details, optionally under a collapsible title.
type ThoughtChainW struct {
	_ [0]func()

	c         *Context
	b         *Box
	st        *chainState
	z         *lazy
	loc       *Locale
	items     []ChainItem
	title     string
	card      bool
	collapsed bool
	toggled   bool
	itemKey   string
}

// ThoughtChain adds a chain of steps.
//
//	ui.ThoughtChain(c, steps).Title("思考过程").Card()
func ThoughtChain(c *Context, items []ChainItem) *ThoughtChainW {
	w := &ThoughtChainW{c: c, items: items, loc: c.Locale()}
	w.b = c.box("thought-chain", layout.Column, nil)
	w.st = State[chainState](c, w.b.n.id, nil)
	if w.st.open == nil {
		w.st.open = map[string]bool{}
	}
	w.z = c.lazy(w.b.n, w.build)
	return w
}

// Title adds a header that collapses the chain and shows its progress.
func (w *ThoughtChainW) Title(s string) *ThoughtChainW { w.title = s; return w }

// Card draws the chain inside a bordered card.
func (w *ThoughtChainW) Card() *ThoughtChainW { w.card = true; return w }

// DefaultCollapsed starts with the steps hidden (needs Title).
func (w *ThoughtChainW) DefaultCollapsed() *ThoughtChainW { w.collapsed = true; return w }

// Layout applies layout options to the chain.
func (w *ThoughtChainW) Layout(opts ...layout.Option) *ThoughtChainW {
	w.b.Style(opts...)
	return w
}

// Toggled reports a click on the title this frame.
func (w *ThoughtChainW) Toggled() bool { w.z.run(); return w.toggled }

// Expanded reports whether the steps are shown.
func (w *ThoughtChainW) Expanded() bool { w.z.run(); return w.title == "" || !w.st.collapsed }

// ItemToggled returns the key of the step whose detail was opened or
// closed this frame.
func (w *ThoughtChainW) ItemToggled() (string, bool) { w.z.run(); return w.itemKey, w.itemKey != "" }

func (w *ThoughtChainW) key(i int) string {
	if k := w.items[i].Key; k != "" {
		return k
	}
	return strconv.Itoa(i)
}

func (w *ThoughtChainW) build() {
	c, st := w.c, w.st
	th := c.ThemeFor(theme.CompChat)
	tk := &th.Components.Chat
	if !st.init {
		st.init, st.collapsed = true, w.collapsed
	}
	if w.card {
		w.b.Bg(tk.CardBg).Radius(tk.CardRadius).Border(1, tk.CardBorder)
		w.b.n.ln.Padding = geom.VH(4, 12)
	}
	if w.title != "" {
		done, status := 0, ChainSuccess
		for _, it := range w.items {
			switch it.Status {
			case ChainSuccess:
				done++
			case ChainRunning:
				status = ChainRunning
			case ChainError:
				if status != ChainRunning {
					status = ChainError
				}
			case ChainPending:
				if status == ChainSuccess {
					status = ChainPending
				}
			}
		}
		progress := w.loc.chatSteps(done, len(w.items))
		if chatHeader(c, th, tk, chatHeaderSpec{
			title: w.title, extra: progress, open: !st.collapsed, status: status, showStatus: len(w.items) > 0,
			name: w.title + " " + progress + " " + status.label(w.loc),
		}) {
			st.collapsed = !st.collapsed
			w.toggled = true
		}
		if st.collapsed {
			return
		}
	}
	Column(c, func() {
		for i := range w.items {
			Key(c, w.key(i), func() { w.step(th, tk, i) })
		}
	}).n.setRole(a11y.RoleList)
}

// step builds one chain item: status icon on a rail, title row, detail.
func (w *ThoughtChainW) step(th *theme.Theme, tk *theme.ChatToken, i int) {
	c, st := w.c, w.st
	it := w.items[i]
	k := w.key(i)
	last := i == len(w.items)-1
	expandable := it.Detail != "" || it.Content != nil
	open := expandable && st.open[k]
	const iconW = 16
	row := Row(c, nil, layout.Gap(10))
	row.n.setRole(a11y.RoleListItem)
	lh := th.LineHeightFor(th.FontSize)
	row.n.paint = func(l *paint.List, r geom.Rect) {
		if last {
			return
		}
		s := c.inline(r)
		off := 6 + (lh-iconW)/2 // icon top within a row
		top := r.Y + off + iconW + 3
		bot := r.Bottom() + off - 3 // the next row's icon
		l.Fill(s.Rect(iconW/2-0.5, top, 1, bot-top), geom.Radii{}, tk.LineColor)
	}
	c.body(row.n, func() {
		ic := c.add("chain-icon", "")
		ic.aHidden = true
		ic.ln.Width, ic.ln.Height = iconW, iconW
		ic.ln.Margin = geom.Insets{Top: 6 + (lh-iconW)/2}
		ic.paint = func(l *paint.List, r geom.Rect) { chatStatusIcon(c, l, r, th, tk, it.Status) }
		Column(c, func() {
			head := Row(c, func() {
				Text(c, it.Title).Color(tk.StepTitleColor).Layout(layout.Grow(1), layout.Basis(0), layout.MinW(0)).n.aHidden = true
				if d := chatElapsed(c, it.Status, it.Duration, it.Started); d != "" {
					Text(c, d).Size(th.FontSizeSM).Color(tk.DurationColor).NoWrap().n.aHidden = true
				}
				if expandable {
					chev := c.add("chain-chevron", "")
					chev.aHidden = true
					chev.ln.Width, chev.ln.Height = 14, 14
					rot := c.Anim(chev.id, "rot", b2f(open), th.DurationMid)
					chev.paint = func(l *paint.List, r geom.Rect) {
						c.paintIcon(l, icon.Right, r, th.TextTertiary, float64(rot)*1.5707963)
					}
				}
			}, layout.Gap(8), layout.Middle(), layout.PadVH(6, 0))
			name := it.Title + ", " + it.Status.label(w.loc)
			head.n.setName(name)
			if expandable {
				head.Radius(th.RadiusSM)
				head.n.setRole(a11y.RoleButton).setState(a11y.Expandable).stateIf(open, a11y.Expanded)
				ia := c.interact(head.n, true, input.CursorPointer)
				if ia.Hovered {
					head.Bg(tk.CardHeaderHover)
				}
				head.n.over = func(l *paint.List, r geom.Rect) {
					if ia.FocusVisible {
						drawFocusRing(l, th, r, geom.Rad(th.RadiusSM))
					}
				}
				if ia.Clicked {
					st.open[k] = !open
					w.itemKey = k
					open = !open
					c.invalid = true
				}
			} else {
				head.n.setRole(a11y.RoleText)
			}
			if it.Description != "" {
				Text(c, it.Description).Size(th.FontSizeSM).Color(tk.StepDescColor)
			}
			if open {
				Column(c, func() {
					if it.Content != nil {
						it.Content()
						return
					}
					Markdown(c, it.Detail).Size(th.FontSizeSM).Color(tk.StepDescColor)
				}, layout.Padding(geom.Insets{Top: 4, Bottom: 6}))
			}
		}, layout.Grow(1), layout.Basis(0), layout.MinW(0), layout.Padding(geom.Insets{Bottom: 4}))
	})
}

// chatStatusIcon draws a step status: hollow circle, spinner, check or
// cross.
func chatStatusIcon(c *Context, l *paint.List, r geom.Rect, th *theme.Theme, tk *theme.ChatToken, s ChainStatus) {
	switch s {
	case ChainRunning:
		c.paintIcon(l, icon.Loading, r, th.Primary.Base, spinAngle(c))
	case ChainSuccess:
		drawStatusIcon(l, "success", r, th.Success.Base, th.BgContainer)
	case ChainError:
		drawStatusIcon(l, "error", r, th.Error.Base, th.BgContainer)
	default:
		l.StrokePath(paint.NewPath().Circle(r.X+r.W/2, r.Y+r.H/2, r.W/2-1.5), paint.StrokeStyle{Width: 1.5}, tk.PendingColor)
	}
}

// chatElapsed formats the duration of a step: the fixed duration, or the
// live time of a running step with a start time.
func chatElapsed(c *Context, s ChainStatus, d time.Duration, started time.Time) string {
	if s == ChainRunning && !started.IsZero() {
		d = max(0, c.now.Sub(started))
		c.WakeAt(c.now.Add(100*time.Millisecond - d%(100*time.Millisecond)))
	}
	if d <= 0 {
		return ""
	}
	return c.Locale().chatDuration(d)
}

type chatHeaderSpec struct {
	icon       *icon.Icon
	title      string
	extra      string // muted text after the title (progress, summary)
	duration   string
	open       bool
	status     ChainStatus
	showStatus bool
	name       string // accessible name
}

// chatHeader is the clickable header of a ThoughtChain or ToolCall; it
// reports a toggle.
func chatHeader(c *Context, th *theme.Theme, tk *theme.ChatToken, h chatHeaderSpec) bool {
	b := Row(c, nil, layout.Gap(8), layout.Middle(), layout.PadVH(6, 4))
	b.Radius(th.RadiusSM)
	b.n.setRole(a11y.RoleButton).setName(h.name).setState(a11y.Expandable).stateIf(h.open, a11y.Expanded)
	ia := c.interact(b.n, true, input.CursorPointer)
	if ia.Hovered {
		b.Bg(tk.CardHeaderHover)
	}
	b.n.over = func(l *paint.List, r geom.Rect) {
		if ia.FocusVisible {
			drawFocusRing(l, th, r, geom.Rad(th.RadiusSM))
		}
	}
	c.body(b.n, func() {
		chev := c.add("chat-chevron", "")
		chev.aHidden = true
		chev.ln.Width, chev.ln.Height = 14, 14
		rot := c.Anim(b.n.id, "rot", b2f(h.open), th.DurationMid)
		chev.paint = func(l *paint.List, r geom.Rect) {
			c.paintIcon(l, icon.Right, r, th.TextTertiary, float64(rot)*1.5707963)
		}
		if h.icon != nil {
			ic := c.add("chat-head-icon", "")
			ic.aHidden = true
			ic.ln.Width, ic.ln.Height = 16, 16
			hi := h.icon
			ic.paint = func(l *paint.List, r geom.Rect) { c.paintIcon(l, hi, r, th.TextSecondary, 0) }
		}
		t := Text(c, h.title).Strong().NoWrap()
		t.n.aHidden = true
		if h.extra != "" {
			x := Text(c, h.extra).Color(th.TextTertiary).Ellipsis().Layout(layout.Shrink(1), layout.MinW(0))
			x.n.aHidden = true
		}
		Spacer(c)
		if h.duration != "" {
			Text(c, h.duration).Size(th.FontSizeSM).Color(tk.DurationColor).NoWrap().n.aHidden = true
		}
		if h.showStatus {
			si := c.add("chat-head-status", "")
			si.aHidden = true
			si.ln.Width, si.ln.Height = 14, 14
			s := h.status
			si.paint = func(l *paint.List, r geom.Rect) { chatStatusIcon(c, l, r, th, tk, s) }
		}
	})
	return ia.Clicked
}

// ---------------------------------------------------------------------------
// ToolCall

type toolState struct {
	init bool
	open bool
}

// ToolCallW is a collapsible card for one tool invocation: name, summary,
// status and duration in the header; arguments and result in the body.
type ToolCallW struct {
	_ [0]func()

	c        *Context
	b        *Box
	st       *toolState
	z        *lazy
	loc      *Locale
	name     string
	summary  string
	status   ChainStatus
	dur      time.Duration
	started  time.Time
	args     string
	argsLang string
	result   string
	resLang  string
	ic       *icon.Icon
	defOpen  bool
	body     func()
	toggled  bool
}

// ToolCall adds a tool call card for tool name.
//
//	ui.ToolCall(c, "read_file").Summary("src/main.go").Status(ui.ChainSuccess).
//		Duration(320*time.Millisecond).Args(`{"path":"src/main.go"}`).Result(out)
func ToolCall(c *Context, name string) *ToolCallW {
	w := &ToolCallW{c: c, name: name, loc: c.Locale(), argsLang: "json", ic: icon.Tool}
	w.b = c.box("tool-call", layout.Column, nil)
	w.b.n.setRole(a11y.RoleGroup).setName(w.loc.Chat.ToolCall + " " + name)
	w.st = State[toolState](c, w.b.n.id, nil)
	w.z = c.lazy(w.b.n, w.build)
	return w
}

// Summary shows a short description after the name (a path, a query).
func (w *ToolCallW) Summary(s string) *ToolCallW { w.summary = s; return w }

// Status sets the call's state (default ChainPending).
func (w *ToolCallW) Status(s ChainStatus) *ToolCallW { w.status = s; return w }

// Duration shows how long the call took.
func (w *ToolCallW) Duration(d time.Duration) *ToolCallW { w.dur = d; return w }

// Started shows the live elapsed time while the call is running.
func (w *ToolCallW) Started(t time.Time) *ToolCallW { w.started = t; return w }

// Args sets the arguments shown in the body (JSON by default, see Lang).
func (w *ToolCallW) Args(s string) *ToolCallW { w.args = s; return w }

// Result sets the output shown in the body.
func (w *ToolCallW) Result(s string) *ToolCallW { w.result = s; return w }

// Lang sets the code languages of the arguments and the result (default
// "json" and none).
func (w *ToolCallW) Lang(args, result string) *ToolCallW {
	w.argsLang, w.resLang = args, result
	return w
}

// Icon replaces the tool icon.
func (w *ToolCallW) Icon(ic *icon.Icon) *ToolCallW { w.ic = ic; return w }

// DefaultOpen starts expanded.
func (w *ToolCallW) DefaultOpen() *ToolCallW { w.defOpen = true; return w }

// Body adds custom content at the end of the expanded body (a diff, an
// image, an Approval).
func (w *ToolCallW) Body(fn func()) *ToolCallW { w.body = fn; return w }

// Layout applies layout options to the card.
func (w *ToolCallW) Layout(opts ...layout.Option) *ToolCallW {
	w.b.Style(opts...)
	return w
}

// Toggled reports a click on the header this frame.
func (w *ToolCallW) Toggled() bool { w.z.run(); return w.toggled }

// Open reports whether the body is shown.
func (w *ToolCallW) Open() bool { w.z.run(); return w.st.open }

func (w *ToolCallW) build() {
	c, st := w.c, w.st
	th := c.ThemeFor(theme.CompChat)
	tk := &th.Components.Chat
	if !st.init {
		st.init, st.open = true, w.defOpen
	}
	border := tk.CardBorder
	if w.status == ChainError {
		border = th.Error.Border
	}
	w.b.Bg(tk.CardBg).Radius(tk.CardRadius).Border(1, border)
	w.b.n.ln.Padding = geom.All(4)
	w.b.Clip()
	if chatHeader(c, th, tk, chatHeaderSpec{
		icon: w.ic, title: w.name, extra: w.summary, open: st.open, status: w.status, showStatus: true,
		duration: chatElapsed(c, w.status, w.dur, w.started),
		name:     w.name + " " + w.summary + " " + w.status.label(w.loc),
	}) {
		st.open = !st.open
		w.toggled = true
	}
	if !st.open {
		return
	}
	Column(c, func() {
		if w.args != "" {
			chatSection(c, th, w.loc.Chat.Arguments, w.args, w.argsLang)
		}
		if w.result != "" {
			chatSection(c, th, w.loc.Chat.Result, w.result, w.resLang)
		}
		if w.body != nil {
			w.body()
		}
	}, layout.Gap(8), layout.Padding(geom.Insets{Left: 8, Right: 8, Top: 2, Bottom: 8}))
}

// chatSection is a labelled code block (scrolls past 240 px).
func chatSection(c *Context, th *theme.Theme, label, body, lang string) {
	Column(c, func() {
		Text(c, label).Size(th.FontSizeSM).Color(th.TextTertiary)
		Scroll(c, func() {
			Markdown(c, chatFence(body, lang)).Size(th.FontSizeSM)
		}, layout.MaxH(240))
	}, layout.Gap(4))
}

// chatFence wraps s in a Markdown code fence longer than any backtick run
// inside it.
func chatFence(s, lang string) string {
	run, longest := 0, 0
	for _, r := range s {
		if r == '`' {
			run++
			longest = max(longest, run)
		} else {
			run = 0
		}
	}
	f := strings.Repeat("`", max(3, longest+1))
	return f + lang + "\n" + strings.TrimRight(s, "\n") + "\n" + f
}
