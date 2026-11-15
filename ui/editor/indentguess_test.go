package editor

import (
	"encoding/json"
	"math/rand/v2"
	"os"
	"runtime"
	"strings"
	"testing"
	"unicode/utf16"

	"github.com/tangoui/tango/ui/editor/buffer"
	"github.com/tangoui/tango/ui/editor/theme"
)

// testdata/indentguess/samples.json is Monaco 0.56's guessIndentation over
// the samples of langs/gen/export_indentguess.mjs.
type indentGuessSample struct {
	Name    string `json:"name"`
	Text    string `json:"text"`
	Results []struct {
		DefaultTabSize      int  `json:"defaultTabSize"`
		DefaultInsertSpaces bool `json:"defaultInsertSpaces"`
		TabSize             int  `json:"tabSize"`
		InsertSpaces        bool `json:"insertSpaces"`
	} `json:"results"`
}

func TestGuessIndentationMonacoParity(t *testing.T) {
	data, err := os.ReadFile("testdata/indentguess/samples.json")
	if err != nil {
		t.Fatal(err)
	}
	var samples []indentGuessSample
	if err := json.Unmarshal(data, &samples); err != nil {
		t.Fatal(err)
	}
	if len(samples) < 30 {
		t.Fatalf("%d samples, want at least 30", len(samples))
	}
	for _, s := range samples {
		snap := buffer.NewSnapshot(s.Text)
		for _, r := range s.Results {
			ts, sp := GuessIndentation(snap, r.DefaultTabSize, r.DefaultInsertSpaces)
			if ts != r.TabSize || sp != r.InsertSpaces {
				t.Errorf("%s (defaults %d,%v): got %d,%v; Monaco %d,%v", s.Name,
					r.DefaultTabSize, r.DefaultInsertSpaces, ts, sp, r.TabSize, r.InsertSpaces)
			}
			// The reference of TestGuessIndentationRandom agrees too.
			lf := strings.ReplaceAll(strings.ReplaceAll(s.Text, "\r\n", "\n"), "\r", "\n")
			if ts, sp := refGuessIndentation(lf, r.DefaultTabSize, r.DefaultInsertSpaces); ts != r.TabSize || sp != r.InsertSpaces {
				t.Errorf("%s (defaults %d,%v): reference %d,%v; Monaco %d,%v", s.Name,
					r.DefaultTabSize, r.DefaultInsertSpaces, ts, sp, r.TabSize, r.InsertSpaces)
			}
		}
	}
}

func TestNewModelDetectsIndentation(t *testing.T) {
	for _, c := range []struct {
		text   string
		tab    int
		spaces bool
	}{
		{"func f() {\n\tif x {\n\t\ty()\n\t}\n}\n", 4, false},
		{"a:\n  b:\n    c: 1\n  d: 2\n", 2, true},
		{"x\ny\n", DefaultTabSize, true},
	} {
		m := NewModel("f.txt", c.text, nil)
		if m.TabSize() != c.tab || m.InsertSpaces() != c.spaces {
			t.Errorf("NewModel(%q): %d,%v; want %d,%v", c.text, m.TabSize(), m.InsertSpaces(), c.tab, c.spaces)
		}
		st := NewState(c.text, "f.txt", nil)
		if st.Model.TabSize() != c.tab || st.Model.InsertSpaces() != c.spaces {
			t.Errorf("NewState(%q): %d,%v; want %d,%v", c.text, st.Model.TabSize(), st.Model.InsertSpaces(), c.tab, c.spaces)
		}
	}
	if o := DefaultModelOptions(); o != (ModelOptions{TabSize: 4, InsertSpaces: true, DetectIndentation: true}) {
		t.Errorf("DefaultModelOptions() = %+v", o)
	}
}

func TestDetectIndentationOptOut(t *testing.T) {
	text := "a {\n  b\n}\n"
	m := NewModelWithOptions("f.txt", text, nil, ModelOptions{TabSize: 8, InsertSpaces: false})
	if m.TabSize() != 8 || m.InsertSpaces() {
		t.Errorf("DetectIndentation=false: %d,%v; want 8,false", m.TabSize(), m.InsertSpaces())
	}
	// Detection starts from the given defaults: nothing to detect keeps them.
	m = NewModelWithOptions("f.txt", "x\n", nil, ModelOptions{TabSize: 3, InsertSpaces: false, DetectIndentation: true})
	if m.TabSize() != 3 || m.InsertSpaces() {
		t.Errorf("defaults 3,false on flat text: %d,%v", m.TabSize(), m.InsertSpaces())
	}

	p := NewModelPool(nil)
	if m := p.Open("a.txt", text); m.TabSize() != 2 || !m.InsertSpaces() {
		t.Errorf("pool with zero Options: %d,%v; want 2,true (detected)", m.TabSize(), m.InsertSpaces())
	}
	p.Options = ModelOptions{TabSize: 8, InsertSpaces: true}
	if m := p.Open("b.txt", text); m.TabSize() != 8 || !m.InsertSpaces() {
		t.Errorf("pool with DetectIndentation=false: %d,%v; want 8,true", m.TabSize(), m.InsertSpaces())
	}
}

func TestDetectIndentationNotReapplied(t *testing.T) {
	st := NewState("a {\n  b\n}\n", "f.txt", nil)
	if st.Model.TabSize() != 2 {
		t.Fatalf("detected %d, want 2", st.Model.TabSize())
	}
	st.Model.SetTabSize(6)
	st.Model.SetInsertSpaces(true)
	snap := st.Snapshot()
	if err := st.Edit([]buffer.Edit{{Start: 0, End: snap.Len(), Text: "a {\n\tb\n\t\tc\n}\n"}}, Selection{}); err != nil {
		t.Fatal(err)
	}
	if st.Model.TabSize() != 6 || !st.Model.InsertSpaces() {
		t.Errorf("after replacing the text: %d,%v; want the manual 6,true", st.Model.TabSize(), st.Model.InsertSpaces())
	}
}

func TestDetectIndentationCommand(t *testing.T) {
	st := NewState("a {\n  b\n}\n", "f.txt", nil)
	snap := st.Snapshot()
	if err := st.Edit([]buffer.Edit{{Start: 0, End: snap.Len(), Text: "a {\n\tb\n\t\tc\n}\n"}}, Selection{}); err != nil {
		t.Fatal(err)
	}
	if !st.Execute("editor.action.detectIndentation", nil) {
		t.Fatal("editor.action.detectIndentation did not run")
	}
	if st.Model.TabSize() != 4 || st.Model.InsertSpaces() {
		t.Errorf("after the command: %d,%v; want 4,false", st.Model.TabSize(), st.Model.InsertSpaces())
	}

	// The command starts from the creation options, as Monaco passes
	// getCreationOptions to model.detectIndentation, not the current values.
	m := NewModelWithOptions("f.txt", "x\ny\n", nil, ModelOptions{TabSize: 3, InsertSpaces: false})
	m.SetTabSize(7)
	m.SetInsertSpaces(true)
	m.DetectIndentation()
	if m.TabSize() != 3 || m.InsertSpaces() {
		t.Errorf("DetectIndentation on flat text: %d,%v; want the creation defaults 3,false", m.TabSize(), m.InsertSpaces())
	}

	// Read-only views can run it: it changes options, not text.
	ro := NewState(strings.Repeat("x\n  y\n", 3), "f.txt", nil)
	ro.Model.SetTabSize(8)
	ro.Options.ReadOnly = true
	if !ro.Execute("editor.action.detectIndentation", nil) || ro.Model.TabSize() != 2 {
		t.Errorf("read-only: tab size %d, want 2", ro.Model.TabSize())
	}
}

// testModelOptions are the creation options of Monaco's unit-test models
// (TextModel.DEFAULT_CREATION_OPTIONS: tabSize 4, insertSpaces true,
// detectIndentation false). The command tables derived from Monaco's
// behaviour assume them, as their small fixtures would otherwise be
// detected as indented with tabs or 2 or 8 spaces.
var testModelOptions = ModelOptions{TabSize: 4, InsertSpaces: true}

// newTestState is NewState with testModelOptions.
func newTestState(text, file string, th *theme.Theme) *State {
	return NewView(NewModelWithOptions(file, text, DefaultRegistry(), testModelOptions), th)
}

// BenchmarkGuessIndentation: detection reads at most 10000 lines, and only
// their indentation, so a 100 MB document costs the same as its top.
func BenchmarkGuessIndentation(b *testing.B) {
	for _, c := range []struct {
		name string
		text string
	}{
		{"1Mlines", strings.Repeat("function f() {\n    if (x) {\n        return y + z; // a comment to make the line long enough\n    }\n}\n", 200_000)},
		{"100MB-one-line", "  " + strings.Repeat("x", 100<<20)},
	} {
		snap := buffer.NewSnapshot(c.text)
		b.Run(c.name, func(b *testing.B) {
			for b.Loop() {
				GuessIndentation(snap, 4, true)
			}
		})
	}
}

// refGuessIndentation is guessIndentation transcribed literally over whole
// lines of UTF-16 code units, the reference for the windowed reader.
func refGuessIndentation(text string, defTab int, defSpaces bool) (int, bool) {
	var lines [][]uint16
	for _, l := range strings.Split(text, "\n") {
		lines = append(lines, utf16.Encode([]rune(l)))
	}
	spacesDiff := func(a []uint16, aLen int, b []uint16, bLen int) (int, bool) {
		i := 0
		for i < aLen && i < bLen && a[i] == b[i] {
			i++
		}
		as, at, bs, bt := 0, 0, 0, 0
		for j := i; j < aLen; j++ {
			if a[j] == ' ' {
				as++
			} else {
				at++
			}
		}
		for j := i; j < bLen; j++ {
			if b[j] == ' ' {
				bs++
			} else {
				bt++
			}
		}
		if as > 0 && at > 0 || bs > 0 && bt > 0 {
			return 0, false
		}
		td, sd := absInt(at-bt), absInt(as-bs)
		if td == 0 {
			align := sd > 0 && 0 <= bs-1 && bs-1 < len(a) && bs < len(b) &&
				b[bs] != ' ' && a[bs-1] == ' ' && a[len(a)-1] == ','
			return sd, align
		}
		if sd%td == 0 {
			return sd / td, false
		}
		return 0, false
	}
	tabLines, spaceLines := 0, 0
	var prev []uint16
	prevInd := 0
	var count [9]int
	for n := 0; n < min(len(lines), 10000); n++ {
		cur := lines[n]
		has, ind, sp, tb := false, 0, 0, 0
		for j, c := range cur {
			if c == '\t' {
				tb++
			} else if c == ' ' {
				sp++
			} else {
				has, ind = true, j
				break
			}
		}
		if !has {
			continue
		}
		if tb > 0 {
			tabLines++
		} else if sp > 1 {
			spaceLines++
		}
		d, align := spacesDiff(prev, prevInd, cur, ind)
		if align && !(defSpaces && defTab == d) {
			continue
		}
		if d <= 8 {
			count[d]++
		}
		prev, prevInd = cur, ind
	}
	spaces := defSpaces
	if tabLines != spaceLines {
		spaces = tabLines < spaceLines
	}
	tab := defTab
	if spaces {
		score := 0
		for _, n := range []int{2, 4, 6, 8, 3, 5, 7} {
			if count[n] > score {
				score, tab = count[n], n
			}
		}
		if tab == 4 && count[4] > 0 && count[2] > 0 && float64(count[2]) >= float64(count[4])*2/3 {
			tab = 2
		}
	}
	return tab, spaces
}

// TestGuessIndentationRandom compares GuessIndentation with the literal
// transcription on random documents, with lines and runs of whitespace
// longer than the reader's window and non-ASCII text after indentation.
func TestGuessIndentationRandom(t *testing.T) {
	rng := rand.New(rand.NewPCG(1, 2))
	pieces := []string{"", " ", "  ", "    ", "\t", "\t ", "        ", " \t"}
	words := []string{"x", "a,", "é b,", "😀 b,", "// c", "}", " * d", "y = 1,"}
	for iter := 0; iter < 400; iter++ {
		var sb strings.Builder
		n := rng.IntN(40)
		for i := 0; i < n; i++ {
			for k := rng.IntN(4); k > 0; k-- {
				sb.WriteString(pieces[rng.IntN(len(pieces))])
			}
			switch rng.IntN(30) {
			case 0:
				sb.WriteString(strings.Repeat(" ", guessWindow+rng.IntN(100)))
			case 1:
				sb.WriteString(strings.Repeat("z", guessWindow+rng.IntN(100)))
			}
			if rng.IntN(5) > 0 {
				sb.WriteString(words[rng.IntN(len(words))])
			}
			sb.WriteByte('\n')
		}
		text := sb.String()
		snap := buffer.NewSnapshot(text)
		for _, d := range []struct {
			tab    int
			spaces bool
		}{{4, true}, {2, false}, {2, true}, {8, true}} {
			gt, gs := GuessIndentation(snap, d.tab, d.spaces)
			wt, ws := refGuessIndentation(text, d.tab, d.spaces)
			if gt != wt || gs != ws {
				t.Fatalf("iter %d defaults %v: got %d,%v want %d,%v\n%q", iter, d, gt, gs, wt, ws, text)
			}
		}
	}
}

// longLinesDoc is n lines each longer than guessWindow, indented by two
// spaces.
func longLinesDoc(n int) buffer.Snapshot {
	line := "  " + strings.Repeat("x", guessWindow+2<<10) + "\n"
	return buffer.NewSnapshot(strings.Repeat(line, n))
}

// TestGuessIndentationLongLinesBoundedAlloc: many lines longer than the
// window do not make detection read (allocate) the whole text.
func TestGuessIndentationLongLinesBoundedAlloc(t *testing.T) {
	const n = 200 // about 13 MB of text
	snap := longLinesDoc(n)
	var before, after runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&before)
	ts, sp := GuessIndentation(snap, 4, true)
	runtime.ReadMemStats(&after)
	if ts != 2 || !sp { // the first line's 2 spaces are the only diff
		t.Errorf("got %d,%v; want 2,true", ts, sp)
	}
	alloc := after.TotalAlloc - before.TotalAlloc
	if limit := uint64(guessWindow + n*2*guessWindowMin); alloc > limit {
		t.Errorf("allocated %d bytes for %d long lines, want <= %d", alloc, n, limit)
	}
}

// BenchmarkGuessIndentationLongLines: 1600 lines of 66 KB (about 100 MB).
func BenchmarkGuessIndentationLongLines(b *testing.B) {
	snap := longLinesDoc(1600)
	b.ReportAllocs()
	for b.Loop() {
		GuessIndentation(snap, 4, true)
	}
}
