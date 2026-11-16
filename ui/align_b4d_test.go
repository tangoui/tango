package ui

import (
	"testing"

	"github.com/tangoui/tango/ui/a11y"
	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/icon"
	"github.com/tangoui/tango/ui/input"
	"github.com/tangoui/tango/ui/theme"
)

// Batch 4 group D (Pro table): CellEditorTable, RowEditorTable,
// TableDropdown, ListToolBar and the ProTable toolbar options.

type b4dRow struct {
	ID, Name, Note string
	Age            float64
}

func b4dCols() []ProColumn[b4dRow] {
	return []ProColumn[b4dRow]{
		{Key: "name", Title: "Name", Width: 220, Value: func(r b4dRow) any { return r.Name },
			Editor: &ProField{Required: true, Validate: func(v any, _ ProValues) string {
				if v == "bad" {
					return "Bad name"
				}
				return ""
			}},
			Set: func(r *b4dRow, v any) { r.Name, _ = v.(string) }},
		{Key: "age", Title: "Age", ValueType: ValueDigit, Width: 160, Value: func(r b4dRow) any { return r.Age },
			Editor: &ProField{Validate: func(v any, _ ProValues) string {
				if f, _ := proNumber(v); f > 150 {
					return "Too old"
				}
				return ""
			}},
			Set: func(r *b4dRow, v any) { r.Age, _ = proNumber(v) }},
		{Key: "note", Title: "Note", Width: 200, Value: func(r b4dRow) any { return r.Note }},
	}
}

func b4dRows() []b4dRow {
	return []b4dRow{{"a", "Alice", "first", 30}, {"b", "Bob", "second", 40}}
}

// b4dDoubleClick double-clicks p.
func b4dDoubleClick(d *Driver, p geom.Point) {
	d.Inject(input.Pointer{Kind: input.Move, Pos: p})
	d.Inject(input.Pointer{Kind: input.Press, Pos: p, Clicks: 1})
	d.Inject(input.Pointer{Kind: input.Release, Pos: p})
	d.Inject(input.Pointer{Kind: input.Press, Pos: p, Clicks: 2})
	d.Inject(input.Pointer{Kind: input.Release, Pos: p})
	d.Settle()
}

// b4dClear deletes n characters before the caret.
func b4dClear(d *Driver, n int) {
	d.Key(input.KeyEnd, 0)
	for range n {
		d.Key(input.KeyBackspace, 0)
	}
}

func b4dAnt() *theme.Theme { return theme.AntDesign() }

func TestCellEditorTableCommitCancelValidate(t *testing.T) {
	pro3Themes(t, func(t *testing.T, th func() *theme.Theme) {
		rows := b4dRows()
		var w *CellEditorTableW[b4dRow]
		saves := 0
		d := pro3Driver(th, 900, 400, func(c *Context) {
			w = CellEditorTable(c, b4dCols(), &rows).RowKey(func(r b4dRow) string { return r.ID }).
				OnSave(func(r b4dRow, i int, done func(error)) { saves++; done(nil) }).Show()
		})
		if !w.e.bordered || w.e.pageSize != 0 {
			t.Fatalf("preset defaults: bordered %v pageSize %d", w.e.bordered, w.e.pageSize)
		}
		if pro3HasText(d, ZhCN.Edit) || pro3HasText(d, ZhCN.EditableTable.Actions) {
			t.Fatal("presets have no action column")
		}
		// A single click does not edit; a double click edits that cell only.
		d.Click(mustFindText(t, d, "Alice").Center())
		if k, _ := w.EditingCell(); k != "" {
			t.Fatal("single click started editing")
		}
		b4dDoubleClick(d, mustFindText(t, d, "Alice").Center())
		if k, col := w.EditingCell(); k != "a" || col != "name" {
			t.Fatalf("editing %q %q", k, col)
		}
		if eds := pro3TextEditors(d); len(eds) != 1 {
			t.Fatalf("cell mode shows %d editors, want 1", len(eds))
		}
		// Focus is in the editor (its text selected, like a tab-in):
		// typing edits the draft; Escape cancels without leaking it.
		d.Key(input.KeyEnd, 0)
		d.Type("X")
		if got := w.e.st.drafts["a"]["name"]; got != "AliceX" {
			t.Fatalf("draft %v", got)
		}
		d.Key(input.KeyEscape, 0)
		if k, _ := w.EditingCell(); k != "" || rows[0].Name != "Alice" || saves != 0 {
			t.Fatalf("escape: editing %q rows %v saves %d", k, rows, saves)
		}
		b4dDoubleClick(d, mustFindText(t, d, "Alice").Center())
		if got := w.e.st.drafts["a"]["name"]; got != "Alice" {
			t.Fatalf("cancelled draft leaked: %v", got)
		}
		// A failing rule keeps the cell open and writes nothing.
		b4dClear(d, 5)
		d.Type("bad")
		d.Key(input.KeyEnter, 0)
		if w.Errors("a")["name"] != "Bad name" || rows[0].Name != "Alice" || saves != 0 {
			t.Fatalf("invalid saved: errs %v rows %v", w.Errors("a"), rows)
		}
		if k, _ := w.EditingCell(); k != "a" || !pro3HasText(d, "Bad name") {
			t.Fatal("editor closed on a validation error")
		}
		// Required runs too.
		b4dClear(d, 3)
		d.Key(input.KeyEnter, 0)
		if w.Errors("a")["name"] == "" || rows[0].Name != "Alice" {
			t.Fatalf("required: %v", w.Errors("a"))
		}
		// Enter saves a valid value.
		d.Type("Ann")
		d.Key(input.KeyEnter, 0)
		if k, _ := w.EditingCell(); k != "" || rows[0].Name != "Ann" || saves != 1 || !w.Saved() && !pro3HasText(d, "Ann") {
			t.Fatalf("enter: editing %q rows %v saves %d", k, rows, saves)
		}
		// Only the edited column is validated: an out-of-range age on
		// another row does not block editing its name.
		rows[1].Age = 999
		b4dDoubleClick(d, mustFindText(t, d, "Bob").Center())
		d.Key(input.KeyEnd, 0)
		d.Type("!")
		// Blur saves: click the read-only note cell of the first row.
		d.Click(mustFindText(t, d, "first").Center())
		if k, _ := w.EditingCell(); k != "" || rows[1].Name != "Bob!" || saves != 2 {
			t.Fatalf("blur: editing %q rows %v saves %d", k, rows, saves)
		}
		// A column without an editor does not edit.
		b4dDoubleClick(d, mustFindText(t, d, "first").Center())
		if k, _ := w.EditingCell(); k != "" {
			t.Fatal("read-only column edited")
		}
	})
}

func TestCellEditorTableKeyboardAndBlurValidation(t *testing.T) {
	rows := b4dRows()
	var w *CellEditorTableW[b4dRow]
	d := pro3Driver(b4dAnt, 900, 400, func(c *Context) {
		w = CellEditorTable(c, b4dCols(), &rows).RowKey(func(r b4dRow) string { return r.ID }).
			RowEditable(func(r b4dRow) bool { return r.ID != "b" }).Show()
	})
	b4dDoubleClick(d, mustFindText(t, d, "Alice").Center())
	b4dClear(d, 5)
	d.Type("bad")
	// Blur with an invalid value: nothing is written and the cell stays
	// in edit mode with its error.
	d.Click(mustFindText(t, d, "first").Center())
	if k, _ := w.EditingCell(); k != "a" || rows[0].Name != "Alice" || w.Errors("a")["name"] != "Bad name" {
		t.Fatalf("blur invalid: editing %q rows %v errs %v", k, rows, w.Errors("a"))
	}
	// Switching to another cell is refused while the open one is invalid.
	b4dDoubleClick(d, mustFindText(t, d, "30").Center())
	if k, col := w.EditingCell(); k != "a" || col != "name" {
		t.Fatalf("switched away from an invalid cell: %q %q", k, col)
	}
	w.Cancel()
	d.Settle()
	if k, _ := w.EditingCell(); k != "" || rows[0].Name != "Alice" {
		t.Fatal("Cancel")
	}
	// Keyboard: Enter on the active cell (the table has focus after the
	// click) starts editing it; Enter in the editor saves.
	d.Click(mustFindText(t, d, "30").Center())
	d.Key(input.KeyEnter, 0)
	if k, col := w.EditingCell(); k != "a" || col != "age" {
		t.Fatalf("enter on cell: %q %q", k, col)
	}
	b4dClear(d, 1)
	d.Type("5")
	d.Key(input.KeyEnter, 0)
	if k, _ := w.EditingCell(); k != "" || rows[0].Age != 35 {
		t.Fatalf("keyboard save: %q %v", k, rows)
	}
	// RowEditable: the second row never edits.
	b4dDoubleClick(d, mustFindText(t, d, "Bob").Center())
	if k, _ := w.EditingCell(); k != "" {
		t.Fatal("RowEditable ignored")
	}
}

func TestRowEditorTableCommitCancelValidate(t *testing.T) {
	pro3Themes(t, func(t *testing.T, th func() *theme.Theme) {
		rows := b4dRows()
		var w *RowEditorTableW[b4dRow]
		var saved []b4dRow
		seq := 0
		d := pro3Driver(th, 900, 400, func(c *Context) {
			w = RowEditorTable(c, b4dCols(), &rows).RowKey(func(r b4dRow) string { return r.ID }).
				NewRow(func() b4dRow { seq++; return b4dRow{ID: "n"} }).
				OnSave(func(r b4dRow, i int, done func(error)) { saved = append(saved, r); done(nil) }).Show()
		})
		// Double-clicking any cell edits the whole row.
		b4dDoubleClick(d, mustFindText(t, d, "first").Center())
		if w.EditingKey() != "a" {
			t.Fatalf("editing %q", w.EditingKey())
		}
		if eds := pro3TextEditors(d); len(eds) != 2 {
			t.Fatalf("row mode shows %d editors, want 2 (name, age)", len(eds))
		}
		// Focus is in the first editor; Escape cancels every draft.
		d.Type("X")
		d.Key(input.KeyEscape, 0)
		if w.EditingKey() != "" || rows[0].Name != "Alice" || len(saved) != 0 {
			t.Fatalf("escape: %q %v", w.EditingKey(), rows)
		}
		b4dDoubleClick(d, mustFindText(t, d, "Alice").Center())
		if got := w.e.st.drafts["a"]["name"]; got != "Alice" {
			t.Fatalf("cancelled draft leaked: %v", got)
		}
		// Every editor of the row is validated: a bad age blocks the save.
		eds := pro3TextEditors(d)
		d.Click(eds[1].Center())
		d.Key(input.KeyEnd, 0)
		d.Type("0")
		d.Key(input.KeyEnter, 0)
		if w.Errors("a")["age"] != "Too old" || rows[0].Age != 30 || len(saved) != 0 || w.EditingKey() != "a" {
			t.Fatalf("invalid row saved: errs %v rows %v", w.Errors("a"), rows)
		}
		// Tab between the row's editors keeps the row in edit mode.
		b4dClear(d, 3)
		d.Type("31")
		d.Key(input.KeyTab, input.ModShift)
		if w.EditingKey() != "a" {
			t.Fatal("moving between editors of the row left edit mode")
		}
		d.Key(input.KeyEnd, 0)
		d.Type("!")
		// Blur (focus leaves the row) saves both cells.
		d.Click(mustFindText(t, d, "second").Center())
		if w.EditingKey() != "" || rows[0].Name != "Alice!" || rows[0].Age != 31 || len(saved) != 1 {
			t.Fatalf("blur save: %q rows %v saved %v", w.EditingKey(), rows, saved)
		}
		// A new row opens in edit mode; Escape drops it.
		if err := d.ClickLabel(ZhCN.Form.AddRow); err != nil {
			t.Fatal(err)
		}
		if w.EditingKey() == "" {
			t.Fatal("new row not editing")
		}
		d.Settle()
		d.Key(input.KeyEscape, 0)
		if w.EditingKey() != "" || len(rows) != 2 || len(w.e.st.newOrder) != 0 {
			t.Fatalf("new row escape: %q %v", w.EditingKey(), rows)
		}
	})
}

func TestRowEditorTableDisabled(t *testing.T) {
	rows := b4dRows()
	var w *RowEditorTableW[b4dRow]
	d := pro3Driver(b4dAnt, 900, 400, func(c *Context) {
		Disabled(c, true, func() {
			w = RowEditorTable(c, b4dCols(), &rows).RowKey(func(r b4dRow) string { return r.ID }).Show()
		})
	})
	b4dDoubleClick(d, mustFindText(t, d, "Alice").Center())
	if w.EditingKey() != "" {
		t.Fatal("disabled table entered edit mode")
	}
}

func TestTableDropdownKeyboardDisabled(t *testing.T) {
	var w *TableDropdownW
	selected := ""
	disabled := false
	d := pro3Driver(b4dAnt, 700, 400, func(c *Context) {
		w = TableDropdown(c, []MenuItem{{Key: "blocked", Label: "Blocked", Disabled: true}, {Key: "edit", Label: "Edit row"}, {Key: "delete", Label: "Delete row"}}).Disabled(disabled).OnSelect(func(k string) { selected = k }).Show()
	})
	d.EnableAccessibility()
	n := mustNode(t, d, a11y.RoleButton, ZhCN.A11y.MoreActions)
	if err := d.PerformAction(n.ID, a11y.Focus, ""); err != nil {
		t.Fatal(err)
	}
	d.Key(input.KeyDown, 0)
	if !w.IsOpen() {
		t.Fatal("keyboard did not open")
	}
	d.Key(input.KeyEnter, 0)
	if selected != "edit" || w.IsOpen() {
		t.Fatalf("selected %q open %v", selected, w.IsOpen())
	}
	d.Key(input.KeyDown, 0)
	d.Key(input.KeyEscape, 0)
	if w.IsOpen() {
		t.Fatal("escape did not close")
	}
	disabled = true
	d.Settle()
	d.Click(mustFind(t, d, ZhCN.A11y.MoreActions).Center())
	if w.IsOpen() {
		t.Fatal("disabled opened")
	}
}

func TestListToolBarComposition(t *testing.T) {
	var w *ListToolBarW
	keyword, submitted, active := "", "", "all"
	actions, settings, filter := 0, 0, false
	d := pro3Driver(b4dAnt, 1000, 500, func(c *Context) {
		w = ListToolBar(c).Title("People").SubTitle("2 records").Tooltip("Table help").Search(&keyword).OnSearch(func(s string) { submitted = s }).
			Filter(func() { Checkbox(c, "Active only", &filter) }).
			Actions(func() {
				if Button(c, "Create").Clicked() {
					actions++
				}
			}).
			Settings(func() {
				if IconButton(c, icon.Reload, ZhCN.ProTable.Reload).Clicked() {
					settings++
				}
			}).
			MultipleLine(true).Tabs(&active, []TabItem{{Key: "all", Label: "All"}, {Key: "recent", Label: "Recent"}}).Show()
	})
	d.EnableAccessibility()
	mustNode(t, d, a11y.RoleToolbar, "People")
	d.Click(mustNodeRect(t, d, a11y.RoleSearchbox, ZhCN.Search).Center())
	d.Type("Ann")
	d.Key(input.KeyEnter, 0)
	if submitted != "Ann" {
		t.Fatalf("search %q", submitted)
	}
	d.ClickLabel("Active only")
	d.ClickLabel("Create")
	d.ClickLabel(ZhCN.ProTable.Reload)
	d.ClickLabel("Recent")
	if !filter || actions != 1 || settings != 1 || active != "recent" {
		t.Fatalf("composition filter %v action %d settings %d active %s", filter, actions, settings, active)
	}
	if w.ID() == 0 {
		t.Fatal("missing ID")
	}
}

func mustNodeRect(t *testing.T, d *Driver, r a11y.Role, name string) geom.Rect {
	t.Helper()
	n := mustNode(t, d, r, name)
	return geom.Rect{X: n.Bounds.X, Y: n.Bounds.Y, W: n.Bounds.W, H: n.Bounds.H}
}

func TestListToolBarProTableSettings(t *testing.T) {
	for _, th := range []func() *theme.Theme{b4dAnt, func() *theme.Theme { return theme.Shadcn() }} {
		rows := b4dRows()
		var p *ProTableW[b4dRow]
		keyword := ""
		filter := false
		requests := 0
		d := pro3Driver(th, 1100, 650, func(c *Context) {
			p = ProTable(c, b4dCols()).Title("People").NoSearch().FullScreen(true).
				Toolbar(func(w *ListToolBarW) {
					w.SubTitle("Records").Search(&keyword).OnSearch(func(string) { p.Reload() }).Filter(func() { Checkbox(c, "Active only", &filter) })
				}).
				Request(func(_ ProRequest, done func(ProResult[b4dRow])) {
					requests++
					done(ProResult[b4dRow]{Rows: rows, Total: len(rows)})
				}).Show()
		})
		d.EnableAccessibility()
		mustNode(t, d, a11y.RoleToolbar, "People")
		if err := d.ClickLabel(ZhCN.ProTable.Reload); err != nil {
			t.Fatal(err)
		}
		if requests < 2 {
			t.Fatal("refresh did not request")
		}
		d.ClickLabel(ZhCN.ProTable.Density)
		d.Key(input.KeyEnd, 0)
		d.Key(input.KeyEnter, 0)
		if p.density() != Small {
			t.Fatalf("density %v", p.density())
		}
		d.ClickLabel(ZhCN.ProTable.ColumnSetting)
		d.ClickLabel("Age")
		if !p.colState().Hidden["age"] {
			t.Fatal("column settings not shared")
		}
		d.Key(input.KeyEscape, 0)
		d.ClickLabel(ZhCN.Fullscreen)
		if !p.st.full {
			t.Fatal("fullscreen did not toggle")
		}
		d.ClickLabel(ZhCN.ExitFullscreen)
		if p.st.full {
			t.Fatal("fullscreen did not close")
		}
		d.ClickLabel("Active only")
		if !filter {
			t.Fatal("filter")
		}
	}
}

func TestProTableFullscreenV3Default(t *testing.T) {
	d := pro3Driver(b4dAnt, 900, 500, func(c *Context) { ProTable(c, b4dCols()).NoSearch().Show() })
	d.EnableAccessibility()
	if _, ok := d.A11yFind(a11y.RoleButton, ZhCN.Fullscreen); ok {
		t.Fatal("v3 defaults fullScreen to false")
	}
}

func TestCellEditorTableAsyncCancelAndProgrammaticRestrictions(t *testing.T) {
	rows := b4dRows()
	var w *CellEditorTableW[b4dRow]
	var done func(string)
	disabled := false
	d := pro3Driver(b4dAnt, 900, 500, func(c *Context) {
		cols := b4dCols()
		cols[0].Editor.AsyncValidate = func(_ any, cb func(string)) { done = cb }
		Disabled(c, disabled, func() {
			w = CellEditorTable(c, cols, &rows).RowKey(func(r b4dRow) string { return r.ID }).RowEditable(func(r b4dRow) bool { return r.ID != "b" }).Show()
		})
	})
	w.Edit("b", "name")
	d.Settle()
	if k, _ := w.EditingCell(); k != "" {
		t.Fatal("Edit ignored RowEditable")
	}
	w.Edit("a", "name")
	d.Settle()
	d.Type("Ann")
	d.Key(input.KeyEnter, 0)
	if !w.Validating("a") || rows[0].Name != "Alice" {
		t.Fatal("pending validator committed")
	}
	w.Cancel()
	done("")
	d.Settle()
	if k, _ := w.EditingCell(); k != "" || rows[0].Name != "Alice" {
		t.Fatal("late validator leaked cancelled draft")
	}
	disabled = true
	d.Settle()
	w.Edit("a", "name")
	d.Settle()
	if k, _ := w.EditingCell(); k != "" {
		t.Fatal("disabled programmatic Edit")
	}
}

func TestRowEditorTableProgrammaticRestrictions(t *testing.T) {
	rows := b4dRows()
	var w *RowEditorTableW[b4dRow]
	d := pro3Driver(b4dAnt, 900, 400, func(c *Context) {
		w = RowEditorTable(c, b4dCols(), &rows).RowKey(func(r b4dRow) string { return r.ID }).RowEditable(func(r b4dRow) bool { return r.ID != "b" }).Show()
	})
	w.Edit("b")
	d.Settle()
	if w.EditingKey() != "" {
		t.Fatal("Edit ignored RowEditable")
	}
	w.Edit("a")
	d.Settle()
	d.Type("bad")
	d.Key(input.KeyEnter, 0)
	if w.EditingKey() != "a" || rows[0].Name != "Alice" {
		t.Fatal("row validation")
	}
	w.Cancel()
	d.Settle()
	w.Edit("a")
	d.Settle()
	if w.e.st.drafts["a"]["name"] != "Alice" {
		t.Fatal("row cancelled draft leaked")
	}
}

func TestCellEditorTablePendingEscape(t *testing.T) {
	for _, save := range []bool{false, true} {
		rows := b4dRows()
		var w *CellEditorTableW[b4dRow]
		var finish func()
		d := pro3Driver(b4dAnt, 900, 400, func(c *Context) {
			cols := b4dCols()
			if !save {
				cols[0].Editor.AsyncValidate = func(_ any, done func(string)) { finish = func() { done("") } }
			}
			w = CellEditorTable(c, cols, &rows).RowKey(func(r b4dRow) string { return r.ID })
			if save {
				w.OnSave(func(_ b4dRow, _ int, done func(error)) { finish = func() { done(nil) } })
			}
			w.Show()
		})
		b4dDoubleClick(d, mustFindText(t, d, "Alice").Center())
		d.Type("Ann")
		d.Key(input.KeyEnter, 0)
		d.Settle()
		if !w.Validating("a") {
			t.Fatal("not pending")
		}
		d.Key(input.KeyEscape, 0)
		if k, _ := w.EditingCell(); k != "" {
			t.Fatalf("pending (save=%v) could not cancel with Escape", save)
		}
		finish()
		d.Settle()
		if rows[0].Name != "Alice" {
			t.Fatal("late success committed cancelled row")
		}
	}
}

func TestCellEditorTableSelectEnterDoesNotSubmit(t *testing.T) {
	rows := b4dRows()
	var w *CellEditorTableW[b4dRow]
	d := pro3Driver(b4dAnt, 900, 400, func(c *Context) {
		cols := b4dCols()
		cols[0].ValueType = ValueSelect
		cols[0].ValueEnum = []ProEnum{{Value: "Alice", Label: "Alice"}, {Value: "Ann", Label: "Ann"}}
		w = CellEditorTable(c, cols, &rows).RowKey(func(r b4dRow) string { return r.ID }).Show()
	})
	w.Edit("a", "name")
	d.Settle()
	d.Key(input.KeyEnter, 0)
	if k, _ := w.EditingCell(); k != "a" {
		t.Fatal("Enter opening Select submitted the cell")
	}
	d.Key(input.KeyDown, 0)
	d.Key(input.KeyEnter, 0)
	if k, _ := w.EditingCell(); k != "a" || rows[0].Name != "Alice" {
		t.Fatal("Select selection submitted instead of keeping draft")
	}
	w.Save()
	d.Settle()
	if rows[0].Name != "Ann" {
		t.Fatalf("selected value not saved: %v", rows)
	}
}

func TestTableFilterResetOnlyDraftV3(t *testing.T) {
	for _, toDefault := range []bool{true, false} {
		filters := map[string][]string{"a": {"y"}}
		var tb *TableW
		vals := []string{"x", "y"}
		cols := []TableColumn{{Key: "a", Title: "A", Width: 200, Text: func(i int) string { return vals[i] }, Filters: []Option{{Label: "X", Value: "x"}, {Label: "Y", Value: "y"}}, FilterSearch: true, DefaultFilter: []string{"x"}, FilterResetToDefault: toDefault, OnFilter: func(i int, v string) bool { return vals[i] == v }}}
		d := pro3Driver(b4dAnt, 700, 450, func(c *Context) { tb = Table(c, cols, 2).Filters(&filters).Show() })
		d.Click(g6Rect(t, d, "a").Center())
		eds := pro3TextEditors(d)
		if len(eds) == 0 {
			t.Fatal("filter search missing")
		}
		d.Click(eds[0].Center())
		d.Type("Y")
		if tb.st.fQuery != "Y" {
			t.Fatal("search not set")
		}
		d.ClickLabel(ZhCN.FilterReset)
		if tb.st.fOpen == "" || tb.st.fQuery != "" {
			t.Fatalf("reset closed panel or retained search: %q %q", tb.st.fOpen, tb.st.fQuery)
		}
		if len(filters["a"]) != 1 || filters["a"][0] != "y" || len(tb.DisplayedRows()) != 1 || tb.DisplayedRows()[0] != 1 {
			t.Fatalf("reset committed: %v rows %v", filters, tb.DisplayedRows())
		}
		if toDefault {
			if len(tb.st.fDraft) != 1 || tb.st.fDraft[0] != "x" {
				t.Fatalf("default draft %v", tb.st.fDraft)
			}
		} else if len(tb.st.fDraft) != 0 {
			t.Fatalf("clear draft %v", tb.st.fDraft)
		}
		d.ClickLabel(ZhCN.FilterConfirm)
		want := 2
		if toDefault {
			want = 1
		}
		if tb.st.fOpen != "" || len(tb.DisplayedRows()) != want {
			t.Fatalf("confirm: %q %v", tb.st.fOpen, tb.DisplayedRows())
		}
	}
}

func TestTableFilterResetRestoresDefaultFromEmptyDraft(t *testing.T) {
	var tb *TableW
	cols := []TableColumn{{Key: "a", Title: "A", Width: 180, Text: func(int) string { return "x" }, Filters: []Option{{Label: "X", Value: "x"}}, DefaultFilter: []string{"x"}, FilterResetToDefault: true}}
	d := pro3Driver(b4dAnt, 700, 450, func(c *Context) { tb = Table(c, cols, 1).Show() })
	d.Click(g6Rect(t, d, "a").Center())
	d.ClickLabel("X")
	if len(tb.st.fDraft) != 0 {
		t.Fatal("deselect failed")
	}
	d.ClickLabel(ZhCN.FilterReset)
	if len(tb.st.fDraft) != 1 || tb.st.fDraft[0] != "x" {
		t.Fatal("Reset to default disabled for empty draft")
	}
}
