package ui

import (
	"testing"

	"github.com/tangoui/tango/ui/input"
)

// numPage builds an InputNumber plus a button to move focus away.
func numPage(val *float64, empty *bool, mod func(*InputNumberW)) (func(*Context), **InputNumberW) {
	var w *InputNumberW
	return func(c *Context) {
		Column(c, func() {
			w = InputNumber(c, val).Width(200)
			if empty != nil {
				w.Empty(empty)
			}
			if mod != nil {
				mod(w)
			}
			Button(c, "别处")
		})
	}, &w
}

func TestInputNumberEmptyTyping(t *testing.T) {
	v, empty := 0.0, true
	page, w := numPage(&v, &empty, func(w *InputNumberW) { w.Placeholder("0.00").Precision(2) })
	d := driver(page)
	d.Settle()
	if got := (*w).display(); got != "" {
		t.Fatalf("empty field displays %q, want nothing", got)
	}
	d.Click((*w).ed.abs.Center())
	d.Settle()
	if got := (*w).st.buf; got != "" {
		t.Fatalf("editing an empty field starts with %q", got)
	}
	d.Type("88.8")
	d.Settle()
	if v != 88.8 || empty {
		t.Fatalf("typed 88.8 -> v=%v empty=%v", v, empty)
	}
	must(t, d.ClickLabel("别处"))
	d.Settle()
	if got := (*w).display(); got != "88.80" {
		t.Fatalf("after blur displays %q", got)
	}

	// Clearing the text empties the field again.
	d.Click((*w).ed.abs.Center())
	d.Settle()
	for range 5 {
		d.Key(input.KeyBackspace, 0)
	}
	d.Settle()
	if !empty {
		t.Fatal("cleared text did not empty the field")
	}
	must(t, d.ClickLabel("别处"))
	d.Settle()
	if !empty || (*w).display() != "" {
		t.Fatalf("after blur: empty=%v display=%q", empty, (*w).display())
	}
}

func TestInputNumberEmptyStep(t *testing.T) {
	v, empty := 42.0, true
	page, w := numPage(&v, &empty, func(w *InputNumberW) { w.Min(5).Step(10) })
	d := driver(page)
	d.Settle()
	d.Move((*w).box.abs.Center()) // handles appear on hover
	d.Settle()
	d.Click((*w).up.abs.Center())
	d.Settle()
	if empty || v != 15 {
		t.Fatalf("step up from empty -> v=%v empty=%v, want 15 (min 5 + step 10)", v, empty)
	}
}

func TestInputNumberWithoutEmptyKeepsValue(t *testing.T) {
	v := 7.0
	page, w := numPage(&v, nil, nil)
	d := driver(page)
	d.Settle()
	d.Click((*w).ed.abs.Center())
	d.Settle()
	d.Key(input.KeyBackspace, 0)
	d.Settle()
	must(t, d.ClickLabel("别处"))
	d.Settle()
	if v != 7 || (*w).display() != "7" {
		t.Fatalf("cleared non-nullable field: v=%v display=%q", v, (*w).display())
	}
}

func TestInputNumberFormStatus(t *testing.T) {
	v, empty := 0.0, true
	var w *InputNumberW
	var f *FormW
	d := driver(func(c *Context) {
		f = Form(c)
		f.Body(func() {
			f.Item("金额").RulesFunc(func() string {
				if empty {
					return ""
				}
				return "x"
			}, Required("必填")).Body(func() {
				w = InputNumber(c, &v).Empty(&empty)
			})
		})
	})
	d.Settle()
	if w.status != "" {
		t.Fatalf("status before validation %q", w.status)
	}
	f.Validate()
	d.Settle()
	if w.status != "error" {
		t.Fatalf("status after failed validation %q, want error", w.status)
	}
}

func must(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}
