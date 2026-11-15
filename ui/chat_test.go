package ui

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/tangoui/tango/ui/a11y"
	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/input"
	"github.com/tangoui/tango/ui/layout"
	"github.com/tangoui/tango/ui/theme"
)

func chatCenter(r a11y.Rect) geom.Point { return geom.Pt(r.X+r.W/2, r.Y+r.H/2) }

func chatFind(t *testing.T, d *Driver, role a11y.Role, name string) *a11y.Node {
	t.Helper()
	n, ok := d.A11yFind(role, name)
	if !ok {
		t.Fatalf("no %v %q", role, name)
	}
	return n
}

// ---------------------------------------------------------------------------
// Sender

type senderHarness struct {
	val       string
	files     []ChatAttachment
	loading   bool
	submitted int
	stopped   int
	picked    string
	removed   int
}

func newSenderDriver(h *senderHarness) *Driver {
	h.removed = -1
	cmds := []Option{{Label: "Clear chat", Value: "clear"}, {Label: "Explain code", Value: "explain"}}
	d := NewDriver(func(c *Context) {
		ConfigProvider(c).Locale(EnUS).Body(func() {
			Column(c, func() {
				s := Sender(c, &h.val).Loading(h.loading).Attachments(&h.files).Commands(cmds).Mentions(Opts("main.go", "go.mod"))
				if s.Submitted() {
					h.submitted++
				}
				if s.Stopped() {
					h.stopped++
				}
				if p, o, ok := s.Picked(); ok {
					h.picked = p + o.Value
				}
				if i, ok := s.Removed(); ok {
					h.removed = i
				}
			}, layout.Pad(40), layout.W(500))
		})
	}, DriverOptions{Width: 600, Height: 600})
	d.Frame()
	return d
}

func senderFocus(d *Driver, h *senderHarness) {
	d.EnableAccessibility()
	n, _ := d.A11yFind(a11y.RoleTextbox, EnUS.Chat.Placeholder)
	d.Click(chatCenter(n.Bounds))
}

func TestSenderEnterSendsShiftEnterBreaks(t *testing.T) {
	h := &senderHarness{}
	d := newSenderDriver(h)
	senderFocus(d, h)
	d.Key(input.KeyEnter, 0)
	if h.submitted != 0 {
		t.Fatal("empty input must not send")
	}
	d.Type("hello")
	d.Key(input.KeyEnter, input.ModShift)
	d.Type("world")
	if h.val != "hello\nworld" {
		t.Fatalf("Shift+Enter: %q", h.val)
	}
	if h.submitted != 0 {
		t.Fatal("Shift+Enter sent")
	}
	d.Key(input.KeyEnter, 0)
	if h.submitted != 1 {
		t.Fatalf("Enter did not send (%d)", h.submitted)
	}
	if h.val != "hello\nworld" {
		t.Fatalf("Enter changed the text: %q", h.val)
	}
}

func TestSenderIMEEnterDoesNotSend(t *testing.T) {
	h := &senderHarness{val: "x"}
	d := newSenderDriver(h)
	senderFocus(d, h)
	d.Compose("ni", 2)
	d.Key(input.KeyEnter, 0) // confirms the candidate inside the IME
	if h.submitted != 0 {
		t.Fatal("Enter during composition sent the message")
	}
	// The commit frame: the composition ends and an Enter may arrive in
	// the same frame on some platforms.
	d.Inject(input.Composition{})
	d.Inject(input.TextInput{Text: "你"})
	d.Inject(input.KeyEvent{Key: input.KeyEnter, Down: true})
	d.Inject(input.KeyEvent{Key: input.KeyEnter})
	d.Settle()
	if h.submitted != 0 {
		t.Fatal("Enter that ended the composition sent the message")
	}
	if strings.Contains(h.val, "\n") {
		t.Fatalf("composition Enter inserted a line break: %q", h.val)
	}
	d.Key(input.KeyEnter, 0)
	if h.submitted != 1 {
		t.Fatal("Enter after the composition did not send")
	}
}

func TestSenderLoadingStops(t *testing.T) {
	h := &senderHarness{val: "next", loading: true}
	d := newSenderDriver(h)
	senderFocus(d, h)
	d.Key(input.KeyEnter, 0)
	if h.submitted != 0 {
		t.Fatal("sent while loading")
	}
	d.Key(input.KeyEscape, 0)
	if h.stopped != 1 {
		t.Fatalf("Escape while loading: stopped=%d", h.stopped)
	}
	stop := chatFind(t, d, a11y.RoleButton, EnUS.Chat.Stop)
	d.Click(chatCenter(stop.Bounds))
	if h.stopped != 2 {
		t.Fatalf("stop button: stopped=%d", h.stopped)
	}
	if _, ok := d.A11yFind(a11y.RoleButton, EnUS.Chat.Send); ok {
		t.Fatal("send button shown while loading")
	}
}

func TestSenderSendButton(t *testing.T) {
	h := &senderHarness{}
	d := newSenderDriver(h)
	d.EnableAccessibility()
	send := chatFind(t, d, a11y.RoleButton, EnUS.Chat.Send)
	if send.States&a11y.Disabled == 0 {
		t.Fatal("send button enabled with nothing to send")
	}
	h.val = "hi"
	d.Settle()
	d.Click(chatCenter(send.Bounds))
	if h.submitted != 1 {
		t.Fatal("send button did not send")
	}
}

func TestSenderCommandSuggestions(t *testing.T) {
	h := &senderHarness{}
	d := newSenderDriver(h)
	senderFocus(d, h)
	d.Type("/")
	if _, ok := d.A11yFind(a11y.RoleListbox, EnUS.Chat.Suggestions); !ok {
		t.Fatal("suggestions not opened by /")
	}
	d.Type("ex")
	d.Key(input.KeyEnter, 0)
	if h.val != "/explain " || h.picked != "/explain" {
		t.Fatalf("picked %q, text %q", h.picked, h.val)
	}
	if h.submitted != 0 {
		t.Fatal("Enter in the popup sent the message")
	}
	if _, ok := d.A11yFind(a11y.RoleListbox, EnUS.Chat.Suggestions); ok {
		t.Fatal("popup still open after picking")
	}
	// @ after a space, Down picks the second option.
	d.Type("see @")
	d.Key(input.KeyDown, 0)
	d.Key(input.KeyTab, 0)
	if h.val != "/explain see @go.mod " {
		t.Fatalf("mention: %q", h.val)
	}
	// Escape closes the list without sending; Enter then sends.
	d.Type("@")
	d.Key(input.KeyEscape, 0)
	if _, ok := d.A11yFind(a11y.RoleListbox, EnUS.Chat.Suggestions); ok {
		t.Fatal("Escape did not close the list")
	}
	d.Key(input.KeyEnter, 0)
	if h.submitted != 1 {
		t.Fatal("Enter after closing the list did not send")
	}
}

func TestSenderAttachments(t *testing.T) {
	h := &senderHarness{files: []ChatAttachment{{Name: "a.txt", Size: 2048}, {Name: "b.png", Status: "uploading", Percent: 40}}}
	d := newSenderDriver(h)
	d.EnableAccessibility()
	if _, ok := d.FindText("2.0 KB"); !ok {
		t.Fatal("file size not shown")
	}
	rm := chatFind(t, d, a11y.RoleButton, EnUS.RemoveItem("a.txt"))
	d.Click(chatCenter(rm.Bounds))
	if len(h.files) != 1 || h.files[0].Name != "b.png" || h.removed != 0 {
		t.Fatalf("remove: %+v removed=%d", h.files, h.removed)
	}
	// Attachments alone can be sent.
	send := chatFind(t, d, a11y.RoleButton, EnUS.Chat.Send)
	if send.States&a11y.Disabled != 0 {
		t.Fatal("send disabled with an attachment")
	}
}

func TestSenderAutoSize(t *testing.T) {
	h := &senderHarness{}
	d := newSenderDriver(h)
	d.EnableAccessibility()
	sendY := func() float32 { return chatFind(t, d, a11y.RoleButton, EnUS.Chat.Send).Bounds.Y }
	lh := d.r.c.Theme.LineHeightFor(d.r.c.Theme.FontSize)
	y1 := sendY()
	h.val = "1\n2\n3"
	d.Settle()
	if got := sendY() - y1; got < 2*lh-1 {
		t.Fatalf("3 lines grew by %v", got)
	}
	h.val = strings.Repeat("x\n", 30)
	d.Settle()
	if got := sendY() - y1; got > 7*lh+1 {
		t.Fatalf("grew past 8 rows: %v", got)
	}
}

// ---------------------------------------------------------------------------
// Bubble

func TestBubbleCopyRetryEdit(t *testing.T) {
	var copied, retried int
	var edited string
	text := "first answer"
	d := NewDriver(func(c *Context) {
		ConfigProvider(c).Locale(EnUS).Body(func() {
			Column(c, func() {
				a := Bubble(c, ChatAssistant, text).Actions(BubbleCopy | BubbleRetry).ActionsAlways()
				if a.Copied() {
					copied++
				}
				if a.Retried() {
					retried++
				}
				u := Bubble(c, ChatUser, "question").Actions(BubbleEdit).ActionsAlways()
				if s, ok := u.Edited(); ok {
					edited = s
				}
			}, layout.Pad(20), layout.Gap(12))
		})
	}, DriverOptions{Width: 700, Height: 500})
	d.EnableAccessibility()
	cp := chatFind(t, d, a11y.RoleButton, EnUS.Copy)
	d.Click(chatCenter(cp.Bounds))
	if copied != 1 || d.Clipboard() != text {
		t.Fatalf("copy: %d %q", copied, d.Clipboard())
	}
	if _, ok := d.A11yFind(a11y.RoleButton, EnUS.Copied); !ok {
		t.Fatal("copy button does not confirm")
	}
	d.Advance(2 * time.Second)
	d.Settle()
	if _, ok := d.A11yFind(a11y.RoleButton, EnUS.Copy); !ok {
		t.Fatal("copy confirmation does not reset")
	}
	d.Click(chatCenter(chatFind(t, d, a11y.RoleButton, EnUS.Retry).Bounds))
	if retried != 1 {
		t.Fatal("retry")
	}
	d.Click(chatCenter(chatFind(t, d, a11y.RoleButton, EnUS.Edit).Bounds))
	ed := chatFind(t, d, a11y.RoleTextbox, EnUS.Chat.EditPlaceholder)
	if ed.Value != "question" {
		t.Fatalf("editor value %q", ed.Value)
	}
	d.Click(chatCenter(ed.Bounds))
	d.Key(input.KeyEnd, shortcut)
	d.Type(" more")
	d.Key(input.KeyEnter, shortcut)
	if edited != "question more" {
		t.Fatalf("edited %q", edited)
	}
	if _, ok := d.A11yFind(a11y.RoleTextbox, EnUS.Chat.EditPlaceholder); ok {
		t.Fatal("editor still open after saving")
	}
	// Escape cancels.
	d.Click(chatCenter(chatFind(t, d, a11y.RoleButton, EnUS.Edit).Bounds))
	ed = chatFind(t, d, a11y.RoleTextbox, EnUS.Chat.EditPlaceholder)
	d.Click(chatCenter(ed.Bounds))
	d.Key(input.KeyEscape, 0)
	if _, ok := d.A11yFind(a11y.RoleTextbox, EnUS.Chat.EditPlaceholder); ok {
		t.Fatal("Escape did not cancel the edit")
	}
	if edited != "question more" {
		t.Fatal("cancel reported an edit")
	}
}

func TestBubbleTypingReveals(t *testing.T) {
	content := strings.Repeat("a", 30)
	typing := true
	var w *BubbleW
	d := NewDriver(func(c *Context) { w = Bubble(c, ChatAssistant, content).Plain().Typing(typing).TypingSpeed(10) }, DriverOptions{Width: 600, Height: 200, Animations: true})
	d.Frame()
	d.Advance(500 * time.Millisecond)
	d.Frame()
	if w.st.shown == 0 || w.st.shown >= 30 {
		t.Fatalf("revealed %d", w.st.shown)
	}
	if _, ok := d.FindText(strings.Repeat("a", w.st.shown) + bubbleCaret); !ok {
		t.Fatal("caret not shown")
	}
	typing = false
	d.Frame()
	if _, ok := d.FindText(content); !ok {
		t.Fatal("typing off")
	}
}

func TestBubbleLoadingAndSystem(t *testing.T) {
	d := NewDriver(func(c *Context) {
		ConfigProvider(c).Locale(EnUS).Body(func() {
			Column(c, func() {
				Bubble(c, ChatAssistant, "").Loading(true)
				Bubble(c, ChatSystem, "Model switched")
			})
		})
	}, DriverOptions{Width: 600, Height: 300})
	d.EnableAccessibility()
	st := chatFind(t, d, a11y.RoleStatus, EnUS.Chat.Thinking)
	if st.States&a11y.Busy == 0 {
		t.Fatal("loading dots not busy")
	}
	g := chatFind(t, d, a11y.RoleGroup, EnUS.Chat.System)
	r, _ := d.FindText("Model switched")
	if mid := r.X + r.W/2; mid < 250 || mid > 350 {
		t.Fatalf("system notice not centered: %v (group %v)", r, g.Bounds)
	}
}

// ---------------------------------------------------------------------------
// BubbleList

type blHarness struct {
	keys  []string
	texts map[string]string
	list  *BubbleListW
	built map[string]bool
}

func newBLDriver(h *blHarness) *Driver {
	d := NewDriver(func(c *Context) {
		h.built = map[string]bool{}
		h.list = BubbleList(c, len(h.keys), func(i int) string { return h.keys[i] }, func(i int) {
			k := h.keys[i]
			h.built[k] = true
			Bubble(c, ChatAssistant, h.texts[k]).Plain().NoAvatar()
		}, layout.H(400))
	}, DriverOptions{Width: 500, Height: 400})
	d.Settle()
	return d
}

func blMessages(n int) *blHarness {
	h := &blHarness{texts: map[string]string{}}
	for i := range n {
		k := fmt.Sprintf("m%d", i)
		h.keys = append(h.keys, k)
		h.texts[k] = fmt.Sprintf("message %d", i) + strings.Repeat("\nline", i%4)
	}
	return h
}

func TestBubbleListVirtualizesAndFollows(t *testing.T) {
	h := blMessages(500)
	d := newBLDriver(h)
	first, end := h.list.Range()
	if end-first > 60 || end != 500 {
		t.Fatalf("built [%d,%d) of 500", first, end)
	}
	if !h.list.Following() {
		t.Fatal("not following at start")
	}
	if _, ok := d.FindText("message 499"); !ok {
		t.Fatal("last message not shown")
	}
	// New content arrives: still at the bottom.
	h.keys = append(h.keys, "new")
	h.texts["new"] = "fresh\nreply\nwith\nlines"
	d.Settle()
	if r, ok := d.FindText("fresh"); !ok || r.Y > 400 {
		t.Fatalf("new message not followed: %v %v", r, ok)
	}
	// Scroll up: stop following, show the button.
	d.Scroll(geom.Pt(250, 200), -600)
	d.Settle()
	if h.list.Following() {
		t.Fatal("still following after scrolling up")
	}
	d.EnableAccessibility()
	btn := chatFind(t, d, a11y.RoleButton, ZhCN.Chat.ScrollToBottom)
	_, before := h.list.sb.Offset()
	h.texts["new"] += "\nmore"
	d.Settle()
	if _, after := h.list.sb.Offset(); after != before {
		t.Fatalf("view moved while not following: %v -> %v", before, after)
	}
	d.Click(chatCenter(btn.Bounds))
	d.Settle()
	if !h.list.Following() {
		t.Fatal("button did not resume following")
	}
	if _, ok := d.A11yFind(a11y.RoleButton, ZhCN.Chat.ScrollToBottom); ok {
		t.Fatal("button still shown at the bottom")
	}
}

func TestBubbleListPrependKeepsView(t *testing.T) {
	h := blMessages(40)
	d := newBLDriver(h)
	// Go to the top.
	for range 30 {
		d.Scroll(geom.Pt(250, 200), -2000)
	}
	d.Settle()
	if _, off := h.list.sb.Offset(); off != 0 {
		t.Fatalf("not at top: %v", off)
	}
	r0, ok := d.FindText("message 0")
	if !ok {
		t.Fatal("first message not visible")
	}
	// Load earlier messages.
	var older []string
	for i := range 20 {
		k := fmt.Sprintf("old%d", i)
		older = append(older, k)
		h.texts[k] = "older " + k
	}
	h.keys = append(older, h.keys...)
	d.Settle()
	d.Settle()
	r1, ok := d.FindText("message 0")
	if !ok || r1.Y != r0.Y {
		t.Fatalf("view jumped after prepending: %v -> %v (%v)", r0, r1, ok)
	}
}

func TestBubbleListTopReached(t *testing.T) {
	h := blMessages(40)
	reached := 0
	d := NewDriver(func(c *Context) {
		l := BubbleList(c, len(h.keys), func(i int) string { return h.keys[i] }, func(i int) {
			Text(c, h.texts[h.keys[i]])
		}, layout.H(300))
		if l.TopReached() {
			reached++
		}
	}, DriverOptions{Width: 500, Height: 300})
	d.Settle()
	for range 40 {
		d.Scroll(geom.Pt(250, 150), -2000)
	}
	d.Settle()
	if reached != 1 {
		t.Fatalf("TopReached fired %d times", reached)
	}
}

// ---------------------------------------------------------------------------
// ThoughtChain, ToolCall, Approval, Conversations, Prompts

func TestThoughtChainToggle(t *testing.T) {
	items := []ChainItem{
		{Key: "a", Title: "Plan", Status: ChainSuccess, Duration: 1500 * time.Millisecond, Detail: "the detail"},
		{Key: "b", Title: "Run", Status: ChainRunning, Started: time.Time{}},
		{Key: "c", Title: "Sum", Status: ChainPending},
	}
	var toggled string
	d := NewDriver(func(c *Context) {
		ConfigProvider(c).Locale(EnUS).Body(func() {
			w := ThoughtChain(c, items).Title("Thinking").Card()
			if k, ok := w.ItemToggled(); ok {
				toggled = k
			}
		})
	}, DriverOptions{Width: 600, Height: 400})
	d.EnableAccessibility()
	head := chatFind(t, d, a11y.RoleButton, "Thinking 1/3 "+EnUS.Chat.Running)
	if head.States&a11y.Expanded == 0 {
		t.Fatal("chain not expanded")
	}
	if _, ok := d.FindText("1.5s"); !ok {
		t.Fatal("duration not shown")
	}
	step := chatFind(t, d, a11y.RoleButton, "Plan, "+EnUS.Chat.Done)
	d.Click(chatCenter(step.Bounds))
	if toggled != "a" {
		t.Fatal("step toggle not reported")
	}
	if _, ok := d.FindText("the detail"); !ok {
		t.Fatal("detail not shown")
	}
	d.Click(chatCenter(head.Bounds))
	if _, ok := d.FindText("Plan"); ok {
		t.Fatal("collapsed chain still shows its steps")
	}
	// Keyboard: Enter on the focused header expands again.
	d.Key(input.KeyEnter, 0)
	if _, ok := d.FindText("Plan"); !ok {
		t.Fatal("Enter did not expand")
	}
}

func TestToolCallToggle(t *testing.T) {
	d := NewDriver(func(c *Context) {
		ConfigProvider(c).Locale(EnUS).Body(func() {
			ToolCall(c, "read_file").Summary("main.go").Status(ChainSuccess).Args(`{"p":1}`).Result("ok ```fence```")
		})
	}, DriverOptions{Width: 600, Height: 400})
	d.EnableAccessibility()
	if _, ok := d.FindText(EnUS.Chat.Arguments); ok {
		t.Fatal("body open by default")
	}
	h := chatFind(t, d, a11y.RoleButton, "read_file main.go "+EnUS.Chat.Done)
	d.Click(chatCenter(h.Bounds))
	if _, ok := d.FindText(EnUS.Chat.Arguments); !ok {
		t.Fatal("arguments not shown")
	}
	if _, ok := d.FindText("ok ```fence```"); !ok {
		t.Fatal("result with backticks not shown verbatim")
	}
}

func TestChatFence(t *testing.T) {
	if got := chatFence("a ``` b", "go"); got != "````go\na ``` b\n````" {
		t.Fatalf("%q", got)
	}
}

func TestApprovalDecide(t *testing.T) {
	var got ApprovalChoice
	note := ""
	decided := ApproveNone
	d := NewDriver(func(c *Context) {
		ConfigProvider(c).Locale(EnUS).Body(func() {
			a := Approval(c, "").Description("Run a command").Command("ls", "sh").Feedback(&note).Decision(decided)
			if ch, ok := a.Decided(); ok {
				got = ch
			}
		})
	}, DriverOptions{Width: 600, Height: 400})
	d.EnableAccessibility()
	g := chatFind(t, d, a11y.RoleGroup, EnUS.Chat.ApprovalTitle)
	if g.Description != "Run a command" {
		t.Fatalf("description %q", g.Description)
	}
	if err := d.ClickLabel(EnUS.Chat.AllowAlways); err != nil {
		t.Fatal(err)
	}
	if got != ApproveAlways {
		t.Fatalf("decided %v", got)
	}
	decided = ApproveDeny
	d.Settle()
	if _, ok := d.FindText(EnUS.Chat.Denied); !ok {
		t.Fatal("decided card does not show the outcome")
	}
	if _, ok := d.Find(EnUS.Chat.AllowOnce); ok {
		t.Fatal("decided card still shows buttons")
	}
}

func TestConversations(t *testing.T) {
	now := time.Date(2026, 11, 14, 15, 0, 0, 0, time.UTC)
	items := []Conversation{
		{Key: "a", Title: "Alpha", Time: now.Add(-time.Hour)},
		{Key: "b", Title: "Beta", Time: now.Add(-26 * time.Hour)},
		{Key: "c", Title: "", Time: now.AddDate(0, 0, -60)},
	}
	active := "a"
	var renamed, deleted, selected string
	d := NewDriver(func(c *Context) {
		ConfigProvider(c).Locale(EnUS).Body(func() {
			w := Conversations(c, items, &active)
			if k, t, ok := w.Renamed(); ok {
				renamed = k + "=" + t
			}
			if k, ok := w.Deleted(); ok {
				deleted = k
			}
			if k, ok := w.Selected(); ok {
				selected = k
			}
		})
	}, DriverOptions{Width: 300, Height: 400})
	d.w.now = now
	d.EnableAccessibility()
	for _, g := range []string{EnUS.Chat.Today, EnUS.Chat.Yesterday, EnUS.Chat.Earlier, EnUS.Chat.Untitled} {
		if _, ok := d.FindText(g); !ok {
			t.Errorf("%q not shown", g)
		}
	}
	b := chatFind(t, d, a11y.RoleOption, "Beta")
	d.Click(chatCenter(b.Bounds))
	if active != "b" || selected != "b" {
		t.Fatalf("select: active=%q selected=%q", active, selected)
	}
	// Keyboard: Up moves focus, Enter selects.
	d.Key(input.KeyUp, 0)
	if f := d.A11yFocused(); f == nil || f.Name != "Alpha" {
		t.Fatalf("focus after Up: %+v", f)
	}
	d.Key(input.KeyEnter, 0)
	if active != "a" {
		t.Fatal("Enter did not select")
	}
	// F2 renames.
	d.Key(input.KeyF2, 0)
	d.Settle()
	in := chatFind(t, d, a11y.RoleTextbox, EnUS.Chat.Rename)
	if in.Value != "Alpha" {
		t.Fatalf("rename input %q", in.Value)
	}
	d.Key(input.KeyA, shortcut)
	d.Type("Gamma")
	d.Key(input.KeyEnter, 0)
	if renamed != "a=Gamma" {
		t.Fatalf("renamed %q", renamed)
	}
	if f := d.A11yFocused(); f == nil || f.Role != a11y.RoleOption {
		t.Fatalf("focus not back on the row: %+v", f)
	}
	// Delete key.
	d.Key(input.KeyDelete, 0)
	if deleted != "a" {
		t.Fatalf("deleted %q", deleted)
	}
	// Hover actions.
	beta := chatFind(t, d, a11y.RoleOption, "Beta").Bounds
	d.Move(chatCenter(beta))
	d.Settle()
	var del *a11y.Node
	for _, n := range d.A11yFindAll(a11y.RoleButton, EnUS.Delete) {
		if p := chatCenter(n.Bounds); p.Y >= beta.Y && p.Y <= beta.Y+beta.H {
			del = n
		}
	}
	if del == nil {
		t.Fatal("no row delete action")
	}
	d.Click(chatCenter(del.Bounds))
	if deleted != "b" {
		t.Fatalf("hover delete: %q", deleted)
	}
}

func TestPromptsClick(t *testing.T) {
	var got string
	d := NewDriver(func(c *Context) {
		w := Prompts(c, []PromptItem{{Key: "x", Label: "Explain"}, {Key: "y", Label: "Off", Disabled: true}}).Wrap()
		if k, ok := w.Clicked(); ok {
			got = k
		}
	}, DriverOptions{Width: 600, Height: 300})
	d.Settle()
	if err := d.ClickLabel("Off"); err != nil {
		t.Fatal(err)
	}
	if got != "" {
		t.Fatal("disabled prompt clicked")
	}
	if err := d.ClickLabel("Explain"); err != nil {
		t.Fatal(err)
	}
	if got != "x" {
		t.Fatalf("clicked %q", got)
	}
}

// ---------------------------------------------------------------------------
// Accessibility, RTL, locales

func chatAllPage(c *Context) {
	now := time.Date(2026, 11, 14, 15, 0, 0, 0, time.UTC)
	msgs := []string{"hello", "world"}
	draft := "hi"
	files := []ChatAttachment{{Name: "a.txt"}}
	active := "a"
	Row(c, func() {
		Conversations(c, []Conversation{{Key: "a", Title: "Alpha", Time: now}, {Key: "b", Title: "Beta", Time: now}}, &active).Layout(layout.W(200))
		Column(c, func() {
			BubbleList(c, len(msgs), func(i int) string { return msgs[i] }, func(i int) {
				Bubble(c, ChatRole(i%2), msgs[i]).Actions(BubbleCopy | BubbleRetry | BubbleEdit).Extra(func() {
					ToolCall(c, "tool").Status(ChainSuccess).Args("{}").DefaultOpen()
				})
			}, layout.H(300))
			ThoughtChain(c, []ChainItem{{Title: "Plan", Detail: "x"}, {Title: "Run", Status: ChainRunning}}).Title("T")
			Approval(c, "").Feedback(new(string))
			Prompts(c, []PromptItem{{Label: "Explain", Description: "d"}})
			Sender(c, &draft).Attachments(&files).AttachButton()
		}, layout.Grow(1), layout.Gap(8))
	}, layout.Gap(8))
}

func TestChatAudit(t *testing.T) {
	for _, th := range []*theme.Theme{theme.AntDesign(), theme.ElementPlus(), theme.Shadcn(), theme.Shadcn(theme.Dark())} {
		d := NewDriver(chatAllPage, DriverOptions{Width: 1000, Height: 1400, Theme: th})
		rep := d.Audit()
		if n := rep.MissingInteractiveNames(); n > 0 {
			t.Errorf("%s: %d interactive nodes without a name\n%s", th.Flavor, n, rep)
		}
		for _, is := range rep.Issues {
			if is.Kind != AuditContrast {
				t.Errorf("%s: %v", th.Flavor, is)
			}
		}
	}
}

func TestChatRTLMirror(t *testing.T) {
	d := NewDriver(func(c *Context) {
		ConfigProvider(c).Direction(RTL).Body(func() { chatAllPage(c) })
	}, DriverOptions{Width: 1000, Height: 1400})
	rep := d.Audit()
	if n := rep.Count(AuditUnmirroredIcon); n > 0 {
		t.Fatalf("unmirrored icons:\n%s", rep)
	}
	// The user bubble sits at the start (left) edge in RTL.
	d.Settle()
	r, ok := d.FindText("world")
	if !ok || r.X > 600 {
		t.Fatalf("user bubble not mirrored: %v", r)
	}
}

func TestChatI18n(t *testing.T) {
	now := time.Date(2026, 11, 14, 15, 0, 0, 0, time.UTC)
	cases := []i18nCase{
		{name: "Bubble",
			page: func(c *Context, ids i18nIDs) {
				Bubble(c, ChatAssistant, "x").Actions(BubbleCopy | BubbleRetry).ActionsAlways()
				Bubble(c, ChatUser, "y").Actions(BubbleEdit).ActionsAlways()
				Bubble(c, ChatAssistant, "").Loading(true)
			},
			texts: func(l *Locale) []string { return []string{l.Copy, l.Retry, l.Edit, l.Chat.Thinking} }},
		{name: "Approval",
			page: func(c *Context, ids i18nIDs) { Approval(c, "").Risk("error") },
			texts: func(l *Locale) []string {
				return []string{l.Chat.ApprovalTitle, l.Chat.RiskDanger, l.Chat.Deny, l.Chat.AllowAlways, l.Chat.AllowOnce}
			}},
		{name: "ApprovalDecided",
			page:  func(c *Context, ids i18nIDs) { Approval(c, "").Decision(ApproveSession) },
			texts: func(l *Locale) []string { return []string{l.Chat.Allowed + " · " + l.Chat.AllowSession} }},
		{name: "Conversations",
			page: func(c *Context, ids i18nIDs) {
				a := ""
				Conversations(c, []Conversation{{Key: "1", Time: c.Now()}, {Key: "2", Title: "t", Time: c.Now().AddDate(0, 0, -1)},
					{Key: "3", Title: "u", Time: c.Now().AddDate(0, 0, -5)}, {Key: "4", Title: "v", Time: c.Now().AddDate(0, 0, -20)},
					{Key: "5", Title: "w", Time: c.Now().AddDate(-1, 0, 0)}}, &a)
			},
			setup: func(d *Driver, _ i18nIDs) { d.w.now = now; d.Settle() },
			texts: func(l *Locale) []string {
				return []string{l.Chat.Today, l.Chat.Yesterday, l.Chat.Last7Days, l.Chat.Last30Days, l.Chat.Earlier}
			}},
		{name: "ToolCall",
			page: func(c *Context, ids i18nIDs) {
				ToolCall(c, "t").Args("{}").Result("r").Duration(1500 * time.Millisecond).DefaultOpen()
			},
			texts: func(l *Locale) []string {
				return []string{l.Chat.Arguments, l.Chat.Result, l.chatDuration(1500 * time.Millisecond)}
			}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			for _, l := range i18nLocales {
				d, _ := i18nRender(tc, l)
				labels := i18nLabels(d)
				own := map[string]bool{}
				for _, s := range tc.texts(l) {
					own[s] = true
					if !labels[s] {
						t.Errorf("%s: text %q not rendered", l.Code, s)
					}
				}
				for _, o := range i18nLocales {
					for _, s := range tc.texts(o) {
						if o != l && !own[s] && labels[s] {
							t.Errorf("%s: %s text %q leaked", l.Code, o.Code, s)
						}
					}
				}
				if l == EnUS {
					for s := range labels {
						if strings.IndexFunc(s, isCJK) >= 0 {
							t.Errorf("en-US: CJK label %q", s)
						}
					}
				}
			}
		})
	}
	// Painted placeholder of the Sender comes from the locale too.
	for _, l := range i18nLocales {
		var s *SenderW
		d := NewDriver(func(c *Context) {
			ConfigProvider(c).Locale(l).Body(func() { s = Sender(c, new(string)) })
		}, DriverOptions{Width: 600, Height: 200})
		d.Frame()
		if s.placeholder != l.Chat.Placeholder {
			t.Errorf("%s: placeholder %q", l.Code, s.placeholder)
		}
	}
}

// shortcut is the platform shortcut modifier (Cmd on Apple, Ctrl elsewhere).
var shortcut = func() input.Modifiers {
	if (input.Modifiers(input.ModMeta)).Shortcut() {
		return input.ModMeta
	}
	return input.ModCtrl
}()
