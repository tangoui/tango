package android

import (
	"testing"
	"time"

	"github.com/tangoui/tango/internal/platform/a11ybridge"
	"github.com/tangoui/tango/ui/a11y"
)

func a11yTestTree(taps string) *a11y.Tree {
	tr := a11y.NewTree(1)
	tr.Add(&a11y.Node{ID: 1, Role: a11y.RoleWindow, Children: []a11y.NodeID{2, 3, 4}, Bounds: a11y.Rect{W: 411, H: 914}})
	tr.Add(&a11y.Node{ID: 2, Parent: 1, Role: a11y.RoleButton, Name: "点我计数", Value: taps, Actions: a11y.Press, Bounds: a11y.Rect{Y: 300, W: 411, H: 72}})
	tr.Add(&a11y.Node{ID: 3, Parent: 1, Role: a11y.RoleTextbox, Name: "用户名", Actions: a11y.SetValue | a11y.Focus, States: a11y.Focusable, Bounds: a11y.Rect{X: 100, Y: 400, W: 200, H: 24}})
	tr.Add(&a11y.Node{ID: 4, Parent: 1, Role: a11y.RoleButton, Name: "隐藏", Actions: a11y.Press, States: a11y.Offscreen, Bounds: a11y.Rect{Y: 2000, W: 411, H: 50}})
	return tr
}

func TestCanvasA11y(t *testing.T) {
	var ax canvasA11y
	now := time.Unix(0, 0)
	if ax.node(a11ybridge.AndroidHostID) != nil || ax.hit(10, 10, 2) != a11ybridge.AndroidNoID || ax.focusVID() != a11ybridge.AndroidNoID {
		t.Fatal("answers without a tree")
	}
	tr := a11yTestTree("0")
	evs := ax.update(tr, a11y.Diff(nil, tr), now)
	if len(evs) != 1 || evs[0].VID != a11ybridge.AndroidHostID || evs[0].Changes != a11ybridge.AndroidChangeSubtree {
		t.Fatalf("first update events %v", evs)
	}
	if ax.node(a11ybridge.AndroidHostID) == nil {
		t.Fatal("no root record")
	}
	btn := ax.vid(2)
	// (50, 310) logical at density 2.625.
	if got := ax.hit(50*2.625, 310*2.625, 2.625); got != btn {
		t.Errorf("hit %d, want %d", got, btn)
	}
	if id, act, _, ok := ax.action(btn, a11ybridge.AndroidClick, "", 0); !ok || id != 2 || act != a11y.Press {
		t.Errorf("click: %d %v %v", id, act, ok)
	}
	field := ax.vid(3)
	if id, act, arg, ok := ax.action(field, a11ybridge.AndroidSetText, "abc", 0); !ok || id != 3 || act != a11y.SetValue || arg != "abc" {
		t.Errorf("set text: %d %v %q %v", id, act, arg, ok)
	}
	if _, _, _, ok := ax.action(12345, a11ybridge.AndroidClick, "", 0); ok {
		t.Error("action on an unknown virtual view accepted")
	}
	ax.focus = 3
	if ax.focusVID() != field {
		t.Errorf("focus %d, want %d", ax.focusVID(), field)
	}

	next := a11yTestTree("1")
	evs = ax.update(next, a11y.Diff(tr, next), now)
	if len(evs) != 1 || evs[0].VID != btn || evs[0].Changes != a11ybridge.AndroidChangeStateDescription {
		t.Errorf("value change events %v", evs)
	}
	if ax.vid(2) != btn {
		t.Error("virtual id changed across updates")
	}

	ax.selfTest = "隐藏"
	if v := ax.selfTestTarget(); v != a11ybridge.AndroidNoID {
		t.Errorf("self-test picked an offscreen node: %d", v)
	}
	ax.selfTest = "点我计数"
	if v := ax.selfTestTarget(); v != btn {
		t.Errorf("self-test target %d, want %d", v, btn)
	}

	ax.setTree(nil, now)
	if ax.node(btn) != nil || ax.focusVID() != a11ybridge.AndroidNoID {
		t.Error("answers after the tree was dropped")
	}
}
