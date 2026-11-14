package ui

import (
	"strings"
	"time"
	"unicode/utf8"

	"github.com/tangoui/tango/ui/a11y"
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
// Bubble (Ant Design X Bubble)

// ChatRole is the author of a chat message.
type ChatRole uint8

const (
	ChatAssistant ChatRole = iota // start-aligned, Markdown content
	ChatUser                      // end-aligned filled bubble, plain text
	ChatSystem                    // centered notice without avatar
)

// BubbleVariant is the look of a Bubble.
type BubbleVariant uint8

const (
	BubbleAuto       BubbleVariant = iota // user filled, assistant borderless
	BubbleFilled                          // tinted background
	BubbleOutlined                        // border, no fill
	BubbleShadow                          // elevated card
	BubbleBorderless                      // content only (full width)
)

// BubbleAction is a set of built-in message actions (bit flags).
type BubbleAction uint8

const (
	BubbleCopy  BubbleAction = 1 << iota // copy the content to the clipboard
	BubbleRetry                          // ask for a new answer (Retried)
	BubbleEdit                           // edit the message in place (Edited)
)

// bubbleCaret is appended to the revealed text of a streaming bubble.
const bubbleCaret = "\u258d"

// copiedFor is how long the copy button shows its check mark.
const copiedFor = 1500 * time.Millisecond

type bubbleState struct {
	shown    int // runes revealed by the typing effect
	acc      float64
	last     time.Time
	started  bool
	copiedAt time.Time
	editing  bool
	draft    string
	bodyID   ID
}

// BubbleW is one chat message: avatar, optional name, content and actions.
type BubbleW struct {
	_ [0]func()

	c          *Context
	b          *Box
	st         *bubbleState
	z          *lazy
	loc        *Locale
	role       ChatRole
	content    string
	name, meta string
	variant    BubbleVariant
	markdown   int8 // 0 auto (assistant), 1 on, -1 off
	typing     bool
	loading    bool
	cps        float32
	actions    BubbleAction
	always     bool
	avatar     func()
	avatarIc   *icon.Icon
	avatarText string
	noAvatar   bool
	extra      func()

	copied, retried, edited bool
	editText                string
	link                    string
}

// Bubble adds a chat message of role with content (Markdown for the
// assistant, plain text for the user; see Markdown/Plain).
//
//	ui.Bubble(c, ui.ChatAssistant, reply).Typing(streaming).Actions(ui.BubbleCopy | ui.BubbleRetry)
func Bubble(c *Context, role ChatRole, content string) *BubbleW {
	w := &BubbleW{c: c, role: role, content: content, cps: 60, loc: c.Locale()}
	w.b = c.box("bubble", layout.Row, nil)
	w.b.n.ln.Align = layout.AlignStart
	w.st = State[bubbleState](c, w.b.n.id, nil)
	w.b.n.setRole(a11y.RoleGroup).fill(w)
	w.z = c.lazy(w.b.n, w.build)
	return w
}

func (w *BubbleW) semantics(s *a11y.Node) {
	if s.Name == "" {
		s.Name = w.roleName()
	}
	switch {
	case w.loading && w.content == "":
		s.Description = w.loc.Chat.Thinking
		s.States |= a11y.Busy
	case w.typing:
		s.Description = w.loc.Chat.Generating
		s.States |= a11y.Busy
	}
}

func (w *BubbleW) roleName() string {
	if w.name != "" {
		return w.name
	}
	switch w.role {
	case ChatUser:
		return w.loc.Chat.User
	case ChatSystem:
		return w.loc.Chat.System
	}
	return w.loc.Chat.Assistant
}

// Name shows the author's name above the bubble (also its accessible name).
func (w *BubbleW) Name(s string) *BubbleW { w.name = s; return w }

// Meta shows a footer text after the actions (time, model, tokens).
func (w *BubbleW) Meta(s string) *BubbleW { w.meta = s; return w }

// Avatar replaces the default avatar with custom content.
func (w *BubbleW) Avatar(fn func()) *BubbleW { w.avatar = fn; return w }

// AvatarIcon sets the icon of the default avatar.
func (w *BubbleW) AvatarIcon(ic *icon.Icon) *BubbleW { w.avatarIc = ic; return w }

// AvatarText shows initials in the default avatar.
func (w *BubbleW) AvatarText(s string) *BubbleW { w.avatarText = s; return w }

// NoAvatar hides the avatar column.
func (w *BubbleW) NoAvatar() *BubbleW { w.noAvatar = true; return w }

// Variant sets the look (default BubbleAuto).
func (w *BubbleW) Variant(v BubbleVariant) *BubbleW { w.variant = v; return w }

// Markdown renders the content as Markdown (default for the assistant).
func (w *BubbleW) Markdown() *BubbleW { w.markdown = 1; return w }

// Plain renders the content as plain text keeping line breaks (default for
// the user and system).
func (w *BubbleW) Plain() *BubbleW { w.markdown = -1; return w }

// Typing marks the content as still streaming: it is revealed at
// TypingSpeed (catching up quickly when far behind) with a caret at the
// end. Turning it off shows the whole content.
func (w *BubbleW) Typing(on bool) *BubbleW { w.typing = on; return w }

// TypingSpeed sets the reveal speed in characters per second (default 60).
func (w *BubbleW) TypingSpeed(cps float32) *BubbleW { w.cps = max(1, cps); return w }

// Loading shows animated dots while the content is still empty.
func (w *BubbleW) Loading(on bool) *BubbleW { w.loading = on; return w }

// Actions shows the built-in action buttons under the bubble. They appear
// while the pointer is over the message or focus is inside it (see
// ActionsAlways) and stay reachable with Tab.
func (w *BubbleW) Actions(a BubbleAction) *BubbleW { w.actions = a; return w }

// ActionsAlways keeps the action buttons visible (the last answer).
func (w *BubbleW) ActionsAlways() *BubbleW { w.always = true; return w }

// Extra adds content under the bubble (tool cards, thought chains,
// attachments, custom actions).
func (w *BubbleW) Extra(fn func()) *BubbleW { w.extra = fn; return w }

// Layout applies layout options to the message row.
func (w *BubbleW) Layout(opts ...layout.Option) *BubbleW {
	flexbox.Apply(&w.b.n.ln.Style, opts...)
	return w
}

// Copied reports that the copy action put the content on the clipboard
// this frame.
func (w *BubbleW) Copied() bool { w.z.run(); return w.copied }

// Retried reports a click on the retry action.
func (w *BubbleW) Retried() bool { w.z.run(); return w.retried }

// Edited returns the new text when an in-place edit was saved this frame.
func (w *BubbleW) Edited() (string, bool) { w.z.run(); return w.editText, w.edited }

// Editing reports whether the in-place editor is open.
func (w *BubbleW) Editing() bool { w.z.run(); return w.st.editing }

// ClickedLink returns the URL of a Markdown link clicked this frame.
func (w *BubbleW) ClickedLink() string { w.z.run(); return w.link }

// ID returns the message row's ID.
func (w *BubbleW) ID() ID { return w.b.n.id }

func (w *BubbleW) useMarkdown() bool {
	if w.markdown != 0 {
		return w.markdown > 0
	}
	return w.role == ChatAssistant
}

func (w *BubbleW) look() BubbleVariant {
	if w.variant != BubbleAuto {
		return w.variant
	}
	if w.role == ChatUser {
		return BubbleFilled
	}
	return BubbleBorderless
}

// revealed advances the typing effect and returns the visible content.
func (w *BubbleW) revealed() string {
	c, st := w.c, w.st
	total := utf8.RuneCountInString(w.content)
	if !w.typing || c.still() {
		st.shown, st.started = total, w.typing
		return w.content
	}
	now := c.now
	if !st.started {
		st.started, st.last, st.acc = true, now, 0
	}
	if st.shown > total { // content replaced
		st.shown = total
	}
	dt := now.Sub(st.last).Seconds()
	st.last = now
	if back := total - st.shown; back > 0 {
		rate := float64(w.cps)
		if float64(back) > 2*rate {
			rate = float64(back) // far behind: catch up within about a second
		}
		st.acc += rate * min(dt, 0.25)
		if n := int(st.acc); n > 0 {
			st.acc -= float64(n)
			st.shown = min(total, st.shown+n)
		}
		if st.shown < total {
			c.animating = true
		}
	}
	if st.shown >= total {
		return w.content
	}
	return string([]rune(w.content)[:st.shown])
}

func (w *BubbleW) build() {
	c := w.c
	th := c.ThemeFor(theme.CompChat)
	tk := &th.Components.Chat
	if w.role == ChatSystem {
		w.buildSystem(th, tk)
		return
	}
	user := w.role == ChatUser
	w.b.n.ln.Gap = tk.BubbleGap
	if user {
		w.b.n.ln.Justify = layout.JustifyEnd
	}
	avatar := func() {
		if w.noAvatar {
			return
		}
		Column(c, func() {
			switch {
			case w.avatar != nil:
				w.avatar()
			case user:
				a := Avatar(c, w.avatarText).Size(tk.AvatarSize)
				if w.avatarIc != nil {
					a.Icon(w.avatarIc)
				}
				a.n.aHidden = true // the message group carries the name
			default:
				chatAssistantAvatar(c, th, tk, w.avatarIc, w.avatarText)
			}
		}, layout.NoStretch())
	}
	if !user {
		avatar()
	}
	body := Column(c, func() { w.buildBody(th, tk) }, layout.Grow(1), layout.Basis(0), layout.MinW(0))
	body.n.ln.Gap = 4
	if user {
		body.n.ln.Align = layout.AlignEnd
	}
	w.st.bodyID = body.n.id
	if user {
		avatar()
	}
}

func (w *BubbleW) buildSystem(th *theme.Theme, tk *theme.ChatToken) {
	c := w.c
	w.b.n.ln.Justify = layout.JustifyCenter
	pill := Row(c, func() {
		RichText(c, Span{Text: w.content}).Size(th.FontSizeSM).Color(tk.SystemColor).Align(AlignCenter)
	}, layout.PadVH(2, 10))
	pill.Bg(tk.SystemBg).Radius(th.RadiusLG + 8)
	pill.n.ln.Shrink = 1
	pill.n.ln.MinW = 0
}

func (w *BubbleW) buildBody(th *theme.Theme, tk *theme.ChatToken) {
	c, st := w.c, w.st
	user := w.role == ChatUser
	if w.name != "" {
		Text(c, w.name).Size(th.FontSizeSM).Color(tk.NameColor).Ellipsis().n.aHidden = true
	}
	if st.editing {
		w.buildEditor(th, tk)
	} else {
		w.buildContent(th, tk)
	}
	if w.extra != nil {
		Column(c, w.extra, layout.Gap(8), layout.Self(layout.AlignStretch))
	}
	if (w.actions != 0 || w.meta != "") && !st.editing {
		w.buildActions(th, tk, user)
	}
}

func (w *BubbleW) buildContent(th *theme.Theme, tk *theme.ChatToken) {
	c := w.c
	look := w.look()
	fg := th.Text
	box := Column(c, nil)
	n := box.n
	switch look {
	case BubbleFilled:
		bg := tk.AssistantBg
		fg = tk.AssistantColor
		if w.role == ChatUser {
			bg, fg = tk.UserBg, tk.UserColor
		}
		box.Bg(bg)
	case BubbleOutlined:
		box.Border(1, tk.OutlinedBorder)
	case BubbleShadow:
		box.Bg(th.BgElevated).Shadow(tk.ShadowBubble)
		if th.PopupBorder.A > 0 { // shadows vanish on dark pages
			box.Border(1, th.PopupBorder)
		}
	}
	if look != BubbleBorderless {
		n.ln.Padding = geom.VH(tk.BubblePaddingBlock, tk.BubblePaddingInl)
		box.Radius(tk.BubbleRadius)
		n.ln.NoStretch = true
		if r, ok := c.rects[w.st.bodyID]; ok && r.W > 0 {
			n.ln.MaxW = max(120, r.W*tk.BubbleMaxWidth)
		}
	} else {
		n.ln.AlignSelf = layout.AlignStretch
	}
	n.ln.MinW = 0
	n.ln.Shrink = 1
	c.body(n, func() {
		if w.loading && w.content == "" {
			chatDots(c, tk.DotColor, w.loc.Chat.Thinking)
			return
		}
		shown := w.revealed()
		if w.typing {
			shown += bubbleCaret
		}
		if w.useMarkdown() {
			md := Markdown(c, shown)
			if fg != th.Text {
				md.Color(fg)
			}
			if u := md.ClickedLink(); u != "" {
				w.link = u
			}
			return
		}
		RichText(c, Span{Text: shown}).Pre().Selectable().Color(fg)
	})
}

func (w *BubbleW) buildEditor(th *theme.Theme, tk *theme.ChatToken) {
	c, st := w.c, w.st
	loc := w.loc
	Column(c, func() {
		ta := TextArea(c, &st.draft).AutoSize(1, 10).A11yLabel(loc.Chat.EditPlaceholder).Placeholder(loc.Chat.EditPlaceholder)
		id := ta.ID()
		save := false
		for range msTakeKeys(c, id, func(k input.KeyEvent) bool {
			return k.Key == input.KeyEnter && k.Mods.Shortcut()
		}) {
			save = true
		}
		cancel := c.focus == id && c.Escape()
		Row(c, func() {
			if Button(c, loc.Cancel).Small().Clicked() {
				cancel = true
			}
			if Button(c, loc.Save).Small().Primary().Disabled(strings.TrimSpace(st.draft) == "").Clicked() {
				save = true
			}
		}, layout.Gap(8), layout.End())
		switch {
		case save && strings.TrimSpace(st.draft) != "":
			w.edited, w.editText = true, st.draft
			st.editing = false
			c.invalid = true
		case cancel:
			st.editing = false
			c.invalid = true
		}
	}, layout.Gap(8), layout.Self(layout.AlignStretch))
}

func (w *BubbleW) buildActions(th *theme.Theme, tk *theme.ChatToken, user bool) {
	c, st := w.c, w.st
	loc := w.loc
	row := Row(c, nil, layout.Gap(2), layout.Middle(), layout.NoStretch())
	show := w.always || c.pointerOver(w.b.n.id) || c.focusWithin(row.n)
	row.Opacity(c.Anim(row.n.id, "show", b2f(show), th.DurationMid))
	c.body(row.n, func() {
		act := func(ic *icon.Icon, label string) bool {
			var clicked bool
			Tooltip(c, label, func() {
				clicked = IconButton(c, ic, label).Size(26, 15).Color(tk.ActionColor).Clicked()
			})
			return clicked
		}
		if w.actions&BubbleCopy != 0 {
			copied := !st.copiedAt.IsZero() && c.now.Sub(st.copiedAt) < copiedFor
			ic, label := icon.Copy, loc.Copy
			if copied {
				ic, label = icon.Check, loc.Copied
				c.WakeAt(st.copiedAt.Add(copiedFor))
			}
			if act(ic, label) {
				if c.win != nil {
					c.win.WriteClipboard(w.content)
				}
				st.copiedAt = c.now
				w.copied = true
				c.Announce(loc.Copied, a11y.LivePolite)
				c.invalid = true
			}
		}
		if w.actions&BubbleRetry != 0 && act(icon.Reload, loc.Retry) {
			w.retried = true
		}
		if w.actions&BubbleEdit != 0 && act(icon.Edit, loc.Edit) {
			st.editing, st.draft = true, w.content
			c.invalid = true
		}
		if w.meta != "" {
			t := Text(c, w.meta).Size(th.FontSizeSM).Color(tk.MetaColor).NoWrap()
			t.n.ln.Margin.Left = 6
		}
	})
	if user {
		row.n.ln.AlignSelf = layout.AlignEnd
	}
}

// chatAssistantAvatar draws the default assistant avatar: an icon (or
// initials) on a tinted circle.
func chatAssistantAvatar(c *Context, th *theme.Theme, tk *theme.ChatToken, ic *icon.Icon, label string) {
	size := tk.AvatarSize
	n := c.add("chat-avatar", label)
	n.aHidden = true
	n.ln.Width, n.ln.Height = size, size
	n.ln.NoStretch = true
	if ic == nil {
		ic = icon.Robot
	}
	n.paint = func(l *paint.List, r geom.Rect) {
		l.Fill(r, geom.Rad(size/2), tk.AssistantAvatarBg)
		if label != "" {
			lay := text.Shape(label, c.style(th.FontSize, text.Regular), text.Options{NoWrap: true})
			l.Glyphs(geom.Pt(r.X+(r.W-lay.Width)/2, r.Y+(r.H-lay.Height)/2), lay.Glyphs, tk.AssistantAvatarFg)
			return
		}
		s := size * 0.56
		c.paintIcon(l, ic, geom.Rect{X: r.X + (r.W-s)/2, Y: r.Y + (r.H-s)/2, W: s, H: s}, tk.AssistantAvatarFg, 0)
	}
}

// chatDots is the "thinking" indicator: three dots pulsing in turn (static
// when animations are off).
func chatDots(c *Context, col theme.Color, name string) {
	n := c.add("chat-dots", name)
	n.setRole(a11y.RoleStatus).setState(a11y.Busy)
	lh := c.Theme.LineHeightFor(c.Theme.FontSize)
	n.ln.Width, n.ln.Height = 36, lh
	n.ln.NoStretch = true
	n.paint = func(l *paint.List, r geom.Rect) {
		var phase float64
		if !c.still() {
			c.animating = true
			phase = float64(c.now.UnixMilli()%1200) / 1200
		}
		for i := range 3 {
			op := 0.35
			if !c.still() {
				d := phase*3 - float64(i)
				if d < 0 {
					d += 3
				}
				if d < 1 {
					op = 0.35 + 0.65*(1-d)
				}
			} else if i == 0 {
				op = 1
			}
			x := c.inline(r).X(3+float32(i)*12, 6) // the first dot leads in reading order
			l.Path(paint.NewPath().Circle(x+3, r.Y+r.H/2, 3), theme.Fade(col, op))
		}
	}
}
