package diffview

import (
	"errors"
	"sort"

	"github.com/tangoui/tango/ui"
	"github.com/tangoui/tango/ui/a11y"
	"github.com/tangoui/tango/ui/editor"
	"github.com/tangoui/tango/ui/editor/buffer"
	"github.com/tangoui/tango/ui/editor/diff"
	"github.com/tangoui/tango/ui/editor/keymap"
)

// Navigation, revert and the diff view's commands
// (docs/specs/diff-view.md, 3.8, 3.10, 3.12).

// Command IDs (Monaco's, except the navigation, which is VS Code's
// diffReview pair bound to Alt+F5 / Shift+Alt+F5).
const (
	CmdNext            = "editor.action.diffReview.next"
	CmdPrev            = "editor.action.diffReview.prev"
	CmdRevert          = "diffEditor.revert"
	CmdSwitchSide      = "diffEditor.switchSide"
	CmdToggleUnchanged = "diffEditor.toggleCollapseUnchangedRegions"
	CmdShowAll         = "diffEditor.showAllUnchangedRegions"
	CmdCollapseAll     = "diffEditor.collapseAllUnchangedRegions"
)

// diffRules are the diff view's keybindings, on top of the editor's.
var diffRules = []keymap.Rule{
	{Command: CmdNext, Key: "alt+f5", When: "isInDiffEditor"},
	{Command: CmdPrev, Key: "shift+alt+f5", When: "isInDiffEditor"},
}

func init() {
	cmd := func(id, label string, run func(d *State, st *editor.State) bool) {
		editor.RegisterCommand(editor.Command{ID: id, Label: label, Precondition: "isInDiffEditor", Run: func(st *editor.State, _ any) bool {
			d := live[st]
			if d == nil {
				return false
			}
			return run(d, st)
		}})
	}
	cmd(CmdNext, "Go to Next Difference", func(d *State, st *editor.State) bool { d.focus = d.sideOf(st); return d.Next() })
	cmd(CmdPrev, "Go to Previous Difference", func(d *State, st *editor.State) bool { d.focus = d.sideOf(st); return d.Prev() })
	cmd(CmdRevert, "Revert Block", func(d *State, st *editor.State) bool {
		i := d.changeAt(d.sideOf(st), caretLine(st))
		return i >= 0 && d.Revert(i) == nil
	})
	cmd(CmdSwitchSide, "Switch Side", func(d *State, st *editor.State) bool { return d.switchSide(d.sideOf(st)) })
	cmd(CmdToggleUnchanged, "Toggle Collapse Unchanged Regions", func(d *State, _ *editor.State) bool {
		d.Options.HideUnchangedRegions.Enabled = !d.Options.HideUnchangedRegions.Enabled
		d.wake()
		return true
	})
	cmd(CmdShowAll, "Show All Unchanged Regions", func(d *State, _ *editor.State) bool { d.ShowAllUnchanged(); return true })
	cmd(CmdCollapseAll, "Collapse All Unchanged Regions", func(d *State, _ *editor.State) bool { d.CollapseAllUnchanged(); return true })
}

func (d *State) sideOf(st *editor.State) diff.Side {
	if st == d.Original {
		return diff.Original
	}
	return diff.Modified
}

// keymap returns the keymap of both editors: the editor's defaults and
// the diff view's bindings (one chord state for both).
func (d *State) keymap(c *ui.Context) *keymap.Keymap {
	if d.km == nil {
		p := keymap.Current()
		d.km = keymap.New(p, keymap.DefaultRules(p), diffRules)
		d.km.SetClock(c.Now)
	}
	return d.km
}

// navState is the announcement due and the change the caret was in.
type navState struct {
	pending    string
	lastChange int
	lastLine   [2]int
	lastSel    [2]editor.Selection // primary selections seen last frame
	ok         bool
}

func caretLine(st *editor.State) int {
	return st.Snapshot().LineOf(st.Primary().Head)
}

// navSide is the side navigation follows: the focused one (always the
// modified in Inline mode).
func (d *State) navSide() diff.Side {
	if d.frameMode == Inline {
		return diff.Modified
	}
	return d.focus
}

// Next moves the caret of the focused side to the next change (wrapping
// around), reveals and announces it; false without changes.
func (d *State) Next() bool { return d.step(true) }

// Prev moves to the previous change, as Next.
func (d *State) Prev() bool { return d.step(false) }

func (d *State) step(next bool) bool {
	s := d.navSide()
	if !d.hasRes || len(d.res.Changes) == 0 {
		d.nav.pending = d.lay.texts.ann.none
		d.wake()
		return false
	}
	line := caretLine(d.side(s))
	var i int
	var ok bool
	if next {
		i, ok = d.res.Next(s, line)
	} else {
		i, ok = d.res.Prev(s, line)
	}
	if !ok {
		return false
	}
	d.goTo(i, s)
	return true
}

// GoTo moves the caret of the focused side to change i, as Next.
func (d *State) GoTo(i int) {
	if !d.hasRes || i < 0 || i >= len(d.res.Changes) {
		return
	}
	d.goTo(i, d.navSide())
}

func (d *State) goTo(i int, s diff.Side) {
	c := d.res.Changes[i]
	lr := c.Original
	if s == diff.Modified {
		lr = c.Modified
	}
	st := d.side(s)
	snap := st.Snapshot()
	line := max(0, min(lr.Start, snap.LineCount()-1))
	off := snap.LineStart(line)
	st.SetSelections([]editor.Selection{{Anchor: off, Head: off}})
	if d.regions.ensureVisible(s, line, revealCloser) {
		d.regionGen++
	}
	d.apply(d.frameMode)
	st.RevealLine(line, editor.RevealCenterIfOutside)
	d.lead = s
	if !d.syncFrom(s) {
		d.needSync = true
	}
	d.nav.pending = announceChange(d.lay.texts.ann, i, len(d.res.Changes), c)
	d.nav.lastChange, d.nav.lastLine[s] = i, line
	d.wake()
}

// changeAt returns the change containing line of side s (an empty range
// contains the line it is before), -1 for none.
func (d *State) changeAt(s diff.Side, line int) int {
	cs := d.res.Changes
	i := sort.Search(len(cs), func(i int) bool { lr := rangeOf(cs[i], s); return lr.End > line || lr.Start >= line })
	for ; i < len(cs); i++ {
		lr := rangeOf(cs[i], s)
		if lr.Start > line {
			break
		}
		if lr.Contains(line) || lr.Empty() && lr.Start == line {
			return i
		}
	}
	return -1
}

func rangeOf(c diff.Change, s diff.Side) diff.LineRange {
	if s == diff.Original {
		return c.Original
	}
	return c.Modified
}

// Revert replaces change i's lines of the modified text with the
// original's (Monaco's "revert block"), as one undo step, and recomputes
// the diff right away. It fails while the modified side is read-only.
func (d *State) Revert(i int) error {
	if d.Modified.Options.ReadOnly {
		return errors.New("diffview: the modified side is read-only")
	}
	if !d.hasRes {
		return errors.New("diffview: no diff yet")
	}
	e, err := revertEdit(d.res, i, d.models[0].Snapshot(), d.models[1].Snapshot())
	if err != nil {
		return err
	}
	buf := d.models[1].Buffer()
	buf.PushUndoStop()
	at := e.Start + len(e.Text)
	if err := d.Modified.Edit([]buffer.Edit{e}, editor.Selection{Anchor: e.Start, Head: at}); err != nil {
		return err
	}
	buf.PushUndoStop()
	d.Modified.MoveCaret(e.Start, false)
	d.recompute()
	d.wake()
	return nil
}

// switchSide moves the caret to the other side, on the line aligned with
// the caret's (Monaco's diffEditor.switchSide), and focuses it.
func (d *State) switchSide(from diff.Side) bool {
	if d.frameMode == Inline || !d.hasRes || d.lay.al == nil {
		return false
	}
	to := 1 - from
	line := caretLine(d.side(from))
	al := d.lay.al
	b := al.blocks[al.blockAt(from, line)]
	lf, lt := b.lines(from), b.lines(to)
	target := lt.Start + min(max(line-lf.Start, 0), max(lt.Len()-1, 0))
	st := d.side(to)
	snap := st.Snapshot()
	target = max(0, min(target, snap.LineCount()-1))
	st.MoveCaret(snap.LineStart(target), false)
	d.focusReq = &to
	d.wake()
	return true
}

// revealSelections reveals the folded lines the primary carets went into
// (a click, a search match, go to line; Monaco's
// ensureModifiedLineIsVisible), and announces the change a caret moved
// into.
func (d *State) revealSelections(mode Mode) {
	if !d.hasRes {
		return
	}
	for s := diff.Original; s <= diff.Modified; s++ {
		if mode == Inline && s == diff.Original {
			continue
		}
		st := d.side(s)
		line := caretLine(st)
		sel := st.Primary()
		moved := sel != d.nav.lastSel[s]
		d.nav.lastSel[s] = sel
		// Only a caret that moved reveals (Monaco reacts to cursor
		// changes): the initial caret may sit in a folded region.
		if o, ok := st.HiddenAreaAt(line); ok && o == owner && moved && d.nav.ok {
			if d.regions.ensureVisible(s, line, revealCloser) {
				d.regionGen++
				d.apply(mode)
				st.RevealCaret()
				d.lead = s
				d.needSync = true
			}
		}
		if s != d.navSide() {
			continue
		}
		if d.nav.ok && line != d.nav.lastLine[s] && d.nav.pending == "" {
			i := d.changeAt(s, line)
			if i >= 0 && i != d.nav.lastChange {
				d.nav.pending = announceChange(d.lay.texts.ann, i, len(d.res.Changes), d.res.Changes[i])
			}
			d.nav.lastChange = i
		}
		d.nav.lastLine[s], d.nav.ok = line, true
	}
}

// announce sends the pending announcement to screen readers.
func (d *State) announce(c *ui.Context, mode Mode) {
	if d.nav.pending != "" {
		c.Announce(d.nav.pending, a11y.LivePolite)
		d.nav.pending = ""
	}
}
