package main

import (
	"strconv"

	"github.com/tangoui/tango/ui"
	"github.com/tangoui/tango/ui/layout"
	"github.com/tangoui/tango/ui/theme"
)

// The component scenes, the same in every framework's program
// (../scenes.md, "Component scenes"). Frame f is 1 on the first frame
// that does the scene's work (the ready frame) and counts up from there.

// stepper drives a component page: it asks for the next frame in every
// build pass and advances f once per new frame, from the second frame on
// (the first is the one that puts the page on screen).
type stepper struct {
	k clock
	f int
}

func (s *stepper) step(c *ui.Context) bool {
	c.WakeAt(c.Now())
	if s.k.next(c) && s.k.frames >= 2 {
		s.f++
		return true
	}
	return false
}

// buttonsPage: 60 buttons in rows of 6; frame f relabels button f%60.
func buttonsPage() func(*ui.Context) {
	var s stepper
	labels := make([]string, buttonCount)
	for i := range labels {
		labels[i] = buttonLabel(i)
	}
	return func(c *ui.Context) {
		adv := s.step(c)
		if adv {
			labels[s.f%buttonCount] = clickedLabel(s.f)
		}
		ui.Column(c, func() {
			for r := 0; r < buttonCount/buttonCols; r++ {
				ui.Row(c, func() {
					for col := 0; col < buttonCols; col++ {
						ui.Button(c, labels[r*buttonCols+col])
					}
				}, layout.Gap(pageGap))
			}
		}, layout.Fill(), layout.Gap(pageGap), layout.Pad(pagePad))
		if adv {
			report(1)
		}
	}
}

// inputsPage: 20 single-line inputs in two columns; frame f types one
// character into input f%20.
func inputsPage() func(*ui.Context) {
	var s stepper
	vals := make([]string, inputCount)
	for i := range vals {
		vals[i] = inputSeed
	}
	return func(c *ui.Context) {
		adv := s.step(c)
		if adv {
			i := s.f % inputCount
			vals[i] = typeInto(vals[i], s.f)
		}
		ui.Row(c, func() {
			for col := 0; col < 2; col++ {
				ui.Column(c, func() {
					for r := 0; r < inputCount/2; r++ {
						ui.Input(c, &vals[col*inputCount/2+r])
					}
				}, layout.Grow(1), layout.Basis(0), layout.Gap(pageGap))
			}
		}, layout.Fill(), layout.Gap(pagePad), layout.Pad(pagePad))
		if adv {
			report(1)
		}
	}
}

// textareaPage: one multi-line input filling the window, 2000 characters
// to start; frame f appends one.
func textareaPage() func(*ui.Context) {
	var s stepper
	seed := textareaSeed()
	val := seed
	return func(c *ui.Context) {
		adv := s.step(c)
		if adv {
			val = appendText(val, seed, s.f)
		}
		ui.Column(c, func() {
			ui.TextArea(c, &val).Layout(layout.Fill())
		}, layout.Fill(), layout.Pad(pagePad))
		if adv {
			report(1)
		}
	}
}

// checkboxesPage: 100 check boxes in 5 columns; frame f toggles f%100.
func checkboxesPage() func(*ui.Context) {
	var s stepper
	on := make([]bool, toggleCount)
	labels := make([]string, toggleCount)
	for i := range labels {
		labels[i] = checkLabel(i)
	}
	return func(c *ui.Context) {
		adv := s.step(c)
		if adv {
			i := s.f % toggleCount
			on[i] = !on[i]
		}
		ui.Column(c, func() {
			for r := 0; r < toggleCount/checkCols; r++ {
				ui.Row(c, func() {
					for col := 0; col < checkCols; col++ {
						i := r*checkCols + col
						ui.Checkbox(c, labels[i], &on[i])
					}
				}, layout.Gap(pageGap))
			}
		}, layout.Fill(), layout.Gap(4), layout.Pad(pagePad))
		if adv {
			report(1)
		}
	}
}

// radiosPage: 20 groups of 5 radios in two columns; frame f moves group
// f%20 to its next radio.
func radiosPage() func(*ui.Context) {
	var s stepper
	opts := make([]ui.Option, radioChoices)
	for i := range opts {
		opts[i] = ui.Option{Label: radioLabel(i), Value: radioLabel(i)}
	}
	chosen := make([]int, radioGroups)
	vals := make([]string, radioGroups)
	return func(c *ui.Context) {
		adv := s.step(c)
		if adv {
			g := s.f % radioGroups
			chosen[g] = (chosen[g] + 1) % radioChoices
		}
		ui.Row(c, func() {
			for col := 0; col < 2; col++ {
				ui.Column(c, func() {
					for r := 0; r < radioGroups/2; r++ {
						g := col*radioGroups/2 + r
						vals[g] = radioLabel(chosen[g])
						ui.RadioGroup(c, &vals[g], opts)
					}
				}, layout.Grow(1), layout.Basis(0), layout.Gap(pageGap))
			}
		}, layout.Fill(), layout.Gap(pagePad), layout.Pad(pagePad))
		if adv {
			report(1)
		}
	}
}

// switchesPage: 100 switches, 10 by 10; frame f toggles f%100.
func switchesPage() func(*ui.Context) {
	var s stepper
	on := make([]bool, toggleCount)
	for i := range on {
		on[i] = switchStart(i)
	}
	return func(c *ui.Context) {
		adv := s.step(c)
		if adv {
			i := s.f % toggleCount
			on[i] = !on[i]
		}
		ui.Column(c, func() {
			for r := 0; r < toggleCount/switchCols; r++ {
				ui.Row(c, func() {
					for col := 0; col < switchCols; col++ {
						ui.Switch(c, &on[r*switchCols+col])
					}
				}, layout.Gap(pagePad))
			}
		}, layout.Fill(), layout.Gap(pageGap), layout.Pad(pagePad))
		if adv {
			report(1)
		}
	}
}

// slidersPage: 20 sliders 0..100 in two columns; frame f moves slider
// f%20 one step.
func slidersPage() func(*ui.Context) {
	var s stepper
	vals := make([]float64, sliderCount)
	for i := range vals {
		vals[i] = sliderStart(i)
	}
	return func(c *ui.Context) {
		adv := s.step(c)
		if adv {
			i := s.f % sliderCount
			vals[i] = nextPercent(vals[i])
		}
		ui.Row(c, func() {
			for col := 0; col < 2; col++ {
				ui.Column(c, func() {
					for r := 0; r < sliderCount/2; r++ {
						ui.Slider(c, &vals[col*sliderCount/2+r], 0, 100)
					}
				}, layout.Grow(1), layout.Basis(0), layout.Gap(pageGap))
			}
		}, layout.Fill(), layout.Gap(pagePad), layout.Pad(pagePad))
		if adv {
			report(1)
		}
	}
}

// progressPage: 50 bars in two columns, no text; every frame moves all
// of them one percent.
func progressPage() func(*ui.Context) {
	var s stepper
	vals := make([]float64, progressCount)
	for i := range vals {
		vals[i] = progressStart(i)
	}
	return func(c *ui.Context) {
		adv := s.step(c)
		if adv {
			for i := range vals {
				vals[i] = nextPercent(vals[i])
			}
		}
		ui.Row(c, func() {
			for col := 0; col < 2; col++ {
				ui.Column(c, func() {
					for r := 0; r < progressCount/2; r++ {
						// The scene's bars show no text (scenes.md); antd's
						// line progress shows its percent unless told not to.
						ui.Progress(c, vals[col*progressCount/2+r]).HideInfo()
					}
				}, layout.Grow(1), layout.Basis(0), layout.Gap(pageGap))
			}
		}, layout.Fill(), layout.Gap(pagePad), layout.Pad(pagePad))
		if adv {
			report(progressCount)
		}
	}
}

// tabsPage: 10 tabs of 20 lines each; every 10th frame chooses the next.
func tabsPage() func(*ui.Context) {
	var s stepper
	items := make([]ui.TabItem, tabCount)
	for i := range items {
		items[i] = ui.TabItem{Key: strconv.Itoa(i), Label: tabLabel(i)}
	}
	tab := 0
	active := "0"
	return func(c *ui.Context) {
		adv := s.step(c)
		work := 0
		if adv && s.f%tabEvery == 0 {
			tab = (tab + 1) % tabCount
			work = 1
		}
		active = strconv.Itoa(tab)
		ui.Column(c, func() {
			t := ui.Tabs(c, &active, items).Show()
			t.Panel(func() {
				ui.Column(c, func() {
					for l := 0; l < tabLines; l++ {
						ui.Text(c, tabLine(tab, l))
					}
				}, layout.Gap(2))
			})
		}, layout.Fill(), layout.Pad(pagePad))
		if adv {
			report(work)
		}
	}
}

// menuPage: a vertical menu of 12 items filling the window's height (it
// scrolls); frame f selects item f%12.
func menuPage() func(*ui.Context) {
	var s stepper
	items := make([]ui.MenuItem, menuCount)
	for i := range items {
		items[i] = ui.MenuItem{Key: menuKey(i), Label: menuLabel(i)}
	}
	sel := menuKey(0)
	return func(c *ui.Context) {
		adv := s.step(c)
		if adv {
			sel = menuKey(s.f % menuCount)
		}
		ui.Row(c, func() {
			ui.Scroll(c, func() {
				ui.Menu(c, items, &sel).Width(menuWidth).Show()
			}, layout.W(menuWidth), layout.H(viewH))
		}, layout.Fill())
		if adv {
			report(1)
		}
	}
}

// dialogPage: a modal with 3 inputs and 2 buttons (Tango's Modal footer:
// Cancel and OK), opened and closed every 30th frame.
func dialogPage() func(*ui.Context) {
	var s stepper
	open := false
	fields := dialogFields
	return func(c *ui.Context) {
		adv := s.step(c)
		work := 0
		if adv && s.f%overlayEvery == 0 {
			open = !open
			work = 1
		}
		ui.Column(c, func() {
			ui.Text(c, dialogBackdrop)
		}, layout.Fill(), layout.Pad(pagePad))
		ui.Modal(c, &open, dialogTitle).CancelText(dialogCancel).OkText(dialogOK).Body(func() {
			ui.Column(c, func() {
				for i := range fields {
					ui.Input(c, &fields[i])
				}
			}, layout.Gap(pageGap))
		})
		if adv {
			report(work)
		}
	}
}

// toastPage: every 30th frame a message (Tango's toast) that goes after a
// second.
func toastPage() func(*ui.Context) {
	var s stepper
	return func(c *ui.Context) {
		adv := s.step(c)
		work := 0
		if adv && s.f%overlayEvery == 0 {
			ui.Message(c, "info", toastText(s.f/overlayEvery)).Duration(toastFor)
			work = 1
		}
		ui.Column(c, func() {
			ui.Text(c, toastBackdrop)
		}, layout.Fill(), layout.Pad(pagePad))
		if adv {
			report(work)
		}
	}
}

// treePage: a virtual tree of 10,000 nodes, all expanded, filling the
// window; frame f centers row treeRow(f), one row further each frame.
func treePage() func(*ui.Context) {
	var s stepper
	nodes := make([]ui.TreeNode, treeGroups)
	expanded := make([]string, treeGroups)
	keys := make([]string, treeNodes)
	for id := range keys {
		keys[id] = strconv.Itoa(id)
	}
	for g := range nodes {
		kids := make([]ui.TreeNode, treeGroup-1)
		for j := range kids {
			id := g*treeGroup + j + 1
			kids[j] = ui.TreeNode{Key: keys[id], Title: treeTitle(id)}
		}
		nodes[g] = ui.TreeNode{Key: keys[g*treeGroup], Title: treeTitle(g * treeGroup), Children: kids}
		expanded[g] = keys[g*treeGroup]
	}
	return func(c *ui.Context) {
		adv := s.step(c)
		t := ui.VirtualTree(c, nodes).Expanded(&expanded).Height(viewH)
		if adv {
			t.ScrollToKey(keys[treeRow(s.f)])
		}
		t.Show()
		if adv {
			report(1)
		}
	}
}

// datepickerPage: an inline date panel; frame f selects the next day,
// within one month.
func datepickerPage() func(*ui.Context) {
	var s stepper
	day := pickDay(0)
	return func(c *ui.Context) {
		adv := s.step(c)
		if adv {
			day = pickDay(s.f)
		}
		ui.Column(c, func() {
			ui.DatePickerPanel(c, &day)
		}, layout.Fill(), layout.Pad(pagePad))
		if adv {
			report(1)
		}
	}
}

// colorpickerPage: an inline color panel; frame f turns the hue one
// degree.
func colorpickerPage() func(*ui.Context) {
	var s stepper
	col := pickColor(0)
	return func(c *ui.Context) {
		adv := s.step(c)
		if adv {
			col = pickColor(s.f)
		}
		ui.Column(c, func() {
			ui.ColorPickerPanel(c, &col)
		}, layout.Fill(), layout.Pad(pagePad))
		if adv {
			report(1)
		}
	}
}

func pickColor(f int) theme.Color {
	r, g, b := hueRGB(f % 360)
	return theme.RGBA(r, g, b, 1)
}
