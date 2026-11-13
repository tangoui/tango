package main

import (
	"time"

	"github.com/tangoui/tango/examples/ui-gallery/gallery"
	"github.com/tangoui/tango/ui"
	"github.com/tangoui/tango/ui/layout"
	"github.com/tangoui/tango/ui/theme"
)

// Locale pages: the built-in texts of the components under en-US, ja-JP
// and zh-TW (the zh-CN look is every other page of the gallery). Sample
// data is ASCII so only the component texts change between languages.

type localeDemoState struct {
	date   time.Time
	cal    time.Time
	rng    [2]time.Time
	page   int
	size   int
	files  []ui.UploadFile
	cards  []ui.UploadFile
	target []string
	color  theme.Color
	modal  bool
}

var localeDemo = map[string]*localeDemoState{}

func localeState(code string) *localeDemoState {
	s := localeDemo[code]
	if s == nil {
		d := time.Date(2026, 11, 12, 0, 0, 0, 0, time.UTC)
		s = &localeDemoState{date: d, cal: d, page: 2, size: 10, target: []string{"k3"}, color: theme.Hex("#1677ff"), modal: true,
			files: []ui.UploadFile{{Name: "report.pdf", Status: "done"}}}
		localeDemo[code] = s
	}
	return s
}

func localeDatePage(l *ui.Locale) func(c *ui.Context) {
	return func(c *ui.Context) {
		s := localeState(l.Code)
		ui.ConfigProvider(c).Locale(l).Body(func() {
			ui.Row(c, func() {
				ui.Column(c, func() {
					ui.Text(c, l.Name+" · DatePicker").Strong()
					ui.DatePicker(c, &s.date).Open()
				}, layout.Gap(8), layout.W(300))
				ui.Column(c, func() {
					ui.Text(c, "Calendar").Strong()
					ui.Calendar(c, &s.cal).Card().Layout(layout.W(320))
				}, layout.Gap(8))
				ui.Column(c, func() {
					ui.Text(c, "RangePicker / TimeSelect / ColorPicker").Strong()
					ui.RangePicker(c, &s.rng)
					var ts string
					ui.TimeSelect(c, &ts)
					ui.ColorPicker(c, &s.color).ShowText()
				}, layout.Gap(8))
			}, layout.Gap(24), layout.Pad(24))
		})
	}
}

func localeDataPage(l *ui.Locale) func(c *ui.Context) {
	return func(c *ui.Context) {
		s := localeState(l.Code)
		ui.ConfigProvider(c).Locale(l).Body(func() {
			ui.Column(c, func() {
				ui.Text(c, l.Name+" · Pagination / Upload / Transfer / Table / Empty").Strong()
				ui.Pagination(c, &s.page, 85, s.size).ShowTotal().SizeChanger(&s.size).Show()
				ui.Row(c, func() {
					ui.Upload(c, &s.files).Width(260)
					ui.Upload(c, &s.cards).PictureCard()
					ui.Upload(c, &s.cards).Dragger().Width(360)
				}, layout.Gap(24), layout.AlignItems(layout.AlignStart))
				items := []ui.TransferItem{{Key: "k1", Title: "Alpha"}, {Key: "k2", Title: "Beta"}, {Key: "k3", Title: "Gamma"}}
				ui.Transfer(c, items, &s.target).Searchable()
				filters := map[string][]string{}
				cols := []ui.TableColumn{
					{Key: "name", Title: "Name", Sortable: true, Text: func(int) string { return "" }},
					{Key: "city", Title: "City", Filters: ui.Opts("Paris", "Tokyo"), Text: func(int) string { return "" }},
				}
				ui.Column(c, func() { ui.Table(c, cols, 0).Filters(&filters).Show() }, layout.W(560))
				ui.Row(c, func() {
					ui.Empty(c)
					ui.Empty(c).Simple()
				}, layout.Gap(48))
			}, layout.Gap(20), layout.Pad(24))
		})
	}
}

func localeModalPage(l *ui.Locale) func(c *ui.Context) {
	return func(c *ui.Context) {
		s := localeState(l.Code)
		ui.ConfigProvider(c).Locale(l).Body(func() {
			ui.Column(c, func() {
				ui.Text(c, l.Name+" · Modal").Strong()
				ui.Modal(c, &s.modal, "Modal").Body(func() {
					ui.Text(c, "Modal content (sample text).")
				})
			}, layout.Pad(24))
		})
	}
}

// localeGroup is the group of the locale pages. It must not be General
// or Form: overview stacks every page of those two groups into one tall
// screenshot, and the Modal page's open dialog (with its full-window mask,
// 80% black under shadcn) and the DatePicker page's open popup would cover
// the whole overview. Home still lists the pages under ConfigProvider
// through Parent.
const localeGroup = gallery.Feedback

func init() {
	for i, l := range []*ui.Locale{ui.EnUS, ui.Ja, ui.ZhTW} {
		name := "ConfigProvider-" + l.Code
		gallery.Register(gallery.Page{Name: name + "-Date", Title: "Locale " + l.Code + ": DatePicker", Group: localeGroup,
			Order: 91 + i, Parent: "ConfigProvider", Height: 470, Width: 1100, Render: localeDatePage(l)})
		gallery.Register(gallery.Page{Name: name + "-Data", Title: "Locale " + l.Code + ": Data", Group: localeGroup,
			Order: 91 + i, Parent: "ConfigProvider", Width: 1000, Render: localeDataPage(l)})
		gallery.Register(gallery.Page{Name: name + "-Modal", Title: "Locale " + l.Code + ": Modal", Group: localeGroup,
			Order: 91 + i, Parent: "ConfigProvider", Height: 360, Render: localeModalPage(l)})
	}
}
