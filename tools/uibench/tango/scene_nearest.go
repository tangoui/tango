package main

import (
	"strconv"

	"github.com/tangoui/tango/ui"
	"github.com/tangoui/tango/ui/layout"
	"github.com/tangoui/tango/ui/theme"
)

// The nearest-counterpart scenes (../scenes.md, "Nearest-counterpart
// scenes"): Tango uses its component; the opponents use theirs or compose
// the nearest one. Frame f counts as in scene_components.go.

// pageCol is the padded column most of these scenes sit in.
func pageCol(c *ui.Context, fn func()) {
	ui.Column(c, fn, layout.Fill(), layout.Gap(pageGap), layout.Pad(pagePad))
}

// shareRows lays out n items in rows of cols, each item an equal share.
func shareRows(c *ui.Context, n, cols int, gap float32, item func(i int)) {
	for r := 0; r < n/cols; r++ {
		ui.Row(c, func() {
			for col := 0; col < cols; col++ {
				ui.Column(c, func() { item(r*cols + col) }, layout.Grow(1), layout.Basis(0))
			}
		}, layout.Gap(gap))
	}
}

// selectPage: an open select of 8 options; frame f chooses option f%8.
// Tango opens its Select with Open(); the list needs no scrolling.
func selectPage() func(*ui.Context) {
	var s stepper
	opts := make([]ui.Option, selectCount)
	for i := range opts {
		opts[i] = ui.Option{Label: selectLabel(i), Value: selectLabel(i)}
	}
	val := selectLabel(0)
	return func(c *ui.Context) {
		if s.step(c) {
			val = selectLabel(s.f % selectCount)
			defer report(1)
		}
		pageCol(c, func() { ui.Select(c, &val, opts).Width(selectWidth).Open() })
	}
}

// tooltipPage: a button with a one-line popover shown and hidden every
// 30th frame. Tango's Tooltip opens only on hover or focus, so its
// nearest component opened from code is a Popover with a manual trigger.
func tooltipPage() func(*ui.Context) {
	var s stepper
	open := false
	return func(c *ui.Context) {
		work := 0
		adv := s.step(c)
		if adv && s.f%tooltipEvery == 0 {
			open, work = !open, 1
		}
		pageCol(c, func() {
			ui.Popover(c).Text(tooltipText).Trigger("manual").Open(&open).Placement("bottom").Body(func() {
				ui.Button(c, tooltipAnchor)
			})
		})
		if adv {
			report(work)
		}
	}
}

// tableColumns are the table scenes' columns over upd, the rows' Updated
// values.
func tableColumns(upd []int) []ui.TableColumn {
	cols := make([]ui.TableColumn, tableCols)
	for col := range cols {
		cols[col] = ui.TableColumn{Key: strconv.Itoa(col), Title: tableTitles[col], Text: func(r int) string {
			if col == tableCols-1 {
				return strconv.Itoa(upd[r])
			}
			return tableCell(r, col)
		}}
	}
	return cols
}

// tangoRowH is antd's default (middle) table row, 55 px (measured with
// ui.NewDriver; scene_nearest_test.go checks the view holds tableShown).
const tangoRowH = 55

// tablePage: a virtual table of 1000 rows and 6 columns, its body
// tableShown rows high (ScrollY) at the default row height; frame f
// writes f into row f%8's Updated cell. No scrolling: Tango's Table has
// no scroll API (R36).
func tablePage() func(*ui.Context) {
	var s stepper
	upd := make([]int, tableRows)
	cols := tableColumns(upd)
	return func(c *ui.Context) {
		if s.step(c) {
			upd[tableRow(s.f)] = s.f
			defer report(1)
		}
		pageCol(c, func() { ui.Table(c, cols, tableRows).Virtual().ScrollY(tableShown * tangoRowH).Show() })
	}
}

// paginationPage: the 10 items of a page over a pager of 50 pages; frame
// f shows page 1+f%50.
func paginationPage() func(*ui.Context) {
	var s stepper
	page := 1
	return func(c *ui.Context) {
		if s.step(c) {
			page = pageOf(s.f)
			defer report(1)
		}
		pageCol(c, func() {
			for i := 0; i < pageSize; i++ {
				ui.Text(c, pageItem(page, i))
			}
			ui.Pagination(c, &page, pageTotal, pageSize).Show()
		})
	}
}

// cascaderPage: an open cascader, 3 levels of 5 (antd's 180 px columns
// show 5 without scrolling); frame f chooses the path
// cascadePath(f).
func cascaderPage() func(*ui.Context) {
	var s stepper
	var build func(above []int) []ui.CascaderOption
	build = func(above []int) []ui.CascaderOption {
		opts := make([]ui.CascaderOption, cascadeFan)
		for i := range opts {
			l := cascadeLabel(above, i)
			opts[i] = ui.CascaderOption{Label: l, Value: l}
			if len(above) < 2 {
				opts[i].Children = build(append(append([]int(nil), above...), i))
			} else {
				opts[i].IsLeaf = true
			}
		}
		return opts
	}
	opts := build(nil)
	path := func(p [3]int) []string {
		return []string{cascadeLabel(nil, p[0]), cascadeLabel(p[:1], p[1]), cascadeLabel(p[:2], p[2])}
	}
	val := path(cascadePath(0))
	return func(c *ui.Context) {
		if s.step(c) {
			val = path(cascadePath(s.f))
			defer report(1)
		}
		pageCol(c, func() { ui.Cascader(c, &val, opts).Width(3 * cascadeWidth).Expand(val...).Open() })
	}
}

// transferPage: a transfer of 10 items; frame f moves item f%10 to the
// other side.
func transferPage() func(*ui.Context) {
	var s stepper
	items := make([]ui.TransferItem, transferCount)
	right := make([]bool, transferCount)
	for i := range items {
		items[i] = ui.TransferItem{Key: transferLabel(i), Title: transferLabel(i)}
		right[i] = transferStart(i)
	}
	var target []string
	return func(c *ui.Context) {
		adv := s.step(c)
		if adv {
			i := s.f % transferCount
			right[i] = !right[i]
		}
		target = target[:0]
		for i, r := range right {
			if r {
				target = append(target, items[i].Key)
			}
		}
		pageCol(c, func() { ui.Transfer(c, items, &target).Titles(transferLeft, transferRight).ListSize(300, 400) })
		if adv {
			report(1)
		}
	}
}

// treeselectPage: an open tree select of 2 groups of 3 leaves, all
// expanded (8 rows, inside antd's 256 px dropdown: its ListHeight applies
// to the virtual dropdown only); frame f chooses leaf treeSelPick(f).
func treeselectPage() func(*ui.Context) {
	var s stepper
	nodes := make([]ui.TreeSelectNode, treeSelGroups)
	for g := range nodes {
		kids := make([]ui.TreeSelectNode, treeSelLeaves)
		for j := range kids {
			kids[j] = ui.TreeSelectNode{Label: treeSelLeaf(g, j), Value: treeSelLeaf(g, j), IsLeaf: true}
		}
		nodes[g] = ui.TreeSelectNode{Label: treeSelGroup(g), Value: treeSelGroup(g), Children: kids}
	}
	val := treeSelLeaf(0, 0)
	return func(c *ui.Context) {
		if s.step(c) {
			val = treeSelLeaf(treeSelPick(s.f))
			defer report(1)
		}
		pageCol(c, func() {
			ui.TreeSelect(c, &val, nodes).Width(300).DefaultExpandAll().Open()
		})
	}
}

// calendarPage: antd's card calendar (Calendar.Card, antd
// fullscreen=false), the counterpart of the opponents' date-panel
// calendars; frame f selects pickDay(f).
func calendarPage() func(*ui.Context) {
	var s stepper
	day := pickDay(0)
	return func(c *ui.Context) {
		if s.step(c) {
			day = pickDay(s.f)
			defer report(1)
		}
		pageCol(c, func() { ui.Calendar(c, &day).Card() })
	}
}

// stepsPage: 5 steps; frame f makes step f%5 the current one.
func stepsPage() func(*ui.Context) {
	var s stepper
	items := make([]ui.StepItem, stepCount)
	for i := range items {
		items[i] = ui.StepItem{Title: stepTitle(i), Description: stepDesc(i)}
	}
	cur := 0
	return func(c *ui.Context) {
		if s.step(c) {
			cur = s.f % stepCount
			defer report(1)
		}
		pageCol(c, func() { ui.Steps(c, items).Current(cur).Show() })
	}
}

// timelinePage: 8 events, each its text over its time; frame f rewrites
// event f%8.
func timelinePage() func(*ui.Context) {
	var s stepper
	changed := make([]int, timelineCount)
	return func(c *ui.Context) {
		if s.step(c) {
			changed[s.f%timelineCount] = s.f
			defer report(1)
		}
		items := make([]ui.TimelineItem, timelineCount)
		for i := range items {
			items[i] = ui.TimelineItem{Body: func() {
				ui.Text(c, timelineText(i, changed[i]))
				ui.Text(c, timelineTime(i)).Color(c.Theme.TextSecondary)
			}}
		}
		pageCol(c, func() { ui.Timeline(c, items).Show() })
	}
}

// descriptionsPage: 12 label/value items in 3 bordered columns; frame f
// rewrites value f%12.
func descriptionsPage() func(*ui.Context) {
	var s stepper
	changed := make([]int, descCount)
	return func(c *ui.Context) {
		if s.step(c) {
			changed[s.f%descCount] = s.f
			defer report(1)
		}
		items := make([]ui.DescItem, descCount)
		for i := range items {
			items[i] = ui.DescItem{Label: descLabel(i), Value: descValue(i, changed[i])}
		}
		pageCol(c, func() { ui.Descriptions(c, items).Bordered().Column(descCols) })
	}
}

// formPage: a form of 10 labelled inputs in two columns of 5 and a
// submit button; frame f types into field f%10.
func formPage() func(*ui.Context) {
	var s stepper
	vals := make([]string, formFields)
	for i := range vals {
		vals[i] = inputSeed
	}
	return func(c *ui.Context) {
		if s.step(c) {
			i := s.f % formFields
			vals[i] = typeInto(vals[i], s.f)
			defer report(1)
		}
		pageCol(c, func() {
			ui.Row(c, func() {
				for col := 0; col < 2; col++ {
					ui.Column(c, func() {
						f := ui.Form(c)
						for r := 0; r < formFields/2; r++ {
							i := col*formFields/2 + r
							f.Item(formLabel(i)).Body(func() { ui.Input(c, &vals[i]) })
						}
					}, layout.Grow(1), layout.Basis(0))
				}
			}, layout.Gap(pagePad))
			ui.Button(c, formSubmit).Primary()
		})
	}
}

// uploadPage: an upload button over 8 files uploading; frame f moves file
// f%8's progress one percent.
func uploadPage() func(*ui.Context) {
	var s stepper
	files := make([]ui.UploadFile, uploadCount)
	for i := range files {
		files[i] = ui.UploadFile{Name: uploadName(i), Status: "uploading", Percent: uploadStart(i)}
	}
	return func(c *ui.Context) {
		if s.step(c) {
			f := &files[s.f%uploadCount]
			f.Percent = nextPercent(f.Percent)
			defer report(1)
		}
		pageCol(c, func() { ui.Upload(c, &files).ButtonText(uploadButton).Width(520) })
	}
}

// slideBg is the carousel slides' background in all three programs.
const slideBg = "#dfe7f2"

// carouselPage: 5 slides 240 px high; every 30th frame the next slide.
func carouselPage() func(*ui.Context) {
	var s stepper
	cur := 0
	return func(c *ui.Context) {
		work := 0
		adv := s.step(c)
		if adv && s.f%carouselEvery == 0 {
			cur, work = (cur+1)%carouselCount, 1
		}
		pageCol(c, func() {
			ui.Carousel(c, carouselCount, func(i int) {
				ui.Column(c, func() { ui.Text(c, slideLabel(i)).Size(24) }, layout.Grow(1), layout.Center()).Bg(theme.Hex(slideBg))
			}).Current(&cur).Height(carouselHeight).Show()
		})
		if adv {
			report(work)
		}
	}
}

// collapsePage: an accordion of 6 panels of 3 lines; every 30th frame the
// next panel opens (and the open one closes).
func collapsePage() func(*ui.Context) {
	var s stepper
	open := 0
	active := []string{"0"}
	return func(c *ui.Context) {
		work := 0
		adv := s.step(c)
		if adv && s.f%collapseEvery == 0 {
			open, work = (open+1)%collapseCount, 1
		}
		active = append(active[:0], strconv.Itoa(open))
		items := make([]ui.CollapseItem, collapseCount)
		for i := range items {
			items[i] = ui.CollapseItem{Key: strconv.Itoa(i), Label: collapseLabel(i), Body: func() {
				for l := 0; l < collapseLines; l++ {
					ui.Text(c, collapseLine(i, l))
				}
			}}
		}
		pageCol(c, func() { ui.Collapse(c, &active, items).Accordion().Show() })
		if adv {
			report(work)
		}
	}
}

// drawerPage: a 360 px drawer at the right with 3 inputs and 2 buttons,
// opened and closed every 30th frame.
func drawerPage() func(*ui.Context) {
	var s stepper
	open := false
	fields := dialogFields
	return func(c *ui.Context) {
		work := 0
		adv := s.step(c)
		if adv && s.f%overlayEvery == 0 {
			open, work = !open, 1
		}
		pageCol(c, func() { ui.Text(c, drawerBackdrop) })
		// Footer before Body: Body builds the drawer.
		ui.Drawer(c, &open, drawerTitle).Size(drawerWidth).Footer(func() {
			ui.Row(c, func() {
				ui.Button(c, dialogCancel)
				ui.Button(c, dialogOK).Primary()
			}, layout.Gap(pageGap))
		}).Body(func() {
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

// tourPage: 3 buttons and a tour of 3 steps pointing at them over a mask;
// every 30th frame the next step.
func tourPage() func(*ui.Context) {
	var s stepper
	open, cur := true, 0
	return func(c *ui.Context) {
		work := 0
		adv := s.step(c)
		if adv && s.f%tourEvery == 0 {
			cur, work = (cur+1)%tourCount, 1
		}
		var ids [tourCount]ui.ID
		pageCol(c, func() {
			ui.Row(c, func() {
				for i := range ids {
					ids[i] = ui.Button(c, tourButtons[i]).ID()
				}
			}, layout.Gap(pageGap))
		})
		steps := make([]ui.TourStep, tourCount)
		for i := range steps {
			steps[i] = ui.TourStep{Target: ids[i], Title: tourTitles[i], Description: tourDesc(i)}
		}
		ui.Tour(c, &open, &cur, steps).Show()
		if adv {
			report(work)
		}
	}
}

// protablePage: ProTable without its toolbar options and pager
// (NoOptions, NoPagination), a query form of one field (Name) over 6 rows
// of the table scene's columns, all in view at antd's 55 px rows; frame f
// writes f into row f%6's Updated cell.
func protablePage() func(*ui.Context) {
	var s stepper
	upd := make([]int, proRows)
	rows := make([]int, proRows)
	for i := range rows {
		rows[i] = i
	}
	cols := make([]ui.ProColumn[int], tableCols)
	for col := range cols {
		cols[col] = ui.ProColumn[int]{Key: strconv.Itoa(col), Title: tableTitles[col],
			HideInSearch: col != 1, // the query form: Name alone
			Value: func(r int) any {
				if col == tableCols-1 {
					return strconv.Itoa(upd[r])
				}
				return tableCell(r, col)
			}}
	}
	return func(c *ui.Context) {
		if s.step(c) {
			upd[proRow(s.f)] = s.f
			defer report(1)
		}
		pageCol(c, func() {
			ui.ProTable(c, cols).NoOptions().NoPagination().Request(func(_ ui.ProRequest, done func(ui.ProResult[int])) {
				done(ui.ProResult[int]{Rows: rows, Total: proRows})
			}).Show()
		})
	}
}

// segmentedPage: 10 segmented controls of 4 choices; frame f moves
// control f%10 to its next choice.
func segmentedPage() func(*ui.Context) {
	var s stepper
	opts := make([]ui.Option, segChoices)
	for i := range opts {
		opts[i] = ui.Option{Label: segLabels[i], Value: segLabels[i]}
	}
	chosen := make([]int, segCount)
	vals := make([]string, segCount)
	for i := range chosen {
		chosen[i] = segStart(i)
	}
	return func(c *ui.Context) {
		if s.step(c) {
			i := s.f % segCount
			chosen[i] = (chosen[i] + 1) % segChoices
			defer report(1)
		}
		pageCol(c, func() {
			for i := range vals {
				vals[i] = segLabels[chosen[i]]
				ui.Segmented(c, &vals[i], opts)
			}
		})
	}
}

// ratePage: 20 five-star ratings in two columns; frame f moves rating
// f%20 one star on (past 5 back to 0).
func ratePage() func(*ui.Context) {
	var s stepper
	vals := make([]float64, rateCount)
	for i := range vals {
		vals[i] = float64(rateStart(i))
	}
	return func(c *ui.Context) {
		if s.step(c) {
			i := s.f % rateCount
			vals[i] = float64((int(vals[i]) + 1) % (rateMax + 1))
			defer report(1)
		}
		pageCol(c, func() {
			shareRows(c, rateCount, rateCols, pagePad, func(i int) { ui.Rate(c, &vals[i]) })
		})
	}
}

// tagsPage: 60 tags in rows of 6; frame f relabels tag f%60.
func tagsPage() func(*ui.Context) {
	var s stepper
	labels := make([]string, tagCount)
	for i := range labels {
		labels[i] = tagLabel(i)
	}
	return func(c *ui.Context) {
		if s.step(c) {
			i := s.f % tagCount
			labels[i] = taggedLabel(i, s.f)
			defer report(1)
		}
		pageCol(c, func() {
			for r := 0; r < tagCount/tagCols; r++ {
				ui.Row(c, func() {
					for col := 0; col < tagCols; col++ {
						ui.Tag(c, labels[r*tagCols+col])
					}
				}, layout.Gap(pageGap))
			}
		})
	}
}

// badgeBox is the square each count badge sits on, in all three programs.
const badgeBox = "#d9dde3"

// badgesPage: 40 squares with a count badge, rows of 10; frame f adds one
// to badge f%40 (1..99).
func badgesPage() func(*ui.Context) {
	var s stepper
	counts := make([]int, badgeCount)
	for i := range counts {
		counts[i] = badgeStart(i)
	}
	return func(c *ui.Context) {
		if s.step(c) {
			i := s.f % badgeCount
			counts[i] = nextCount(counts[i])
			defer report(1)
		}
		ui.Column(c, func() {
			for r := 0; r < badgeCount/badgeCols; r++ {
				ui.Row(c, func() {
					for col := 0; col < badgeCols; col++ {
						ui.Badge(c).Count(counts[r*badgeCols+col]).Body(func() {
							ui.Div(c, layout.Size(badgeSize, badgeSize)).Bg(theme.Hex(badgeBox)).Radius(6)
						})
					}
				}, layout.Gap(2*pagePad))
			}
		}, layout.Fill(), layout.Gap(2*pagePad), layout.Pad(2*pagePad))
	}
}

// cardsPage: 12 cards in rows of 4, each a title over 2 lines; frame f
// rewrites the second line of card f%12.
func cardsPage() func(*ui.Context) {
	var s stepper
	updated := make([]int, cardCount)
	return func(c *ui.Context) {
		if s.step(c) {
			updated[s.f%cardCount] = s.f
			defer report(1)
		}
		pageCol(c, func() {
			shareRows(c, cardCount, cardCols, pageGap, func(i int) {
				ui.Card(c, cardTitle(i)).Body(func() {
					ui.Text(c, cardLine(i))
					ui.Text(c, cardUpdated(updated[i]))
				})
			})
		})
	}
}

// statisticsPage: 12 statistics in rows of 4; frame f changes the value of
// statistic f%12.
func statisticsPage() func(*ui.Context) {
	var s stepper
	vals := make([]float64, statCount)
	for i := range vals {
		vals[i] = statValue(i, 0)
	}
	return func(c *ui.Context) {
		if s.step(c) {
			i := s.f % statCount
			vals[i] = statValue(i, s.f)
			defer report(1)
		}
		pageCol(c, func() {
			shareRows(c, statCount, statCols, pagePad, func(i int) { ui.Statistic(c, statTitle(i), vals[i], 2) })
		})
	}
}
