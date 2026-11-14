package ui

import (
	"github.com/tangoui/tango/ui/a11y"
	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/icon"
	"github.com/tangoui/tango/ui/layout"
	"github.com/tangoui/tango/ui/paint"
	"github.com/tangoui/tango/ui/theme"
)

// ---------------------------------------------------------------------------
// Approval (tool permission request)

// ApprovalChoice is a decision on an Approval.
type ApprovalChoice uint8

const (
	ApproveNone    ApprovalChoice = iota // undecided
	ApproveOnce                          // allow this request
	ApproveSession                       // allow for the rest of the conversation
	ApproveAlways                        // allow from now on
	ApproveDeny
)

func (a ApprovalChoice) label(l *Locale) string {
	switch a {
	case ApproveOnce:
		return l.Chat.AllowOnce
	case ApproveSession:
		return l.Chat.AllowSession
	case ApproveAlways:
		return l.Chat.AllowAlways
	case ApproveDeny:
		return l.Chat.Deny
	}
	return ""
}

type approvalField struct{ label, value string }

type approvalState struct{ announced bool }

// ApprovalW is a card asking the user to allow or deny an action (a tool
// call, a command, a file write) before the assistant goes on.
type ApprovalW struct {
	_ [0]func()

	c        *Context
	b        *Box
	z        *lazy
	loc      *Locale
	title    string
	desc     string
	fields   []approvalField
	code     string
	lang     string
	risk     string
	choices  []ApprovalChoice
	feedback *string
	decision ApprovalChoice
	body     func()
	decided  ApprovalChoice
}

// Approval adds a permission card; title "" uses Locale.Chat.ApprovalTitle.
//
//	a := ui.Approval(c, "").Description("运行 shell 命令").Command("rm -rf build", "sh")
//	if d, ok := a.Decided(); ok { resolve(d) }
func Approval(c *Context, title string) *ApprovalW {
	w := &ApprovalW{c: c, title: title, loc: c.Locale(), risk: "warning"}
	if w.title == "" {
		w.title = w.loc.Chat.ApprovalTitle
	}
	w.choices = []ApprovalChoice{ApproveDeny, ApproveAlways, ApproveOnce}
	w.b = c.box("approval", layout.Column, nil)
	w.b.n.setRole(a11y.RoleGroup).setName(w.title).fill(w)
	w.z = c.lazy(w.b.n, w.build)
	return w
}

func (w *ApprovalW) semantics(s *a11y.Node) { s.Description = w.desc }

// Description explains what will happen.
func (w *ApprovalW) Description(s string) *ApprovalW { w.desc = s; return w }

// Field adds a labelled detail row (tool, resource, scope …).
func (w *ApprovalW) Field(label, value string) *ApprovalW {
	w.fields = append(w.fields, approvalField{label, value})
	return w
}

// Command shows the command or parameters as a code block in lang.
func (w *ApprovalW) Command(code, lang string) *ApprovalW { w.code, w.lang = code, lang; return w }

// Risk sets the level: "warning" (default) or "error" (dangerous).
func (w *ApprovalW) Risk(level string) *ApprovalW { w.risk = level; return w }

// Choices sets the buttons in order (default Deny, Always, Once; the last
// one is the primary button).
func (w *ApprovalW) Choices(ch ...ApprovalChoice) *ApprovalW { w.choices = ch; return w }

// Feedback adds an optional note input bound to *note, sent with the
// decision.
func (w *ApprovalW) Feedback(note *string) *ApprovalW { w.feedback = note; return w }

// Decision shows a decided card: the buttons give way to the outcome.
func (w *ApprovalW) Decision(d ApprovalChoice) *ApprovalW { w.decision = d; return w }

// Body adds custom content above the buttons.
func (w *ApprovalW) Body(fn func()) *ApprovalW { w.body = fn; return w }

// Layout applies layout options to the card.
func (w *ApprovalW) Layout(opts ...layout.Option) *ApprovalW {
	w.b.Style(opts...)
	return w
}

// Decided returns the choice clicked this frame.
func (w *ApprovalW) Decided() (ApprovalChoice, bool) {
	w.z.run()
	return w.decided, w.decided != ApproveNone
}

func (w *ApprovalW) build() {
	c := w.c
	th := c.ThemeFor(theme.CompChat)
	tk := &th.Components.Chat
	loc := w.loc
	danger := w.risk == "error"
	bg, border, icCol := tk.ApprovalBg, tk.ApprovalBorder, tk.ApprovalIcon
	if danger {
		bg, border, icCol = tk.DangerBg, tk.DangerBorder, tk.DangerIcon
	}
	w.b.Bg(bg).Radius(tk.CardRadius).Border(1, border)
	w.b.n.ln.Padding = geom.All(12)
	w.b.n.ln.Gap = 8
	st := State[approvalState](c, w.b.n.id, nil)
	if w.decision == ApproveNone && !st.announced {
		st.announced = true
		c.Announce(w.title+" "+w.desc, a11y.LiveAssertive)
	}
	Row(c, func() {
		ic := c.add("approval-icon", "")
		ic.aHidden = true
		ic.ln.Width, ic.ln.Height = 16, 16
		ic.paint = func(l *paint.List, r geom.Rect) { c.paintIcon(l, icon.Lock, r, icCol, 0) }
		Text(c, w.title).Strong().Ellipsis().Layout(layout.Shrink(1), layout.MinW(0)).n.aHidden = true
		if w.decision != ApproveNone {
			return
		}
		risk, col := loc.Chat.RiskWarning, "warning"
		if danger {
			risk, col = loc.Chat.RiskDanger, "error"
		}
		Tag(c, risk).Color(col)
	}, layout.Gap(8), layout.Middle())
	if w.desc != "" {
		Text(c, w.desc).Secondary().n.aHidden = true
	}
	if len(w.fields) > 0 {
		Column(c, func() {
			for i, f := range w.fields {
				Key(c, i, func() {
					Row(c, func() {
						Text(c, loc.WithColon(f.label)).Tertiary().NoWrap()
						Text(c, f.value).Code().Layout(layout.Shrink(1), layout.MinW(0))
					}, layout.Gap(4), layout.Middle())
				})
			}
		}, layout.Gap(4))
	}
	if w.code != "" {
		Scroll(c, func() { Markdown(c, chatFence(w.code, w.lang)).Size(th.FontSizeSM) }, layout.MaxH(200))
	}
	if w.body != nil {
		w.body()
	}
	if w.decision != ApproveNone {
		w.outcome(th)
		return
	}
	if w.feedback != nil {
		Input(c, w.feedback).Placeholder(loc.Chat.FeedbackPlaceholder).A11yLabel(loc.Chat.FeedbackPlaceholder).Size(Small)
	}
	Row(c, func() {
		for i, ch := range w.choices {
			Key(c, int(ch), func() {
				b := Button(c, ch.label(loc)).Small()
				if i == len(w.choices)-1 {
					b.Primary()
					if danger {
						b.Danger()
					}
				}
				if b.Clicked() {
					w.decided = ch
				}
			})
		}
	}, layout.Gap(8), layout.End(), layout.WrapItems())
}

// outcome replaces the buttons of a decided card.
func (w *ApprovalW) outcome(th *theme.Theme) {
	c := w.c
	allowed := w.decision != ApproveDeny
	s, col, status := w.loc.Chat.Denied, th.Error.Base, "error"
	if allowed {
		s, col, status = w.loc.Chat.Allowed, th.Success.Base, "success"
		if w.decision != ApproveOnce {
			s += " · " + w.decision.label(w.loc)
		}
	}
	Row(c, func() {
		statusIcon(c, status, 14).aHidden = true
		Text(c, s).Color(col)
	}, layout.Gap(6), layout.Middle())
}
