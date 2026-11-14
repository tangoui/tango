package diffview

import (
	"math/rand/v2"
	"slices"
	"strings"
	"testing"

	"github.com/tangoui/tango/ui/editor/buffer"
	"github.com/tangoui/tango/ui/editor/diff"
)

func TestRevertEditRandom(t *testing.T) {
	r := rand.New(rand.NewPCG(5, 6))
	for iter := 0; iter < 2000; iter++ {
		a := randLines(r, 1+r.IntN(15))
		b := mutate(r, a)
		if iter%7 == 0 {
			b = []string{""} // revert into an empty document
		}
		if iter%11 == 0 {
			a = []string{""} // revert to an empty document
		}
		res := diffLines(a, b)
		orig := buffer.NewSnapshot(strings.Join(a, "\n"))
		mod := buffer.NewSnapshot(strings.Join(b, "\n"))
		for i, c := range res.Changes {
			e, err := revertEdit(res, i, orig, mod)
			if err != nil {
				t.Fatal(err)
			}
			ns, err := mod.Apply([]buffer.Edit{e})
			if err != nil {
				t.Fatalf("iter %d change %d: %v (edit %+v)", iter, i, err, e)
			}
			want := slices.Concat(b[:c.Modified.Start], a[c.Original.Start:c.Original.End], b[c.Modified.End:])
			if len(want) == 0 {
				want = []string{""}
			}
			if got := split(ns.String()); !slices.Equal(got, want) {
				t.Fatalf("iter %d: revert %v of\n%q\n%q\ngot  %q\nwant %q (edit %+v)", iter, c, a, b, got, want, e)
			}
		}
		// Reverting every change, last first, restores the original.
		cur := mod
		for i := len(res.Changes) - 1; i >= 0; i-- {
			e, _ := revertEdit(res, i, orig, cur)
			cur, _ = cur.Apply([]buffer.Edit{e})
		}
		if cur.String() != orig.String() {
			t.Fatalf("iter %d: reverting all gives %q, want %q", iter, cur.String(), orig.String())
		}
	}
}

func TestRevertEditErrors(t *testing.T) {
	a, b := split("a\nb"), split("a\nc")
	res := diffLines(a, b)
	orig, mod := buffer.NewSnapshot("a\nb"), buffer.NewSnapshot("a\nc")
	if _, err := revertEdit(res, 1, orig, mod); err == nil {
		t.Fatal("no error for a change index out of range")
	}
	if _, err := revertEdit(res, 0, orig, buffer.NewSnapshot("a")); err == nil {
		t.Fatal("no error for a stale result")
	}
}

func TestAnnounce(t *testing.T) {
	tx := announceTexts(nil)
	cases := []struct {
		c    diff.Change
		want string
	}{
		{diff.Change{Original: lr(39, 41), Modified: lr(39, 44)}, "第 3 处差异，共 12 处：删除第 40–41 行，新增第 40–44 行"},
		{diff.Change{Original: lr(4, 5), Modified: lr(4, 4)}, "第 3 处差异，共 12 处：删除第 5 行"},
		{diff.Change{Original: lr(4, 4), Modified: lr(4, 6)}, "第 3 处差异，共 12 处：新增第 5–6 行"},
	}
	for _, c := range cases {
		if got := announceChange(tx, 2, 12, c.c); got != c.want {
			t.Errorf("got %q, want %q", got, c.want)
		}
	}
	if got := announceChange(tx, 0, 0, diff.Change{}); got != "没有差异" {
		t.Errorf("no changes: %q", got)
	}
	// A locale overrides the formats by key.
	en := map[string]string{
		"diffview.a11y.change":   "Change %d of %d: ",
		"diffview.a11y.removed":  "removed line %s",
		"diffview.a11y.inserted": "added line %s",
		"diffview.a11y.sep":      ", ",
		"diffview.a11y.range":    "%d-%d",
	}
	tx = announceTexts(func(key, fallback string) string {
		if s, ok := en[key]; ok {
			return s
		}
		return fallback
	})
	if got, want := announceChange(tx, 0, 2, cases[0].c), "Change 1 of 2: removed line 40-41, added line 40-44"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}
