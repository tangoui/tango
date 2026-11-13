package ui

import (
	"github.com/tangoui/tango/ui/input"
)

// focusID is the keyboard target of the table (its frame).
func (f *tblFrame) focusID() ID { return f.frameID }

func (f *tblFrame) firstDataLeaf() int {
	if len(f.data) > 0 {
		return f.data[0]
	}
	return 0
}

// keys handles keyboard navigation while the table has focus: arrows move
// the active cell (Left / Right visually), Home/End (first / last column,
// logical) and PageUp/PageDown jump, Space toggles the row's
// selection, Enter edits (or expands), Alt+Up / Alt+Down move the row of a
// drag-sorted table and Ctrl/Cmd+C copies the cell.
func (f *tblFrame) keys() {
	c, st, t := f.c, f.st, f.t
	keys := c.KeysFor(f.frameID)
	if len(keys) == 0 || f.n == 0 || len(f.leaves) == 0 {
		return
	}
	if !st.actOn {
		st.actPos, st.actCol, st.actOn = max(0, f.first), f.firstDataLeaf(), true
	}
	st.actPos = max(0, min(st.actPos, f.n-1))
	st.actCol = max(0, min(st.actCol, len(f.leaves)-1))
	page := max(1, int(f.viewH/f.rh)-1)
	if !f.bounded {
		page = 10
	}
	for _, k := range keys {
		if !k.Down {
			continue
		}
		short := k.Mods.Has(input.ModCtrl) || k.Mods.Has(input.ModMeta)
		if k.Mods.Has(input.ModAlt) && t.dragSort != nil && (k.Key == input.KeyUp || k.Key == input.KeyDown) {
			// Keyboard alternative of the drag handle: move the active row.
			to := st.actPos - 1
			if k.Key == input.KeyDown {
				to = st.actPos + 1
			}
			if to >= 0 && to < f.n {
				t.dragSort(f.row(st.actPos).data, f.row(to).data)
				t.movedCh = true
				st.actPos = to
			}
			continue
		}
		// Left / Right move visually: columns are mirrored in RTL, so
		// Left goes to the next (logical) column there.
		if prev, next := c.inlineKey(k.Key); prev || next {
			if prev {
				st.actCol = max(0, st.actCol-1)
			} else {
				st.actCol = min(len(f.leaves)-1, st.actCol+1)
			}
			continue
		}
		switch k.Key {
		case input.KeyUp:
			st.actPos = max(0, st.actPos-1)
		case input.KeyDown:
			st.actPos = min(f.n-1, st.actPos+1)
		case input.KeyHome:
			if short {
				st.actPos = 0
			} else {
				st.actCol = 0
			}
		case input.KeyEnd:
			if short {
				st.actPos = f.n - 1
			} else {
				st.actCol = len(f.leaves) - 1
			}
		case input.KeyPageUp:
			st.actPos = max(0, st.actPos-page)
		case input.KeyPageDown:
			st.actPos = min(f.n-1, st.actPos+page)
		case input.KeySpace:
			r := f.row(st.actPos)
			if f.hasSelection() && f.selectable(r) {
				f.setSelected(r, t.single || !f.selected(r))
			}
		case input.KeyEnter:
			r := f.row(st.actPos)
			if f.maybeStartEdit(r, st.actCol) {
				break
			}
			switch {
			case f.tree != nil && r.kids:
				ks := f.openKeys()
				*ks = tblToggle(*ks, r.key, !r.open)
				t.expCh = true
			case t.expRender != nil && f.canExpand(r):
				ks := f.expKeys()
				*ks = tblToggle(*ks, r.key, !f.expSet[r.key])
				t.expCh = true
			default:
				t.clickRow = r.data
				if t.highlight {
					st.cur, st.curOn = r.key, true
				}
			}
		case input.KeyC:
			if short {
				f.copyCell(st.actPos, st.actCol)
			}
		}
	}
	st.kbd = true
	c.invalid = true
	f.ensureVisible(st.actPos, st.actCol)
}

// copyCell writes a cell's text to the clipboard.
func (f *tblFrame) copyCell(pos, li int) {
	if pos < 0 || pos >= f.n || li < 0 || li >= len(f.leaves) {
		return
	}
	col := f.leaves[li].col
	if col == nil || f.c.win == nil {
		return
	}
	f.c.win.WriteClipboard(editValue(col, f.row(pos).data))
}
