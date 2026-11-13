package uihost_test

import (
	"slices"
	"testing"

	"github.com/tangoui/tango"
	"github.com/tangoui/tango/internal/platform/fake"
	"github.com/tangoui/tango/internal/platform/fake/faketest"
	"github.com/tangoui/tango/ui"
	"github.com/tangoui/tango/ui/a11y"
	"github.com/tangoui/tango/uihost"
)

func settle(cv *fake.Canvas) {
	for i := 0; i < 10 && cv.FlushFrame(); i++ {
	}
}

func ops(calls []fake.A11yCall) []string {
	var out []string
	for _, c := range calls {
		out = append(out, c.Op)
	}
	return out
}

func TestA11yOffByDefault(t *testing.T) {
	a, fb := faketest.Run(t, tango.Options{})
	r, _, cv := open(t, a, fb, 300, 200, func(c *ui.Context) { ui.Button(c, "OK") })
	settle(cv)
	if n := len(cv.A11yCalls()); n != 0 {
		t.Fatalf("calls without an assistive technology: %v", ops(cv.A11yCalls()))
	}
	if tango.Await(a, func() *a11y.Tree { return r.AccessibilityTree() }) != nil {
		t.Fatal("tree built while off")
	}
}

func TestA11yPlatformBridge(t *testing.T) {
	a, fb := faketest.Run(t, tango.Options{})
	clicks := 0
	label := "Count 0"
	announce := ""
	_, _, cv := open(t, a, fb, 300, 200, func(c *ui.Context) {
		if ui.Button(c, "OK").Clicked() {
			clicks++
		}
		ui.Text(c, label)
		if announce != "" {
			c.Announce(announce, a11y.LiveAssertive)
			announce = ""
		}
	})
	settle(cv)

	// An assistive technology attaches: the first tree is sent whole.
	cv.SetA11yActive(true)
	settle(cv)
	calls := cv.A11yCalls()
	if len(calls) == 0 || calls[0].Op != "set" || calls[0].Tree == nil || !calls[0].Main {
		t.Fatalf("first call %v", ops(calls))
	}
	tree := cv.A11yTree()
	btn := tree.Find(a11y.RoleButton, "OK")
	if btn == nil {
		t.Fatalf("no button in\n%s", tree.Dump())
	}
	if btn.Bounds.W <= 0 || btn.Bounds.H <= 0 {
		t.Fatalf("button bounds %+v", btn.Bounds)
	}

	// An action from the assistive technology clicks the button.
	cv.ResetA11yCalls()
	if err := cv.A11yAction(btn.ID, a11y.Press, ""); err != nil {
		t.Fatal(err)
	}
	settle(cv)
	if clicks != 1 {
		t.Fatalf("clicks = %d after AXPress", clicks)
	}
	if err := cv.A11yAction(987654, a11y.Press, ""); err == nil {
		t.Fatal("action on an unknown node accepted")
	}

	// A change arrives as an incremental update.
	cv.ResetA11yCalls()
	tango.Await(a, func() bool { label = "Count 1"; return true })
	cv.Invalidate()
	settle(cv)
	var upd *fake.A11yCall
	for _, c := range cv.A11yCalls() {
		if c.Op == "update" {
			upd = &c
			break
		}
	}
	if upd == nil {
		t.Fatalf("no update: %v", ops(cv.A11yCalls()))
	}
	txt := upd.Tree.Find(a11y.RoleText, "Count 1")
	if txt == nil || !slices.Contains(upd.Changed, txt.ID) {
		t.Fatalf("changed %v does not list the text\n%s", upd.Changed, upd.Tree.Dump())
	}
	if upd.Tree.Find(a11y.RoleText, "Count 0") != nil {
		t.Fatal("stale text")
	}

	// Announcements follow the update.
	cv.ResetA11yCalls()
	tango.Await(a, func() bool { announce = "Saved"; return true })
	cv.Invalidate()
	settle(cv)
	if got := cv.A11yAnnouncements(); !slices.Equal(got, []string{"Saved"}) {
		t.Fatalf("announcements %v (%v)", got, ops(cv.A11yCalls()))
	}

	// Focus: the Focus action moves keyboard focus, reported once.
	cv.ResetA11yCalls()
	if err := cv.A11yAction(btn.ID, a11y.Focus, ""); err != nil {
		t.Fatal(err)
	}
	settle(cv)
	if cv.A11yFocus() != btn.ID {
		t.Fatalf("focus %d, want %d (%v)", cv.A11yFocus(), btn.ID, ops(cv.A11yCalls()))
	}

	// The assistive technology leaves: the platform drops its tree.
	cv.ResetA11yCalls()
	cv.SetA11yActive(false)
	settle(cv)
	if c := cv.A11yCalls(); len(c) != 1 || c[0].Op != "set" || c[0].Tree != nil {
		t.Fatalf("after deactivation %v", ops(c))
	}
}

func TestA11yTapAndForce(t *testing.T) {
	a, fb := faketest.Run(t, tango.Options{})
	r, _, cv := open(t, a, fb, 300, 200, func(c *ui.Context) { ui.Button(c, "OK") })
	settle(cv)
	if !tango.Await(a, func() bool { return uihost.HasA11y(r) }) {
		t.Fatal("no bridge")
	}
	type got struct {
		trees   []*a11y.Tree
		changed [][]a11y.NodeID
	}
	var one, two got
	cancelOne := tango.Await(a, func() func() {
		return uihost.A11yTap(r, func(t *a11y.Tree, ch []a11y.NodeID) {
			one.trees = append(one.trees, t)
			one.changed = append(one.changed, ch)
		})
	})
	release := tango.Await(a, func() func() { return uihost.A11yForce(r) })
	settle(cv)
	if len(one.trees) != 1 || one.trees[0] == nil || one.trees[0].Find(a11y.RoleButton, "OK") == nil {
		t.Fatalf("tap one got %d trees", len(one.trees))
	}
	if len(cv.A11yCalls()) != 0 {
		t.Fatalf("platform called while only forced: %v", ops(cv.A11yCalls()))
	}
	// A late subscriber gets the current tree as a full baseline.
	tango.Await(a, func() bool {
		uihost.A11yTap(r, func(t *a11y.Tree, ch []a11y.NodeID) {
			two.trees = append(two.trees, t)
			two.changed = append(two.changed, ch)
		})
		return true
	})
	if len(two.trees) != 1 || two.trees[0] != one.trees[0] || len(two.changed[0]) != two.trees[0].Len() {
		t.Fatalf("late tap: %d trees", len(two.trees))
	}
	// The platform attaching later still gets the whole tree, though the
	// tree did not change.
	cv.SetA11yActive(true)
	settle(cv)
	if c := cv.A11yCalls(); len(c) == 0 || c[0].Op != "set" || c[0].Tree != one.trees[0] {
		t.Fatalf("platform after force: %v", ops(c))
	}
	cv.SetA11yActive(false)
	settle(cv)
	// Both subscribers stay on while forced; cancelling one keeps the other.
	tango.Await(a, func() bool { cancelOne(); return true })
	n1, n2 := len(one.trees), len(two.trees)
	cv.Input(nil)
	tango.Await(a, func() bool { r.Invalidate(); return true })
	settle(cv)
	_ = n2
	// Release: the tree is dropped and the remaining subscriber hears it.
	tango.Await(a, func() bool { release(); return true })
	settle(cv)
	if len(one.trees) != n1 {
		t.Fatal("cancelled subscriber still called")
	}
	if last := two.trees[len(two.trees)-1]; last != nil {
		t.Fatal("remaining subscriber did not get (nil, nil) on drop")
	}
	if tango.Await(a, func() *a11y.Tree { return r.AccessibilityTree() }) != nil {
		t.Fatal("runner kept the tree")
	}
}

func TestReduceMotionFollowsSystem(t *testing.T) {
	a, fb := faketest.Run(t, tango.Options{})
	fb.SetReduceMotion(true)
	r, _, cv := open(t, a, fb, 300, 200, func(c *ui.Context) {})
	settle(cv)
	rm := func(r *ui.Runner) bool { return tango.Await(a, func() bool { return r.Context().ReduceMotion() }) }
	if !rm(r) {
		t.Fatal("system reduce motion not applied at open")
	}
	o := tango.Await(a, func() opened {
		r, s, err := uihost.Open(a, uihost.Options{Surface: tango.SurfaceOptions{Width: 100, Height: 100}},
			func(*ui.Context) {}, ui.RunnerOptions{ReduceMotion: true})
		return opened{r, s, err}
	})
	if o.err != nil {
		t.Fatal(o.err)
	}
	fb.SetReduceMotion(false)
	fb.Do(func() {})
	if rm(r) {
		t.Fatal("reduce motion still on after the system turned it off")
	}
	if !rm(o.r) {
		t.Fatal("RunnerOptions.ReduceMotion overridden by the system")
	}
	fb.SetReduceMotion(true)
	fb.Do(func() {})
	if !rm(r) {
		t.Fatal("reduce motion not re-applied")
	}
}

func TestA11yAttachedAction(t *testing.T) {
	a, fb := faketest.Run(t, tango.Options{})
	s := webSurface(t, a, 400, 300)
	clicks := 0
	_, at, cv := attach(t, a, fb, s, uihost.Layer{Above: true}, func(c *ui.Context) {
		if ui.Button(c, "Go").Clicked() {
			clicks++
		}
	})
	settle(cv.Canvas)
	cv.SetA11yActive(true)
	settle(cv.Canvas)
	btn := cv.A11yTree().Find(a11y.RoleButton, "Go")
	if btn == nil {
		t.Fatal("no button")
	}
	if err := cv.A11yAction(btn.ID, a11y.Press, ""); err != nil {
		t.Fatal(err)
	}
	settle(cv.Canvas)
	if clicks != 1 {
		t.Fatalf("clicks %d", clicks)
	}
	tango.Await(a, func() bool { at.Detach(); return true })
	if err := cv.A11yAction(btn.ID, a11y.Press, ""); err == nil {
		t.Fatal("action after Detach accepted")
	}
}
