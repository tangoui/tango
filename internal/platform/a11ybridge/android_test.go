package a11ybridge

import (
	"encoding/binary"
	"math"
	"testing"
	"time"

	"github.com/tangoui/tango/ui/a11y"
)

func TestAndroidClass(t *testing.T) {
	for role, p := range a11y.Mappings {
		if got := AndroidClass(&a11y.Node{Role: role}); got != p.Android {
			t.Errorf("%s: class %q, want %q", role, got, p.Android)
		}
	}
	cases := []struct {
		n    a11y.Node
		want string
	}{
		{a11y.Node{Role: a11y.RoleButton}, "android.widget.Button"},
		{a11y.Node{Role: a11y.RoleButton, States: a11y.Toggleable}, "android.widget.ToggleButton"},
		{a11y.Node{Role: a11y.RoleSwitch}, "android.widget.Switch"},
		{a11y.Node{Role: a11y.RoleTextbox, States: a11y.Protected}, "android.widget.EditText"},
		{a11y.Node{Role: a11y.RoleText}, "android.widget.TextView"},
		{a11y.Node{Role: a11y.RoleNone}, "android.view.View"},
	}
	for _, c := range cases {
		if got := AndroidClass(&c.n); got != c.want {
			t.Errorf("%v: %q, want %q", &c.n, got, c.want)
		}
	}
}

func TestAndroidStates(t *testing.T) {
	cases := []struct {
		n    a11y.Node
		want AndroidState
		not  AndroidState
	}{
		{a11y.Node{Role: a11y.RoleSwitch, States: a11y.Checkable | a11y.Checked, Actions: a11y.Press},
			AndroidCheckable | AndroidChecked | AndroidEnabled | AndroidClickable | AndroidVisible, AndroidMixed},
		{a11y.Node{Role: a11y.RoleSwitch, Actions: a11y.Press}, AndroidCheckable, AndroidChecked},
		{a11y.Node{Role: a11y.RoleCheckbox, States: a11y.Checkable | a11y.Mixed}, AndroidCheckable | AndroidMixed, AndroidChecked},
		{a11y.Node{Role: a11y.RoleButton, States: a11y.Toggleable | a11y.Pressed}, AndroidCheckable | AndroidChecked, 0},
		{a11y.Node{Role: a11y.RoleButton, States: a11y.Disabled | a11y.Offscreen}, 0, AndroidEnabled | AndroidVisible | AndroidCheckable},
		{a11y.Node{Role: a11y.RoleTextbox, States: a11y.Focusable | a11y.Focused}, AndroidEditable | AndroidFocusable | AndroidFocused, AndroidMultiLine | AndroidPassword},
		{a11y.Node{Role: a11y.RoleTextbox, States: a11y.Multiline | a11y.ReadOnly}, AndroidMultiLine, AndroidEditable},
		{a11y.Node{Role: a11y.RoleTextbox, States: a11y.Protected | a11y.Invalid | a11y.Required}, AndroidPassword | AndroidInvalid | AndroidRequired, 0},
		{a11y.Node{Role: a11y.RoleHeading}, AndroidHeading, AndroidClickable},
		{a11y.Node{Role: a11y.RoleColumnHeader}, AndroidHeading, 0},
		{a11y.Node{Role: a11y.RoleTab, States: a11y.Selectable | a11y.Selected}, AndroidSelected, 0},
		{a11y.Node{Role: a11y.RoleLink, States: a11y.Current}, AndroidSelected, 0},
		{a11y.Node{Role: a11y.RoleCombobox, States: a11y.Expandable, Actions: a11y.Expand}, AndroidCollapsed | AndroidClickable, AndroidExpanded},
		{a11y.Node{Role: a11y.RoleCombobox, States: a11y.Expandable | a11y.Expanded, Actions: a11y.Collapse}, AndroidExpanded, AndroidCollapsed},
		{a11y.Node{Role: a11y.RoleDialog, Actions: a11y.Dismiss}, AndroidDismissable, 0},
	}
	for _, c := range cases {
		got := AndroidStates(&c.n)
		if !got.Has(c.want) || got&c.not != 0 {
			t.Errorf("%v: states %b, want %b without %b", &c.n, got, c.want, c.not)
		}
	}
}

func TestAndroidActions(t *testing.T) {
	cases := []struct {
		n    a11y.Node
		want AndroidAction
	}{
		{a11y.Node{Role: a11y.RoleButton, Actions: a11y.Press}, AndroidClick},
		{a11y.Node{Role: a11y.RoleTextbox, Actions: a11y.SetValue | a11y.Focus | a11y.ScrollIntoView},
			AndroidSetText | AndroidFocus | AndroidShowOnScreen},
		{a11y.Node{Role: a11y.RoleTextbox, States: a11y.ReadOnly, Actions: a11y.SetValue}, 0},
		{a11y.Node{Role: a11y.RoleSlider, HasRange: true, Actions: a11y.Increment | a11y.Decrement | a11y.SetValue},
			AndroidScrollForward | AndroidScrollBackward | AndroidSetProgress},
		{a11y.Node{Role: a11y.RoleCombobox, States: a11y.Expandable, Actions: a11y.Expand | a11y.Collapse}, AndroidClick | AndroidExpand},
		{a11y.Node{Role: a11y.RoleCombobox, States: a11y.Expandable | a11y.Expanded, Actions: a11y.Expand | a11y.Collapse}, AndroidClick | AndroidCollapse},
		{a11y.Node{Role: a11y.RoleTreeItem, Actions: a11y.Press | a11y.Expand | a11y.Collapse}, AndroidClick | AndroidExpand | AndroidCollapse},
		{a11y.Node{Role: a11y.RoleDialog, Actions: a11y.Dismiss}, AndroidDismiss},
	}
	for _, c := range cases {
		if got := AndroidActions(&c.n); got != c.want {
			t.Errorf("%v (%v): actions %b, want %b", &c.n, c.n.Actions, got, c.want)
		}
	}
}

func TestAndroidPerform(t *testing.T) {
	btn := &a11y.Node{Role: a11y.RoleButton, Actions: a11y.Press}
	if a, _, ok := AndroidPerform(btn, AndroidClick, "", 0); !ok || a != a11y.Press {
		t.Errorf("click: %v %v", a, ok)
	}
	if _, _, ok := AndroidPerform(btn, AndroidSetText, "x", 0); ok {
		t.Error("set text on a button accepted")
	}
	combo := &a11y.Node{Role: a11y.RoleCombobox, States: a11y.Expandable, Actions: a11y.Expand | a11y.Collapse}
	if a, _, _ := AndroidPerform(combo, AndroidClick, "", 0); a != a11y.Expand {
		t.Errorf("click on collapsed combobox: %v", a)
	}
	combo.States |= a11y.Expanded
	if a, _, _ := AndroidPerform(combo, AndroidClick, "", 0); a != a11y.Collapse {
		t.Errorf("click on expanded combobox: %v", a)
	}
	tb := &a11y.Node{Role: a11y.RoleTextbox, Actions: a11y.SetValue}
	if a, arg, ok := AndroidPerform(tb, AndroidSetText, "你好 😀", 0); !ok || a != a11y.SetValue || arg != "你好 😀" {
		t.Errorf("set text: %v %q %v", a, arg, ok)
	}
	sl := &a11y.Node{Role: a11y.RoleSlider, HasRange: true, ValueMin: 0, ValueMax: 100, ValueNow: 30, Actions: a11y.SetValue | a11y.Increment}
	if a, arg, ok := AndroidPerform(sl, AndroidSetProgress, "", 55.5); !ok || a != a11y.SetValue || arg != "55.5" {
		t.Errorf("set progress: %v %q %v", a, arg, ok)
	}
	if _, arg, _ := AndroidPerform(sl, AndroidSetProgress, "", 1000); arg != "100" {
		t.Errorf("set progress clamps: %q", arg)
	}
	if a, _, _ := AndroidPerform(sl, AndroidScrollForward, "", 0); a != a11y.Increment {
		t.Errorf("scroll forward: %v", a)
	}
	step := &a11y.Node{Role: a11y.RoleSlider, HasRange: true, ValueMax: 10, ValueNow: 5, Actions: a11y.Increment | a11y.Decrement}
	if a, _, _ := AndroidPerform(step, AndroidSetProgress, "", 2); a != a11y.Decrement {
		t.Errorf("set progress without SetValue: %v", a)
	}
	if _, _, ok := AndroidPerform(step, AndroidSetProgress, "", 5); ok {
		t.Error("set progress to the current value accepted")
	}
	if _, _, ok := AndroidPerform(step, AndroidSetProgress, "", math.NaN()); ok {
		t.Error("NaN progress accepted")
	}
}

func TestAndroidText(t *testing.T) {
	cell := &a11y.Node{Role: a11y.RoleButton, Name: "点我计数", Value: "3", Description: "每次点按加一"}
	if s := AndroidText(cell); s.Text != "点我计数" || s.StateDescription != "3" || s.Hint != "每次点按加一" || s.ContentDescription != "" {
		t.Errorf("cell: %+v", s)
	}
	field := &a11y.Node{Role: a11y.RoleTextbox, Name: "邮箱", Value: "a@b.c", Placeholder: "name@example.com"}
	if s := AndroidText(field); s.Text != "a@b.c" || s.Hint != "邮箱, name@example.com" || s.StateDescription != "" {
		t.Errorf("field: %+v", s)
	}
	pw := &a11y.Node{Role: a11y.RoleTextbox, Name: "密码", Value: "secret", States: a11y.Protected | a11y.Invalid, Description: "太短"}
	if s := AndroidText(pw); s.Text != "" || s.Hint != "密码" || s.Error != "太短" {
		t.Errorf("password: %+v", s)
	}
	img := &a11y.Node{Role: a11y.RoleImage, Name: "logo"}
	if s := AndroidText(img); s.ContentDescription != "logo" || s.Text != "" {
		t.Errorf("image: %+v", s)
	}
	dlg := &a11y.Node{Role: a11y.RoleDialog, Name: "确认", RoleDescription: "对话框"}
	if s := AndroidText(dlg); s.PaneTitle != "确认" || s.RoleDescription != "对话框" {
		t.Errorf("dialog: %+v", s)
	}
}

// listTree: window > list "网络" > [switch, switch, button], heading.
func listTree() *a11y.Tree {
	tr := a11y.NewTree(1)
	tr.Add(&a11y.Node{ID: 1, Role: a11y.RoleWindow, Children: []a11y.NodeID{2, 3}, Bounds: a11y.Rect{W: 390, H: 844}})
	tr.Add(&a11y.Node{ID: 2, Parent: 1, Role: a11y.RoleHeading, Name: "网络", Bounds: a11y.Rect{Y: 45, W: 390, H: 39}})
	tr.Add(&a11y.Node{ID: 3, Parent: 1, Role: a11y.RoleList, Name: "网络", Children: []a11y.NodeID{4, 5, 6}, Bounds: a11y.Rect{Y: 84, W: 390, H: 150}})
	tr.Add(&a11y.Node{ID: 4, Parent: 3, Role: a11y.RoleSwitch, Name: "无线局域网", States: a11y.Checkable | a11y.Checked, Actions: a11y.Press, Bounds: a11y.Rect{Y: 84, W: 390, H: 50}})
	tr.Add(&a11y.Node{ID: 5, Parent: 3, Role: a11y.RoleSwitch, Name: "蓝牙", States: a11y.Checkable, Actions: a11y.Press, Bounds: a11y.Rect{Y: 134, W: 390, H: 50}})
	tr.Add(&a11y.Node{ID: 6, Parent: 3, Role: a11y.RoleButton, Name: "蜂窝网络", Value: "已开启", Actions: a11y.Press, Bounds: a11y.Rect{Y: 184, W: 390, H: 50}})
	return tr
}

func TestAndroidCollections(t *testing.T) {
	tr := listTree()
	c, ok := AndroidCollectionInfo(tr, tr.Node(3))
	if !ok || c.Rows != 3 || c.Cols != 1 || c.Selection != 0 {
		t.Errorf("list: %+v %v", c, ok)
	}
	it, ok := AndroidCollectionItem(tr, tr.Node(5))
	if !ok || it.Row != 1 || it.Col != 0 {
		t.Errorf("second item: %+v %v", it, ok)
	}
	if _, ok := AndroidCollectionItem(tr, tr.Node(2)); ok {
		t.Error("heading outside a list is an item")
	}

	tabs := a11y.NewTree(1)
	tabs.Add(&a11y.Node{ID: 1, Role: a11y.RoleTabList, Children: []a11y.NodeID{2, 3}})
	tabs.Add(&a11y.Node{ID: 2, Parent: 1, Role: a11y.RoleTab, States: a11y.Selectable | a11y.Selected, PosInSet: 1, SetSize: 5})
	tabs.Add(&a11y.Node{ID: 3, Parent: 1, Role: a11y.RoleTab, States: a11y.Selectable, PosInSet: 2, SetSize: 5})
	if c, _ := AndroidCollectionInfo(tabs, tabs.Node(1)); c.Rows != 1 || c.Cols != 5 || c.Selection != 1 {
		t.Errorf("tab list: %+v", c)
	}
	if it, _ := AndroidCollectionItem(tabs, tabs.Node(3)); it.Col != 1 || it.Row != 0 || it.Selected {
		t.Errorf("tab: %+v", it)
	}

	tbl := a11y.NewTree(1)
	tbl.Add(&a11y.Node{ID: 1, Role: a11y.RoleTable, Children: []a11y.NodeID{2, 3}})
	tbl.Add(&a11y.Node{ID: 2, Parent: 1, Role: a11y.RoleRow, Children: []a11y.NodeID{4, 5}})
	tbl.Add(&a11y.Node{ID: 4, Parent: 2, Role: a11y.RoleColumnHeader, RowIndex: 1, ColIndex: 1})
	tbl.Add(&a11y.Node{ID: 5, Parent: 2, Role: a11y.RoleColumnHeader, RowIndex: 1, ColIndex: 2})
	tbl.Add(&a11y.Node{ID: 3, Parent: 1, Role: a11y.RoleRow, Children: []a11y.NodeID{6, 7}})
	tbl.Add(&a11y.Node{ID: 6, Parent: 3, Role: a11y.RoleCell, RowIndex: 2, ColIndex: 1})
	tbl.Add(&a11y.Node{ID: 7, Parent: 3, Role: a11y.RoleCell, RowIndex: 2, ColIndex: 2})
	if c, _ := AndroidCollectionInfo(tbl, tbl.Node(1)); c.Rows != 2 || c.Cols != 2 {
		t.Errorf("table without counts: %+v", c)
	}
	tbl.Node(1).RowCount, tbl.Node(1).ColCount = 100, 2
	if c, _ := AndroidCollectionInfo(tbl, tbl.Node(1)); c.Rows != 100 || c.Cols != 2 {
		t.Errorf("virtualized table: %+v", c)
	}
	if it, ok := AndroidCollectionItem(tbl, tbl.Node(5)); !ok || !it.Heading || it.Row != 0 || it.Col != 1 {
		t.Errorf("header: %+v", it)
	}
	if it, ok := AndroidCollectionItem(tbl, tbl.Node(7)); !ok || it.Heading || it.Row != 1 || it.Col != 1 {
		t.Errorf("cell: %+v", it)
	}
}

func TestVirtualIDs(t *testing.T) {
	var v VirtualIDs
	now := time.Unix(1000, 0)
	tr := listTree()
	v.Sync(tr, now)
	if vid, ok := v.VID(1); !ok || vid != AndroidHostID {
		t.Fatalf("root: %d %v", vid, ok)
	}
	if id, ok := v.Node(AndroidHostID); !ok || id != 1 {
		t.Fatalf("host: %d %v", id, ok)
	}
	ids := map[a11y.NodeID]int32{}
	seen := map[int32]bool{}
	for id := a11y.NodeID(2); id <= 6; id++ {
		vid, ok := v.VID(id)
		if !ok || vid < 0 || seen[vid] {
			t.Fatalf("node %d: vid %d %v", id, vid, ok)
		}
		seen[vid] = true
		ids[id] = vid
		if back, _ := v.Node(vid); back != id {
			t.Fatalf("vid %d maps back to %d", vid, back)
		}
	}
	// Stable across a full resync and an update that changes node 4.
	v.Sync(tr, now)
	v.Update(tr, []a11y.NodeID{4}, now)
	for id, vid := range ids {
		if got, _ := v.VID(id); got != vid {
			t.Fatalf("node %d moved from %d to %d", id, vid, got)
		}
	}
	// Remove node 6: its id is gone at once but not reused for a while.
	tr2 := listTree()
	tr2.Node(3).Children = []a11y.NodeID{4, 5}
	tr2.Remove(6)
	v.Update(tr2, []a11y.NodeID{3, 6}, now)
	if _, ok := v.VID(6); ok {
		t.Fatal("removed node still mapped")
	}
	if _, ok := v.Node(ids[6]); ok {
		t.Fatal("removed id still resolves")
	}
	tr2.Add(&a11y.Node{ID: 7, Parent: 3, Role: a11y.RoleButton})
	v.Update(tr2, []a11y.NodeID{7}, now.Add(time.Hour))
	if vid, _ := v.VID(7); vid == ids[6] {
		t.Fatalf("id %d reused after one update", vid)
	}
	for i := 0; i < 4; i++ {
		v.Update(tr2, nil, now)
	}
	tr2.Add(&a11y.Node{ID: 8, Parent: 3, Role: a11y.RoleButton})
	v.Update(tr2, []a11y.NodeID{8}, now.Add(time.Second))
	if vid, _ := v.VID(8); vid == ids[6] {
		t.Fatalf("id %d reused before MinAge", vid)
	}
	tr2.Add(&a11y.Node{ID: 9, Parent: 3, Role: a11y.RoleButton})
	v.Update(tr2, []a11y.NodeID{9}, now.Add(time.Minute))
	if vid, _ := v.VID(9); vid != ids[6] {
		t.Fatalf("retired id not reused: got %d, want %d", vid, ids[6])
	}
	// Dropping the tree retires everything.
	v.Sync(nil, now)
	if v.Len() != 0 {
		t.Fatalf("%d ids left after Sync(nil)", v.Len())
	}
	if _, ok := v.Node(AndroidHostID); ok {
		t.Fatal("host resolves without a tree")
	}
}

func TestAndroidEvents(t *testing.T) {
	var v VirtualIDs
	now := time.Unix(0, 0)
	prev := listTree()
	v.Sync(prev, now)
	next := listTree()
	next.Node(5).States |= a11y.Checked // toggled
	next.Node(6).Value = "已关闭"          // value of a non-text node
	next.Node(2).Name = "网络与连接"         // text
	next.Node(4).Bounds.Y += 10
	next.Node(4).States |= a11y.Offscreen              // moved and scrolled out of view only
	next.Node(3).Children = []a11y.NodeID{4, 5, 6, 10} // a node added
	next.Add(&a11y.Node{ID: 10, Parent: 3, Role: a11y.RoleButton, Name: "新"})
	changed := a11y.Diff(prev, next)
	v.Update(next, changed, now)
	evs := AndroidEvents(prev, next, changed, &v, 0)
	got := map[a11y.NodeID]AndroidChange{}
	host := false
	for _, e := range evs {
		if e.VID == AndroidHostID {
			host = true
			continue
		}
		id, _ := v.Node(e.VID)
		got[id] = e.Changes
	}
	want := map[a11y.NodeID]AndroidChange{
		2: AndroidChangeText,
		3: AndroidChangeSubtree,
		5: AndroidChangeStateDescription,
		6: AndroidChangeStateDescription,
	}
	for id, c := range want {
		if got[id] != c {
			t.Errorf("node %d: changes %b, want %b (all %v)", id, got[id], c, evs)
		}
	}
	if _, ok := got[4]; ok {
		t.Errorf("moved node got its own event: %v", evs)
	}
	if !host {
		t.Errorf("no host event for moved bounds: %v", evs)
	}
	// Removal changes the parent's subtree.
	gone := listTree()
	gone.Node(3).Children = []a11y.NodeID{4, 5}
	gone.Remove(6)
	ch := a11y.Diff(next, gone)
	v.Update(gone, ch, now)
	for _, e := range AndroidEvents(next, gone, ch, &v, 0) {
		if id, _ := v.Node(e.VID); id == 3 && e.Changes.has(AndroidChangeSubtree) {
			goto ok
		}
	}
	t.Error("removal did not change the list's subtree")
ok:
	if evs := AndroidEvents(prev, next, changed, &v, 2); len(evs) != 1 || evs[0].VID != AndroidHostID || evs[0].Changes != AndroidChangeSubtree {
		t.Errorf("collapse past max: %v", evs)
	}
}

func (c AndroidChange) has(x AndroidChange) bool { return c&x == x }

// decoded is a PackAndroidNode record read back as CanvasA11y.java does.
type decoded struct {
	vid, parent int32
	states      AndroidState
	actions     AndroidAction
	live, flags uint8
	bounds      [4]float32
	rangeType   uint8
	rng         [3]float32
	coll        [2]int32
	collH       bool
	collSel     uint8
	item        [2]int32
	itemH       bool
	itemSel     bool
	children    []int32
	strs        [8]string
}

func decodeRecord(t *testing.T, b []byte) decoded {
	t.Helper()
	le := binary.LittleEndian
	o := 0
	need := func(n int) {
		if o+n > len(b) {
			t.Fatalf("record truncated at %d (+%d) of %d", o, n, len(b))
		}
	}
	u8 := func() uint8 { need(1); o++; return b[o-1] }
	u16 := func() uint16 { need(2); o += 2; return le.Uint16(b[o-2:]) }
	u32 := func() uint32 { need(4); o += 4; return le.Uint32(b[o-4:]) }
	i32 := func() int32 { return int32(u32()) }
	f32 := func() float32 { return math.Float32frombits(u32()) }
	var d decoded
	if v := u8(); v != AndroidRecordVersion {
		t.Fatalf("version %d", v)
	}
	d.vid, d.parent = i32(), i32()
	d.states = AndroidState(u32())
	d.actions = AndroidAction(u16())
	d.live, d.flags = u8(), u8()
	for i := range d.bounds {
		d.bounds[i] = f32()
	}
	if d.flags&1 != 0 {
		d.rangeType = u8()
		for i := range d.rng {
			d.rng[i] = f32()
		}
	}
	if d.flags&2 != 0 {
		d.coll[0], d.coll[1] = i32(), i32()
		d.collH, d.collSel = u8() != 0, u8()
	}
	if d.flags&4 != 0 {
		d.item[0], d.item[1] = i32(), i32()
		d.itemH, d.itemSel = u8() != 0, u8() != 0
	}
	n := int(i32())
	for range n {
		d.children = append(d.children, i32())
	}
	for i := range d.strs {
		l := int(i32())
		need(l)
		d.strs[i] = string(b[o : o+l])
		o += l
	}
	if o != len(b) {
		t.Fatalf("%d trailing bytes", len(b)-o)
	}
	return d
}

func TestPackAndroidNode(t *testing.T) {
	var v VirtualIDs
	tr := listTree()
	v.Sync(tr, time.Unix(0, 0))
	vid := func(id a11y.NodeID) int32 { x, _ := v.VID(id); return x }

	root := decodeRecord(t, PackAndroidNode(tr, 1, &v))
	if root.vid != AndroidHostID || root.parent != AndroidNoID || len(root.children) != 2 ||
		root.children[0] != vid(2) || root.children[1] != vid(3) {
		t.Errorf("root: %+v", root)
	}
	list := decodeRecord(t, PackAndroidNode(tr, 3, &v))
	if list.parent != AndroidHostID || list.flags&2 == 0 || list.coll != [2]int32{3, 1} ||
		len(list.children) != 3 || list.children[2] != vid(6) || list.strs[0] != "android.widget.ListView" {
		t.Errorf("list: %+v", list)
	}
	sw := decodeRecord(t, PackAndroidNode(tr, 4, &v))
	if sw.strs[0] != "android.widget.Switch" || sw.strs[1] != "无线局域网" || !sw.states.Has(AndroidChecked|AndroidCheckable) ||
		sw.actions != AndroidClick || sw.flags&4 == 0 || sw.item != [2]int32{0, 0} || sw.parent != vid(3) ||
		sw.bounds != [4]float32{0, 84, 390, 50} {
		t.Errorf("switch: %+v", sw)
	}
	cell := decodeRecord(t, PackAndroidNode(tr, 6, &v))
	if cell.strs[1] != "蜂窝网络" || cell.strs[3] != "已开启" || cell.item[0] != 2 {
		t.Errorf("cell: %+v", cell)
	}
	tr.Add(&a11y.Node{ID: 20, Parent: 1, Role: a11y.RoleSlider, Name: "音量", HasRange: true, ValueMin: 0, ValueMax: 10, ValueNow: 3,
		Live: a11y.LivePolite, Actions: a11y.Increment | a11y.Decrement})
	tr.Node(1).Children = append(tr.Node(1).Children, 20)
	v.Sync(tr, time.Unix(0, 0))
	sl := decodeRecord(t, PackAndroidNode(tr, 20, &v))
	if sl.flags&1 == 0 || sl.rangeType != AndroidRangeInt || sl.rng != [3]float32{0, 10, 3} || sl.live != 1 ||
		sl.actions != AndroidScrollForward|AndroidScrollBackward|AndroidSetProgress {
		t.Errorf("slider: %+v", sl)
	}
	if PackAndroidNode(tr, 99, &v) != nil {
		t.Error("unknown node packed")
	}
}

func TestAndroidHit(t *testing.T) {
	var v VirtualIDs
	tr := listTree()
	v.Sync(tr, time.Unix(0, 0))
	want, _ := v.VID(5)
	if got := AndroidHit(tr, &v, 100, 150); got != want {
		t.Errorf("hit %d, want %d", got, want)
	}
	if got := AndroidHit(tr, &v, 100, 500); got != AndroidHostID {
		t.Errorf("hit on the window %d", got)
	}
	if got := AndroidHit(tr, &v, 1000, 5000); got != AndroidNoID {
		t.Errorf("hit outside %d", got)
	}
	// RTL: bounds are already mirrored, children stay in reading order.
	tr.RTL = true
	tr.Node(4).RTL = true
	if got := AndroidHit(tr, &v, 100, 100); got != func() int32 { x, _ := v.VID(4); return x }() {
		t.Errorf("RTL hit %d", got)
	}
	rec := decodeRecord(t, PackAndroidNode(tr, 3, &v))
	a, _ := v.VID(4)
	if rec.children[0] != a {
		t.Errorf("RTL children reordered: %v", rec.children)
	}
}
