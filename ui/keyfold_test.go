package ui

import (
	"testing"

	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/input"
)

// The keypad Enter must submit inputs like the main Enter key: the runner
// folds keypad duplicates onto the main key and keeps the physical key in
// KeyEvent.Code.
func TestKeypadEnterSubmitsInput(t *testing.T) {
	val := ""
	submitted := 0
	d := NewDriver(func(c *Context) {
		if Input(c, &val).Placeholder("name").Submitted() {
			submitted++
		}
	}, DriverOptions{Width: 400, Height: 200})
	d.Frame()
	d.Click(geom.Point{X: 60, Y: 16}) // the input fills the top row
	d.Frame()
	d.Type("go")
	d.Frame()
	d.Key(input.KeyNumpadEnter, 0)
	d.Frame()
	if submitted != 1 {
		t.Fatalf("keypad Enter submitted %d times, want 1 (value %q)", submitted, val)
	}
}
