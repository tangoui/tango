package monarch

import "testing"

func TestBacktrackLiteralFirstNestedAlternatives(t *testing.T) {
	for _, tc := range []struct {
		source string
		first  byte
		ok     bool
	}{
		{`a(?=b|c)`, 'a', true},
		{`\/(?=x|y)`, '/', true},
		{`a[|](?=b)`, 'a', true},
		{`a\|b(?=c)`, 'a', true},
		{`a(?:b|c)(?=d)`, 'a', true},
		{`a[()|](?=b)`, 'a', true},
		{`^a(?=b|c)`, 'a', true},
		{`\((?=a|b)`, '(', true},
		{`a[)](?=b)`, 'a', true},
		{`a(?:\)|b)(?=c)`, 'a', true},
		{`a(?:b|)(?=c)`, 'a', true},
		{"a(?x)# [\n|b# ]\n", 0, false},
		{`a(?# [)|b(?# ])`, 0, false},
		{`a(?i:b|c)`, 0, false},
		{`a(?>b|c)`, 0, false},
		{`a(?=b)|c`, 0, false},
		{`a[|](?=b)|c`, 0, false},
		{`a?b(?=c)`, 0, false},
		{`a*(?=b)`, 0, false},
		{`a{0,2}(?=b)`, 0, false},
		{`(a|b)(?=c)`, 0, false},
		{`é(?=x)`, 0, false},
	} {
		if first, ok := backtrackLiteralFirst(tc.source); first != tc.first || ok != tc.ok {
			t.Fatalf("%q: %q/%v, want %q/%v", tc.source, first, ok, tc.first, tc.ok)
		}
	}
}
