package linux

import (
	"testing"

	"github.com/tangoui/tango/internal/imebridge/proto"
	"github.com/tangoui/tango/internal/platform"
)

func TestIBusModifierKeyval(t *testing.T) {
	for _, tc := range []struct {
		key, state uint32
		want       platform.IMEModifierKey
	}{
		{0xffe1, 0, platform.IMEModifierKey{Code: "ShiftLeft", Down: true, Shift: true}},
		{0xffe1, ibusReleaseMask | 1, platform.IMEModifierKey{Code: "ShiftLeft"}},
		{0xffe5, 2, platform.IMEModifierKey{Code: "CapsLock", Down: true, CapsLock: true}},
	} {
		got, ok := ibusModifier(tc.key, tc.state)
		if !ok || got != tc.want {
			t.Errorf("modifier %x/%x = %+v, %v", tc.key, tc.state, got, ok)
		}
	}
}

func TestIBusCapsState(t *testing.T) {
	if !ibusKey('A', 30, 2, 'A').CapsLock {
		t.Fatal("CapsLock lost")
	}
}

func TestIBusLookupTablePaging(t *testing.T) {
	table := newIBusLookup(make([]platform.IMECandidate, 12), 10)
	if table.pageSize != 9 || table.cursor/table.pageSize != 1 || table.cursor%table.pageSize != 1 || !table.cursorVisible {
		t.Fatalf("table = %+v", table)
	}
	if newIBusLookup(make([]platform.IMECandidate, 12), -1).cursorVisible {
		t.Fatal("unset cursor visible")
	}
}

func TestIBusCandidateClickIndex(t *testing.T) {
	table := newIBusLookup(make([]platform.IMECandidate, 12), 10)
	if index, ok := table.clickedIndex(1); !ok || index != 10 {
		t.Fatalf("second candidate on page two = %d, %t", index, ok)
	}
	if _, ok := table.clickedIndex(3); ok {
		t.Fatal("accepted a click past the end of the last page")
	}
	table = newIBusLookup(make([]platform.IMECandidate, 19), -1)
	if _, ok := table.clickedIndex(9); ok {
		t.Fatal("accepted a click outside the displayed page")
	}
}

func TestIBusPropsFromModes(t *testing.T) {
	modes := []proto.Mode{{ID: "zh", Label: "中文", Short: "中"}, {ID: "en", Label: "英文", Short: "英"}}
	menu := []proto.MenuItem{{ID: "toggle", Title: "切换", Checked: true}, {Separator: true}, {ID: "about", Title: "关于"}}
	got := ibusProps(modes, "en", menu)
	if len(got) != 2 || got[0].kind != ibusPropNormal || got[0].label != "英" ||
		got[1].kind != ibusPropMenu || len(got[1].children) != 5 {
		t.Fatalf("properties = %+v", got)
	}
	if choice := got[1].children[1]; choice.key != "mode:en" || choice.label != "英文" || !choice.checked {
		t.Fatalf("selected property = %+v", choice)
	}
	if about := got[1].children[4]; about.key != "menu:about" || about.label != "关于" {
		t.Fatalf("about = %+v", about)
	}
}
