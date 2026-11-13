package linux

import (
	"strings"
	"testing"
	"unsafe"

	"github.com/tangoui/tango/ui/a11y"
)

func TestGtkWidgetClassMirror(t *testing.T) {
	if unsafe.Sizeof(uintptr(0)) != 8 {
		t.Skip("offsets are for LP64 targets")
	}
	var w gtkWidgetClassMirror
	if got := unsafe.Sizeof(gObjectClassMirror{}); got != 136 {
		t.Errorf("GObjectClass size %d, want 136", got)
	}
	for _, c := range []struct {
		name      string
		got, want uintptr
	}{
		{"activate_signal", unsafe.Offsetof(w.activateSignal), 136},
		{"dispatch_child_properties_changed", unsafe.Offsetof(w.dispatchChildPropertiesChanged), 144},
		{"destroy", unsafe.Offsetof(w.basics), 152},
		{"draw", unsafe.Offsetof(w.basics) + 17*8, 288},
		{"event", unsafe.Offsetof(w.events), 376},
		{"get_accessible", unsafe.Offsetof(w.getAccessible), gtkWidgetClassGetAccessible},
	} {
		if c.got != c.want {
			t.Errorf("%s: offset %d, want %d", c.name, c.got, c.want)
		}
	}
	var q gTypeQueryMirror
	if unsafe.Offsetof(q.classSize) != 16 || unsafe.Sizeof(q) != 24 {
		t.Error("GTypeQuery layout")
	}
}

func TestAtspiPaths(t *testing.T) {
	for _, o := range []atspiObj{
		{app: true},
		{key: 0x1f, alias: true},
		{key: 3, id: 0xdeadbeefcafe},
	} {
		p := atspiPath(o)
		if strings.Count(strings.TrimPrefix(p, atspiBase+"/"), "/") != 0 {
			t.Errorf("%s: not one segment", p)
		}
		got, ok := atspiParse(p)
		if !ok || got != o {
			t.Errorf("parse %s = %+v %v, want %+v", p, got, ok, o)
		}
		if got, ok := atspiParse(atspiNodeName(o)); !ok || got != o {
			t.Errorf("parse node %s = %+v", atspiNodeName(o), got)
		}
	}
	for _, bad := range []string{atspiBase + "/x1", atspiBase + "/c", atspiBase + "/c1_zz", atspiBase + "/c1/2"} {
		if _, ok := atspiParse(bad); ok {
			t.Errorf("%s parsed", bad)
		}
	}
}

func TestAtspiVariantText(t *testing.T) {
	if got := gvRef(":1.5", "/a/b"); got != "(':1.5', objectpath '/a/b')" {
		t.Errorf("gvRef %s", got)
	}
	if got := gvRef("", ""); got != "('', objectpath '/org/a11y/atspi/null')" {
		t.Errorf("null ref %s", got)
	}
	if gvRefs(nil) != "@a(so) []" || gvStrings(nil) != "@as []" || gvDict(nil) != "@a{ss} {}" {
		t.Error("empty containers")
	}
	if got := gvDict(map[string]string{"b": "2", "a": "it's"}); got != `{'a': 'it\'s', 'b': '2'}` {
		t.Errorf("gvDict %s", got)
	}
	if gvDouble(3) != "double 3.0" || gvDouble(0.5) != "double 0.5" || gvDouble(1e21) != "double 1e+21" {
		t.Errorf("gvDouble %s %s %s", gvDouble(3), gvDouble(0.5), gvDouble(1e21))
	}
	if got := gvStates([2]uint32{5, 1}); got != "[uint32 5, 1]" {
		t.Errorf("gvStates %s", got)
	}
	if got := atspiEventArgs("focused", 1, 0, ""); got != "('focused', 1, 0, <0>, @a{sv} {})" {
		t.Errorf("event %s", got)
	}
	if got := atspiEventArgs("add", 2, 0, gvRef(":1.1", "/p")); got != "('add', 2, 0, <(':1.1', objectpath '/p')>, @a{sv} {})" {
		t.Errorf("event %s", got)
	}
}

func TestAtspiText(t *testing.T) {
	if runeSlice("héllo", 1, 3) != "él" || runeSlice("héllo", 2, -1) != "llo" || runeSlice("ab", 5, 9) != "" {
		t.Error("runeSlice")
	}
	if s, a, b := atspiStringAt("one two", 5, atspiGranularityWord); s != "two" || a != 4 || b != 7 {
		t.Errorf("word %q %d %d", s, a, b)
	}
	if s, a, b := atspiStringAt("héllo", 1, atspiGranularityChar); s != "é" || a != 1 || b != 2 {
		t.Errorf("char %q %d %d", s, a, b)
	}
	if s, a, b := atspiStringAt("x y", 0, 3); s != "x y" || a != 0 || b != 3 {
		t.Errorf("line %q %d %d", s, a, b)
	}
	pw := &a11y.Node{Role: a11y.RoleTextbox, Value: "secret", States: a11y.Protected, Actions: a11y.SetValue}
	if atspiText(pw) != "" || !atspiHasText(pw) || !atspiEditable(pw) {
		t.Error("password text")
	}
	if atspiText(&a11y.Node{Role: a11y.RoleText, Name: "hi"}) != "hi" {
		t.Error("static text")
	}
}

func TestAtspiInterfacesAndAttributes(t *testing.T) {
	slider := &a11y.Node{Role: a11y.RoleSlider, HasRange: true, Actions: a11y.Increment | a11y.Decrement | a11y.SetValue}
	got := strings.Join(atspiInterfaces(slider), ",")
	if got != "org.a11y.atspi.Accessible,org.a11y.atspi.Component,org.a11y.atspi.Action,org.a11y.atspi.Value" {
		t.Errorf("slider interfaces %s", got)
	}
	tr := a11y.NewTree(1)
	tr.Add(&a11y.Node{ID: 1, Role: a11y.RoleWindow, Children: []a11y.NodeID{2, 3}})
	tr.Add(&a11y.Node{ID: 2, Parent: 1, Role: a11y.RoleText, Name: "Email"})
	tr.Add(&a11y.Node{ID: 3, Parent: 1, Role: a11y.RoleTextbox, LabelledBy: 2, Controls: 99, Level: 0, PosInSet: 2, SetSize: 5, Placeholder: "you@x"})
	rel := atspiRelations(tr, tr.Node(3))
	if len(rel) != 1 || rel[0] != (atspiRelation{atspiRelationLabelledBy, 2}) {
		t.Errorf("relations %+v", rel)
	}
	live := &a11y.Node{Live: a11y.LiveAssertive}
	m := atspiAttributes(tr.Node(3), live)
	if m["posinset"] != "2" || m["setsize"] != "5" || m["placeholder-text"] != "you@x" || m["container-live"] != "assertive" || m["level"] != "" {
		t.Errorf("attributes %v", m)
	}
	if atspiPlugID(":1.9", "/p") != ":1.9:/p" {
		t.Error("plug id")
	}
}

func TestATSPITextDirection(t *testing.T) {
	ltr := &a11y.Node{Role: a11y.RoleTextbox}
	rtl := &a11y.Node{Role: a11y.RoleTextbox, RTL: true}
	if m := atspiTextAttributes(ltr); len(m) != 1 || m["direction"] != "ltr" {
		t.Errorf("ltr attributes %v", m)
	}
	if m := atspiTextAttributes(rtl); m["direction"] != "rtl" {
		t.Errorf("rtl attributes %v", m)
	}
	if got := gvDict(atspiTextAttributes(rtl)); !strings.Contains(got, "'direction'") || !strings.Contains(got, "'rtl'") {
		t.Errorf("gvariant %s", got)
	}
	for _, m := range []string{"GetAttributeValue", "GetAttributes", "GetAttributeRun", "GetDefaultAttributes", "GetDefaultAttributeSet"} {
		if !strings.Contains(atspiIntrospection, `<method name="`+m+`">`) {
			t.Errorf("introspection lacks Text.%s", m)
		}
	}
}
