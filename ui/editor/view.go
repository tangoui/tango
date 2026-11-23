package editor

import (
	"math"
	"sync"
	"sync/atomic"
	"time"

	"github.com/tangoui/tango/ui"
	"github.com/tangoui/tango/ui/a11y"
	"github.com/tangoui/tango/ui/editor/buffer"
	"github.com/tangoui/tango/ui/editor/keymap"
	"github.com/tangoui/tango/ui/editor/monarch"
	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/input"
	"github.com/tangoui/tango/ui/layout"
	"github.com/tangoui/tango/ui/text"
)

// lineHeightRatio is the line height relative to the font size (Monaco's
// GOLDEN_LINE_HEIGHT_RATIO on macOS; one value on every platform keeps the
// layout identical everywhere).
const lineHeightRatio = 1.5

// lineCacheSize is the capacity of a View's LineCache: a few screens of
// lines. Long lines are cached chunk by chunk (see longLine).
const lineCacheSize = 512

// ViewW is the code editor view of a State: it draws the visible lines with
// syntax colors, line numbers, the current-line highlight, the selection
// and the blinking caret, scrolls on the mouse wheel and trackpad, and edits
// the State from the keyboard (keys run commands through a keymap; see
// command.go and keymap.DefaultRules), the mouse and the IME.
//
// The View measures and shapes only the visible lines (plus two above and
// below); lines longer than longLineRunes are cut into chunks and only the
// chunks in the visible horizontal range are tokenized and shaped (see
// longLine). With word wrap (Options.WordWrap, editor.action.toggleWordWrap)
// the lines are view lines: a long model line is shown as several rows,
// each shaped on its own, and only the lines around the viewport are
// wrapped (wrap.go, viewlines.go). Redraws are driven by input and by the application: apart
// from the caret blink (a Context.WakeAt while focused), one frame after
// input it applied while painting and, in a large document, a frame when
// the folding controls' ranges are due (paint.go, foldRangesNow), and one
// frame when the occurrences of the word at the caret are due, and a frame
// after each frame that left a document search (the occurrences or the
// selection's matches) unfinished (occurrences.go: a long search is spread
// over frames), the View never requests frames on its own, not even
// while highlighting is
// incomplete. An application that starts the Highlighter's background
// goroutine passes c.Invalidator() (or ui.Invalidate) as its ready
// callback, so newly final tokens are drawn when they arrive.
type ViewW struct {
	c   *ui.Context
	st  *State
	box *ui.Box
	vs  *viewState
	ia  ui.Interaction

	opts             Options // this frame's options: st.Options with the modifiers' overrides
	km               *keymap.Keymap
	ctxFn            func(key string) (any, bool)
	pasteImage       func() bool
	onChange         func(ModelChange)
	completion       completionKeys
	gutterClick      func(GutterClick)
	glyphMarginClick func(GutterClick)
	foldOpacity      float32
	// lnFn, lnChars and lnGen are this frame's LineNumbersFunc (gutter.go).
	lnFn    LineNumberFunc
	lnChars int
	lnGen   uint64
	changed bool
	done    bool // input applied (finish)
}

// viewState is the View's retained state.
type viewState struct {
	lines *text.LineCache
	toks  []monarch.Token
	long  map[longKey]*longLine

	// Temporary shaping inputs; LineCache copies runs on a miss. Returned
	// paint colors are independently owned by each segment.
	bounds          []styleBound
	overrideScratch []styleBound
	shapeRuns       []text.Run

	// Widest line seen (content width); reset when the buffer or the
	// wrapping column changes.
	buf     *buffer.Buffer
	maxW    float32
	wrapCol int

	// Geometry of the last paint, absolute: the whole view, the gutter and
	// the text area, and the monospace column width.
	rect, gutter, text geom.Rect
	colW               float32

	inputState

	stickyLines                                          []int
	stickyY                                              []float32
	stickyClips                                          []geom.Rect
	stickyScopes                                         []FoldingRange // scratch: the ranges enclosing the view's top (sticky.go)
	stickyTokens                                         map[int]*stickyTokenLine
	highlightDeadline                                    time.Time
	stickyRect, minimapRect, overviewRect, minimapSlider geom.Rect
	mini                                                 minimapState
	miniRows                                             []miniRow
	navPaint                                             []fixedPaint
	railDrag                                             int // 1 minimap, 2 overview, 3 sticky press
	railGrab                                             float32
	railThumbHeight                                      float32

	// menu is the context menu (menu.go).
	menu menuState

	// Folding (paint.go): the x of the folding controls' box in the last
	// paint (-1: none), and the shaped placeholder of folded lines.
	foldX        float32
	ellipsis     *text.Layout
	ellipsisFont text.Style
	// The folding ranges awaiting the debounce (foldRangesNow): the text
	// they are for and when they are due.
	foldStale foldRangesKey
	foldDue   time.Time
	// placeholderPress is the folded line whose placeholder the pointer
	// went down on, -1 for none (unfolded on release there; input.go).
	placeholderPress int

	// brk is this frame's colored brackets per model line, wsMarks the
	// shaped whitespace marks (bracketpairs.go, whitespace.go); match
	// the matched brackets for matchKey.
	brk map[int][]brSpan
	brc brCache
	// brcLines counts the lines bracketSpans colored afresh (tests).
	brcLines int
	// runMemo keeps the rows' style runs across paints (memoRuns), runGen
	// numbers the paints, runsBuilt counts the runs computed (tests).
	runMemo   map[runKey]*runMemo
	runGen    uint64
	runsBuilt uint64
	// bgPairs is this frame's bracket pairs with guides (bgOn: bracket
	// pair guides are on), bgRows their guides on the laid out rows,
	// bgc their cache (bracketguides.go).
	bgPairs []guidePair
	bgOn    bool
	bgRows  []bgRow
	bgc     bgCache
	// guideActive caches the active guide of a caret outside the view;
	// guideScans counts its whole-block searches (tests).
	guideActive activeKey
	guideScans  uint64
	wsMarks     *whitespaceMarks
	match       []brange
	matchRes    []caretMatch // per selection, for matchKey
	matchKey    matchKey
	matchOK     bool
	matchTries  int       // frames matchKey requested while unresolved
	matchFrom   int       // the caret the next frame's match starts with
	matchPend   matchKey  // the key awaiting matchDelay
	matchDue    time.Time // when it is due

	// dmg is what the last paint drew, for the redraw hints (damage.go);
	// dmgSpare holds the buffers of the paint before it, for reuse.
	dmg, dmgSpare damageRec
	glyphInk      map[glyphInkKey]spanX

	// Display decorations (paint.go, occurrences.go, unicode.go, links.go,
	// colors.go): the occurrences and selection match states, scratch
	// buffers, the links and swatches of the last paint (for the
	// pointer), the active link, the link pressed with the trigger
	// modifier, the host's link opener and the open color picker.
	occ                    occState
	selHL                  selHLState
	occMatches, selMatches documentMatches
	scanNow                func() time.Time // the searches' budget clock (nil: scanClock; tests)
	decoLines              []int            // scratch: the laid out model lines
	decoRanges             []brange
	decoToks               []monarch.Token
	decoLinks              []linkSpan
	linkHits               []linkHit
	swatches               []swatchHit
	// Per-frame scratch of the rows' color literals, injected gaps and
	// their seg pieces (injected.go), reused while arena is set (paint's
	// layout of the rows).
	fColors    []colorSpan
	fInj       []injection
	fSegs      []seg
	arena      bool
	activeLink linkKey
	linkDown   *linkHit
	openLink   func(url string)
	picker     *pickerState
	pickerID   ui.ID
	zoomG      zoomGestureState
	decoCount  [decoSwatch + 1]int // decorations of the laid out lines, per kind (tests)

	// The Model's decorations (decorations.go): the query and inline
	// scratch, the shaped gutter signs and the overview ruler's marks.
	udq   []decorationEntryOf
	udIn  []inlineSpan
	signs map[signKey]*text.Layout
	udOv  udOverview

	// View zones (zones.go): this frame's fingerprints of the drawings of
	// the zones without a Version, a count of frames with drawings the
	// fingerprint does not describe (damageGlobal), the zone pressed and
	// the pointer position last seen by the zones.
	zfp        map[*ViewZone]uint64
	zoneOpaque uint64
	zonePress  *ViewZone
	zonePtr    geom.Point
}

func newViewState() viewState {
	return viewState{lines: text.NewLineCache(lineCacheSize), long: map[longKey]*longLine{}, foldX: -1, placeholderPress: -1, activeLink: linkKey{line: -1}}
}

// shapeHook, when set (tests), sees every LineCache call the View makes for
// a document line (a chunk of it for long lines).
var shapeHook func(line int, s string)

// View shows st. The application holds st; the View sets st.Viewport's
// measured sizes and scrolls it.
func View(c *ui.Context, st *State) *ViewW {
	w := &ViewW{c: c, st: st, opts: st.Options}
	w.opts.zoom = ZoomLevel()
	st.frameTab = 0
	st.view = w
	w.box = ui.Custom(c, w.measure, w.paint, layout.Grow(1))
	w.box.Clip()
	// Describe the focusable custom surface as a code editor, not the
	// Box.Focusable fallback button. Hosts can override the name on Box().
	name := st.Model.URI()
	if name == "" {
		name = "Code editor"
	}
	w.box.Semantics(a11y.RoleTextbox, name).A11yState(a11y.Multiline)
	w.vs = ui.State(c, w.box.ID(), newViewState)
	// While dragging selected text the pointer is an arrow, or the copy
	// arrow while the trigger modifier is held (Monaco's mouseStyle
	// "default" / "copy"; dnd.go).
	cur := input.CursorText
	if d := w.vs.dnd; d != nil && d.moved {
		cur = input.CursorDefault
		if w.dndCopy(d) {
			cur = input.CursorCopy
		}
	}
	if w.linkHover() {
		cur = input.CursorPointer // over a link with the trigger modifier (links.go)
	}
	if w.vs.dnd == nil {
		if w.gutterHover() {
			cur = input.CursorDefault // the gutter: the arrow, as Monaco's margin
		}
		if w.foldHover() {
			cur = input.CursorPointer // a folding control or placeholder (input.go)
		}
	}
	if p, in := c.Pointer(); in {
		if zc, ok := w.zoneCursor(p); ok {
			cur = zc // over a view zone (zones.go)
		}
	}
	w.ia = w.box.Focusable(cur)
	w.foldOpacity = 1
	if w.opts.AnimateFoldingControls && w.foldControlsShown() && w.opts.foldingControls() == FoldingControlsMouseOver {
		p, in := c.Pointer()
		target := float32(0)
		if in && w.ia.Hovered && w.vs.gutter.Contains(p) {
			target = 1
		}
		w.foldOpacity = c.Anim(w.box.ID(), "fold-controls", target, 500*time.Millisecond)
	}
	w.collect()
	wheel := false
	if dx, dy, ok := c.WheelFor(w.box.ID()); ok && (dx != 0 || dy != 0) {
		if !w.opts.MouseWheelZoom || !w.zoomGesture(dy) {
			st.ScrollBy(dy, dx)
			wheel = true
		}
	}
	w.contextMenu(wheel)
	w.colorPicker()  // the color picker opened from a swatch (colors.go)
	st.checkScroll() // the wheel (scroll.go)
	return w
}

// Editor zoom (Monaco's EditorZoom): one zoom level shared by every
// editor, from -5 to 20; the font size and line height scale by
// 1 + level/10. editor.action.fontZoomIn / fontZoomOut step it by one,
// editor.action.fontZoomReset sets it to 0, and with
// Options.MouseWheelZoom Cmd+wheel (Ctrl+wheel elsewhere) changes it by
// one level per zoomWheelPx of wheel distance (zoomGesture).
var zoomBits atomic.Uint64

// ZoomLevel returns the editor zoom level (0 by default).
func ZoomLevel() float64 { return math.Float64frombits(zoomBits.Load()) }

// SetZoomLevel sets the editor zoom level, clamped to [-5, 20]. Views
// apply it from their next frame; the application redraws the windows
// that show editors other than the one that changed it.
func SetZoomLevel(z float64) {
	z = max(-5, min(z, 20))
	zoomBits.Store(math.Float64bits(z))
}

func init() {
	step := func(id, label string, f func(z float64) float64) {
		RegisterCommand(Command{ID: id, Label: label, Run: func(st *State, _ any) bool {
			SetZoomLevel(f(ZoomLevel()))
			if w := st.view; w != nil {
				ui.Invalidate(w.c.Window())
			}
			return true
		}})
	}
	step("editor.action.fontZoomIn", "Increase Editor Font Size", func(z float64) float64 { return z + 1 })
	step("editor.action.fontZoomOut", "Decrease Editor Font Size", func(z float64) float64 { return z - 1 })
	step("editor.action.fontZoomReset", "Reset Editor Font Size", func(float64) float64 { return 0 })
}

// zoomMods reports whether the wheel's modifiers zoom (Monaco's
// hasMouseWheelZoomModifiers): Cmd or Ctrl without Shift and Alt on
// macOS, Ctrl alone elsewhere.
func (w *ViewW) zoomMods(m input.Modifiers) bool {
	if m&(input.ModShift|input.ModAlt) != 0 {
		return false
	}
	if w.keymap().Platform() == keymap.Mac {
		return m&(input.ModMeta|input.ModCtrl) != 0
	}
	return m&input.ModCtrl != 0 && m&input.ModMeta == 0
}

// Wheel zoom gestures (Monaco's _setupMouseWheelZoomListener for wheels
// it does not classify as a physical mouse wheel, the only path here as
// input.Scroll does not tell them apart): wheel frames less than
// zoomGestureGap apart are one gesture; whether it zooms is decided by the
// modifiers at its start (so the inertia of a trackpad scroll does not zoom
// when Cmd is pressed during it, nor scroll when Cmd is released), and the
// zoom level is the gesture's starting level plus the gesture's wheel
// distance over zoomWheelPx (fractional levels, as Monaco's).
const (
	zoomGestureGap = 50 * time.Millisecond
	// zoomWheelPx is the wheel distance of one zoom level, in logical
	// pixels: Monaco divides pixel wheel deltas by 40 to normalize them,
	// then accumulates deltaY/5 for non-physical wheels.
	zoomWheelPx = 40 * 5
)

// zoomGestureState is the wheel zoom gesture in progress.
type zoomGestureState struct {
	last  time.Time // the gesture's last wheel frame
	start float64   // the zoom level at its start
	acc   float32   // its wheel distance (positive: down, zooms out)
	zoom  bool      // it zooms (modifiers at its start)
}

// zoomGesture applies a wheel turn of dy (up, negative, zooms in) to the
// gesture in progress and reports whether it zooms; it applies the new
// level to this frame.
func (w *ViewW) zoomGesture(dy float32) bool {
	g := &w.vs.zoomG
	now := w.c.Now()
	if g.last.IsZero() || now.Sub(g.last) > zoomGestureGap {
		*g = zoomGestureState{start: ZoomLevel(), zoom: w.zoomMods(w.c.PointerMods())}
	}
	g.last = now
	if !g.zoom {
		return false
	}
	g.acc += dy
	SetZoomLevel(g.start - float64(g.acc)/zoomWheelPx)
	w.opts.zoom = ZoomLevel()
	return true
}

// The modifiers below override the State's Options for this frame only
// (immediate mode): a frame that does not call them uses st.Options, the
// persistent settings the application sets.

// LineNumbers shows or hides the line numbers this frame (default:
// st.Options.LineNumbers).
func (w *ViewW) LineNumbers(on bool) *ViewW {
	w.opts.LineNumbers = LineNumbersOff
	if on {
		w.opts.LineNumbers = LineNumbersOn
	}
	return w
}

// FontSize sets the font size in logical pixels this frame (default:
// st.Options.FontSize); values <= 0 are ignored.
func (w *ViewW) FontSize(px float32) *ViewW {
	if px > 0 {
		w.opts.FontSize = px
	}
	return w
}

// TabSize sets the tab width in columns this frame (default: the Model's
// TabSize), for drawing and for the caret movement the State does until
// the next frame; values <= 0 are ignored. It does not change the Model's
// TabSize, which Tab and the indentation commands use.
func (w *ViewW) TabSize(n int) *ViewW {
	if n > 0 {
		w.st.frameTab = n
	}
	return w
}

// ReadOnly makes the view refuse edits this frame (typing, deleting, Enter,
// Tab, paste, cut, undo, redo, IME); the caret, selection, navigation and
// copy keep working. A frame that does not call it is read-only only when
// st.Options.ReadOnly is set.
func (w *ViewW) ReadOnly(on bool) *ViewW { w.opts.ReadOnly = on; return w }

// Keymap sets the keymap that turns this frame's keys into commands
// (default: keymap.DefaultRules for the current platform, with a chord
// state of the view's own). Keep the same Keymap across frames: it holds
// the pending chord. Several views may share one; a view that loses focus
// clears its pending chord.
func (w *ViewW) Keymap(km *keymap.Keymap) *ViewW {
	if km != nil {
		w.km = km
	}
	return w
}

// Context adds application context keys for this frame's keybinding when
// clauses and command preconditions: fn is asked before the built-in keys
// (see contextkeys.go) and reports false for keys it does not define.
func (w *ViewW) Context(fn func(key string) (any, bool)) *ViewW {
	w.ctxFn = fn
	return w
}

// defaultKeymap is the compiled default keymap; views clone it.
var defaultKeymap = sync.OnceValue(func() *keymap.Keymap {
	p := keymap.Current()
	return keymap.New(p, keymap.DefaultRules(p))
})

// keymap returns the keymap in effect this frame.
func (w *ViewW) keymap() *keymap.Keymap {
	if w.km != nil {
		return w.km
	}
	vs := w.vs
	if vs.defKeymap == nil {
		vs.defKeymap = defaultKeymap().Clone()
		c := w.c
		vs.defKeymap.SetClock(c.Now)
	}
	return vs.defKeymap
}

// active reports whether the view has keyboard focus in an active window.
func (w *ViewW) active() bool { return w.ia.Focused && w.c.WindowFocused() }

// Layout applies layout options to the view's box (it grows by default).
func (w *ViewW) Layout(opts ...layout.Option) *ViewW { w.box.Style(opts...); return w }

// Box returns the view's box (ID, rect).
func (w *ViewW) Box() *ui.Box { return w.box }

// Changed reports whether the text was edited through the View this
// frame. It applies the frame's input, so call it after the modifiers
// (ReadOnly ...); without a call the input is applied when the view paints.
func (w *ViewW) Changed() bool { w.finish(); return w.changed }

// Focused reports whether the view has keyboard focus.
func (w *ViewW) Focused() bool { return w.ia.Focused }

func (w *ViewW) measure(maxW, maxH float32) geom.Size {
	sz := geom.Size{W: 400, H: 300}
	if !math.IsInf(float64(maxW), 0) && maxW > 0 {
		sz.W = maxW
	}
	if !math.IsInf(float64(maxH), 0) && maxH > 0 {
		sz.H = maxH
	}
	return sz
}

// lineHeight returns the line height for the font size.
func (w *ViewW) lineHeight() float32 {
	if h := w.opts.LineHeight; h > 0 {
		return max(1, float32(math.Round(float64(h)*(1+w.opts.zoom*0.1))))
	}
	return float32(math.Round(float64(w.opts.fontSize()) * lineHeightRatio))
}

func (w *ViewW) cursorWidth() float32 {
	if w.opts.CursorWidth > 0 {
		return w.opts.CursorWidth
	}
	return 2
}

// displayViewport uses whole device pixels for the scroll displacement so
// the renderer can copy the previous frame. Keep the input position in
// State continuous, including the part of a wheel delta still below a pixel.
// Keep the same top row so it still refers to the same view-line anchor.
func (w *ViewW) displayViewport() Viewport {
	v := w.st.Viewport
	scale := float64(max(0.01, w.c.PixelScale()))
	top := float64(v.TopLine) * float64(v.LineHeight) * scale
	v.OffsetY = float32((math.Round(top+float64(v.OffsetY)*scale) - top) / scale)
	v.ScrollX = float32(math.Round(float64(v.ScrollX)*scale) / scale)
	return v
}

// monoStyle returns the text style of the document text.
func (w *ViewW) monoStyle() text.Style {
	return text.Style{Size: w.opts.fontSize(), LineHeight: w.lineHeight(), Mono: true, FontFamily: w.opts.FontFamily}
}

// tabs returns the tab width in columns.
func (w *ViewW) tabs() int { return w.st.tabSize() }
