package when

import (
	"strings"
	"testing"
)

// TestWhenParseEval: the cases of MinCode's scripts/command-when.test.mjs
// (transcribed), VS Code operator precedence and the operators MinCode's
// dialect lacks.
func TestWhenParseEval(t *testing.T) {
	// MinCode "when parser accepts the supported dialect".
	for _, expr := range []string{
		"connected",
		"!connected",
		`mode == "debug"`,
		`mode != "debug" && connected`,
		"a && b && c",
	} {
		if _, err := Parse(expr); err != nil {
			t.Errorf("Parse(%q): %v", expr, err)
		}
	}

	// MinCode "when parser rejects unsupported syntax", minus the cases
	// VS Code's grammar accepts ("connected || ready", "mode == debug",
	// "Connected", more than 8 clauses; see TestWhenVSCodeAccepts).
	for _, expr := range []string{
		"",
		"&&",
		"connected &&",
		"&& connected",
		`mode = "debug"`,
		`mode == "unterminated`,
		"!",
		strings.Repeat("a", MaxLength+1),
		// VS Code grammar errors.
		"a b",
		"(a",
		"a )",
		"a ||",
		"a & b",
		"a | b",
		"a ==",
		"a =~ b",
		"a =~ /x",
		"a in",
		// "!" takes only a key, true, false or ( ... ) (VS Code's _term).
		"!lang == 'go'",
		"!!a",
		"!a == b",
		"!a =~ /x/",
		"!a in b",
		"!'x'",
	} {
		if _, err := Parse(expr); err == nil {
			t.Errorf("Parse(%q) accepted", expr)
		}
	}

	// MinCode "when evaluation mirrors host semantics".
	ctx := MapContext{"connected": true, "mode": "debug", "count": 0}
	cases := []struct {
		expr string
		want bool
	}{
		{"connected", true},
		{"!connected", false},
		{"missing", false},
		{"!missing", true},
		{"count", false},
		{`mode == "debug"`, true},
		{`mode == "release"`, false},
		{`mode != "release"`, true},
		{`missing != "release"`, true},
		{`connected == "true"`, false},
		{`connected && mode == "debug"`, true},
		{`connected && mode == "release"`, false},
		{"connected && missing", false},
	}
	for _, c := range cases {
		if got := Eval(c.expr, ctx); got != c.want {
			t.Errorf("Eval(%q) = %v, want %v", c.expr, got, c.want)
		}
	}

	// MinCode "malformed when expressions hide the command".
	if Eval("connected ||", MapContext{"connected": true}) {
		t.Error(`Eval("connected ||") = true`)
	}
	if !Eval("", nil) || !Eval("   ", nil) {
		t.Error("blank expression is not true")
	}

	// VS Code semantics and precedence.
	vs := MapContext{
		"a": true, "b": false, "c": true,
		"lang": "go", "n": 3, "f": 2.5, "s": "", "z": 0,
		"ext":   ".ts",
		"list":  []string{"go", "rust"},
		"anys":  []any{"x", 1},
		"set":   map[string]bool{"go": true},
		"obj":   map[string]any{"k": nil},
		"other": map[string]int{"go": 1},
	}
	prec := []struct {
		expr string
		want bool
	}{
		{"!a && b || c", true},    // ((!a) && b) || c
		{"!a && (b || c)", false}, // explicit grouping
		{"a || b && false", true}, // && binds tighter
		{"(a || b) && false", false},
		{"!(a && c)", false},
		{"true", true},
		{"false", false},
		{"!false && a", true},
		{"lang == go", true},
		{"lang == 'go'", true},
		{"lang === 'go'", true},
		{"lang !== 'go'", false},
		{"lang != 'rust'", true},
		{"!(lang == 'go')", false},
		{"!(a) && c", false},
		{"!true || c", true},
		{"a == true", true},
		{"b == true", false},
		{"b == false", true},
		{"a != true", false},
		{"missing == false", true},
		{"a == 'true'", false}, // quoted: a string, never equal to a boolean
		{"a != 'true'", true},
		{"lang == 'true'", false},
		{"n == 3", true},
		{"n == '3'", true},
		{"n > 2", true},
		{"n >= 3", true},
		{"n < 3", false},
		{"n <= 3", true},
		{"f > 2", true},
		{"f < 2.6", true},
		{"lang > 1", false},
		{"n > x", false},
		{"missing < 1", false},
		{"s", false},
		{"z", false},
		{"!s && !z", true},
		{"ext == .ts", true},
		{"lang in list", true},
		{"lang not in list", false},
		{"ext in list", false},
		{"ext not in list", true},
		{"lang in set", true},
		{"lang in other", true},
		{"missing in list", false},
		{"lang in missing", false},
		{"n in anys", false},
		{"lang in obj", false},
		{"editor.lang in list", false},
		{"a && lang == go && n > 1", true},
		{"a&&c", true},
		{"a||b", true},
	}
	for _, c := range prec {
		e, err := Parse(c.expr)
		if err != nil {
			t.Errorf("Parse(%q): %v", c.expr, err)
			continue
		}
		if got := e.Eval(vs); got != c.want {
			t.Errorf("%q = %v, want %v", c.expr, got, c.want)
		}
		// String round trip: same meaning, stable form.
		e2, err := Parse(e.String())
		if err != nil {
			t.Errorf("Parse(String(%q) = %q): %v", c.expr, e.String(), err)
			continue
		}
		if e2.String() != e.String() || e2.Eval(vs) != c.want {
			t.Errorf("round trip of %q: %q -> %q", c.expr, e.String(), e2.String())
		}
	}
	if Eval("a", nil) {
		t.Error("nil context has keys")
	}
}

// TestWhenVSCodeAccepts: expressions MinCode's plugin dialect rejects but
// VS Code's grammar (and so this package) accepts.
func TestWhenVSCodeAccepts(t *testing.T) {
	ctx := MapContext{"connected": true, "Connected": true}
	for _, c := range []struct {
		expr string
		want bool
	}{
		{"connected || ready", true},
		{"mode == debug", false},
		{"Connected", true},
		{"k0 && k1 && k2 && k3 && k4 && k5 && k6 && k7 && k8", false},
	} {
		if got := Eval(c.expr, ctx); got != c.want {
			t.Errorf("Eval(%q) = %v, want %v", c.expr, got, c.want)
		}
	}
}

func TestWhenRegex(t *testing.T) {
	ctx := MapContext{
		"resourceFilename": "Dockerfile.dev",
		"path":             "src/a/b.test.ts",
		"lang":             "typescript",
		"n":                42,
	}
	for _, c := range []struct {
		expr string
		want bool
	}{
		{"resourceFilename =~ /docker/", false}, // case-sensitive
		{"resourceFilename =~ /docker/i", true},
		{"resourceFilename =~ /^Dockerfile/", true},
		{"resourceFilename =~ /^dev/", false}, // ^ anchors at the start, not anywhere
		{"resourceFilename =~ /\\.dev$/", true},
		{"path =~ /\\.test\\.ts$/", true},
		{"path =~ /[/]b\\./", true}, // / inside a class does not end the regexp
		{"path =~ /a\\/b/", true},
		{"lang =~ /^(type|java)script$/", true},
		{"lang =~ /script(?=$)/", true}, // look-ahead: backtracking engine
		{"missing =~ /.*/", false},
		{"n =~ /^4\\d$/", true},
		{"!(lang =~ /^go$/) && lang =~ /type/", true},
		{"lang =~ /x/ || path =~ /src/", true},
	} {
		e, err := Parse(c.expr)
		if err != nil {
			t.Errorf("Parse(%q): %v", c.expr, err)
			continue
		}
		if got := e.Eval(ctx); got != c.want {
			t.Errorf("%q = %v, want %v", c.expr, got, c.want)
		}
		if _, err := Parse(e.String()); err != nil {
			t.Errorf("Parse(String(%q) = %q): %v", c.expr, e.String(), err)
		}
	}
	for _, bad := range []string{"a =~ /(/", "a =~ /x/q", "a =~ /x/ii"} {
		if _, err := Parse(bad); err == nil {
			t.Errorf("Parse(%q) accepted", bad)
		}
	}
}

func TestWhenImplies(t *testing.T) {
	mp := func(s string) Expr {
		t.Helper()
		if s == "" {
			return nil
		}
		e, err := Parse(s)
		if err != nil {
			t.Fatalf("Parse(%q): %v", s, err)
		}
		return e
	}
	for _, c := range []struct {
		p, q string
		want bool
	}{
		// Inclusion cases as in Monaco's whenIsEntirelyIncluded tests.
		{"key1", "", true},
		{"key1", "key1", true},
		{"key1 && key2", "key2", true},
		{"key1 && key2", "key1", true},
		{"key1 && key2", "key1 && key2", true},
		{"key1", "key1 && key2", false},
		{"key1 || key2", "key1 || key2", true},
		{"key1 || key2", "key1", false},
		{"key1", "key1 || key2", true},
		{"", "key1", false},
		// Order, parentheses and nesting do not matter.
		{"b && a", "(a && b)", true},
		{"a && (b && c)", "c && a", true},
		{"(a || b) || c", "c || b || a", true},
		{"!a && b == 'x'", "b == 'x'", true},
		{"!a && b == 'x'", "a", false},
		{"false", "anything", true},
		{"a", "true", true},
	} {
		if got := Implies(mp(c.p), mp(c.q)); got != c.want {
			t.Errorf("Implies(%q, %q) = %v, want %v", c.p, c.q, got, c.want)
		}
	}
}
