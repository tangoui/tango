package updater

import (
	"bytes"
	"math/rand/v2"
	"slices"
	"testing"
)

func naiveSuffixArray(b []byte) []int32 {
	sa := make([]int32, len(b))
	for i := range sa {
		sa[i] = int32(i)
	}
	slices.SortFunc(sa, func(x, y int32) int { return bytes.Compare(b[x:], b[y:]) })
	return sa
}

func TestSuffixArray(t *testing.T) {
	r := rand.New(rand.NewPCG(1, 2))
	inputs := [][]byte{nil, []byte("a"), []byte("aa"), []byte("banana"), []byte("mississippi"),
		bytes.Repeat([]byte("ab"), 50), bytes.Repeat([]byte{0}, 100), []byte("abracadabra abracadabra")}
	for range 300 {
		n := r.IntN(200)
		alpha := 1 + r.IntN(4)
		if r.IntN(3) == 0 {
			alpha = 256
		}
		b := make([]byte, n)
		for i := range b {
			b[i] = byte(r.IntN(alpha))
		}
		inputs = append(inputs, b)
	}
	for _, in := range inputs {
		if got, want := suffixArray(in), naiveSuffixArray(in); !slices.Equal(got, want) {
			t.Fatalf("suffixArray(%q) = %v, want %v", in, got, want)
		}
	}
}

func TestDiffPatchRoundTrip(t *testing.T) {
	r := rand.New(rand.NewPCG(3, 4))
	random := func(n int) []byte {
		b := make([]byte, n)
		for i := range b {
			b[i] = byte(r.Uint32())
		}
		return b
	}
	base := random(50000)
	edited := slices.Clone(base)
	for range 40 { // scattered byte edits
		edited[r.IntN(len(edited))] ^= 0x5a
	}
	edited = slices.Insert(edited, 20000, random(3000)...) // an insertion
	edited = slices.Delete(edited, 40000, 41000)           // a deletion
	cases := []struct {
		name     string
		old, new []byte
	}{
		{"empty to data", nil, []byte("hello")},
		{"data to empty", []byte("hello"), nil},
		{"identical", base, base},
		{"edited", base, edited},
		{"unrelated", random(4000), random(5000)},
		{"repetitive", bytes.Repeat([]byte("abc"), 5000), bytes.Repeat([]byte("abcd"), 4000)},
	}
	for _, c := range cases {
		p, err := Diff(c.old, c.new)
		if err != nil {
			t.Fatalf("%s: %v", c.name, err)
		}
		got, err := Patch(c.old, p)
		if err != nil {
			t.Fatalf("%s: Patch: %v", c.name, err)
		}
		if !bytes.Equal(got, c.new) {
			t.Fatalf("%s: round trip differs", c.name)
		}
		t.Logf("%s: %d → %d bytes, patch %d", c.name, len(c.old), len(c.new), len(p))
	}
	p, _ := Diff(base, edited)
	if len(p) > len(edited)/5 {
		t.Errorf("patch for a lightly edited file is %d bytes of %d", len(p), len(edited))
	}
	if _, err := Patch(edited, p); err != ErrPatchBase {
		t.Errorf("patch on the wrong base: %v", err)
	}
	bad := slices.Clone(p)
	bad[len(bad)-3] ^= 0xff
	if _, err := Patch(base, bad); err == nil {
		t.Error("corrupt patch applied")
	}
}
