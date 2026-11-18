//go:build !darwin || ios || !tango_coretext

package editor

import (
	"image"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tangoui/tango/ui/editor/theme"
	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/input"
	"github.com/tangoui/tango/ui/text"
)

// Golden images of the View, recorded with the pure-Go (go-text) text
// engine: exact pixels of its shaping and rasterization. The opt-in macOS
// build tag tango_coretext has only CoreText (text.UseSystemEngine(false) is
// a no-op there), so they do not build with it.

// goldenSource is the document of the golden images: keywords, strings,
// comments, numbers, a tab, CJK and a selection over two lines.
const goldenSource = `// Package demo 演示高亮
package demo

import "fmt"

func Sum(xs []int) (n int) {
	for _, x := range xs {
		n += x * 2 // double
	}
	fmt.Println("sum:", n, 0x1F)
	return
}
`

func TestViewGolden(t *testing.T) {
	goText(t)
	got := strings.Join(text.Families(), "\n")
	fonts := filepath.Join("testdata", "golden_fonts.txt")
	if *update {
		os.MkdirAll("testdata", 0o755)
		os.WriteFile(fonts, []byte(got), 0o644)
	} else if want, err := os.ReadFile(fonts); err != nil || string(want) != got {
		t.Skip("golden images were recorded with different fonts")
	}
	for _, name := range []string{"vs", "vs-dark"} {
		t.Run(name, func(t *testing.T) {
			st := NewState(goldenSource, "demo.go", theme.Base(name))
			d, _ := viewDriver(st, 520, 300, nil)
			d.Frame()
			d.Click(geom.Pt(400, 280)) // focus
			snap := st.Snapshot()
			// Select from "range" on line 6 to "x * 2" on line 7.
			a := snap.LineStart(6) + strings.Index(snap.Line(6), "range")
			b := snap.LineStart(7) + strings.Index(snap.Line(7), "x *")
			st.MoveCaret(a, false)
			st.MoveCaret(b, true)
			checkGolden(t, d.Frame(), "view_"+strings.ReplaceAll(name, "-", "_")+".png")
		})
		// Every display decoration of Task 12 on: relative line numbers,
		// whitespace marks, a control character, indentation guides with
		// the active one, colored brackets (an unexpected one), the matched
		// pair at the caret, the line highlight in the gutter and the text,
		// and a top padding.
		t.Run(name+"/decorations", func(t *testing.T) {
			st := NewState(goldenDecoSource, "demo.go", theme.Base(name))
			o := &st.Options
			o.LineNumbers = LineNumbersRelative
			o.RenderWhitespace = RenderWhitespaceAll
			o.RenderLineHighlight = RenderLineHighlightAll
			o.Padding = PaddingOptions{Top: 6}
			d, _ := viewDriver(st, 520, 300, nil)
			d.Frame()
			d.Click(geom.Pt(400, 280)) // focus
			snap := st.Snapshot()
			// The caret after the "{" of the for loop (line 4).
			st.MoveCaret(snap.LineStart(4)+strings.Index(snap.Line(4), "{")+1, false)
			checkGolden(t, d.Frame(), "view_"+strings.ReplaceAll(name, "-", "_")+"_decorations.png")
		})
		// Bracket pair guides for every pair, the caret's pair (the if
		// block) highlighted with its horizontal lines.
		t.Run(name+"/bracketguides", func(t *testing.T) {
			st := NewState(goldenDecoSource, "demo.go", theme.Base(name))
			st.Options.Guides.BracketPairs = BracketGuidesOn
			d, _ := viewDriver(st, 520, 300, nil)
			d.Frame()
			d.Click(geom.Pt(400, 280)) // focus
			snap := st.Snapshot()
			st.MoveCaret(snap.LineStart(6)+3, false) // in the if block
			checkGolden(t, d.Frame(), "view_"+strings.ReplaceAll(name, "-", "_")+"_bracketguides.png")
		})
	}
}

// goldenDecoSource is the document of the decorations golden images.
const goldenDecoSource = "// Package demo 演示装饰\n" +
	"package demo\n" +
	"\n" +
	"func Sum(xs []int) (n int) {\n" +
	"\tfor _, x := range xs {\n" +
	"\t\tif x > 0 {\n" +
	"\t\t\tn += x  // twice\n" +
	"\n" +
	"\t\t}\n" +
	"\t}\n" +
	"\treturn n \x01\n" +
	"}\n" +
	"\n" +
	"var bad = [1, 2)\n"

// checkGolden compares img with testdata/file (rewritten with -update).
func checkGolden(t *testing.T, img image.Image, file string) {
	t.Helper()
	path := filepath.Join("testdata", file)
	if *update {
		if err := savePNG(path, img); err != nil {
			t.Fatal(err)
		}
		return
	}
	want, err := loadPNG(path)
	if err != nil {
		t.Fatalf("missing golden (go test ./ui/editor -run Golden -update): %v", err)
	}
	if diff := comparePNG(img, want, 24); diff > 0.005 {
		out := filepath.Join(t.TempDir(), filepath.Base(path))
		savePNG(out, img)
		t.Fatalf("%.2f%% pixels differ; got %s", diff*100, out)
	}
}

// goldenColorSource is the document of the color golden images.
const goldenColorSource = "a {\n" +
	"  color: #ff0000;\n" +
	"  background: rgba(0, 0, 255, 0.5);\n" +
	"  border: 1px solid hsl(120, 100%, 25%);\n" +
	"}\n" +
	"#fff at the line start, \"#0f08\" in quotes\n"

// TestColorInjectedGolden: the swatches in their gaps (Monaco's
// .colorpicker-color-decoration: a 0.8em box with a 0.1em border, black or
// #eee on dark themes, 0.2em margins), a translucent one over the
// background, a selection across a gap (covered), one of a literal's "#"
// (not covered) and its caret right of the gap after Shift+Left.
func TestColorInjectedGolden(t *testing.T) {
	goText(t)
	if want, err := os.ReadFile(filepath.Join("testdata", "golden_fonts.txt")); err != nil || string(want) != strings.Join(text.Families(), "\n") {
		t.Skip("golden images were recorded with different fonts")
	}
	for _, name := range []string{"vs", "vs-dark"} {
		t.Run(name, func(t *testing.T) {
			st := NewState(goldenColorSource, "demo.css", theme.Base(name))
			d, _ := viewDriver(st, 520, 200, nil)
			d.Frame()
			d.Click(geom.Pt(400, 190)) // focus
			snap := st.Snapshot()
			a := snap.LineStart(3) + strings.Index(snap.Line(3), "solid")
			st.SetSelections([]Selection{
				{Anchor: a, Head: a + len("solid hsl")},
				{Anchor: snap.LineStart(1) + 10, Head: snap.LineStart(1) + 10},
			})
			d.Frame()
			// Shift+Left: the selection still crosses its gap; the caret
			// selects "#" and is drawn right of its gap.
			d.Key(input.KeyLeft, input.ModShift)
			checkGolden(t, d.Frame(), "view_"+strings.ReplaceAll(name, "-", "_")+"_colors.png")
		})
	}
}
