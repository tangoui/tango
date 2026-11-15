package monarch

import "testing"

func TestBacktrackLiteralFirst(t *testing.T) {
	for _, tc := range []struct {
		src  string
		want byte
	}{
		{`\/\*\*(?!\/)`, '/'},
		{`foo(?=x)`, 'f'},
	} {
		got, ok := backtrackLiteralFirst(tc.src)
		if !ok || got != tc.want {
			t.Fatalf("backtrackLiteralFirst(%q) = %q, %v; want %q, true", tc.src, got, ok, tc.want)
		}
	}
	for _, src := range []string{`[<>](?!(?:x))`, `\w+`, `[^/]`} {
		if _, ok := backtrackLiteralFirst(src); ok {
			t.Fatalf("backtrackLiteralFirst(%q) unexpectedly safe", src)
		}
	}
}
