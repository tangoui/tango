package main

import (
	"fmt"

	"github.com/tangoui/tango/examples/ui-gallery/gallery"
	"github.com/tangoui/tango/ui"
	"github.com/tangoui/tango/ui/layout"
)

type b4dPerson struct {
	ID, Name string
	Age      float64
}

func b4dPersonColumns() []ui.ProColumn[b4dPerson] {
	return []ui.ProColumn[b4dPerson]{
		{Key: "name", Title: "Name", Width: 260, Value: func(r b4dPerson) any { return r.Name }, Editor: &ui.ProField{Required: true}, Set: func(r *b4dPerson, v any) { r.Name, _ = v.(string) }},
		{Key: "age", Title: "Age", ValueType: ui.ValueDigit, Width: 160, Value: func(r b4dPerson) any { return r.Age }, Editor: &ui.ProField{Validate: func(v any, _ ui.ProValues) string {
			if n, ok := v.(float64); ok && n > 150 {
				return "Age must be at most 150"
			}
			return ""
		}}, Set: func(r *b4dPerson, v any) { r.Age, _ = v.(float64) }},
	}
}

func init() {
	cellRows := []b4dPerson{{"a", "Alice", 30}, {"b", "Bob", 40}}
	rowRows := append([]b4dPerson(nil), cellRows...)
	gallery.Register(gallery.Page{Name: "CellEditorTable", Title: "CellEditorTable · Cell editing", Group: gallery.Display, Parent: "EditableProTable", Order: 110, Height: 330, Render: func(c *ui.Context) {
		ui.Text(c, "Double-click a cell or press Enter; Enter saves, Escape cancels.")
		ui.CellEditorTable(c, b4dPersonColumns(), &cellRows).Title("Cell editing").RowKey(func(r b4dPerson) string { return r.ID }).Show()
	}})
	gallery.Register(gallery.Page{Name: "RowEditorTable", Title: "RowEditorTable · Row editing", Group: gallery.Display, Parent: "EditableProTable", Order: 111, Height: 330, Render: func(c *ui.Context) {
		ui.Text(c, "Double-click a row. Tab moves between its fields; leaving saves.")
		ui.RowEditorTable(c, b4dPersonColumns(), &rowRows).Title("Row editing").RowKey(func(r b4dPerson) string { return r.ID }).Show()
	}})
	choice := "None"
	gallery.Register(gallery.Page{Name: "TableDropdown", Title: "TableDropdown · Row actions", Group: gallery.Display, Parent: "ProTable", Order: 112, Height: 250, Render: func(c *ui.Context) {
		items := []ui.MenuItem{{Key: "edit", Label: "Edit row"}, {Key: "duplicate", Label: "Duplicate row"}, {Key: "delete", Label: "Delete row", Disabled: true}}
		ui.Row(c, func() {
			ui.Text(c, "Alice")
			ui.TableDropdown(c, items).OnSelect(func(k string) { choice = k }).Show()
			ui.TableDropdown(c, items).Button("Actions").Trigger(ui.TriggerClick).OnSelect(func(k string) { choice = k }).Show()
		}, layout.Middle(), layout.Gap(16))
		ui.Text(c, "Selected: "+choice)
	}})
	keyword, active := "", "all"
	onlyActive := false
	refresh := 0
	gallery.Register(gallery.Page{Name: "ListToolBar", Title: "ListToolBar · Search, filters and table settings", Group: gallery.Display, Parent: "ProTable", Order: 113, Height: 460, Render: func(c *ui.Context) {
		ui.ProTable(c, b4dPersonColumns()).Title("People").NoSearch().FullScreen(true).
			ToolBar(func() { ui.Button(c, "Create") }).
			Toolbar(func(w *ui.ListToolBarW) {
				w.SubTitle("List composition").Tooltip("Search and filters compose with the table settings").Search(&keyword).MultipleLine(true).
					Filter(func() { ui.Checkbox(c, "Active only", &onlyActive) }).Tabs(&active, []ui.TabItem{{Key: "all", Label: "All"}, {Key: "recent", Label: "Recent"}})
			}).
			Request(func(_ ui.ProRequest, done func(ui.ProResult[b4dPerson])) {
				refresh++
				done(ui.ProResult[b4dPerson]{Rows: cellRows, Total: len(cellRows)})
			}).Show()
		ui.Text(c, fmt.Sprintf("Keyword: %s · Active: %s · Requests: %d", keyword, active, refresh))
	}})
}
