package editor

import (
	"fmt"
	"image/color"
	"math"
	"strings"
	"testing"

	"github.com/tangoui/tango/ui/editor/theme"
	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/input"
)

func TestDenseCursorHorizontalDamage(t *testing.T) {
	goText(t)
	for _, scale := range []float32{1, 1.25, 2} {
		t.Run(fmt.Sprint(scale), func(t *testing.T) {
			const count = 18
			var doc strings.Builder
			var sels []Selection
			for i := range count {
				doc.WriteString(strings.Repeat("x", 1+i%9))
				sels = append(sels, Selection{Anchor: doc.Len(), Head: doc.Len()})
				doc.WriteByte('\n')
			}
			st := NewState(doc.String(), "a.txt", theme.Base("vs-dark"))
			t.Cleanup(st.Close)
			st.Options.Minimap.Enabled = false
			st.Options.RenderLineHighlight = RenderLineHighlightNone
			st.Options.OccurrencesHighlight = OccurrencesOff
			h := newHintHost(t, st, geom.Size{W: 720, H: 400}, scale)
			h.focusAt(0)
			st.SetSelections(sels)
			h.frame()
			h.typeText("j")
			requireNarrowRows(h, "type at 18 carets")
			h.key(input.KeyBackspace, 0)
			requireNarrowRows(h, "shorten 18 lines")
			h.key(input.KeyBackspace, 0)
			requireNarrowRows(h, "shorten lines and delete one-character lines to empty")
			snap := st.Snapshot()
			if snap.Line(0) != "" || snap.Line(8) != "xxxxxxxx" || snap.LineCount() != count+1 {
				t.Fatal("backspace did not shorten the lines as expected")
			}
		})
	}
}

func requireNarrowRows(h *hintHost, label string) {
	h.t.Helper()
	r := h.requirePartialStep(label).frames[0]
	area := 0
	for _, rect := range r.dirty {
		area += rect.Dx() * rect.Dy()
	}
	if r.copies != 0 || area*4 >= r.size.X*r.size.Y {
		h.t.Fatalf("%s: repaint %d pixels of %v with %d copies, want below a quarter with no scroll", label, area, r.size, r.copies)
	}
}

func TestHorizontalDamageIncludesGlyphOverhang(t *testing.T) {
	goText(t)
	th, err := theme.Parse([]byte(`{
		"base": "vs-dark", "inherit": true,
		"rules": [{"token": "comment", "fontStyle": "italic"}]
	}`))
	if err != nil {
		t.Fatal(err)
	}
	const line = "// f"
	st := NewState(line+"\n\n\n", "a.go", th)
	t.Cleanup(st.Close)
	st.Options.Minimap.Enabled = false
	st.Options.RenderLineHighlight = RenderLineHighlightNone
	st.Options.FontSize = 52
	const scale = 1.25
	h := newHintHost(t, st, geom.Size{W: 900, H: 325}, scale)
	h.focusAt(2)
	before, err := h.dev.ReadPixels()
	if err != nil {
		t.Fatal(err)
	}
	snap := st.Snapshot()
	vw := h.vw
	lv := vw.layoutLine(snap, 0, st.Highlighter(), th, vw.vs.colW)
	advanceEnd := int(math.Ceil(float64((vw.vs.text.X + lv.endX) * scale)))
	// Edit through a second view so the caret's independent damage cannot
	// cover missing ink to the right of the deleted glyph.
	other := NewView(st.Model, th)
	t.Cleanup(other.Close)
	other.SetSelections([]Selection{{Anchor: len(line) - 1, Head: len(line)}})
	other.Insert("")
	requireNarrowRows(h, "delete overhanging italic f")
	after, err := h.dev.ReadPixels()
	if err != nil {
		t.Fatal(err)
	}
	for y := range int(st.Viewport.LineHeight * scale) {
		for x := advanceEnd; x < before.Bounds().Max.X; x++ {
			if before.RGBAAt(x, y) != after.RGBAAt(x, y) {
				return
			}
		}
	}
	t.Fatal("fixture drew no changed pixels to the right of the line's advance")
}

func TestHorizontalDamageFallsBackForComplexRows(t *testing.T) {
	goText(t)
	for _, name := range []string{"line highlight", "line numbers", "gutter highlight", "soft wrap", "fold", "view zone", "IME", "decoration", "line count"} {
		t.Run(name, func(t *testing.T) {
			st := NewState("top\nshort\n\tinside\nend\n\nlast\n", "a.txt", theme.Base("vs-dark"))
			t.Cleanup(st.Close)
			st.Options.Minimap.Enabled = false
			st.Options.RenderLineHighlight = RenderLineHighlightNone
			switch name {
			case "line highlight":
				st.Options.RenderLineHighlight = RenderLineHighlightLine
			case "line numbers":
				st.Options.LineNumbers = LineNumbersRelative
			case "gutter highlight":
				st.Options.RenderLineHighlight = RenderLineHighlightGutter
			case "soft wrap":
				st.Options.WordWrap = WrapColumn
				st.Options.WordWrapColumn = 5
			case "view zone":
				st.SetViewZones("test", []ViewZone{{ID: 1, AfterLine: 3, Rows: 1}})
			}
			h := newHintHost(t, st, geom.Size{W: 840, H: 400}, 1.25)
			h.focusAt(0)
			if name == "fold" {
				caretAt(st, 1, 0)
				if !st.Execute("editor.fold", nil) || len(st.Folded()) == 0 {
					t.Fatal("expected a folded block")
				}
				caretAt(st, 0, 1)
				h.frame()
			}
			switch name {
			case "line highlight", "line numbers", "gutter highlight":
				caretAt(st, 1, 0)
			case "IME":
				h.r.HandleEvent(input.Composition{Text: "ni", Cursor: 2})
			case "decoration":
				st.SetDecorations("test", []Decoration{{Start: 0, End: 1, Options: &DecorationOptions{
					WholeLine: true, Background: color.NRGBA{B: 170, A: 255},
				}}})
			case "line count":
				h.key(input.KeyEnter, 0)
			default:
				h.typeText("x")
			}
			h.requirePartialStep(name)
			if name == "line count" && st.Snapshot().LineCount() != 8 {
				t.Fatal("Enter did not insert a line")
			}
			dam, shifts := h.lineHints()
			if len(shifts) != 0 {
				t.Fatalf("unexpected scroll shifts %v", shifts)
			}
			for _, r := range dam {
				if r.X <= h.vw.vs.gutter.X && r.Right() >= h.vw.vs.text.Right() {
					return
				}
			}
			t.Fatalf("damage %v did not cover the full row width", dam)
		})
	}
}

func TestHorizontalDamageRepaintsLineNumberOnThemeChange(t *testing.T) {
	goText(t)
	st := NewState("a", "a.txt", theme.Base("vs"))
	t.Cleanup(st.Close)
	st.Theme = nil
	st.Options.LineNumbers = LineNumbersOn
	st.Options.Minimap.Enabled = false
	st.Options.RenderLineHighlight = RenderLineHighlightNone
	h := newHintHost(t, st, geom.Size{W: 720, H: 400}, 2)
	h.focusAt(0)
	st.Theme = theme.Base("vs")
	h.typeText("b")
	h.requirePartialStep("theme change and edit")
}
