package ui

import (
	"strings"

	"github.com/tangoui/tango/ui/a11y"
	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/icon"
	"github.com/tangoui/tango/ui/input"
	"github.com/tangoui/tango/ui/internal/flexbox"
	"github.com/tangoui/tango/ui/layout"
	"github.com/tangoui/tango/ui/paint"
	"github.com/tangoui/tango/ui/text"
	"github.com/tangoui/tango/ui/theme"
)

func (f *tblFrame) headerBg() theme.Color {
	if f.el && f.depth > 1 {
		return f.tk.RowHoverBg // Element tints grouped headers
	}
	return f.tk.HeaderBg
}

// header builds the (possibly multi-level) header. Non-fixed cells are
// built first so sticky cells paint above them.
func (f *tblFrame) header() {
	if f.depth == 0 {
		return // a Transposed table without TransposedHeader
	}
	c := f.c
	hdr := c.box("thead", layout.Column, nil)
	hdr.n.setRole(a11y.RoleRow) // one header row (grouped headers carry their own RowIndex)
	if c.a11yOn {
		hdr.n.fill(tblSem{row: 1})
	}
	hdr.n.ln.Height = f.hh * float32(f.depth)
	hdr.n.clip = true
	if f.t.sticky {
		hdr.n.ln.Absolute = true
		hdr.n.ln.Pos = flexbox.Pos{Left: 0, Top: f.headY, Right: 0, Bottom: layout.Unset}
		hdr.n.hit = true // covers the rows scrolled under it
	}
	// Ant rounds the header's top corners. Nothing clips to the rounded
	// shape (a rounded clip layer is expensive on the CPU): the header
	// background and its edge cells draw rounded corners themselves.
	hr := f.tk.HeaderBorderRadius
	rad := geom.Radii{TL: hr, TR: hr}
	hdr.Paint(func(l *paint.List, r geom.Rect) { l.Fill(r, rad, f.headerBg()) })
	hdr.Body(func() {
		for pass := int8(0); pass < 3; pass++ {
			for i := range f.heads {
				hd := &f.heads[i]
				fx := f.leaves[hd.first].fixed
				if (pass == 0 && fx != tfNone) || (pass == 1 && fx != tfLeft) || (pass == 2 && fx != tfRight) {
					continue
				}
				f.headCell(hd)
			}
		}
	})
}

func (f *tblFrame) headCell(hd *tblHead) {
	c, th, tk, t := f.c, f.th, f.tk, f.t
	w := f.spanW(hd.first, hd.last-hd.first+1)
	if f.culled(hd.first, w) {
		return
	}
	x := f.cx(hd.first) - f.sx
	y := float32(hd.row) * f.hh
	h := float32(hd.rows) * f.hh
	title := ""
	var col *TableColumn
	if hd.col != nil {
		col = hd.col
		title = col.Title
	}
	leaf := hd.first == hd.last && hd.kind == tkData && hd.rows+hd.row == f.depth
	n := c.add("th", title)
	n.ln.Absolute = true
	n.ln.Pos = flexbox.Pos{Left: x, Top: y, Right: layout.Unset, Bottom: layout.Unset}
	n.ln.Width, n.ln.Height = w, h
	n.ln.Dir = layout.Row
	n.ln.Align = layout.AlignCenter
	n.ln.Padding = geom.VH(0, f.ph)
	lastCol := hd.last == len(f.leaves)-1
	sortable := col != nil && leaf && (col.Sortable || col.Compare != nil)
	filterable := col != nil && leaf && (len(col.Filters) > 0 || col.FilterPanel != nil)
	order := SortNone
	if col != nil {
		order = f.sortOf[col.Key]
	}
	filtered := col != nil && len(f.filterOf[col.Key]) > 0
	n.setRole(a11y.RoleColumnHeader)
	var ia Interaction
	if sortable {
		// A Tab stop: Enter / Space sort like a click.
		ia = c.interact(n, true, input.CursorPointer)
		if ia.Clicked {
			f.clickSort(col)
			order = f.sortOf[col.Key]
		}
		if ia.Hovered && !f.el {
			loc := c.Locale()
			tip := loc.SortAsc
			switch order {
			case SortAsc:
				tip = loc.SortDesc
			case SortDesc:
				tip = loc.CancelSort
			}
			f.tip(n.id, tip, false)
		}
	}
	if c.a11yOn {
		sem := tblSem{row: hd.row + 1, col: hd.first + 1}
		switch order {
		case SortAsc:
			sem.sort = a11y.SortAscending
		case SortDesc:
			sem.sort = a11y.SortDescending
		}
		n.fill(sem)
	}
	special := hd.kind != tkData
	grouped := f.depth > 1
	align := AlignStart
	if col != nil {
		align = f.logicalAlign(col.Align)
		if !leaf {
			align = AlignCenter
		}
	}
	// Space reserved at the end for the sorter (Ant) and filter trigger.
	trigW := float32(0)
	if filterable {
		trigW = 20
		if f.el {
			trigW = 18
		}
	}
	var rad geom.Radii
	if hr := f.tk.HeaderBorderRadius; hr > 0 && hd.row == 0 {
		// x is logical: the start edge is the right one in RTL.
		start, end := &rad.TL, &rad.TR
		if f.rtl {
			start, end = end, start
		}
		if x <= 0.5 {
			*start = hr
		}
		if x+w >= f.viewW-0.5 {
			*end = hr
		}
	}
	n.paint = func(l *paint.List, r geom.Rect) {
		bg := f.headerBg()
		l.Fill(r, rad, bg)
		if !f.el && (order != SortNone || ia.Hovered) && tk.HeaderSortActiveBg.A > 0 {
			l.Fill(r, rad, tk.HeaderSortActiveBg)
		}
		l.Fill(geom.Rect{X: r.X, Y: r.Bottom() - 1, W: r.W, H: 1}, geom.Radii{}, f.spl)
		if ia.FocusVisible && !f.shad {
			l.Stroke(r.Inset(geom.All(1)), geom.Radii{}, 2, th.Primary.Base)
		}
		s := f.inl(r)
		endX := s.X(r.W-1, 1) // separator on the cell's end edge
		switch {
		case t.bordered || (grouped && !f.el):
			if !lastCol || !t.bordered {
				l.Fill(geom.Rect{X: endX, Y: r.Y, W: 1, H: r.H}, geom.Radii{}, f.spl)
			}
		case !f.el && !lastCol && !special && th.Flavor != theme.StyleShadcn:
			sh := float32(22)
			l.Fill(geom.Rect{X: endX, Y: r.Y + (r.H-sh)/2, W: 1, H: sh}, geom.Radii{}, tk.HeaderSplitColor)
		}
		if title == "" {
			return
		}
		ts := f.font(themeWeight(tk.HeaderFontWeight))
		tc := tk.HeaderColor
		inner := r.W - 2*f.ph - trigW
		sorterW := float32(0)
		if sortable {
			sorterW = 15
			if f.el {
				sorterW = 22
			}
			if f.shad {
				sorterW = shad2bSorterGap + shad2bSorterSize
			}
		}
		tw := min(text.Width(title, ts), max(0, inner-sorterW))
		// Offsets from the cell's start edge: title, then the sorter after
		// it (Ant start-aligned titles keep the sorter at the end).
		to := f.ph
		switch align {
		case AlignCenter:
			to += (inner - tw - sorterW) / 2
		case AlignEnd:
			to += inner - tw - sorterW
		}
		tx := s.X(to, tw+0.5)
		uo := f.c.textOpts(text.Options{}) // interface text: scope direction
		if sortable && f.shad {
			// shadcn data table: the title is a ghost button with the
			// sort arrow right after it.
			shad2bHeadSort(l, th, tk, s.Rect(to, r.Y, tw+sorterW, r.H), order, ia.Hovered, ia.FocusVisible, f.rtl)
			f.drawTextOpts(l, title, tx, r.Y, tw+0.5, r.H, AlignStart, ts, shad2bHeadText(th, tk, order, ia.Hovered), uo)
			return
		}
		f.drawTextOpts(l, title, tx, r.Y, tw+0.5, r.H, AlignStart, ts, tc, uo)
		if sortable {
			so := to + tw + 4
			if !f.el && align == AlignStart {
				so = f.ph + inner - 11
			}
			if f.el {
				so = to + tw + 5
			}
			w := float32(11)
			if f.el {
				w = 14
			}
			tblDrawSorter(th, tk, l, s.Rect(so, r.Y+(r.H-16)/2, w, 16), order)
		}
	}
	c.body(n, func() {
		switch {
		case hd.kind == tkSelect && !t.single && !t.hideAll:
			all, some := f.pageSelState()
			v := all
			Row(c, func() {
				if Checkbox(c, "", &v).Indeterminate(some).Disabled(f.n == 0).A11yLabel(c.Locale().SelectAll).Changed() {
					f.selectPage(v)
				}
				if t.selMenu {
					f.selectionMenu()
				}
			}, layout.Grow(1), layout.Center(), layout.Gap(2))
		case filterable:
			f.filterTrigger(col, w, h, filtered, align)
		}
		if leaf && t.resizable && col.Key != "" {
			f.resizeHandle(col, hd.first, w, h)
		}
	})
}

// filterTrigger adds the funnel button and its dropdown.
func (f *tblFrame) filterTrigger(col *TableColumn, cellW, cellH float32, active bool, align TextAlign) {
	c, th, st := f.c, f.th, f.st
	n := c.add("filter-trigger", col.Key)
	n.ln.Absolute = true
	tw, thh := float32(20), cellH-2*max(4, f.pv-4)
	left := cellW - f.ph + 4 - tw
	if f.el {
		// Element puts the arrow right after the title (and sorter).
		ts := f.font(text.Semibold)
		tx := f.ph + text.Width(col.Title, ts) + 4
		if col.Sortable || col.Compare != nil {
			tx += 24
		}
		left = min(tx, cellW-f.ph-18)
		tw, thh = 18, 22
		if align == AlignEnd {
			left = cellW - f.ph - 18
		}
	}
	n.ln.Pos = flexbox.Pos{Left: left, Top: (cellH - thh) / 2, Right: layout.Unset, Bottom: layout.Unset}
	n.ln.Width, n.ln.Height = tw, thh
	ia := c.interact(n, true, input.CursorPointer)
	open := st.fOpen == col.Key
	if ia.Clicked {
		if open {
			f.closeFilter(col, !f.el)
			open = false
		} else {
			st.fOpen = col.Key
			st.fDraft = append([]string(nil), f.filterOf[col.Key]...)
			st.fQuery = ""
			st.fAnchor = n.id
			open = true
		}
		c.invalid = true
	}
	n.setRole(a11y.RoleButton).setName(c.Locale().Table.Filter).setState(a11y.Expandable|a11y.HasPopup).stateIf(open, a11y.Expanded)
	n.paint = func(l *paint.List, r geom.Rect) {
		if ia.FocusVisible {
			l.Stroke(r, geom.Rad(th.RadiusSM), 2, th.Primary.Base)
		}
		col := f.tk.SorterIdleColor
		if active {
			col = th.Primary.Base
		}
		if f.el {
			if ia.Hovered {
				col = th.Primary.Base
			}
			icon.Draw(l, icon.Down, geom.Rect{X: r.X + (r.W-12)/2, Y: r.Y + (r.H-12)/2, W: 12, H: 12}, col, 0)
			return
		}
		if ia.Hovered || open {
			l.Fill(r, geom.Rad(th.RadiusSM), th.FillSecondary)
			if !active {
				col = th.TextTertiary
			}
		}
		icon.Draw(l, tblIconFunnel, geom.Rect{X: r.X + (r.W-12)/2, Y: r.Y + (r.H-12)/2, W: 12, H: 12}, col, 0)
	}
	if open {
		f.filterDropdown(col, n.id)
	}
}

// closeFilter closes the dropdown, optionally applying the draft.
func (f *tblFrame) closeFilter(col *TableColumn, apply bool) {
	st := f.st
	if apply {
		f.applyFilter(col.Key, st.fDraft)
	}
	st.fOpen = ""
	f.c.invalid = true
}

func (f *tblFrame) applyFilter(key string, vals []string) {
	old := f.filterOf[key]
	same := len(old) == len(vals)
	for i := 0; same && i < len(vals); i++ {
		same = old[i] == vals[i]
	}
	if len(vals) == 0 {
		delete(f.filterOf, key)
	} else {
		f.filterOf[key] = append([]string(nil), vals...)
	}
	if !same {
		f.t.filterCh = true
		f.resetPage()
	}
}

func (f *tblFrame) filterDropdown(col *TableColumn, anchor ID) {
	c, th, st := f.c, f.th, f.st
	place := BottomEnd
	if f.el {
		place = BottomStart
	}
	a := c.AnimFrom(anchor, "fdrop", 0, 1, th.DurationMid)
	var apply, reset bool
	ov := Overlay(c, OverlayOptions{Anchor: anchor, Place: place, Offset: 8, Flip: true}, func() {
		if col.FilterPanel != nil {
			p := &TableFilterPanel{c: c, Values: &st.fDraft}
			Column(c, func() { col.FilterPanel(p) }, layout.Pad(8))
			apply, reset = p.confirm, p.reset
			if p.close {
				st.fOpen = ""
			}
			return
		}
		if f.el && col.FilterSingle {
			f.elSingleList(col, &apply)
			return
		}
		if col.FilterSearch {
			Row(c, func() {
				Input(c, &st.fQuery).Placeholder(c.Locale().Table.FilterSearch).Prefix(icon.Search).AllowClear().Layout(layout.Grow(1))
			}, layout.Pad(8), layout.MinW(200))
			Div(c, layout.H(1)).Bg(th.Split)
		}
		q := strings.ToLower(st.fQuery)
		shown := 0
		list := Column(c, func() {
			for _, o := range col.Filters {
				if q != "" && !strings.Contains(strings.ToLower(o.Label), q) {
					continue
				}
				shown++
				f.filterItem(col, o)
			}
			if shown == 0 {
				Row(c, func() { Text(c, c.Locale().NoData).Color(th.TextQuaternary) }, layout.PadVH(12, 12), layout.Center())
			}
		}, layout.PadVH(4, 4), layout.MaxH(264), layout.MinW(120))
		list.Semantics(a11y.RoleListbox, col.Title)
		if !col.FilterSingle {
			list.A11yState(a11y.Multiselectable)
		}
		// Footer.
		Row(c, func() {
			if f.el {
				if Button(c, c.Locale().Table.Filter).Text().Small().Disabled(len(st.fDraft) == 0).Clicked() {
					apply = true
				}
				if Button(c, c.Locale().FilterReset).Text().Small().Clicked() {
					reset = true
				}
				return
			}
			if Button(c, c.Locale().FilterReset).Link().Small().Disabled(len(st.fDraft) == 0).Clicked() {
				reset = true
			}
			Spacer(c)
			if Button(c, c.Locale().FilterConfirm).Primary().Small().Clicked() {
				apply = true
			}
		}, layout.PadVH(7, 8), layout.Middle(), layout.Gap(8))
	})
	ov.Bg(th.BgElevated).Radius(th.RadiusLG).Shadow(th.Shadow).Opacity(a)
	if f.el {
		ov.Radius(th.Radius).Border(1, th.BorderSecondary)
	}
	// Split line above the footer.
	ov.PaintOver(func(l *paint.List, r geom.Rect) {
		if col.FilterPanel != nil || (f.el && col.FilterSingle) {
			return
		}
		fh := float32(24 + 14)
		l.Fill(geom.Rect{X: r.X, Y: r.Bottom() - fh - 1, W: r.W, H: 1}, geom.Radii{}, th.Split)
	})
	st.fPanel = ov.n.id
	switch {
	case apply:
		f.applyFilter(col.Key, st.fDraft)
		st.fOpen = ""
	case reset:
		f.applyFilter(col.Key, nil)
		st.fDraft = nil
		st.fOpen = ""
	case c.EscapeAt(st.fPanel):
		st.fOpen = ""
	case c.PressedOutside(anchor, st.fPanel):
		f.closeFilter(col, !f.el)
	}
	if st.fOpen == "" {
		c.invalid = true
	}
}

func (f *tblFrame) filterItem(col *TableColumn, o Option) {
	c, th, st := f.c, f.th, f.st
	Key(c, o.Value, func() {
		on := tblHas(st.fDraft, o.Value)
		b := Row(c, func() {
			// The control takes focus (Tab, Space); the row is the option
			// and reports that focus.
			if col.FilterSingle {
				rw := Radio(c, o.Label, on)
				rw.n.aHidden = true
				if rw.Clicked() {
					st.fDraft = []string{o.Value}
				}
			} else {
				v := on
				cb := Checkbox(c, o.Label, &v)
				cb.n.aHidden = true
				if cb.Changed() {
					st.fDraft = tblToggle(st.fDraft, o.Value, v)
				}
			}
		}, layout.PadVH(5, 12), layout.Middle(), layout.MinH(32))
		b.Semantics(a11y.RoleOption, o.Label).A11yState(a11y.Selectable).A11yKeyboard()
		if on {
			b.A11yState(a11y.Selected)
		}
		if f.el {
			b.Style(layout.PadVH(2, 10), layout.MinH(28))
		}
		ia := b.Interact(input.CursorPointer)
		if ia.Clicked {
			if col.FilterSingle {
				st.fDraft = []string{o.Value}
			} else {
				st.fDraft = tblToggle(st.fDraft, o.Value, !on)
			}
			c.invalid = true
		}
		if ia.Hovered && !f.el {
			b.Bg(th.ItemHoverBg).Radius(th.RadiusSM)
		}
	})
}

// elSingleList is Element's single-choice filter: a plain list where a
// click applies at once.
func (f *tblFrame) elSingleList(col *TableColumn, apply *bool) {
	c, th, st := f.c, f.th, f.st
	cur := ""
	if len(f.filterOf[col.Key]) > 0 {
		cur = f.filterOf[col.Key][0]
	}
	items := append([]Option{{Label: c.Locale().Table.FilterAll, Value: ""}}, col.Filters...)
	Column(c, func() {
		for _, o := range items {
			o := o
			Key(c, o.Value, func() {
				on := o.Value == cur
				lay := c.add("filter-item", o.Label)
				lay.setRole(a11y.RoleOption).setState(a11y.Selectable).stateIf(on, a11y.Selected)
				lay.ln.Height = 36
				lay.ln.MinW = 100
				lay.ln.Padding = geom.VH(0, 10)
				ia := c.interact(lay, true, input.CursorPointer)
				lay.ln.Measure = func(_, _ float32) geom.Size {
					return geom.Size{W: text.Width(o.Label, c.style(th.FontSize, text.Regular)) + 20, H: 36}
				}
				lay.paint = func(l *paint.List, r geom.Rect) {
					col := th.TextSecondary
					if on {
						l.Fill(r, geom.Radii{}, th.Primary.Bg)
						col = th.Primary.Base
					} else if ia.Hovered || ia.FocusVisible {
						l.Fill(r, geom.Radii{}, th.Primary.Bg)
						col = th.Primary.Base
					}
					f.drawText(l, o.Label, r.X+10, r.Y, r.W-20, r.H, AlignStart, c.style(th.FontSize, text.Regular), col)
				}
				if ia.Clicked {
					if o.Value == "" {
						st.fDraft = nil
					} else {
						st.fDraft = []string{o.Value}
					}
					*apply = true
				}
			})
		}
	}, layout.PadVH(6, 0)).Semantics(a11y.RoleListbox, col.Title)
}

// resizeHandle lets the user drag a column's end edge (right in LTR).
func (f *tblFrame) resizeHandle(col *TableColumn, li int, w, h float32) {
	c, th, st, t := f.c, f.th, f.st, f.t
	n := c.add("col-resize", col.Key)
	n.ln.Absolute = true
	n.ln.Pos = flexbox.Pos{Left: w - 5, Top: 0, Right: layout.Unset, Bottom: layout.Unset}
	n.ln.Width, n.ln.Height = 10, h
	// A focusable separator: Left / Down narrow the column, Right / Up
	// widen it (10px per press).
	n.setRole(a11y.RoleSeparator).setName(c.Locale().A11y.Resize).setState(a11y.Vertical)
	n.ownKeys = true
	ia := c.interact(n, true, input.CursorResizeEW)
	if ia.PressedNow {
		st.rzKey, st.rzX, st.rzW = col.Key, c.pointer.X, f.leaves[li].w
	}
	minW := col.MinWidth
	if minW <= 0 {
		minW = 50
	}
	cw := f.leaves[li].w
	if c.a11yOn {
		maxW := max(cw, f.viewW)
		n.fill(semFunc(func(s *a11y.Node) {
			rangeSem(s, float64(cw), float64(minW), float64(maxW), 10)
			s.Actions |= a11y.Increment | a11y.Decrement
		}))
	}
	// The handle sits on the column's end edge (the left one in RTL):
	// the arrow toward the end widens, so Left widens in RTL.
	step := float32(0)
	for _, k := range c.KeysFor(n.id) {
		if !k.Down {
			continue
		}
		prev, next := c.inlineKey(k.Key)
		switch {
		case prev || k.Key == input.KeyDown:
			step -= 10
		case next || k.Key == input.KeyUp:
			step += 10
		}
	}
	dragging := c.Dragging(n.id) && st.rzKey == col.Key
	if dragging || step != 0 {
		nw := max(minW, cw+step)
		if dragging {
			nw = max(minW, st.rzW+(c.pointer.X-st.rzX)*f.inl(geom.Rect{}).Sign())
		}
		if nw != st.widths[col.Key] {
			st.widths[col.Key] = nw
			if t.colState != nil {
				if t.colState.Widths == nil {
					t.colState.Widths = map[string]float32{}
				}
				t.colState.Widths[col.Key] = nw
			}
			t.resizedCh = true
			c.invalid = true
		}
	}
	n.paint = func(l *paint.List, r geom.Rect) {
		if ia.Hovered || dragging || ia.FocusVisible {
			l.Fill(geom.Rect{X: r.X + r.W/2 - 1, Y: r.Y + 4, W: 2, H: r.H - 8}, geom.Rad(1), th.Primary.Base)
		}
	}
}

// selectionItems returns the header selection menu (built-in defaults
// when Selections got no items).
func (f *tblFrame) selectionItems() []TableSelection {
	if len(f.t.selItems) > 0 {
		return f.t.selItems
	}
	items := []TableSelection{{Key: SelectionAll}}
	if f.t.page != nil {
		items = append(items, TableSelection{Key: SelectionPage})
	}
	return append(items, TableSelection{Key: SelectionInvert}, TableSelection{Key: SelectionNone})
}

// selectionText is the menu label of a selection item.
func (f *tblFrame) selectionText(it TableSelection) string {
	if it.Text != "" {
		return it.Text
	}
	loc := f.c.Locale()
	switch it.Key {
	case SelectionAll:
		return loc.SelectionAll
	case SelectionPage:
		return loc.Table.SelectPage
	case SelectionInvert:
		return loc.SelectionInvert
	case SelectionNone:
		return loc.Table.SelectNone
	}
	return it.Key
}

// selectionMenu is the down-arrow dropdown next to the header checkbox.
func (f *tblFrame) selectionMenu() {
	c := f.c
	items := f.selectionItems()
	menu := make([]MenuItem, len(items))
	for i, it := range items {
		menu[i] = MenuItem{Key: it.Key, Label: f.selectionText(it)}
	}
	label := c.Locale().Table.Selections
	dd := Dropdown(c, menu).Trigger(TriggerClick).Placement(BottomStart).Body(func() {
		pro3IconBtn(c, label, icon.Down, 16)
	})
	k := dd.Clicked()
	if k == "" {
		return
	}
	for _, it := range items {
		if it.Key == k {
			f.applySelection(it)
			return
		}
	}
}

// applySelection runs a selection menu item.
func (f *tblFrame) applySelection(it TableSelection) {
	t := f.t
	switch {
	case it.OnSelect != nil:
		it.OnSelect(f.dataRows())
		f.loadSelection()
		t.selCh = true
		f.c.invalid = true
	case it.Key == SelectionPage:
		f.selectPage(true)
	case it.Key == SelectionAll:
		f.selectRows(f.allRows(), true)
	case it.Key == SelectionNone:
		if t.selection != nil {
			*t.selection = (*t.selection)[:0]
		} else if t.selKeys != nil {
			*t.selKeys = (*t.selKeys)[:0]
		}
		f.loadSelection()
		t.selCh = true
		f.c.invalid = true
	case it.Key == SelectionInvert:
		var on, off []tblRow
		for i := 0; i < f.n; i++ {
			r := f.row(i)
			if !f.selectable(r) {
				continue
			}
			if f.selected(r) {
				off = append(off, r)
			} else {
				on = append(on, r)
			}
		}
		f.selectRows(off, false)
		f.selectRows(on, true)
	}
}

// allRows lists every displayed row before paging (local data: all pages
// after filtering; server pagination: the current page; trees: the
// expanded rows of the page).
func (f *tblFrame) allRows() []tblRow {
	t := f.t
	if f.tree != nil || t.serverPage || t.page == nil {
		out := make([]tblRow, 0, f.n)
		for i := 0; i < f.n; i++ {
			out = append(out, f.row(i))
		}
		return out
	}
	out := make([]tblRow, 0, f.allN)
	for i := 0; i < f.allN; i++ {
		d := i
		if f.ids != nil {
			d = f.ids[i]
		}
		out = append(out, tblRow{data: d, key: tblKey(t.rowKey, d)})
	}
	return out
}

// selectRows selects or clears rows (disabled rows are skipped).
func (f *tblFrame) selectRows(rows []tblRow, on bool) {
	t := f.t
	if len(rows) == 0 {
		return
	}
	if f.linked {
		var ids []int
		for _, r := range rows {
			if f.selectable(r) {
				ids = append(ids, r.data)
			}
		}
		f.linkedCheck(ids, on)
		return
	}
	switch {
	case t.selection != nil:
		set := map[int]bool{}
		for _, v := range *t.selection {
			set[v] = true
		}
		for _, r := range rows {
			if f.selectable(r) {
				set[r.data] = on
			}
		}
		out := (*t.selection)[:0]
		seen := map[int]bool{}
		for _, v := range *t.selection {
			if set[v] && !seen[v] {
				out, seen[v] = append(out, v), true
			}
		}
		for _, r := range rows {
			if set[r.data] && !seen[r.data] {
				out, seen[r.data] = append(out, r.data), true
			}
		}
		*t.selection = out
	case t.selKeys != nil:
		set := map[string]bool{}
		for _, v := range *t.selKeys {
			set[v] = true
		}
		for _, r := range rows {
			if f.selectable(r) {
				set[r.key] = on
			}
		}
		out := (*t.selKeys)[:0]
		seen := map[string]bool{}
		for _, v := range *t.selKeys {
			if set[v] && !seen[v] {
				out, seen[v] = append(out, v), true
			}
		}
		for _, r := range rows {
			if set[r.key] && !seen[r.key] {
				out, seen[r.key] = append(out, r.key), true
			}
		}
		*t.selKeys = out
	}
	f.loadSelection()
	t.selCh = true
	f.c.invalid = true
}
