package main

import (
	"strconv"
	"time"

	"github.com/egoist/mygo/ui"
)

// The nearest-counterpart scenes (../scenes.md, "Nearest-counterpart
// scenes"): MyGo's component where it has one, else the nearest one
// composed from its components, as the mapping there says. Frame f counts
// as in scene_components.go.

// shares lays out n items in rows of cols, each item an equal share.
func shares(c *ui.Context, n, cols int, gap float32, item func(i int)) {
	for r := 0; r < n/cols; r++ {
		ui.Row(c).Gap(gap).Children(func() {
			for col := 0; col < cols; col++ {
				ui.Column(c).Grow(1).Basis(0).Children(func() { item(r*cols + col) })
			}
		})
	}
}

// faint is a secondary text.
func faint(c *ui.Context, s string) ui.Element { return ui.Text(c, s).TextColor(c.Theme().TextMuted) }

// pick is a list of n rows whose row *sel is chosen, as a select's or a
// cascader's open list shows it.
func pick(c *ui.Context, st *ui.ListState, sel *int, n int, label func(i int) string, arrow bool) ui.Element {
	st.Selected = sel
	return ui.List(c, st, n, func(i int) {
		ui.Row(c).Padding(6, 12).Children(func() {
			ui.Text(c, label(i)).Grow(1)
			if arrow {
				faint(c, "›")
			}
		})
	})
}

// newSelect: an open select of 8 options; frame f chooses option f%8.
// MyGo's Select has no open control: the scene shows the Select, closed,
// with a popover under it holding the list of options, the chosen one
// selected, which is what an open select draws.
func newSelect() *selectScene {
	s := &selectScene{open: true, labels: make([]string, selectCount)}
	for i := range s.labels {
		s.labels[i] = selectLabel(i)
	}
	return s
}

type selectScene struct {
	s      stepper
	sel    int
	val    string
	open   bool
	st     ui.ListState
	labels []string
}

func (s *selectScene) view(c *ui.Context) {
	adv := s.s.step(c)
	if adv {
		s.sel = s.s.f % selectCount
	}
	s.val, s.open = s.labels[s.sel], true
	page(c).Children(func() {
		field := ui.Select(c, &s.val, s.labels).Width(selectWidth)
		ui.Popover(c, field, &s.open, func() {
			pick(c, &s.st, &s.sel, selectCount, selectLabel, false)
		})
	})
	if adv {
		report(1)
	}
}

// newTooltip: a button with a one-line popover shown and hidden every
// 30th frame. MyGo's tooltip shows only on hover or keyboard focus; its
// panel opened from code is a Popover.
func newTooltip() *tooltipScene { return &tooltipScene{} }

type tooltipScene struct {
	s    stepper
	open bool
}

func (t *tooltipScene) view(c *ui.Context) {
	adv := t.s.step(c)
	work := 0
	if adv && t.s.f%tooltipEvery == 0 {
		t.open, work = !t.open, 1
	}
	page(c).AlignItems(ui.Start).Children(func() {
		b := ui.Button(c, tooltipAnchor)
		ui.Popover(c, b, &t.open, func() { ui.Text(c, tooltipText) })
	})
	if adv {
		report(work)
	}
}

// tableColumns are the table scenes' columns.
func tableColumns() []ui.TableColumn {
	cols := make([]ui.TableColumn, tableCols)
	for i := range cols {
		cols[i] = ui.TableColumn{Title: tableTitles[i]}
	}
	return cols
}

// tableText is a table cell's text, upd the rows' Updated values.
func tableText(upd []int, r, col int) string {
	if col == tableCols-1 {
		return strconv.Itoa(upd[r])
	}
	return tableCell(r, col)
}

// MyGo's default table rows are 32 DIPs and its header 33 (measured with
// ui.NewTester, asserted in scene_nearest_test.go): the tables are as high
// as their header and the scene's rows, so each shows as many rows as the
// other programs' (scenes.md).
const (
	mygoRowH  = 32
	mygoHeadH = 33
)

// newTable: a table of 1000 rows and 6 columns, 8 rows in view; frame f
// writes f into row f%8's Updated cell. No scrolling.
func newTable() *tableScene {
	return &tableScene{upd: make([]int, tableRows), cols: tableColumns()}
}

type tableScene struct {
	s    stepper
	st   ui.ListState
	upd  []int
	cols []ui.TableColumn
}

func (t *tableScene) view(c *ui.Context) {
	adv := t.s.step(c)
	if adv {
		t.upd[tableRow(t.s.f)] = t.s.f
	}
	page(c).Children(func() {
		ui.Table(c, &t.st, t.cols, tableRows, func(r, col int) { ui.Text(c, tableText(t.upd, r, col)) }).
			Height(mygoHeadH + tableShown*mygoRowH)
	})
	if adv {
		report(1)
	}
}

// newPagination: the 10 items of a page over a pager of 50 pages; frame f
// shows page 1+f%50. MyGo has no pagination: the pager is composed of
// buttons, previous and next and the pages antd shows (pagerItems), the
// current one primary, an ellipsis for the pages left out.
func newPagination() *paginationScene { return &paginationScene{page: 1} }

type paginationScene struct {
	s    stepper
	page int
}

func (p *paginationScene) view(c *ui.Context) {
	adv := p.s.step(c)
	if adv {
		p.page = pageOf(p.s.f)
	}
	page(c).Children(func() {
		for i := 0; i < pageSize; i++ {
			ui.Text(c, pageItem(p.page, i))
		}
		ui.Row(c).Gap(pageGap).AlignItems(ui.Center).Children(func() {
			ui.Button(c, "‹").Disabled(p.page == 1)
			for _, n := range pagerItems(p.page) {
				switch {
				case n == 0:
					faint(c, "…")
				case n == p.page:
					ui.PrimaryButton(c, strconv.Itoa(n))
				default:
					ui.Button(c, strconv.Itoa(n))
				}
			}
			ui.Button(c, "›").Disabled(p.page == pageCount)
		})
	})
	if adv {
		report(1)
	}
}

// newCascader: a cascader open on 3 levels of 5; frame f chooses the path
// cascadePath(f). MyGo has no cascader: a Combobox shows the path, and a
// popover under it holds three lists, one a level, each with its choice
// selected.
func newCascader() *cascaderScene { return &cascaderScene{open: true} }

type cascaderScene struct {
	s     stepper
	path  [3]int
	text  string
	open  bool
	lists [3]ui.ListState
}

func (k *cascaderScene) view(c *ui.Context) {
	adv := k.s.step(c)
	if adv {
		k.path = cascadePath(k.s.f)
	}
	k.text, k.open = cascadeText(k.path), true
	page(c).Children(func() {
		field := ui.Combobox(c, &k.text, nil).Width(3 * cascadeWidth)
		ui.Popover(c, field, &k.open, func() {
			ui.Row(c).Children(func() {
				for lv := range k.path {
					above := k.path[:lv]
					pick(c, &k.lists[lv], &k.path[lv], cascadeFan, func(i int) string { return cascadeLabel(above, i) }, lv < 2).
						Width(cascadeWidth)
				}
			})
		})
	})
	if adv {
		report(1)
	}
}

// newTransfer: two lists of check boxes, 10 items between them, and the
// two move buttons; frame f moves item f%10 to the other side. MyGo has no
// transfer: each side is a bordered box, a header (the count and the
// title) over its items' check boxes.
func newTransfer() *transferScene {
	t := &transferScene{right: make([]bool, transferCount), checks: make([]bool, transferCount)}
	for i := range t.right {
		t.right[i] = transferStart(i)
	}
	return t
}

type transferScene struct {
	s      stepper
	right  []bool
	checks []bool
}

func (t *transferScene) side(c *ui.Context, right bool, title string) {
	th := c.Theme()
	n := 0
	for _, r := range t.right {
		if r == right {
			n++
		}
	}
	ui.Column(c).Size(300, 400).Border(1, th.Border).Radius(th.Radius).Children(func() {
		ui.Row(c).Padding(8, 12).Children(func() {
			ui.Text(c, strconv.Itoa(n)+" items").Grow(1)
			ui.Text(c, title)
		})
		ui.Divider(c)
		ui.Column(c).Padding(4, 12).Gap(4).Children(func() {
			for i, r := range t.right {
				if r == right {
					ui.Checkbox(c, &t.checks[i], transferLabel(i))
				}
			}
		})
	})
}

func (t *transferScene) view(c *ui.Context) {
	adv := t.s.step(c)
	if adv {
		i := t.s.f % transferCount
		t.right[i] = !t.right[i]
	}
	page(c).Children(func() {
		ui.Row(c).Gap(pageGap).AlignItems(ui.Center).Children(func() {
			t.side(c, false, transferLeft)
			ui.Column(c).Gap(pageGap).Children(func() {
				ui.Button(c, "›").Disabled(true)
				ui.Button(c, "‹").Disabled(true)
			})
			t.side(c, true, transferRight)
		})
	})
	if adv {
		report(1)
	}
}

// newTreeselect: a tree select open on 2 groups of 3 leaves, all
// expanded; frame f chooses leaf treeSelPick(f). MyGo has no tree select:
// its Select, closed, shows the leaf, and a popover under it holds a Tree
// with the leaf selected.
func newTreeselect() *treeselectScene {
	t := &treeselectScene{open: true, groups: make([]bool, treeSelGroups)}
	for g := range t.groups {
		t.groups[g] = true
		for j := 0; j < treeSelLeaves; j++ {
			t.leaves = append(t.leaves, treeSelLeaf(g, j))
		}
	}
	return t
}

type treeselectScene struct {
	s      stepper
	g, j   int
	val    string
	open   bool
	groups []bool
	leaves []string
}

func (t *treeselectScene) view(c *ui.Context) {
	adv := t.s.step(c)
	if adv {
		t.g, t.j = treeSelPick(t.s.f)
	}
	t.val, t.open = treeSelLeaf(t.g, t.j), true
	for g := range t.groups {
		t.groups[g] = true
	}
	page(c).Children(func() {
		field := ui.Select(c, &t.val, t.leaves).Width(300)
		ui.Popover(c, field, &t.open, func() {
			ui.Tree(c, func() {
				for g := range t.groups {
					ui.TreeItem(c, treeSelGroup(g), &t.groups[g], func() {
						for j := 0; j < treeSelLeaves; j++ {
							ui.TreeItem(c, treeSelLeaf(g, j), nil, nil).Selected(g == t.g && j == t.j)
						}
					})
				}
			})
		})
	})
	if adv {
		report(1)
	}
}

// newCalendar: MyGo's calendar (its only one, which the datepicker scene
// shows too); frame f selects pickDay(f).
func newCalendar() *calendarScene { return &calendarScene{day: pickDay(0)} }

type calendarScene struct {
	s   stepper
	day time.Time
}

func (k *calendarScene) view(c *ui.Context) {
	adv := k.s.step(c)
	if adv {
		k.day = pickDay(k.s.f)
	}
	page(c).Children(func() { ui.Calendar(c, &k.day) })
	if adv {
		report(1)
	}
}

// newSteps: 5 steps; frame f makes step f%5 the current one. MyGo has no
// steps: each is a numbered circle (a check once done, filled while
// current) beside its title and details, joined by lines.
func newSteps() *stepsScene { return &stepsScene{} }

type stepsScene struct {
	s   stepper
	cur int
}

func (st *stepsScene) view(c *ui.Context) {
	adv := st.s.step(c)
	if adv {
		st.cur = st.s.f % stepCount
	}
	th := c.Theme()
	page(c).Children(func() {
		ui.Row(c).Gap(pageGap).AlignItems(ui.Center).Children(func() {
			for i := 0; i < stepCount; i++ {
				if i > 0 {
					ui.Box(c).Height(1).Grow(1).Background(th.Border)
				}
				mark, bg, fg := strconv.Itoa(i+1), th.Surface, th.TextMuted
				switch {
				case i < st.cur:
					mark, fg = "✓", th.Accent
				case i == st.cur:
					bg, fg = th.Accent, th.AccentText
				}
				ui.Row(c).Gap(pageGap).AlignItems(ui.Center).Children(func() {
					ui.Column(c).Size(32, 32).Radius(16).Background(bg).Center().Children(func() {
						ui.Text(c, mark).TextColor(fg)
					})
					ui.Column(c).Children(func() {
						ui.Text(c, stepTitle(i))
						faint(c, stepDesc(i))
					})
				})
			}
		})
	})
	if adv {
		report(1)
	}
}

// newTimeline: 8 events, each its text over its time; frame f rewrites
// event f%8. MyGo has no timeline: each event is a dot on a line beside
// its two texts.
func newTimeline() *timelineScene { return &timelineScene{changed: make([]int, timelineCount)} }

type timelineScene struct {
	s       stepper
	changed []int
}

func (t *timelineScene) view(c *ui.Context) {
	adv := t.s.step(c)
	if adv {
		t.changed[t.s.f%timelineCount] = t.s.f
	}
	th := c.Theme()
	page(c).Children(func() {
		for i := range t.changed {
			ui.Row(c).Gap(12).Children(func() {
				ui.Column(c).AlignItems(ui.Center).Width(10).Children(func() {
					ui.Box(c).Size(10, 10).Radius(5).Border(2, th.Accent)
					if i < timelineCount-1 {
						ui.Box(c).Width(2).Grow(1).Background(th.Border)
					}
				})
				ui.Column(c).Padding(0, 0, 12, 0).Children(func() {
					ui.Text(c, timelineText(i, t.changed[i]))
					faint(c, timelineTime(i))
				})
			})
		}
	})
	if adv {
		report(1)
	}
}

// newDescriptions: 12 label/value items in 3 bordered columns; frame f
// rewrites value f%12. MyGo has no descriptions: rows of bordered cells,
// the labels on the surface color.
func newDescriptions() *descriptionsScene {
	return &descriptionsScene{changed: make([]int, descCount)}
}

type descriptionsScene struct {
	s       stepper
	changed []int
}

func (d *descriptionsScene) view(c *ui.Context) {
	adv := d.s.step(c)
	if adv {
		d.changed[d.s.f%descCount] = d.s.f
	}
	th := c.Theme()
	page(c).Children(func() {
		ui.Column(c).Border(1, th.Border).Radius(th.Radius).Clip().Children(func() {
			for r := 0; r < descCount/descCols; r++ {
				ui.Row(c).Children(func() {
					for col := 0; col < descCols; col++ {
						i := r*descCols + col
						ui.Text(c, descLabel(i)).Padding(12, 16).Background(th.Surface).Border(1, th.Border)
						ui.Text(c, descValue(i, d.changed[i])).Padding(12, 16).Grow(1).Basis(0).Border(1, th.Border)
					}
				})
			}
		})
	})
	if adv {
		report(1)
	}
}

// newForm: a form of 10 labelled inputs in two columns of 5 and a submit
// button; frame f types into field f%10. MyGo's Form and Field.
func newForm() *formScene {
	f := &formScene{vals: make([]string, formFields)}
	for i := range f.vals {
		f.vals[i] = inputSeed
	}
	return f
}

type formScene struct {
	s    stepper
	vals []string
}

func (f *formScene) view(c *ui.Context) {
	adv := f.s.step(c)
	if adv {
		i := f.s.f % formFields
		f.vals[i] = typeInto(f.vals[i], f.s.f)
	}
	page(c).Children(func() {
		ui.Row(c).Gap(pagePad).Children(func() {
			for col := 0; col < 2; col++ {
				ui.Form(c, func() {
					for r := 0; r < formFields/2; r++ {
						i := col*formFields/2 + r
						ui.Field(c, formLabel(i), func() { ui.TextInput(c, &f.vals[i]) })
					}
				}).Grow(1).Basis(0)
			}
		})
		ui.Row(c).Children(func() { ui.PrimaryButton(c, formSubmit) })
	})
	if adv {
		report(1)
	}
}

// newUpload: an upload button over 8 files uploading; frame f moves file
// f%8's progress one percent. MyGo has no upload: each file is its name
// over a progress bar.
func newUpload() *uploadScene {
	u := &uploadScene{pct: make([]float64, uploadCount)}
	for i := range u.pct {
		u.pct[i] = uploadStart(i)
	}
	return u
}

type uploadScene struct {
	s   stepper
	pct []float64
}

func (u *uploadScene) view(c *ui.Context) {
	adv := u.s.step(c)
	if adv {
		i := u.s.f % uploadCount
		u.pct[i] = nextPercent(u.pct[i])
	}
	page(c).Children(func() {
		ui.Row(c).Children(func() { ui.Button(c, uploadButton) })
		ui.Column(c).Width(520).Gap(pageGap).Children(func() {
			for i, p := range u.pct {
				ui.Column(c).Gap(2).Children(func() {
					ui.Text(c, uploadName(i))
					ui.Progress(c, p/100)
				})
			}
		})
	})
	if adv {
		report(1)
	}
}

// newCarousel: 5 slides 240 px high; every 30th frame the next slide.
// MyGo has no carousel: the slide in view over a row of dots, the current
// one wider; the slide changes at once.
func newCarousel() *carouselScene { return &carouselScene{} }

type carouselScene struct {
	s   stepper
	cur int
}

func (k *carouselScene) view(c *ui.Context) {
	adv := k.s.step(c)
	work := 0
	if adv && k.s.f%carouselEvery == 0 {
		k.cur, work = (k.cur+1)%carouselCount, 1
	}
	th := c.Theme()
	page(c).Children(func() {
		ui.Column(c).Height(carouselHeight).Background(ui.Hex("#dfe7f2")).Center().Children(func() {
			ui.Text(c, slideLabel(k.cur)).FontSize(24)
		})
		ui.Row(c).Gap(6).Justify(ui.Center).Children(func() {
			for i := 0; i < carouselCount; i++ {
				w, col := float32(16), th.Border
				if i == k.cur {
					w, col = 24, th.Accent
				}
				ui.Box(c).Size(w, 3).Radius(2).Background(col)
			}
		})
	})
	if adv {
		report(work)
	}
}

// newCollapse: an accordion of 6 panels of 3 lines; every 30th frame the
// next panel opens (and the open one closes). MyGo's Accordion.
func newCollapse() *collapseScene { return &collapseScene{open: make([]bool, collapseCount)} }

type collapseScene struct {
	s    stepper
	cur  int
	open []bool
}

func (k *collapseScene) view(c *ui.Context) {
	adv := k.s.step(c)
	work := 0
	if adv && k.s.f%collapseEvery == 0 {
		k.cur, work = (k.cur+1)%collapseCount, 1
	}
	page(c).Children(func() {
		ui.Accordion(c, func() {
			for i := range k.open {
				k.open[i] = i == k.cur
				ui.AccordionItem(c, collapseLabel(i), &k.open[i], func() {
					for l := 0; l < collapseLines; l++ {
						ui.Text(c, collapseLine(i, l))
					}
				})
			}
		})
	})
	if adv {
		report(work)
	}
}

// newDrawer: a 360 px drawer at the right with 3 inputs and 2 buttons,
// opened and closed every 30th frame. MyGo has no drawer: its documented
// one (docs/ui/overlays.md), a panel on the right of a dimmed backdrop in
// an Overlay, with a title, the inputs and the buttons.
func newDrawer() *drawerScene { return &drawerScene{fields: dialogFields} }

type drawerScene struct {
	s      stepper
	open   bool
	fields [3]string
}

func (d *drawerScene) view(c *ui.Context) {
	adv := d.s.step(c)
	work := 0
	if adv && d.s.f%overlayEvery == 0 {
		d.open, work = !d.open, 1
	}
	th := c.Theme()
	page(c).Children(func() { ui.Text(c, drawerBackdrop) })
	if d.open {
		ui.Overlay(c, func() {
			ui.Row(c).Absolute().Left(0).Top(0).Right(0).Bottom(0).Justify(ui.End).
				Background(ui.RGBA(0, 0, 0, 0.45)).Modal().Children(func() {
				ui.Column(c).Width(drawerWidth).FillHeight().Padding(pagePad).Gap(pagePad).Background(th.Background).
					Role(ui.RoleDialog).Label(drawerTitle).Children(func() {
					ui.Text(c, drawerTitle).Bold()
					ui.Column(c).Gap(pageGap).Grow(1).Children(func() {
						for i := range d.fields {
							ui.TextInput(c, &d.fields[i])
						}
					})
					ui.Row(c).Gap(pageGap).Justify(ui.End).Children(func() {
						ui.Button(c, dialogCancel)
						ui.PrimaryButton(c, dialogOK)
					})
				})
			})
		})
	}
	if adv {
		report(work)
	}
}

// newTour: 3 buttons and a tour of 3 steps pointing at them over a mask;
// every 30th frame the next step. MyGo has no tour: a dimmed backdrop in
// an Overlay and a popover at the step's button with its title, text and
// the Previous and Next buttons.
func newTour() *tourScene { return &tourScene{open: true} }

type tourScene struct {
	s    stepper
	cur  int
	open bool
}

func (t *tourScene) view(c *ui.Context) {
	adv := t.s.step(c)
	work := 0
	if adv && t.s.f%tourEvery == 0 {
		t.cur, work = (t.cur+1)%tourCount, 1
	}
	t.open = true
	var target ui.Element
	page(c).Children(func() {
		ui.Row(c).Gap(pageGap).Children(func() {
			for i, label := range tourButtons {
				if b := ui.Button(c, label); i == t.cur {
					target = b
				}
			}
		})
	})
	ui.Overlay(c, func() {
		ui.Box(c).Absolute().Left(0).Top(0).Right(0).Bottom(0).Background(ui.RGBA(0, 0, 0, 0.45))
	})
	ui.Popover(c, target, &t.open, func() {
		ui.Column(c).Gap(pageGap).Width(260).Children(func() {
			ui.Text(c, tourTitles[t.cur]).Bold()
			ui.Text(c, tourDesc(t.cur))
			ui.Row(c).Gap(pageGap).Justify(ui.End).Children(func() {
				ui.Button(c, "Previous")
				ui.PrimaryButton(c, "Next")
			})
		})
	})
	if adv {
		report(work)
	}
}

// newProtable: a query form of one field (Name, with Reset and Query) over
// a table of 6 rows of the table scene's columns, all in view; frame f
// writes f into row f%6's Updated cell. MyGo has no ProTable: a Field and two buttons
// over its Table.
func newProtable() *protableScene {
	return &protableScene{upd: make([]int, proRows), cols: tableColumns()}
}

type protableScene struct {
	s     stepper
	st    ui.ListState
	upd   []int
	cols  []ui.TableColumn
	query string
}

func (p *protableScene) view(c *ui.Context) {
	adv := p.s.step(c)
	if adv {
		p.upd[proRow(p.s.f)] = p.s.f
	}
	page(c).Children(func() {
		ui.Row(c).Gap(pageGap).AlignItems(ui.Center).Children(func() {
			ui.Form(c, func() {
				ui.Field(c, tableTitles[1], func() { ui.TextInput(c, &p.query).Placeholder("Please enter").Width(240) })
			})
			ui.Spacer(c)
			ui.Button(c, resetButton)
			ui.PrimaryButton(c, queryButton)
		})
		ui.Table(c, &p.st, p.cols, proRows, func(r, col int) { ui.Text(c, tableText(p.upd, r, col)) }).
			Height(mygoHeadH + proRows*mygoRowH)
	})
	if adv {
		report(1)
	}
}

// newSegmented: 10 segmented controls of 4 choices; frame f moves control
// f%10 to its next choice. MyGo's Segmented.
func newSegmented() *segmentedScene {
	s := &segmentedScene{chosen: make([]int, segCount)}
	for i := range s.chosen {
		s.chosen[i] = segStart(i)
	}
	return s
}

type segmentedScene struct {
	s      stepper
	chosen []int
}

func (s *segmentedScene) view(c *ui.Context) {
	adv := s.s.step(c)
	if adv {
		i := s.s.f % segCount
		s.chosen[i] = (s.chosen[i] + 1) % segChoices
	}
	page(c).AlignItems(ui.Start).Children(func() {
		for i := range s.chosen {
			ui.Segmented(c, &s.chosen[i], segLabels[:]...)
		}
	})
	if adv {
		report(1)
	}
}

// newRate: 20 five-star ratings in two columns; frame f moves rating f%20
// one star on (past 5 back to 0). MyGo's Rating.
func newRate() *rateScene {
	r := &rateScene{vals: make([]int, rateCount)}
	for i := range r.vals {
		r.vals[i] = rateStart(i)
	}
	return r
}

type rateScene struct {
	s    stepper
	vals []int
}

func (r *rateScene) view(c *ui.Context) {
	adv := r.s.step(c)
	if adv {
		i := r.s.f % rateCount
		r.vals[i] = (r.vals[i] + 1) % (rateMax + 1)
	}
	page(c).Children(func() {
		shares(c, rateCount, rateCols, pagePad, func(i int) { ui.Rating(c, &r.vals[i], rateMax) })
	})
	if adv {
		report(1)
	}
}

// newTags: 60 tags in rows of 6; frame f relabels tag f%60. MyGo's tag is
// its Badge, a short text in a pill.
func newTags() *tagsScene {
	t := &tagsScene{labels: make([]string, tagCount)}
	for i := range t.labels {
		t.labels[i] = tagLabel(i)
	}
	return t
}

type tagsScene struct {
	s      stepper
	labels []string
}

func (t *tagsScene) view(c *ui.Context) {
	adv := t.s.step(c)
	if adv {
		i := t.s.f % tagCount
		t.labels[i] = taggedLabel(i, t.s.f)
	}
	page(c).Children(func() {
		for r := 0; r < tagCount/tagCols; r++ {
			ui.Row(c).Gap(pageGap).Children(func() {
				for col := 0; col < tagCols; col++ {
					ui.Badge(c, t.labels[r*tagCols+col])
				}
			})
		}
	})
	if adv {
		report(1)
	}
}

// newBadges: 40 squares with a count badge, rows of 10; frame f adds one to
// badge f%40 (1..99). MyGo's Badge on the square's corner.
func newBadges() *badgesScene {
	b := &badgesScene{counts: make([]int, badgeCount)}
	for i := range b.counts {
		b.counts[i] = badgeStart(i)
	}
	return b
}

type badgesScene struct {
	s      stepper
	counts []int
}

func (b *badgesScene) view(c *ui.Context) {
	adv := b.s.step(c)
	if adv {
		i := b.s.f % badgeCount
		b.counts[i] = nextCount(b.counts[i])
	}
	probe(ui.Column(c).Fill().Padding(2 * pagePad).Gap(2 * pagePad)).Children(func() {
		for r := 0; r < badgeCount/badgeCols; r++ {
			ui.Row(c).Gap(2 * pagePad).Children(func() {
				for col := 0; col < badgeCols; col++ {
					ui.Box(c).Size(badgeSize, badgeSize).Radius(6).Background(ui.Hex("#d9dde3")).Children(func() {
						ui.Badge(c, strconv.Itoa(b.counts[r*badgeCols+col])).Absolute().Top(-8).Right(-8)
					})
				}
			})
		}
	})
	if adv {
		report(1)
	}
}

// newCards: 12 cards in rows of 4, each a title over 2 lines; frame f
// rewrites the second line of card f%12. MyGo has no card: a bordered,
// rounded, padded column.
func newCards() *cardsScene { return &cardsScene{updated: make([]int, cardCount)} }

type cardsScene struct {
	s       stepper
	updated []int
}

func (k *cardsScene) view(c *ui.Context) {
	adv := k.s.step(c)
	if adv {
		k.updated[k.s.f%cardCount] = k.s.f
	}
	th := c.Theme()
	page(c).Children(func() {
		shares(c, cardCount, cardCols, pageGap, func(i int) {
			ui.Column(c).Padding(pagePad).Gap(pageGap).Border(1, th.Border).Radius(8).Background(th.Background).Children(func() {
				ui.Text(c, cardTitle(i)).Bold()
				ui.Text(c, cardLine(i))
				ui.Text(c, cardUpdated(k.updated[i]))
			})
		})
	})
	if adv {
		report(1)
	}
}

// newStatistics: 12 statistics in rows of 4; frame f changes the value of
// statistic f%12. MyGo has no statistic: a muted title over a 24 px value.
func newStatistics() *statisticsScene {
	s := &statisticsScene{vals: make([]float64, statCount)}
	for i := range s.vals {
		s.vals[i] = statValue(i, 0)
	}
	return s
}

type statisticsScene struct {
	s    stepper
	vals []float64
}

func (s *statisticsScene) view(c *ui.Context) {
	adv := s.s.step(c)
	if adv {
		i := s.s.f % statCount
		s.vals[i] = statValue(i, s.s.f)
	}
	page(c).Children(func() {
		shares(c, statCount, statCols, pagePad, func(i int) {
			faint(c, statTitle(i))
			ui.Text(c, statText(s.vals[i])).FontSize(24)
		})
	})
	if adv {
		report(1)
	}
}
