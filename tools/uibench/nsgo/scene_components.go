package main

import (
	"time"

	"github.com/nsgo-dev/nsgo/base"
	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/ui"
)

// The component scenes, the same in every framework's program
// (../scenes.md, "Component scenes"). Frame f is 1 on the first frame
// callback, which reports ready, and counts up from there.

// everyFrame chains Scope.RequestAnimationFrame: each callback runs the
// scene's frame f, which sets the signals it changes and returns the work
// it did.
func everyFrame(cx *core.Scope, frame func(f int) int) {
	f := 0
	var step func(time.Time)
	step = func(time.Time) {
		f++
		report(frame(f))
		cx.RequestAnimationFrame(step)
	}
	cx.RequestAnimationFrame(step)
}

var (
	pageStyle  = core.Style{FlexGrow: 1, Padding: core.All(pagePad), Gap: pageGap}
	lineStyle  = core.Style{Direction: core.Row, Gap: pageGap}
	halfStyle  = core.Style{FlexGrow: 1, FlexBasis: core.Pt(0), Gap: pageGap}
	halvesRow  = core.Style{Direction: core.Row, FlexGrow: 1, Padding: core.All(pagePad), Gap: pagePad}
	fillStyle  = core.Style{FlexGrow: 1}
	menuColumn = core.Style{Width: core.Pt(menuWidth), Height: core.Pt(viewH)}
)

// halves lays out n controls in two columns of n/2.
func halves(n int, control func(i int) core.Node) core.Node {
	cols := make([]core.Node, 2)
	for col := range cols {
		kids := make([]core.Node, n/2)
		for r := range kids {
			kids[r] = control(col*n/2 + r)
		}
		cols[col] = core.View{Style: halfStyle, Children: kids}
	}
	return core.View{Style: halvesRow, Children: cols}
}

// grid lays out n controls in rows of cols.
func grid(style core.Style, rowGap core.Points, n, cols int, control func(i int) core.Node) core.Node {
	rows := make([]core.Node, 0, n/cols)
	for r := 0; r < n/cols; r++ {
		kids := make([]core.Node, cols)
		for col := range kids {
			kids[col] = control(r*cols + col)
		}
		rows = append(rows, core.View{Style: core.Style{Direction: core.Row, Gap: rowGap}, Children: kids})
	}
	return core.View{Style: style, Children: rows}
}

// buttonsRoot: 60 buttons in rows of 6; frame f relabels button f%60. A
// ui.Button's Text is fixed when it is built, so each button is a
// core.Dynamic over its label's signal, built again when the label
// changes: NSGo's way to change what a component was built with.
func buttonsRoot(cx *core.Scope) core.Node {
	labels := make([]*core.Signal[string], buttonCount)
	for i := range labels {
		labels[i] = core.NewSignal(cx, buttonLabel(i))
	}
	everyFrame(cx, func(f int) int {
		labels[f%buttonCount].Set(clickedLabel(f))
		return 1
	})
	return grid(pageStyle, pageGap, buttonCount, buttonCols, func(i int) core.Node {
		return core.Dynamic(func(*core.Scope) core.Node { return ui.Button{Text: labels[i].Get()} })
	})
}

// inputsRoot: 20 single-line inputs in two columns; frame f types one
// character into input f%20.
func inputsRoot(cx *core.Scope) core.Node {
	vals := make([]*core.Signal[string], inputCount)
	for i := range vals {
		vals[i] = core.NewSignal(cx, inputSeed)
	}
	everyFrame(cx, func(f int) int {
		v := vals[f%inputCount]
		v.Set(typeInto(v.GetUntracked(), f))
		return 1
	})
	return halves(inputCount, func(i int) core.Node {
		return ui.Input{Value: vals[i].Get, OnChange: vals[i].Set}
	})
}

// textareaRoot: one multi-line input filling the window, 2000 characters
// to start; frame f appends one.
func textareaRoot(cx *core.Scope) core.Node {
	seed := textareaSeed()
	val := core.NewSignal(cx, seed)
	everyFrame(cx, func(f int) int {
		val.Set(appendText(val.GetUntracked(), seed, f))
		return 1
	})
	return core.View{Style: pageStyle, Children: []core.Node{
		ui.Input{Multiline: true, Value: val.Get, OnChange: val.Set, Style: fillStyle},
	}}
}

// checkboxesRoot: 100 check boxes in 5 columns; frame f toggles f%100.
func checkboxesRoot(cx *core.Scope) core.Node {
	on := make([]*core.Signal[bool], toggleCount)
	for i := range on {
		on[i] = core.NewSignal(cx, false)
	}
	everyFrame(cx, func(f int) int {
		s := on[f%toggleCount]
		s.Set(!s.GetUntracked())
		return 1
	})
	style := pageStyle
	style.Gap = 4
	return grid(style, pageGap, toggleCount, checkCols, func(i int) core.Node {
		return ui.Checkbox{Checked: on[i].Get, OnChange: on[i].Set, Text: checkLabel(i)}
	})
}

// radiosRoot: 20 groups of 5 radios in two columns; frame f moves group
// f%20 to its next radio.
func radiosRoot(cx *core.Scope) core.Node {
	chosen := make([]*core.Signal[int], radioGroups)
	for g := range chosen {
		chosen[g] = core.NewSignal(cx, 0)
	}
	everyFrame(cx, func(f int) int {
		s := chosen[f%radioGroups]
		s.Set((s.GetUntracked() + 1) % radioChoices)
		return 1
	})
	return halves(radioGroups, func(g int) core.Node {
		kids := make([]core.Node, radioChoices)
		for i := range kids {
			kids[i] = ui.Radio{Text: radioLabel(i),
				Checked: func() bool { return chosen[g].Get() == i },
				OnChange: func(on bool) {
					if on {
						chosen[g].Set(i)
					}
				}}
		}
		return ui.RadioGroup{Horizontal: true, Children: kids}
	})
}

// switchesRoot: 100 switches, 10 by 10; frame f toggles f%100.
func switchesRoot(cx *core.Scope) core.Node {
	on := make([]*core.Signal[bool], toggleCount)
	for i := range on {
		on[i] = core.NewSignal(cx, switchStart(i))
	}
	everyFrame(cx, func(f int) int {
		s := on[f%toggleCount]
		s.Set(!s.GetUntracked())
		return 1
	})
	return grid(pageStyle, pagePad, toggleCount, switchCols, func(i int) core.Node {
		return ui.Switch{Checked: on[i].Get, OnChange: on[i].Set}
	})
}

// slidersRoot: 20 sliders 0..100 in two columns; frame f moves slider
// f%20 one step.
func slidersRoot(cx *core.Scope) core.Node {
	vals := make([]*core.Signal[float64], sliderCount)
	for i := range vals {
		vals[i] = core.NewSignal(cx, sliderStart(i))
	}
	everyFrame(cx, func(f int) int {
		s := vals[f%sliderCount]
		s.Set(nextPercent(s.GetUntracked()))
		return 1
	})
	return halves(sliderCount, func(i int) core.Node {
		return ui.Slider{Value: vals[i].Get, OnChange: vals[i].Set, Min: 0, Max: 100}
	})
}

// progressRoot: 50 bars in two columns, no text; every frame moves all of
// them one percent.
func progressRoot(cx *core.Scope) core.Node {
	vals := make([]*core.Signal[float64], progressCount)
	for i := range vals {
		vals[i] = core.NewSignal(cx, progressStart(i))
	}
	everyFrame(cx, func(int) int {
		cx.Batch(func() {
			for _, s := range vals {
				s.Set(nextPercent(s.GetUntracked()))
			}
		})
		return progressCount
	})
	return halves(progressCount, func(i int) core.Node {
		return ui.Progress{Value: func() float32 { return float32(vals[i].Get()) }}
	})
}

// tabsRoot: 10 tabs of 20 lines each; every 10th frame chooses the next.
func tabsRoot(cx *core.Scope) core.Node {
	tab := core.NewSignal(cx, 0)
	everyFrame(cx, func(f int) int {
		if f%tabEvery != 0 {
			return 0
		}
		tab.Set((tab.GetUntracked() + 1) % tabCount)
		return 1
	})
	opts := make([]ui.Option[int], tabCount)
	for i := range opts {
		opts[i] = ui.Option[int]{Value: i, Label: tabLabel(i)}
	}
	lines := make([]core.Node, tabLines)
	for l := range lines {
		lines[l] = core.Text{ContentFunc: func() string { return tabLine(tab.Get(), l) }}
	}
	return core.View{Style: pageStyle, Children: []core.Node{
		ui.Tabs[int]{Options: opts, Value: tab.Get, OnChange: tab.Set},
		core.View{Style: core.Style{Gap: 2}, Children: lines},
	}}
}

// menuRoot: a vertical menu of 50 items filling the window's height (it
// scrolls); frame f selects item f%50. NSGo's vertical, selectable menu
// is its Sidebar (its Menu is a popup from a trigger).
func menuRoot(cx *core.Scope) core.Node {
	sel := core.NewSignal(cx, menuKey(0))
	everyFrame(cx, func(f int) int {
		sel.Set(menuKey(f % menuCount))
		return 1
	})
	items := make([]ui.SidebarItem, menuCount)
	for i := range items {
		items[i] = ui.SidebarItem{ID: menuKey(i), Text: menuLabel(i)}
	}
	return core.View{Style: core.Style{Direction: core.Row, FlexGrow: 1}, Children: []core.Node{
		core.View{Style: menuColumn, Children: []core.Node{
			ui.Sidebar{Sections: []ui.SidebarSection{{Items: items}}, Selected: sel.Get, OnSelect: sel.Set},
		}},
	}}
}

// dialogRoot: a modal with 3 inputs and 2 buttons, opened and closed
// every 30th frame.
func dialogRoot(cx *core.Scope) core.Node {
	open := core.NewSignal(cx, false)
	everyFrame(cx, func(f int) int {
		if f%overlayEvery != 0 {
			return 0
		}
		open.Set(!open.GetUntracked())
		return 1
	})
	fields := make([]core.Node, len(dialogFields))
	for i, v := range dialogFields {
		s := core.NewSignal(cx, v)
		fields[i] = ui.Input{Value: s.Get, OnChange: s.Set}
	}
	return core.View{Style: pageStyle, Children: []core.Node{
		core.Text{Content: dialogBackdrop},
		ui.Dialog{Open: open.Get, OnOpenChange: func(o bool, _ base.Reason) { open.Set(o) }, Title: dialogTitle,
			Children: fields,
			Footer: []core.Node{
				ui.Button{Variant: ui.ButtonSecondary, Text: dialogCancel, OnPress: func() { open.Set(false) }},
				ui.Button{Text: dialogOK, OnPress: func() { open.Set(false) }},
			}},
	}}
}

// toastRoot: every 30th frame a toast that goes after a second.
func toastRoot(cx *core.Scope) core.Node {
	toaster := ui.NewToaster(cx)
	everyFrame(cx, func(f int) int {
		if f%overlayEvery != 0 {
			return 0
		}
		toaster.Show(ui.ToastContent{Title: toastText(f / overlayEvery), Duration: toastFor})
		return 1
	})
	return core.View{Style: pageStyle, Children: []core.Node{
		core.Text{Content: toastBackdrop},
		ui.Toasts{Toaster: toaster},
	}}
}

// treeRoot: 10,000 nodes, all expanded, filling the window; frame f
// centers row treeRow(f), one row further each frame. ui.Tree builds only
// the rows in view.
func treeRoot(cx *core.Scope) core.Node {
	ref := &core.Ref{}
	everyFrame(cx, func(f int) int {
		ref.ScrollToIndexAt(treeRow(f), 0.5, 0, false)
		return 1
	})
	roots := make([]any, treeGroups)
	kids := make(map[int][]any, treeGroups)
	for g := range roots {
		id := g * treeGroup
		roots[g] = id
		leaves := make([]any, treeGroup-1)
		for j := range leaves {
			leaves[j] = id + j + 1
		}
		kids[id] = leaves
	}
	return ui.Tree{
		Roots:    func() []any { return roots },
		Children: func(key any) []any { return kids[key.(int)] },
		Expanded: func(key any) bool { return key.(int)%treeGroup == 0 },
		Row: func(_ *core.Scope, item ui.TreeItem) core.Node {
			return core.Text{ContentFunc: func() string { return treeTitle(item.Key().(int)) }}
		},
		Ref:   ref,
		Style: fillStyle,
	}
}

// datepickerRoot: an inline calendar (NSGo's date panel, which its
// DatePicker opens); frame f selects the next day, within one month.
func datepickerRoot(cx *core.Scope) core.Node {
	day := core.NewSignal(cx, base.DateValue{Start: pickDay(0)})
	everyFrame(cx, func(f int) int {
		day.Set(base.DateValue{Start: pickDay(f)})
		return 1
	})
	return core.View{Style: pageStyle, Children: []core.Node{
		ui.Calendar{Value: day.Get, OnChange: day.Set},
	}}
}

// colorpickerRoot: the inline color picker (what its ColorWell opens);
// frame f turns the hue one degree.
func colorpickerRoot(cx *core.Scope) core.Node {
	col := core.NewSignal(cx, pickColor(0))
	everyFrame(cx, func(f int) int {
		col.Set(pickColor(f))
		return 1
	})
	return core.View{Style: pageStyle, Children: []core.Node{
		ui.ColorPicker{Value: col.Get, OnChange: func(v core.Color, _ string) { col.Set(v) }},
	}}
}

func pickColor(f int) core.Color {
	r, g, b := hueRGB(f % 360)
	return core.RGB(uint32(r)<<16 | uint32(g)<<8 | uint32(b))
}
