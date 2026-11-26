package windows

import (
	"testing"

	"github.com/tangoui/tango/internal/platform"
	"github.com/tangoui/tango/ui/input"
)

func TestWindowsWebKey(t *testing.T) {
	k, ok := planWebKey(input.KeyK, input.ModShift)
	if !ok || k.vk != 'K' || k.key != "K" || k.code != "KeyK" || k.mods != 8 || k.text != "K" {
		t.Fatalf("Shift+K: %+v %v", k, ok)
	}
	d := k.event(true, "")
	u := k.event(false, "")
	if d["type"] != "keyDown" || d["text"] != "K" || d["unmodifiedText"] != "k" || u["type"] != "keyUp" || u["text"] != nil {
		t.Fatalf("down=%v up=%v", d, u)
	}
	k, ok = planWebKey(input.KeyNumpadEnter, 0)
	if !ok || k.vk != 0x0d || k.code != "NumpadEnter" || k.event(true, "")["isKeypad"] != true {
		t.Fatalf("NumpadEnter: %+v %v", k, ok)
	}
	k, _ = planWebKey(input.KeyA, input.ModCtrl)
	if d := k.event(true, "selectAll"); d["type"] != "rawKeyDown" || d["text"] != nil || d["commands"].([]string)[0] != "selectAll" {
		t.Fatalf("edit event: %v", d)
	}
}

func TestWindowsWebKeyMenu(t *testing.T) {
	p := buildMenuPlan([]platform.MenuItem{
		{ID: 1, Label: "Save", Accelerator: "CmdOrCtrl+S"},
		{ID: 2, Label: "Disabled", Accelerator: "CmdOrCtrl+B", Disabled: true},
		{Role: "editMenu"},
	}, "Test")
	for _, tc := range []struct {
		key     input.Key
		mods    input.Modifiers
		handled bool
		edit    string
	}{
		{input.KeyS, input.ModCtrl, true, ""},
		{input.KeyS, input.ModCtrl | input.ModMeta, false, ""},
		{input.KeyS, input.ModCtrl | input.ModShift, false, ""},
		{input.KeyB, input.ModCtrl, false, ""},
		{input.KeyA, input.ModCtrl, false, "selectAll"},
		{input.KeyC, input.ModCtrl, false, "copy"},
		{input.KeyV, input.ModCtrl, false, "paste"},
	} {
		k, _ := planWebKey(tc.key, tc.mods)
		cmd, edit := k.menu(p)
		if (cmd != 0) != tc.handled || edit != tc.edit {
			t.Errorf("%v %v: command %d edit %q", tc.key, tc.mods, cmd, edit)
		}
	}
}
