package ui

import (
	"fmt"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"time"

	"github.com/tangoui/tango/ui/a11y"
	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/input"
	"github.com/tangoui/tango/ui/theme"
)

// Driver runs a page without a native window: inject events, run frames and
// read back the rendered *image.RGBA. It is used by tests, screenshot tools
// and the ui-gallery example. Time is simulated: it only advances through
// Advance / Settle, so output is deterministic.
type Driver struct {
	_ [0]func()

	r   *Runner
	w   *driverWindow
	img *image.RGBA
}

// DriverOptions configure a Driver.
type DriverOptions struct {
	_ [0]func()

	Width, Height float32 // logical size, default 1024×768
	Scale         float32 // default 1
	Theme         *theme.Theme
	Animations    bool // run transitions (default: jump to end state)
	ReduceMotion  bool // RunnerOptions.ReduceMotion (use with Animations)
	Accessibility bool // build the semantics tree from the first frame
}

type driverWindow struct {
	size      geom.Size
	scale     float32
	now       time.Time
	cursor    input.Cursor
	clip      string
	textOn    bool
	caret     geom.Rect
	invalid   atomic.Bool // set from any goroutine (Window.Invalidate)
	wake      time.Time
	presented int
	back      bool // BackWindow state (back.go)

	a11yOn      bool // AccessibilityHost: EnableAccessibility turns it on
	a11yUpdates int
	a11yChanged []a11y.NodeID
}

func (w *driverWindow) AccessibilityEnabled() bool { return w.a11yOn }
func (w *driverWindow) AccessibilityUpdate(t *a11y.Tree, changed []a11y.NodeID) {
	w.a11yUpdates++
	w.a11yChanged = append(w.a11yChanged[:0], changed...)
}

func (w *driverWindow) Size() geom.Size               { return w.size }
func (w *driverWindow) Scale() float32                { return w.scale }
func (w *driverWindow) Invalidate()                   { w.invalid.Store(true) }
func (w *driverWindow) InvalidateAt(t time.Time)      { w.wake = t }
func (w *driverWindow) SetCursor(c input.Cursor)      { w.cursor = c }
func (w *driverWindow) ReadClipboard() string         { return w.clip }
func (w *driverWindow) WriteClipboard(s string)       { w.clip = s }
func (w *driverWindow) Present(img *image.RGBA) error { w.presented++; return nil }
func (w *driverWindow) SetBackHandling(on bool)       { w.back = on }
func (w *driverWindow) SetTextInput(on bool, caret geom.Rect) {
	w.textOn, w.caret = on, caret
}

// NewDriver creates a headless driver for page.
func NewDriver(page func(*Context), o DriverOptions) *Driver {
	if o.Width == 0 {
		o.Width = 1024
	}
	if o.Height == 0 {
		o.Height = 768
	}
	if o.Scale == 0 {
		o.Scale = 1
	}
	w := &driverWindow{size: geom.Size{W: o.Width, H: o.Height}, scale: o.Scale, now: time.Date(2026, 1, 1, 9, 0, 0, 0, time.UTC)}
	d := &Driver{w: w}
	d.r = NewRunner(page, RunnerOptions{Theme: o.Theme, Clock: func() time.Time { return w.now }, NoAnimations: !o.Animations, ReduceMotion: o.ReduceMotion})
	d.r.Attach(w)
	w.a11yOn = o.Accessibility
	return d
}

// Runner returns the underlying runner.
func (d *Driver) Runner() *Runner { return d.r }

// SetTheme switches theme.
func (d *Driver) SetTheme(t *theme.Theme) { d.r.SetTheme(t) }

// Resize changes the logical window size.
func (d *Driver) Resize(w, h float32) { d.w.size = geom.Size{W: w, H: h} }

// Frame runs one frame and returns the image.
func (d *Driver) Frame() *image.RGBA {
	d.w.invalid.Store(false)
	if err := d.r.Frame(); err != nil {
		panic(err)
	}
	if d.r.cpu != nil {
		d.img = d.r.cpu.Image()
	}
	return d.img
}

// Settle runs frames, advancing simulated time 16ms per frame, until nothing
// animates (at most 2 simulated seconds).
func (d *Driver) Settle() *image.RGBA {
	d.Frame()
	for i := 0; i < 125 && d.r.Animating(); i++ {
		d.Advance(16 * time.Millisecond)
		d.Frame()
	}
	return d.img
}

// Advance moves simulated time forward.
func (d *Driver) Advance(dt time.Duration) { d.w.now = d.w.now.Add(dt) }

// Now returns simulated time.
func (d *Driver) Now() time.Time { return d.w.now }

// Image returns the last frame.
func (d *Driver) Image() *image.RGBA { return d.img }

// Inject queues an event (processed on the next Frame).
func (d *Driver) Inject(e input.Event) { d.r.HandleEvent(e) }

// Move moves the pointer and runs a frame.
func (d *Driver) Move(p geom.Point) {
	d.Inject(input.Pointer{Kind: input.Move, Pos: p})
	d.Frame()
}

// Click moves, presses and releases at p, then settles.
func (d *Driver) Click(p geom.Point) {
	d.Inject(input.Pointer{Kind: input.Move, Pos: p})
	d.Inject(input.Pointer{Kind: input.Press, Pos: p})
	d.Inject(input.Pointer{Kind: input.Release, Pos: p})
	d.Settle()
}

// ClickLabel clicks the center of the first widget whose label is text.
func (d *Driver) ClickLabel(text string) error {
	r, ok := d.Find(text)
	if !ok {
		return fmt.Errorf("ui: no widget labelled %q", text)
	}
	d.Click(r.Center())
	return nil
}

// RightClick presses and releases the secondary button at p (context
// menus), then settles.
func (d *Driver) RightClick(p geom.Point) {
	d.Inject(input.Pointer{Kind: input.Move, Pos: p})
	d.Inject(input.Pointer{Kind: input.Press, Pos: p, Button: input.ButtonRight})
	d.Inject(input.Pointer{Kind: input.Release, Pos: p, Button: input.ButtonRight})
	d.Settle()
}

// Type delivers committed text to the focused widget.
func (d *Driver) Type(s string) {
	d.Inject(input.TextInput{Text: s})
	d.Settle()
}

// Compose sets the IME composition string.
func (d *Driver) Compose(s string, cursor int) {
	d.Inject(input.Composition{Text: s, Cursor: cursor})
	d.Settle()
}

// Key presses and releases a key.
func (d *Driver) Key(k input.Key, mods input.Modifiers) {
	d.Inject(input.KeyEvent{Key: k, Down: true, Mods: mods})
	d.Inject(input.KeyEvent{Key: k, Down: false, Mods: mods})
	d.Settle()
}

// Scroll scrolls at p by dy.
func (d *Driver) Scroll(p geom.Point, dy float32) {
	d.Inject(input.Scroll{Pos: p, DY: dy})
	d.Settle()
}

// Find returns the rect of the top-most interactive widget with label.
func (d *Driver) Find(label string) (geom.Rect, bool) {
	hs := d.r.c.hits
	for i := len(hs) - 1; i >= 0; i-- {
		if hs[i].label == label {
			return hs[i].rect, true
		}
	}
	return geom.Rect{}, false
}

// Drag presses at from, moves to to in a few steps (one frame each) and
// releases there, then settles.
func (d *Driver) Drag(from, to geom.Point) {
	d.Inject(input.Pointer{Kind: input.Move, Pos: from})
	d.Inject(input.Pointer{Kind: input.Press, Pos: from})
	d.Frame()
	const steps = 4
	for i := 1; i <= steps; i++ {
		f := float32(i) / steps
		d.Inject(input.Pointer{Kind: input.Move, Pos: geom.Pt(from.X+(to.X-from.X)*f, from.Y+(to.Y-from.Y)*f)})
		d.Frame()
	}
	d.Inject(input.Pointer{Kind: input.Release, Pos: to})
	d.Settle()
}

// FindText returns the rect of a node labelled text in the last frame,
// including non-interactive ones (Text, Title, Paragraph, labels inside
// widgets) that Find does not see. Layers are searched top-most first. An
// exact label match wins; otherwise the first label containing text is
// used.
func (d *Driver) FindText(text string) (geom.Rect, bool) {
	c := d.r.c
	if c.root == nil {
		return geom.Rect{}, false
	}
	roots := make([]*node, 0, len(c.overlays)+1)
	for i := len(c.overlays) - 1; i >= 0; i-- {
		roots = append(roots, c.overlays[i].root)
	}
	roots = append(roots, c.root)
	for _, exact := range []bool{true, false} {
		for _, r := range roots {
			if n := findLabel(r, text, exact); n != nil {
				return n.abs, true
			}
		}
	}
	return geom.Rect{}, false
}

func findLabel(n *node, s string, exact bool) *node {
	if !n.abs.Empty() && n.label != "" && (n.label == s || !exact && strings.Contains(n.label, s)) {
		return n
	}
	for _, k := range n.kids {
		if m := findLabel(k, s, exact); m != nil {
			return m
		}
	}
	return nil
}

// ContentSize returns the extent of the page's top-level widgets in the
// last frame (useful to size screenshots to their content).
func (d *Driver) ContentSize() geom.Size { return d.r.c.contentSize }

// Cursor returns the cursor last requested by the UI.
func (d *Driver) Cursor() input.Cursor { return d.w.cursor }

// Clipboard returns the fake clipboard.
func (d *Driver) Clipboard() string { return d.w.clip }

// SetClipboard sets the fake clipboard.
func (d *Driver) SetClipboard(s string) { d.w.clip = s }

// TextInputActive reports whether the UI requested IME input and the caret.
func (d *Driver) TextInputActive() (bool, geom.Rect) { return d.w.textOn, d.w.caret }

// Presented returns how many frames were presented.
func (d *Driver) Presented() int { return d.w.presented }

// BackHandling reports what the runner last told the window through
// BackWindow: whether the system Back action has something to close.
func (d *Driver) BackHandling() bool { return d.w.back }

// NeedsFrame reports whether the UI asked for another frame.
func (d *Driver) NeedsFrame() bool { return d.w.invalid.Load() || d.r.Animating() }

// SavePNG writes the last frame to path, creating directories.
func (d *Driver) SavePNG(path string) error {
	if d.img == nil {
		d.Frame()
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	return png.Encode(f, d.img)
}

// ---------------------------------------------------------------------------
// Accessibility

// EnableAccessibility turns the semantics tree on (the fake window acts as
// an AccessibilityHost that asked for it), settles and returns the tree.
func (d *Driver) EnableAccessibility() *a11y.Tree {
	d.w.a11yOn = true
	d.Settle()
	return d.r.a11yTree
}

// AccessibilityTree returns the semantics tree of the last frame (nil
// until EnableAccessibility).
func (d *Driver) AccessibilityTree() *a11y.Tree { return d.r.a11yTree }

// A11yFind returns the first node with role and name in the last tree
// (a11y.RoleNone matches any role, "" any name).
func (d *Driver) A11yFind(role a11y.Role, name string) (*a11y.Node, bool) {
	n := d.r.a11yTree.Find(role, name)
	return n, n != nil
}

// A11yFindAll returns every node with role and name in the last tree.
func (d *Driver) A11yFindAll(role a11y.Role, name string) []*a11y.Node {
	return d.r.a11yTree.FindAll(role, name)
}

// A11yFocused returns the node with keyboard focus (nil if none).
func (d *Driver) A11yFocused() *a11y.Node {
	t := d.r.a11yTree
	if t == nil || t.Focus == 0 {
		return nil
	}
	return t.Node(t.Focus)
}

// PerformAction runs an accessibility action like a screen reader would
// (Runner.PerformAction) and settles.
func (d *Driver) PerformAction(id a11y.NodeID, act a11y.Action, arg string) error {
	if err := d.r.PerformAction(id, act, arg); err != nil {
		return err
	}
	d.Settle()
	return nil
}

// A11yPerform finds the node with role and name and performs act on it.
func (d *Driver) A11yPerform(role a11y.Role, name string, act a11y.Action, arg string) error {
	n, ok := d.A11yFind(role, name)
	if !ok {
		return fmt.Errorf("ui: no %s named %q in the accessibility tree", role, name)
	}
	return d.PerformAction(n.ID, act, arg)
}

// A11yUpdates returns how many times the host was notified and the
// changed IDs of the last notification.
func (d *Driver) A11yUpdates() (int, []a11y.NodeID) { return d.w.a11yUpdates, d.w.a11yChanged }
