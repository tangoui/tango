// Command ledger is a small personal bookkeeping app built with Tango's
// self-drawn UI (no WebView): record income and expenses, filter and sort
// them, see monthly totals and where the money went, export to CSV.
//
//	go run ./examples/ledger                      # data in the user config dir
//	go run ./examples/ledger -data ./ledger.json  # use another file
//
// Packaged like a real app with the tan CLI (from this directory):
//
//	tan doctor                                # checks the icons too
//	tan build                                 # this machine, e.g. Ledger.app
//	tan build -platform windows/amd64         # Ledger.exe with icon and version info
//	tan package -platform linux/amd64         # tarball + AppDir with hicolor icons
//
// Records are saved to a JSON file after every change. On first launch the
// ledger is filled with two months of sample data. Closing the window keeps
// the app in the tray (menu bar on macOS); quit from the tray menu.
//
// Icons: icons/icon.png is the master app icon referenced by tan.json (tan
// build derives the macOS, Windows, Linux, iOS and Android icons from it);
// icons/tray*.png are embedded for the tray.
package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/tangoui/tango"
	"github.com/tangoui/tango/sys/dialog"
	"github.com/tangoui/tango/sys/tray"
	"github.com/tangoui/tango/ui"
	"github.com/tangoui/tango/ui/icon"
	"github.com/tangoui/tango/ui/layout"
	"github.com/tangoui/tango/uihost"
)

const allMonths = "全部月份"

// Ledger is the app: the store plus UI state. Main thread only.
type Ledger struct {
	app *tango.App // nil in tests (no export dialog, no tray)
	win *tango.Surface
	// tray (nil without one; see tray.go)
	tray     *tray.Tray
	runner   *ui.Runner
	quitting bool
	store    *Store

	month  string // allMonths or YYYY-MM
	kind   string // "全部" | "支出" | "收入"
	search string
	page   int
	size   int
	rev    uint64 // bumped on every data change so the table re-sorts

	// focusSearch moves the keyboard focus to the search box on the next
	// frame (CmdOrCtrl+F).
	focusSearch bool

	// editor
	editing  bool
	edit     Entry // ID 0 = new
	editDate time.Time
	editKind string  // "支出" | "收入"
	editAmt  float64 // yuan
	amtEmpty bool    // the amount field is empty
	editCat  string
	editNote string

	notice func(c *ui.Context) // a message to show on the next frame
}

func newLedger(s *Store) *Ledger {
	l := &Ledger{store: s, kind: "全部", page: 1, size: 10, month: allMonths}
	if ms := s.Months(); len(ms) > 0 {
		l.month = ms[0]
	}
	return l
}

// view returns the indexes of the entries that pass the filters, newest first.
func (l *Ledger) view() []int {
	q := strings.ToLower(strings.TrimSpace(l.search))
	var out []int
	for i, e := range l.store.Entries {
		if l.month != allMonths && e.Month() != l.month {
			continue
		}
		if l.kind == "支出" && e.Kind != Expense || l.kind == "收入" && e.Kind != Income {
			continue
		}
		if q != "" && !strings.Contains(strings.ToLower(e.Note+" "+e.Category), q) {
			continue
		}
		out = append(out, i)
	}
	es := l.store.Entries
	sort.SliceStable(out, func(a, b int) bool {
		ea, eb := es[out[a]], es[out[b]]
		if ea.Date != eb.Date {
			return ea.Date > eb.Date
		}
		return ea.ID > eb.ID
	})
	return out
}

// totals sums the selected month (ignoring the kind and search filters).
func (l *Ledger) totals() (in, out int64, n int, byCat map[string]int64) {
	byCat = map[string]int64{}
	for _, e := range l.store.Entries {
		if l.month != allMonths && e.Month() != l.month {
			continue
		}
		n++
		if e.Kind == Income {
			in += e.Cents
		} else {
			out += e.Cents
			byCat[e.Category] += e.Cents
		}
	}
	return
}

func (l *Ledger) changed(err error, c *ui.Context, ok string) {
	l.rev++
	l.refreshTray()
	if err != nil {
		ui.Message(c, "error", "保存失败："+err.Error())
		return
	}
	ui.Message(c, "success", ok)
}

func (l *Ledger) Page(c *ui.Context) {
	if l.notice != nil {
		l.notice(c)
		l.notice = nil
	}
	ui.Scroll(c, func() {
		ui.Column(c, func() {
			l.header(c)
			l.stats(c)
			ui.Row(c, func() {
				l.records(c)
				l.breakdown(c)
			}, layout.Gap(16), layout.AlignItems(layout.AlignStart))
		}, layout.Gap(16), layout.Pad(24))
	})
	l.editor(c)
}

func (l *Ledger) header(c *ui.Context) {
	ui.Row(c, func() {
		ui.Column(c, func() {
			ui.Title(c, "记账本", 3)
			ui.Text(c, "数据文件："+l.store.Path()).Tertiary().Size(12)
		}, layout.Gap(2))
		ui.Spacer(c)
		months := append([]string{allMonths}, l.store.Months()...)
		if ui.Select(c, &l.month, ui.Opts(months...)).Width(140).Changed() {
			l.page = 1
		}
		if ui.Button(c, "导出 CSV").Icon(icon.Download).Clicked() {
			l.export(c)
		}
		if ui.Button(c, "记一笔").Primary().Icon(icon.Plus).Clicked() {
			l.openEditor(Entry{})
		}
	}, layout.Middle(), layout.Gap(8))
}

func (l *Ledger) stats(c *ui.Context) {
	in, out, n, _ := l.totals()
	th := c.Theme
	card := func(title string, cents int64, col string) {
		ui.Card(c, "").Layout(layout.Grow(1)).Body(func() {
			s := ui.Statistic(c, title, float64(cents)/100, 2).Prefix("¥")
			switch col {
			case "success":
				s.Color(th.Success.Base)
			case "error":
				s.Color(th.Error.Base)
			}
		})
	}
	ui.Row(c, func() {
		card("收入", in, "success")
		card("支出", out, "error")
		bal := "success"
		if in-out < 0 {
			bal = "error"
		}
		card("结余", in-out, bal)
		ui.Card(c, "").Layout(layout.Grow(1)).Body(func() {
			ui.Statistic(c, "笔数", float64(n), 0)
		})
	}, layout.Gap(16))
}

func (l *Ledger) records(c *ui.Context) {
	es := l.store.Entries
	rows := l.view()
	e := func(i int) Entry { return es[rows[i]] }
	th := c.Theme

	ui.Card(c, "明细").Layout(layout.Grow(1)).Extra(func() {
		ui.Row(c, func() {
			if ui.Segmented(c, &l.kind, ui.Opts("全部", "支出", "收入")).Changed() {
				l.page = 1
			}
			sw := ui.Search(c, &l.search).Placeholder("搜索备注或分类").AllowClear().Width(200)
			if sw.Changed() {
				l.page = 1
			}
			if l.focusSearch {
				l.focusSearch = false
				c.SetFocus(sw.InputID())
			}
		}, layout.Gap(8), layout.Middle())
	}).Body(func() {
		cols := []ui.TableColumn{
			{Key: "date", Title: "日期", Width: 120, Sortable: true,
				Compare: func(a, b int) int { return strings.Compare(e(a).Date, e(b).Date) },
				Text:    func(i int) string { return e(i).Date }},
			{Key: "kind", Title: "类型", Width: 80, Render: func(i int) {
				if e(i).Kind == Income {
					ui.Tag(c, "收入").Color("success")
				} else {
					ui.Tag(c, "支出").Color("error")
				}
			}},
			{Key: "cat", Title: "分类", Width: 90, Text: func(i int) string { return e(i).Category }},
			{Key: "note", Title: "备注", Ellipsis: true, Text: func(i int) string { return e(i).Note }},
			{Key: "amt", Title: "金额", Width: 130, Align: ui.AlignRight, Sortable: true,
				Compare: func(a, b int) int { return cmp64(e(a).Signed(), e(b).Signed()) },
				Render: func(i int) {
					en := e(i)
					if en.Kind == Income {
						ui.Text(c, "+"+money(en.Cents)).Color(th.Success.Base).Strong()
					} else {
						ui.Text(c, "-"+money(en.Cents)).Strong()
					}
				}},
			{Key: "op", Title: "操作", Width: 120, Render: func(i int) {
				en := e(i)
				ui.Row(c, func() {
					if ui.Button(c, "编辑").Link().Small().Clicked() {
						l.openEditor(en)
					}
					if ui.Popconfirm(c, "删除这条记录？", money(en.Cents)+" · "+en.Category, func() {
						ui.Button(c, "删除").Link().Danger().Small()
					}).Confirmed() {
						l.changed(l.store.Delete(en.ID), c, "已删除")
					}
				}, layout.Gap(4))
			}},
		}
		ui.Table(c, cols, len(rows)).
			RowKey(func(i int) string { return strconv.FormatInt(e(i).ID, 10) }).
			Revision(l.rev).
			Pagination(&l.page, &l.size).
			PageSizes(10, 20, 50).
			EmptyText("没有符合条件的记录").
			Show()
	})
}

func (l *Ledger) breakdown(c *ui.Context) {
	_, out, _, byCat := l.totals()
	type kv struct {
		cat   string
		cents int64
	}
	var list []kv
	for k, v := range byCat {
		list = append(list, kv{k, v})
	}
	sort.Slice(list, func(i, j int) bool {
		if list[i].cents != list[j].cents {
			return list[i].cents > list[j].cents
		}
		return list[i].cat < list[j].cat
	})
	ui.Card(c, "支出构成").Layout(layout.W(300)).Body(func() {
		if len(list) == 0 {
			ui.Empty(c)
			return
		}
		ui.Column(c, func() {
			for _, it := range list {
				ui.Key(c, it.cat, func() {
					ui.Column(c, func() {
						ui.Row(c, func() {
							ui.Text(c, it.cat)
							ui.Spacer(c)
							ui.Text(c, money(it.cents)).Secondary()
						})
						pct := float64(it.cents) * 100 / float64(out)
						ui.Progress(c, pct).Small().Format(func(p float64) string { return fmt.Sprintf("%.0f%%", p) })
					}, layout.Gap(2))
				})
			}
		}, layout.Gap(12))
	})
}

// ---------------------------------------------------------------------------
// editor

// setupShortcuts binds the window's keyboard shortcuts (they also work
// while a text field has the focus): CmdOrCtrl+N opens the editor for a
// new record, CmdOrCtrl+F focuses the search box. Escape closing the
// editor is the Modal's own.
func (l *Ledger) setupShortcuts(s *tango.Surface) {
	for _, sc := range []struct {
		accel string
		fn    func()
	}{{"CmdOrCtrl+N", l.newEntry}, {"CmdOrCtrl+F", l.findEntries}} {
		if _, err := uihost.Shortcut(s, sc.accel, sc.fn); err != nil {
			log.Print(err)
		}
	}
}

// newEntry opens the editor for a new record unless it is already open.
func (l *Ledger) newEntry() {
	if !l.editing {
		l.openEditor(Entry{})
	}
}

// findEntries focuses the search box (not while the editor is open).
func (l *Ledger) findEntries() {
	if !l.editing {
		l.focusSearch = true
	}
}

func (l *Ledger) openEditor(e Entry) {
	l.edit, l.editing = e, true
	l.editKind, l.editCat, l.editNote, l.editAmt, l.amtEmpty = "支出", "", "", 0, true
	l.editDate = time.Now()
	if e.ID != 0 {
		if e.Kind == Income {
			l.editKind = "收入"
		}
		l.editCat, l.editNote = e.Category, e.Note
		l.editAmt, l.amtEmpty = float64(e.Cents)/100, false
		if t, err := time.ParseInLocation(dateLayout, e.Date, time.Local); err == nil {
			l.editDate = t
		}
	}
}

func (l *Ledger) editor(c *ui.Context) {
	title := "记一笔"
	if l.edit.ID != 0 {
		title = "编辑记录"
	}
	kind := Expense
	if l.editKind == "收入" {
		kind = Income
	}
	var form *ui.FormW
	m := ui.Modal(c, &l.editing, title).Width(460).OkText("保存").Body(func() {
		form = ui.Form(c).LabelWidth(64)
		form.Body(func() {
			form.Item("类型").Body(func() {
				if ui.Segmented(c, &l.editKind, ui.Opts("支出", "收入")).Changed() {
					l.editCat = "" // categories differ per kind
				}
			})
			form.Item("日期").Body(func() {
				ui.DatePicker(c, &l.editDate).DisabledDate(func(t time.Time) bool { return t.After(time.Now()) })
			})
			form.Item("金额").RulesFunc(l.amountText, ui.Required("请输入金额"), ui.Check(checkAmount)).Body(func() {
				ui.InputNumber(c, &l.editAmt).Empty(&l.amtEmpty).Placeholder("0.00").
					Min(0).Max(1e9).Precision(2).Step(10).Width(200)
			})
			form.Item("分类").Rules(&l.editCat, ui.Required("请选择分类")).Body(func() {
				ui.Select(c, &l.editCat, ui.Opts(categories[kind]...)).Placeholder("请选择").Width(200)
			})
			form.Item("备注").Body(func() {
				ui.Input(c, &l.editNote).Placeholder("可选").MaxLength(60).AllowClear()
			})
		})
	})
	if m.Cancelled() && form != nil {
		form.ResetValidation()
	}
	if !m.Ok() || form == nil || !form.Validate() {
		return
	}
	e := l.edit
	e.Kind, e.Category, e.Note = kind, l.editCat, strings.TrimSpace(l.editNote)
	e.Cents = toCents(l.editAmt)
	e.Date = l.editDate.Format(dateLayout)
	msg := "已记录"
	if e.ID != 0 {
		msg = "已更新"
	}
	l.changed(l.store.Put(e), c, msg)
	l.editing = false
	if l.month != allMonths && e.Month() != l.month {
		l.month = e.Month() // jump to where the new record is
	}
	form.ResetValidation()
}

// ---------------------------------------------------------------------------
// export

// export asks for a file name and writes the filtered records as CSV. The
// native dialog blocks, so it runs on its own goroutine and reports back
// through app.Post.
func (l *Ledger) export(c *ui.Context) {
	if l.app == nil {
		return
	}
	rows := l.view()
	entries := make([]Entry, len(rows))
	for i, r := range rows {
		entries[i] = l.store.Entries[r]
	}
	name := "账本.csv"
	if l.month != allMonths {
		name = "账本-" + l.month + ".csv"
	}
	wake := c.Invalidator()
	go func() {
		path, err := dialog.Save(l.app, dialog.FileOptions{
			Parent: l.win, Title: "导出 CSV", Filename: name, CanCreateDirs: true,
			Filters: []dialog.FileFilter{{Name: "CSV", Patterns: []string{"*.csv"}}},
		})
		if err == dialog.ErrCanceled {
			return
		}
		if err == nil {
			err = writeCSVFile(path, entries)
		}
		l.app.Post(func() {
			l.notice = func(c *ui.Context) {
				if err != nil {
					ui.Message(c, "error", "导出失败："+err.Error())
				} else {
					ui.Message(c, "success", fmt.Sprintf("已导出 %d 条到 %s", len(entries), filepath.Base(path)))
				}
			}
			wake()
		})
	}()
}

func writeCSVFile(path string, entries []Entry) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	if err := WriteCSV(f, entries); err != nil {
		f.Close()
		return err
	}
	return f.Close()
}

// amountText is the amount as the form validates it ("" when empty).
func (l *Ledger) amountText() string {
	if l.amtEmpty {
		return ""
	}
	return centsString(toCents(l.editAmt))
}

// checkAmount rejects amounts that round to zero cents.
func checkAmount(v string) string {
	if c, err := parseCents(v); v != "" && (err != nil || c <= 0) {
		return "金额必须大于 0"
	}
	return ""
}

func cmp64(a, b int64) int {
	switch {
	case a < b:
		return -1
	case a > b:
		return 1
	}
	return 0
}

func defaultDataPath() string {
	dir, err := os.UserConfigDir()
	if err != nil {
		dir = "."
	}
	return filepath.Join(dir, "TangoLedger", "ledger.json")
}

func main() {
	data := flag.String("data", defaultDataPath(), "ledger file")
	flag.Parse()

	store, err := Open(*data, seedSample(time.Now()))
	if err != nil {
		log.Fatal(err)
	}
	l := newLedger(store)

	quit := true
	l.app = tango.New(tango.Options{Name: "记账本", ID: "dev.tango.ledger", QuitOnLastClose: &quit})
	l.app.OnReady(func() {
		r, s, err := uihost.Open(l.app, uihost.Options{Surface: tango.SurfaceOptions{
			Title: "记账本", Width: 1180, Height: 780, MinWidth: 900, MinHeight: 560,
			Resizable: true, Centered: true,
		}}, l.Page, ui.RunnerOptions{})
		if err != nil {
			log.Fatal(err)
		}
		l.win = s
		l.setupShortcuts(s)
		l.setupTray(r)
	})
	if err := l.app.Run(); err != nil {
		log.Fatal(err)
	}
}
