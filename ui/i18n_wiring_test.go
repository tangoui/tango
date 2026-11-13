package ui

import (
	"image"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/tangoui/tango/ui/a11y"
	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/layout"
	"github.com/tangoui/tango/ui/theme"
)

// The wiring tests render every component that has built-in texts under
// the four built-in locales and check that the active language's texts are
// present and that no other language's texts leak through.

var i18nLocales = []*Locale{ZhCN, EnUS, ZhTW, Ja}

type i18nIDs map[string]ID

type i18nCase struct {
	name  string
	el    bool                                                  // Element Plus flavor
	page  func(c *Context, ids i18nIDs)                         // built inside ConfigProvider(c).Locale(l)
	setup func(d *Driver, ids i18nIDs)                          // opens popups etc.
	texts func(l *Locale) []string                              // must all be node labels
	check func(t *testing.T, l *Locale, d *Driver, ids i18nIDs) // optional extra checks
}

func i18nClick(d *Driver, ids i18nIDs, key string) {
	if r, ok := d.r.c.rects[ids[key]]; ok {
		d.Click(r.Center())
	}
}

// i18nLabels collects the labels of every laid-out node (page and overlays).
func i18nLabels(d *Driver) map[string]bool {
	c := d.r.c
	out := map[string]bool{}
	var walk func(n *node)
	walk = func(n *node) {
		if n.label != "" && !n.abs.Empty() {
			out[n.label] = true
		}
		for _, k := range n.kids {
			walk(k)
		}
	}
	if c.root != nil {
		walk(c.root)
	}
	for _, ov := range c.overlays {
		if ov.root != nil {
			walk(ov.root)
		}
	}
	return out
}

var i18nDate = time.Date(2026, 1, 15, 0, 0, 0, 0, time.UTC)

func i18nWeekdays(l *Locale) []string { return l.WeekdayNames() }

func i18nCases() []i18nCase {
	dateOpen := func(d *Driver, ids i18nIDs) { i18nClick(d, ids, "w") }
	datePanel := func(l *Locale) []string {
		return append([]string{l.PrevYear, l.PrevMonth, l.NextMonth, l.NextYear, l.YearMonth(2026, 1)}, i18nWeekdays(l)...)
	}
	img := image.NewRGBA(image.Rect(0, 0, 40, 30))
	var cases []i18nCase
	for _, el := range []bool{false, true} {
		el := el
		cases = append(cases,
			i18nCase{name: "DatePicker", el: el,
				page: func(c *Context, ids i18nIDs) {
					v := i18nDate
					ids["w"] = DatePicker(c, &v).n.id
				},
				setup: dateOpen,
				texts: func(l *Locale) []string {
					if el {
						return datePanel(l)
					}
					return append(datePanel(l), l.Today)
				}},
			i18nCase{name: "RangePicker", el: el,
				page: func(c *Context, ids i18nIDs) {
					v := [2]time.Time{i18nDate, i18nDate.AddDate(0, 0, 3)}
					ids["w"] = RangePicker(c, &v).n.id
				},
				setup: dateOpen,
				texts: func(l *Locale) []string {
					return append([]string{l.PrevYear, l.NextYear, l.YearMonth(2026, 1), l.YearMonth(2026, 2)}, i18nWeekdays(l)...)
				}},
			i18nCase{name: "DateTimePicker", el: el,
				page: func(c *Context, ids i18nIDs) {
					v := i18nDate
					ids["w"] = DateTimePicker(c, &v).f.n.id
				},
				setup: dateOpen,
				texts: func(l *Locale) []string { return append(datePanel(l), l.Now, l.OK) }},
			i18nCase{name: "TimePicker", el: el,
				page: func(c *Context, ids i18nIDs) {
					v := i18nDate.Add(9 * time.Hour)
					ids["w"] = TimePicker(c, &v).f.n.id
				},
				setup: dateOpen,
				texts: func(l *Locale) []string {
					if el {
						return []string{l.Cancel, l.OK}
					}
					return []string{l.Now, l.OK}
				}},
			i18nCase{name: "Calendar", el: el,
				page: func(c *Context, ids i18nIDs) {
					v := i18nDate
					Calendar(c, &v)
				},
				texts: func(l *Locale) []string {
					if el {
						return append([]string{l.YearMonth(2026, 1), l.PrevMonth, l.Today, l.NextMonth}, i18nWeekdays(l)...)
					}
					return append([]string{l.Calendar.Month, l.Calendar.Year}, i18nWeekdays(l)...)
				}},
			i18nCase{name: "Table", el: el,
				page: func(c *Context, ids i18nIDs) {
					filters := map[string][]string{}
					cols := []TableColumn{{Key: "city", Title: "City", Filters: Opts("A", "B"),
						OnFilter: func(int, string) bool { return true }, Text: func(int) string { return "" }}}
					Table(c, cols, 0).Filters(&filters).Show()
				},
				setup: func(d *Driver, _ i18nIDs) { d.ClickLabel("city") },
				texts: func(l *Locale) []string {
					if el {
						return []string{l.NoData, l.Table.Filter, l.FilterReset}
					}
					return []string{l.NoData, l.FilterConfirm, l.FilterReset}
				}},
			i18nCase{name: "MessageBox", el: el,
				page: func(c *Context, ids i18nIDs) {
					if Button(c, "open").Clicked() {
						MessageBox(c, ConfirmOptions{Title: "T", Content: "C"})
					}
				},
				setup: func(d *Driver, _ i18nIDs) { d.ClickLabel("open") },
				texts: func(l *Locale) []string {
					if el {
						return []string{l.OK, l.Close}
					}
					return []string{l.GotIt}
				}},
			i18nCase{name: "Upload", el: el,
				page: func(c *Context, ids i18nIDs) {
					var files, cards []UploadFile
					Upload(c, &files)
					Upload(c, &cards).PictureCard()
				},
				texts: func(l *Locale) []string { return []string{l.Upload.Button, l.Upload.Trigger} }},
			i18nCase{name: "Transfer", el: el,
				page: func(c *Context, ids i18nIDs) {
					target := []string{}
					Transfer(c, []TransferItem{{Key: "a", Title: "a"}, {Key: "b", Title: "b"}}, &target)
				},
				texts: func(l *Locale) []string {
					if el {
						return []string{l.Transfer.Source, l.Transfer.Target, l.NoData}
					}
					return []string{l.Transfer.Source, l.Transfer.Target, l.NoData, l.Items(2), l.Items(0)}
				}},
			i18nCase{name: "ColorPicker", el: el,
				page: func(c *Context, ids i18nIDs) {
					v := theme.Hex("#1677ff")
					ids["w"] = ColorPicker(c, &v).AllowClear().n.id
				},
				setup: dateOpen,
				texts: func(l *Locale) []string {
					if el {
						return []string{l.ColorPicker.Saturation, l.ColorPicker.Hue, l.ColorPicker.Alpha, l.ColorPicker.Clear, l.OK}
					}
					return []string{l.ColorPicker.Saturation, l.ColorPicker.Hue, l.ColorPicker.Alpha, l.ColorPicker.ClearColor}
				}},
		)
	}
	cases = append(cases,
		i18nCase{name: "Pagination",
			page: func(c *Context, ids i18nIDs) {
				p := 1
				Pagination(c, &p, 85, 10).ShowTotal().Show()
			},
			texts: func(l *Locale) []string { return []string{l.Total(85), l.PrevPage, l.NextPage} }},
		i18nCase{name: "Modal",
			page: func(c *Context, ids i18nIDs) {
				open := true
				Modal(c, &open, "T").Body(func() { Text(c, "body") })
			},
			texts: func(l *Locale) []string { return []string{l.OK, l.Cancel} }},
		i18nCase{name: "Popconfirm",
			page: func(c *Context, ids i18nIDs) {
				Popconfirm(c, "T", "", func() { Button(c, "open") })
			},
			setup: func(d *Driver, _ i18nIDs) { d.ClickLabel("open") },
			texts: func(l *Locale) []string { return []string{l.OK, l.Cancel} }},
		i18nCase{name: "Alert",
			page:  func(c *Context, ids i18nIDs) { Alert(c, "info", "T").Closable().Show() },
			texts: func(l *Locale) []string { return []string{l.Close} }},
		i18nCase{name: "Confirm",
			page: func(c *Context, ids i18nIDs) {
				if Button(c, "open").Clicked() {
					Confirm(c, ConfirmOptions{Title: "T", Content: "C"})
				}
			},
			setup: func(d *Driver, _ i18nIDs) { d.ClickLabel("open") },
			texts: func(l *Locale) []string { return []string{l.OK, l.Cancel} }},
		i18nCase{name: "Empty",
			page: func(c *Context, ids i18nIDs) {
				Empty(c)
				List(c, 0).Item(func(int) {})
			},
			texts: func(l *Locale) []string { return []string{l.NoData} }},
		i18nCase{name: "Select",
			page: func(c *Context, ids i18nIDs) {
				var v string
				ids["w"] = Select(c, &v, nil).n.id
			},
			setup: dateOpen,
			texts: func(l *Locale) []string { return []string{l.NoData} }},
		i18nCase{name: "MultiSelect",
			page: func(c *Context, ids i18nIDs) {
				var v []string
				ids["w"] = MultiSelect(c, &v, nil).box.n.id
			},
			setup: dateOpen,
			texts: func(l *Locale) []string { return []string{l.NoData} }},
		i18nCase{name: "TreeSelect",
			page: func(c *Context, ids i18nIDs) {
				var v string
				ids["w"] = TreeSelect(c, &v, nil).n.id
			},
			setup: dateOpen,
			texts: func(l *Locale) []string { return []string{l.NoData} }},
		i18nCase{name: "CheckAll",
			page: func(c *Context, ids i18nIDs) {
				sel := []string{}
				CheckAll(c, &sel, Opts("a", "b"))
			},
			texts: func(l *Locale) []string { return []string{l.SelectAll} }},
		i18nCase{name: "Search",
			page: func(c *Context, ids i18nIDs) {
				var v string
				Search(c, &v).EnterButton("")
			},
			texts: func(l *Locale) []string { return []string{l.Search} }},
		i18nCase{name: "Tour",
			page: func(c *Context, ids i18nIDs) {
				open, cur := true, 0
				Tour(c, &open, &cur, []TourStep{{Title: "one"}, {Title: "two"}}).Show()
			},
			texts: func(l *Locale) []string { return []string{l.Tour.Close, l.Tour.Next} }},
		i18nCase{name: "Tour-last",
			page: func(c *Context, ids i18nIDs) {
				open, cur := true, 1
				Tour(c, &open, &cur, []TourStep{{Title: "one"}, {Title: "two"}}).Show()
			},
			texts: func(l *Locale) []string { return []string{l.Tour.Close, l.Tour.Prev, l.Tour.Finish} }},
		i18nCase{name: "Image",
			page: func(c *Context, ids i18nIDs) {
				ids["w"] = Image(c, img).Width(40).Height(30).Preview(true).n.id
			},
			setup: dateOpen,
			texts: func(l *Locale) []string {
				return []string{l.Image.PreviewLabel, l.Image.ClosePreview, l.Image.ZoomIn, l.Image.ZoomOut,
					l.Image.RotateLeft, l.Image.RotateRight, l.Image.FlipHorizontal, l.Image.FlipVertical}
			}},
		i18nCase{name: "Image-Element", el: true,
			page: func(c *Context, ids i18nIDs) {
				ids["w"] = Image(c, img).Width(40).Height(30).PreviewGroup([]image.Image{img, img}, 0).n.id
			},
			setup: dateOpen,
			texts: func(l *Locale) []string {
				return []string{l.Image.ClosePreview, l.Image.Prev, l.Image.Next, l.Image.ZoomIn, l.Image.ZoomOut, l.Reset,
					l.Image.RotateLeft, l.Image.RotateRight}
			}},
		i18nCase{name: "Carousel",
			page: func(c *Context, ids i18nIDs) {
				Carousel(c, 3, func(int) { Text(c, "slide") }).Arrows("always").Show()
			},
			texts: func(l *Locale) []string { return []string{l.Carousel.Prev, l.Carousel.Next} }},
		i18nCase{name: "ProTable",
			page: func(c *Context, ids i18nIDs) {
				type row struct{ Name string }
				ProTable(c, []ProColumn[row]{{Key: "name", Title: "Name", Value: func(r row) any { return r.Name }}}).Show()
			},
			texts: func(l *Locale) []string {
				return []string{l.ProTable.Reload, l.ProTable.Density, l.ProTable.ColumnSetting, l.Fullscreen,
					l.ProTable.Query, l.Reset, l.WithColon("Name")}
			}},
		i18nCase{name: "InfiniteScroll",
			page: func(c *Context, ids i18nIDs) {
				InfiniteScroll(c, func() { Text(c, "row") }, layout.H(100)).Done(true)
				InfiniteScroll(c, func() { Text(c, "row") }, layout.H(100)).Loading(true)
			},
			texts: func(l *Locale) []string { return []string{l.NoMore, l.Loading + "..."} }},
		i18nCase{name: "MarkdownEditor",
			page: func(c *Context, ids i18nIDs) {
				src := ""
				MarkdownEditor(c, &src)
			},
			texts: func(l *Locale) []string {
				m := l.MarkdownEditor
				return []string{m.Heading, m.Bold, m.Italic, m.Strike, m.Code, m.Link, m.Quote, m.BulletList,
					m.OrderedList, m.Task, m.CodeBlock, m.Table, m.Divider, m.Edit, m.Split, m.Preview}
			}},
		i18nCase{name: "Markdown",
			page: func(c *Context, ids i18nIDs) {
				Markdown(c, "```go\nx := 1\n```\n")
			},
			texts: func(l *Locale) []string { return []string{l.Markdown.CopyCode} }},
		i18nCase{name: "ProLayout",
			page: func(c *Context, ids i18nIDs) {
				collapsed := false
				ProLayout(c, ProLayoutConfig{Title: "App", Collapsed: &collapsed}, func() { Text(c, "content") })
			},
			texts: func(l *Locale) []string { return []string{l.CollapseMenu} }},
		i18nCase{name: "FormList",
			page: func(c *Context, ids i18nIDs) {
				rows := []string{"a", "b"}
				f := Form(c)
				f.Body(func() {
					FormList(f, "List", &rows).Body(func(row *FormListRow[string]) { Text(c, *row.Value) })
				})
			},
			texts: func(l *Locale) []string { return []string{l.Form.MoveUp, l.Form.MoveDown, l.Delete, l.Form.AddRow} }},
		i18nCase{name: "ProForm",
			page: func(c *Context, ids i18nIDs) {
				ProForm(c, []ProField{{Key: "a", Label: "A"}, {Key: "b", Label: "B", ValueType: ValueSelect}}).Show()
			},
			texts: func(l *Locale) []string { return []string{l.Form.Submit, l.Reset} }},
		i18nCase{name: "StepsForm",
			page: func(c *Context, ids i18nIDs) {
				StepsForm(c, []StepsFormStep{{Title: "S1", Fields: []ProField{{Key: "a", Label: "A"}}}, {Title: "S2"}}).Show()
			},
			texts: func(l *Locale) []string { return []string{l.Form.NextStep} }},
		i18nCase{name: "QueryFilter",
			page: func(c *Context, ids i18nIDs) {
				QueryFilter(c, []ProField{{Key: "a", Label: "A"}, {Key: "b", Label: "B"}, {Key: "c", Label: "C"}, {Key: "d", Label: "D"}}).Show()
			},
			texts: func(l *Locale) []string { return []string{l.Form.Query, l.Reset, l.Expand, l.WithColon("A")} }},
	)
	return append(cases, i18nProCases()...)
}

// i18nA11y checks that a node of role is named pick(l) and that no node of
// role carries another locale's (different) name.
func i18nA11y(t *testing.T, l *Locale, d *Driver, role a11y.Role, pick func(*Locale) string) {
	t.Helper()
	if d.AccessibilityTree() == nil {
		d.EnableAccessibility()
	}
	own := pick(l)
	if _, ok := d.A11yFind(role, own); !ok {
		t.Errorf("%s: no %s named %q", l.Code, role, own)
	}
	for _, o := range i18nLocales {
		if s := pick(o); o != l && s != own {
			if _, ok := d.A11yFind(role, s); ok {
				t.Errorf("%s: %s name %q leaked", l.Code, o.Code, s)
			}
		}
	}
}

// i18nWant checks got against pick(l) and against the other locales'
// texts (for values that are not node labels: placeholders, displays).
func i18nWant(t *testing.T, l *Locale, what, got string, pick func(*Locale) string) {
	t.Helper()
	if want := pick(l); got != want {
		t.Errorf("%s: %s = %q, want %q", l.Code, what, got, want)
	}
}

// i18nProCases cover the texts of the Pro / entry components added after
// the first locale round (Locale.DatePicker, TimePicker, Tabs, Pagination,
// Slider, DropZone, EditableTable, PageContainer, ProCard, StatisticCard,
// ProField, ProDescriptions, LightFilter, LoginForm and the fields added to
// Table and Upload).
func i18nProCases() []i18nCase {
	var (
		week, quarter *DatePickerW
		trp           *TimeRangePickerW
	)
	weekLabel := map[string]string{"zh-CN": "2026-第3周", "en-US": "2026-3rd", "zh-TW": "2026-第3週", "ja-JP": "2026-第3週"} // Jan 15 2026: ISO week 3
	type row struct{ ID, Name string }
	return []i18nCase{
		{name: "DatePicker-Week",
			page: func(c *Context, ids i18nIDs) {
				v := i18nDate
				var z time.Time
				week = DatePicker(c, &v).Picker(PickerWeek)
				quarter = DatePicker(c, &z).Picker(PickerQuarter)
			},
			texts: func(l *Locale) []string { return nil },
			check: func(t *testing.T, l *Locale, d *Driver, ids i18nIDs) {
				i18nWant(t, l, "week value", week.display(), func(o *Locale) string { return weekLabel[o.Code] })
				i18nWant(t, l, "quarter placeholder", quarter.ph(), func(o *Locale) string { return o.DatePicker.SelectQuarter })
				if got, want := p2aFormatPeriod(l, i18nDate.AddDate(0, 9, 0), PickerQuarter, "", l.WeekStart), "2026-Q4"; got != want {
					t.Errorf("%s: quarter value %q", l.Code, got)
				}
			}},
		{name: "DatePicker-Year",
			page: func(c *Context, ids i18nIDs) {
				v := i18nDate
				ids["w"] = DatePicker(c, &v).Picker(PickerYear).n.id
			},
			setup: func(d *Driver, ids i18nIDs) { i18nClick(d, ids, "w") },
			texts: func(l *Locale) []string { return []string{l.DatePicker.PrevDecade, l.DatePicker.NextDecade} }},
		{name: "TimePicker-12h",
			page: func(c *Context, ids i18nIDs) {
				v := i18nDate.Add(15 * time.Hour)
				ids["w"] = TimePicker(c, &v).Use12Hours().f.n.id
				var r [2]time.Time
				trp = TimeRangePicker(c, &r)
			},
			setup: func(d *Driver, ids i18nIDs) { i18nClick(d, ids, "w") },
			texts: func(l *Locale) []string { return nil },
			check: func(t *testing.T, l *Locale, d *Driver, ids i18nIDs) {
				i18nA11y(t, l, d, a11y.RoleOption, func(o *Locale) string { return o.TimePicker.AM })
				i18nA11y(t, l, d, a11y.RoleOption, func(o *Locale) string { return o.TimePicker.PM })
				i18nWant(t, l, "range start", trp.ph[0], func(o *Locale) string { return o.TimePicker.StartTime })
				i18nWant(t, l, "range end", trp.ph[1], func(o *Locale) string { return o.TimePicker.EndTime })
				i18nA11y(t, l, d, a11y.RoleListbox, func(o *Locale) string { return o.TimePicker.Meridiem })
			}},
		{name: "Tabs-Pagination-Slider",
			page: func(c *Context, ids i18nIDs) {
				active := "a"
				Tabs(c, &active, []TabItem{{Key: "a", Label: "A"}, {Key: "b", Label: "B"}}).OnAdd(func() {}).Show()
				p, sp := 1, 3
				Pagination(c, &p, 500, 10).ShowQuickJumper().Show()
				Pagination(c, &sp, 500, 10).Simple().Show()
				lo, hi := 20.0, 60.0
				SliderRange(c, &lo, &hi, 0, 100)
			},
			texts: func(l *Locale) []string { return []string{l.Tabs.Add} },
			check: func(t *testing.T, l *Locale, d *Driver, ids i18nIDs) {
				i18nA11y(t, l, d, a11y.RoleTextbox, func(o *Locale) string { return o.Pagination.Jump })
				i18nA11y(t, l, d, a11y.RoleTextbox, func(o *Locale) string { return o.Pagination.Current })
				i18nA11y(t, l, d, a11y.RoleSlider, func(o *Locale) string { return o.Slider.Min })
				i18nA11y(t, l, d, a11y.RoleSlider, func(o *Locale) string { return o.Slider.Max })
			}},
		{name: "ProCards",
			page: func(c *Context, ids i18nIDs) {
				PageContainer(c, "Page").Body(func() {
					ProCard(c, "Card").Tooltip("tip").Body(func() { Text(c, "x") })
					StatisticCard(c, "Up").Value(1, 0).Trend("up").Show()
					StatisticCard(c, "Down").Value(1, 0).Trend("down").Show()
				})
				FooterToolbar(c).Body(func() { Button(c, "Ok") })
			},
			texts: func(l *Locale) []string { return nil },
			check: func(t *testing.T, l *Locale, d *Driver, ids i18nIDs) {
				i18nA11y(t, l, d, a11y.RoleToolbar, func(o *Locale) string { return o.PageContainer.FooterToolbar })
				i18nA11y(t, l, d, a11y.RoleImage, func(o *Locale) string { return o.ProCard.MoreInfo })
				i18nA11y(t, l, d, a11y.RoleImage, func(o *Locale) string { return o.StatisticCard.Up })
				i18nA11y(t, l, d, a11y.RoleImage, func(o *Locale) string { return o.StatisticCard.Down })
			}},
		{name: "ProDescriptions",
			page: func(c *Context, ids i18nIDs) {
				vals := ProValues{"name": "abc", "on": true, "off": false}
				ProDescriptions(c, []ProDescItem{{Key: "name", Label: "Name"}, {Key: "on", Label: "S1", ValueType: ValueSwitch},
					{Key: "off", Label: "S2", ValueType: ValueSwitch}}).DataSource(&vals).
					Editable(func(string, any, ProValues, func(error)) {}).Show()
			},
			texts: func(l *Locale) []string {
				return []string{locFmt(l.ProDescriptions.EditFmt, "Name"), l.ProField.On, l.ProField.Off}
			}},
		{name: "ProField-Ranges",
			page: func(c *Context, ids i18nIDs) {
				ProForm(c, []ProField{{Key: "span", Label: "Span", ValueType: ValueTimeRange},
					{Key: "price", Label: "Price", ValueType: ValueDigitRange}}).Show()
			},
			texts: func(l *Locale) []string { return nil },
			check: func(t *testing.T, l *Locale, d *Driver, ids i18nIDs) {
				i18nA11y(t, l, d, a11y.RoleSpinButton, func(o *Locale) string { return locFmt(o.ProField.RangeEndFmt, "Price", o.ProField.Max) })
				i18nA11y(t, l, d, a11y.RoleCombobox, func(o *Locale) string { return locFmt(o.ProField.StartFmt, "Span") })
				i18nA11y(t, l, d, a11y.RoleCombobox, func(o *Locale) string { return locFmt(o.ProField.EndFmt, "Span") })
			}},
		{name: "LightFilter",
			page: func(c *Context, ids i18nIDs) {
				vals := ProValues{"a": "x"}
				LightFilter(c, []ProField{{Key: "a", Label: "A"}, {Key: "b", Label: "B"}, {Key: "c", Label: "C"}}).
					Values(&vals).Collapse(1).Show()
			},
			texts: func(l *Locale) []string { return []string{l.LightFilter.More} },
			check: func(t *testing.T, l *Locale, d *Driver, ids i18nIDs) {
				i18nA11y(t, l, d, a11y.RoleToolbar, func(o *Locale) string { return o.LightFilter.Name })
				i18nA11y(t, l, d, a11y.RoleButton, func(o *Locale) string { return locFmt(o.LightFilter.PillFmt, "A", "x") })
				i18nA11y(t, l, d, a11y.RoleButton, func(o *Locale) string { return locFmt(o.LightFilter.ClearFmt, "A") })
			}},
		{name: "LoginForm",
			page: func(c *Context, ids i18nIDs) {
				LoginForm(c).Title("App").OnForgot(func() {}).Actions(func() { Text(c, "wechat") }).Show()
			},
			texts: func(l *Locale) []string {
				f := l.LoginForm
				return []string{f.AccountTab, f.MobileTab, f.Remember, f.Forgot, f.Submit, f.Others}
			}},
		{name: "LoginForm-Mobile",
			page: func(c *Context, ids i18nIDs) {
				LoginForm(c).MobileOnly().Show()
			},
			texts: func(l *Locale) []string { return []string{l.LoginForm.GetCode, l.LoginForm.Submit} },
			check: func(t *testing.T, l *Locale, d *Driver, ids i18nIDs) {
				d.ClickLabel(l.LoginForm.Submit)
				d.Settle()
				for _, s := range []string{l.LoginForm.MobileRequired, l.LoginForm.CaptchaRequired} {
					if _, ok := d.FindText(s); !ok {
						t.Errorf("%s: %q not shown", l.Code, s)
					}
				}
			}},
		{name: "EditableProTable",
			page: func(c *Context, ids i18nIDs) {
				rows := []row{{"a", "Alice"}}
				EditableProTable(c, []ProColumn[row]{{Key: "name", Title: "Name", Value: func(r row) any { return r.Name },
					Set: func(r *row, v any) { r.Name, _ = v.(string) }}}, &rows).
					RowKey(func(r row) string { return r.ID }).Show()
			},
			texts: func(l *Locale) []string { return []string{l.EditableTable.Actions} }},
		{name: "Table-Selections",
			page: func(c *Context, ids i18nIDs) {
				cols := []TableColumn{{Key: "v", Title: "V", Text: func(int) string { return "row" }}}
				var sel []int
				Table(c, cols, 3).Selection(&sel).Selections().Show()
			},
			setup: func(d *Driver, _ i18nIDs) {
				for _, l := range i18nLocales {
					if d.ClickLabel(l.Table.Selections) == nil {
						return
					}
				}
			},
			texts: func(l *Locale) []string {
				return []string{l.Table.Selections, l.SelectionAll, l.SelectionInvert, l.Table.SelectNone}
			}},
		{name: "DropZone",
			page: func(c *Context, ids i18nIDs) {
				DropZone(c, func(bool) { Text(c, "area") })
			},
			texts: func(l *Locale) []string { return nil },
			check: func(t *testing.T, l *Locale, d *Driver, ids i18nIDs) {
				i18nA11y(t, l, d, a11y.RoleGroup, func(o *Locale) string { return o.DropZone.Name })
			}},
		{name: "Upload-Reject",
			page: func(c *Context, ids i18nIDs) {
				var files []UploadFile
				ids["w"] = Upload(c, &files).Dragger().Accept("image/*").n.id
			},
			texts: func(l *Locale) []string { return nil },
			check: func(t *testing.T, l *Locale, d *Driver, ids i18nIDs) {
				p := filepath.Join(t.TempDir(), "a.txt")
				if err := os.WriteFile(p, []byte("x"), 0o644); err != nil {
					t.Fatal(err)
				}
				d.DropFiles(d.r.c.rects[ids["w"]].Center(), p)
				d.Settle()
				for _, o := range i18nLocales {
					msg := locFmt(o.Upload.BadTypeFmt, "a.txt")
					if _, ok := d.FindText(msg); ok != (o == l) {
						t.Errorf("%s: %s rejection %q shown=%v", l.Code, o.Code, msg, ok)
					}
				}
			}},
	}
}

func i18nRender(tc i18nCase, l *Locale) (*Driver, i18nIDs) {
	ids := i18nIDs{}
	th := theme.AntDesign()
	if tc.el {
		th = theme.ElementPlus()
	}
	d := NewDriver(func(c *Context) {
		ConfigProvider(c).Locale(l).Body(func() {
			Column(c, func() { tc.page(c, ids) }, layout.Pad(20), layout.Gap(12))
		})
	}, DriverOptions{Width: 1200, Height: 900, Theme: th})
	d.Frame()
	if tc.setup != nil {
		tc.setup(d, ids)
		d.Frame()
	}
	return d, ids
}

func TestI18nWiring(t *testing.T) {
	for _, tc := range i18nCases() {
		name := tc.name
		if tc.el && !strings.Contains(name, "Element") {
			name += "-Element"
		}
		t.Run(name, func(t *testing.T) {
			for _, l := range i18nLocales {
				d, ids := i18nRender(tc, l)
				labels := i18nLabels(d)
				own := map[string]bool{}
				for _, s := range tc.texts(l) {
					own[s] = true
					if !labels[s] {
						t.Errorf("%s: text %q not rendered", l.Code, s)
					}
				}
				for _, o := range i18nLocales {
					if o == l {
						continue
					}
					for _, s := range tc.texts(o) {
						if !own[s] && labels[s] {
							t.Errorf("%s: %s text %q leaked", l.Code, o.Code, s)
						}
					}
				}
				if l == EnUS {
					for s := range labels {
						if strings.IndexFunc(s, isCJK) >= 0 {
							t.Errorf("en-US: CJK label %q", s)
						}
					}
				}
				if tc.check != nil {
					tc.check(t, l, d, ids)
				}
			}
		})
	}
}

// TestI18nPaintedTexts covers texts drawn directly by paint callbacks (not
// node labels): the widgets must capture the locale while building, since
// painting runs after the ConfigProvider scope has ended.
func TestI18nPaintedTexts(t *testing.T) {
	for _, l := range i18nLocales {
		var (
			up, drag, upEl *UploadW
			qr             *QRCodeW
			im             *ImageW
			cp             *ColorPickerW
			rp             *RangePickerW
			rpp            [4]*RangePickerW // week, month, quarter, year
			rpx            *RangePickerW    // explicit placeholders win
			dp             *DatePickerW
			ts             *TreeSelectW
			tsel           *TimeSelectW
			md             *MarkdownEditorW
			dt             *DateTimePickerW
			tp             *TimePickerW
		)
		page := func(c *Context) {
			ConfigProvider(c).Locale(l).Body(func() {
				Column(c, func() {
					var files []UploadFile
					up = Upload(c, &files)
					drag = Upload(c, &files).Dragger()
					qr = QRCode(c, "x").Status("expired")
					im = Image(c, nil).Width(40).Height(30)
					col := theme.Color{}
					cp = ColorPicker(c, &col).ShowText()
					var r [2]time.Time
					rp = RangePicker(c, &r)
					for i, p := range []PickerType{PickerWeek, PickerMonth, PickerQuarter, PickerYear} {
						rpp[i] = RangePicker(c, &r).Picker(p)
					}
					rpx = RangePicker(c, &r).Placeholder("a", "b").Picker(PickerMonth)
					var d0, d1, d2 time.Time
					dp = DatePicker(c, &d0)
					dt = DateTimePicker(c, &d1)
					tp = TimePicker(c, &d2)
					var s, ts0 string
					ts = TreeSelect(c, &s, nil)
					tsel = TimeSelect(c, &ts0)
					src := ""
					md = MarkdownEditor(c, &src)
				})
			})
		}
		d := NewDriver(page, DriverOptions{Width: 900, Height: 1600})
		d.Frame()
		checks := []struct {
			what, got, want string
		}{
			{"upload button", up.btnText, l.Upload.Button},
			{"dragger title", drag.title, l.Upload.DragTitle},
			{"dragger hint", drag.hint, ""},
			{"upload uploading", up.loc.Upload.Uploading, l.Upload.Uploading},
			{"qrcode expired", qr.loc.QRCode.Expired, l.QRCode.Expired},
			{"image loading", im.loc.Loading, l.Loading},
			{"color none", cp.loc.ColorPicker.None, l.ColorPicker.None},
			{"color presets", cp.presetName, l.ColorPicker.Presets},
			{"range start", rp.placeholders()[0], l.StartDate},
			{"range end", rp.placeholders()[1], l.EndDate},
			{"week range", i18nJoinPH(rpp[0]), l.DatePicker.StartWeek + "|" + l.DatePicker.EndWeek},
			{"month range", i18nJoinPH(rpp[1]), l.DatePicker.StartMonth + "|" + l.DatePicker.EndMonth},
			{"quarter range", i18nJoinPH(rpp[2]), l.DatePicker.StartQuarter + "|" + l.DatePicker.EndQuarter},
			{"year range", i18nJoinPH(rpp[3]), l.DatePicker.StartYear + "|" + l.DatePicker.EndYear},
			{"explicit range", i18nJoinPH(rpx), "a|b"},
			{"range sep", rp.sep, l.RangeSeparator},
			{"date placeholder", dp.placeholder, l.SelectDate},
			{"datetime placeholder", dt.f.ph, l.SelectDateTime},
			{"time placeholder", tp.f.ph, l.SelectTime},
			{"treeselect placeholder", ts.box.placeholder, l.SelectPlaceholder},
			{"timeselect placeholder", tsel.placeholder, l.SelectTime},
			{"markdown placeholder", md.placeholder, l.MarkdownEditor.Placeholder},
		}
		for _, c := range checks {
			if c.got != c.want {
				t.Errorf("%s: %s = %q, want %q", l.Code, c.what, c.got, c.want)
			}
		}
		// Element dragger: "<DropPrefix><Button>".
		d2 := NewDriver(func(c *Context) {
			ConfigProvider(c).Locale(l).Body(func() {
				var files []UploadFile
				upEl = Upload(c, &files).Dragger()
			})
		}, DriverOptions{Width: 600, Height: 400, Theme: theme.ElementPlus()})
		d2.Frame()
		if upEl.title != l.Upload.DropPrefix || upEl.hint != "" {
			t.Errorf("%s: element dragger %q / %q", l.Code, upEl.title, upEl.hint)
		}
	}
}

// i18nJoinPH joins a RangePicker's effective placeholders as "start|end".
func i18nJoinPH(w *RangePickerW) string { p := w.placeholders(); return p[0] + "|" + p[1] }

// TestI18nDefaultLocale switches the default locale (no ConfigProvider),
// as uihost will do when following the system language.
func TestI18nDefaultLocale(t *testing.T) {
	defer SetDefaultLocale(ZhCN)
	page := func(c *Context) {
		p := 1
		Pagination(c, &p, 85, 10).ShowTotal().Show()
		Empty(c)
	}
	for _, l := range i18nLocales {
		SetDefaultLocale(l)
		d := NewDriver(page, DriverOptions{Width: 800, Height: 300})
		d.Frame()
		for _, s := range []string{l.Total(85), l.NoData} {
			if _, ok := d.FindText(s); !ok {
				t.Errorf("%s: %q missing", l.Code, s)
			}
		}
	}
}

// TestI18nWeekStart checks the month grid order follows Locale.WeekStart:
// the first cell of January 2026 is Sunday Dec 28 for a Sunday-first locale
// (all built-in packs) and Monday Dec 29 for a Monday-first one.
func TestI18nWeekStart(t *testing.T) {
	for _, l := range i18nLocales {
		ids := i18nIDs{}
		tc := i18nCase{page: func(c *Context, ids i18nIDs) {
			v := i18nDate
			ids["w"] = DatePicker(c, &v).n.id
		}, setup: func(d *Driver, ids i18nIDs) { i18nClick(d, ids, "w") }}
		d, _ := i18nRender(tc, l)
		_ = ids
		first := "2025-12-28"
		if l.WeekStart == time.Monday {
			first = "2025-12-29"
		}
		r1, ok1 := d.Find(first)
		r2, ok2 := d.Find("2026-01-01")
		if !ok1 || !ok2 {
			t.Fatalf("%s: grid cells not found", l.Code)
		}
		// first is the top-left cell: same row as Jan 1, left of it.
		if r1.Y != r2.Y || r1.X >= r2.X {
			t.Errorf("%s: %s at %v, Jan 1 at %v", l.Code, first, r1, r2)
		}
		if _, ok := d.Find(map[bool]string{true: "2025-12-28", false: "2025-12-29"}[l.WeekStart == time.Monday]); ok && l.WeekStart == time.Monday {
			// Dec 28 is a Sunday: not in a Monday-first grid of January.
			t.Errorf("%s: Dec 28 shown in a Monday-first grid", l.Code)
		}
	}
	_ = geom.Rect{}
}
