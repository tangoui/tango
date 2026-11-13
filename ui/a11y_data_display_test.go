package ui

import (
	"image"
	"image/color"
	"strings"
	"testing"
	"time"

	"github.com/tangoui/tango/ui/a11y"
	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/icon"
	"github.com/tangoui/tango/ui/input"
)

func a11yTestImage() image.Image {
	img := image.NewRGBA(image.Rect(0, 0, 40, 30))
	for i := range img.Pix {
		img.Pix[i] = 0x80
	}
	img.Set(0, 0, color.Black)
	return img
}

func TestA11yImage(t *testing.T) {
	var loc *Locale
	img := a11yTestImage()
	var w *ImageW
	page := func(c *Context) {
		loc = c.Locale()
		Image(c, img).Size(80, 60).Alt("Sunset")
		Image(c, nil).Size(80, 60).Loading(true)
		w = Image(c, img).Size(80, 60).A11yLabel("Cat").PreviewGroup([]image.Image{img, img}, 0).Preview(true)
	}
	d := NewDriver(page, DriverOptions{})
	d.EnableAccessibility()
	mustNode(t, d, a11y.RoleImage, "Sunset")
	if n := mustNode(t, d, a11y.RoleImage, loc.A11y.Image); !n.States.Has(a11y.Busy) {
		t.Errorf("loading image: %s", n)
	}
	cat := mustNode(t, d, a11y.RoleImage, "Cat")
	if !cat.States.Has(a11y.Focusable|a11y.HasPopup) || !cat.Actions.Has(a11y.Press) {
		t.Fatalf("preview image: %s %s", cat, cat.Actions)
	}
	if err := d.PerformAction(cat.ID, a11y.Press, ""); err != nil {
		t.Fatal(err)
	}
	d.Settle()
	if !w.PreviewOpen() {
		t.Fatal("press did not open the preview")
	}
	dlg := mustNode(t, d, a11y.RoleDialog, loc.Image.PreviewLabel)
	if !dlg.States.Has(a11y.Modal) || !dlg.Actions.Has(a11y.Dismiss) {
		t.Errorf("preview dialog: %s %s", dlg, dlg.Actions)
	}
	for _, name := range []string{loc.Image.ClosePreview, loc.Image.ZoomIn, loc.Image.ZoomOut, loc.Image.RotateLeft, loc.Image.Next} {
		b := mustNode(t, d, a11y.RoleButton, name)
		if !b.States.Has(a11y.Focusable) {
			t.Errorf("%q not focusable: %s", name, b)
		}
	}
	// Focus is trapped in the preview.
	f := d.A11yFocused()
	if f == nil || !strings.Contains(d.AccessibilityTree().Dump(), "dialog") {
		t.Fatalf("focus %v", f)
	}
	for i := 0; i < 12; i++ {
		d.Key(input.KeyTab, 0)
		if f := d.A11yFocused(); f == nil || f.Role != a11y.RoleButton {
			t.Fatalf("tab %d left the preview: %v", i, f)
		}
	}
	// The page behind the modal is hidden.
	if _, ok := d.A11yFind(a11y.RoleImage, "Sunset"); ok {
		t.Error("page image visible under the modal preview")
	}
	// Zoom in with the keyboard.
	before := w.PreviewScale()
	if err := d.A11yPerform(a11y.RoleButton, loc.Image.ZoomIn, a11y.Press, ""); err != nil {
		t.Fatal(err)
	}
	if w.PreviewScale() <= before {
		t.Errorf("zoom in: %v -> %v", before, w.PreviewScale())
	}
	d.Key(input.KeyEscape, 0)
	d.Settle()
	if w.PreviewOpen() {
		t.Fatal("Escape did not close the preview")
	}
	if f := d.A11yFocused(); f == nil || f.ID != cat.ID {
		t.Errorf("focus after close: %v", f)
	}
}

func TestA11yPopover(t *testing.T) {
	open := false
	page := func(c *Context) {
		Popover(c).Trigger("click").Title("Details").Text("Body text").Body(func() { Button(c, "Open") })
		Popover(c).Trigger("click").Text("Plain").Body(func() { Text(c, "Info") })
		Popover(c).Text("Hover tip").Body(func() { Button(c, "Hover me") })
		Button(c, "After")
	}
	d := NewDriver(page, DriverOptions{})
	d.EnableAccessibility()
	b := mustNode(t, d, a11y.RoleButton, "Open")
	if !b.States.Has(a11y.HasPopup|a11y.Expandable) || b.States.Has(a11y.Expanded) {
		t.Fatalf("trigger: %s", b)
	}
	// Enter on the trigger opens the panel (keyboard click).
	d.Key(input.KeyTab, 0)
	if f := d.A11yFocused(); f == nil || f.Name != "Open" {
		t.Fatalf("focus %v", f)
	}
	d.Key(input.KeyEnter, 0)
	d.Settle()
	b = mustNode(t, d, a11y.RoleButton, "Open")
	dlg := mustNode(t, d, a11y.RoleDialog, "Details")
	if !b.States.Has(a11y.Expanded) || b.Controls != dlg.ID {
		t.Fatalf("open trigger: %s controls=%d dialog=%d", b, b.Controls, dlg.ID)
	}
	d.Key(input.KeyEscape, 0)
	d.Settle()
	if _, ok := d.A11yFind(a11y.RoleDialog, "Details"); ok {
		t.Fatal("Escape did not close the popover")
	}
	// A plain-text trigger becomes a focusable button; untitled panels
	// take the trigger's name.
	info := mustNode(t, d, a11y.RoleButton, "Info")
	if !info.States.Has(a11y.Focusable | a11y.HasPopup) {
		t.Fatalf("text trigger: %s", info)
	}
	if err := d.PerformAction(info.ID, a11y.Expand, ""); err != nil {
		t.Fatal(err)
	}
	d.Settle()
	mustNode(t, d, a11y.RoleDialog, "Info")
	d.Key(input.KeyEscape, 0)
	d.Settle()
	// Hover popovers open on keyboard focus as tooltips; Escape closes.
	hv := mustNode(t, d, a11y.RoleButton, "Hover me")
	if err := d.PerformAction(hv.ID, a11y.Focus, ""); err != nil {
		t.Fatal(err)
	}
	d.Settle()
	mustNode(t, d, a11y.RoleTooltip, "")
	d.Key(input.KeyEscape, 0)
	d.Settle()
	if _, ok := d.A11yFind(a11y.RoleTooltip, ""); ok {
		t.Error("Escape did not close the hover popover")
	}
	_ = open
}

func TestA11yTour(t *testing.T) {
	open, cur := true, 0
	var loc *Locale
	var target ID
	page := func(c *Context) {
		loc = c.Locale()
		target = Button(c, "Upload").ID()
		Tour(c, &open, &cur, []TourStep{
			{Target: target, Title: "Step one", Description: "Upload a file"},
			{Title: "Step two", Description: "Done"},
		}).Show()
	}
	d := NewDriver(page, DriverOptions{})
	d.EnableAccessibility()
	d.Settle()
	dlg := mustNode(t, d, a11y.RoleDialog, "Step one")
	if !dlg.States.Has(a11y.Modal) || dlg.Description != "Upload a file" || !dlg.Actions.Has(a11y.Dismiss) {
		t.Fatalf("tour dialog: %s %s", dlg, dlg.Actions)
	}
	mustNode(t, d, a11y.RoleButton, loc.Tour.Close)
	mustNode(t, d, a11y.RoleText, "1 / 2")
	if f := d.A11yFocused(); f == nil || f.Role != a11y.RoleButton {
		t.Fatalf("focus not moved into the tour: %v", f)
	}
	if err := d.A11yPerform(a11y.RoleButton, loc.Tour.Next, a11y.Press, ""); err != nil {
		t.Fatal(err)
	}
	d.Settle()
	if cur != 1 {
		t.Fatalf("next: cur=%d", cur)
	}
	mustNode(t, d, a11y.RoleDialog, "Step two")
	if err := d.A11yPerform(a11y.RoleDialog, "Step two", a11y.Dismiss, ""); err != nil {
		t.Fatal(err)
	}
	d.Settle()
	if open {
		t.Fatal("dismiss did not close the tour")
	}
}

func TestA11yProgressAndStill(t *testing.T) {
	var loc *Locale
	page := func(c *Context) {
		loc = c.Locale()
		Progress(c, 30)
		Progress(c, 70).Circle(80).A11yLabel("Upload")
		Progress(c, 0).Indeterminate().A11yLabel("Sync")
	}
	d := NewDriver(page, DriverOptions{})
	d.EnableAccessibility()
	p := mustNode(t, d, a11y.RoleProgressBar, loc.A11y.Progress)
	if !p.HasRange || p.ValueNow != 30 || p.ValueMax != 100 || p.Value != "30%" {
		t.Errorf("progress: %s", p)
	}
	if p := mustNode(t, d, a11y.RoleProgressBar, "Upload"); p.ValueNow != 70 {
		t.Errorf("circle: %s", p)
	}
	if p := mustNode(t, d, a11y.RoleProgressBar, "Sync"); p.HasRange || !p.States.Has(a11y.Busy) {
		t.Errorf("indeterminate: %s", p)
	}
	// Reduced motion stops the decorative motion the components drive.
	still := func(c *Context) {
		Progress(c, 40).Status("active")
		Skeleton(c).Active().Show()
		Image(c, nil).Size(60, 60).Loading(true)
	}
	rd := NewDriver(still, DriverOptions{Animations: true, ReduceMotion: true})
	rd.Settle()
	rd.Advance(100 * time.Millisecond)
	rd.Frame()
	if rd.r.Animating() {
		t.Error("animating under ReduceMotion")
	}
	ad := NewDriver(still, DriverOptions{Animations: true})
	ad.Frame()
	if !ad.r.Animating() {
		t.Error("shimmer not animating with motion on")
	}
}

func TestA11yQRCode(t *testing.T) {
	var loc *Locale
	refreshed := 0
	page := func(c *Context) {
		loc = c.Locale()
		QRCode(c, "https://example.com")
		QRCode(c, "x").A11yLabel("Login code").Status("loading")
		if QRCode(c, "y").A11yLabel("Old").Status("expired").Refreshed() {
			refreshed++
		}
	}
	d := NewDriver(page, DriverOptions{})
	d.EnableAccessibility()
	mustNode(t, d, a11y.RoleImage, "https://example.com")
	if n := mustNode(t, d, a11y.RoleImage, "Login code"); !n.States.Has(a11y.Busy) {
		t.Errorf("loading: %s", n)
	}
	b := mustNode(t, d, a11y.RoleButton, loc.QRCode.Refresh)
	if b.Description != loc.QRCode.Expired || !b.States.Has(a11y.Focusable) {
		t.Errorf("refresh: %s", b)
	}
	d.Key(input.KeyTab, 0)
	d.Key(input.KeyEnter, 0)
	if refreshed != 1 {
		t.Errorf("keyboard refresh: %d", refreshed)
	}
}

func TestA11yResultEmptySkeleton(t *testing.T) {
	var loc *Locale
	page := func(c *Context) {
		loc = c.Locale()
		Result(c, "success", "Order placed").SubTitle("Number 42").Show()
		Result(c, "404", "Not found").Show()
		Empty(c)
		Empty(c).Description("")
		Skeleton(c).Show()
		Countdown(c, "Ends in", c.now.Add(90*time.Second))
	}
	d := NewDriver(page, DriverOptions{})
	d.EnableAccessibility()
	if h := mustNode(t, d, a11y.RoleHeading, "Order placed"); h.Level != 2 {
		t.Errorf("heading level %d", h.Level)
	}
	mustNode(t, d, a11y.RoleParagraph, "Number 42")
	if imgs := d.A11yFindAll(a11y.RoleImage, ""); len(imgs) != 1 {
		t.Errorf("decorative result pictures exposed: %v", imgs)
	}
	mustNode(t, d, a11y.RoleText, loc.NoData)
	mustNode(t, d, a11y.RoleImage, loc.NoData) // no description: the picture is named
	if g := mustNode(t, d, a11y.RoleGroup, loc.Loading); !g.States.Has(a11y.Busy) {
		t.Errorf("skeleton: %s", g)
	}
	if tm := mustNode(t, d, a11y.RoleTimer, "Ends in"); tm.Value != "00:01:30" {
		t.Errorf("countdown: %s", tm)
	}
	if rep := d.AuditNoContrast(); len(rep.Issues) != 0 {
		t.Errorf("audit: %v", rep.Issues)
	}
}

func TestA11yListsAndTimeline(t *testing.T) {
	page := 2
	pg := func(c *Context) {
		List(c, 7).Pagination(&page, 3).Item(func(i int) { Text(c, "Row "+itoa64(int64(i))) })
		Timeline(c, []TimelineItem{{Content: "Created"}, {Content: "Shipped"}}).Pending("Delivering").Show()
	}
	d := NewDriver(pg, DriverOptions{})
	d.EnableAccessibility()
	it := mustNode(t, d, a11y.RoleListItem, "Row 4")
	if it.PosInSet != 5 || it.SetSize != 7 {
		t.Errorf("paged item position %d/%d", it.PosInSet, it.SetSize)
	}
	if p := d.AccessibilityTree().Node(it.Parent); p == nil || p.Role != a11y.RoleList {
		t.Errorf("item parent %v", p)
	}
	sh := mustNode(t, d, a11y.RoleListItem, "Shipped")
	if sh.PosInSet != 2 || sh.SetSize != 3 {
		t.Errorf("timeline item %d/%d", sh.PosInSet, sh.SetSize)
	}
	if p := mustNode(t, d, a11y.RoleListItem, "Delivering"); !p.States.Has(a11y.Busy) {
		t.Errorf("pending: %s", p)
	}
}

func TestA11yCardDescriptions(t *testing.T) {
	clicks := 0
	hits := 0
	pg := func(c *Context) {
		Card(c, "Profile").Actions(func() {
			if CardAction(c, icon.Edit, "Edit") {
				clicks++
			}
		}).Body(func() { Text(c, "Hello") })
		cw := Card(c, "Pick me").Hoverable().Body(func() { Text(c, "Body") })
		if cw.Clicked() {
			hits++
		}
		Descriptions(c, []DescItem{{Label: "Name", Value: "Ann"}, {Label: "City", Value: "Oslo"}}).Title("User").Bordered()
		Descriptions(c, []DescItem{{Label: "Phone", Value: "123"}}).Vertical()
	}
	d := NewDriver(pg, DriverOptions{})
	d.EnableAccessibility()
	mustNode(t, d, a11y.RoleGroup, "Profile")
	if err := d.A11yPerform(a11y.RoleButton, "Edit", a11y.Press, ""); err != nil || clicks != 1 {
		t.Fatalf("card action: %v clicks=%d", err, clicks)
	}
	g := mustNode(t, d, a11y.RoleGroup, "Pick me")
	if !g.States.Has(a11y.Focusable) {
		t.Errorf("hoverable card: %s", g)
	}
	if err := d.PerformAction(g.ID, a11y.Focus, ""); err != nil {
		t.Fatal(err)
	}
	d.Key(input.KeyEnter, 0)
	if hits != 1 {
		t.Errorf("keyboard card click: %d", hits)
	}
	mustNode(t, d, a11y.RoleTable, "User")
	rh := mustNode(t, d, a11y.RoleRowHeader, "Name")
	if p := d.AccessibilityTree().Node(rh.Parent); p == nil || p.Role != a11y.RoleRow {
		t.Errorf("row header parent %v", p)
	}
	mustNode(t, d, a11y.RoleCell, "Oslo")
	mustNode(t, d, a11y.RoleColumnHeader, "Phone")
	mustNode(t, d, a11y.RoleCell, "123")
}

func TestA11yTypography(t *testing.T) {
	var loc *Locale
	title := "Title"
	open := false
	long := strings.Repeat("A long paragraph of text that wraps. ", 20)
	pg := func(c *Context) {
		loc = c.Locale()
		Text(c, "Copy me").Copyable()
		Text(c, title).Editable(&title)
		Paragraph(c, long).Lines(2).Expandable(&open)
	}
	d := NewDriver(pg, DriverOptions{})
	d.Resize(400, 600)
	d.EnableAccessibility()
	d.Settle()
	cp := mustNode(t, d, a11y.RoleButton, loc.Copy)
	r, _ := d.FindText("Copy me")
	if cp.Bounds.X < r.X+40 || cp.Bounds.X > r.X+100 || cp.Bounds.W != typoIcon || cp.Bounds.Y < r.Y || cp.Bounds.Y > r.Y+r.H {
		t.Errorf("copy button bounds %+v (label %+v)", cp.Bounds, r)
	}
	d.Key(input.KeyTab, 0)
	if f := d.A11yFocused(); f == nil || f.ID != cp.ID {
		t.Fatalf("tab to copy: %v", f)
	}
	d.Key(input.KeyEnter, 0)
	if d.Clipboard() != "Copy me" {
		t.Errorf("clipboard %q", d.Clipboard())
	}
	d.Settle()
	mustNode(t, d, a11y.RoleButton, loc.Copied)
	ex := mustNode(t, d, a11y.RoleButton, loc.Expand)
	if !ex.States.Has(a11y.Expandable) || ex.States.Has(a11y.Expanded) {
		t.Errorf("expand: %s", ex)
	}
	if err := d.PerformAction(ex.ID, a11y.Expand, ""); err != nil {
		t.Fatal(err)
	}
	d.Settle()
	if !open {
		t.Fatal("expand action did not open")
	}
	mustNode(t, d, a11y.RoleButton, loc.Collapse)
	if err := d.A11yPerform(a11y.RoleButton, loc.Edit, a11y.Press, ""); err != nil {
		t.Fatal(err)
	}
	d.Settle()
	mustNode(t, d, a11y.RoleTextbox, "")
	_ = geom.Rect{}
}
