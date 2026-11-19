package diffview

import (
	"github.com/tangoui/tango/ui"
	"slices"
	"sync"
	"sync/atomic"
	"time"

	"github.com/tangoui/tango/ui/editor"
	"github.com/tangoui/tango/ui/editor/diff"
	"github.com/tangoui/tango/ui/editor/keymap"
	"github.com/tangoui/tango/ui/editor/theme"
)

// Mode is how a diff view shows the two texts.
type Mode uint8

const (
	// SideBySide shows the original on the left, the modified text on
	// the right (Monaco renderSideBySide: true).
	SideBySide Mode = iota
	// Inline shows the modified text only, with the deleted lines of the
	// original between its lines.
	Inline
)

// UnchangedRegionsOptions folds the unchanged lines between changes
// (Monaco's hideUnchangedRegions).
type UnchangedRegionsOptions struct {
	Enabled bool // false (Monaco's default; hosts showing AI edits turn it on)
	// ContextLineCount unchanged lines stay visible next to every change
	// (3); a run is only folded when it hides at least MinimumLineCount
	// lines (3); the placeholder's "show more" buttons reveal
	// RevealLineCount lines (20).
	ContextLineCount, MinimumLineCount, RevealLineCount int
}

// Options are a diff view's settings; DefaultOptions has Monaco's
// defaults (diffEditor.js). Change them on State.Options; they apply from
// the next frame.
type Options struct {
	Mode Mode // SideBySide
	// InlineBreakpoint is the width under which the view switches to
	// Inline when UseInlineWhenSpaceIsLimited (900 logical pixels).
	InlineBreakpoint            float32
	UseInlineWhenSpaceIsLimited bool // true
	IgnoreTrimWhitespace        bool // true
	// ComputeMoves detects moved blocks (false; forced off for large
	// files). Moves are not drawn yet.
	ComputeMoves       bool
	MaxComputationTime time.Duration // 5s (diff.DefaultTimeout)
	// Debounce is how long the view waits after an edit before it
	// recomputes the diff (200ms); meanwhile the last result is mapped
	// through the edits.
	Debounce             time.Duration
	HideUnchangedRegions UnchangedRegionsOptions
	// OriginalEditable lets the user edit the original side (false: it is
	// read-only).
	OriginalEditable bool
	// RenderIndicators draws "+" and "−" in the changed lines' gutters
	// (true).
	RenderIndicators bool
	// RenderMarginRevertIcon draws a revert arrow next to every change
	// while the modified side is editable (true).
	RenderMarginRevertIcon bool
	// RenderOverviewRuler draws the merged two-lane overview ruler right of
	// the modified side in SideBySide mode (true).
	RenderOverviewRuler bool
	// DiffWordWrap wraps both sides ("": each side's own Options.WordWrap,
	// Monaco's "inherit").
	DiffWordWrap editor.WrapMode
}

// DefaultOptions returns Monaco's defaults.
func DefaultOptions() Options {
	return Options{
		InlineBreakpoint:            900,
		UseInlineWhenSpaceIsLimited: true,
		IgnoreTrimWhitespace:        true,
		MaxComputationTime:          diff.DefaultTimeout,
		Debounce:                    200 * time.Millisecond,
		HideUnchangedRegions:        UnchangedRegionsOptions{ContextLineCount: 3, MinimumLineCount: 3, RevealLineCount: 20},
		RenderIndicators:            true,
		RenderMarginRevertIcon:      true,
		RenderOverviewRuler:         true,
	}
}

// State is a diff view of two Models: the editor states of both sides
// and the latest diff. The application holds it, shows it with View and
// closes it with Close. Use it on the UI goroutine only.
type State struct {
	// Original and Modified are the views of both sides. In Inline mode
	// Original is not shown but kept (switching back restores it). The
	// diff view owns some of their Options (folding, minimap, overview
	// ruler lanes, sticky scroll while folding unchanged regions, the
	// original's ReadOnly, the font size and padding of the original) and
	// sets them every frame.
	Original, Modified *editor.State
	Options            Options

	models [2]*editor.Model
	run    *runner
	cfg    computeConfig
	wakeFn atomic.Pointer[func()]
	unsub  []func()
	closed bool

	// res describes the current texts: the last computed result, mapped
	// through the edits made since (applyEdits) while the next one is due.
	res    diff.Result
	hasRes bool
	first  bool // no result taken yet (regions start collapsed)
	// gen counts changes of res; shapeGen only those that move lines
	// between the sides or resize changes (see sameShape): decorations
	// and zones follow the text by themselves otherwise.
	gen, shapeGen uint64
	updated       listeners[func(diff.Result)]

	regions   regionSet
	regionGen uint64

	// The view's applied state (layout.go).
	lay layoutState

	// Scrolling (view.go): the side that scrolled last, positions the
	// diff view set (their OnDidScroll reports are its own), whether the
	// frame is in its build, and a pending resync.
	lead     diff.Side
	expect   [2]scrollPos
	inBuild  bool
	needSync bool

	focus     diff.Side // the side focused last
	nav       navState
	frameMode Mode // the mode this frame's tree was built for
	modeOver  *Mode
	actions   func(i int, ch diff.Change) []HunkAction
	labels    [2]string
	lanes     int     // the Modified's overview ruler lanes before New took them
	width     float32 // the view's width in the last frame
	boxID     ui.ID
	km        *keymap.Keymap
	focusReq  *diff.Side
}

type scrollPos struct {
	line      int
	off, left float32
	ok        bool
}

// live maps the editor states of open diff views to them (commands).
// Diff views of windows on different UI threads open and close at the
// same time, so it is locked; it is touched only when a view opens or
// closes and when a diff command runs.
var live struct {
	sync.Mutex
	m map[*editor.State]*State
}

// liveView returns the open diff view showing st, or nil.
func liveView(st *editor.State) *State {
	live.Lock()
	defer live.Unlock()
	return live.m[st]
}

// New starts a diff view of original and modified, both drawn with th.
// The two Models are independent; modified may be open in other editors
// as well (the diff's decorations, zones and hidden areas belong to this
// view's States and do not show there). The first diff is computed right
// away in the background.
func New(original, modified *editor.Model, th *theme.Theme) *State {
	d := &State{
		Original: editor.NewView(original, th),
		Modified: editor.NewView(modified, th),
		Options:  DefaultOptions(),
		models:   [2]*editor.Model{original, modified},
		first:    true,
		lead:     diff.Modified,
		focus:    diff.Modified,
	}
	d.lay.revertHover = -1
	d.nav.lastChange = -1
	d.lanes = d.Modified.Options.OverviewRulerLanes
	d.cfg = d.computeConfig()
	d.run = newRunner(nil, d.Options.Debounce, d.cfg.compute(), func() {
		if f := d.wakeFn.Load(); f != nil {
			(*f)()
		}
	})
	for s, m := range d.models {
		side := diff.Side(s)
		d.unsub = append(d.unsub, m.Subscribe(func(ch editor.ModelChange) { d.modelChanged(side, ch) }))
	}
	live.Lock()
	if live.m == nil {
		live.m = map[*editor.State]*State{}
	}
	for s, st := range d.sides() {
		side := diff.Side(s)
		d.unsub = append(d.unsub, st.OnDidScroll(func(ev editor.ScrollEvent) { d.scrolled(side, ev) }))
		live.m[st] = d
	}
	live.Unlock()
	d.run.now(original.Snapshot(), modified.Snapshot())
	return d
}

func (d *State) sides() [2]*editor.State { return [2]*editor.State{d.Original, d.Modified} }

func (d *State) side(s diff.Side) *editor.State {
	if s == diff.Original {
		return d.Original
	}
	return d.Modified
}

// computeConfig returns what the computation needs from the options.
func (d *State) computeConfig() computeConfig {
	return computeConfig{
		ignoreTrimWhitespace: d.Options.IgnoreTrimWhitespace,
		computeMoves:         d.Options.ComputeMoves && !d.models[0].LargeFile() && !d.models[1].LargeFile(),
		timeout:              d.Options.MaxComputationTime,
	}
}

// Close cancels the background computation and closes both editor
// states. The State is not usable afterwards.
func (d *State) Close() {
	if d.closed {
		return
	}
	d.closed = true
	d.run.close()
	for _, f := range d.unsub {
		f()
	}
	live.Lock()
	for _, st := range d.sides() {
		delete(live.m, st)
	}
	live.Unlock()
	for _, st := range d.sides() {
		st.Close()
	}
}

// Result returns the diff of the current texts and whether it was
// computed for them: while a recomputation is due after an edit, it is
// the previous result mapped through the edits (changed lines near the
// edits are approximate).
func (d *State) Result() (r diff.Result, upToDate bool) {
	return d.res, d.hasRes && d.run.upToDate()
}

// OnDidUpdateDiff calls fn on the UI goroutine with every newly computed
// diff (Monaco's onDidUpdateDiff), from the frame that takes it; cancel
// removes fn.
func (d *State) OnDidUpdateDiff(fn func(diff.Result)) (cancel func()) { return d.updated.add(fn) }

// modelChanged maps the current result and the folding state through an
// edit of side s and schedules the recomputation.
func (d *State) modelChanged(s diff.Side, ch editor.ModelChange) {
	if d.closed {
		return
	}
	if len(ch.Edits) > 0 && d.hasRes {
		old := d.res
		d.res = applyEdits(d.res, s, ch.Old, ch.Edits)
		d.gen++
		if !sameShape(old, d.res) {
			d.shapeGen++
		}
		hidden := mapHidden(d.regions.hiddenMaps(), s, ch.Old, ch.Edits)
		d.setRegions(d.res, hidden, false)
	}
	d.run.configure(d.Options.Debounce, nil)
	d.run.schedule(d.models[0].Snapshot(), d.models[1].Snapshot())
}

// sameShape reports whether b has the changes of a with the same line
// counts on both sides: what an edit inside a changed block, or one that
// only shifts lines, leaves. The decorations, hidden areas and zones the
// view set for a follow the text through such an edit by themselves.
func sameShape(a, b diff.Result) bool {
	if len(a.Changes) != len(b.Changes) {
		return false
	}
	for i := range a.Changes {
		x, y := a.Changes[i], b.Changes[i]
		if x.Original.Len() != y.Original.Len() || x.Modified.Len() != y.Modified.Len() {
			return false
		}
	}
	return true
}

// take collects a finished computation (on the UI goroutine).
func (d *State) take() bool {
	if cfg := d.computeConfig(); cfg != d.cfg {
		d.cfg = cfg
		d.run.configure(d.Options.Debounce, cfg.compute())
		d.run.now(d.models[0].Snapshot(), d.models[1].Snapshot())
		return false
	}
	out, ok := d.run.take()
	if !ok {
		return false
	}
	d.res, d.hasRes = out.result, true
	d.gen++
	d.shapeGen++
	d.setRegions(d.res, d.regions.hiddenMaps(), d.first)
	d.first = false
	res := d.res
	d.updated.each(func(fn func(diff.Result)) { fn(res) })
	return true
}

// setRegions recomputes the foldable regions of r, keeping hidden what
// hidden (already mapped to the current texts) hides.
func (d *State) setRegions(r diff.Result, hidden []hiddenMap, first bool) {
	o := d.Options.HideUnchangedRegions
	regions := migrate(regionsOf(r, max(o.ContextLineCount, 0), max(o.MinimumLineCount, 1)), hidden, first)
	if !slices.Equal(regions, d.regions.regions) {
		d.regions.regions = regions
		d.regionGen++
	}
}

// ShowAllUnchanged reveals every folded unchanged region.
func (d *State) ShowAllUnchanged() {
	d.regions.showAll()
	d.regionGen++
	d.wake()
}

// CollapseAllUnchanged folds every unchanged region again.
func (d *State) CollapseAllUnchanged() {
	d.regions.collapseAll()
	d.regionGen++
	d.wake()
}

// wake requests a frame (after a change made outside a frame).
func (d *State) wake() {
	if d.inBuild {
		return
	}
	if f := d.wakeFn.Load(); f != nil {
		(*f)()
	}
}

// listeners is a list of callbacks that may be removed while it runs.
type listeners[F any] struct {
	list []*F
}

func (l *listeners[F]) add(fn F) func() {
	p := &fn
	l.list = append(l.list, p)
	return func() {
		l.list = slices.DeleteFunc(l.list, func(q *F) bool { return q == p })
	}
}

func (l *listeners[F]) each(call func(F)) {
	for _, p := range slices.Clone(l.list) {
		call(*p)
	}
}
