package terminal

import (
	"bytes"
	"math"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/tangoui/tango/ui"
	"github.com/tangoui/tango/ui/a11y"
	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/input"
	"github.com/tangoui/tango/ui/layout"
	"github.com/tangoui/tango/ui/paint"
	"github.com/tangoui/tango/ui/terminal/vt"
	"github.com/tangoui/tango/ui/text"
	"github.com/tangoui/tango/ui/theme"
)

// viewRig runs a View of a Terminal on a pipeConn in a headless Driver.
// A goroutine collects everything the terminal sends to the program.
type viewRig struct {
	t    *testing.T
	conn *pipeConn
	term *Terminal
	d    *ui.Driver
	vw   *ViewW

	mu  sync.Mutex
	in  bytes.Buffer // bytes the terminal sent to the program
	end chan struct{}
}

// newViewRig builds a rig whose page is page(c, view builder); a nil page
// shows the view alone.
func newViewRig(t *testing.T, w, h float32, page func(c *ui.Context, view func())) *viewRig {
	t.Helper()
	g := &viewRig{t: t, conn: newPipeConn(), end: make(chan struct{})}
	g.term = New(Options{Conn: g.conn})
	go func() {
		defer close(g.end)
		buf := make([]byte, 4096)
		for {
			n, err := g.conn.inR.Read(buf)
			g.mu.Lock()
			g.in.Write(buf[:n])
			g.mu.Unlock()
			if err != nil {
				return
			}
		}
	}()
	view := func(c *ui.Context) { g.vw = View(c, g.term).Layout(layout.Grow(1)) }
	g.d = ui.NewDriver(func(c *ui.Context) {
		if page == nil {
			view(c)
			return
		}
		page(c, func() { view(c) })
	}, ui.DriverOptions{Width: w, Height: h})
	t.Cleanup(func() {
		g.term.Close()
		g.conn.inR.Close()
		<-g.end
	})
	g.d.Settle()
	return g
}

// input returns what the program has received so far.
func (g *viewRig) input() string {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.in.String()
}

// waitInput waits until the program has received exactly want.
func (g *viewRig) waitInput(want string) {
	g.t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for g.input() != want {
		if time.Now().After(deadline) {
			g.t.Fatalf("program input = %q, want %q", g.input(), want)
		}
		time.Sleep(time.Millisecond)
	}
}

// output writes program output and waits until the emulator satisfies
// cond, then runs a frame.
func (g *viewRig) output(s string, cond func(v *vt.Terminal) bool) {
	g.t.Helper()
	if _, err := g.conn.outW.Write([]byte(s)); err != nil {
		g.t.Fatal(err)
	}
	waitFor(g.t, 5*time.Second, "output "+strings.TrimSpace(s), func() bool {
		v := g.term.Lock()
		defer g.term.Unlock()
		return cond(v)
	})
	g.d.Frame()
}

// focus clicks the view, which focuses it, and settles.
func (g *viewRig) focus() {
	g.t.Helper()
	g.d.Click(g.vw.Box().Rect().Center())
	g.d.Settle()
	if g.d.Runner().Context().Focused() != g.vw.Box().ID() {
		g.t.Fatal("view not focused")
	}
}

// cell returns the cell size of the default font size.
func cellSize() (w, h float32) {
	st := text.Style{Size: defaultFontSize, Mono: true}
	cw := float32(math.Ceil(float64(text.Width("M", st))))
	return cw, float32(math.Round(defaultFontSize * 1.2))
}

// shapedIDs returns the glyph IDs of s in the view's text style.
func shapedIDs(s string) []uint32 {
	_, lh := cellSize()
	lay := text.Shape(s, text.Style{Size: defaultFontSize, LineHeight: lh, Mono: true, Weight: text.Regular}, text.Options{NoWrap: true, Direction: text.DirLTR})
	ids := make([]uint32, len(lay.Glyphs))
	for i, gl := range lay.Glyphs {
		ids[i] = gl.ID
	}
	return ids
}

// findText returns the Text ops of the last frame that draw exactly s.
func findText(l *paint.List, s string) []paint.Text {
	want := shapedIDs(s)
	var out []paint.Text
	for _, op := range l.Ops {
		t, ok := op.(paint.Text)
		if !ok || len(t.Glyphs) != len(want) {
			continue
		}
		ids := make([]uint32, len(t.Glyphs))
		for i, gl := range t.Glyphs {
			ids[i] = gl.ID
		}
		if slices.Equal(ids, want) {
			out = append(out, t)
		}
	}
	return out
}

func lastResize(c *pipeConn) [4]int {
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.resizes) == 0 {
		return [4]int{}
	}
	return c.resizes[len(c.resizes)-1]
}

func TestViewSizeToCells(t *testing.T) {
	g := newViewRig(t, 400, 200, nil)
	cw, lh := cellSize()
	wantCols, wantRows := int(400/cw), int(200/lh)
	got := lastResize(g.conn)
	if got[0] != wantCols || got[1] != wantRows {
		t.Fatalf("Resize = %v, want %d cols × %d rows", got, wantCols, wantRows)
	}
	if got[2] <= 0 || got[3] <= 0 {
		t.Errorf("Resize pixel size = %d×%d, want the real size", got[2], got[3])
	}
	v := g.term.Lock()
	cols, rows := v.Screen().Size()
	g.term.Unlock()
	if cols != wantCols || rows != wantRows {
		t.Errorf("emulator size = %d×%d, want %d×%d", cols, rows, wantCols, wantRows)
	}
}

func TestViewResizeThrottled(t *testing.T) {
	g := newViewRig(t, 400, 200, nil)
	cw, _ := cellSize()
	g.conn.mu.Lock()
	n := len(g.conn.resizes)
	g.conn.mu.Unlock()
	g.d.Resize(300, 200)
	g.d.Frame()
	g.d.Resize(250, 200)
	g.d.Frame()
	g.conn.mu.Lock()
	extra := len(g.conn.resizes) - n
	g.conn.mu.Unlock()
	if extra > 1 {
		t.Fatalf("%d resizes within the throttle window, want at most 1", extra)
	}
	g.d.Advance(60 * time.Millisecond)
	g.d.Frame()
	if got := lastResize(g.conn); got[0] != int(250/cw) {
		t.Fatalf("after the throttle Resize = %v, want %d cols", got, int(250/cw))
	}
}

func TestViewPaintsText(t *testing.T) {
	g := newViewRig(t, 400, 200, nil)
	g.output("\x1b[31mred\x1b[0m ok", func(v *vt.Terminal) bool { return rowText(v, 0) == "red ok" })
	tok := g.d.Runner().Context().ThemeFor(theme.CompTerminal).Components.Terminal
	ops := findText(g.d.Runner().List(), "red")
	if len(ops) == 0 {
		t.Fatal("no text op draws \"red\"")
	}
	if ops[0].Color != tok.Palette[1] {
		t.Errorf("\"red\" color = %v, want palette red %v", ops[0].Color, tok.Palette[1])
	}
	if len(findText(g.d.Runner().List(), "ok")) == 0 {
		t.Error("no text op draws \"ok\"")
	}
}

func TestViewTypingSends(t *testing.T) {
	g := newViewRig(t, 400, 200, nil)
	g.focus()
	g.d.Type("ls")
	g.d.Key(input.KeyEnter, 0)
	g.waitInput("ls\r")
}

func TestViewEscInsideModal(t *testing.T) {
	open := true
	g := newViewRig(t, 800, 600, func(c *ui.Context, view func()) {
		ui.Modal(c, &open, "Terminal").Body(func() {
			ui.Div(c, layout.Size(400, 200)).Body(view)
		})
	})
	g.focus()
	g.d.Key(input.KeyEscape, 0)
	g.waitInput("\x1b")
	if !open {
		t.Fatal("Esc closed the modal")
	}
}

func TestViewCtrlTabLeaves(t *testing.T) {
	g := newViewRig(t, 400, 300, func(c *ui.Context, view func()) {
		ui.Column(c, func() {
			ui.Div(c, layout.Size(400, 200)).Body(view)
			ui.Button(c, "Next")
		})
	})
	g.focus()
	g.d.Key(input.KeyTab, 0)
	g.waitInput("\t")
	g.d.Key(input.KeyTab, input.ModCtrl)
	if g.d.Runner().Context().Focused() == g.vw.Box().ID() {
		t.Fatal("Ctrl+Tab left the focus on the terminal")
	}
	if got := g.input(); got != "\t" {
		t.Errorf("program input = %q, want only the Tab", got)
	}
}

// copyMods is the platform's copy / paste shortcut modifier set.
func copyMods() input.Modifiers {
	if runtime.GOOS == "darwin" {
		return input.ModMeta
	}
	return input.ModCtrl | input.ModShift
}

func TestViewSelectCopy(t *testing.T) {
	g := newViewRig(t, 400, 200, nil)
	v := g.term.Lock()
	cols, _ := v.Screen().Size()
	g.term.Unlock()
	first := strings.Repeat("x", cols-1) + "y"
	g.output(first+"tail", func(v *vt.Terminal) bool { return rowText(v, 1) == "tail" })
	g.focus()
	r := g.vw.Box().Rect()
	cw, lh := cellSize()
	g.d.Drag(geom.Pt(r.X+1, r.Y+lh/2), geom.Pt(r.X+4*cw+1, r.Y+lh*1.5))
	g.d.Key(input.KeyC, copyMods())
	if got, want := g.d.Clipboard(), first+"tail"; got != want {
		t.Fatalf("clipboard = %q, want %q", got, want)
	}
	if got := g.input(); got != "" {
		t.Errorf("copy sent %q to the program", got)
	}
}

func TestViewPasteBracketed(t *testing.T) {
	g := newViewRig(t, 400, 200, nil)
	g.output("\x1b[?2004h", func(v *vt.Terminal) bool { return v.Modes().BracketedPaste })
	g.focus()
	g.d.SetClipboard("a\nb")
	g.d.Key(input.KeyV, copyMods())
	g.waitInput("\x1b[200~a\rb\x1b[201~")
}

func TestViewDamageOnlyDirtyRows(t *testing.T) {
	g := newViewRig(t, 400, 200, nil)
	g.output("one\r\ntwo\r\n", func(v *vt.Terminal) bool { return rowText(v, 1) == "two" })
	g.d.Settle()
	g.d.Frame()
	g.output("x", func(v *vt.Terminal) bool { return rowText(v, 2) == "x" })
	_, lh := cellSize()
	r := g.vw.Box().Rect()
	row := geom.Rect{X: r.X, Y: r.Y + 2*lh, W: r.W, H: lh}
	var dam []geom.Rect
	for _, op := range g.d.Runner().List().Ops {
		switch op := op.(type) {
		case paint.Damage:
			dam = append(dam, op.Rect)
		case paint.ScrollRegion:
			t.Errorf("unexpected scroll region %v", op)
		}
	}
	if len(dam) == 0 {
		t.Fatal("frame declared no damage (full redraw)")
	}
	for _, d := range dam {
		if d.Y < row.Y-0.5 || d.Bottom() > row.Bottom()+0.5 {
			t.Errorf("damage %v outside the changed row %v", d, row)
		}
	}
}

func TestViewIMEComposition(t *testing.T) {
	g := newViewRig(t, 400, 200, nil)
	g.focus()
	g.d.Compose("zhong", 5)
	g.d.Frame()
	if len(findText(g.d.Runner().List(), "zhong")) == 0 {
		t.Error("composition not drawn")
	}
	if on, _ := g.d.TextInputActive(); !on {
		t.Error("text input not requested while focused")
	}
	time.Sleep(20 * time.Millisecond)
	if got := g.input(); got != "" {
		t.Fatalf("composition sent %q", got)
	}
	g.d.Type("中")
	g.waitInput("中")
}

func TestViewExitMessage(t *testing.T) {
	g := newViewRig(t, 400, 200, nil)
	g.conn.outW.Close()
	waitFor(t, 5*time.Second, "exit", func() bool { ok, _ := g.term.Exited(); return ok })
	g.d.Frame()
	if len(findText(g.d.Runner().List(), exitedText)) == 0 {
		t.Fatal("exit message not drawn")
	}
}

// inject delivers events in one frame and settles.
func (g *viewRig) inject(evs ...input.Event) {
	for _, e := range evs {
		g.d.Inject(e)
	}
	g.d.Settle()
}

func TestViewCtrlKeyDropsHostText(t *testing.T) {
	g := newViewRig(t, 400, 200, nil)
	g.focus()
	// Windows delivers both the key and its text: Ctrl+C sends ^C once.
	g.inject(input.KeyEvent{Key: input.KeyC, Down: true, Mods: input.ModCtrl}, input.TextInput{Text: "c"},
		input.KeyEvent{Key: input.KeyC, Mods: input.ModCtrl})
	g.waitInput("\x03")
	// A plain key's text is sent once (EncodeKey leaves printable keys
	// to the text).
	g.inject(input.KeyEvent{Key: input.KeyA, Down: true}, input.TextInput{Text: "a"}, input.KeyEvent{Key: input.KeyA})
	g.waitInput("\x03a")
}

func TestViewAltGrPrefersText(t *testing.T) {
	g := newViewRig(t, 400, 200, nil)
	g.focus()
	// AltGr+Q on a German layout: Ctrl+Alt+Q followed by "@".
	g.inject(input.KeyEvent{Key: input.KeyQ, Down: true, Mods: input.ModCtrl | input.ModAlt}, input.TextInput{Text: "@"},
		input.KeyEvent{Key: input.KeyQ, Mods: input.ModCtrl | input.ModAlt})
	g.waitInput("@")
	// Without text the combination is encoded.
	g.inject(input.KeyEvent{Key: input.KeyQ, Down: true, Mods: input.ModCtrl | input.ModAlt}, input.KeyEvent{Key: input.KeyQ, Mods: input.ModCtrl | input.ModAlt})
	want := "@\x1b\x11"
	if runtime.GOOS == "darwin" {
		want = "@\x11" // Option composes: Alt is dropped (OptionAsMeta off)
	}
	g.waitInput(want)
}

func TestViewOptionAsMeta(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("Option is macOS only")
	}
	meta := false
	g := newViewRig(t, 400, 200, nil)
	g.d = ui.NewDriver(func(c *ui.Context) {
		g.vw = View(c, g.term).OptionAsMeta(meta)
	}, ui.DriverOptions{Width: 400, Height: 200})
	g.d.Settle()
	g.focus()
	g.d.Key(input.KeyB, input.ModAlt) // Option composes: nothing encoded
	meta = true
	g.d.Settle()
	g.d.Key(input.KeyB, input.ModAlt)
	g.waitInput("\x1bb")
}

func TestViewKeysDuringCompositionGoToIME(t *testing.T) {
	g := newViewRig(t, 400, 200, nil)
	g.focus()
	g.d.Compose("ni", 2)
	g.d.Key(input.KeyBackspace, 0)
	g.d.Key(input.KeyEnter, 0)
	time.Sleep(20 * time.Millisecond)
	if got := g.input(); got != "" {
		t.Fatalf("keys during composition sent %q", got)
	}
	g.d.Type("你")
	g.waitInput("你")
	g.d.Key(input.KeyEnter, 0)
	g.waitInput("你\r")
}

// fill writes n numbered lines and waits until the last is on screen.
func (g *viewRig) fill(n int) {
	g.t.Helper()
	var sb strings.Builder
	for i := range n {
		sb.WriteString("line ")
		sb.WriteString(itoaTest(i))
		sb.WriteString("\r\n")
	}
	want := "line " + itoaTest(n-1)
	g.output(sb.String(), func(v *vt.Terminal) bool {
		_, rows := v.Screen().Size()
		return rowText(v, rows-2) == want
	})
}

func itoaTest(n int) string { return strconv.Itoa(n) }

func viewOffset(g *viewRig) int {
	v := g.term.Lock()
	defer g.term.Unlock()
	return v.Screen().ViewOffset()
}

func TestViewScrollback(t *testing.T) {
	g := newViewRig(t, 400, 200, nil)
	g.fill(40)
	g.focus()
	_, lh := cellSize()
	g.d.Scroll(g.vw.Box().Rect().Center(), -3*lh)
	if got := viewOffset(g); got != 3 {
		t.Fatalf("wheel up 3 lines: offset %d", got)
	}
	g.d.Key(input.KeyPageUp, input.ModShift)
	v := g.term.Lock()
	_, rows := v.Screen().Size()
	g.term.Unlock()
	if got := viewOffset(g); got != 3+rows-1 {
		t.Fatalf("Shift+PageUp: offset %d, want %d", got, 3+rows-1)
	}
	if got := g.input(); got != "" {
		t.Fatalf("scrolling sent %q", got)
	}
	// Typing returns to the bottom.
	g.d.Type("x")
	g.waitInput("x")
	if got := viewOffset(g); got != 0 {
		t.Fatalf("after typing offset %d, want 0", got)
	}
}

func TestViewScrollShift(t *testing.T) {
	g := newViewRig(t, 400, 200, nil)
	g.fill(30)
	g.d.Settle()
	g.d.Frame()
	g.output("more\r\n", func(v *vt.Terminal) bool {
		_, rows := v.Screen().Size()
		return rowText(v, rows-2) == "more"
	})
	_, lh := cellSize()
	var sc []paint.ScrollRegion
	for _, op := range g.d.Runner().List().Ops {
		if s, ok := op.(paint.ScrollRegion); ok {
			sc = append(sc, s)
		}
	}
	if len(sc) != 1 || sc[0].Delta != geom.Pt(0, -lh) {
		t.Fatalf("scroll regions %v, want one shifting up a row (%v)", sc, lh)
	}
}

func TestViewDoubleClickWord(t *testing.T) {
	g := newViewRig(t, 400, 200, nil)
	g.output("echo (hello) world", func(v *vt.Terminal) bool { return rowText(v, 0) == "echo (hello) world" })
	g.focus()
	r := g.vw.Box().Rect()
	cw, lh := cellSize()
	p := geom.Pt(r.X+7.5*cw, r.Y+lh/2) // on "hello"
	g.d.Advance(time.Second)           // not a continuation of the focusing click
	g.inject(input.Pointer{Kind: input.Move, Pos: p}, input.Pointer{Kind: input.Press, Pos: p}, input.Pointer{Kind: input.Release, Pos: p})
	g.inject(input.Pointer{Kind: input.Press, Pos: p}, input.Pointer{Kind: input.Release, Pos: p})
	g.d.Key(input.KeyC, copyMods())
	if got := g.d.Clipboard(); got != "hello" {
		t.Fatalf("double click copied %q, want \"hello\"", got)
	}
	g.inject(input.Pointer{Kind: input.Press, Pos: p}, input.Pointer{Kind: input.Release, Pos: p})
	g.d.Key(input.KeyC, copyMods())
	if got := g.d.Clipboard(); got != "echo (hello) world" {
		t.Fatalf("triple click copied %q", got)
	}
}

func TestViewSelectionFollowsScroll(t *testing.T) {
	g := newViewRig(t, 400, 200, nil)
	g.output("first\r\nsecond", func(v *vt.Terminal) bool { return rowText(v, 1) == "second" })
	g.focus()
	r := g.vw.Box().Rect()
	cw, lh := cellSize()
	g.d.Drag(geom.Pt(r.X+1, r.Y+lh/2), geom.Pt(r.X+5*cw+1, r.Y+lh/2))
	g.fill(30) // "first" scrolls into the scrollback
	g.d.Key(input.KeyC, copyMods())
	if got := g.d.Clipboard(); got != "first" {
		t.Fatalf("copied %q after scrolling, want \"first\"", got)
	}
}

func TestViewSelectAllAndClear(t *testing.T) {
	g := newViewRig(t, 400, 200, nil)
	g.output("a\r\nb\r\n$ ", func(v *vt.Terminal) bool { return rowText(v, 2) == "$" })
	g.focus()
	g.d.Key(input.KeyA, copyMods())
	g.d.Key(input.KeyC, copyMods())
	if got := g.d.Clipboard(); got != "a\nb\n$" && got != "a\r\nb\r\n$" {
		t.Fatalf("select all copied %q", got)
	}
	g.d.Key(input.KeyK, copyMods())
	v := g.term.Lock()
	top, sb := rowText(v, 0), v.Screen().ScrollbackLen()
	cx, cy, _ := v.Cursor()
	g.term.Unlock()
	if top != "$" || sb != 0 || cx != 2 || cy != 0 {
		t.Fatalf("after clear: row 0 %q, scrollback %d, cursor %d,%d; want \"$\", 0, 2,0", top, sb, cx, cy)
	}
	if got := g.input(); got != "" {
		t.Errorf("shortcuts sent %q", got)
	}
}

func TestViewWideAndAttrs(t *testing.T) {
	g := newViewRig(t, 400, 200, nil)
	g.output("中\x1b[7minv\x1b[0m\x1b[8mhid\x1b[0m", func(v *vt.Terminal) bool { return rowText(v, 0) == "中invhid" })
	l := g.d.Runner().List()
	tok := g.d.Runner().Context().ThemeFor(theme.CompTerminal).Components.Terminal
	cw, _ := cellSize()
	wide := findText(l, "中")
	if len(wide) == 0 {
		t.Fatal("wide character not drawn")
	}
	if x := wide[0].Origin.X + wide[0].Glyphs[0].X; x < g.vw.Box().Rect().X || x > g.vw.Box().Rect().X+cw {
		t.Errorf("wide character at x %v, want centered in the first two cells", x)
	}
	inv := findText(l, "inv")
	if len(inv) == 0 || inv[0].Color != tok.Background {
		t.Errorf("inverse text %v, want the background color %v", inv, tok.Background)
	}
	if len(findText(l, "hid")) != 0 {
		t.Error("invisible text drawn")
	}
}

func TestViewSchemeOverride(t *testing.T) {
	g := newViewRig(t, 400, 200, nil)
	g.d = ui.NewDriver(func(c *ui.Context) {
		g.vw = View(c, g.term).Scheme(SchemeDracula)
	}, ui.DriverOptions{Width: 400, Height: 200})
	g.d.Settle()
	g.output("\x1b[31mred", func(v *vt.Terminal) bool { return rowText(v, 0) == "red" })
	ops := findText(g.d.Runner().List(), "red")
	if len(ops) == 0 || ops[0].Color != SchemeDracula.Palette[1] {
		t.Fatalf("red with Dracula: %v, want %v", ops, SchemeDracula.Palette[1])
	}
}

func TestViewA11y(t *testing.T) {
	g := newViewRig(t, 400, 200, nil)
	g.d.EnableAccessibility()
	g.output("hello", func(v *vt.Terminal) bool { return rowText(v, 0) == "hello" })
	g.d.Frame()
	n, ok := g.d.A11yFind(a11y.RoleTextbox, "Terminal")
	if !ok {
		t.Fatal("no Terminal textbox")
	}
	if n.States&(a11y.Multiline|a11y.ReadOnly) != a11y.Multiline|a11y.ReadOnly {
		t.Errorf("states %v, want multiline and read-only", n.States)
	}
	if n.Value != "hello" {
		t.Errorf("value %q, want \"hello\"", n.Value)
	}
}

func TestTrackEviction(t *testing.T) {
	v := vt.New(vt.Options{Cols: 10, Rows: 3, Scrollback: 2})
	var vs viewState
	v.Write([]byte("a\r\nb\r\nc\r\nd")) // "a" scrolls into the scrollback
	vs.trackEviction(v.Screen())
	if vs.evicted != 0 {
		t.Fatalf("growing scrollback: evicted %d", vs.evicted)
	}
	vs.sel = selection{anchor: selPos{1, 0}, head: selPos{1, 1}, on: true} // "b"
	v.Write([]byte("\r\ne\r\nf"))                                          // scrollback full: "a" is dropped
	vs.trackEviction(v.Screen())
	if vs.evicted != 1 {
		t.Fatalf("evicted %d, want 1", vs.evicted)
	}
	var w ViewW
	w.vs = &vs
	if got := w.selectedText(v.Screen()); got != "b" {
		t.Fatalf("selection %q after eviction, want \"b\"", got)
	}
	v.Write([]byte("\r\ng\r\nh")) // "b" is dropped too
	vs.trackEviction(v.Screen())
	if vs.sel.on {
		t.Error("a selection whose lines were all dropped was kept")
	}
}

// newSmallRig is a rig whose Terminal keeps only sb scrollback lines.
func newSmallRig(t *testing.T, sb int) *viewRig {
	g := newViewRig(t, 400, 200, nil)
	g.term.Close()
	g.conn.inR.Close()
	<-g.end
	g2 := &viewRig{t: t, conn: newPipeConn(), end: make(chan struct{})}
	g2.term = New(Options{Conn: g2.conn, Scrollback: sb})
	go func() {
		defer close(g2.end)
		buf := make([]byte, 4096)
		for {
			n, err := g2.conn.inR.Read(buf)
			g2.mu.Lock()
			g2.in.Write(buf[:n])
			g2.mu.Unlock()
			if err != nil {
				return
			}
		}
	}()
	g2.d = ui.NewDriver(func(c *ui.Context) { g2.vw = View(c, g2.term) }, ui.DriverOptions{Width: 400, Height: 200})
	t.Cleanup(func() {
		g2.term.Close()
		g2.conn.inR.Close()
		<-g2.end
	})
	g2.d.Settle()
	return g2
}

// selectionFills returns the selection fills of the last frame.
func selectionFills(g *viewRig, sel theme.Color) []geom.Rect {
	var out []geom.Rect
	for _, op := range g.d.Runner().List().Ops {
		if f, ok := op.(paint.FillRect); ok && f.Color == sel {
			out = append(out, f.Rect)
		}
	}
	return out
}

// TestViewSelectionWhileScrolledUpAndEvicting: with the view scrolled up
// and the scrollback full, new output drops the oldest lines while the
// view keeps showing the same text; the selection stays on it (highlight
// and copied text), and is cleared once its lines are dropped.
func TestViewSelectionWhileScrolledUpAndEvicting(t *testing.T) {
	g := newSmallRig(t, 20)
	g.fill(40) // scrollback full
	g.focus()
	_, lh := cellSize()
	g.d.Scroll(g.vw.Box().Rect().Center(), -10*lh)
	if viewOffset(g) != 10 {
		t.Fatalf("offset %d, want 10", viewOffset(g))
	}
	v := g.term.Lock()
	top := viewRowText(v, 0)
	g.term.Unlock()
	r := g.vw.Box().Rect()
	cw, _ := cellSize()
	g.d.Advance(time.Second)
	g.d.Drag(geom.Pt(r.X+1, r.Y+lh/2), geom.Pt(r.X+float32(len(top))*cw+1, r.Y+lh/2))
	sel := g.d.Runner().Context().ThemeFor(theme.CompTerminal).Components.Terminal.Selection
	before := selectionFills(g, sel)
	if len(before) != 1 {
		t.Fatalf("selection fills %v", before)
	}
	// Three more lines: the viewport keeps its text (offset 13).
	g.output("x1\r\nx2\r\nx3\r\n", func(v *vt.Terminal) bool { return v.Screen().ViewOffset() == 13 })
	g.d.Settle()
	v = g.term.Lock()
	still := viewRowText(v, 0)
	g.term.Unlock()
	if still != top {
		t.Fatalf("viewport top %q, want %q", still, top)
	}
	if after := selectionFills(g, sel); len(after) != 1 || after[0] != before[0] {
		t.Fatalf("highlight moved: %v, want %v", after, before)
	}
	g.d.Key(input.KeyC, copyMods())
	if got := g.d.Clipboard(); got != top {
		t.Fatalf("copied %q, want %q", got, top)
	}
	// Enough output to drop the selected line: the selection goes.
	g.fill(30)
	g.d.SetClipboard("")
	g.d.Key(input.KeyC, copyMods())
	if got := g.d.Clipboard(); got != "" {
		t.Fatalf("copied %q after the selected line was dropped", got)
	}
}

// viewRowText returns the text of viewport row y, trailing blanks trimmed.
func viewRowText(v *vt.Terminal, y int) string {
	var sb strings.Builder
	for _, c := range v.Screen().ViewLine(y).Cells {
		switch {
		case c.Width == 0:
		case c.Rune == 0:
			sb.WriteByte(' ')
		default:
			sb.WriteRune(c.Rune)
		}
	}
	return strings.TrimRight(sb.String(), " ")
}

func TestViewPasteDoesNotBlock(t *testing.T) {
	g := newViewRig(t, 400, 200, nil)
	g.focus()
	// Nobody reads the program's input during the paste.
	g.mu.Lock()
	big := strings.Repeat("p", 64<<10)
	g.d.SetClipboard(big)
	start := time.Now()
	g.d.Key(input.KeyV, copyMods())
	d := time.Since(start)
	g.mu.Unlock()
	if d > 2*time.Second {
		t.Fatalf("paste frame took %v", d)
	}
	g.waitInput(big)
}

// TestViewClearThenSelectAll: Cmd+K then Cmd+A in one frame selects the
// cleared screen, and the selection survives the next frame.
func TestViewClearThenSelectAll(t *testing.T) {
	g := newViewRig(t, 400, 200, nil)
	g.fill(30)
	g.output("$ ", func(v *vt.Terminal) bool {
		_, rows := v.Screen().Size()
		return rowText(v, rows-1) == "$"
	})
	g.focus()
	m := copyMods()
	g.inject(input.KeyEvent{Key: input.KeyK, Down: true, Mods: m}, input.KeyEvent{Key: input.KeyK, Mods: m},
		input.KeyEvent{Key: input.KeyA, Down: true, Mods: m}, input.KeyEvent{Key: input.KeyA, Mods: m})
	g.d.Frame()
	g.d.Key(input.KeyC, m)
	if got := g.d.Clipboard(); got != "$" {
		t.Fatalf("copied %q after Cmd+K, Cmd+A, want \"$\"", got)
	}
}
