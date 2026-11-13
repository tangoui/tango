package input

import "testing"

func TestKeyForRuneRoundTrip(t *testing.T) {
	for r := 'a'; r <= 'z'; r++ {
		k := KeyForRune(r)
		if k == KeyUnknown || KeyForRune(r-'a'+'A') != k || k.Rune() != r {
			t.Fatalf("%c -> %v", r, k)
		}
	}
	for r := '0'; r <= '9'; r++ {
		if k := KeyForRune(r); k == KeyUnknown || k.Rune() != r {
			t.Fatalf("%c -> %v", r, k)
		}
	}
	// Earlier values are unchanged (hosts and the dev protocol send numbers).
	if KeyA != 15 || KeyY != 20 || KeyForRune('-') != KeyUnknown {
		t.Fatal("key values moved")
	}
}

func TestFocusOrderSkipsNoTab(t *testing.T) {
	var f FocusOrder[int]
	f.Add(1)
	f.AddNoTab(2)
	f.Add(3)
	if n, _ := f.Next(1, false); n != 3 {
		t.Fatalf("next %d", n)
	}
	if n, _ := f.Next(2, false); n != 3 {
		t.Fatalf("from no-tab %d", n)
	}
	if n, _ := f.Next(1, true); n != 3 {
		t.Fatalf("back %d", n)
	}
	if !f.Contains(2) {
		t.Fatal("contains")
	}
	var g FocusOrder[int]
	g.AddNoTab(5)
	if _, ok := g.Next(0, false); ok {
		t.Fatal("only no-tab ids")
	}
}
