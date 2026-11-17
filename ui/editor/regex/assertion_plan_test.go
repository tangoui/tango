package regex

import (
	"reflect"
	"testing"
)

func TestASCIIAssertionPlanMatchesNative(t *testing.T) {
	for _, tc := range []struct {
		source string
		input  string
		want   []int
	}{
		{"a(?=b)", "ab", []int{0, 1}},
		{"a(?=b)", "ac", nil},
		{"a(?!b)", "ac", []int{0, 1}},
		{"a(?!b)", "ab", nil},
		{`a\.(?=b)`, "a.b", []int{0, 2}},
		{"!(?=([^=]|$))", "!x", []int{0, 1}},
		{"!(?=([^=]|$))", "!=", nil},
		{"!(?=([^=]|$))", "!", []int{0, 1}},
	} {
		re := mustCompile(t, tc.source, "")
		got, err := TryMatchStartNoCaptures(re, tc.input, nil, nil)
		if err != nil || !reflect.DeepEqual(got, tc.want) {
			t.Fatalf("%q on %q: got %v/%v, want %v", tc.source, tc.input, got, err, tc.want)
		}
	}
}

func TestASCIIAssertionPlanRejectsComplexForms(t *testing.T) {
	for _, source := range []string{`a(?=b|c)`, `a(?=(b))`, `é(?=x)`, `a(?=b)` + ""} {
		re := mustCompile(t, source, "")
		br, ok := re.(*backtrackRegexp)
		if !ok {
			t.Fatalf("%q unexpectedly uses standard engine", source)
		}
		if source != `a(?=b)` && br.assertion != nil {
			t.Fatalf("%q received an unsafe assertion plan", source)
		}
	}
}

func TestASCIILiteralUsesDirectEngine(t *testing.T) {
	for _, source := range []string{"<", `\/script`, "abc"} {
		re := mustCompile(t, source, "")
		if _, ok := re.(*literalRegexp); !ok {
			t.Fatalf("%q engine = %T, want *literalRegexp", source, re)
		}
	}
}
