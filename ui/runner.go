package ui

import (
	"time"

	"github.com/tangoui/tango/ui/a11y"
	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/input"
	"github.com/tangoui/tango/ui/layout"
	"github.com/tangoui/tango/ui/paint"
	"github.com/tangoui/tango/ui/theme"
)

// RunnerOptions configure a Runner.
type RunnerOptions struct {
	Theme *theme.Theme // default theme.AntDesign()
	// Renderer overrides the CPU renderer (GPU backends). When it is not a
	// *paint.CPU the runner calls Window.Present(nil) after rendering.
	Renderer paint.Renderer
	// Clock overrides time.Now (tests, deterministic screenshots).
	Clock func() time.Time
	// NoAnimations makes every transition jump to its end state.
	NoAnimations bool
	// ReduceMotion honours the user's "reduce motion" preference: every
	// transition (Anim, AnimFrom, enter/leave motion, chart growth)
	// jumps to its end value and decorative loops (skeleton shimmer,
	// badge pulse) stand still. Unlike NoAnimations it does not change
	// timers such as hover delays. Hosts that track the OS setting call
	// Runner.SetReduceMotion when it changes.
	ReduceMotion bool
}

// Runner drives one window: it queues input, runs the page function, lays
// out, paints and presents. All methods except Invalidate must be called on
// the UI thread; Invalidate is safe from any goroutine.
type Runner struct {
	c         *Context
	page      func(*Context)
	win       Window
	renderer  paint.Renderer
	cpu       *paint.CPU
	clock     func() time.Time
	queue     []input.Event
	list      paint.List
	lastCur   input.Cursor
	textOn    bool
	lastCaret geom.Rect
	closed    bool

	// Input-method state (textstate.go): the state last reported, whether
	// it is in effect, the widget it came from, and the focused widget
	// that took TextEdit events last frame.
	lastState  input.TextInputState
	stateOn    bool
	stateFocus ID
	editAware  ID

	a11yForce   bool         // EnableAccessibility (Driver, tests)
	a11yTree    *a11y.Tree   // last tree reported (nil while off)
	a11yUpdates int          // AccessibilityUpdate calls (tests)
	actions     []a11yAction // queued PerformAction requests

	dmg damageState // redraw-hint change tracking (damage.go)

	backOn bool // last value sent to BackWindow (back.go)
}

// NewRunner creates a runner for page. Call Attach before Frame.
func NewRunner(page func(*Context), o RunnerOptions) *Runner {
	th := o.Theme
	if th == nil {
		th = theme.AntDesign()
	}
	r := &Runner{page: page, c: newContext(th), clock: o.Clock, renderer: o.Renderer}
	if r.clock == nil {
		r.clock = time.Now
	}
	if r.renderer == nil {
		r.cpu = paint.NewCPU()
		r.renderer = r.cpu
	} else if cpu, ok := r.renderer.(*paint.CPU); ok {
		r.cpu = cpu
	}
	r.c.noAnim = o.NoAnimations
	r.c.reduceMotion = o.ReduceMotion
	return r
}

// SetReduceMotion turns the reduced-motion preference on or off (see
// RunnerOptions.ReduceMotion) and requests a frame.
func (r *Runner) SetReduceMotion(on bool) {
	r.c.reduceMotion = on
	r.Invalidate()
}

// EnableAccessibility builds the semantics tree every frame even when the
// window is not an AccessibilityHost (or reports it disabled): tests,
// automation and hosts that enable it explicitly. Off again leaves the
// decision to the host.
func (r *Runner) EnableAccessibility(on bool) {
	r.a11yForce = on
	r.Invalidate()
}

// AccessibilityTree returns the semantics tree of the last frame, or nil
// when accessibility is off.
func (r *Runner) AccessibilityTree() *a11y.Tree { return r.a11yTree }

// accessibilityOn decides whether this frame builds a tree.
func (r *Runner) accessibilityOn() (AccessibilityHost, bool) {
	h, _ := r.win.(AccessibilityHost)
	if r.a11yForce {
		return h, true
	}
	if h != nil && h.AccessibilityEnabled() {
		return h, true
	}
	return nil, false
}

// Attach binds the runner to its window.
func (r *Runner) Attach(w Window) { r.win = w; r.c.win = w }

// Context exposes the context (tests, advanced integrations).
func (r *Runner) Context() *Context { return r.c }

// SetTheme switches the theme and requests a frame.
func (r *Runner) SetTheme(t *theme.Theme) {
	r.c.Theme = t
	r.Invalidate()
}

// Invalidate requests a frame. It is safe to call from any goroutine (it
// only forwards to Window.Invalidate, which hosts must make thread-safe);
// the frame itself still runs on the UI thread.
func (r *Runner) Invalidate() {
	if r.win != nil {
		r.win.Invalidate()
	}
}

// HandleEvent queues an input event and requests a frame.
func (r *Runner) HandleEvent(e input.Event) {
	r.queue = append(r.queue, e)
	r.Invalidate()
}

// Close releases resources.
func (r *Runner) Close() { r.closed = true }

// List returns the display list of the last frame (debugging, GPU hosts).
func (r *Runner) List() *paint.List { return &r.list }

// Frame processes queued input, rebuilds the UI and presents it.
func (r *Runner) Frame() error {
	if r.closed || r.win == nil {
		return nil
	}
	c := r.c
	c.now = r.clock()
	c.size = r.win.Size()
	r.updateSafeArea()
	r.updateKeyboardInset()
	r.editAware, c.ime.aware = c.ime.aware, 0
	c.animating = false
	c.wakeAt = time.Time{}
	host, a11yOn := r.accessibilityOn()
	if !a11yOn && r.a11yTree != nil {
		r.a11yTree, c.a11yNodes, c.a11yParent = nil, nil, nil
	}
	c.a11yOn = a11yOn
	c.announce = c.announce[:0]
	c.beginDamage()
	had := r.processEvents()
	for pass := 0; ; pass++ {
		c.invalid = false
		r.build()
		if !(had || c.invalid) || pass >= 2 {
			break
		}
		had = false
		r.clearFrameInput()
	}
	r.clearFrameInput()
	r.applyKeyboardReveal()
	r.layoutAndPaint()
	scale := r.win.Scale()
	dirty := r.collectDamage(scale)
	r.afterFrame()
	if a11yOn {
		r.reportA11y(host)
	}
	err := r.renderer.Render(&r.list, c.size, scale)
	if err == nil {
		if r.cpu == nil {
			err = r.win.Present(nil)
		} else if dp, ok := r.win.(DamagePresenter); ok && dirty != nil {
			err = dp.PresentDamage(r.cpu.Image(), dirty)
		} else {
			err = r.win.Present(r.cpu.Image())
		}
	}
	if err != nil {
		r.dmg.started = false // this frame may not be on screen: next one is full
	}
	return err
}

// reportA11y builds the semantics tree of the frame just painted and
// sends the changes to the host.
func (r *Runner) reportA11y(host AccessibilityHost) {
	c := r.c
	t := c.buildA11yTree()
	changed := a11y.Diff(r.a11yTree, t)
	focusMoved := r.a11yTree == nil || r.a11yTree.Focus != t.Focus
	r.a11yTree = t
	if host != nil && (len(changed) > 0 || focusMoved || len(t.Announcements) > 0) {
		r.a11yUpdates++
		host.AccessibilityUpdate(t, changed)
	}
}

func (r *Runner) clearFrameInput() {
	c := r.c
	clear(c.pressed)
	clear(c.clicked)
	clear(c.released)
	clear(c.releasePos)
	c.pressedAny = false
	c.ctxPressed = false
	c.keys = c.keys[:0]
	c.text = c.text[:0]
	clear(c.events)
	c.events = c.events[:0]
	c.escape = false
	c.ime.edits = c.ime.edits[:0]
	clear(c.setValues)
	c.actNow = c.actNow[:0]
	c.clearCapFrame()
	c.clearTouchFrame()
}

func (r *Runner) processEvents() bool {
	c := r.c
	had := len(r.queue) > 0
	r.queue = r.expandTouches(r.queue) // touch.go: route fingers, emulate the pointer
	r.queue = r.imeEdits(r.queue)      // textstate.go: TextEdit for widgets that know only keys
	for _, e := range r.queue {
		switch ev := e.(type) {
		case input.Pointer:
			c.pointerMods = ev.Mods
			c.pointer = ev.Pos
			c.pointerIn = ev.Kind != input.Leave
			c.updateHot()
			switch ev.Kind {
			case input.Press:
				if ev.Button == input.ButtonRight {
					c.ctxPressed, c.ctxPos, c.ctxLayer = true, ev.Pos, c.topLayer
				}
				if ev.Button != input.ButtonLeft {
					continue
				}
				c.active = c.hot
				c.pressPos = ev.Pos
				c.pressMods = ev.Mods
				c.pressedAny = true
				c.pressLayer = c.topLayer
				c.focusVisible = false
				if bar, ok := c.scrollbarAt(ev.Pos); ok {
					// The press grabs the scrollbar, not the widget under it.
					c.active = bar
					c.pressed[bar] = true
				} else if c.hot != 0 {
					c.pressed[c.hot] = true
				}
				c.capPress(ev.Pos, ev.Source) // pointer_capture.go
				if !c.focusOrder.Contains(c.hot) {
					c.focus = 0
				}
				n := ev.Clicks
				if n == 0 {
					if c.hot == c.lastClickID && c.now.Sub(c.lastClick) < 400*time.Millisecond {
						c.clickCount++
					} else {
						c.clickCount = 1
					}
					n = c.clickCount
				}
				c.clickCount = n
				c.lastClick, c.lastClickID = c.now, c.hot
			case input.Release:
				if ev.Button != input.ButtonLeft {
					continue
				}
				taken := c.capRelease()
				if c.active != 0 {
					c.released[c.active] = true
					if c.releasePos == nil {
						c.releasePos = map[ID]geom.Point{}
					}
					c.releasePos[c.active] = ev.Pos
					if c.hot == c.active && !taken {
						c.clicked[c.active] = max(1, c.clickCount)
					}
				}
				c.active = 0
			case input.Move:
				c.capMove(ev.Pos)
			case input.Cancel:
				c.capCancel()
				c.active = 0
			}
		case input.Scroll:
			c.pointerMods = ev.Mods
			c.pointer, c.pointerIn = ev.Pos, true
			c.updateHot()
			r.applyScroll(ev)
		case input.KeyEvent:
			c.pointerMods = ev.Mods
			if ev.Key == input.KeyTab && c.tabCaptured(ev) {
				ev.Code = ev.Key
				c.keys = append(c.keys, ev) // tab_capture.go: the focused widget wants Tab
				c.events = append(c.events, ev)
				continue
			}
			if ev.Down && ev.Key == input.KeyTab {
				if id, ok := c.tabNext(c.focus, ev.Mods.Has(input.ModShift)); ok {
					c.focus = id
					c.focusVisible = true
				}
				continue
			}
			if ev.Down && ev.Key == input.KeyEscape {
				c.tabEscaped() // the next Tab moves focus even out of a Tab-capturing widget
				c.escape = true
				ev.Code = ev.Key
				c.events = append(c.events, ev) // ordered stream only: Esc stays an overlay key
				continue
			}
			if ev.Down {
				c.tabCap.escaped = false
			}
			// Components match on Key; keypad duplicates (NumpadEnter,
			// Numpad0-9, ...) fold onto the main key so Enter submits and
			// digits type everywhere. Code keeps the physical key.
			if ev.Code == 0 {
				ev.Code = ev.Key
			}
			ev.Key = ev.Key.Base()
			c.keys = append(c.keys, ev)
			c.events = append(c.events, ev)
		case input.TextInput:
			c.text = append(c.text, ev.Text)
			c.events = append(c.events, ev)
			c.composing = ""
		case input.Composition:
			c.composing = ev.Text
			c.composeCursor = ev.Cursor
			c.events = append(c.events, ev)
		case input.TextEdit, input.SelectionChange:
			c.ime.edits = append(c.ime.edits, ev)
			c.events = append(c.events, ev)
		case input.WindowFocus:
			c.winBlurred = !ev.Focused
			if !ev.Focused {
				c.capCancel()
				c.active = 0
			}
		}
	}
	r.queue = r.queue[:0]
	if len(r.actions) > 0 {
		had = true
		for _, a := range r.actions {
			c.applyAction(a)
		}
		r.actions = r.actions[:0]
	}
	return had
}

func (c *Context) updateHot() {
	c.hot, c.topLayer = 0, 0
	if !c.pointerIn {
		return
	}
	for _, h := range c.hits {
		if h.rect.Contains(c.pointer) && h.layer > c.topLayer {
			c.topLayer = h.layer
		}
	}
	for i := len(c.hits) - 1; i >= 0; i-- {
		h := c.hits[i]
		if h.layer >= c.topLayer && h.rect.Contains(c.pointer) {
			c.hot = h.id
			return
		}
	}
}

func (r *Runner) build() {
	c := r.c
	c.pass++
	c.stashShifts()
	clear(c.counters)
	c.resetRootEnv()
	c.root = &node{id: hashID("root")}
	c.root.ln.Ref = c.root
	c.root.ln.Width, c.root.ln.Height = c.size.W, c.size.H
	c.stampEnv(c.root)
	c.cur, c.scope = c.root, c.root.id
	c.overlays = c.overlays[:0]
	c.curOverlay = 0
	c.escNext = 0
	c.escPending = c.escPending[:0]
	c.backNext = false
	c.layer = 0
	c.wantText = false
	c.ime.want = false
	c.traps = c.traps[:0]
	clear(c.semOver)
	c.running = true
	c.deferred = c.deferred[:0]
	r.page(c)
	c.useEnv(c.rootEnvPtr()) // global layers and layout start in the root scope
	i := 0
	runDeferred := func() {
		for ; i < len(c.deferred); i++ {
			c.deferred[i]()
		}
	}
	runDeferred()
	renderGlobals(c)
	runDeferred() // work registered by widgets inside global layers
	c.running = false
}

func link(n *node) {
	n.ln.Children = n.ln.Children[:0]
	wrapMeasure(n) // nested-scope nodes measure in their own env (env.go)
	for _, k := range n.kids {
		link(k)
		n.ln.Children = append(n.ln.Children, &k.ln)
	}
}

func (r *Runner) layoutAndPaint() {
	c := r.c
	// The last frame's ops stay with the fingerprints (damage_fp.go).
	r.list.Ops = c.fp.fpBegin(r.list.Ops)
	r.list.Background = c.Theme.BgLayout
	c.nextHits = c.nextHits[:0]
	clear(c.nextRects)
	c.nextFocusOrder.Reset()
	c.nextFocus = c.nextFocus[:0]
	link(c.root)
	layout.Solve(&c.root.ln, c.size.W, c.size.H)
	view := geom.Rect{W: c.size.W, H: c.size.H}
	c.contentSize = geom.Size{}
	for _, k := range c.root.kids {
		c.contentSize.W = max(c.contentSize.W, k.ln.Rect.Right()+k.ln.Margin.Right)
		c.contentSize.H = max(c.contentSize.H, k.ln.Rect.Bottom()+k.ln.Margin.Bottom)
	}
	c.paintNode(&r.list, c.root, 0, 0, view, 0)
	clear(c.nextOverlayLayer)
	for i, o := range c.overlays {
		c.nextOverlayLayer[o.root.id] = i + 1
		c.layoutOverlay(o)
		c.paintNode(&r.list, o.root, 0, 0, view, i+1)
	}
}

func (c *Context) paintNode(l *paint.List, n *node, ox, oy float32, clip geom.Rect, layer int) {
	if e := n.env; e != nil && e != c.env {
		// Paint (and the subtree) in the scope the node was built in.
		prev, prevTh := c.enterEnv(e)
		defer c.leaveEnv(prev, prevTh)
	}
	rr := n.ln.Rect
	r := geom.Rect{X: rr.X + ox, Y: rr.Y + oy, W: rr.W, H: rr.H}
	n.abs = r
	if n.xf != nil && c.paintTransformed(l, n, r, ox, oy, clip, layer) {
		return
	}
	c.nextRects[n.id] = c.screenRect(r)
	vis := r.Intersect(clip)
	if vis.Empty() && !n.ln.Absolute {
		return
	}
	n.drawn, n.scr = c.pass, c.screenRect(r)
	region := c.openRegion(n, vis, layer) // touch / takeover declarations (pointer_capture.go)
	fi := c.fpOpen(n, r, clip, len(l.Ops))
	if n.opacity > 0 {
		l.Opacity(n.opacity)
	}
	decl := len(c.damage) + len(c.shifts)
	if n.paint != nil {
		n.paint(l, r)
	}
	self := len(c.damage)+len(c.shifts) != decl
	if n.hit {
		c.nextHits = append(c.nextHits, hit{id: n.id, label: n.label, rect: c.screenRect(vis), layer: layer, cursor: n.cursor, blocks: n.blocks})
		if n.focus {
			if n.noTab {
				c.nextFocusOrder.AddNoTab(n.id)
			} else {
				c.nextFocusOrder.Add(n.id)
			}
			c.nextFocus = append(c.nextFocus, focusEntry{id: n.id, layer: layer, tab: !n.noTab})
		}
	}
	childClip := clip
	clipped := n.clip || n.scroll != nil
	if clipped {
		l.Clip(r, n.radii)
		childClip = vis
	}
	var sx, sy float32
	if s := n.scroll; s != nil {
		s.kbPad = c.keyboardPad(c.screenRect(r)) // keyboard.go
		sx, sy = s.frame(r.Size(), n.ln.Content)
		sv := c.screenRect(vis)
		c.nextHits = append(c.nextHits, hit{id: n.id ^ scrollTag, rect: sv, layer: layer})
		c.scrollTargets = append(c.scrollTargets, scrollTarget{id: n.id, rect: sv, layer: layer, s: s, from: len(c.nextHits) - 1})
	}
	ti := len(c.scrollTargets) - 1
	c.fp.cur[fi].a1 = int32(len(l.Ops))
	parent := c.fp.parent
	c.fp.parent = fi
	for _, k := range n.kids {
		c.paintNode(l, k, r.X-sx, r.Y-sy, childClip, layer)
	}
	c.fp.parent = parent
	c.fp.cur[fi].b0 = int32(len(l.Ops))
	if n.scroll != nil {
		c.scrollTargets[ti].to = len(c.nextHits)
	}
	c.closeRegion(region)
	if clipped {
		l.Pop()
	}
	if n.over != nil {
		decl = len(c.damage) + len(c.shifts)
		n.over(l, r)
		self = self || len(c.damage)+len(c.shifts) != decl
	}
	if n.opacity > 0 {
		l.Pop()
	}
	fr := &c.fp.cur[fi]
	fr.b1, fr.self = int32(len(l.Ops)), self
}

// paintTransformed paints a node that carries a transform: the subtree is
// drawn through the matrix with its clip kept in the subtree's own
// (untransformed) space and conservatively widened, while hit rects and
// Rect report screen-space bounding boxes. It returns false when the
// transform is the identity (paint normally).
func (c *Context) paintTransformed(l *paint.List, n *node, r geom.Rect, ox, oy float32, clip geom.Rect, layer int) bool {
	xf := n.xf(r)
	if xf.IsIdentity() {
		return false
	}
	inv, ok := xf.Invert()
	if !ok {
		return true // collapsed to nothing: neither visible nor hittable
	}
	prevXf, prevOn := c.paintXf, c.paintXfOn
	if prevOn {
		c.paintXf = prevXf.Mul(xf)
	} else {
		c.paintXf, c.paintXfOn = xf, true
	}
	l.Transform(xf)
	f := n.xf
	n.xf = nil
	c.paintNode(l, n, ox, oy, inv.Bounds(clip), layer)
	n.xf = f
	l.Pop()
	c.paintXf, c.paintXfOn = prevXf, prevOn
	return true
}

// screenRect maps a rect in the space being painted to its screen-space
// bounding box (identity outside transformed subtrees).
func (c *Context) screenRect(r geom.Rect) geom.Rect {
	if !c.paintXfOn {
		return r
	}
	return c.paintXf.Bounds(r)
}

func (r *Runner) afterFrame() {
	c := r.c
	c.hits, c.nextHits = c.nextHits, c.hits
	c.rects, c.nextRects = c.nextRects, c.rects
	c.focusOrder, c.nextFocusOrder = c.nextFocusOrder, c.focusOrder
	c.prevScroll, c.scrollTargets = c.scrollTargets, c.prevScroll[:0]
	c.overlayLayer, c.nextOverlayLayer = c.nextOverlayLayer, c.overlayLayer
	c.swapRegions()
	c.tabCap.cur, c.tabCap.next = c.tabCap.next, 0 // tab_capture.go
	for _, id := range c.escPending {
		if l, ok := c.overlayLayer[id]; ok && l > c.escNext {
			c.escNext = l
		}
	}
	c.escTop = c.escNext
	r.updateBack() // back.go
	c.focusList, c.nextFocus = c.nextFocus, c.focusList
	if f := c.focus; c.updateFocus() || c.focus != f {
		c.animating = true // repaint the focus ring where focus moved
	}
	if c.focus != 0 && !c.focusOrder.Contains(c.focus) {
		c.focus = 0
	}
	oldHot := c.hot
	c.updateHot()
	if c.hot != oldHot {
		c.animating = true // hover changed under a static pointer after relayout
	}
	// Cursor.
	cur := input.CursorDefault
	for i := len(c.hits) - 1; i >= 0; i-- {
		h := c.hits[i]
		if h.id == c.hot {
			cur = h.cursor
			break
		}
	}
	if cur != r.lastCur {
		r.win.SetCursor(cur)
		r.lastCur = cur
	}
	r.sendTextInput() // textstate.go
	r.checkKeyboard() // keyboard.go
	// GC retained state.
	for k, s := range c.states {
		if c.pass-s.seen > 300 {
			delete(c.states, k)
		}
	}
	for k, a := range c.anims {
		if c.pass-a.seen > 300 {
			delete(c.anims, k)
		}
	}
	if c.animating {
		r.win.InvalidateAt(c.now.Add(16 * time.Millisecond))
	} else if !c.wakeAt.IsZero() {
		r.win.InvalidateAt(c.wakeAt)
	}
}

// Animating reports whether the last frame had running transitions.
func (r *Runner) Animating() bool { return r.c.animating }

// NextWake returns the requested wake time (zero if none).
func (r *Runner) NextWake() time.Time { return r.c.wakeAt }
