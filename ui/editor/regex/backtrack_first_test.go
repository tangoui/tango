package regex

import (
	"reflect"
	"strings"
	"testing"

	"github.com/dlclark/regexp2"
)

func nativeCaptureBytes(m *regexp2.Match, nums []int, input string) []int {
	if m == nil {
		return nil
	}
	var offsets []int
	for b := range input {
		offsets = append(offsets, b)
	}
	offsets = append(offsets, len(input))
	result := []int{offsets[m.Index], offsets[m.Index+m.Length]}
	for _, n := range nums {
		g := m.GroupByNumber(n)
		if g == nil || len(g.Captures) == 0 {
			result = append(result, -1, -1)
		} else {
			result = append(result, offsets[g.Index], offsets[g.Index+g.Length])
		}
	}
	return result
}

func TestBacktrackFirstFilterMatchesNative(t *testing.T) {
	patterns := []string{
		`[a-z](?=:)`, `[^A-Z]+(?=!)`, `[A-Z]+(?=!)`, `[a-z]+(?=!)`,
		`(?=a)a`, `(?!a)b`, `a(?<=a)b`, `(?<!a)b`, `(?<=a)b`,
		`a?(?=b)b`, `a*(?=b)b`, `[a-z]?(?=:)`, `(?=a)`, `(?!a)`,
		`(?:a|b)(?=:)`, `a(?=b|c)`, `a(?=b)|c`, `\/(?=x|y)`,
		`(?=(a))\1`, `(a)(b)?(?=!)`, `(?<x>a)(b)?\k<x>`,
		`(?=(é😀))é`, `(.)(?=(.))`, `\b[a-z]+(?=:)`, `\B(?=a)a`,
		`K(?=x)`, `ſ(?=x)`, `İ(?=x)`, `(?:é|a)(?=x)`, `^(?=a)a`,
	}
	inputs := []string{"", "a", "b", "ab", "ac", "c", "a:", "A:", "abc!", "ABC!", "!!", "aa!", "/x", "/y", "é😀", "éx", "Kx", "kx", "ſx", "Sx", "ix", "İx", "\xffx", "a\xff", "\n", "a\nb", "\x00:"}
	parts := []string{"a", "b", "c", "A", "S", "K", "!", ":", "/", " ", "\n", "é", "😀", "K", "ſ", "\xff"}
	var seed uint32 = 6831
	for n := range 140 {
		s := ""
		for range n % 8 {
			seed = seed*1664525 + 1013904223
			s += parts[int(seed>>8)%len(parts)]
		}
		inputs = append(inputs, s)
	}
	for _, source := range patterns {
		for _, flags := range []string{"", "i", "m", "s", "iu"} {
			re := mustCompile(t, source, flags)
			br, ok := re.(*backtrackRegexp)
			if !ok {
				t.Fatalf("%q unexpectedly avoided backtracking", source)
			}
			fl, err := parseFlags(flags)
			if err != nil {
				t.Fatal(err)
			}
			translated, _, err := translate(source, fl, toBacktrack)
			if err != nil {
				t.Fatal(err)
			}
			opts := regexp2.RegexOptions(regexp2.ECMAScript)
			if fl.i {
				opts |= regexp2.IgnoreCase
			}
			// Independent unfiltered engine, with no clock deadline for these
			// bounded inputs. The wrapper's timeout tests remain separate.
			native := regexp2.MustCompile(anchorBacktrack(translated), opts)
			for _, input := range inputs {
				m, err := native.FindStringMatch(input)
				if err != nil {
					t.Fatal(err)
				}
				want := nativeCaptureBytes(m, br.nums, input)
				got, err := re.TryMatchStart(input, nil)
				if err != nil || !reflect.DeepEqual(got, want) ||
					!reflect.DeepEqual(re.MatchStart(input), want) ||
					!reflect.DeepEqual(re.MatchStartRunes(input, []rune(input)), want) {
					t.Fatalf("%q/%q on %q: got %v/%v want %v", source, flags, input, got, err, want)
				}
				if want != nil {
					want = want[:2]
				}
				var dst [2]int
				got, err = TryMatchStartNoCaptures(re, input, []rune(input), dst[:0])
				if err != nil || !reflect.DeepEqual(got, want) {
					t.Fatalf("span %q/%q on %q: got %v/%v want %v", source, flags, input, got, err, want)
				}
			}
		}
	}
}

func TestLongBacktrackFirstFilterKeepsNativePath(t *testing.T) {
	source := strings.Repeat("a", 600) + `(?=(b))`
	re := mustCompile(t, source, "")
	input := strings.Repeat("a", 600) + "b"
	got, err := re.TryMatchStart(input, nil)
	if err != nil || !reflect.DeepEqual(got, []int{0, 600, 600, 601}) {
		t.Fatalf("long expression captures=%v err=%v", got, err)
	}
}
