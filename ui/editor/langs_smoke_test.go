package editor

import (
	"encoding/json"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tangoui/tango/ui/a11y"
	"github.com/tangoui/tango/ui/editor/langs"
	"github.com/tangoui/tango/ui/editor/monarch"
	"github.com/tangoui/tango/ui/editor/theme"
	"github.com/tangoui/tango/ui/geom"
)

// acceptanceDecorations turns on the optional display layers too. This is a
// smoke test, not a claim that every layer is applicable to every language.
func acceptanceDecorations(st *State) {
	st.Options.WordWrap = WrapOn
	st.Options.RenderWhitespace = RenderWhitespaceAll
	st.Options.RenderControlCharacters = true
	st.Options.RenderLineHighlight = RenderLineHighlightAll
	st.Options.Minimap.ShowSlider = MinimapSliderAlways
	st.Options.Padding = PaddingOptions{Top: 4, Bottom: 4}
	st.Options.Guides.BracketPairs = BracketGuidesOn
	st.Options.UnicodeHighlight.NonBasicASCII = UnicodeHighlightTrue
	st.Options.UnicodeHighlight.IncludeComments = UnicodeHighlightTrue
	st.Options.MouseWheelZoom = true
	st.SetOverviewMarks("acceptance", []OverviewMark{{Line: 1, Lane: 7, Color: color.NRGBA{R: 255, A: 255}}})
}

func acceptanceSample(t *testing.T, id string) string {
	t.Helper()
	// Monaco has shared grammars with aliases, which have no separate fixture.
	fixture := id
	switch id {
	case "c":
		fixture = "cpp"
	case "verilog":
		fixture = "systemverilog"
	case "vue":
		fixture = "html"
	}
	if id == "freemarker2" {
		fixture = "freemarker2.tag-auto.interpolation-dollar"
	}
	// Upstream exports no nonempty tests for these five IDs. Keep the
	// explicit local samples visible rather than claim upstream coverage.
	local := map[string]string{
		"csp":      "default-src 'self'; script-src https://example.test",
		"ecl":      "IMPORT Std; OUTPUT('hello');",
		"ini":      "[section]\nkey=value",
		"pgsql":    "SELECT id FROM items WHERE id = 1;",
		"redshift": "SELECT id FROM items LIMIT 10;",
	}
	if sample, ok := local[id]; ok {
		t.Log("local sample: upstream fixture absent/empty")
		return sample
	}
	raw, err := os.ReadFile(filepath.Join("langs/testdata/tokenize", fixture+".json"))
	if err != nil {
		t.Fatal(err)
	}
	var cases [][]struct {
		Line string `json:"line"`
	}
	if err := json.Unmarshal(raw, &cases); err != nil {
		t.Fatal(err)
	}
	for _, c := range cases {
		var lines []string
		for _, l := range c {
			lines = append(lines, l.Line)
		}
		if s := strings.Join(lines, "\n"); s != "" {
			return s
		}
	}
	t.Fatalf("%s: no nonempty Monaco sample", id)
	return ""
}

func TestAllLanguagesInView(t *testing.T) {
	goText(t)
	all := langs.Builtin()
	if len(all) < 85 {
		t.Fatalf("registered %d languages, want at least 85", len(all))
	}
	_, panics, _ := monarch.Stats()
	for _, lang := range all {
		t.Run(lang.ID, func(t *testing.T) {
			sample := acceptanceSample(t, lang.ID)
			m := NewModel("acceptance", sample, DefaultRegistry())
			m.SetLanguage(lang.ID)
			st := NewView(m, theme.Base("vs-dark"))
			defer st.Close()
			acceptanceDecorations(st)
			d, _ := viewDriver(st, 700, 360, nil)
			d.Frame()
			if m.Language() != lang.ID || st.Highlighter() == nil {
				t.Fatal("language not active")
			}
			d.Click(geom.Pt(160, 80))
			st.MoveCaret(0, false)
			d.Type("x")
			d.Frame()
			if st.Snapshot().String() == sample {
				t.Fatal("typing did not edit")
			}
			if !st.Execute("undo", nil) {
				t.Fatal("undo refused")
			}
			d.Frame()
			if st.Snapshot().String() != sample {
				t.Fatal("undo did not restore sample")
			}
			if d.Image() == nil {
				t.Fatal("no rendered view")
			}
		})
	}
	if _, got, _ := monarch.Stats(); got != panics {
		t.Fatalf("%d recovered tokenizer panics", got-panics)
	}
	t.Logf("%d registered IDs exercised (brief minimum 85); shared grammar aliases use upstream sample", len(all))
}

func TestAllBaseThemesInView(t *testing.T) {
	goText(t)
	for _, name := range []string{"vs", "vs-dark", "hc-black", "hc-light"} {
		t.Run(name, func(t *testing.T) {
			th := theme.Base(name)
			for _, id := range []string{"editor.background", "editor.foreground", "editorLineNumber.foreground", "editorCursor.foreground"} {
				c, ok := th.Color(id)
				if !ok || c.A == 0 {
					t.Errorf("%s missing/transparent %s: %v", name, id, c)
				}
			}
			st := NewState("function sample() {\n\tconst color = '#ff0000'; // 中文 а\n\treturn color;\n}\nhttps://example.test\n", "sample.ts", th)
			defer st.Close()
			acceptanceDecorations(st)
			d, _ := viewDriver(st, 800, 400, nil)
			d.Frame()
			img := d.Image()
			bg, _ := th.Color("editor.background")
			if countColor(img, st.view.vs.text, bg) < 1000 {
				t.Fatal("theme background not rendered")
			}
			// Images are evidence, never silently rewrite peer golden files.
			if dir := os.Getenv("TANGO_EDITOR_ACCEPTANCE_SHOTS"); dir != "" {
				if err := os.MkdirAll(dir, 0755); err != nil {
					t.Fatal(err)
				}
				f, err := os.Create(filepath.Join(dir, name+".png"))
				if err != nil {
					t.Fatal(err)
				}
				err = png.Encode(f, img)
				closeErr := f.Close()
				if err != nil {
					t.Fatal(err)
				}
				if closeErr != nil {
					t.Fatal(closeErr)
				}
			}
		})
	}
}

func TestViewAccessibilityCodeEditor(t *testing.T) {
	goText(t)
	st := NewState("hello\nworld", "named.go", theme.Base("vs"))
	defer st.Close()
	d, _ := viewDriver(st, 400, 240, nil)
	d.EnableAccessibility()
	n, ok := d.A11yFind(a11y.RoleTextbox, "named.go")
	if !ok {
		t.Fatalf("editor must be named textbox, got:\n%s", d.AccessibilityTree().Dump())
	}
	if !n.States.Has(a11y.Multiline | a11y.Focusable) {
		t.Fatalf("not focusable multiline: %s", n)
	}
}
