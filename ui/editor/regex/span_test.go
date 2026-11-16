package regex

import (
	"errors"
	"reflect"
	"regexp"
	"strings"
	"sync"
	"testing"
)

func TestNoCaptureSpansPreserveLiteralCases(t *testing.T) {
	for _, tc := range []struct {
		source, flags, input string
		want                 []int
	}{
		{`"([^"]*)"`, "", `"a b" tail`, []int{0, 5}},
		{`[a-z]+`, "i", "AbC!", []int{0, 3}},
		{`a*ab`, "", "aaab!", []int{0, 4}},
		{`[ab]*a?b`, "", "aaab!", []int{0, 4}},
		{`a+?`, "", "aaa", []int{0, 1}},
		{`a*$`, "", "aaab", nil},
		{`a?b`, "", "b!", []int{0, 1}},
		{`a{2,4}b`, "", "aaab!", []int{0, 4}},
		{`(a*)*`, "", "aaab", []int{0, 3}},
		{`a*`, "", "b", []int{0, 0}},
		{`[a-z]+`, "i", "aKſ!", []int{0, 6}},
		{`(?=(é😀))é`, "", "é😀!", []int{0, 2}},
		{`(é)(x)?(?<n>中)(?=!)`, "", "é中!", []int{0, 5}},
		{`(?=.)`, "", "\xff", []int{0, 0}},
	} {
		re := mustCompile(t, tc.source, tc.flags)
		got, err := TryMatchStartNoCaptures(re, tc.input, nil, make([]int, 0, 2))
		if err != nil || !reflect.DeepEqual(got, tc.want) {
			t.Fatalf("%q flags %q on %q: %v/%v want %v", tc.source, tc.flags, tc.input, got, err, tc.want)
		}
	}
}

func TestSpanAndFirstFilterMatchNativeRE2(t *testing.T) {
	patterns := []string{
		``, `a`, `(a)`, `ab`, `a?b`, `a+ab`, `a*ab`, `a*b`, `a{2,4}b`, `a{2}a`, `a{0}b`,
		`a+?`, `a*?b`, `(a*)*`, `[a-z]+`, `[^<]+`, `[a-z_$][\w$]*`, `#?[a-z_$][\w$]*`,
		`"([^"]*)"`, `([a-z]+)(:)?`, `a|bc`, `(a|)b`, `(ab)?ab`, `(?:a|b)+c`,
		`^a`, `a$`, `\ba`, `a\b`, `\Bfoo`, `\s+`, `\S*`, `\w+`, `\W*`, `\d{1,3}`, `.`, `.*`,
		`[A-Z]+`, `[^A-Z]+`, `[^a-z]+`, `K+`, `ſ+`, `é+`, `[éa]+`, `(é)(x)?(?<n>中)`,
		strings.Repeat("a?", 70), strings.Repeat("b", 600),
	}
	inputs := []string{"", "a", "b", "ab", "aaab!", "abc: ", "ababc", "foo", "a\nb", "a\r\nb", "a\u2028", "a\u2029", "é", "aKſ!", "ſ", "K", "😀a", "\xff", "a\xff", "\u3000x", "\"abc\"", "#xyz"}
	var seed uint32 = 4719
	parts := []string{"a", "b", "z", "A", "S", "K", "#", "_", " ", "\n", "\r", "é", "K", "ſ", "😀", "\xff"}
	for n := 0; n < 240; n++ {
		s := ""
		for range n % 9 {
			seed = seed*1664525 + 1013904223
			s += parts[int(seed>>8)%len(parts)]
		}
		inputs = append(inputs, s)
	}
	for _, source := range patterns {
		for _, flags := range []string{"", "i", "m", "s", "iu"} {
			translated, ok := Translate(source, flags)
			if !ok {
				continue
			}
			ref, err := regexp.Compile(anchorStd(translated))
			if err != nil {
				t.Fatal(err)
			}
			re := mustCompile(t, source, flags)
			for _, input := range inputs {
				want := ref.FindStringSubmatchIndex(input)
				got, err := re.TryMatchStart(input, nil)
				if err != nil || !reflect.DeepEqual(got, want) {
					t.Fatalf("full %q/%q on %q: %v/%v want %v", source, flags, input, got, err, want)
				}
				if !reflect.DeepEqual(re.MatchStart(input), want) || !reflect.DeepEqual(re.MatchStartRunes(input, nil), want) {
					t.Fatalf("public capture methods disagree: %q/%q on %q", source, flags, input)
				}
				span := want
				if span != nil {
					span = span[:2]
				}
				got, err = TryMatchStartNoCaptures(re, input, []rune(input), make([]int, 0, 2))
				if err != nil || !reflect.DeepEqual(got, span) {
					t.Fatalf("span %q/%q on %q: %v/%v want %v", source, flags, input, got, err, span)
				}
			}
		}
	}
}

func TestSpanScratchPreservesFullCaptureOwnership(t *testing.T) {
	re := mustCompile(t, `(a+)(b)?`, "")
	full := re.MatchStart("aab")
	var storage [2]int
	if _, err := TryMatchStartNoCaptures(re, "aaa", nil, storage[:0]); err != nil {
		t.Fatal(err)
	}
	storage[0], storage[1] = 9, 9
	if !reflect.DeepEqual(full, []int{0, 3, 0, 2, 2, 3}) {
		t.Fatalf("full capture result was overwritten: %v", full)
	}
}

func TestSpanProgramsAreConcurrent(t *testing.T) {
	re := mustCompile(t, `"([^"]*)"`, "i")
	var wg sync.WaitGroup
	for range 12 {
		wg.Go(func() {
			var dst [2]int
			for range 100 {
				m, err := TryMatchStartNoCaptures(re, `"hello"`, nil, dst[:0])
				if err != nil || !reflect.DeepEqual(m, []int{0, 7}) {
					t.Errorf("concurrent span: %v, %v", m, err)
					return
				}
			}
		})
	}
	wg.Wait()
}

func TestNoCaptureMatchRetainsTimeoutErrors(t *testing.T) {
	re := mustCompile(t, `(a+)+(?=b)`, "")
	_, _, before := Stats()
	m, err := TryMatchStartNoCaptures(re, strings.Repeat("a", 60)+"c", nil, make([]int, 0, 2))
	if m != nil || !errors.Is(err, ErrTimeout) {
		t.Fatalf("got %v/%v, want no match and ErrTimeout", m, err)
	}
	if _, _, after := Stats(); after != before+1 {
		t.Fatalf("confirmed timeout counted %d times, want one", after-before)
	}
}
