package ui

import (
	"errors"
	"fmt"
	"math"
	"slices"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/tangoui/tango/ui/a11y"
	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/input"
	"github.com/tangoui/tango/ui/theme"
)

// ---------------------------------------------------------------------------
// EditableProTable

type p2eRow struct {
	ID, Name string
	Age      float64
}

func p2eEditCols() []ProColumn[p2eRow] {
	return []ProColumn[p2eRow]{
		{Key: "name", Title: "Name", Width: 220, Value: func(r p2eRow) any { return r.Name },
			Editor: &ProField{Required: true, AsyncValidate: func(v any, done func(string)) {
				if v == "taken" {
					done("Name taken")
					return
				}
				go done("")
			}},
			Set: func(r *p2eRow, v any) { r.Name, _ = v.(string) }},
		{Key: "age", Title: "Age", ValueType: ValueDigit, Width: 160, Value: func(r p2eRow) any { return r.Age },
			Editor: &ProField{Validate: func(v any, _ ProValues) string {
				if f, _ := proNumber(v); f > 150 {
					return "Too old"
				}
				return ""
			}},
			Set: func(r *p2eRow, v any) { r.Age, _ = proNumber(v) }},
		{Key: "op", Title: "Actions", ValueType: ValueOption},
	}
}

func TestEditableProTableAddEditDelete(t *testing.T) {
	pro3Themes(t, func(t *testing.T, th func() *theme.Theme) {
		rows := []p2eRow{{"a", "Alice", 30}, {"b", "Bob", 40}}
		seq := 0
		var e *EditableProTableW[p2eRow]
		var saves []string
		d := pro3Driver(th, 900, 500, func(c *Context) {
			e = EditableProTable(c, p2eEditCols(), &rows).Title("People").
				RowKey(func(r p2eRow) string { return r.ID }).
				NewRow(func() p2eRow { seq++; return p2eRow{ID: fmt.Sprint("n", seq)} }).
				OnSave(func(r p2eRow, i int, done func(error)) {
					saves = append(saves, fmt.Sprintf("%s@%d", r.Name, i))
					go done(nil)
				}).Show()
		})
		if !pro3HasText(d, "Alice") || !pro3HasText(d, "Bob") {
			t.Fatal("rows not shown")
		}
		// Add a row: it opens in edit mode; an empty name is refused.
		if err := d.ClickLabel(ZhCN.Form.AddRow); err != nil {
			t.Fatal(err)
		}
		if !e.Editing("new:1") {
			t.Fatal("new row not editing")
		}
		d.ClickLabel(ZhCN.Save)
		if e.Errors("new:1")["name"] == "" || !pro3HasText(d, fmt.Sprintf(ZhCN.Form.RequiredInputFmt, "Name")) {
			t.Fatalf("required error missing: %v", e.Errors("new:1"))
		}
		if len(rows) != 2 {
			t.Fatal("invalid row saved")
		}
		// The async validator refuses "taken".
		eds := pro3TextEditors(d)
		if len(eds) == 0 {
			t.Fatal("no editor")
		}
		d.Click(eds[0].Center())
		d.Type("taken")
		d.ClickLabel(ZhCN.Save)
		pro3Wait(t, d, func() bool { return !e.Validating("new:1") })
		if e.Errors("new:1")["name"] != "Name taken" || !pro3HasText(d, "Name taken") {
			t.Fatalf("async error: %v", e.Errors("new:1"))
		}
		d.Click(eds[0].Center())
		for range 5 {
			d.Key(input.KeyBackspace, 0)
		}
		d.Type("Carol")
		d.ClickLabel(ZhCN.Save)
		pro3Wait(t, d, func() bool { return !e.Editing("new:1") })
		if len(rows) != 3 || rows[2].Name != "Carol" || rows[2].ID != "n1" || len(saves) != 1 || saves[0] != "Carol@-1" {
			t.Fatalf("rows %v saves %v", rows, saves)
		}
		// Edit an existing row and cancel: nothing changes.
		p2eClickInRow(t, d, "a", ZhCN.Edit)
		if !e.Editing("a") {
			t.Fatalf("edit: keys %v", *e.editKeys())
		}
		eds = pro3TextEditors(d)
		d.Click(eds[0].Center())
		d.Type("ZZ")
		d.ClickLabel(ZhCN.Cancel)
		if e.Editing("a") || rows[0].Name != "Alice" {
			t.Fatalf("cancel: %v", rows)
		}
		// Edit and save.
		p2eClickInRow(t, d, "a", ZhCN.Edit)
		eds = pro3TextEditors(d)
		d.Click(eds[0].Center())
		d.Type("!")
		d.ClickLabel(ZhCN.Save)
		pro3Wait(t, d, func() bool { return !e.Editing("a") })
		if rows[0].Name != "Alice!" || saves[1] != "Alice!@0" {
			t.Fatalf("save: %v %v", rows, saves)
		}
		// Delete with confirmation.
		p2eClickInRow(t, d, "a", ZhCN.Delete)
		if err := d.ClickLabel(ZhCN.OK); err != nil {
			t.Fatal(err)
		}
		if len(rows) != 2 || rows[0].ID != "b" {
			t.Fatalf("delete: %v", rows)
		}
	})
}

// p2eClickInRow clicks the widget labelled label inside table row key.
func p2eClickInRow(t *testing.T, d *Driver, key, label string) {
	t.Helper()
	row := mustFind(t, d, "row:"+key)
	for _, h := range d.r.c.hits {
		if h.label == label && h.rect.Center().Y >= row.Y && h.rect.Center().Y < row.Bottom() {
			d.Click(h.rect.Center())
			return
		}
	}
	t.Fatalf("no %q in row %s", label, key)
}

func TestEditableProTableSingleRowAndCrossFieldValidate(t *testing.T) {
	rows := []p2eRow{{"a", "Alice", 30}, {"b", "Bob", 40}}
	var e *EditableProTableW[p2eRow]
	keys := []string{"a"}
	d := pro3Driver(func() *theme.Theme { return theme.AntDesign() }, 900, 400, func(c *Context) {
		e = EditableProTable(c, p2eEditCols(), &rows).RowKey(func(r p2eRow) string { return r.ID }).
			EditableKeys(&keys).Show()
	})
	if !e.Editing("a") {
		t.Fatal("controlled keys")
	}
	// Single mode: the other row's edit link is disabled.
	p2eClickInRow(t, d, "b", ZhCN.Edit)
	if e.Editing("b") {
		t.Fatal("second row editable in single mode")
	}
	// The age editor is a spin button; 200 fails Validate.
	var num geom.Rect
	for _, h := range d.r.c.hits {
		if h.cursor == input.CursorText && h.rect.X > 200 {
			num = h.rect
			break
		}
	}
	d.Click(num.Center())
	d.Key(input.KeyEnd, 0)
	d.Key(input.KeyBackspace, 0)
	d.Key(input.KeyBackspace, 0)
	d.Type("200")
	d.Key(input.KeyEnter, 0)
	d.ClickLabel(ZhCN.Save)
	if e.Errors("a")["age"] != "Too old" {
		t.Fatalf("validate: %v", e.Errors("a"))
	}
	if rows[0].Age != 30 {
		t.Fatal("invalid value written")
	}
}

func TestEditableProTableA11y(t *testing.T) {
	rows := []p2eRow{{"a", "Alice", 30}}
	keys := []string{"a"}
	d := pro3Driver(func() *theme.Theme { return theme.ElementPlus() }, 900, 400, func(c *Context) {
		EditableProTable(c, p2eEditCols(), &rows).Title("People").RowKey(func(r p2eRow) string { return r.ID }).
			EditableKeys(&keys).NewRow(func() p2eRow { return p2eRow{ID: "x"} }).Show()
	})
	d.EnableAccessibility()
	d.Frame()
	if n := mustNode(t, d, a11y.RoleTextbox, "Name"); !n.States.Has(a11y.Required) {
		t.Fatalf("name editor: %+v", n)
	}
	mustNode(t, d, a11y.RoleSpinButton, "Age")
	if rep := d.AuditNoContrast(); rep.Count(AuditMissingName) != 0 {
		t.Fatalf("audit: %s", rep)
	}
}

func TestEditableProTableDragSort(t *testing.T) {
	rows := []p2eRow{{"a", "A", 1}, {"b", "B", 2}, {"c", "C", 3}}
	var moved [2]int
	d := pro3Driver(func() *theme.Theme { return theme.AntDesign() }, 800, 400, func(c *Context) {
		EditableProTable(c, p2eEditCols(), &rows).RowKey(func(r p2eRow) string { return r.ID }).
			DragSort(func(from, to int) { moved = [2]int{from, to} }).Show()
	})
	h := mustFind(t, d, "drag:a")
	to := mustFind(t, d, "drag:c")
	d.Drag(h.Center(), to.Center())
	if rows[0].ID != "b" || rows[2].ID != "a" || moved != [2]int{0, 2} {
		t.Fatalf("rows %v moved %v", rows, moved)
	}
}

// ---------------------------------------------------------------------------
// ProTable DragSort, RowClass and currency

func TestProTableDragSortAndCurrency(t *testing.T) {
	type row struct {
		ID     string
		Amount float64
	}
	data := []row{{"a", 1234.5}, {"b", -3}, {"c", 7}}
	cols := []ProColumn[row]{
		{Key: "id", Title: "ID", Value: func(r row) any { return r.ID }, HideInSearch: true},
		{Key: "amount", Title: "Amount", ValueType: ValueMoney, Value: func(r row) any { return r.Amount }, HideInSearch: true},
	}
	for _, tc := range []struct {
		loc       *Locale
		want, neg string
	}{{ZhCN, "¥1,234.50", "-¥3.00"}, {EnUS, "$1,234.50", "-$3.00"}, {ZhTW, "NT$1,234.50", "-NT$3.00"}, {Ja, "¥1,235", "-¥3"}} {
		var p *ProTableW[row]
		var froms []int
		d := pro3Driver(func() *theme.Theme { return theme.AntDesign() }, 800, 500, func(c *Context) {
			ConfigProvider(c).Locale(tc.loc).Body(func() {
				p = ProTable(c, cols).RowKey(func(r row) string { return r.ID }).
					Request(func(req ProRequest, done func(ProResult[row])) { done(ProResult[row]{Rows: data, Total: len(data)}) }).
					DragSort(func(from, to int) { froms = append(froms, from, to); p.MovePageRow(from, to) }).Show()
			})
		})
		if !pro3HasText(d, tc.want) {
			t.Fatalf("%s: %q not shown", tc.loc.Code, tc.want)
		}
		if !pro3HasText(d, tc.neg) {
			t.Fatalf("%s: negative amount", tc.loc.Code)
		}
		if tc.loc != ZhCN {
			continue
		}
		h := mustFind(t, d, "drag:a")
		to := mustFind(t, d, "drag:c")
		d.Drag(h.Center(), to.Center())
		if !slices.Equal(froms, []int{0, 2}) || p.Rows()[2].ID != "a" {
			t.Fatalf("drag: %v %v", froms, p.Rows())
		}
	}
	// ProColumn.Currency overrides the locale.
	cols[1].Currency = "€"
	d := pro3Driver(func() *theme.Theme { return theme.ElementPlus() }, 800, 500, func(c *Context) {
		ProTable(c, cols).Request(func(req ProRequest, done func(ProResult[row])) { done(ProResult[row]{Rows: data, Total: 3}) }).Show()
	})
	if !pro3HasText(d, "€1,234.50") {
		t.Fatal("currency override")
	}
}

func TestTableRowClass(t *testing.T) {
	pro3Themes(t, func(t *testing.T, th func() *theme.Theme) {
		cols := []TableColumn{{Key: "v", Title: "V", Text: func(i int) string { return fmt.Sprint("row ", i) }}}
		red := theme.Hex("#ff0000")
		d := pro3Driver(th, 500, 400, func(c *Context) {
			Table(c, cols, 3).RowKey(func(i int) string { return fmt.Sprint(i) }).RowClass(func(i int) RowStyle {
				switch i {
				case 0:
					return RowStyle{Bg: red, Bold: true}
				case 1:
					return RowStyle{Status: "warning"}
				}
				return RowStyle{}
			}).Show()
		})
		img := d.Frame()
		at := func(key string) theme.Color {
			r := mustFind(t, d, "row:"+key)
			c := img.RGBAAt(int(r.X+r.W-6), int(r.Y+4))
			return theme.Color{R: c.R, G: c.G, B: c.B, A: c.A}
		}
		if got := at("0"); got.R < 240 || got.G > 20 {
			t.Fatalf("row 0 bg %v", got)
		}
		warn := d.r.c.Theme.Warning.Bg
		if got := at("1"); p2eDiff(got.R, warn.R) > 3 || p2eDiff(got.G, warn.G) > 3 || p2eDiff(got.B, warn.B) > 3 {
			t.Fatalf("row 1 bg %v want %v", got, warn)
		}
		if got := at("2"); got.R < 250 || got.G < 250 {
			t.Fatalf("row 2 bg %v", got)
		}
	})
}

func p2eDiff(a, b uint8) int {
	if a > b {
		return int(a - b)
	}
	return int(b - a)
}

func TestTableSelectionsMenu(t *testing.T) {
	pro3Themes(t, func(t *testing.T, th func() *theme.Theme) {
		cols := []TableColumn{{Key: "v", Title: "V", Text: func(i int) string { return fmt.Sprint("row ", i) }}}
		var sel []int
		page, size := 1, 3
		var custom []int
		d := pro3Driver(th, 600, 500, func(c *Context) {
			Table(c, cols, 7).Selection(&sel).Pagination(&page, &size).
				SelectDisabled(func(i int) bool { return i == 5 }).Selections().Show()
		})
		menu := func(item string) {
			t.Helper()
			if err := d.ClickLabel(ZhCN.Table.Selections); err != nil {
				t.Fatal(err)
			}
			if err := d.ClickLabel(item); err != nil {
				t.Fatal(err)
			}
		}
		menu(ZhCN.SelectionAll)
		slices.Sort(sel)
		if !slices.Equal(sel, []int{0, 1, 2, 3, 4, 6}) {
			t.Fatalf("all: %v", sel)
		}
		menu(ZhCN.SelectionInvert)
		slices.Sort(sel)
		if !slices.Equal(sel, []int{3, 4, 6}) {
			t.Fatalf("invert: %v", sel)
		}
		menu(ZhCN.Table.SelectNone)
		if len(sel) != 0 {
			t.Fatalf("none: %v", sel)
		}
		page = 2
		d.Settle()
		menu(ZhCN.Table.SelectPage)
		slices.Sort(sel)
		if !slices.Equal(sel, []int{3, 4}) {
			t.Fatalf("page: %v", sel)
		}
		// Custom items get the page rows.
		d2 := pro3Driver(th, 600, 500, func(c *Context) {
			Table(c, cols, 4).Selection(&sel).Selections(TableSelection{Key: "odd", Text: "Odd rows", OnSelect: func(rows []int) {
				custom = rows
				sel = sel[:0]
				for _, r := range rows {
					if r%2 == 1 {
						sel = append(sel, r)
					}
				}
			}}).Show()
		})
		d2.ClickLabel(ZhCN.Table.Selections)
		d2.ClickLabel("Odd rows")
		if !slices.Equal(custom, []int{0, 1, 2, 3}) || !slices.Equal(sel, []int{1, 3}) {
			t.Fatalf("custom: %v %v", custom, sel)
		}
	})
}

// ---------------------------------------------------------------------------
// ProField value types

var p2eTree3 = []ProEnum{
	{Value: "zj", Label: "Zhejiang", Children: []ProEnum{{Value: "hz", Label: "Hangzhou", Children: []ProEnum{{Value: "xh", Label: "West Lake"}}}}},
	{Value: "js", Label: "Jiangsu", Children: []ProEnum{{Value: "nj", Label: "Nanjing"}}},
}

func p2eTypeFields() []ProField {
	return []ProField{
		{Key: "area", Label: "Area", ValueType: ValueCascader, ValueEnum: p2eTree3},
		{Key: "city", Label: "City", ValueType: ValueTreeSelect, ValueEnum: p2eTree3},
		{Key: "files", Label: "Files", ValueType: ValueUpload},
		{Key: "at", Label: "At", ValueType: ValueTime},
		{Key: "span", Label: "Span", ValueType: ValueTimeRange},
		{Key: "color", Label: "Color", ValueType: ValueColor},
		{Key: "price", Label: "Price", ValueType: ValueDigitRange},
	}
}

func p2eTypeValues() ProValues {
	at := time.Date(2026, 1, 2, 9, 30, 0, 0, time.UTC)
	return ProValues{
		"area":  []string{"zj", "hz", "xh"},
		"city":  "nj",
		"files": []UploadFile{{Name: "a.png", Status: "done"}, {Name: "b.pdf", Status: "done"}},
		"at":    at,
		"span":  [2]time.Time{at, at.Add(90 * time.Minute)},
		"color": "#1677ff",
		"price": [2]float64{10, math.NaN()},
	}
}

func TestProFieldValueTypesReadOnly(t *testing.T) {
	pro3Themes(t, func(t *testing.T, th func() *theme.Theme) {
		vals := p2eTypeValues()
		var items []ProDescItem
		for _, fd := range p2eTypeFields() {
			items = append(items, ProDescItem{Key: fd.Key, Label: fd.Label, ValueType: fd.ValueType, ValueEnum: fd.ValueEnum})
		}
		d := pro3Driver(th, 1000, 500, func(c *Context) {
			ProDescriptions(c, items).Column(2).DataSource(&vals).Show()
		})
		for _, s := range []string{"Zhejiang / Hangzhou / West Lake", "Nanjing", "a.png, b.pdf", "09:30:00",
			"09:30:00 " + ZhCN.RangeSeparator + " 11:00:00", "#1677FF", "10 " + ZhCN.RangeSeparator + " -"} {
			if !pro3HasText(d, s) {
				t.Fatalf("%q not shown", s)
			}
		}
		// Empty values read "-".
		empty := ProValues{}
		c := d.r.c
		for _, fd := range p2eTypeFields() {
			if s := pro3ValueText(c, fd.ValueType, fd.ValueEnum, empty[fd.Key]); s != "-" {
				t.Fatalf("%s empty: %q", fd.ValueType, s)
			}
		}
	})
}

func TestProFieldValueTypesEdit(t *testing.T) {
	pro3Themes(t, func(t *testing.T, th func() *theme.Theme) {
		vals := p2eTypeValues()
		picks := 0
		fields := p2eTypeFields()
		fields[2].Pick = func(done func([]UploadFile)) {
			picks++
			go done([]UploadFile{{Name: "c.txt", Status: "done"}})
		}
		d := pro3Driver(th, 900, 1100, func(c *Context) {
			ProForm(c, fields).Values(&vals).Show()
		})
		d.EnableAccessibility()
		d.Frame()
		for _, n := range []struct {
			role a11y.Role
			name string
		}{{a11y.RoleCombobox, "Area"}, {a11y.RoleCombobox, "City"}, {a11y.RoleCombobox, "Color"},
			{a11y.RoleSpinButton, locFmt(ZhCN.ProField.RangeEndFmt, "Price", ZhCN.ProField.Min)}, {a11y.RoleSpinButton, locFmt(ZhCN.ProField.RangeEndFmt, "Price", ZhCN.ProField.Max)}} {
			mustNode(t, d, n.role, n.name)
		}
		// Upload: the trigger asks the app for files.
		if err := d.ClickLabel(ZhCN.Upload.Button); err != nil {
			t.Fatal(err)
		}
		pro3Wait(t, d, func() bool { return len(vals.Files("files")) == 3 })
		if picks != 1 || vals.Files("files")[2].Name != "c.txt" {
			t.Fatalf("upload: %v", vals.Files("files"))
		}
		// Digit range: typing the max fills the NaN end.
		var maxBox geom.Rect
		n := mustNode(t, d, a11y.RoleSpinButton, locFmt(ZhCN.ProField.RangeEndFmt, "Price", ZhCN.ProField.Max))
		maxBox = geom.Rect{X: n.Bounds.X, Y: n.Bounds.Y, W: n.Bounds.W, H: n.Bounds.H}
		d.Click(maxBox.Center())
		d.Type("99")
		d.Key(input.KeyEnter, 0)
		d.Key(input.KeyTab, 0)
		if r := vals.DigitRange("price"); r[0] != 10 || r[1] != 99 {
			t.Fatalf("digit range: %v", r)
		}
		if rep := d.AuditNoContrast(); rep.Count(AuditMissingName) != 0 {
			t.Fatalf("audit: %s", rep)
		}
	})
}

func TestProFieldValueTypeHelpers(t *testing.T) {
	v := ProValues{"r": [2]float64{1, math.NaN()}, "f": []UploadFile{{Name: "x"}}}
	if proValueText(v["r"]) != "1~" || proValueText(v["f"]) != "x" {
		t.Fatal(proValueText(v["r"]), proValueText(v["f"]))
	}
	if r := (ProValues{}).DigitRange("r"); !math.IsNaN(r[0]) || !math.IsNaN(r[1]) {
		t.Fatal("missing range")
	}
	c := v.Clone()
	c.Files("f")[0].Name = "y"
	if v.Files("f")[0].Name != "x" {
		t.Fatal("clone shares files")
	}
	for code, want := range map[string]string{"zh-CN": "CNY", "en-US": "USD", "zh-TW": "TWD", "ja-JP": "JPY", "zh-HK": "HKD", "de-DE": "EUR", "de-CH": "CHF", "zh-Hant": "TWD", "zh-Hant-HK": "HKD", "ko": "KRW", "en": "$", "es-419": "$", "": "CNY"} {
		if got := p2eCurrency(&Locale{Code: code}, ""); got != want {
			t.Fatalf("%s: %q", code, got)
		}
	}
	for _, l := range []*Locale{ZhCN, EnUS, ZhTW, Ja} {
		if got := p2eCurrency(l, ""); got != l.Currency {
			t.Fatalf("%s pack: %q", l.Code, got)
		}
	}
	if p2eCurrency(&Locale{Code: "en-US", Currency: "US$"}, "") != "US$" {
		t.Fatal("locale override")
	}
	if p2eCurrency(ZhCN, "€") != "€" {
		t.Fatal("field override")
	}
}

// ---------------------------------------------------------------------------
// ProDescriptions AsyncValidate

func TestProDescriptionsAsyncValidate(t *testing.T) {
	pro3Themes(t, func(t *testing.T, th func() *theme.Theme) {
		vals := ProValues{"name": "abc"}
		var saves int
		var calls atomic.Int32
		items := []ProDescItem{{Key: "name", Label: "Name", Field: &ProField{AsyncValidate: func(v any, done func(string)) {
			calls.Add(1)
			go func() {
				if v == "abcX" {
					done("Name taken")
					return
				}
				done("")
			}()
		}}}}
		var p *ProDescriptionsW
		d := pro3Driver(th, 700, 300, func(c *Context) {
			p = ProDescriptions(c, items).DataSource(&vals).
				Editable(func(key string, v any, all ProValues, done func(error)) { saves++; done(nil) }).Show()
		})
		d.ClickLabel(locFmt(ZhCN.ProDescriptions.EditFmt, "Name"))
		eds := pro3TextEditors(d)
		d.Click(eds[0].Center())
		d.Type("X")
		d.ClickLabel(ZhCN.Save)
		pro3Wait(t, d, func() bool { return pro3HasText(d, "Name taken") })
		if saves != 0 || vals.String("name") != "abc" || !p.Editing("name") {
			t.Fatal("saved despite the async error")
		}
		d.Click(eds[0].Center())
		d.Type("Y")
		d.ClickLabel(ZhCN.Save)
		pro3Wait(t, d, func() bool { return !p.Editing("name") })
		if saves != 1 || vals.String("name") != "abcXY" || calls.Load() != 2 {
			t.Fatalf("saves %d vals %v calls %d", saves, vals, calls.Load())
		}
	})
}

// ---------------------------------------------------------------------------
// LightFilter collapse

func TestLightFilterCollapse(t *testing.T) {
	pro3Themes(t, func(t *testing.T, th func() *theme.Theme) {
		fields := []ProField{
			{Key: "a", Label: "Alpha"}, {Key: "b", Label: "Beta"},
			{Key: "c", Label: "Gamma", ValueType: ValueSelect, ValueEnum: []ProEnum{{Value: "x", Label: "Ex"}}},
			{Key: "d", Label: "Delta", ValueType: ValueSwitch},
		}
		vals := ProValues{}
		var lf *LightFilterW
		changed := 0
		d := pro3Driver(th, 900, 500, func(c *Context) {
			lf = LightFilter(c, fields).Values(&vals).Collapse(2).Show()
			if lf.Changed() {
				changed++
			}
		})
		if _, ok := d.Find("Gamma"); ok {
			t.Fatal("collapsed field shown as a pill")
		}
		if _, ok := d.Find("Alpha"); !ok {
			t.Fatal("pill missing")
		}
		if err := d.ClickLabel(ZhCN.LightFilter.More); err != nil {
			t.Fatal(err)
		}
		if lf.OpenKey() == "" || !pro3HasText(d, ZhCN.WithColon("Gamma")) || !pro3HasText(d, ZhCN.WithColon("Delta")) {
			t.Fatal("more popup")
		}
		d.EnableAccessibility()
		d.Frame()
		n := mustNode(t, d, a11y.RoleSwitch, "Delta")
		d.Click(geom.Rect{X: n.Bounds.X, Y: n.Bounds.Y, W: n.Bounds.W, H: n.Bounds.H}.Center())
		d.ClickLabel(ZhCN.Confirm)
		if !vals.Bool("d") || lf.OpenKey() != "" || changed != 1 {
			t.Fatalf("confirm: %v open %q changed %d", vals, lf.OpenKey(), changed)
		}
		if !pro3HasText(d, locFmt(ZhCN.LightFilter.MoreFmt, 1)) {
			t.Fatal("count not shown")
		}
		d.ClickLabel(ZhCN.LightFilter.More)
		d.ClickLabel(ZhCN.Clear)
		if _, ok := vals["d"]; ok {
			t.Fatal("clear")
		}
		if rep := d.AuditNoContrast(); rep.Count(AuditMissingName) != 0 {
			t.Fatalf("audit: %s", rep)
		}
	})
}

// ---------------------------------------------------------------------------
// ProList Request

func TestProListRequest(t *testing.T) {
	pro3Themes(t, func(t *testing.T, th func() *theme.Theme) {
		all := pro3Rows(12)
		release := make(chan struct{}, 8)
		var pages []int
		fail := false
		var p *ProListW[pro3Row]
		d := pro3Driver(th, 800, 900, func(c *Context) {
			p = ProList(c, nil, pro3Metas()).RowKey(func(r pro3Row) string { return r.id }).Pagination(5).
				Request(func(page, size int) ([]pro3Row, int, error) {
					<-release
					pages = append(pages, page)
					if fail {
						return nil, 0, errors.New("offline")
					}
					a := (page - 1) * size
					return all[a:min(len(all), a+size)], len(all), nil
				}).Show()
		})
		if !p.IsLoading() {
			t.Fatal("not loading")
		}
		release <- struct{}{}
		pro3Wait(t, d, func() bool { return !p.IsLoading() })
		if !pro3HasText(d, "Item 0") || pro3HasText(d, "Item 5") || p.Total() != 12 {
			t.Fatal("page 1")
		}
		if err := d.ClickLabel("3"); err != nil {
			t.Fatal(err)
		}
		if !p.IsLoading() || p.Page() != 3 {
			t.Fatalf("page change: %d", p.Page())
		}
		release <- struct{}{}
		pro3Wait(t, d, func() bool { return !p.IsLoading() })
		if !pro3HasText(d, "Item 10") || pro3HasText(d, "Item 0") || !slices.Equal(pages, []int{1, 3}) {
			t.Fatalf("page 3: %v", pages)
		}
		fail = true
		p.Reload()
		release <- struct{}{}
		d.Settle()
		pro3Wait(t, d, func() bool { return !p.IsLoading() })
		if p.Err() == nil || !pro3HasText(d, "offline") {
			t.Fatal("error not shown")
		}
		fail = false
		release <- struct{}{}
		d.ClickLabel(ZhCN.Retry)
		pro3Wait(t, d, func() bool { return !p.IsLoading() && p.Err() == nil })
	})
}

// ---------------------------------------------------------------------------
// LoginForm countdown announcements

func TestP2ELoginCountdownAnnounce(t *testing.T) {
	d := pro3Driver(func() *theme.Theme { return theme.AntDesign() }, 600, 700, func(c *Context) {
		ConfigProvider(c).Locale(EnUS).Body(func() { LoginForm(c).MobileOnly().CodeInterval(3).Show() })
	})
	d.EnableAccessibility()
	d.Frame()
	eds := pro3TextEditors(d)
	d.Click(eds[0].Center())
	d.Type("13800000000")
	var got []string
	collect := func() {
		for _, a := range d.AccessibilityTree().Announcements {
			got = append(got, a.Text)
		}
	}
	b := mustFind(t, d, "Get code")
	d.Inject(input.Pointer{Kind: input.Move, Pos: b.Center()})
	d.Inject(input.Pointer{Kind: input.Press, Pos: b.Center()})
	d.Inject(input.Pointer{Kind: input.Release, Pos: b.Center()})
	d.Frame()
	collect()
	for range 5 {
		d.Advance(time.Second)
		d.Frame()
		collect()
	}
	var codes []string
	for _, s := range got {
		if strings.Contains(strings.ToLower(s), "code") {
			codes = append(codes, s)
		}
	}
	if len(codes) != 2 || !strings.Contains(codes[0], "3 seconds") || !strings.Contains(codes[1], "new code") {
		t.Fatalf("announcements: %q", got)
	}
}

// ---------------------------------------------------------------------------
// DropZone

func TestDropZoneHoverAndDrop(t *testing.T) {
	pro3Themes(t, func(t *testing.T, th func() *theme.Theme) {
		var outer, inner *DropZoneW
		var innerOver bool
		var dropped []string
		d := pro3Driver(th, 600, 400, func(c *Context) {
			outer = DropZone(c, func(bool) {
				Text(c, "outer")
				inner = DropZone(c, func(over bool) {
					innerOver = over
					Text(c, "inner area")
				}).A11yLabel("Inner").Layout()
				if p := inner.Dropped(); p != nil {
					dropped = p
				}
			}).A11yLabel("Outer")
		})
		ir, _ := d.FindText("inner area")
		or, _ := d.FindText("outer")
		if outer.Over() || inner.Over() {
			t.Fatal("over without a drag")
		}
		d.FileDragOver(ir.Center())
		if !inner.Over() || !innerOver || outer.Over() {
			t.Fatalf("inner drag: inner %v outer %v", inner.Over(), outer.Over())
		}
		if p, ok := d.r.c.FileDrag(); !ok || p != ir.Center() {
			t.Fatal("FileDrag")
		}
		// The highlight is painted (tinted pixels inside the zone).
		// The highlight tints the zone (it spans the window width; the
		// text sits on the left).
		img := d.Image()
		if px := img.RGBAAt(int(ir.Right()+20), int(ir.Center().Y)); px.R == 255 && px.G == 255 && px.B == 255 {
			t.Fatalf("no highlight: %v", px)
		}
		d.FileDragOver(or.Center())
		if inner.Over() || !outer.Over() {
			t.Fatal("outer drag")
		}
		d.FileDragLeave()
		if inner.Over() || outer.Over() {
			t.Fatal("leave")
		}
		if _, ok := d.r.c.FileDrag(); ok {
			t.Fatal("FileDrag after leave")
		}
		d.DropFiles(ir.Center(), "/tmp/a.txt", "/tmp/b.png")
		if !slices.Equal(dropped, []string{"/tmp/a.txt", "/tmp/b.png"}) {
			t.Fatalf("dropped %v", dropped)
		}
		dropped = nil
		d.Settle()
		if dropped != nil {
			t.Fatal("drop delivered twice")
		}
		// Idle windows keep no state.
		if fileDragOf(d.r.c, false) != nil {
			t.Fatal("state kept")
		}
	})
}

func TestDropZoneA11yAnnounce(t *testing.T) {
	d := pro3Driver(func() *theme.Theme { return theme.AntDesign() }, 400, 300, func(c *Context) {
		ConfigProvider(c).Locale(EnUS).Body(func() {
			DropZone(c, func(bool) { Text(c, "drop here") }).A11yLabel("Attachments")
		})
	})
	d.EnableAccessibility()
	d.Frame()
	mustNode(t, d, a11y.RoleGroup, "Attachments")
	r, _ := d.FindText("drop here")
	d.r.FileDragOver(r.Center())
	d.Frame()
	var said []string
	for _, a := range d.AccessibilityTree().Announcements {
		said = append(said, a.Text)
	}
	d.r.FileDragOver(geom.Pt(r.Center().X+1, r.Center().Y))
	d.Frame()
	for _, a := range d.AccessibilityTree().Announcements {
		said = append(said, a.Text)
	}
	if len(said) != 1 || !strings.Contains(said[0], "Release") {
		t.Fatalf("announcements %q", said)
	}
}
