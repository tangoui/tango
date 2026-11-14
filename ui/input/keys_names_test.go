package input

import "testing"

func TestKeyValuesStableAll(t *testing.T) {
	// Hosts and recorded sessions depend on these numbers.
	old := []Key{KeyUnknown, KeyEnter, KeyTab, KeyEscape, KeyBackspace, KeyDelete, KeyLeft, KeyRight, KeyUp, KeyDown,
		KeyHome, KeyEnd, KeyPageUp, KeyPageDown, KeySpace, KeyA, KeyC, KeyV, KeyX, KeyZ, KeyY}
	for i, k := range old {
		if int(k) != i {
			t.Fatalf("%v = %d, want %d", k, k, i)
		}
	}
}

func TestKeyNames(t *testing.T) {
	seen := map[string]Key{}
	for k := KeyUnknown; k < keyCount; k++ {
		n := k.String()
		if n == "" || n[0] == 'K' && len(n) > 3 && n[:4] == "Key(" {
			t.Fatalf("key %d has no name", k)
		}
		if o, dup := seen[n]; dup {
			t.Fatalf("%q used by %d and %d", n, o, k)
		}
		seen[n] = k
		if back, ok := KeyNamed(n); !ok || back != k {
			t.Fatalf("KeyNamed(%q) = %v", n, back)
		}
	}
	if Key(9999).String() != "Key(9999)" {
		t.Fatal(Key(9999).String())
	}
	cases := map[Key]string{KeyB: "B", Key7: "7", KeyF12: "F12", KeyMetaRight: "MetaRight", KeyBackquote: "Backquote", KeyNumpadEnter: "NumpadEnter"}
	for k, w := range cases {
		if k.String() != w {
			t.Fatalf("%d = %q want %q", k, k.String(), w)
		}
	}
}

func TestKeyHelpers(t *testing.T) {
	if KeyForRune('5') != Key5 || Key5.Rune() != '5' || KeyNumpad5.Rune() != '5' {
		t.Fatal("digit")
	}
	if KeyAltRight.Modifier() != ModAlt || KeyShiftLeft.Modifier() != ModShift || KeyA.Modifier() != 0 {
		t.Fatal("Modifier")
	}
	if KeyNumpadEnter.Base() != KeyEnter || KeyNumpad3.Base() != Key3 || KeyNumpadAdd.Base() != KeyNumpadAdd {
		t.Fatal("Base")
	}
	if KeyBackslash.Rune() != '\\' || KeyNumpadMultiply.Rune() != '*' || KeyF1.Rune() != 0 {
		t.Fatal("Rune")
	}
}

// KeyBack marks Android's system Back (Key KeyEscape, Code KeyBack). It was
// appended after KeyContextMenu and is its own Base.
func TestKeyBack(t *testing.T) {
	if KeyBack != KeyContextMenu+1 {
		t.Fatalf("KeyBack = %d, want %d", KeyBack, KeyContextMenu+1)
	}
	if KeyBack.String() != "Back" || KeyBack.Base() != KeyBack || KeyBack.Rune() != 0 {
		t.Fatalf("KeyBack: %q base %v", KeyBack.String(), KeyBack.Base())
	}
}
