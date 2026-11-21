package editor

import (
	"cmp"
	"image/color"
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/tangoui/tango/ui/editor/buffer"
	"github.com/tangoui/tango/ui/editor/theme"
	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/paint"
	"github.com/tangoui/tango/ui/text"
)

// InlayHint inserts a visual label before a UTF-8 byte Offset in the
// document. The offset may be at a line end. Equal offsets retain input
// order. A transparent Foreground uses the editor's subdued foreground.
type InlayHint struct {
	Offset     int
	Text       string
	Foreground color.NRGBA
	Background color.NRGBA
}

type inlaySet struct {
	version uint64
	hints   []InlayHint
	layout  *inlayLayout
}

type inlayLayout struct {
	size  float32
	lines map[int][]injection
}

type inlayGlyph struct {
	hint InlayHint
	text *text.Layout
	pad  float32
}

// SetInlayHints replaces this view's visual hints for the given snapshot
// version. It returns false and keeps the current hints if a newer
// document version is active. Hints expire on a document edit. Pass nil
// to clear them. Empty or multiline labels and invalid offsets are
// ignored. Offsets on a rune boundary inside a grapheme snap left.
// Unwrapped long lines use the editor's chunked layout and omit hints.
func (s *State) SetInlayHints(version uint64, hints []InlayHint) bool {
	snap := s.Snapshot()
	if snap.Version() != version {
		return false
	}
	if s.inlays != nil && s.inlays.version == version && slices.Equal(s.inlays.hints, hints) {
		return true
	}
	if len(hints) == 0 {
		s.inlays = nil
		return true
	}
	s.inlays = &inlaySet{version: version, hints: slices.Clone(hints)}
	return true
}

func (w *ViewW) inlayLayout() *inlayLayout {
	data := w.st.inlays
	if data == nil {
		return nil
	}
	snap := w.st.Snapshot()
	if data.version != snap.Version() {
		return nil
	}
	size := w.opts.fontSize()
	if data.layout != nil && data.layout.size == size {
		return data.layout
	}
	lay := &inlayLayout{size: size, lines: make(map[int][]injection)}
	hints := slices.Clone(data.hints)
	slices.SortStableFunc(hints, func(a, b InlayHint) int { return cmp.Compare(a.Offset, b.Offset) })
	style := text.Style{Size: size * 0.9, Mono: true}
	line, start, at, next := -1, 0, 0, 0
	var src string
	for _, h := range hints {
		if h.Offset < 0 || h.Offset > snap.Len() || h.Text == "" || strings.ContainsAny(h.Text, "\r\n\t") {
			continue
		}
		l := snap.LineOf(h.Offset)
		if l != line {
			line, start, src = l, snap.LineStart(l), snap.Line(l)
			at, next = 0, 0
		}
		col := h.Offset - start
		if col > len(src) || col < len(src) && !utf8.RuneStart(src[col]) {
			continue
		}
		for next <= col && at < len(src) {
			at = next
			if at == len(src) {
				break
			}
			next = nextGrapheme(src, at)
		}
		pad := size * 0.2
		label := text.Shape(h.Text, style, text.Options{NoWrap: true})
		entry := &inlayGlyph{hint: h, text: label, pad: pad}
		lay.lines[l] = append(lay.lines[l], injection{col: at, w: label.Width + 2*pad, hint: entry})
	}
	data.layout = lay
	return lay
}

func (cfg wrapConfig) injections(line int, s string, dst []injection) []injection {
	n := len(dst)
	dst = cfg.inj.injections(s, dst)
	return appendInlays(dst, n, cfg.hints, line)
}

func appendInlays(dst []injection, n int, hints *inlayLayout, line int) []injection {
	if hints == nil || len(hints.lines[line]) == 0 {
		return dst
	}
	m := len(dst)
	dst = append(dst, hints.lines[line]...)
	if m > n {
		slices.SortStableFunc(dst[n:], func(a, b injection) int { return cmp.Compare(a.col, b.col) })
	}
	return dst
}

func (w *ViewW) paintInlays(l *paint.List, lv *lineView, tx, y, lh float32, th *theme.Theme) {
	lv.eachGap(func(in injection, x0, x1 float32) {
		h := in.hint
		if h == nil {
			return
		}
		fg := h.hint.Foreground
		if fg.A == 0 {
			fg = defaultColor(th, "editorCodeLens.foreground")
			if fg.A == 0 {
				fg = defaultColor(th, "editor.foreground")
			}
		}
		x := tx + x0
		if h.hint.Background.A > 0 {
			l.Fill(geom.R(x, y, x1-x0, lh), geom.Radii{}, h.hint.Background)
		}
		l.Glyphs(geom.Pt(x+h.pad, y+(lh-h.text.Height)/2), h.text.Glyphs, fg)
	})
}

func (w *ViewW) inlayAt(snap buffer.Snapshot, p geom.Point) (int, bool) {
	r := w.pointRow(p)
	vl := w.st.lines()
	if r < 0 || r >= vl.Count() {
		return 0, false
	}
	line, row := vl.find(r)
	if row < 0 || row >= vl.rowsOf(line) {
		return 0, false
	}
	lv := w.rowAt(snap, line, row)
	if len(lv.segs) != len(lv.inj)+1 {
		return 0, false
	}
	x := p.X - w.vs.text.X + w.displayViewport().ScrollX
	for k, in := range lv.inj {
		if in.hint != nil {
			right := lv.segs[k+1].startX()
			if x >= right-in.w && x < right {
				return snap.LineStart(line) + in.col, true
			}
		}
	}
	return 0, false
}

// InlayHintAtPoint returns the source byte offset of the rendered hint
// at a UI logical point. It reports false in the gutter, outside the
// viewport, and over areas where no hint is drawn.
func (w *ViewW) InlayHintAtPoint(p geom.Point) (int, bool) {
	if !w.hasGeom() || w.vs.buf != w.st.Model.Buffer() || !w.vs.text.Contains(p) ||
		w.vs.stickyRect.Contains(p) || w.inlayLayout() == nil {
		return 0, false
	}
	return w.inlayAt(w.st.Snapshot(), p)
}
