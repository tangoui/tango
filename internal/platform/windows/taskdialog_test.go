package windows

import (
	"encoding/binary"
	"testing"
)

// TestTaskDialogConfigLayout checks the packed TASKDIALOGCONFIG offsets
// (64-bit, commctrl.h) member by member.
func TestTaskDialogConfigLayout(t *testing.T) {
	c := taskDialogConfig{
		Parent: 0x1111, Instance: 0x2222, Flags: 0x3333, WindowTitle: 0x4444,
		MainIcon: 0x5555, MainInstruction: 0x6666, Content: 0x7777,
		ButtonCount: 3, Buttons: 0x8888, DefaultButton: 100,
	}
	b := c.encode()
	if len(b) != 160 {
		t.Fatalf("size = %d, want 160", len(b))
	}
	le := binary.LittleEndian
	for _, f := range []struct {
		name string
		off  int
		size int
		want uint64
	}{
		{"cbSize", 0, 4, 160},
		{"hwndParent", 4, 8, 0x1111},
		{"hInstance", 12, 8, 0x2222},
		{"dwFlags", 20, 4, 0x3333},
		{"dwCommonButtons", 24, 4, 0},
		{"pszWindowTitle", 28, 8, 0x4444},
		{"pszMainIcon", 36, 8, 0x5555},
		{"pszMainInstruction", 44, 8, 0x6666},
		{"pszContent", 52, 8, 0x7777},
		{"cButtons", 60, 4, 3},
		{"pButtons", 64, 8, 0x8888},
		{"nDefaultButton", 72, 4, 100},
	} {
		var got uint64
		if f.size == 4 {
			got = uint64(le.Uint32(b[f.off:]))
		} else {
			got = le.Uint64(b[f.off:])
		}
		if got != f.want {
			t.Errorf("%s at %d = %#x, want %#x", f.name, f.off, got, f.want)
		}
	}
	for i := 76; i < 160; i++ {
		if b[i] != 0 {
			t.Fatalf("byte %d = %#x, want 0 (unused members)", i, b[i])
		}
	}

	btns := taskDialogButtons(100, []uintptr{0xAA, 0xBB})
	if len(btns) != 24 || le.Uint32(btns[0:]) != 100 || le.Uint64(btns[4:]) != 0xAA ||
		le.Uint32(btns[12:]) != 101 || le.Uint64(btns[16:]) != 0xBB {
		t.Errorf("buttons = % x", btns)
	}
}
