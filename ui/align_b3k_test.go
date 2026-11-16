package ui

// Spec alignment batch 3, group K (overlays and dialogs): behaviour tests
// for the gaps closed against antd 6.7 / Element Plus 2.14.8 / shadcn v4
// (docs/specs/align/3-overlay.md).

import (
	"testing"
	"time"

	"github.com/tangoui/tango/ui/a11y"
	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/input"
	"github.com/tangoui/tango/ui/layout"
	"github.com/tangoui/tango/ui/theme"
)

// b3kPanel returns the rect of the a11y node role/name (the dialog panel).
func b3kPanel(t *testing.T, d *Driver, role a11y.Role, name string) geom.Rect {
	t.Helper()
	return b3kRect(mustNode(t, d, role, name))
}

func b3kRect(n *a11y.Node) geom.Rect {
	b := n.Bounds
	return geom.Rect{X: b.X, Y: b.Y, W: b.W, H: b.H}
}

// ---------------------------------------------------------------------------
// Modal

// TestModalCloseButtonPlacement: the close button is absolutely placed
// over the header: Ant 32×32 at 12 / 12 (modalCloseBtnSize, (56 − 32) /
// 2), Element 48×48 at 0 / 0, shadcn 16×16 at top-4 right-4.
func TestModalCloseButtonPlacement(t *testing.T) {
	for _, tc := range []struct {
		name        string
		th          *theme.Theme
		size, inset float32
	}{
		{"antd", theme.AntDesign(), 32, 12},
		{"element", theme.ElementPlus(), 48, 0},
		{"shadcn", theme.Shadcn(), 16, 16},
	} {
		open := true
		d := NewDriver(func(c *Context) {
			Modal(c, &open, "Dialog").Body(func() { Text(c, "Content") })
		}, DriverOptions{Theme: tc.th, Accessibility: true})
		d.Frame()
		d.Frame()
		panel := b3kPanel(t, d, a11y.RoleDialog, "Dialog")
		cl := b3kRect(mustNode(t, d, a11y.RoleButton, ZhCN.Close))
		if cl.W != tc.size || cl.H != tc.size {
			t.Errorf("%s: close %vx%v, want %v", tc.name, cl.W, cl.H, tc.size)
		}
		if dx, dy := panel.Right()-cl.Right(), cl.Y-panel.Y; abs32(dx-tc.inset) > 0.5 || abs32(dy-tc.inset) > 0.5 {
			t.Errorf("%s: close inset %v / %v, want %v", tc.name, dy, dx, tc.inset)
		}
		title, _ := d.FindText("Dialog")
		if title.Right() > cl.X+0.5 {
			t.Errorf("%s: title %v runs under the close button %v", tc.name, title, cl)
		}
	}
}

// TestModalKeyboard: Escape closes by default; Keyboard(false) and
// Defaults("Modal", {"keyboard": false}) keep the dialog open (antd
// keyboard, Element close-on-press-escape).
func TestModalKeyboard(t *testing.T) {
	for _, mode := range []string{"default", "method", "defaults"} {
		open := true
		d := NewDriver(func(c *Context) {
			body := func() {
				m := Modal(c, &open, "Dialog")
				if mode == "method" {
					m.Keyboard(false)
				}
				m.Body(func() { Button(c, "Inside") })
			}
			if mode == "defaults" {
				ConfigProvider(c).Defaults("Modal", Props{"keyboard": false}).Body(body)
			} else {
				body()
			}
		}, DriverOptions{})
		d.Frame()
		d.Frame()
		d.Key(input.KeyEscape, 0)
		d.Frame()
		if want := mode != "default"; open != want {
			t.Errorf("%s: open after Escape %v, want %v", mode, open, want)
		}
	}
}

// TestModalLoading: Loading shows a skeleton instead of the content and
// hides the footer (antd loading).
func TestModalLoading(t *testing.T) {
	open := true
	d := NewDriver(func(c *Context) {
		Modal(c, &open, "Dialog").Loading(true).Body(func() { Text(c, "Content") })
	}, DriverOptions{})
	d.Frame()
	d.Frame()
	if _, ok := d.FindText("Content"); ok {
		t.Error("content shown while loading")
	}
	if _, ok := d.Find(ZhCN.OK); ok {
		t.Error("footer shown while loading")
	}
}

// TestModalAfterOpenChanged: reported once when the open transition ends
// and once when the close transition ends.
func TestModalAfterOpenChanged(t *testing.T) {
	open := false
	n := 0
	d := NewDriver(func(c *Context) {
		if Modal(c, &open, "Dialog").Body(func() {}).AfterOpenChanged() {
			n++
		}
	}, DriverOptions{Animations: true})
	d.Frame()
	open = true
	for i := 0; i < 30; i++ {
		d.Advance(30 * time.Millisecond)
		d.Frame()
	}
	if n != 1 {
		t.Fatalf("after opening: %d events, want 1", n)
	}
	open = false
	for i := 0; i < 30; i++ {
		d.Advance(30 * time.Millisecond)
		d.Frame()
	}
	if n != 2 {
		t.Errorf("after closing: %d events, want 2", n)
	}
}

// TestModalWidthDefaults: the default width comes from the theme (shadcn
// sm:max-w-lg 512) and never exceeds the window less 2 × 16 (antd
// calc(100vw − 32px), shadcn max-w-[calc(100%−2rem)]).
func TestModalWidthDefaults(t *testing.T) {
	open := true
	page := func(c *Context) { Modal(c, &open, "Dialog").Body(func() {}) }
	d := NewDriver(page, DriverOptions{Theme: theme.Shadcn(), Accessibility: true})
	d.Frame()
	d.Frame()
	if w := b3kPanel(t, d, a11y.RoleDialog, "Dialog").W; w != 512 {
		t.Errorf("shadcn dialog width %v, want 512", w)
	}
	d = NewDriver(page, DriverOptions{Width: 400, Height: 600, Accessibility: true})
	d.Frame()
	d.Frame()
	if w := b3kPanel(t, d, a11y.RoleDialog, "Dialog").W; w != 368 {
		t.Errorf("narrow window: width %v, want 368", w)
	}
}

// ---------------------------------------------------------------------------
// Drawer

// TestDrawerClosablePlacement: Ant puts the close button before the title
// (closable.placement "start"), Element / shadcn after it; the option and
// the Defaults key override the theme.
func TestDrawerClosablePlacement(t *testing.T) {
	for _, tc := range []struct {
		name  string
		th    *theme.Theme
		place string
		props Props
		end   bool
	}{
		{"antd", theme.AntDesign(), "", nil, false},
		{"antd end", theme.AntDesign(), "end", nil, true},
		{"antd defaults", theme.AntDesign(), "", Props{"closablePlacement": "end"}, true},
		{"element", theme.ElementPlus(), "", nil, true},
		{"shadcn start", theme.Shadcn(), "start", nil, false},
	} {
		open := true
		d := NewDriver(func(c *Context) {
			ConfigProvider(c).Defaults("Drawer", tc.props).Body(func() {
				dr := Drawer(c, &open, "Panel")
				if tc.place != "" {
					dr.ClosablePlacement(tc.place)
				}
				dr.Body(func() {})
			})
		}, DriverOptions{Theme: tc.th, Accessibility: true})
		d.Frame()
		d.Frame()
		cl := b3kRect(mustNode(t, d, a11y.RoleButton, ZhCN.Close))
		title, _ := d.FindText("Panel")
		if got := cl.X > title.X; got != tc.end {
			t.Errorf("%s: close after title %v, want %v", tc.name, got, tc.end)
		}
	}
}

// TestDrawerAntCloseButton: the Ant close button is fontSizeLG + paddingXS
// = 24 square with marginXS 8 before the title.
func TestDrawerAntCloseButton(t *testing.T) {
	open := true
	d := NewDriver(func(c *Context) { Drawer(c, &open, "Panel").Body(func() {}) }, DriverOptions{Accessibility: true})
	d.Frame()
	d.Frame()
	cl := b3kRect(mustNode(t, d, a11y.RoleButton, ZhCN.Close))
	title, _ := d.FindText("Panel")
	if cl.W != 24 || cl.H != 24 {
		t.Errorf("close %vx%v, want 24", cl.W, cl.H)
	}
	if gap := title.X - cl.Right(); abs32(gap-8) > 0.5 {
		t.Errorf("close to title %v, want 8", gap)
	}
}

// TestDrawerDefaultSize: shadcn Sheet w-3/4 sm:max-w-sm: 384, at most 75%
// of the window; Ant 378.
func TestDrawerDefaultSize(t *testing.T) {
	for _, tc := range []struct {
		th   *theme.Theme
		w    float32
		want float32
	}{
		{theme.Shadcn(), 1024, 384},
		{theme.Shadcn(), 400, 300},
		{theme.AntDesign(), 1024, 378},
		{theme.ElementPlus(), 1000, 300},
	} {
		open := true
		var dr *DrawerW
		d := NewDriver(func(c *Context) { dr = Drawer(c, &open, "Panel").Body(func() {}) }, DriverOptions{Theme: tc.th, Width: tc.w, Height: 600})
		d.Frame()
		if got := dr.CurrentSize(); got != tc.want {
			t.Errorf("%s at %v: size %v, want %v", tc.th.Flavor, tc.w, got, tc.want)
		}
	}
}

// TestDrawerKeyboardLoadingPush: Keyboard(false) ignores Escape; Loading
// replaces the content; Push(0) keeps the parent in place when a nested
// drawer opens.
func TestDrawerKeyboardLoadingPush(t *testing.T) {
	open, child := true, false
	var parent *DrawerW
	d := NewDriver(func(c *Context) {
		parent = Drawer(c, &open, "Parent").Keyboard(false).Push(0).Body(func() {
			Text(c, "Content")
			Drawer(c, &child, "Child").Loading(true).Body(func() { Text(c, "Child content") })
		})
	}, DriverOptions{Accessibility: true})
	d.Frame()
	d.Frame()
	before, _ := d.FindText("Content")
	d.Key(input.KeyEscape, 0)
	d.Frame()
	if !open || parent.Closed() {
		t.Fatal("Escape closed a drawer with Keyboard(false)")
	}
	child = true
	d.Frame()
	d.Frame()
	if _, ok := d.FindText("Child content"); ok {
		t.Error("child content shown while loading")
	}
	if after, _ := d.FindText("Content"); after.X != before.X {
		t.Errorf("Push(0): parent moved %v -> %v", before.X, after.X)
	}
}

// ---------------------------------------------------------------------------
// Popconfirm

// b3kPopPanel returns the Popconfirm bubble (role alertdialog).
func b3kPopPanel(d *Driver) *node {
	for _, ov := range d.r.c.overlays {
		if n := b2gFindRole(ov.root, a11y.RoleAlertDialog); n != nil {
			return n
		}
	}
	return nil
}

// TestPopconfirmElementDefaults: el-popconfirm opens below (popper
// placement bottom), 150 wide, with a text-type cancel button; Ant opens
// above and fits its content.
func TestPopconfirmElementDefaults(t *testing.T) {
	var p *PopconfirmW
	page := func(c *Context) {
		Column(c, func() {
			p = Popconfirm(c, "Delete?", "", func() { Button(c, "Trigger") })
		}, layout.Pad(200))
	}
	d := NewDriver(page, DriverOptions{Theme: theme.ElementPlus()})
	d.Frame()
	d.ClickLabel("Trigger")
	d.Frame()
	trig, _ := d.Find("Trigger")
	n := b3kPopPanel(d)
	if n == nil {
		t.Fatal("bubble not open")
	}
	if n.abs.Y < trig.Bottom() || n.abs.W != 150 {
		t.Errorf("Element bubble %v (trigger %v), want below and 150 wide", n.abs, trig)
	}
	if p.cancelType != "text" {
		t.Errorf("Element cancel type %q, want text", p.cancelType)
	}
	d = NewDriver(page, DriverOptions{})
	d.Frame()
	d.ClickLabel("Trigger")
	d.Frame()
	trig, _ = d.Find("Trigger")
	if n = b3kPopPanel(d); n == nil || n.abs.Bottom() > trig.Y || n.abs.W == 150 {
		t.Errorf("Ant bubble %v (trigger %v), want above, fitting its content", n.abs, trig)
	}
}

// TestPopconfirmTypesWidthArrow: OkType / CancelType style the buttons,
// Width fixes the width and Arrow(false) leaves the 4px popup gap.
func TestPopconfirmTypesWidthArrow(t *testing.T) {
	d := NewDriver(func(c *Context) {
		Column(c, func() {
			Popconfirm(c, "Delete?", "", func() { Button(c, "Trigger") }).
				OkType("danger").CancelType("link").Width(240).Arrow(false).Placement("bottom")
		}, layout.Pad(200))
	}, DriverOptions{})
	d.Frame()
	d.ClickLabel("Trigger")
	d.Frame()
	trig, _ := d.Find("Trigger")
	n := b3kPopPanel(d)
	if n == nil {
		t.Fatal("bubble not open")
	}
	if n.abs.W != 240 {
		t.Errorf("width %v, want 240", n.abs.W)
	}
	if gap := n.abs.Y - trig.Bottom(); gap != 4 {
		t.Errorf("gap without arrow %v, want 4", gap)
	}
	if n.over != nil {
		t.Error("Arrow(false) still paints an arrow")
	}
	b := NewDriver(func(c *Context) {
		if v := popButtonType(Button(c, "x"), "danger"); !v.danger || v.variant != VariantPrimary {
			t.Errorf("danger type: %v %v", v.danger, v.variant)
		}
		if v := popButtonType(Button(c, "y"), "text"); v.variant != VariantText {
			t.Errorf("text type: %v", v.variant)
		}
	}, DriverOptions{})
	b.Frame()
}

// TestPopconfirmDisabledConfig: ConfigProvider.ComponentDisabled and
// Defaults("Popconfirm", {"disabled": true}) keep the bubble closed, also
// from the keyboard.
func TestPopconfirmDisabledConfig(t *testing.T) {
	for _, mode := range []string{"component", "defaults"} {
		var p *PopconfirmW
		d := NewDriver(func(c *Context) {
			cp := ConfigProvider(c)
			if mode == "component" {
				cp.ComponentDisabled(true)
			} else {
				cp.Defaults("Popconfirm", Props{"disabled": true})
			}
			cp.Body(func() {
				p = Popconfirm(c, "Delete?", "", func() { Text(c, "Trigger") })
			})
		}, DriverOptions{})
		d.Frame()
		r, _ := d.FindText("Trigger")
		d.Click(center(r))
		d.Frame()
		d.Key(input.KeyEnter, 0)
		d.Frame()
		if p.IsOpen() {
			t.Errorf("%s: disabled popconfirm opened", mode)
		}
	}
}

// ---------------------------------------------------------------------------
// Tour

func b3kTourPage(open *bool, cur *int, ids *[3]ID, clicks *int, mod func(t *TourW)) func(c *Context) {
	return func(c *Context) {
		Row(c, func() {
			b := Button(c, "A")
			ids[0] = b.ID()
			if b.Clicked() {
				*clicks++
			}
			ids[1] = Button(c, "B").ID()
			ids[2] = Button(c, "C").ID()
		}, layout.Pad(80), layout.Gap(40))
		t := Tour(c, open, cur, []TourStep{
			{Target: ids[0], Title: "one"},
			{Target: ids[1], Title: "two"},
			{Target: ids[2], Title: "three"},
		})
		if mod != nil {
			mod(t)
		}
		t.Show()
	}
}

// TestTourKeyboardArrows: ←/→ move between steps (rc-tour keyboard);
// Keyboard(false) turns them and Escape off.
func TestTourKeyboardArrows(t *testing.T) {
	open, cur := true, 0
	var ids [3]ID
	n := 0
	d := NewDriver(b3kTourPage(&open, &cur, &ids, &n, nil), DriverOptions{})
	d.Settle()
	d.Key(input.KeyRight, 0)
	d.Frame()
	d.Key(input.KeyRight, 0)
	d.Frame()
	d.Key(input.KeyRight, 0) // past the last step: stays
	d.Frame()
	if cur != 2 {
		t.Fatalf("after →→→ current %d, want 2", cur)
	}
	d.Key(input.KeyLeft, 0)
	d.Frame()
	if cur != 1 {
		t.Fatalf("after ← current %d, want 1", cur)
	}
	open, cur = true, 0
	d = NewDriver(b3kTourPage(&open, &cur, &ids, &n, func(t *TourW) { t.Keyboard(false) }), DriverOptions{})
	d.Settle()
	d.Key(input.KeyRight, 0)
	d.Frame()
	d.Key(input.KeyEscape, 0)
	d.Frame()
	if cur != 0 || !open {
		t.Errorf("Keyboard(false): current %d open %v", cur, open)
	}
}

// TestTourOutOfRange: a current outside the steps shows nothing and keeps
// *open and *current (rc-tour mergedOpen false), without panicking.
func TestTourOutOfRange(t *testing.T) {
	for _, start := range []int{5, -1} {
		open, cur := true, start
		var ids [3]ID
		n := 0
		d := NewDriver(b3kTourPage(&open, &cur, &ids, &n, nil), DriverOptions{Accessibility: true})
		d.Settle()
		if _, ok := d.A11yFind(a11y.RoleDialog, ""); ok {
			t.Errorf("current %d: a tour panel is shown", start)
		}
		if _, ok := d.FindText("one"); ok {
			t.Errorf("current %d: a step is shown", start)
		}
		if !open || cur != start {
			t.Errorf("current %d: open %v current %d changed", start, open, cur)
		}
		// The page stays interactive.
		r, _ := d.Find("B")
		d.Click(center(r))
		d.Frame()
	}
}

// TestTourTargetInteraction: the highlighted target stays clickable through
// the mask hole (antd default, Element target-area-clickable);
// DisabledInteraction blocks it. The page around the hole is blocked.
func TestTourTargetInteraction(t *testing.T) {
	for _, off := range []bool{false, true} {
		open, cur := true, 0
		var ids [3]ID
		n := 0
		d := NewDriver(b3kTourPage(&open, &cur, &ids, &n, func(t *TourW) { t.DisabledInteraction(off) }), DriverOptions{})
		d.Settle()
		r, _ := d.Find("A")
		d.Click(center(r))
		d.Frame()
		if got := n == 1; got == off {
			t.Errorf("DisabledInteraction(%v): target clicks %d", off, n)
		}
	}
	// Without a mask the page is interactive.
	open, cur := true, 0
	var ids [3]ID
	n := 0
	d := NewDriver(b3kTourPage(&open, &cur, &ids, &n, func(t *TourW) { t.NoMask() }), DriverOptions{})
	d.Settle()
	cur = 1 // the hole is around B now; A is outside it
	d.Settle()
	r, _ := d.Find("A")
	d.Click(center(r))
	d.Frame()
	if n != 1 {
		t.Errorf("NoMask: click on the page %d, want 1", n)
	}
	open, cur = true, 1
	n = 0
	d = NewDriver(b3kTourPage(&open, &cur, &ids, &n, nil), DriverOptions{})
	d.Settle()
	r, _ = d.Find("A")
	d.Click(center(r))
	d.Frame()
	if n != 0 {
		t.Errorf("masked page outside the hole received %d clicks", n)
	}
}

// TestTourClosableAndGap: Closable(false) removes the close button and
// Escape; GapRadius / MaskColor / Placement apply.
func TestTourClosableAndGap(t *testing.T) {
	open, cur := true, 0
	var ids [3]ID
	n := 0
	col := theme.RGBA(255, 0, 0, .5)
	d := NewDriver(b3kTourPage(&open, &cur, &ids, &n, func(t *TourW) {
		t.Closable(false).MaskColor(col).GapRadius(0).Placement("right")
	}), DriverOptions{Accessibility: true, Width: 800, Height: 600})
	d.Settle()
	if _, ok := d.A11yFind(a11y.RoleButton, ZhCN.Tour.Close); ok {
		t.Error("close button shown with Closable(false)")
	}
	d.Key(input.KeyEscape, 0)
	d.Frame()
	if !open {
		t.Error("Escape closed a tour without close")
	}
	if t2bCount(d, col) == 0 {
		t.Error("MaskColor not used")
	}
	panel := b3kPanel(t, d, a11y.RoleDialog, "one")
	if a, _ := d.Find("A"); panel.X < a.Right() {
		t.Errorf("Placement(right): panel %v not right of target %v", panel, a)
	}
}

// ---------------------------------------------------------------------------
// HoverCard (Popover hover under shadcn)

// TestHoverCardDelays: under shadcn a hover Popover (HoverCard) opens after
// Radix openDelay 700 ms.
func TestHoverCardDelays(t *testing.T) {
	var p *PopoverW
	d := NewDriver(func(c *Context) {
		p = Popover(c).Text("card")
		p.Body(func() { Button(c, "Trigger") })
	}, DriverOptions{Theme: theme.Shadcn(), Animations: true})
	d.Frame()
	r, _ := d.FindText("Trigger")
	d.Move(center(r))
	d.Frame()
	d.Advance(500 * time.Millisecond)
	d.Frame()
	if p.IsOpen() {
		t.Error("hover card opened before 700 ms")
	}
	d.Advance(250 * time.Millisecond)
	d.Frame()
	d.Frame()
	if !p.IsOpen() {
		t.Error("hover card not open after 700 ms")
	}
}

// ---------------------------------------------------------------------------
// Confirm / MessageBox / Prompt

// TestConfirmAutoFocusAndKeyboard: AutoFocusButton "cancel" focuses Cancel
// (antd autoFocusButton); NoKeyboard keeps Escape from closing.
func TestConfirmAutoFocusAndKeyboard(t *testing.T) {
	var ref *ConfirmRef
	d := NewDriver(func(c *Context) {
		if Button(c, "Open").Clicked() {
			ref = Confirm(c, ConfirmOptions{Title: "Sure?", AutoFocusButton: "cancel", NoKeyboard: true})
		}
	}, DriverOptions{Accessibility: true})
	d.Frame()
	d.ClickLabel("Open")
	d.Frame()
	d.Frame()
	if f := d.A11yFocused(); f == nil || f.Name != ZhCN.Cancel {
		t.Errorf("focus %v, want Cancel", f)
	}
	d.Key(input.KeyEscape, 0)
	d.Frame()
	if !ref.IsOpen() {
		t.Error("Escape closed a NoKeyboard confirm")
	}
}

// TestConfirmAntButtonsMargin: the Ant confirm buttons sit
// confirmBtnsMarginTop (marginSM 12) below the text.
func TestConfirmAntButtonsMargin(t *testing.T) {
	d := NewDriver(func(c *Context) {
		if Button(c, "Open").Clicked() {
			Confirm(c, ConfirmOptions{Title: "Sure?"})
		}
	}, DriverOptions{})
	d.Frame()
	d.ClickLabel("Open")
	d.Frame()
	d.Frame()
	title, _ := d.FindText("Sure?")
	ok, _ := d.Find(ZhCN.OK)
	if gap := ok.Y - title.Bottom(); abs32(gap-12) > 1 {
		t.Errorf("title to buttons %v, want 12", gap)
	}
}

// ---------------------------------------------------------------------------
// Stacked overlays (Review Focus 2) and a11y (Review Focus 5)

// TestOverlayEscTopmostModalPopconfirm: a Popconfirm opened inside a Modal
// closes first on Escape and gives focus back to its trigger; the next
// Escape closes the Modal and focus returns to the page opener.
func TestOverlayEscTopmostModalPopconfirm(t *testing.T) {
	open := false
	var pc *PopconfirmW
	d := NewDriver(func(c *Context) {
		if Button(c, "Open").Clicked() {
			open = true
		}
		Modal(c, &open, "Dialog").Body(func() {
			pc = Popconfirm(c, "Delete?", "", func() { Button(c, "Remove") })
		})
	}, DriverOptions{Accessibility: true})
	d.Frame()
	d.Key(input.KeyTab, 0)
	d.Key(input.KeyEnter, 0) // keyboard open: focus returns here later
	d.Frame()
	d.Frame()
	if !open {
		t.Fatal("modal not open")
	}
	trig := mustNode(t, d, a11y.RoleButton, "Remove")
	if err := d.PerformAction(trig.ID, a11y.Focus, ""); err != nil {
		t.Fatal(err)
	}
	d.Key(input.KeyEnter, 0)
	d.Frame()
	if !pc.IsOpen() {
		t.Fatal("popconfirm not open")
	}
	d.Key(input.KeyEscape, 0)
	d.Frame()
	d.Frame()
	if pc.IsOpen() || !open {
		t.Fatalf("first Escape: popconfirm open %v, modal open %v", pc.IsOpen(), open)
	}
	if f := d.A11yFocused(); f == nil || f.Name != "Remove" {
		t.Errorf("focus after closing the popconfirm %v, want Remove", f)
	}
	d.Key(input.KeyEscape, 0)
	d.Frame()
	d.Frame()
	if open {
		t.Fatal("second Escape did not close the modal")
	}
	if f := d.A11yFocused(); f == nil || f.Name != "Open" {
		t.Errorf("focus after closing the modal %v, want Open", f)
	}
}

// TestOverlayEscTopmostDrawerModal: a Modal opened over a Drawer closes
// first; the Drawer stays.
func TestOverlayEscTopmostDrawerModal(t *testing.T) {
	drawer, modal := true, true
	d := NewDriver(func(c *Context) {
		Drawer(c, &drawer, "Panel").Body(func() {
			Modal(c, &modal, "Dialog").Body(func() { Button(c, "Inside") })
		})
	}, DriverOptions{})
	d.Frame()
	d.Frame()
	d.Key(input.KeyEscape, 0)
	d.Frame()
	if modal || !drawer {
		t.Fatalf("first Escape: modal %v drawer %v", modal, drawer)
	}
	d.Key(input.KeyEscape, 0)
	d.Frame()
	if drawer {
		t.Error("second Escape did not close the drawer")
	}
}

// TestAlignB3KA11yDialogs: Modal / Drawer are modal dialogs with Dismiss,
// their close buttons are named from the Locale (EnUS), the Popconfirm
// bubble is an alertdialog and the Tour panel a dialog.
func TestAlignB3KA11yDialogs(t *testing.T) {
	m, dr, open, cur := true, false, true, 0
	var target ID
	d := NewDriver(func(c *Context) {
		ConfigProvider(c).Locale(EnUS).Body(func() {
			target = Button(c, "Target").ID()
			Modal(c, &m, "Dialog").Body(func() {})
			Drawer(c, &dr, "Panel").Body(func() {})
		})
	}, DriverOptions{Accessibility: true})
	d.Frame()
	d.Frame()
	dlg := mustNode(t, d, a11y.RoleDialog, "Dialog")
	if !dlg.States.Has(a11y.Modal) || !dlg.Actions.Has(a11y.Dismiss) {
		t.Errorf("modal: %s %s", dlg, dlg.Actions)
	}
	mustNode(t, d, a11y.RoleButton, EnUS.Close)
	m, dr = false, true
	d.Frame()
	d.Frame()
	pn := mustNode(t, d, a11y.RoleDialog, "Panel")
	if !pn.States.Has(a11y.Modal) || !pn.Actions.Has(a11y.Dismiss) {
		t.Errorf("drawer: %s %s", pn, pn.Actions)
	}
	if err := d.PerformAction(pn.ID, a11y.Dismiss, ""); err != nil {
		t.Fatal(err)
	}
	d.Frame()
	if dr {
		t.Error("Dismiss did not close the drawer")
	}
	d = NewDriver(func(c *Context) {
		target = Button(c, "Target").ID()
		Tour(c, &open, &cur, []TourStep{{Target: target, Title: "Step", Description: "Desc"}}).Show()
	}, DriverOptions{Accessibility: true})
	d.Settle()
	tp := mustNode(t, d, a11y.RoleDialog, "Step")
	if !tp.States.Has(a11y.Modal) || tp.Description != "Desc" {
		t.Errorf("tour panel: %s %q", tp, tp.Description)
	}
}

// TestModalCloseButtonRTL: in RTL the close button sits at the inline end
// (the left edge), 12px in, and the title keeps clear of it.
func TestModalCloseButtonRTL(t *testing.T) {
	open := true
	d := NewDriver(func(c *Context) {
		ConfigProvider(c).Direction(RTL).Body(func() {
			Modal(c, &open, "Dialog").Body(func() { Text(c, "Content") })
		})
	}, DriverOptions{Accessibility: true})
	d.Frame()
	d.Frame()
	panel := b3kPanel(t, d, a11y.RoleDialog, "Dialog")
	cl := b3kRect(mustNode(t, d, a11y.RoleButton, ZhCN.Close))
	if dx := cl.X - panel.X; abs32(dx-12) > 0.5 {
		t.Errorf("RTL close inset %v from the left, want 12", dx)
	}
	if title, _ := d.FindText("Dialog"); title.X < cl.Right()-0.5 {
		t.Errorf("RTL title %v runs under the close button %v", title, cl)
	}
}

// TestTourNoMaskBlocking: rc-tour renders its pointer mask with or without
// the dimming. Without a mask: a centred step (no target) and
// DisabledInteraction block the page; a target step with interaction
// leaves it clickable (TestTourTargetInteraction).
func TestTourNoMaskBlocking(t *testing.T) {
	for _, tc := range []struct {
		name      string
		noTarget  bool
		disabled  bool
		wantClick int
	}{
		{"centred", true, false, 0},
		{"disabledInteraction", false, true, 0},
		{"target", false, false, 1},
	} {
		open, cur, n := true, 0, 0
		var a ID
		d := NewDriver(func(c *Context) {
			Row(c, func() {
				b := Button(c, "A")
				a = b.ID()
				if b.Clicked() {
					n++
				}
				Button(c, "B")
			}, layout.Pad(80), layout.Gap(40))
			target := a
			if tc.noTarget {
				target = 0
			}
			Tour(c, &open, &cur, []TourStep{{Target: target, Title: "one"}}).NoMask().DisabledInteraction(tc.disabled).Show()
		}, DriverOptions{})
		d.Settle()
		r, _ := d.Find("A")
		d.Click(center(r))
		d.Frame()
		if n != tc.wantClick {
			t.Errorf("%s: page clicks %d, want %d", tc.name, n, tc.wantClick)
		}
	}
}

// TestModalCloseReserve: a long title stops before the close button (the
// header keeps CloseInsetEnd + CloseBtnSize − Padding clear), also in RTL.
func TestModalCloseReserve(t *testing.T) {
	const long = "A very long dialog title that would otherwise run under the close button of the panel"
	for _, rtl := range []bool{false, true} {
		open := true
		d := NewDriver(func(c *Context) {
			cp := ConfigProvider(c)
			if rtl {
				cp.Direction(RTL)
			}
			cp.Body(func() { Modal(c, &open, long).Width(360).Body(func() {}) })
		}, DriverOptions{Accessibility: true})
		d.Frame()
		d.Frame()
		cl := b3kRect(mustNode(t, d, a11y.RoleButton, ZhCN.Close))
		var title *node
		for _, ov := range d.r.c.overlays {
			if n := b3kFindText(ov.root, long); n != nil {
				title = n
			}
		}
		if title == nil {
			t.Fatal("title node missing")
		}
		tr := title.abs
		if !rtl && tr.Right() > cl.X+0.5 {
			t.Errorf("LTR title box %v reaches the close button %v", tr, cl)
		}
		if rtl && tr.X < cl.Right()-0.5 {
			t.Errorf("RTL title box %v reaches the close button %v", tr, cl)
		}
	}
}

func b3kFindText(n *node, s string) *node {
	if n == nil {
		return nil
	}
	if n.label == s && len(n.kids) == 0 {
		return n
	}
	for _, k := range n.kids {
		if f := b3kFindText(k, s); f != nil {
			return f
		}
	}
	return nil
}

// TestModalElementCloseHover: the Element close button has no hover fill
// (headerbtn background transparent) and turns the glyph primary.
func TestModalElementCloseHover(t *testing.T) {
	th := theme.ElementPlus()
	tk := th.Components.Modal
	if !tk.CloseNoHoverBg || tk.CloseIconColor != th.Info.Base || tk.CloseHoverColor != th.Primary.Base {
		t.Fatalf("Element close tokens: %v %v %v", tk.CloseNoHoverBg, tk.CloseIconColor, tk.CloseHoverColor)
	}
	open := true
	d := NewDriver(func(c *Context) { Modal(c, &open, "Dialog").Body(func() {}) }, DriverOptions{Theme: th, Accessibility: true})
	d.Frame()
	d.Frame()
	cl := b3kRect(mustNode(t, d, a11y.RoleButton, ZhCN.Close))
	d.Move(center(cl))
	d.Settle()
	if t2bCount(d, th.BgTextHover) != 0 {
		t.Error("Element close button painted a hover fill")
	}
}

// TestPopoverArrowOffsetTokens: the default arrow and the panel distance
// come from PopoverToken (Ant arrow, 12 = sizePopupArrow/2 + marginXXS,
// 4 without; Element 12; shadcn no arrow, sideOffset 4), read from the
// scoped component theme, so a component-level override applies.
func TestPopoverArrowOffsetTokens(t *testing.T) {
	gap := func(th *theme.Theme, wrap func(c *Context, fn func()), mod func(p *PopoverW)) (bool, float32) {
		var p *PopoverW
		open := true
		d := NewDriver(func(c *Context) {
			Column(c, func() {
				wrap(c, func() {
					p = Popover(c).Trigger("click").Open(&open).Placement("bottom").Text("panel")
					if mod != nil {
						mod(p)
					}
					p.Body(func() { Button(c, "Trigger") })
				})
			}, layout.Pad(80))
		}, DriverOptions{Theme: th})
		d.Frame()
		d.Frame()
		trig, _ := d.Find("Trigger")
		var panel *node
		for _, ov := range d.r.c.overlays {
			if n := b2gFindRole(ov.root, a11y.RoleDialog); n != nil {
				panel = n
			}
		}
		if panel == nil {
			t.Fatal("panel not open")
		}
		return p.arrow, panel.abs.Y - trig.Bottom()
	}
	plain := func(c *Context, fn func()) { fn() }
	for _, tc := range []struct {
		th    *theme.Theme
		arrow bool
		off   float32
	}{
		{theme.AntDesign(), true, 12},
		{theme.ElementPlus(), true, 12},
		{theme.Shadcn(), false, 4},
	} {
		if a, g := gap(tc.th, plain, nil); a != tc.arrow || g != tc.off {
			t.Errorf("%s: arrow %v gap %v, want %v / %v", tc.th.Flavor, a, g, tc.arrow, tc.off)
		}
	}
	if a, g := gap(theme.AntDesign(), plain, func(p *PopoverW) { p.Arrow(false) }); a || g != 4 {
		t.Errorf("Ant Arrow(false): arrow %v gap %v, want 4", a, g)
	}
	// Component-level token override (scoped theme, not c.Theme; zero
	// fields inherit, so the shadcn arrow is turned on).
	override := func(c *Context, fn func()) {
		var k theme.Components
		k.Popover.Arrow, k.Popover.Offset = true, 20
		ConfigProvider(c).Components(k).Body(fn)
	}
	if a, g := gap(theme.Shadcn(), override, nil); !a || g != 20 {
		t.Errorf("override: arrow %v gap %v, want arrow, 20", a, g)
	}
}
