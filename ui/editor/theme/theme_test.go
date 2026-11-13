package theme

import (
	"encoding/json"
	"fmt"
	"image/color"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/tangoui/tango/ui/editor/langs"
)

// jsonTheme mirrors the JSON in ui/editor/langs/data/themes.json so expected
// values are read from Monaco's exported data rather than typed by hand.
type jsonTheme struct {
	Base    string            `json:"base"`
	Inherit bool              `json:"inherit"`
	Rules   []jsonRule        `json:"rules"`
	Colors  map[string]string `json:"colors"`
}

type jsonRule struct {
	Token      string  `json:"token"`
	Foreground *string `json:"foreground"`
	Background *string `json:"background"`
	FontStyle  *string `json:"fontStyle"`
}

func baseData(t *testing.T, name string) jsonTheme {
	t.Helper()
	raw, ok := langs.BaseThemes()[name]
	if !ok {
		t.Fatalf("themes.json has no %q", name)
	}
	var th jsonTheme
	if err := json.Unmarshal(raw, &th); err != nil {
		t.Fatal(err)
	}
	return th
}

// ruleFG returns the foreground of the last rule for token in the base theme.
func ruleFG(t *testing.T, th jsonTheme, token string) color.NRGBA {
	t.Helper()
	var fg *string
	for _, r := range th.Rules {
		if r.Token == token && r.Foreground != nil {
			fg = r.Foreground
		}
	}
	if fg == nil {
		t.Fatalf("no %q rule with foreground", token)
	}
	return hex(t, *fg)
}

// hex is an independent RRGGBB / RRGGBBAA parser for expected values.
func hex(t *testing.T, s string) color.NRGBA {
	t.Helper()
	s = strings.TrimPrefix(s, "#")
	v, err := strconv.ParseUint(s, 16, 32)
	if err != nil {
		t.Fatalf("bad hex %q", s)
	}
	switch len(s) {
	case 6:
		return color.NRGBA{uint8(v >> 16), uint8(v >> 8), uint8(v), 0xff}
	case 8:
		return color.NRGBA{uint8(v >> 24), uint8(v >> 16), uint8(v >> 8), uint8(v)}
	}
	t.Fatalf("bad hex length %q", s)
	return color.NRGBA{}
}

func defaultRule(t *testing.T, th jsonTheme) (fg, bg color.NRGBA) {
	t.Helper()
	for _, r := range th.Rules {
		if r.Token == "" {
			return hex(t, *r.Foreground), hex(t, *r.Background)
		}
	}
	t.Fatal("no default rule")
	return
}

func TestBaseVSDark(t *testing.T) {
	data := baseData(t, "vs-dark")
	th := Base("vs-dark")
	if th == nil {
		t.Fatal("Base(vs-dark) = nil")
	}
	for _, c := range []struct{ token, rule string }{
		{"comment.go", "comment"},
		{"comment", "comment"},
		{"keyword.go", "keyword"},
		{"string.escape.ts", "string"}, // no string.escape rule: falls back
		{"string.key.json", "string.key.json"},
		{"keyword.flow.go", "keyword.flow"},
		{"number.hex.go", "number.hex"},
	} {
		if got, want := th.Style(c.token).Foreground, ruleFG(t, data, c.rule); got != want {
			t.Errorf("Style(%q).Foreground = %v, want %v (rule %q)", c.token, got, want, c.rule)
		}
	}
	fg, bg := defaultRule(t, data)
	s := th.Style("identifier.go")
	if s.Foreground != fg || s.Background != bg {
		t.Errorf("Style(identifier.go) = %+v, want fg %v bg %v", s, fg, bg)
	}
	// Background is inherited from the default rule by every token.
	if got := th.Style("comment.go").Background; got != bg {
		t.Errorf("comment.go background = %v, want %v", got, bg)
	}
	// fontStyle from the base data.
	if s := th.Style("strong.md"); !s.Bold || s.Italic {
		t.Errorf("strong.md = %+v, want bold", s)
	}
	if s := th.Style("emphasis"); !s.Italic || s.Bold {
		t.Errorf("emphasis = %+v, want italic", s)
	}
	if got, ok := th.Color("editor.lineHighlightBackground"); ok {
		t.Errorf("unexpected lineHighlightBackground %v", got)
	}
	if got, ok := th.Color("editor.selectionHighlightBackground"); !ok || got != hex(t, data.Colors["editor.selectionHighlightBackground"]) {
		t.Errorf("Color(selectionHighlightBackground) = %v, %v", got, ok)
	}
	for _, name := range []string{"vs", "vs-dark", "hc-black", "hc-light"} {
		b := Base(name)
		if b == nil {
			t.Fatalf("Base(%q) = nil", name)
		}
		if Base(name) != b {
			t.Errorf("Base(%q) not cached", name)
		}
		d := baseData(t, name)
		if got, want := b.Style("comment.go").Foreground, ruleFG(t, d, "comment"); got != want {
			t.Errorf("%s comment.go = %v, want %v", name, got, want)
		}
	}
	if Base("monokai") != nil {
		t.Error("Base(unknown) != nil")
	}
}

func TestInheritOverride(t *testing.T) {
	vs := baseData(t, "vs")
	th, err := Parse([]byte(`{
		"base": "vs", "inherit": true,
		"rules": [{"token": "keyword", "foreground": "#123456", "fontStyle": "bold"}],
		"colors": {"editor.lineHighlightBackground": "#abcdef"}
	}`))
	if err != nil {
		t.Fatal(err)
	}
	want := color.NRGBA{0x12, 0x34, 0x56, 0xff}
	if s := th.Style("keyword.go"); s.Foreground != want || !s.Bold {
		t.Errorf("keyword.go = %+v, want fg %v bold", s, want)
	}
	if got, want := th.Style("string.go").Foreground, ruleFG(t, vs, "string"); got != want {
		t.Errorf("string.go = %v, want vs %v", got, want)
	}
	// Defaults and colors come from vs when not overridden.
	fg, bg := defaultRule(t, vs)
	if s := th.Style("identifier.go"); s.Foreground != fg || s.Background != bg {
		t.Errorf("identifier.go = %+v, want vs defaults %v %v", s, fg, bg)
	}
	if got, ok := th.Color("editor.lineHighlightBackground"); !ok || got != (color.NRGBA{0xab, 0xcd, 0xef, 0xff}) {
		t.Errorf("own color = %v %v", got, ok)
	}
	if got, ok := th.Color("editor.background"); !ok || got != hex(t, vs.Colors["editor.background"]) {
		t.Errorf("inherited editor.background = %v %v", got, ok)
	}

	// editor.foreground / editor.background in the theme's own colors become
	// the default token colors, overriding the base defaults; later rules still
	// win.
	th, err = Parse([]byte(`{
		"base": "vs-dark", "inherit": true,
		"rules": [{"token": "comment", "foreground": "010203"}],
		"colors": {"editor.foreground": "#aabbcc", "editor.background": "#102030"}
	}`))
	if err != nil {
		t.Fatal(err)
	}
	if s := th.Style("identifier"); s.Foreground != (color.NRGBA{0xaa, 0xbb, 0xcc, 0xff}) || s.Background != (color.NRGBA{0x10, 0x20, 0x30, 0xff}) {
		t.Errorf("defaults from colors = %+v", s)
	}
	if s := th.Style("comment.go"); s.Foreground != (color.NRGBA{1, 2, 3, 0xff}) || s.Background != (color.NRGBA{0x10, 0x20, 0x30, 0xff}) {
		t.Errorf("comment.go = %+v", s)
	}

	// Without inherit, base rules are not applied.
	th, err = Parse([]byte(`{"base": "vs", "inherit": false, "rules": [], "colors": {}}`))
	if err != nil {
		t.Fatal(err)
	}
	if got := th.Style("string.go").Foreground; got == ruleFG(t, vs, "string") {
		t.Errorf("non-inheriting theme picked up vs string color")
	}
	if _, ok := th.OwnColor("editor.background"); ok {
		t.Error("non-inheriting theme picked up vs colors")
	}
	// Color falls back to the registry default (#FFFFFF for light themes),
	// not to vs's own #FFFFFE.
	if got, ok := th.Color("editor.background"); !ok || got != (color.NRGBA{0xff, 0xff, 0xff, 0xff}) {
		t.Errorf("non-inheriting editor.background = %v %v, want the registry default #FFFFFF", got, ok)
	}
}

func TestLongestPrefix(t *testing.T) {
	th, err := Parse([]byte(`{
		"base": "vs", "inherit": false,
		"rules": [
			{"token": "keyword.go.x", "fontStyle": ""},
			{"token": "keyword.go", "foreground": "333333"},
			{"token": "", "foreground": "111111", "background": "eeeeee"},
			{"token": "keyword", "foreground": "222222", "background": "dddddd", "fontStyle": "bold italic"},
			{"token": "string", "foreground": "444444"},
			{"token": "string", "fontStyle": "underline strikethrough"},
			{"token": "string", "foreground": "555555"}
		],
		"colors": {}
	}`))
	if err != nil {
		t.Fatal(err)
	}
	c := func(v uint8) color.NRGBA { return color.NRGBA{v, v, v, 0xff} }
	cases := []struct {
		token string
		want  Style
	}{
		{"keyword", Style{Foreground: c(0x22), Background: c(0xdd), Bold: true, Italic: true}},
		// keyword.go overrides only foreground; fontStyle and background come
		// from keyword.
		{"keyword.go", Style{Foreground: c(0x33), Background: c(0xdd), Bold: true, Italic: true}},
		{"keyword.go.y", Style{Foreground: c(0x33), Background: c(0xdd), Bold: true, Italic: true}},
		// fontStyle "" resets inherited font style.
		{"keyword.go.x", Style{Foreground: c(0x33), Background: c(0xdd)}},
		{"keyword.go.x.z", Style{Foreground: c(0x33), Background: c(0xdd)}},
		{"keyword.ts", Style{Foreground: c(0x22), Background: c(0xdd), Bold: true, Italic: true}},
		// Matching is by whole dot segments, not string prefix.
		{"keywords", Style{Foreground: c(0x11), Background: c(0xee)}},
		{"go.keyword", Style{Foreground: c(0x11), Background: c(0xee)}},
		{"", Style{Foreground: c(0x11), Background: c(0xee)}},
		// Later rules for the same token override earlier ones field by field.
		{"string.go", Style{Foreground: c(0x55), Background: c(0xee), Underline: true, Strikethrough: true}},
	}
	for _, tc := range cases {
		if got := th.Style(tc.token); got != tc.want {
			t.Errorf("Style(%q) = %+v, want %+v", tc.token, got, tc.want)
		}
		// Cached result is identical.
		if got := th.Style(tc.token); got != tc.want {
			t.Errorf("cached Style(%q) = %+v, want %+v", tc.token, got, tc.want)
		}
	}

	// No "" rule and no editor colors: Monaco defaults (black on white).
	th, err = Parse([]byte(`{"base": "vs-dark", "rules": [{"token": "a", "fontStyle": "italic"}]}`))
	if err != nil {
		t.Fatal(err)
	}
	if got, want := th.Style("b"), (Style{Foreground: c(0), Background: c(0xff)}); got != want {
		t.Errorf("Monaco defaults = %+v, want %+v", got, want)
	}
}

func TestColorFormats(t *testing.T) {
	th, err := Parse([]byte(`{
		"base": "vs", "inherit": false,
		"rules": [
			{"token": "a", "foreground": "#abc"},
			{"token": "b", "foreground": "A1B2C3"},
			{"token": "c", "foreground": "#a1b2c3", "background": "#0102037f"}
		],
		"colors": {
			"short": "#abc",
			"short-no-hash": "abc",
			"long": "#A1b2C3",
			"long-no-hash": "a1b2c3",
			"alpha": "#a1b2c37f",
			"alpha-no-hash": "a1b2c37f"
		}
	}`))
	if err != nil {
		t.Fatal(err)
	}
	abc := color.NRGBA{0xaa, 0xbb, 0xcc, 0xff}
	a1 := color.NRGBA{0xa1, 0xb2, 0xc3, 0xff}
	a1a := color.NRGBA{0xa1, 0xb2, 0xc3, 0x7f}
	for key, want := range map[string]color.NRGBA{
		"short": abc, "short-no-hash": abc,
		"long": a1, "long-no-hash": a1,
		"alpha": a1a, "alpha-no-hash": a1a,
	} {
		if got, ok := th.Color(key); !ok || got != want {
			t.Errorf("Color(%q) = %v %v, want %v", key, got, ok, want)
		}
	}
	if _, ok := th.Color("missing"); ok {
		t.Error("Color(missing) ok")
	}
	if got := th.Style("a").Foreground; got != abc {
		t.Errorf("a = %v, want %v", got, abc)
	}
	if got := th.Style("b").Foreground; got != a1 {
		t.Errorf("b = %v, want %v", got, a1)
	}
	// Token colors are opaque in Monaco: the alpha digits are dropped.
	if got := th.Style("c").Background; got != (color.NRGBA{1, 2, 3, 0xff}) {
		t.Errorf("c background = %v", got)
	}

	// Only structural problems fail Parse; bad individual colors are warnings
	// (TestInvalidColorsAreWarnings).
	for _, bad := range []string{
		`not json`,
		`{"base": "monokai", "rules": []}`,
		`{"rules": []}`,
	} {
		if _, err := Parse([]byte(bad)); err == nil {
			t.Errorf("Parse(%s) succeeded", bad)
		}
	}
}

// #RGBA (CSS 4-digit hex) is accepted in the colors table and in rules.
func TestColorRGBA(t *testing.T) {
	th, err := Parse([]byte(`{"base":"vs","inherit":false,
		"rules":[{"token":"a","foreground":"#abc8"}],
		"colors":{"x":"#abc8","y":"abc8"}}`))
	if err != nil {
		t.Fatal(err)
	}
	want := color.NRGBA{0xaa, 0xbb, 0xcc, 0x88}
	for _, k := range []string{"x", "y"} {
		if got, ok := th.Color(k); !ok || got != want {
			t.Errorf("Color(%q) = %v %v, want %v", k, got, ok, want)
		}
	}
	if got := th.Style("a").Foreground; got != (color.NRGBA{0xaa, 0xbb, 0xcc, 0xff}) {
		t.Errorf("rule #abc8 = %v, want opaque aabbcc", got)
	}
	if w := th.Warnings(); len(w) != 0 {
		t.Errorf("warnings %v", w)
	}
}

// Invalid individual color values are ignored with a warning instead of
// failing the whole theme (Monaco shows red for a bad colors entry and throws
// for a bad rule color; we ignore both).
func TestInvalidColorsAreWarnings(t *testing.T) {
	th, err := Parse([]byte(`{"base":"vs","inherit":false,
		"rules":[
			{"token":"a","foreground":"#12","background":"#010203"},
			{"token":"b","foreground":"red"},
			{"token":"c","background":"#12345g","fontStyle":"bold"}
		],
		"colors":{"good":"#102030","bad":"#1234567","worse":"blue"}}`))
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}
	if _, ok := th.Color("bad"); ok {
		t.Error("invalid color kept")
	}
	if _, ok := th.Color("worse"); ok {
		t.Error("invalid color kept")
	}
	if got, ok := th.Color("good"); !ok || got != (color.NRGBA{0x10, 0x20, 0x30, 0xff}) {
		t.Errorf("good = %v %v", got, ok)
	}
	def := th.Style("")
	a := th.Style("a")
	if a.Foreground != def.Foreground || a.Background != (color.NRGBA{1, 2, 3, 0xff}) {
		t.Errorf("a = %+v: bad foreground must be ignored, valid background kept", a)
	}
	if b := th.Style("b"); b.Foreground != def.Foreground {
		t.Errorf("b = %+v", b)
	}
	if c := th.Style("c"); !c.Bold || c.Background != def.Background {
		t.Errorf("c = %+v", c)
	}
	w := th.Warnings()
	if len(w) != 5 {
		t.Fatalf("warnings = %q, want 5", w)
	}
	joined := strings.Join(w, "\n")
	for _, s := range []string{`"#12"`, `"red"`, `"#12345g"`, `"#1234567"`, `"blue"`} {
		if !strings.Contains(joined, s) {
			t.Errorf("no warning mentions %s: %q", s, w)
		}
	}
	// Warnings returns a copy.
	w[0] = "x"
	if th.Warnings()[0] == "x" {
		t.Error("Warnings exposes internal slice")
	}
}

// Non-string fields in a rule are ignored field by field, as Monaco's
// parseTokenTheme does with its typeof checks; a non-object rule is skipped.
func TestRuleFieldsDecodedLeniently(t *testing.T) {
	th, err := Parse([]byte(`{"base":"vs","inherit":false,
		"rules":[
			{"token":"a","foreground":123,"background":"#010203","fontStyle":true},
			{"token":"b","foreground":"#040506","fontStyle":["bold"]},
			{"token":7,"foreground":"#070809"},
			"junk",
			{"token":"c","foreground":null,"fontStyle":"italic"}
		]}`))
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}
	def := th.Style("")
	if a := th.Style("a"); a.Foreground != def.Foreground || a.Background != (color.NRGBA{1, 2, 3, 0xff}) || a.Bold || a.Italic {
		t.Errorf("a = %+v", a)
	}
	if b := th.Style("b"); b.Foreground != (color.NRGBA{4, 5, 6, 0xff}) || b.Bold {
		t.Errorf("b = %+v", b)
	}
	// token 7 is not a string: Monaco's entry.token || '' keeps the number, which
	// never equals a token type segment; we treat it as the default rule "".
	if def.Foreground != (color.NRGBA{7, 8, 9, 0xff}) {
		t.Errorf("default = %+v", def)
	}
	if c := th.Style("c"); !c.Italic || c.Foreground != def.Foreground {
		t.Errorf("c = %+v", c)
	}
	if len(th.Warnings()) == 0 {
		t.Error("ignored fields should be reported as warnings")
	}
}

// The synthetic default rule from editor.foreground/background is created only
// for non-empty valid values (Monaco: if (editorForeground || editorBackground)).
func TestEditorColorsDefaultRule(t *testing.T) {
	vs := Base("vs").Style("")
	for _, c := range []struct {
		colors string
		fg, bg color.NRGBA
	}{
		{`{"editor.foreground":"","editor.background":""}`, vs.Foreground, vs.Background},
		{`{"editor.foreground":"bogus","editor.background":"#102030"}`, vs.Foreground, color.NRGBA{0x10, 0x20, 0x30, 0xff}},
		{`{"editor.foreground":"#405060","editor.background":""}`, color.NRGBA{0x40, 0x50, 0x60, 0xff}, vs.Background},
	} {
		th, err := Parse([]byte(`{"base":"vs","inherit":true,"rules":[],"colors":` + c.colors + `}`))
		if err != nil {
			t.Fatalf("%s: %v", c.colors, err)
		}
		if got := th.Style(""); got.Foreground != c.fg || got.Background != c.bg {
			t.Errorf("%s: default = %+v, want fg %v bg %v", c.colors, got, c.fg, c.bg)
		}
	}
}

func TestConcurrentStyle(t *testing.T) {
	th := Base("vs-dark")
	want := map[string]Style{}
	tokens := make([]string, 0, 64)
	for i := range 64 {
		tok := fmt.Sprintf("keyword.l%d", i%16)
		if i%3 == 0 {
			tok = fmt.Sprintf("comment.l%d", i%16)
		}
		tokens = append(tokens, tok)
	}
	var wg sync.WaitGroup
	results := make([][]Style, 8)
	for g := range results {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for _, tok := range tokens {
				results[g] = append(results[g], th.Style(tok))
			}
		}()
	}
	wg.Wait()
	for i, tok := range tokens {
		want[tok] = th.Style(tok)
		for g := range results {
			if results[g][i] != want[tok] {
				t.Fatalf("goroutine %d: %s = %+v, want %+v", g, tok, results[g][i], want[tok])
			}
		}
	}
}
