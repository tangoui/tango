package ui

import (
	"testing"
	"time"

	"github.com/tangoui/tango/ui/a11y"
	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/input"
	"github.com/tangoui/tango/ui/layout"
)

// TestCascaderPanelClickAndKeys: the inline panel shows the columns of
// the value, selects leaves on click without closing, and the keyboard
// moves through the columns from the single Tab stop.
func TestCascaderPanelClickAndKeys(t *testing.T) {
	val := []string{"zj", "hz", "xh"}
	changed := 0
	var p *CascaderPanelW
	d := driver(func(c *Context) {
		Column(c, func() {
			p = CascaderPanel(c, &val, casTestOpts).A11yLabel("地区")
			if p.Changed() {
				changed++
			}
		}, layout.Pad(20))
	})
	d.EnableAccessibility()
	d.Settle()
	// The value's path is expanded: three columns.
	for _, s := range []string{"浙江", "杭州", "西湖"} {
		if _, ok := d.Find(s); !ok {
			t.Fatalf("%s column missing", s)
		}
	}
	if _, ok := d.A11yFind(a11y.RoleGroup, "地区"); !ok {
		t.Fatal("panel group missing")
	}
	d.ClickLabel("宁波")
	if casKey(val) != casKey([]string{"zj", "nb"}) || changed != 1 {
		t.Fatalf("leaf click: %v changed=%d", val, changed)
	}
	if _, ok := d.Find("浙江"); !ok {
		t.Fatal("panel closed after a pick")
	}
	// Clicking a parent only expands (no ChangeOnSelect).
	d.ClickLabel("江苏")
	if casKey(val) != casKey([]string{"zj", "nb"}) {
		t.Fatalf("parent click selected: %v", val)
	}
	if _, ok := d.Find("南京"); !ok {
		t.Fatal("parent did not expand")
	}
	// Keyboard: the click focused the panel; Left back to the first
	// column, Up to 浙江, Right opens 杭州, Right opens 西湖, Enter picks.
	if d.r.c.focus != p.ID() {
		t.Fatal("panel not focused after a click")
	}
	d.Key(input.KeyLeft, 0)
	d.Key(input.KeyUp, 0)
	d.Key(input.KeyRight, 0)
	d.Key(input.KeyRight, 0)
	d.Key(input.KeyEnter, 0)
	if casKey(val) != casKey([]string{"zj", "hz", "xh"}) {
		t.Fatalf("keyboard pick: %v", val)
	}
	// The application changing the value moves the expanded path.
	val = []string{"js", "nj"}
	d.Settle()
	if _, ok := d.Find("南京"); !ok {
		t.Fatal("expanded path did not follow the value")
	}
}

// TestCascaderPanelMultipleAndLazy: checkboxes with the checked strategy,
// and LoadData children loaded on expand.
func TestCascaderPanelMultipleAndLazy(t *testing.T) {
	var vals [][]string
	var p *CascaderPanelW
	d := driver(func(c *Context) {
		p = CascaderPanel(c, nil, casTestOpts).Multiple(&vals)
	})
	d.Settle()
	d.ClickLabel("浙江")
	// Check 杭州 (both leaves) through its checkbox area.
	r := mustFind(t, d, "杭州")
	d.Click(geom.Pt(r.X+16, r.Y+r.H/2))
	if len(vals) != 2 {
		t.Fatalf("checked %v", vals)
	}
	if cp := p.CheckedPaths(); len(cp) != 1 || casKey(cp[0]) != casKey([]string{"zj", "hz"}) {
		t.Fatalf("ShowParent paths %v", cp)
	}

	var val []string
	roots := []CascaderOption{{Label: "远程", Value: "r"}}
	loads := 0
	d2 := driver(func(c *Context) {
		CascaderPanel(c, &val, roots).LoadData(func(path []string, done func([]CascaderOption)) {
			loads++
			done([]CascaderOption{{Label: "子项", Value: "k", IsLeaf: true}})
		})
	})
	d2.Settle()
	d2.ClickLabel("远程")
	d2.Advance(60 * time.Millisecond)
	d2.Settle()
	if loads != 1 {
		t.Fatalf("loads = %d", loads)
	}
	d2.ClickLabel("子项")
	if casKey(val) != casKey([]string{"r", "k"}) {
		t.Fatalf("lazy pick %v", val)
	}
}
