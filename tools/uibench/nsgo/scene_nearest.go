package main

import (
	"strconv"

	"github.com/nsgo-dev/nsgo/base"
	"github.com/nsgo-dev/nsgo/base/datatable"
	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/ui"
)

// The nearest-counterpart scenes (../scenes.md, "Nearest-counterpart
// scenes"): NSGo's component where it has one, else the nearest one
// composed from its components, as the mapping there says. Frame f counts
// as in scene_components.go.

var (
	startStyle = core.Style{FlexGrow: 1, Padding: core.All(pagePad), Gap: pageGap, AlignItems: core.AlignStart}
	lineRow    = core.Style{Direction: core.Row, Gap: pageGap, AlignItems: core.AlignCenter}
)

// page is the padded column most scenes sit in.
func page(kids ...core.Node) core.Node { return core.View{Style: pageStyle, Children: kids} }

// text is a text reading fn, muted when asked.
func text(cx *core.Scope, fn func() string, muted bool) core.Node {
	t := core.Text{ContentFunc: fn}
	if muted {
		t.StyleFunc = ui.Colored(ui.ColorsOf(cx).Muted)
	}
	return t
}

// shares lays out n items in rows of cols, each item an equal share.
func shares(n, cols int, gap core.Points, item func(i int) []core.Node) []core.Node {
	rows := make([]core.Node, 0, n/cols)
	for r := 0; r < n/cols; r++ {
		kids := make([]core.Node, cols)
		for col := range kids {
			kids[col] = core.View{Style: halfStyle, Children: item(r*cols + col)}
		}
		rows = append(rows, core.View{Style: core.Style{Direction: core.Row, Gap: gap}, Children: kids})
	}
	return rows
}

// pick is a list of n rows of the given size whose row sel() is chosen,
// as a select's or a cascader's open list shows it.
func pick(cx *core.Scope, n int, w, h core.Points, sel func() int, label func(i int) string, arrow bool) core.Node {
	s := ui.NewSelection(cx, false)
	cx.Effect(func() { s.Set(sel()) })
	return ui.List{Count: func() int { return n }, Key: func(i int) any { return i }, Selection: s,
		EstimatedSize: 32, Style: core.Style{Width: core.Pt(w), Height: core.Pt(h)},
		Row: func(cx *core.Scope, row core.ListRow) core.Node {
			kids := []core.Node{text(cx, func() string { return label(row.Index()) }, false)}
			if arrow {
				kids = append(kids, ui.Text("›", 13, ui.Regular, ui.ColorsOf(cx).Muted))
			}
			return core.View{Style: core.Style{Direction: core.Row, JustifyContent: core.JustifySpaceBetween, Padding: core.Symmetric(6, 12)}, Children: kids}
		}}
}

// selectRoot: an open select of 8 options; frame f chooses option f%8.
// NSGo's Select has no open control: the Select, closed, is the trigger
// of a HoverCard opened from code, which holds the list of options with
// the chosen one selected, which is what an open select draws.
func selectRoot(cx *core.Scope) core.Node {
	sel := core.NewSignal(cx, 0)
	everyFrame(cx, func(f int) int { sel.Set(f % selectCount); return 1 })
	opts := make([]ui.Option[int], selectCount)
	for i := range opts {
		opts[i] = ui.Option[int]{Value: i, Label: selectLabel(i)}
	}
	return core.View{Style: startStyle, Children: []core.Node{
		ui.HoverCard{Open: func() bool { return true }, OnOpenChange: func(bool, base.Reason) {},
			Trigger:  ui.Select[int]{Options: opts, Value: sel.Get, OnChange: sel.Set, Style: core.Style{Width: core.Pt(selectWidth)}},
			Children: []core.Node{pick(cx, selectCount, selectWidth, selectCount*32, sel.Get, selectLabel, false)}},
	}}
}

// tooltipRoot: a button with a one-line popover shown and hidden every
// 30th frame: a Popover opened from code, as in the other two programs.
func tooltipRoot(cx *core.Scope) core.Node {
	open := core.NewSignal(cx, false)
	everyFrame(cx, func(f int) int {
		if f%tooltipEvery != 0 {
			return 0
		}
		open.Set(!open.GetUntracked())
		return 1
	})
	return core.View{Style: startStyle, Children: []core.Node{
		ui.Popover{Open: open.Get, OnOpenChange: func(bool, base.Reason) {},
			Trigger:  ui.Button{Text: tooltipAnchor, Variant: ui.ButtonSecondary},
			Children: []core.Node{core.Text{Content: tooltipText}}},
	}}
}

// tableColumns are the table scenes' columns.
func tableColumns() []datatable.Column {
	cols := make([]datatable.Column, tableCols)
	for i := range cols {
		cols[i] = datatable.Column{Key: strconv.Itoa(i), Title: tableTitles[i]}
	}
	return cols
}

// dataTable is a DataTable of n rows over upd, a signal per row of its
// Updated value: a change updates that row's cell only. It is as high as
// its header and shown rows at DataTable's default heights (row 32, the
// header a row's: base/datatable RowHeight, HeaderHeight), so it shows as
// many rows as the other programs' tables (scenes.md).
func dataTable(n, shown int, upd []*core.Signal[int]) core.Node {
	cols := tableColumns()
	return datatable.DataTable{Columns: func() []datatable.Column { return cols }, RowCount: func() int { return n },
		Text: func(r, col int) string {
			if col == tableCols-1 {
				return strconv.Itoa(upd[r].Get())
			}
			return tableCell(r, col)
		}, Style: core.Style{Height: core.Pt(core.Points(nsgoRowH * (shown + 1)))}}
}

const nsgoRowH = 32 // DataTable's default row (and header) height

func rowSignals(cx *core.Scope, n int) []*core.Signal[int] {
	s := make([]*core.Signal[int], n)
	for i := range s {
		s[i] = core.NewSignal(cx, 0)
	}
	return s
}

// tableRoot: a table of 1000 rows and 6 columns, 8 rows in view
// (base/datatable: NSGo has no styled table); frame f writes f into row
// f%8's Updated cell.
func tableRoot(cx *core.Scope) core.Node {
	upd := rowSignals(cx, tableRows)
	everyFrame(cx, func(f int) int { upd[tableRow(f)].Set(f); return 1 })
	return page(dataTable(tableRows, tableShown, upd))
}

// paginationRoot: the 10 items of a page over a pager of 50 pages; frame f
// shows page 1+f%50. NSGo's Pagination, with 7 visible pages, as many as
// antd's pager shows.
func paginationRoot(cx *core.Scope) core.Node {
	pg := core.NewSignal(cx, 1)
	everyFrame(cx, func(f int) int { pg.Set(pageOf(f)); return 1 })
	kids := make([]core.Node, 0, pageSize+1)
	for i := 0; i < pageSize; i++ {
		kids = append(kids, text(cx, func() string { return pageItem(pg.Get(), i) }, false))
	}
	kids = append(kids, ui.Pagination{Page: pg.Get, TotalPages: func() int { return pageCount }, VisiblePages: 7, OnChange: pg.Set})
	return page(kids...)
}

// cascaderRoot: a cascader open on 3 levels of 5; frame f chooses the path
// cascadePath(f). NSGo has no cascader: a Combobox shows the path, the
// trigger of a HoverCard opened from code that holds three lists, one a
// level, each with its choice selected.
func cascaderRoot(cx *core.Scope) core.Node {
	path := core.NewSignal(cx, cascadePath(0))
	everyFrame(cx, func(f int) int { path.Set(cascadePath(f)); return 1 })
	var paths []ui.Option[string]
	for f := 0; f < cascadeFan*cascadeFan*cascadeFan; f++ {
		t := cascadeText(cascadePath(f))
		paths = append(paths, ui.Option[string]{Value: t, Label: t})
	}
	cols := make([]core.Node, 3)
	for lv := range cols {
		cols[lv] = pick(cx, cascadeFan, cascadeWidth, cascadeFan*36, func() int { return path.Get()[lv] },
			func(i int) string { p := path.Get(); return cascadeLabel(p[:lv], i) }, lv < 2)
	}
	return core.View{Style: startStyle, Children: []core.Node{
		ui.HoverCard{Open: func() bool { return true }, OnOpenChange: func(bool, base.Reason) {},
			Trigger: ui.Combobox[string]{Options: paths, Value: func() []string { return []string{cascadeText(path.Get())} },
				OnChange: func([]string) {}, Style: core.Style{Width: core.Pt(3 * cascadeWidth)}},
			Children: []core.Node{core.View{Style: core.Style{Direction: core.Row}, Children: cols}}},
	}}
}

// transferRoot: two lists of check boxes, 10 items between them, and the
// two move buttons; frame f moves item f%10 to the other side. NSGo has no
// transfer: each side is a bordered box, a header (the count and the
// title) over its items' check boxes.
func transferRoot(cx *core.Scope) core.Node {
	right := make([]*core.Signal[bool], transferCount)
	for i := range right {
		right[i] = core.NewSignal(cx, transferStart(i))
	}
	everyFrame(cx, func(f int) int {
		s := right[f%transferCount]
		s.Set(!s.GetUntracked())
		return 1
	})
	colors := ui.ColorsOf(cx)
	side := func(onRight bool, title string) core.Node {
		return core.Dynamic(func(cx *core.Scope) core.Node {
			var boxes []core.Node
			for i, r := range right {
				if r.Get() == onRight {
					boxes = append(boxes, ui.Checkbox{Checked: func() bool { return false }, OnChange: func(bool) {}, Text: transferLabel(i)})
				}
			}
			return core.View{Style: core.Style{Width: core.Pt(300), Height: core.Pt(400), BorderWidth: 1, BorderColor: colors.Border(), CornerRadius: 6},
				Children: []core.Node{
					core.View{Style: core.Style{Direction: core.Row, JustifyContent: core.JustifySpaceBetween, Padding: core.Symmetric(8, 12)},
						Children: []core.Node{core.Text{Content: strconv.Itoa(len(boxes)) + " items"}, core.Text{Content: title}}},
					core.View{Style: core.Style{Height: core.Pt(1)}, StyleFunc: ui.Filled(colors.Border)},
					core.View{Style: core.Style{Padding: core.Symmetric(4, 12), Gap: 4}, Children: boxes},
				}}
		})
	}
	never := func() bool { return true }
	return page(core.View{Style: lineRow, Children: []core.Node{
		side(false, transferLeft),
		core.View{Style: core.Style{Gap: pageGap}, Children: []core.Node{
			ui.Button{Text: "›", Variant: ui.ButtonSecondary, Disabled: never},
			ui.Button{Text: "‹", Variant: ui.ButtonSecondary, Disabled: never},
		}},
		side(true, transferRight),
	}})
}

// treeselectRoot: a tree select open on 2 groups of 3 leaves, all
// expanded; frame f chooses leaf treeSelPick(f). NSGo has no tree select:
// its Select, closed, shows the leaf, the trigger of a HoverCard opened
// from code that holds a Tree with the leaf selected.
func treeselectRoot(cx *core.Scope) core.Node {
	leaf := core.NewSignal(cx, 0) // g*treeSelLeaves + j
	everyFrame(cx, func(f int) int { g, j := treeSelPick(f); leaf.Set(g*treeSelLeaves + j); return 1 })
	var opts []ui.Option[int]
	roots := make([]any, treeSelGroups)
	kids := make(map[int][]any, treeSelGroups)
	for g := range roots {
		roots[g] = -1 - g // groups are negative, leaves g*treeSelLeaves+j
		for j := 0; j < treeSelLeaves; j++ {
			n := g*treeSelLeaves + j
			kids[-1-g] = append(kids[-1-g], n)
			opts = append(opts, ui.Option[int]{Value: n, Label: treeSelLeaf(g, j)})
		}
	}
	s := ui.NewSelection(cx, false)
	cx.Effect(func() { s.Set(leaf.Get()) })
	title := func(k int) string {
		if k < 0 {
			return treeSelGroup(-1 - k)
		}
		return treeSelLeaf(k/treeSelLeaves, k%treeSelLeaves)
	}
	rows := treeSelGroups * (treeSelLeaves + 1)
	return core.View{Style: startStyle, Children: []core.Node{
		ui.HoverCard{Open: func() bool { return true }, OnOpenChange: func(bool, base.Reason) {},
			Trigger: ui.Select[int]{Options: opts, Value: leaf.Get, OnChange: leaf.Set, Style: core.Style{Width: core.Pt(300)}},
			Children: []core.Node{ui.Tree{
				Roots:     func() []any { return roots },
				Children:  func(k any) []any { return kids[k.(int)] },
				Expanded:  func(k any) bool { return k.(int) < 0 },
				Selection: s,
				Row: func(_ *core.Scope, item ui.TreeItem) core.Node {
					return core.Text{ContentFunc: func() string { return title(item.Key().(int)) }}
				},
				Style: core.Style{Width: core.Pt(300), Height: core.Pt(core.Points(rows * 32))},
			}}},
	}}
}

// calendarRoot: NSGo's calendar (its only one, which the datepicker scene
// shows too); frame f selects pickDay(f).
func calendarRoot(cx *core.Scope) core.Node {
	day := core.NewSignal(cx, base.DateValue{Start: pickDay(0)})
	everyFrame(cx, func(f int) int { day.Set(base.DateValue{Start: pickDay(f)}); return 1 })
	return page(ui.Calendar{Value: day.Get, OnChange: day.Set})
}

// stepsRoot: 5 steps; frame f makes step f%5 the current one. NSGo's
// Stepper.
func stepsRoot(cx *core.Scope) core.Node {
	cur := core.NewSignal(cx, 0)
	everyFrame(cx, func(f int) int { cur.Set(f % stepCount); return 1 })
	steps := make([]ui.StepperStep, stepCount)
	for i := range steps {
		steps[i] = ui.StepperStep{Label: stepTitle(i), Description: stepDesc(i)}
	}
	return page(ui.Stepper{Steps: steps, Current: cur.Get})
}

// timelineRoot: 8 events, each its text over its time; frame f rewrites
// event f%8. NSGo has no timeline: each event is a dot on a line beside
// its two texts.
func timelineRoot(cx *core.Scope) core.Node {
	changed := rowSignals(cx, timelineCount)
	everyFrame(cx, func(f int) int { changed[f%timelineCount].Set(f); return 1 })
	colors := ui.ColorsOf(cx)
	kids := make([]core.Node, timelineCount)
	for i := range kids {
		mark := []core.Node{core.View{Style: core.Style{Width: core.Pt(10), Height: core.Pt(10), CornerRadius: 5, BorderWidth: 2, BorderColor: colors.Accent()}}}
		if i < timelineCount-1 {
			mark = append(mark, core.View{Style: core.Style{Width: core.Pt(2), FlexGrow: 1}, StyleFunc: ui.Filled(colors.Border)})
		}
		kids[i] = core.View{Style: core.Style{Direction: core.Row, Gap: 12}, Children: []core.Node{
			core.View{Style: core.Style{Width: core.Pt(10), AlignItems: core.AlignCenter}, Children: mark},
			core.View{Style: core.Style{Padding: core.Edges{Bottom: 12}}, Children: []core.Node{
				text(cx, func() string { return timelineText(i, changed[i].Get()) }, false),
				text(cx, func() string { return timelineTime(i) }, true),
			}},
		}}
	}
	return page(kids...)
}

// descriptionsRoot: 12 label/value items in 3 bordered columns; frame f
// rewrites value f%12. NSGo has no descriptions: rows of bordered cells,
// the labels on the surface color.
func descriptionsRoot(cx *core.Scope) core.Node {
	changed := rowSignals(cx, descCount)
	everyFrame(cx, func(f int) int { changed[f%descCount].Set(f); return 1 })
	colors := ui.ColorsOf(cx)
	cell := core.Style{Padding: core.Symmetric(12, 16), BorderWidth: 1, BorderColor: colors.Border()}
	rows := make([]core.Node, descCount/descCols)
	for r := range rows {
		var kids []core.Node
		for col := 0; col < descCols; col++ {
			i := r*descCols + col
			label := cell
			label.Background = colors.Surface()
			value := cell
			value.FlexGrow, value.FlexBasis = 1, core.Pt(0)
			kids = append(kids,
				core.View{Style: label, Children: []core.Node{core.Text{Content: descLabel(i)}}},
				core.View{Style: value, Children: []core.Node{text(cx, func() string { return descValue(i, changed[i].Get()) }, false)}})
		}
		rows[r] = core.View{Style: core.Style{Direction: core.Row}, Children: kids}
	}
	return page(core.View{Style: core.Style{BorderWidth: 1, BorderColor: colors.Border(), CornerRadius: 6, Overflow: core.Hidden}, Children: rows})
}

// formRoot: a form of 10 labelled inputs in two columns of 5 and a submit
// button; frame f types into field f%10. NSGo's FormSection of
// FormFields.
func formRoot(cx *core.Scope) core.Node {
	vals := make([]*core.Signal[string], formFields)
	for i := range vals {
		vals[i] = core.NewSignal(cx, inputSeed)
	}
	everyFrame(cx, func(f int) int {
		v := vals[f%formFields]
		v.Set(typeInto(v.GetUntracked(), f))
		return 1
	})
	cols := make([]core.Node, 2)
	for col := range cols {
		fields := make([]core.Node, formFields/2)
		for r := range fields {
			i := col*formFields/2 + r
			fields[r] = ui.FormField{Title: formLabel(i), Input: base.Input{Value: vals[i].Get, OnChange: vals[i].Set}}
		}
		cols[col] = core.View{Style: halfStyle, Children: []core.Node{ui.FormSection{Children: fields}}}
	}
	return core.View{Style: startStyle, Children: []core.Node{
		core.View{Style: core.Style{Direction: core.Row, Gap: pagePad, AlignSelf: core.AlignStretch}, Children: cols},
		ui.Button{Text: formSubmit},
	}}
}

// uploadRoot: an upload button over 8 files uploading; frame f moves file
// f%8's progress one percent. NSGo has no upload list: each file is its
// name over a progress bar.
func uploadRoot(cx *core.Scope) core.Node {
	pct := make([]*core.Signal[float64], uploadCount)
	for i := range pct {
		pct[i] = core.NewSignal(cx, uploadStart(i))
	}
	everyFrame(cx, func(f int) int {
		s := pct[f%uploadCount]
		s.Set(nextPercent(s.GetUntracked()))
		return 1
	})
	files := make([]core.Node, uploadCount)
	for i := range files {
		files[i] = core.View{Style: core.Style{Gap: 2}, Children: []core.Node{
			core.Text{Content: uploadName(i)},
			ui.Progress{Value: func() float32 { return float32(pct[i].Get()) }},
		}}
	}
	return core.View{Style: startStyle, Children: []core.Node{
		ui.Button{Text: uploadButton, Variant: ui.ButtonSecondary},
		core.View{Style: core.Style{Width: core.Pt(520), Gap: pageGap}, Children: files},
	}}
}

// carouselRoot: 5 slides 240 px high; every 30th frame the next slide.
// NSGo's Carousel, which glides to the slide set.
func carouselRoot(cx *core.Scope) core.Node {
	cur := core.NewSignal(cx, 0)
	everyFrame(cx, func(f int) int {
		if f%carouselEvery != 0 {
			return 0
		}
		cur.Set((cur.GetUntracked() + 1) % carouselCount)
		return 1
	})
	return page(ui.Carousel{Count: carouselCount, Index: cur.Get, OnChange: cur.Set,
		Slide: func(_ *core.Scope, i int) core.Node {
			return core.View{Style: core.Style{Height: core.Pt(carouselHeight), Background: core.RGB(0xdfe7f2),
				AlignItems: core.AlignCenter, JustifyContent: core.JustifyCenter},
				Children: []core.Node{core.Text{Content: slideLabel(i), Style: core.Style{FontSize: 24}}}}
		}})
}

// collapseRoot: an accordion of 6 panels of 3 lines; every 30th frame the
// next panel opens (and the open one closes). NSGo's Accordion.
func collapseRoot(cx *core.Scope) core.Node {
	cur := core.NewSignal(cx, 0)
	everyFrame(cx, func(f int) int {
		if f%collapseEvery != 0 {
			return 0
		}
		cur.Set((cur.GetUntracked() + 1) % collapseCount)
		return 1
	})
	items := make([]ui.AccordionItem, collapseCount)
	for i := range items {
		lines := make([]core.Node, collapseLines)
		for l := range lines {
			lines[l] = core.Text{Content: collapseLine(i, l)}
		}
		items[i] = ui.AccordionItem{Text: collapseLabel(i), Content: core.View{Children: lines}}
	}
	return page(ui.Accordion{Items: items, Open: func(i int) bool { return cur.Get() == i },
		OnOpenChange: func(i int, open bool) {
			if open {
				cur.Set(i)
			}
		}})
}

// drawerRoot: a 360 px drawer at the right with 3 inputs and 2 buttons,
// opened and closed every 30th frame. NSGo's styled Sheet sits at the
// bottom only: the drawer is base.Sheet on the right side, styled with
// ui's colors, holding a title, the inputs and the buttons.
func drawerRoot(cx *core.Scope) core.Node {
	open := core.NewSignal(cx, false)
	everyFrame(cx, func(f int) int {
		if f%overlayEvery != 0 {
			return 0
		}
		open.Set(!open.GetUntracked())
		return 1
	})
	colors := ui.ColorsOf(cx)
	fields := make([]core.Node, len(dialogFields))
	for i, v := range dialogFields {
		s := core.NewSignal(cx, v)
		fields[i] = ui.Input{Value: s.Get, OnChange: s.Set}
	}
	return page(
		core.Text{Content: drawerBackdrop},
		base.Sheet{Open: open.Get, OnOpenChange: func(o bool, _ base.Reason) { open.Set(o) }, Side: core.PlaceRight,
			BackdropStyle: core.Style{Background: ui.Black(0.45)},
			PopupStyleFunc: func() core.Style {
				return core.Style{Width: core.Pt(drawerWidth), Padding: core.All(pagePad), Gap: pagePad, Background: colors.Surface()}
			},
			Children: []core.Node{
				ui.Text(drawerTitle, 15, ui.Semibold, colors.Text),
				core.View{Style: core.Style{Gap: pageGap, FlexGrow: 1}, Children: fields},
				core.View{Style: core.Style{Direction: core.Row, Gap: pageGap, JustifyContent: core.JustifyEnd}, Children: []core.Node{
					ui.Button{Variant: ui.ButtonSecondary, Text: dialogCancel, OnPress: func() { open.Set(false) }},
					ui.Button{Text: dialogOK, OnPress: func() { open.Set(false) }},
				}},
			}},
	)
}

// tourRoot: 3 buttons and a tour of 3 steps pointing at them over a mask;
// every 30th frame the next step. NSGo has no tour: a dimming view over
// the page, and each button the trigger of a Popover opened while its
// step is current, holding the step's title, text and the Previous and
// Next buttons.
func tourRoot(cx *core.Scope) core.Node {
	cur := core.NewSignal(cx, 0)
	everyFrame(cx, func(f int) int {
		if f%tourEvery != 0 {
			return 0
		}
		cur.Set((cur.GetUntracked() + 1) % tourCount)
		return 1
	})
	colors := ui.ColorsOf(cx)
	buttons := make([]core.Node, tourCount)
	for i := range buttons {
		buttons[i] = ui.Popover{Open: func() bool { return cur.Get() == i }, OnOpenChange: func(bool, base.Reason) {},
			Trigger: ui.Button{Text: tourButtons[i], Variant: ui.ButtonSecondary},
			Children: []core.Node{core.View{Style: core.Style{Width: core.Pt(260), Gap: pageGap}, Children: []core.Node{
				ui.Text(tourTitles[i], 13, ui.Semibold, colors.Text),
				core.Text{Content: tourDesc(i)},
				core.View{Style: core.Style{Direction: core.Row, Gap: pageGap, JustifyContent: core.JustifyEnd}, Children: []core.Node{
					ui.Button{Text: "Previous", Variant: ui.ButtonSecondary}, ui.Button{Text: "Next"},
				}},
			}}}}
	}
	return core.View{Style: core.Style{FlexGrow: 1}, Children: []core.Node{
		page(core.View{Style: lineRow, Children: buttons}),
		core.View{Style: core.Style{Position: core.Absolute, Top: core.Pt(0), Left: core.Pt(0), Right: core.Pt(0), Bottom: core.Pt(0),
			Background: ui.Black(0.45)}},
	}}
}

// protableRoot: a query form of one field (Name, with Reset and Query)
// over a table of 6 rows of the table scene's columns, all in view; frame
// f writes f into row f%6's Updated cell. NSGo has no ProTable: a
// FormField and two buttons over a DataTable. The field is a FormField,
// as in the form scene: its Input is a base.Input (the platform text
// field) by FormField's API; the inputs scene's ui.Input is the boxed
// field for a field on its own.
func protableRoot(cx *core.Scope) core.Node {
	upd := rowSignals(cx, proRows)
	everyFrame(cx, func(f int) int { upd[proRow(f)].Set(f); return 1 })
	query := core.NewSignal(cx, "")
	return page(
		core.View{Style: lineRow, Children: []core.Node{
			core.View{Style: core.Style{Width: core.Pt(320)}, Children: []core.Node{
				ui.FormField{Title: tableTitles[1], Input: base.Input{Value: query.Get, OnChange: query.Set, Placeholder: "Please enter"}},
			}},
			core.View{Style: core.Style{FlexGrow: 1}},
			ui.Button{Text: resetButton, Variant: ui.ButtonSecondary},
			ui.Button{Text: queryButton},
		}},
		dataTable(proRows, proRows, upd),
	)
}

// segmentedRoot: 10 segmented controls of 4 choices; frame f moves control
// f%10 to its next choice. NSGo's segmented control is its Tabs in their
// default, segmented look, without a panel.
func segmentedRoot(cx *core.Scope) core.Node {
	chosen := make([]*core.Signal[int], segCount)
	for i := range chosen {
		chosen[i] = core.NewSignal(cx, segStart(i))
	}
	everyFrame(cx, func(f int) int {
		s := chosen[f%segCount]
		s.Set((s.GetUntracked() + 1) % segChoices)
		return 1
	})
	opts := make([]ui.Option[int], segChoices)
	for i := range opts {
		opts[i] = ui.Option[int]{Value: i, Label: segLabels[i]}
	}
	kids := make([]core.Node, segCount)
	for i := range kids {
		kids[i] = ui.Tabs[int]{Options: opts, Value: chosen[i].Get, OnChange: chosen[i].Set}
	}
	return core.View{Style: startStyle, Children: kids}
}

// rateRoot: 20 five-star ratings in two columns; frame f moves rating f%20
// one star on (past 5 back to 0). NSGo's Rating.
func rateRoot(cx *core.Scope) core.Node {
	vals := make([]*core.Signal[int], rateCount)
	for i := range vals {
		vals[i] = core.NewSignal(cx, rateStart(i))
	}
	everyFrame(cx, func(f int) int {
		s := vals[f%rateCount]
		s.Set((s.GetUntracked() + 1) % (rateMax + 1))
		return 1
	})
	return page(shares(rateCount, rateCols, pagePad, func(i int) []core.Node {
		return []core.Node{ui.Rating{Value: vals[i].Get, OnChange: vals[i].Set, Max: rateMax}}
	})...)
}

// tagsRoot: 60 tags in rows of 6; frame f relabels tag f%60. NSGo's tag is
// its Badge, a short text in a pill.
func tagsRoot(cx *core.Scope) core.Node {
	labels := make([]*core.Signal[string], tagCount)
	for i := range labels {
		labels[i] = core.NewSignal(cx, tagLabel(i))
	}
	everyFrame(cx, func(f int) int {
		i := f % tagCount
		labels[i].Set(taggedLabel(i, f))
		return 1
	})
	return grid(pageStyle, pageGap, tagCount, tagCols, func(i int) core.Node {
		return ui.Badge{TextFunc: labels[i].Get}
	})
}

// badgesRoot: 40 squares with a count badge, rows of 10; frame f adds one
// to badge f%40 (1..99). NSGo's Badge on the square's corner.
func badgesRoot(cx *core.Scope) core.Node {
	counts := make([]*core.Signal[int], badgeCount)
	for i := range counts {
		counts[i] = core.NewSignal(cx, badgeStart(i))
	}
	everyFrame(cx, func(f int) int {
		s := counts[f%badgeCount]
		s.Set(nextCount(s.GetUntracked()))
		return 1
	})
	style := core.Style{FlexGrow: 1, Padding: core.All(2 * pagePad), Gap: 2 * pagePad}
	return grid(style, 2*pagePad, badgeCount, badgeCols, func(i int) core.Node {
		return core.View{Style: core.Style{Width: core.Pt(badgeSize), Height: core.Pt(badgeSize), CornerRadius: 6, Background: core.RGB(0xd9dde3)},
			Children: []core.Node{core.View{Style: core.Style{Position: core.Absolute, Top: core.Pt(-8), Right: core.Pt(-8)},
				Children: []core.Node{ui.Badge{TextFunc: func() string { return strconv.Itoa(counts[i].Get()) }}}}}}
	})
}

// cardsRoot: 12 cards in rows of 4, each a title over 2 lines; frame f
// rewrites the second line of card f%12. NSGo has no card: a bordered,
// rounded, padded view.
func cardsRoot(cx *core.Scope) core.Node {
	updated := rowSignals(cx, cardCount)
	everyFrame(cx, func(f int) int { updated[f%cardCount].Set(f); return 1 })
	colors := ui.ColorsOf(cx)
	return page(shares(cardCount, cardCols, pageGap, func(i int) []core.Node {
		return []core.Node{core.View{Style: core.Style{Padding: core.All(pagePad), Gap: pageGap, BorderWidth: 1, BorderColor: colors.Border(),
			CornerRadius: 8, Background: colors.Background()}, Children: []core.Node{
			ui.Text(cardTitle(i), 13, ui.Semibold, colors.Text),
			core.Text{Content: cardLine(i)},
			text(cx, func() string { return cardUpdated(updated[i].Get()) }, false),
		}}}
	})...)
}

// statisticsRoot: 12 statistics in rows of 4; frame f changes the value of
// statistic f%12. NSGo has no statistic: a muted title over a 24 px value.
func statisticsRoot(cx *core.Scope) core.Node {
	vals := make([]*core.Signal[float64], statCount)
	for i := range vals {
		vals[i] = core.NewSignal(cx, statValue(i, 0))
	}
	everyFrame(cx, func(f int) int {
		i := f % statCount
		vals[i].Set(statValue(i, f))
		return 1
	})
	return page(shares(statCount, statCols, pagePad, func(i int) []core.Node {
		return []core.Node{
			text(cx, func() string { return statTitle(i) }, true),
			core.Text{ContentFunc: func() string { return statText(vals[i].Get()) }, Style: core.Style{FontSize: 24}},
		}
	})...)
}
