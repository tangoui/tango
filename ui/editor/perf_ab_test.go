package editor

import (
	"fmt"
	"os"
	"reflect"
	"slices"
	"strings"
	"testing"
)

// perfOffEnv lists the display features the benchmarks turn off (the
// paired A/B measurements of ui/editor/perfab/run.sh): comma-separated
// names of perfOffNames, or "all".
const perfOffEnv = "TANGO_EDITOR_PERF_OFF"

// perfOffNames are the feature names of perfOffEnv, in the order "all"
// applies them.
var perfOffNames = []string{
	"minimap", "sticky", "overview", "folding", "brackets", "guides",
	"matchbrackets", "occurrences", "selectionhl", "unicode", "links",
	"colors", "whitespace", "linehl", "minimapchars",
}

// perfModelOffNames are the names of perfOffEnv that turn off a Model
// creation option instead of a display feature (not part of "all"):
// detectindent creates the opened documents of the open benchmarks
// (openFile) with DetectIndentation false.
var perfModelOffNames = []string{"detectindent"}

// perfModelOptions returns the creation options of the documents the
// open benchmarks open: DefaultModelOptions less the perfModelOffNames in
// $TANGO_EDITOR_PERF_OFF.
func perfModelOptions() ModelOptions {
	o := DefaultModelOptions()
	for _, name := range strings.Split(os.Getenv(perfOffEnv), ",") {
		if strings.TrimSpace(name) == "detectindent" {
			o.DetectIndentation = false
		}
	}
	return o
}

// perfOff turns off one feature of perfOffNames in o.
func perfOff(o *Options, name string) bool {
	switch name {
	case "minimap":
		o.Minimap.Enabled = false
	case "sticky":
		o.StickyScroll.Enabled = false
	case "overview":
		o.OverviewRulerLanes = 0 // zero lanes hide the ruler
	case "folding":
		o.Folding = false
	case "brackets":
		o.BracketPairColorization = false
	case "guides":
		o.Guides.Indentation = false
		o.Guides.HighlightActiveIndentation = false
	case "matchbrackets":
		o.MatchBrackets = MatchBracketsNever
	case "occurrences":
		o.OccurrencesHighlight = OccurrencesOff
	case "selectionhl":
		o.SelectionHighlight = false
	case "unicode":
		o.UnicodeHighlight.InvisibleCharacters = false
		o.UnicodeHighlight.AmbiguousCharacters = false
		o.UnicodeHighlight.NonBasicASCII = UnicodeHighlightFalse
	case "links":
		o.Links = false
	case "colors":
		o.ColorDecorators = false
	case "whitespace":
		o.RenderWhitespace = RenderWhitespaceNone
	case "linehl":
		o.RenderLineHighlight = RenderLineHighlightNone
	case "minimapchars": // the minimap draws blocks (minimap_chars.go)
		o.Minimap.RenderCharacters = false
	default:
		return false
	}
	return true
}

// perfApplyOff turns off in o the features named by spec (a
// comma-separated list of perfOffNames or "all"; blanks and empty items
// are ignored). An unknown name is an error.
func perfApplyOff(o *Options, spec string) error {
	for _, name := range strings.Split(spec, ",") {
		name = strings.TrimSpace(name)
		switch {
		case name == "", slices.Contains(perfModelOffNames, name): // model options: perfModelOptions
		case name == "all":
			for _, n := range perfOffNames {
				perfOff(o, n)
			}
		case !perfOff(o, name):
			return fmt.Errorf("%s: unknown feature %q (want %s, %s or all)", perfOffEnv, name,
				strings.Join(perfOffNames, ", "), strings.Join(perfModelOffNames, ", "))
		}
	}
	return nil
}

// perfOnEnv lists the display features off by default the benchmarks
// turn on (perfOnNames, comma-separated), applied after perfOffEnv: an
// arm of ui/editor/perfab/arms.sh measures one with a test binary run
// through a script setting it.
const perfOnEnv = "TANGO_EDITOR_PERF_ON"

// perfOnNames are the feature names of perfOnEnv.
var perfOnNames = []string{"bracketguides"}

// perfOn turns on one feature of perfOnNames in o.
func perfOn(o *Options, name string) bool {
	switch name {
	case "bracketguides":
		o.Guides.BracketPairs = BracketGuidesOn
	default:
		return false
	}
	return true
}

// perfApplyOn turns on in o the features named by spec (a
// comma-separated list of perfOnNames; blanks and empty items are
// ignored). An unknown name is an error.
func perfApplyOn(o *Options, spec string) error {
	for _, name := range strings.Split(spec, ",") {
		if name = strings.TrimSpace(name); name != "" && !perfOn(o, name) {
			return fmt.Errorf("%s: unknown feature %q (want %s)", perfOnEnv, name, strings.Join(perfOnNames, ", "))
		}
	}
	return nil
}

// perfApplyOffEnv applies $TANGO_EDITOR_PERF_OFF, then
// $TANGO_EDITOR_PERF_ON, to st's options, failing tb on an unknown name.
func perfApplyOffEnv(tb testing.TB, st *State) {
	tb.Helper()
	if spec := os.Getenv(perfOffEnv); spec != "" {
		if err := perfApplyOff(&st.Options, spec); err != nil {
			tb.Fatalf("%v", err)
		}
	}
	if spec := os.Getenv(perfOnEnv); spec != "" {
		if err := perfApplyOn(&st.Options, spec); err != nil {
			tb.Fatalf("%v", err)
		}
	}
}

// TestPerfApplyOn checks perfApplyOn: bracketguides turns on the bracket
// pair guides (only), an unknown name is an error.
func TestPerfApplyOn(t *testing.T) {
	def := DefaultOptions()
	got, want := def, def
	if err := perfApplyOn(&got, " bracketguides, "); err != nil {
		t.Fatal(err)
	}
	want.Guides.BracketPairs = BracketGuidesOn
	if !reflect.DeepEqual(got, want) {
		t.Errorf("bracketguides:\n got %+v\nwant %+v", got, want)
	}
	if err := perfApplyOn(&got, "minimap"); err == nil {
		t.Error("unknown name accepted")
	}
}

// TestPerfApplyOff checks perfApplyOff: every feature name turns off its
// option fields (and only those), "all" turns off every feature, and an
// unknown name is an error.
func TestPerfApplyOff(t *testing.T) {
	def := DefaultOptions()
	for _, c := range []struct {
		name string
		want func(o *Options)
	}{
		{"minimap", func(o *Options) { o.Minimap.Enabled = false }},
		{"sticky", func(o *Options) { o.StickyScroll.Enabled = false }},
		{"overview", func(o *Options) { o.OverviewRulerLanes = 0 }},
		{"folding", func(o *Options) { o.Folding = false }},
		{"brackets", func(o *Options) { o.BracketPairColorization = false }},
		{"guides", func(o *Options) { o.Guides.Indentation, o.Guides.HighlightActiveIndentation = false, false }},
		{"matchbrackets", func(o *Options) { o.MatchBrackets = MatchBracketsNever }},
		{"occurrences", func(o *Options) { o.OccurrencesHighlight = OccurrencesOff }},
		{"selectionhl", func(o *Options) { o.SelectionHighlight = false }},
		{"unicode", func(o *Options) {
			o.UnicodeHighlight.InvisibleCharacters, o.UnicodeHighlight.AmbiguousCharacters = false, false
			o.UnicodeHighlight.NonBasicASCII = UnicodeHighlightFalse
		}},
		{"links", func(o *Options) { o.Links = false }},
		{"colors", func(o *Options) { o.ColorDecorators = false }},
		{"whitespace", func(o *Options) { o.RenderWhitespace = RenderWhitespaceNone }},
		{"linehl", func(o *Options) { o.RenderLineHighlight = RenderLineHighlightNone }},
		{"minimapchars", func(o *Options) { o.Minimap.RenderCharacters = false }},
	} {
		got, want := def, def
		if err := perfApplyOff(&got, c.name); err != nil {
			t.Fatalf("%s: %v", c.name, err)
		}
		c.want(&want)
		if reflect.DeepEqual(want, def) {
			t.Fatalf("%s: test expects no change", c.name)
		}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("%s:\n got %+v\nwant %+v", c.name, got, want)
		}
	}

	all := def
	if err := perfApplyOff(&all, "all"); err != nil {
		t.Fatal(err)
	}
	each := def
	for _, n := range perfOffNames {
		if err := perfApplyOff(&each, n); err != nil {
			t.Fatal(err)
		}
	}
	if len(perfOffNames) != 15 || !reflect.DeepEqual(all, each) {
		t.Errorf("all = %+v, want every feature off: %+v", all, each)
	}

	combo := def
	if err := perfApplyOff(&combo, " minimap, sticky ,"); err != nil {
		t.Fatal(err)
	}
	if combo.Minimap.Enabled || combo.StickyScroll.Enabled || !combo.Folding {
		t.Errorf("list: %+v", combo)
	}
	empty := def
	if err := perfApplyOff(&empty, ""); err != nil || !reflect.DeepEqual(empty, def) {
		t.Errorf("empty spec changed options or failed: %v", err)
	}
	if err := perfApplyOff(&combo, "minimap,nope"); err == nil {
		t.Error("unknown name nope: no error")
	}
	if err := perfApplyOff(&combo, "nope"); err == nil {
		t.Error("unknown name nope: no error")
	}
}

// TestPerfModelOptions checks the detectindent name of perfOffEnv: it is
// accepted by perfApplyOff without changing the editor options, and turns
// off DetectIndentation in perfModelOptions.
func TestPerfModelOptions(t *testing.T) {
	def := DefaultOptions()
	got := def
	if err := perfApplyOff(&got, "detectindent"); err != nil || !reflect.DeepEqual(got, def) {
		t.Errorf("perfApplyOff(detectindent): %v, options changed %v", err, !reflect.DeepEqual(got, def))
	}
	t.Setenv(perfOffEnv, "")
	if o := perfModelOptions(); o != DefaultModelOptions() {
		t.Errorf("no spec: %+v", o)
	}
	t.Setenv(perfOffEnv, "minimap, detectindent")
	if o := perfModelOptions(); o.DetectIndentation || o.TabSize != DefaultTabSize || !o.InsertSpaces {
		t.Errorf("detectindent: %+v", o)
	}
}
