package ui

import (
	"github.com/tangoui/tango/ui/icon"
)

// TableDropdownW is a row-action dropdown backed by DropdownW. Menu keys,
// disabled items, focus navigation and dismissal use the same menu model.
type TableDropdownW struct {
	_        [0]func()
	c        *Context
	d        *DropdownW
	label    string
	button   bool
	onSelect func(string)
}

// TableDropdown starts a table-row action menu. Show uses an ellipsis
// trigger with a localized accessible name. Button selects the ordinary
// button variant (not a split button). ConfigProvider.Defaults keys:
// "disabled" (false), "trigger" (TriggerHover), "placement" (BottomStart).
func TableDropdown(c *Context, items []MenuItem) *TableDropdownW {
	d := Dropdown(c, items)
	d.Disabled(Prop(c, "TableDropdown", "disabled", false))
	d.Trigger(Prop(c, "TableDropdown", "trigger", TriggerHover))
	d.Placement(Prop(c, "TableDropdown", "placement", BottomStart))
	return &TableDropdownW{c: c, d: d}
}

// Label replaces the ellipsis with text.
func (w *TableDropdownW) Label(s string) *TableDropdownW { w.label = s; return w }

// Button uses an ordinary button with a down arrow, as TableDropdown.Button.
func (w *TableDropdownW) Button(label string) *TableDropdownW {
	w.label = label
	w.button = true
	return w
}

// Trigger chooses hover, click or context menu activation.
func (w *TableDropdownW) Trigger(t Trigger) *TableDropdownW { w.d.Trigger(t); return w }

// Disabled disables the trigger and menu.
func (w *TableDropdownW) Disabled(on bool) *TableDropdownW { w.d.Disabled(on); return w }

// Open binds the expanded state.
func (w *TableDropdownW) Open(on *bool) *TableDropdownW { w.d.Open(on); return w }

// OnSelect receives the selected menu key.
func (w *TableDropdownW) OnSelect(fn func(string)) *TableDropdownW { w.onSelect = fn; return w }

// Show builds the trigger and menu.
func (w *TableDropdownW) Show() *TableDropdownW {
	c := w.c
	w.d.Body(func() {
		Disabled(c, w.d.disabled, func() {
			if w.button {
				Button(c, w.label).Icon(icon.Down).IconPlacement("end")
			} else if w.label != "" {
				Link(c, w.label)
			} else {
				IconButton(c, icon.More, c.Locale().A11y.MoreActions)
			}
		})
	})
	if key := w.d.Clicked(); key != "" && w.onSelect != nil {
		w.onSelect(key)
	}
	return w
}

// Clicked returns the menu key selected this frame, or an empty string.
func (w *TableDropdownW) Clicked() string { return w.d.Clicked() }

// IsOpen reports whether the menu is expanded.
func (w *TableDropdownW) IsOpen() bool { return w.d.IsOpen() }

// ID returns the trigger wrapper ID after Show.
func (w *TableDropdownW) ID() ID { return w.d.ID() }
