package langs

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// Monaco 0.56.0 has 89 registerLanguage() calls (81 grammar modules; cpp also
// registers c, systemverilog also registers verilog, freemarker2 registers 7
// ids) plus the extra .vue registration.
const wantLanguages = 90

func TestBuiltinCount(t *testing.T) {
	all := Builtin()
	if len(all) != wantLanguages {
		t.Fatalf("len(Builtin()) = %d, want %d", len(all), wantLanguages)
	}
	for i := 1; i < len(all); i++ {
		if all[i-1].ID >= all[i].ID {
			t.Fatalf("Builtin() not sorted by ID: %q before %q", all[i-1].ID, all[i].ID)
		}
	}
	for _, l := range all {
		if len(l.Grammar) == 0 {
			t.Errorf("%s: empty grammar", l.ID)
		}
		if ByID(l.ID) != l {
			t.Errorf("ByID(%q) does not return the Builtin() entry", l.ID)
		}
	}
	if ByID("no-such-language") != nil {
		t.Error("ByID(unknown) != nil")
	}
	// Builtin parses once: repeated calls share the same pointers.
	if Builtin()[0] != all[0] {
		t.Error("Builtin() re-parsed")
	}
}

func TestForFilename(t *testing.T) {
	cases := map[string]string{
		"main.go":                "go",
		"Dockerfile":             "dockerfile",
		"x.vue":                  "vue",
		"a.TS":                   "typescript",
		"/src/pkg/a.TS":          "typescript",
		`C:\src\pkg\main.go`:     "go",
		"dockerfile":             "dockerfile", // filenames are case-insensitive
		"Gemfile":                "ruby",
		"index.html":             "html",
		"README.md":              "markdown",
		"config.yaml":            "yaml",
		"lib.d.ts":               "typescript",
		"x.c":                    "c",
		"x.cpp":                  "cpp",
		"x.pp":                   "ruby", // pascal and ruby both claim .pp; last registered wins
		"x.ftl":                  "freemarker2",
		"no-extension-or-match":  "",
		"archive.unknown-suffix": "",
	}
	for name, want := range cases {
		got := ForFilename(name)
		gotID := ""
		if got != nil {
			gotID = got.ID
		}
		if gotID != want {
			t.Errorf("ForFilename(%q) = %q, want %q", name, gotID, want)
		}
	}
}

func TestConfDecodes(t *testing.T) {
	for _, l := range Builtin() {
		// Builtin() already decoded Conf; re-decode strictly here so a
		// malformed entry fails with the language id rather than a panic.
		raw, err := os.ReadFile(filepath.Join("data", l.ID+".json"))
		if err != nil {
			t.Fatal(err)
		}
		var file struct {
			Conf json.RawMessage `json:"conf"`
		}
		if err := json.Unmarshal(raw, &file); err != nil {
			t.Fatalf("%s: %v", l.ID, err)
		}
		var c Conf
		if err := json.Unmarshal(file.Conf, &c); err != nil {
			t.Errorf("%s: conf: %v", l.ID, err)
		}
	}

	g := ByID("go")
	if g == nil {
		t.Fatal("go missing")
	}
	if g.Conf.LineComment != "//" {
		t.Errorf("go LineComment = %q, want //", g.Conf.LineComment)
	}
	if g.Conf.BlockComment != [2]string{"/*", "*/"} {
		t.Errorf("go BlockComment = %q", g.Conf.BlockComment)
	}
	if len(g.Conf.Brackets) == 0 || g.Conf.Brackets[0] != [2]string{"{", "}"} {
		t.Errorf("go Brackets = %q", g.Conf.Brackets)
	}
	var quote *Pair
	for i := range g.Conf.AutoClosingPairs {
		if g.Conf.AutoClosingPairs[i].Open == "'" {
			quote = &g.Conf.AutoClosingPairs[i]
		}
	}
	if quote == nil || quote.Close != "'" || len(quote.NotIn) != 2 || quote.NotIn[1] != "comment" {
		t.Errorf("go ' auto-closing pair = %+v", quote)
	}

	html := ByID("html").Conf
	if html.WordPattern == nil || html.WordPattern.Flags != "g" || html.WordPattern.Source == "" {
		t.Errorf("html WordPattern = %+v", html.WordPattern)
	}
	if html.Folding.Start == nil || html.Folding.End == nil {
		t.Errorf("html folding markers = %+v", html.Folding)
	}
	if len(html.SurroundingPairs) == 0 {
		t.Error("html SurroundingPairs empty")
	}
	if py := ByID("python").Conf; !py.Folding.OffSide {
		t.Error("python Folding.OffSide = false")
	}
	if rb := ByID("ruby").Conf; rb.Indentation.Increase == nil || rb.Indentation.Decrease == nil {
		t.Errorf("ruby indentation = %+v", rb.Indentation)
	}
	if ts := ByID("typespec").Conf; ts.Indentation.Unindented == nil {
		t.Errorf("typespec indentation = %+v", ts.Indentation)
	}
	if vue, html := ByID("vue"), ByID("html"); string(vue.Grammar) != string(html.Grammar) {
		t.Error("vue grammar differs from html")
	}
}

func TestRegexSpecJSON(t *testing.T) {
	var r RegexSpec
	if err := json.Unmarshal([]byte(`{"$regex":"a+b","flags":"im"}`), &r); err != nil {
		t.Fatal(err)
	}
	if r != (RegexSpec{Source: "a+b", Flags: "im"}) {
		t.Fatalf("got %+v", r)
	}
	out, err := json.Marshal(r)
	if err != nil || string(out) != `{"$regex":"a+b","flags":"im"}` {
		t.Fatalf("Marshal = %s, %v", out, err)
	}
	if err := json.Unmarshal([]byte(`"a+b"`), &r); err == nil {
		t.Error("plain string accepted as RegexSpec")
	}
}

func TestBaseThemes(t *testing.T) {
	themes := BaseThemes()
	for _, name := range []string{"vs", "vs-dark", "hc-black", "hc-light"} {
		raw, ok := themes[name]
		if !ok {
			t.Errorf("theme %s missing", name)
			continue
		}
		var th struct {
			Base   string            `json:"base"`
			Rules  []json.RawMessage `json:"rules"`
			Colors map[string]string `json:"colors"`
		}
		if err := json.Unmarshal(raw, &th); err != nil {
			t.Errorf("%s: %v", name, err)
			continue
		}
		if th.Base == "" || len(th.Rules) == 0 || th.Colors["editor.background"] == "" {
			t.Errorf("%s: incomplete theme base=%q rules=%d colors=%v", name, th.Base, len(th.Rules), th.Colors)
		}
	}
	if len(themes) != 4 {
		t.Errorf("len(BaseThemes()) = %d, want 4", len(themes))
	}
}

func TestFixturesPresent(t *testing.T) {
	for _, id := range []string{"go", "typescript", "yaml", "html", "markdown"} {
		raw, err := os.ReadFile(filepath.Join("testdata", "tokenize", id+".json"))
		if err != nil {
			t.Errorf("%s: %v", id, err)
			continue
		}
		var cases [][]struct {
			Line   string `json:"line"`
			Tokens []struct {
				StartIndex int    `json:"startIndex"`
				Type       string `json:"type"`
			} `json:"tokens"`
		}
		if err := json.Unmarshal(raw, &cases); err != nil {
			t.Errorf("%s: %v", id, err)
			continue
		}
		if len(cases) == 0 || len(cases[0]) == 0 {
			t.Errorf("%s: no cases", id)
		}
	}
	// Every fixture targets a known language.
	files, _ := filepath.Glob(filepath.Join("testdata", "tokenize", "*.json"))
	for _, f := range files {
		id := filepath.Base(f[:len(f)-len(".json")])
		if ByID(id) == nil {
			t.Errorf("fixture %s has no language", id)
		}
	}
}

func TestGlobRegexp(t *testing.T) {
	cases := []struct {
		glob, target string
		want         bool
	}{
		{"*.conf", "nginx.conf", true},
		{"*.conf", "a/nginx.conf", false},
		{"**/.github/workflows/*.yml", "/repo/.github/workflows/ci.yml", true},
		{"**/.github/workflows/*.yml", ".github/workflows/ci.yml", true},
		{"**/.github/workflows/*.yml", "/repo/.github/ci.yml", false},
		{"*.{yml,yaml}", "X.YAML", true},
		{"file?.txt", "file1.txt", true},
		{"file?.txt", "file12.txt", false},
		{"[!a]bc", "xbc", true},
		{"[!a]bc", "abc", false},
		{"Makefile.*", "makefile.am", true},
	}
	for _, c := range cases {
		re := globRegexp(c.glob)
		if re == nil {
			t.Errorf("globRegexp(%q) = nil", c.glob)
			continue
		}
		if got := re.MatchString(c.target); got != c.want {
			t.Errorf("glob %q on %q = %v, want %v (re %s)", c.glob, c.target, got, c.want, re)
		}
	}
	if globRegexp("{a,b") != nil {
		t.Error("unbalanced brace accepted")
	}
}

// TestDecodeOnEnterRules: Conf decodes onEnterRules, autoCloseBefore and
// colorizedBracketPairs from the exported configuration (TypeScript's
// JSDoc rules verbatim), and every built-in language's configuration still
// decodes, with as many onEnter rules as its ConfRaw lists.
func TestDecodeOnEnterRules(t *testing.T) {
	ts := ByID("typescript").Conf
	if len(ts.OnEnterRules) != 4 {
		t.Fatalf("typescript has %d onEnter rules, want 4", len(ts.OnEnterRules))
	}
	doc := `^\s*\/\*\*(?!\/)([^\*]|\*(?!\/))*$`
	want := []OnEnterRule{
		{BeforeText: &RegexSpec{Source: doc}, AfterText: &RegexSpec{Source: `^\s*\*\/$`}, Action: EnterAction{Indent: "indentOutdent", AppendText: " * "}},
		{BeforeText: &RegexSpec{Source: doc}, Action: EnterAction{Indent: "none", AppendText: " * "}},
		{BeforeText: &RegexSpec{Source: `^(\t|(\ \ ))*\ \*(\ ([^\*]|\*(?!\/))*)?$`}, Action: EnterAction{Indent: "none", AppendText: "* "}},
		{BeforeText: &RegexSpec{Source: `^(\t|(\ \ ))*\ \*\/\s*$`}, Action: EnterAction{Indent: "none", RemoveText: 1}},
	}
	eq := func(a, b *RegexSpec) bool { return (a == nil) == (b == nil) && (a == nil || *a == *b) }
	for i, r := range ts.OnEnterRules {
		w := want[i]
		if !eq(r.BeforeText, w.BeforeText) || !eq(r.AfterText, w.AfterText) || !eq(r.PreviousLineText, w.PreviousLineText) || r.Action != w.Action {
			t.Errorf("rule %d = %+v %+v %+v %+v, want %+v", i, r.BeforeText, r.AfterText, r.PreviousLineText, r.Action, w)
		}
	}
	if py := ByID("python").Conf; len(py.OnEnterRules) != 1 || py.OnEnterRules[0].Action.Indent != "indent" {
		t.Errorf("python onEnter rules = %+v", py.OnEnterRules)
	}
	if html := ByID("html").Conf; len(html.OnEnterRules) != 2 || html.OnEnterRules[0].BeforeText.Flags != "i" {
		t.Errorf("html onEnter rules = %+v", html.OnEnterRules)
	}
	if got := ByID("proto").Conf.AutoCloseBefore; got != ".,=}])>' \n\t" {
		t.Errorf("proto autoCloseBefore = %q", got)
	}
	if got := ByID("go").Conf.AutoCloseBefore; got != "" {
		t.Errorf("go autoCloseBefore = %q, want none", got)
	}
	var c Conf
	if err := json.Unmarshal([]byte(`{"colorizedBracketPairs":[["(",")"],["<",">"]]}`), &c); err != nil {
		t.Fatal(err)
	}
	if len(c.ColorizedBracketPairs) != 2 || c.ColorizedBracketPairs[1] != [2]string{"<", ">"} {
		t.Errorf("colorizedBracketPairs = %q", c.ColorizedBracketPairs)
	}
	// A malformed rule is rejected on its own and skipped in a Conf.
	good := `{"beforeText":{"$regex":"x"},"action":{"indentAction":1}}`
	for _, bad := range []string{
		`{"action":{"indentAction":0}}`,
		`{"beforeText":{"$regex":"a"},"action":{}}`,
		`{"beforeText":{"$regex":"a"},"action":{"indentAction":4}}`,
	} {
		var r OnEnterRule
		if err := json.Unmarshal([]byte(bad), &r); err == nil {
			t.Errorf("%s decoded", bad)
		}
		var c Conf
		if err := json.Unmarshal([]byte(`{"onEnterRules":[`+bad+`,`+good+`]}`), &c); err != nil {
			t.Errorf("a Conf with %s fails: %v", bad, err)
		} else if len(c.OnEnterRules) != 1 || c.OnEnterRules[0].Action.Indent != "indent" {
			t.Errorf("a Conf with %s: rules %+v", bad, c.OnEnterRules)
		}
	}
	// Every language: the decoded rules match ConfRaw's count, and the
	// other two fields are decoded when present.
	for _, l := range Builtin() {
		var raw struct {
			OnEnterRules          []json.RawMessage `json:"onEnterRules"`
			AutoCloseBefore       *string           `json:"autoCloseBefore"`
			ColorizedBracketPairs [][2]string       `json:"colorizedBracketPairs"`
		}
		if err := json.Unmarshal(l.ConfRaw, &raw); err != nil {
			t.Fatalf("%s: %v", l.ID, err)
		}
		if len(raw.OnEnterRules) != len(l.Conf.OnEnterRules) {
			t.Errorf("%s: %d onEnter rules decoded, ConfRaw has %d", l.ID, len(l.Conf.OnEnterRules), len(raw.OnEnterRules))
		}
		if raw.AutoCloseBefore != nil && *raw.AutoCloseBefore != l.Conf.AutoCloseBefore {
			t.Errorf("%s: autoCloseBefore %q, ConfRaw %q", l.ID, l.Conf.AutoCloseBefore, *raw.AutoCloseBefore)
		}
		if len(raw.ColorizedBracketPairs) != len(l.Conf.ColorizedBracketPairs) {
			t.Errorf("%s: colorizedBracketPairs not decoded", l.ID)
		}
		for i, r := range l.Conf.OnEnterRules {
			if r.BeforeText == nil || r.BeforeText.Source == "" {
				t.Errorf("%s: rule %d has no beforeText", l.ID, i)
			}
		}
	}
}
