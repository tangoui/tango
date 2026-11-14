package diffview

import (
	"fmt"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/tangoui/tango/internal/render/gpu"
	"github.com/tangoui/tango/internal/render/gpu/cpuexec"
	"github.com/tangoui/tango/ui"
	"github.com/tangoui/tango/ui/a11y"
	"github.com/tangoui/tango/ui/editor"
	"github.com/tangoui/tango/ui/editor/diff"
	"github.com/tangoui/tango/ui/editor/theme"
	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/input"
	"github.com/tangoui/tango/ui/text"
)

// testWindow is a ui.Window without a screen.
type testWindow struct {
	size  geom.Size
	scale float32
	clip  string
	inval int
}

func (w *testWindow) Size() geom.Size              { return w.size }
func (w *testWindow) Scale() float32               { return w.scale }
func (w *testWindow) Invalidate()                  { w.inval++ }
func (w *testWindow) InvalidateAt(time.Time)       {}
func (w *testWindow) SetCursor(input.Cursor)       {}
func (w *testWindow) ReadClipboard() string        { return w.clip }
func (w *testWindow) WriteClipboard(s string)      { w.clip = s }
func (w *testWindow) SetTextInput(bool, geom.Rect) {}
func (w *testWindow) Present(*image.RGBA) error    { return nil }

// a11yWindow is a testWindow with accessibility on; it records the
// announcements.
type a11yWindow struct {
	*testWindow
	said []string
}

func (w *a11yWindow) AccessibilityEnabled() bool { return true }
func (w *a11yWindow) AccessibilityUpdate(t *a11y.Tree, _ []a11y.NodeID) {
	for _, a := range t.Announcements {
		w.said = append(w.said, a.Text)
	}
}

// host runs a page showing a diff view through a gpu.Renderer on a
// cpuexec.Device, in strict verify mode when verify is set: every partial
// frame is compared with a full redraw, and a difference fails the frame.
type host struct {
	t      testing.TB
	d      *State
	w      *testWindow
	r      *ui.Runner
	dev    *cpuexec.Device
	vw     *ViewW
	now    time.Time
	mods   func(*ViewW)
	aw     *a11yWindow // with accessibility (newA11yHost)
	frames int
	parts  int
}

func newHost(t testing.TB, d *State, size geom.Size, scale float32, verify bool) *host {
	t.Helper()
	h := &host{t: t, d: d, w: &testWindow{size: size, scale: scale}, dev: cpuexec.NewDevice(),
		now: time.Date(2026, 11, 15, 9, 0, 0, 0, time.UTC)}
	h.dev.SampledOnly = true
	ren := gpu.NewRenderer(h.dev)
	ren.SetVerify(verify)
	ren.SetVerifyStrict(verify)
	h.r = ui.NewRunner(func(c *ui.Context) {
		h.vw = View(c, d)
		if h.mods != nil {
			h.mods(h.vw)
		}
	}, ui.RunnerOptions{Renderer: ren, NoAnimations: true, Clock: func() time.Time { return h.now }})
	h.r.Attach(h.w)
	return h
}

// newA11yHost is newHost with accessibility on.
func newA11yHost(t testing.TB, d *State, size geom.Size) *host {
	h := newHost(t, d, size, 1, false)
	h.aw = &a11yWindow{testWindow: h.w}
	h.r.Attach(h.aw)
	return h
}

// frame runs a frame; it reports whether it was drawn partially.
func (h *host) frame() bool {
	h.t.Helper()
	before := h.dev.Stats().Partial
	if err := h.r.Frame(); err != nil {
		h.t.Fatalf("frame %d: %v", h.frames, err)
	}
	h.frames++
	p := h.dev.Stats().Partial > before
	if p {
		h.parts++
	}
	return p
}

// settle waits for the running diff computation and runs frames until
// the result is shown.
func (h *host) settle() {
	h.t.Helper()
	h.d.run.wait()
	for range 3 {
		h.frame()
	}
}

func (h *host) key(k input.Key, mods input.Modifiers) {
	h.r.HandleEvent(input.KeyEvent{Key: k, Down: true, Mods: mods})
	h.r.HandleEvent(input.KeyEvent{Key: k, Mods: mods})
}

func (h *host) typeText(s string) {
	h.r.HandleEvent(input.KeyEvent{Key: input.KeyX, Down: true})
	h.r.HandleEvent(input.TextInput{Text: s})
	h.r.HandleEvent(input.KeyEvent{Key: input.KeyX})
}

func (h *host) click(p geom.Point) {
	h.r.HandleEvent(input.Pointer{Kind: input.Move, Pos: p})
	h.r.HandleEvent(input.Pointer{Kind: input.Press, Pos: p})
	h.r.HandleEvent(input.Pointer{Kind: input.Release, Pos: p})
}

// editorRect returns the rectangle of side s's editor in the last frame.
func (h *host) editorRect(s diff.Side) geom.Rect {
	e := h.vw.ed[s]
	if e == nil {
		h.t.Fatalf("side %v not shown", s)
	}
	return e.Box().Rect()
}

// wheel scrolls side s by dy pixels with the wheel.
func (h *host) wheel(s diff.Side, dy float32) {
	r := h.editorRect(s)
	h.r.HandleEvent(input.Scroll{Pos: geom.Pt(r.X+r.W/2, r.Y+r.H/2), DY: dy})
}

// lineAt clicks line (visible) of side s.
func (h *host) clickLine(s diff.Side, line int) {
	h.t.Helper()
	st := h.d.side(s)
	r := h.editorRect(s)
	y := r.Y + float32(st.LineTop(line)-st.ScrollTop()) + st.Viewport.LineHeight/2
	h.click(geom.Pt(r.X+r.W/2, y))
}

func useTestText(t testing.TB) {
	prev := text.UseSystemEngine(false)
	t.Cleanup(func() { text.UseSystemEngine(prev) })
}

// docLines returns n lines of Go-like code.
func docLines(n int) []string {
	out := make([]string, n)
	for i := range out {
		out[i] = fmt.Sprintf("func f%d(x int) string { return \"v%d\" + fmt.Sprint(x*%d) }", i, i, i)
	}
	return out
}

// modify returns lines with a change every step lines: alternately a few
// lines replaced, inserted and deleted.
func modify(lines []string, step int) []string {
	var out []string
	for i := 0; i < len(lines); i++ {
		if i%step == step/2 {
			switch (i / step) % 3 {
			case 0: // replace 2 lines by 3
				out = append(out, lines[i]+" // changed", "extra1", "extra2")
				i++
			case 1: // insert 4 lines
				out = append(out, "ins1", "ins2", "ins3", "ins4", lines[i])
			case 2: // delete 3 lines
				i += 2
			}
			continue
		}
		out = append(out, lines[i])
	}
	return out
}

func newTestDiff(t testing.TB, orig, mod []string) *State {
	t.Helper()
	om := editor.NewModel("a.go", strings.Join(orig, "\n"), editor.DefaultRegistry())
	mm := editor.NewModel("b.go", strings.Join(mod, "\n"), editor.DefaultRegistry())
	d := New(om, mm, theme.Base("vs"))
	t.Cleanup(d.Close)
	return d
}

// checkAligned verifies that every aligned block visible on both sides
// starts at the same distance from the viewports' top edges (to the
// pixel).
func checkAligned(t testing.TB, d *State, what string) {
	t.Helper()
	al := d.lay.al
	if al == nil {
		t.Fatalf("%s: no alignment", what)
	}
	gs := [2]sideGeom{d.geom(diff.Original), d.geom(diff.Modified)}
	tops := [2]float64{d.Original.ScrollTop(), d.Modified.ScrollTop()}
	h := float64(d.Modified.Viewport.Height)
	checked := 0
	for bi := range al.blocks {
		yo := al.blockTop(diff.Original, bi, gs[0]) - tops[0]
		ym := al.blockTop(diff.Modified, bi, gs[1]) - tops[1]
		ho := al.blockTop(diff.Original, bi+1, gs[0]) - tops[0] - yo
		hm := al.blockTop(diff.Modified, bi+1, gs[1]) - tops[1] - ym
		if (yo+ho <= 0 || yo > h) && (ym+hm <= 0 || ym > h) {
			continue
		}
		// A block starting above the viewport lines up only when it is as
		// tall on both sides (otherwise the shorter side is clamped).
		if yo < 0 && ym < 0 && ho != hm {
			continue
		}
		checked++
		if yo != ym {
			t.Fatalf("%s: block %d (%v | %v) at %v on the original, %v on the modified side (tops %v)", what, bi, al.blocks[bi].orig, al.blocks[bi].mod, yo, ym, tops)
		}
	}
	if checked == 0 {
		t.Fatalf("%s: no block visible", what)
	}
}

// shot saves the last frame to $DIFFVIEW_SHOTS/name.png when the
// variable is set (looking at the view while developing).
func (h *host) shot(name string) {
	dir := os.Getenv("DIFFVIEW_SHOTS")
	if dir == "" {
		return
	}
	img := h.dev.Target()
	if img == nil {
		return
	}
	f, err := os.Create(filepath.Join(dir, name+".png"))
	if err != nil {
		h.t.Fatal(err)
	}
	defer f.Close()
	if err := png.Encode(f, img); err != nil {
		h.t.Fatal(err)
	}
}
