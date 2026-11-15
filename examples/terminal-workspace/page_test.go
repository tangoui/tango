package main

import (
	"bytes"
	"errors"
	"fmt"
	"math"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/tangoui/tango/ui"
	"github.com/tangoui/tango/ui/a11y"
	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/input"
	"github.com/tangoui/tango/ui/terminal"
)

// A controllable byte stream exercises the real terminal emulator and view.
// Native acceptance separately checks the PTY boundary.
type screenConn struct {
	mu            sync.Mutex
	data, written []byte
	sizes         [][2]int
	ready, done   chan struct{}
	once          sync.Once
}

func newScreenConn() *screenConn {
	return &screenConn{data: []byte("ready\r\n$ "), ready: make(chan struct{}, 1), done: make(chan struct{})}
}
func (c *screenConn) Read(b []byte) (int, error) {
	for {
		c.mu.Lock()
		if len(c.data) > 0 {
			n := copy(b, c.data)
			c.data = c.data[n:]
			c.mu.Unlock()
			return n, nil
		}
		c.mu.Unlock()
		select {
		case <-c.ready:
		case <-c.done:
			return 0, os.ErrClosed
		}
	}
}
func (c *screenConn) Write(b []byte) (int, error) {
	c.mu.Lock()
	c.written = append(c.written, b...)
	c.mu.Unlock()
	return len(b), nil
}
func (c *screenConn) Resize(cols, rows, _, _ int) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.sizes = append(c.sizes, [2]int{cols, rows})
	return nil
}
func (c *screenConn) Close() error { c.once.Do(func() { close(c.done) }); return nil }
func (c *screenConn) feed(s string) {
	c.mu.Lock()
	c.data = append(c.data, s...)
	c.mu.Unlock()
	select {
	case c.ready <- struct{}{}:
	default:
	}
}
func (c *screenConn) size() (int, [2]int) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.sizes) == 0 {
		return 0, [2]int{}
	}
	return len(c.sizes), c.sizes[len(c.sizes)-1]
}

func pageHarness(t *testing.T) (*terminalApp, *ui.Driver, map[string]*screenConn) {
	t.Helper()
	connections := map[string]*screenConn{}
	a := &terminalApp{dark: true, fontSize: 13, focusPending: true}
	a.workspace.openSession = func(id string) (*session, error) {
		conn := newScreenConn()
		term := terminal.New(terminal.Options{Conn: conn})
		connections[id] = conn
		return &session{id: id, term: term, closeConn: term.Close}, nil
	}
	if err := a.workspace.newTab(); err != nil {
		t.Fatal(err)
	}
	d := ui.NewDriver(a.page, ui.DriverOptions{Width: 1100, Height: 720, Theme: a.theme(), Accessibility: true})
	d.Settle()
	t.Cleanup(a.workspace.close)
	return a, d, connections
}

func clickControl(t *testing.T, d *ui.Driver, name string) {
	t.Helper()
	n, ok := d.A11yFind(a11y.RoleButton, name)
	if !ok {
		t.Fatalf("missing button %q", name)
	}
	d.Click(geom.Pt(n.Bounds.X+n.Bounds.W/2, n.Bounds.Y+n.Bounds.H/2))
}

func TestPageTabSplitAndClose(t *testing.T) {
	a, d, _ := pageHarness(t)
	first := a.workspace.activeSession()
	clickControl(t, d, "New tab")
	if len(a.workspace.tabs) != 2 || a.workspace.activeSession() == first {
		t.Fatal("new tab did not get its own session")
	}
	clickControl(t, d, "Split right")
	if len(d.A11yFindAll(a11y.RoleTextbox, "")) != 2 {
		t.Fatal("split did not render two terminals")
	}
	active := a.workspace.activeSession()
	focused := d.A11yFocused()
	if focused == nil || focused.Name != "Terminal "+active.name {
		t.Fatal("new split was not focused")
	}
	clickControl(t, d, "Close pane")
	if len(a.workspace.activeTab().panes()) != 1 {
		t.Fatal("close did not collapse the split")
	}
	clickControl(t, d, "Close pane")
	if a.workspace.activeSession() != first {
		t.Fatal("closing last pane did not return to surviving tab")
	}
	clickControl(t, d, "Close pane")
	if len(a.workspace.tabs) != 0 {
		t.Fatal("last tab remains")
	}
	clickControl(t, d, "New terminal")
	if a.workspace.activeSession() == nil {
		t.Fatal("empty workspace cannot reopen a terminal")
	}
}

func TestPageBackgroundPaneKeepsItsSizeAndOutput(t *testing.T) {
	a, d, connections := pageHarness(t)
	firstTab, first := a.workspace.activeTab(), a.workspace.activeSession()
	conn := connections[first.id]
	_, originalSize := conn.size()
	clickControl(t, d, "New tab")
	hiddenCalls, _ := conn.size()
	conn.feed("background-output\r\n")
	d.Resize(960, 640)
	d.Settle()
	calls, _ := conn.size()
	if calls != hiddenCalls {
		t.Fatal("resized a hidden terminal")
	}
	a.workspace.selectTab(firstTab.id)
	a.focusPending = true
	d.Advance(100 * time.Millisecond)
	d.Settle()
	deadline := time.Now().Add(time.Second)
	for {
		v := first.term.Lock()
		var runes []rune
		for _, cell := range v.Screen().Line(1).Cells {
			if cell.Rune != 0 {
				runes = append(runes, cell.Rune)
			}
		}
		text := string(runes)
		first.term.Unlock()
		if bytes.Contains([]byte(text), []byte("background-output")) {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("background output was lost: %q", text)
		}
		time.Sleep(time.Millisecond)
	}
	_, shownSize := conn.size()
	if shownSize == originalSize {
		t.Fatal("returned pane did not fit the new window size")
	}
}

func TestPageQueuedInputDoesNotReachNewTab(t *testing.T) {
	a, d, connections := pageHarness(t)
	oldTab := a.workspace.activeTab()
	clickControl(t, d, "New tab")
	next := connections[a.workspace.activeSession().id]
	a.workspace.selectTab(oldTab.id)
	a.focusPending = true
	d.Settle()
	d.Inject(input.TextInput{Text: "old-pane-input"})
	a.workspace.cycleTab(1)
	a.focusPending = true
	d.Settle()
	time.Sleep(10 * time.Millisecond)
	next.mu.Lock()
	text := string(next.written)
	next.mu.Unlock()
	if text != "" {
		t.Fatalf("old input reached the new session: %q", text)
	}
}

func TestPageThemeAndLaunchFailure(t *testing.T) {
	a, d, _ := pageHarness(t)
	first := a.workspace.activeSession()
	clickControl(t, d, "Light theme")
	if a.dark {
		t.Fatal("theme toggle did not change state")
	}
	r, g, b, _ := d.Frame().At(4, 4).RGBA()
	if r+g+b < 3*0x8000 {
		t.Fatal("light theme kept painting a dark background")
	}
	clickControl(t, d, "Dark theme")
	a.workspace.openSession = func(string) (*session, error) { return nil, errors.New("test shell unavailable") }
	clickControl(t, d, "Split down")
	if a.workspace.activeSession() != first {
		t.Fatal("failure replaced the working terminal")
	}
	if _, ok := d.FindText("test shell unavailable"); !ok {
		t.Fatal("launch error was not shown")
	}
	d.Resize(720, 460)
	d.Settle()
	if n, ok := d.A11yFind(a11y.RoleButton, "New tab"); !ok || n.Bounds.W == 0 {
		t.Fatal("narrow toolbar lost its controls")
	}
}

func TestPageKeyboardActionsKeepRequestedPaneFocus(t *testing.T) {
	a, d, _ := pageHarness(t)
	first := a.workspace.activeSession()
	a.action(func() error { return a.workspace.split(splitRight) })
	want := a.workspace.activeSession()
	if want == first {
		t.Fatal("split did not select its new session")
	}
	d.Settle()
	if a.workspace.activeSession() != want {
		t.Fatal("the old terminal focus overrode the requested new pane")
	}
	a.changed(a.workspace.cyclePane)
	d.Settle()
	if a.workspace.activeSession() != first {
		t.Fatal("keyboard pane cycling did not move focus")
	}
	a.changed(a.workspace.cyclePane)
	d.Settle()
	if a.workspace.activeSession() != want {
		t.Fatal("the earlier pane stole keyboard focus back")
	}
}

func TestPageCollapseRestoresSurvivingPTYSize(t *testing.T) {
	for _, axis := range []splitAxis{splitRight, splitDown} {
		t.Run(fmt.Sprint(axis), func(t *testing.T) {
			a, d, connections := pageHarness(t)
			first := a.workspace.activeSession()
			conn := connections[first.id]
			_, full := conn.size()
			a.action(func() error { return a.workspace.split(axis) })
			d.Advance(100 * time.Millisecond)
			d.Settle()
			_, divided := conn.size()
			if divided == full {
				t.Fatal("split did not resize the first PTY")
			}
			a.changed(a.workspace.closePane)
			d.Advance(100 * time.Millisecond)
			d.Settle()
			_, restored := conn.size()
			if restored != full {
				t.Fatalf("surviving PTY remains at split size: full %v, split %v, restored %v", full, divided, restored)
			}
		})
	}
}

func TestPageSplitRatioSurvivesHiddenViewEviction(t *testing.T) {
	a, d, _ := pageHarness(t)
	a.action(func() error { return a.workspace.split(splitRight) })
	d.Settle()
	firstTab := a.workspace.activeTab()
	bar, ok := d.A11yFind(a11y.RoleSeparator, "")
	if !ok {
		t.Fatal("missing splitter")
	}
	p := geom.Pt(bar.Bounds.X+bar.Bounds.W/2, bar.Bounds.Y+bar.Bounds.H/2)
	d.Drag(p, geom.Pt(p.X-150, p.Y))
	d.Advance(100 * time.Millisecond)
	d.Settle()
	before := d.A11yFindAll(a11y.RoleTextbox, "")[0].Bounds.W
	a.action(a.workspace.newTab)
	d.Settle()
	for range 320 {
		d.Advance(16 * time.Millisecond)
		d.Frame()
	}
	a.workspace.selectTab(firstTab.id)
	a.focusPending = true
	d.Settle()
	after := d.A11yFindAll(a11y.RoleTextbox, "")[0].Bounds.W
	if math.Abs(float64(after-before)) > 2 {
		t.Fatalf("hidden tab lost split ratio: width %v -> %v", before, after)
	}
}
