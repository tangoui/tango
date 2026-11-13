package ui

import (
	"strings"
	"testing"

	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/icon"
	"github.com/tangoui/tango/ui/layout"
	"github.com/tangoui/tango/ui/paint"
)

// iconCheckPage draws the same directional icon four ways: with icon.Draw
// (never mirrors: the bug the check finds), with c.paintIcon (mirrors in
// RTL), with icon.Draw inside an LTR scope and with icon.Draw in a
// Physical() box.
func iconCheckPage(c *Context) {
	labelled := func(label string, draw func(l *paint.List, r geom.Rect), opts ...layout.Option) {
		b := Row(c, func() {}, append([]layout.Option{layout.W(24), layout.H(24)}, opts...)...)
		b.n.label = label
		b.Paint(draw)
	}
	Column(c, func() {
		labelled("bad", func(l *paint.List, r geom.Rect) { icon.Draw(l, icon.Right, r, c.Theme.Text, 0) })
		labelled("good", func(l *paint.List, r geom.Rect) { c.paintIcon(l, icon.Right, r, c.Theme.Text, 0) })
		labelled("symmetric", func(l *paint.List, r geom.Rect) { icon.Draw(l, icon.Search, r, c.Theme.Text, 0) })
		ConfigProvider(c).Direction(LTR).Body(func() {
			labelled("ltr-scope", func(l *paint.List, r geom.Rect) { icon.Draw(l, icon.Left, r, c.Theme.Text, 0) })
		})
		labelled("physical", func(l *paint.List, r geom.Rect) { icon.Draw(l, icon.ArrowRight, r, c.Theme.Text, 0) }, layout.Physical())
	}, layout.Gap(8))
}

func TestMirrorCheckIcons(t *testing.T) {
	d := NewDriver(iconCheckPage, DriverOptions{Width: 300, Height: 300})
	rep := d.MirrorCheck(iconCheckPage)
	if len(rep.Icons) != 1 {
		t.Fatalf("want exactly the icon.Draw icon, got:\n%s", rep)
	}
	v := rep.Icons[0]
	if v.Icon != "right" || v.Label != "bad" {
		t.Fatalf("violation %+v", v)
	}
	if rep.IconsSkipped != 1 {
		t.Fatalf("Physical() icon not skipped (%d):\n%s", rep.IconsSkipped, rep)
	}
	if rep.OK() || !strings.Contains(rep.String(), `icon "right" unmirrored in "bad"`) {
		t.Fatalf("report:\n%s", rep)
	}
	// The recorder is gone after the check: the driver's list is clean.
	if d.r.list.IconNote != nil {
		t.Fatal("IconNote left on the caller's driver")
	}
}

func TestAuditUnmirroredIcon(t *testing.T) {
	rtl := func(c *Context) { ConfigProvider(c).Direction(RTL).Body(func() { iconCheckPage(c) }) }
	d := NewDriver(rtl, DriverOptions{Width: 300, Height: 300})
	rep := d.AuditNoContrast()
	if n := rep.Count(AuditUnmirroredIcon); n != 1 {
		t.Fatalf("unmirrored icons = %d:\n%s", n, rep)
	}
	for _, i := range rep.Issues {
		if i.Kind == AuditUnmirroredIcon && (i.Label != "bad" || !strings.Contains(i.Detail, `"right"`)) {
			t.Fatalf("issue %v", i)
		}
	}
	if d.r.list.IconNote != nil {
		t.Fatal("audit left its recorder installed")
	}
	// Left to right nothing is reported.
	if n := NewDriver(iconCheckPage, DriverOptions{Width: 300, Height: 300}).AuditNoContrast().Count(AuditUnmirroredIcon); n != 0 {
		t.Fatalf("LTR page: %d unmirrored icons", n)
	}
}

func TestIconRecorderKeepsLastFrame(t *testing.T) {
	rtl := func(c *Context) { ConfigProvider(c).Direction(RTL).Body(func() { iconCheckPage(c) }) }
	d := NewDriver(rtl, DriverOptions{Width: 300, Height: 300})
	rec, done := d.recordIcons()
	defer done()
	d.Frame()
	d.Frame()
	if len(rec.notes) != 5 {
		t.Fatalf("notes after two frames = %d, want 5 (one frame)", len(rec.notes))
	}
	want := map[string][2]bool{ // name: directional, mirrored
		"right": {true, false}, "search": {false, false}, "left": {true, false}, "arrow-right": {true, false},
	}
	mirroredRight := 0
	for _, n := range rec.notes {
		if n.name == "right" && n.mirrored {
			mirroredRight++
			continue
		}
		if w, ok := want[n.name]; !ok || n.directional != w[0] || n.mirrored != w[1] {
			t.Errorf("note %+v", n)
		}
		if n.name == "left" && n.rtl {
			t.Errorf("icon in the LTR scope recorded as RTL")
		}
	}
	if mirroredRight != 1 {
		t.Errorf("paintIcon draw not recorded as mirrored")
	}
}
