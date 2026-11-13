package ui

import (
	"fmt"
	"hash/maphash"
	"time"

	"github.com/tangoui/tango/ui/a11y"
	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/input"
	"github.com/tangoui/tango/ui/layout"
	"github.com/tangoui/tango/ui/paint"
	"github.com/tangoui/tango/ui/theme"
)

// ID identifies a widget across frames. IDs derive from the parent scope,
// the widget kind, its label and its occurrence count; wrap dynamic lists in
// Key to make identity independent of position.
type ID uint64

var seed = maphash.MakeSeed()

func hashID(parts ...any) ID {
	var h maphash.Hash
	h.SetSeed(seed)
	for _, p := range parts {
		switch v := p.(type) {
		case string:
			h.WriteString(v)
		case ID:
			var b [8]byte
			for i := range b {
				b[i] = byte(v >> (8 * i))
			}
			h.Write(b[:])
		case int:
			var b [8]byte
			for i := range b {
				b[i] = byte(uint64(v) >> (8 * i))
			}
			h.Write(b[:])
		default:
			h.WriteString(fmt.Sprint(v))
		}
		h.WriteByte(0)
	}
	return ID(h.Sum64())
}

// PaintFunc draws a node; r is the node's absolute rectangle.
type PaintFunc func(l *paint.List, r geom.Rect)

type node struct {
	ln       layout.Node
	id       ID
	label    string
	parent   *node
	kids     []*node
	paint    PaintFunc
	over     PaintFunc
	hit      bool
	cursor   input.Cursor
	focus    bool
	clip     bool
	radii    geom.Radii
	scroll   *scrollState
	opacity  float32                       // 0 = opaque
	xf       func(r geom.Rect) geom.Affine // subtree transform (absolute coordinates); nil = none
	abs      geom.Rect
	blocks   bool // swallow pointer for lower layers (panels, masks)
	disabled bool
	ownKeys  bool // widget handles Enter/Space itself (no implicit click)
	noTab    bool // focusable by pointer only; Tab skips it

	// Accessibility (a11y_tree.go). Plain field stores so annotating costs
	// nothing when no tree is built.
	role     a11y.Role  // semantic role; RoleNone = flattened into the parent
	aName    string     // accessible name when aNameSet (else label)
	aNameSet bool       //
	aState   a11y.State // states known at build time
	aFill    semFiller  // completes the node while the tree is built
	aTarget  ID         // node that receives Press/Expand/Collapse (0 = itself)
	aHidden  bool       // subtree not exposed (decorative, duplicated content)
	aKbd     bool       // keyboard equivalent provided elsewhere (Audit)
	aScroll  bool       // scrolls its content itself (Box.A11yScroller)

	// Redraw hints (damage.go): pass in which paintNode reached the node
	// unculled, and its screen-space bounding box in that pass.
	drawn uint64
	scr   geom.Rect

	ptr ptrFlags // touch / pointer-takeover declarations (pointer_capture.go)

	env *env // configuration scope the node was built in (env.go); nil = inherit while painting
}

type hit struct {
	id     ID
	label  string
	rect   geom.Rect
	layer  int
	cursor input.Cursor
	blocks bool
}

type stateEntry struct {
	v    any
	seen uint64
}

// Context is passed to page functions; it builds the frame's widget tree
// and answers interaction queries. It is only valid on the UI goroutine
// during a frame.
type Context struct {
	Theme *theme.Theme

	win     Window
	now     time.Time
	pass    uint64
	size    geom.Size
	noAnim  bool
	running bool

	root     *node
	cur      *node
	scope    ID
	counters map[ID]int
	overlays []*overlay
	layer    int

	states map[ID]*stateEntry
	anims  map[animKey]*anim

	// Interaction, evaluated against the previous frame's hits.
	hits, nextHits []hit
	rects          map[ID]geom.Rect
	nextRects      map[ID]geom.Rect
	pointer        geom.Point
	pointerIn      bool
	hot            ID
	topLayer       int
	active         ID
	pressPos       geom.Point
	pressMods      input.Modifiers // modifiers held at the last primary press
	pointerMods    input.Modifiers // modifiers of the latest pointer or key event (PointerMods)
	pressed        map[ID]bool
	clicked        map[ID]int
	released       map[ID]bool
	releasePos     map[ID]geom.Point // position of each primary release this frame (ReleasePos)
	pressedAny     bool
	lastClick      time.Time
	lastClickID    ID
	clickCount     int
	tabCap         tabCapture    // CaptureTab declarations (tab_capture.go)
	events         []input.Event // ordered key / text / composition events of this frame (events_for.go)
	winBlurred     bool          // the window lost key focus (WindowFocused)
	dir            dirState      // resolved layout direction of the current scope (direction.go)
	env            *env          // current configuration scope (env.go); swapped during paint / measure
	envRoot        env           // the window's root scope, refreshed every build
	focus          ID
	focusVisible   bool
	focusOrder     input.FocusOrder[ID]
	nextFocusOrder input.FocusOrder[ID]
	keys           []input.KeyEvent
	text           []string
	compose        *input.Composition
	composing      string
	composeCursor  int
	escape         bool
	scrollQ        []input.Scroll
	wantText       bool
	textCaret      geom.Rect
	cursorShape    input.Cursor
	invalid        bool
	animating      bool
	wakeAt         time.Time
	disabledDepth  int
	deferred       []func()
	contentSize    geom.Size
	itemStatus     string // validation status of the enclosing FormItem
	scrollTargets  []scrollTarget
	prevScroll     []scrollTarget

	// Layer stack (see layers.go).
	curOverlay       ID         // overlay whose content is being built (0 = page)
	overlayLayer     map[ID]int // overlay root -> layer, last painted frame
	nextOverlayLayer map[ID]int
	escTop, escNext  int  // highest layer listening for Escape: last frame / this build
	escPending       []ID // Escape listeners whose panel had no layer yet
	pressLayer       int  // layer under the pointer when it went down this frame
	ctxPressed       bool // secondary-button press this frame
	ctxPos           geom.Point
	ctxLayer         int

	// Paint-time transform of the subtree being painted (see paintNode).
	paintXf   geom.Affine
	paintXfOn bool

	// Accessibility and focus management (a11y_tree.go, focus.go).
	a11yOn       bool
	reduceMotion bool
	announce     []a11y.Announcement
	semOver      map[ID]semOverride
	a11yNodes    map[ID]*node // semantic nodes of the last tree
	a11yParent   map[ID]*node
	setValues    map[ID]string // pending SetValue actions
	actNow       []a11yAction  // actions applied this frame (A11yAction)
	traps        []ID          // focus-trapping panels built this frame
	trapPrev     []trapEntry   // trapping panels of the last frame
	focusList    []focusEntry  // focusable widgets of the last frame, paint order
	nextFocus    []focusEntry  // being collected
	focusReturn  ID            // where focus goes if its overlay closes (anchor)

	// Redraw hints declared this frame (damage.go).
	damage      []geom.Rect   // Context.Damage, window logical pixels
	shifts      []scrollShift // ScrollShift calls of the current build pass
	staleShifts []scrollShift // ScrollShift calls of earlier passes this frame
	declared    bool          // any Damage / ScrollShift this frame
	fp          fpState       // paint fingerprints (damage_fp.go)

	// Touch input, pointer takeover and safe area (touch.go,
	// pointer_capture.go, safearea.go).
	ptr  ptrState
	safe safeState

	// Soft keyboard and input-method state (keyboard.go, textstate.go).
	kb  kbState
	ime imeState
}

func newContext(th *theme.Theme) *Context {
	c := &Context{
		Theme:     th,
		states:    map[ID]*stateEntry{},
		anims:     map[animKey]*anim{},
		rects:     map[ID]geom.Rect{},
		nextRects: map[ID]geom.Rect{},
		pressed:   map[ID]bool{},
		clicked:   map[ID]int{},
		released:  map[ID]bool{},
		counters:  map[ID]int{},

		overlayLayer:     map[ID]int{},
		nextOverlayLayer: map[ID]int{},
	}
	c.resetRootEnv()
	return c
}

// Now returns the frame time (use it for timers, not time.Now).
func (c *Context) Now() time.Time { return c.now }

// Size returns the window's logical size.
func (c *Context) Size() geom.Size { return c.size }

// Window returns the host window.
func (c *Context) Window() Window { return c.win }

// Invalidate requests another build pass of the current frame (e.g. after
// changing state that widgets built earlier in this frame read). It must be
// called on the UI goroutine during a frame; from other goroutines use the
// function returned by Invalidator.
func (c *Context) Invalidate() { c.invalid = true }

// Invalidator returns a function that requests a new frame. Unlike
// Invalidate it is safe to call from any goroutine (background loaders,
// network callbacks) and after the frame that obtained it has ended.
func (c *Context) Invalidator() func() {
	w := c.win
	return func() {
		if w != nil {
			w.Invalidate()
		}
	}
}

// Invalidate requests a frame of the window w; it is safe to call from any
// goroutine. Equivalent to w.Invalidate, but tolerates a nil window.
func Invalidate(w Window) {
	if w != nil {
		w.Invalidate()
	}
}

// WakeAt asks for a frame at t (timers, auto-dismiss, caret blink).
func (c *Context) WakeAt(t time.Time) {
	if c.wakeAt.IsZero() || t.Before(c.wakeAt) {
		c.wakeAt = t
	}
}

// newID derives a widget ID in the current scope.
func (c *Context) newID(kind, label string) ID {
	base := hashID(c.scope, kind, label)
	n := c.counters[base]
	c.counters[base] = n + 1
	return hashID(base, n)
}

// Key runs fn in an identity scope named by key, so widgets inside keep
// their state when siblings are inserted or removed (list rows, tabs).
func Key(c *Context, key any, fn func()) {
	old := c.scope
	c.scope = hashID(old, "key", fmt.Sprint(key))
	fn()
	c.scope = old
}

// add creates a node under the current parent.
func (c *Context) add(kind, label string) *node {
	n := &node{id: c.newID(kind, label), label: label, parent: c.cur, disabled: c.disabledDepth > 0}
	n.ln.Ref = n
	c.stampEnv(n)
	if c.cur != nil {
		c.cur.kids = append(c.cur.kids, n)
	}
	return n
}

// body runs fn with n as the current parent.
func (c *Context) body(n *node, fn func()) {
	if fn == nil {
		return
	}
	prev, prevScope := c.cur, c.scope
	c.cur, c.scope = n, n.id
	fn()
	c.cur, c.scope = prev, prevScope
}

// State returns retained per-widget state, creating it with init on first
// use. State that is not requested for a while is dropped.
func State[T any](c *Context, id ID, init func() T) *T {
	if e, ok := c.states[id]; ok {
		e.seen = c.pass
		if p, ok := e.v.(*T); ok {
			return p
		}
	}
	v := new(T)
	if init != nil {
		*v = init()
	}
	c.states[id] = &stateEntry{v: v, seen: c.pass}
	return v
}

// RectOf returns the absolute rectangle id had in the last painted frame.
func (c *Context) RectOf(id ID) (geom.Rect, bool) {
	r, ok := c.rects[id]
	return r, ok
}

// Disabled runs fn with every widget inside disabled.
func Disabled(c *Context, on bool, fn func()) {
	if on {
		c.disabledDepth++
	}
	fn()
	if on {
		c.disabledDepth--
	}
}

// ---------------------------------------------------------------------------
// Interaction

// Interaction is the per-frame pointer/keyboard state of a widget.
type Interaction struct {
	ID           ID
	Hovered      bool // pointer over it and not covered by a higher layer
	Pressed      bool // pointer went down on it and is still held
	Clicked      bool // activated this frame (pointer or Enter/Space)
	Clicks       int  // click count of the activation (2 = double click)
	PressedNow   bool // pointer went down on it this frame
	Focused      bool
	FocusVisible bool // focused via keyboard: draw the focus ring
}

func (c *Context) interact(n *node, focusable bool, cur input.Cursor) Interaction {
	n.hit = true
	n.cursor = cur
	n.focus = focusable && !n.disabled
	id := n.id
	ia := Interaction{ID: id}
	if n.disabled {
		return ia
	}
	ia.Hovered = c.hovered(id)
	ia.Pressed = c.active == id && c.hot == id
	ia.PressedNow = c.pressed[id]
	if k := c.clicked[id]; k > 0 {
		ia.Clicked, ia.Clicks = true, k
	}
	ia.Focused = c.focus == id
	ia.FocusVisible = ia.Focused && c.focusVisible
	if focusable && ia.Focused && !n.ownKeys {
		for _, k := range c.keys {
			if k.Down && (k.Key == input.KeyEnter || k.Key == input.KeySpace) {
				ia.Clicked, ia.Clicks = true, 1
			}
		}
	}
	if ia.PressedNow && focusable {
		c.SetFocus(id)
		ia.Focused = true
	}
	return ia
}

func (c *Context) hovered(id ID) bool {
	if !c.pointerIn {
		return false
	}
	for i := len(c.hits) - 1; i >= 0; i-- {
		h := c.hits[i]
		if h.id == id && h.rect.Contains(c.pointer) {
			return h.layer >= c.topLayer
		}
	}
	return false
}

// SetFocus moves keyboard focus to id (0 clears it).
func (c *Context) SetFocus(id ID) {
	if c.focus != id {
		c.focus = id
		c.invalid = true
	}
}

// Focused returns the focused widget.
func (c *Context) Focused() ID { return c.focus }

// KeysFor returns the key events delivered to id this frame when it has
// focus, and marks them consumed.
func (c *Context) KeysFor(id ID) []input.KeyEvent {
	if c.focus != id {
		return nil
	}
	k := c.keys
	c.keys = nil
	return k
}

// TextFor returns committed text input for focused id.
func (c *Context) TextFor(id ID) []string {
	if c.focus != id {
		return nil
	}
	t := c.text
	c.text = nil
	return t
}

// CompositionFor returns the current IME pre-edit string for focused id.
func (c *Context) CompositionFor(id ID) (string, int) {
	if c.focus != id {
		return "", 0
	}
	return c.composing, c.composeCursor
}

// WantTextInput tells the host that a text field has focus and where its
// caret is (for IME candidate windows). Called while painting, caret is in
// the space being painted; it is mapped to window coordinates.
func (c *Context) WantTextInput(caret geom.Rect) {
	c.wantText = true
	c.textCaret = c.screenRect(caret)
}

// Pointer returns the pointer position and whether it is inside the window.
func (c *Context) Pointer() (geom.Point, bool) { return c.pointer, c.pointerIn }

// Dragging reports whether id holds the pointer capture.
func (c *Context) Dragging(id ID) bool { return c.active == id }

// ---------------------------------------------------------------------------
// Containers

// Box is a styled layout container.
type Box struct {
	c *Context
	n *node

	bg      theme.Color
	border  theme.Color
	bw      float32
	shadows []paint.Shadow
	dashed  bool
	ia      *Interaction
}

func (c *Context) box(kind string, dir layout.Direction, opts []layout.Option) *Box {
	n := c.add(kind, "")
	n.ln.Dir = dir
	n.ln.Style.Apply(opts...)
	b := &Box{c: c, n: n}
	n.paint = b.paintBox
	return b
}

func (b *Box) paintBox(l *paint.List, r geom.Rect) {
	if len(b.shadows) > 0 {
		l.Shadows(r, b.n.radii, b.shadows)
	}
	l.Fill(r, b.n.radii, b.bg)
	if b.bw > 0 {
		if b.dashed {
			l.StrokeDashed(r, b.n.radii, b.bw, b.border, []float32{4, 3})
		} else {
			l.Stroke(r, b.n.radii, b.bw, b.border)
		}
	}
}

// Body adds children.
func (b *Box) Body(fn func()) *Box { b.c.body(b.n, fn); return b }

// Bg sets the background color.
func (b *Box) Bg(col theme.Color) *Box { b.bg = col; return b }

// Radius rounds the corners.
func (b *Box) Radius(r float32) *Box { b.n.radii = geom.Rad(r); return b }

// Radii sets per-corner radii.
func (b *Box) Radii(r geom.Radii) *Box { b.n.radii = r; return b }

// Border draws a border inside the box.
func (b *Box) Border(w float32, col theme.Color) *Box { b.bw, b.border = w, col; return b }

// DashedBorder draws a dashed border.
func (b *Box) DashedBorder(w float32, col theme.Color) *Box {
	b.bw, b.border, b.dashed = w, col, true
	return b
}

// Shadow sets layered box shadows.
func (b *Box) Shadow(s []paint.Shadow) *Box { b.shadows = s; return b }

// Clip clips children to the (rounded) box.
func (b *Box) Clip() *Box { b.n.clip = true; return b }

// Opacity fades the box and its children.
func (b *Box) Opacity(a float32) *Box {
	if a >= 1 {
		b.n.opacity = 0 // 0 means fully opaque: no group needed
		return b
	}
	b.n.opacity = max(a, 0.0001)
	return b
}

// Transform draws the box and its subtree through m, given in the box's
// own coordinates (origin at its top-left corner), like a CSS transform
// with transform-origin 0 0. Layout is unaffected; hit testing and Rect use
// the transformed bounding box. Combine with geom.ScaleAround /
// RotateAround for other origins, e.g. scaling about the centre:
//
//	b.Transform(geom.ScaleAround(k, k, geom.Pt(w/2, h/2)))
func (b *Box) Transform(m geom.Affine) *Box {
	if m.IsIdentity() {
		b.n.xf = nil
		return b
	}
	b.n.xf = func(r geom.Rect) geom.Affine {
		return geom.Translate(r.X, r.Y).Mul(m).Mul(geom.Translate(-r.X, -r.Y))
	}
	return b
}

// Scale scales the box and its subtree by k about its centre (enter /
// leave motion); 1 removes the transform.
func (b *Box) Scale(k float32) *Box {
	if k == 1 {
		b.n.xf = nil
		return b
	}
	b.n.xf = func(r geom.Rect) geom.Affine {
		return geom.ScaleAround(k, k, geom.Pt(r.X+r.W/2, r.Y+r.H/2))
	}
	return b
}

// Style tweaks layout after creation.
func (b *Box) Style(opts ...layout.Option) *Box { b.n.ln.Style.Apply(opts...); return b }

// Paint adds a custom painter drawn above the background, below children.
func (b *Box) Paint(fn PaintFunc) *Box {
	prev := b.n.paint
	b.n.paint = func(l *paint.List, r geom.Rect) { prev(l, r); fn(l, r) }
	return b
}

// PaintOver adds a painter drawn above children.
func (b *Box) PaintOver(fn PaintFunc) *Box { b.n.over = fn; return b }

// Interact makes the box hoverable/clickable and returns its state.
func (b *Box) Interact(cursor input.Cursor) Interaction {
	if b.ia == nil {
		ia := b.c.interact(b.n, false, cursor)
		b.ia = &ia
	}
	return *b.ia
}

// Hovered reports pointer hover (registers the box for hit testing).
func (b *Box) Hovered() bool { return b.Interact(input.CursorDefault).Hovered }

// Clicked reports a click on the box.
func (b *Box) Clicked() bool { return b.Interact(input.CursorPointer).Clicked }

// ID returns the box's widget ID.
func (b *Box) ID() ID { return b.n.id }

// Rect returns the box's absolute rect from the previous frame.
func (b *Box) Rect() geom.Rect { return b.c.rects[b.n.id] }

// Div creates an empty styled box (Column by default); add children with Body.
func Div(c *Context, opts ...layout.Option) *Box { return c.box("div", layout.Column, opts) }

// Row lays out children horizontally.
func Row(c *Context, fn func(), opts ...layout.Option) *Box {
	return c.box("row", layout.Row, opts).Body(fn)
}

// Column lays out children vertically.
func Column(c *Context, fn func(), opts ...layout.Option) *Box {
	return c.box("col", layout.Column, opts).Body(fn)
}

// StackBox overlaps children.
func StackBox(c *Context, fn func(), opts ...layout.Option) *Box {
	return c.box("stack", layout.Stack, opts).Body(fn)
}

// Spacer grows to take free space.
func Spacer(c *Context) { c.add("spacer", "").ln.Grow = 1 }

// Gap inserts a fixed-size blank.
func Gap(c *Context, size float32) {
	n := c.add("gap", "")
	n.ln.Width, n.ln.Height = size, size
	n.ln.AlignSelf = layout.AlignStart
}

// Custom adds a leaf with a measure and paint function.
func Custom(c *Context, measure func(maxW, maxH float32) geom.Size, p PaintFunc, opts ...layout.Option) *Box {
	b := c.box("custom", layout.Column, opts)
	b.n.ln.Measure = measure
	// Keep the box painter (Bg / Border / Shadow / Radius) under p.
	if p != nil {
		b.Paint(p)
	}
	return b
}
