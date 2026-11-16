package main

import (
	"time"

	"github.com/egoist/mygo/ui"
)

// The component scenes, the same in every framework's program
// (../scenes.md, "Component scenes"). Frame f is 1 on the first frame
// that does the scene's work (the ready frame) and counts up from there.

// stepper drives a component view: it asks for the next frame in every
// build pass (c.AnimationFrame, MyGo's per-frame request) and advances f
// once per new frame, from the second frame on.
type stepper struct {
	k clock
	f int
}

func (s *stepper) step(c *ui.Context) bool {
	c.AnimationFrame()
	if s.k.next(c) && s.k.frames >= 2 {
		s.f++
		return true
	}
	return false
}

// page is the padded column every component scene but menu and tree
// sits in.
func page(c *ui.Context) ui.Element {
	return probe(ui.Column(c).Fill().Padding(pagePad).Gap(pageGap))
}

// buttons: 60 buttons in rows of 6; frame f relabels button f%60.
type buttons struct {
	s      stepper
	labels []string
}

func newButtons() *buttons {
	b := &buttons{labels: make([]string, buttonCount)}
	for i := range b.labels {
		b.labels[i] = buttonLabel(i)
	}
	return b
}

func (b *buttons) view(c *ui.Context) {
	adv := b.s.step(c)
	if adv {
		b.labels[b.s.f%buttonCount] = clickedLabel(b.s.f)
	}
	page(c).Children(func() {
		for r := 0; r < buttonCount/buttonCols; r++ {
			ui.Row(c).Gap(pageGap).Children(func() {
				for col := 0; col < buttonCols; col++ {
					ui.Button(c, b.labels[r*buttonCols+col])
				}
			})
		}
	})
	if adv {
		report(1)
	}
}

// halves lays out n controls in two columns of n/2.
func halves(c *ui.Context, n int, control func(i int)) {
	probe(ui.Row(c).Fill().Padding(pagePad).Gap(pagePad)).Children(func() {
		for col := 0; col < 2; col++ {
			ui.Column(c).Grow(1).Basis(0).Gap(pageGap).Children(func() {
				for r := 0; r < n/2; r++ {
					control(col*n/2 + r)
				}
			})
		}
	})
}

// inputs: 20 single-line inputs in two columns; frame f types one
// character into input f%20.
type inputs struct {
	s    stepper
	vals []string
}

func newInputs() *inputs {
	in := &inputs{vals: make([]string, inputCount)}
	for i := range in.vals {
		in.vals[i] = inputSeed
	}
	return in
}

func (in *inputs) view(c *ui.Context) {
	adv := in.s.step(c)
	if adv {
		i := in.s.f % inputCount
		in.vals[i] = typeInto(in.vals[i], in.s.f)
	}
	halves(c, inputCount, func(i int) { ui.TextInput(c, &in.vals[i]) })
	if adv {
		report(1)
	}
}

// textarea: one multi-line input filling the window, 2000 characters to
// start; frame f appends one.
type textarea struct {
	s         stepper
	seed, val string
}

func newTextarea() *textarea {
	seed := textareaSeed()
	return &textarea{seed: seed, val: seed}
}

func (t *textarea) view(c *ui.Context) {
	adv := t.s.step(c)
	if adv {
		t.val = appendText(t.val, t.seed, t.s.f)
	}
	page(c).Children(func() { ui.TextArea(c, &t.val).Grow(1) })
	if adv {
		report(1)
	}
}

// checkboxes: 100 check boxes in 5 columns; frame f toggles f%100.
type checkboxes struct {
	s      stepper
	on     []bool
	labels []string
}

func newCheckboxes() *checkboxes {
	b := &checkboxes{on: make([]bool, toggleCount), labels: make([]string, toggleCount)}
	for i := range b.labels {
		b.labels[i] = checkLabel(i)
	}
	return b
}

func (b *checkboxes) view(c *ui.Context) {
	adv := b.s.step(c)
	if adv {
		i := b.s.f % toggleCount
		b.on[i] = !b.on[i]
	}
	probe(ui.Column(c).Fill().Padding(pagePad).Gap(4)).Children(func() {
		for r := 0; r < toggleCount/checkCols; r++ {
			ui.Row(c).Gap(pageGap).Children(func() {
				for col := 0; col < checkCols; col++ {
					i := r*checkCols + col
					ui.Checkbox(c, &b.on[i], b.labels[i])
				}
			})
		}
	})
	if adv {
		report(1)
	}
}

// radios: 20 groups of 5 radios in two columns; frame f moves group f%20
// to its next radio.
type radios struct {
	s      stepper
	chosen []int
}

func (r *radios) view(c *ui.Context) {
	adv := r.s.step(c)
	if r.chosen == nil {
		r.chosen = make([]int, radioGroups)
	}
	if adv {
		g := r.s.f % radioGroups
		r.chosen[g] = (r.chosen[g] + 1) % radioChoices
	}
	halves(c, radioGroups, func(g int) {
		ui.RadioGroup(c, func() {
			for i := 0; i < radioChoices; i++ {
				ui.Radio(c, &r.chosen[g], i, radioLabel(i))
			}
		}).Row().Gap(pageGap)
	})
	if adv {
		report(1)
	}
}

// switches: 100 switches, 10 by 10; frame f toggles f%100.
type switches struct {
	s  stepper
	on []bool
}

func newSwitches() *switches {
	w := &switches{on: make([]bool, toggleCount)}
	for i := range w.on {
		w.on[i] = switchStart(i)
	}
	return w
}

func (w *switches) view(c *ui.Context) {
	adv := w.s.step(c)
	if adv {
		i := w.s.f % toggleCount
		w.on[i] = !w.on[i]
	}
	page(c).Children(func() {
		for r := 0; r < toggleCount/switchCols; r++ {
			ui.Row(c).Gap(pagePad).Children(func() {
				for col := 0; col < switchCols; col++ {
					ui.Switch(c, &w.on[r*switchCols+col])
				}
			})
		}
	})
	if adv {
		report(1)
	}
}

// sliders: 20 sliders 0..100 in two columns; frame f moves slider f%20
// one step.
type sliders struct {
	s    stepper
	vals []float64
}

func newSliders() *sliders {
	l := &sliders{vals: make([]float64, sliderCount)}
	for i := range l.vals {
		l.vals[i] = sliderStart(i)
	}
	return l
}

func (l *sliders) view(c *ui.Context) {
	adv := l.s.step(c)
	if adv {
		i := l.s.f % sliderCount
		l.vals[i] = nextPercent(l.vals[i])
	}
	halves(c, sliderCount, func(i int) { ui.Slider(c, &l.vals[i], 0, 100) })
	if adv {
		report(1)
	}
}

// progress: 50 bars in two columns, no text; every frame moves all of
// them one percent.
type progress struct {
	s    stepper
	vals []float64
}

func newProgress() *progress {
	p := &progress{vals: make([]float64, progressCount)}
	for i := range p.vals {
		p.vals[i] = progressStart(i)
	}
	return p
}

func (p *progress) view(c *ui.Context) {
	adv := p.s.step(c)
	if adv {
		for i := range p.vals {
			p.vals[i] = nextPercent(p.vals[i])
		}
	}
	halves(c, progressCount, func(i int) { ui.Progress(c, p.vals[i]/100) })
	if adv {
		report(progressCount)
	}
}

// tabs: 10 tabs of 20 lines each; every 10th frame chooses the next.
type tabs struct {
	s      stepper
	tab    int
	labels []string
}

func newTabs() *tabs {
	t := &tabs{labels: make([]string, tabCount)}
	for i := range t.labels {
		t.labels[i] = tabLabel(i)
	}
	return t
}

func (t *tabs) view(c *ui.Context) {
	adv := t.s.step(c)
	work := 0
	if adv && t.s.f%tabEvery == 0 {
		t.tab = (t.tab + 1) % tabCount
		work = 1
	}
	page(c).Children(func() {
		ui.Tabs(c, &t.tab, t.labels...)
		ui.Column(c).Gap(2).Children(func() {
			for l := 0; l < tabLines; l++ {
				ui.Text(c, tabLine(t.tab, l))
			}
		})
	})
	if adv {
		report(work)
	}
}

// menu: a vertical menu of 50 items filling the window's height (it
// scrolls); frame f selects item f%50. MyGo's vertical, selectable menu
// is its Sidebar (a source list).
type menu struct {
	s   stepper
	sel string
}

func (m *menu) view(c *ui.Context) {
	adv := m.s.step(c)
	if adv || m.sel == "" {
		m.sel = menuKey(m.s.f % menuCount)
	}
	probe(ui.Row(c).Fill()).Children(func() {
		ui.Sidebar(c, &m.sel, func() {
			for i := 0; i < menuCount; i++ {
				ui.SidebarItem(c, menuKey(i), nil, menuLabel(i))
			}
		}).Width(menuWidth).Height(viewH)
	})
	if adv {
		report(1)
	}
}

// dialog: a modal with 3 inputs and 2 buttons, opened and closed every
// 30th frame.
type dialog struct {
	s      stepper
	open   bool
	fields [3]string
}

func newDialog() *dialog { return &dialog{fields: dialogFields} }

func (d *dialog) view(c *ui.Context) {
	adv := d.s.step(c)
	work := 0
	if adv && d.s.f%overlayEvery == 0 {
		d.open = !d.open
		work = 1
	}
	page(c).Children(func() { ui.Text(c, dialogBackdrop) })
	ui.Modal(c, &d.open, func() {
		ui.Text(c, dialogTitle).Bold()
		for i := range d.fields {
			ui.TextInput(c, &d.fields[i])
		}
		ui.Row(c).Gap(pageGap).Justify(ui.End).Children(func() {
			ui.Button(c, dialogCancel)
			ui.PrimaryButton(c, dialogOK)
		})
	})
	if adv {
		report(work)
	}
}

// toast: every 30th frame a toast that goes after a second.
type toast struct{ s stepper }

func (t *toast) view(c *ui.Context) {
	adv := t.s.step(c)
	work := 0
	if adv && t.s.f%overlayEvery == 0 {
		c.AddToast(ui.Toast{Title: toastText(t.s.f / overlayEvery), Timeout: toastFor})
		work = 1
	}
	page(c).Children(func() { ui.Text(c, toastBackdrop) })
	if adv {
		report(work)
	}
}

// tree: 10,000 nodes, all expanded, filling the window; frame f centers
// row treeRow(f), one row further each frame. MyGo's tree for thousands
// of items is its Outline, which builds only the rows in view (its Tree
// builds every item; ui/tree.go's documentation sends large trees to
// Outline).
type tree struct {
	s     stepper
	st    ui.OutlineState[int]
	roots []int
	kids  map[int][]int
}

func newTree() *tree {
	t := &tree{roots: make([]int, treeGroups), kids: make(map[int][]int, treeGroups)}
	for g := range t.roots {
		id := g * treeGroup
		t.roots[g] = id
		leaves := make([]int, treeGroup-1)
		for j := range leaves {
			leaves[j] = id + j + 1
		}
		t.kids[id] = leaves
		t.st.Open.Add(id)
	}
	return t
}

func (t *tree) view(c *ui.Context) {
	adv := t.s.step(c)
	if adv {
		t.st.List.ScrollTo(treeRow(t.s.f), ui.Center)
	}
	probe(ui.Outline(c, &t.st, t.roots, func(id int) []int { return t.kids[id] }, func(id int) {
		ui.Text(c, treeTitle(id))
	}).Fill())
	if adv {
		report(1)
	}
}

// datepicker: an inline calendar (MyGo's date panel, which its DateInput
// opens); frame f selects the next day, within one month.
type datepicker struct {
	s   stepper
	day time.Time
}

func (d *datepicker) view(c *ui.Context) {
	adv := d.s.step(c)
	if adv || d.day.IsZero() {
		d.day = pickDay(d.s.f)
	}
	page(c).Children(func() { ui.Calendar(c, &d.day) })
	if adv {
		report(1)
	}
}

// colorpicker: the inline color picker (what its ColorWell opens); frame
// f turns the hue one degree.
type colorpicker struct {
	s   stepper
	col ui.Color
	set bool
}

func (p *colorpicker) view(c *ui.Context) {
	adv := p.s.step(c)
	if adv || !p.set {
		r, g, b := hueRGB(p.s.f % 360)
		p.col, p.set = ui.RGB(r, g, b), true
	}
	page(c).Children(func() { ui.ColorPicker(c, &p.col) })
	if adv {
		report(1)
	}
}
