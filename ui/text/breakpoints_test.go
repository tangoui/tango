package text

import "testing"

func TestReflowReusesLineBreakOpportunities(t *testing.T) {
	clearCache()
	const s = "office window renderer text frame paragraph glyph layout office window"
	st := Style{Size: 13}.norm()
	chain := reg.chain(false)
	rs := []runStyle{{st: st, chain: chain, lang: resolveLang(st)}}
	mu.Lock()
	defer mu.Unlock()
	p := prepareText(s, rs, nil, nil, Options{MaxWidth: 150})
	a := p.lineEnds(0)
	b := p.lineEnds(0)
	if len(a) == 0 || len(a) != len(b) || &a[0] != &b[0] {
		t.Fatal("reflow rebuilt line break opportunities")
	}
	for _, w := range []float32{70, 150, 300} {
		got := breakAt(p.runes, 0, p.adv, w, a)
		want := breakLines(p.runes, 0, len(p.runes), p.adv, w)
		if len(got) != len(want) {
			t.Fatalf("width%v: line count changed", w)
		}
		for i := range got {
			if got[i] != want[i] {
				t.Fatalf("width%v: line %d changed", w, i)
			}
		}
	}
}
