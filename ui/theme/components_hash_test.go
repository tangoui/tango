package theme

import "testing"

func TestComponentsHashNoAlloc(t *testing.T) {
	c := Components{Button: ButtonToken{PaddingInline: 20, PrimaryShadow: NoShadow}}
	c.Table.HeaderBg = Hex("#f0f5ff")
	var sink uint64
	if n := testing.AllocsPerRun(50, func() { sink ^= c.Hash() }); n != 0 {
		t.Fatalf("Hash allocates %v times per call", n)
	}
	_ = sink
	z := Components{Button: ButtonToken{PaddingInline: ZeroPx}}
	if z.Hash() == (Components{}).Hash() {
		t.Fatal("ZeroPx must hash differently from the zero value")
	}
	if c.Hash() == (Components{}).Hash() {
		t.Fatal("overrides must change the hash")
	}
}
