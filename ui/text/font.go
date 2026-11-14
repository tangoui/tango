// Package text shapes and lays out text for the Tango self-drawn UI.
//
// It uses go-text/typesetting (HarfBuzz port) for shaping and UAX #14 line
// breaking, which gives correct per-character breaking for Chinese and the
// basic kinsoku rules (no break before "，。！？）" or after "（「"). Fonts are
// resolved per rune through a fallback chain: fonts registered by the
// application with RegisterFont first, then platform fonts. By default the
// platform text engine finds and rasterizes them (CoreText: SF Pro +
// PingFang SC on macOS; DirectWrite: Segoe UI + Microsoft YaHei on Windows;
// fontconfig + FreeType: Noto / DejaVu + Noto Sans CJK / WenQuanYi on
// Linux), with its hinting, and per-rune platform fallback; see engine.go
// and UseSystemEngine. Without the engine, font files are parsed and
// rasterized in pure Go (SF + Hiragino on macOS, Segoe UI + Microsoft
// YaHei on Windows, Noto/DejaVu/WenQuanYi on Linux). No font files are
// bundled. On platforms with no discoverable fonts (js/wasm) text is
// measured with approximate metrics and not drawn until the app registers
// a font.
//
// API tiers: only the symbols listed in api/stable/ui_text.txt are covered
// by the compatibility promise of docs/release.md ("API 稳定性"); every
// other exported symbol is Experimental and may change in any 0.x
// release. The API reference on the documentation site marks the
// experimental symbols.
package text

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"sort"
	"strings"
	"sync"

	"github.com/go-text/typesetting/font"
	ot "github.com/go-text/typesetting/font/opentype"
	"github.com/tangoui/tango/ui/paint"
)

// Weight is a CSS font weight.
type Weight int

const (
	Light    Weight = 300
	Regular  Weight = 400
	Medium   Weight = 500
	Semibold Weight = 600
	Bold     Weight = 700
)

// Face is one loaded font face at one weight and slant. It implements
// paint.GlyphSource.
type Face struct {
	Family string
	Weight Weight
	Italic bool // a designed italic or oblique face
	face   *font.Face
	upem   float32
	asc    float32 // font units, positive
	desc   float32 // font units, positive

	mu    sync.Mutex
	paths map[uint32]*paint.Path // unit outline (font units, y down)

	// System text engine faces (engine.go); nil for pure-Go faces.
	ras      glyphEngine
	unitPath func(id uint32) *paint.Path // outline in font units when go-text has none
	noRaster map[rasterKey]bool          // keys the engine failed on (fall back forever)
}

func newFace(family string, w Weight, f *font.Face) *Face {
	fc := &Face{Family: family, Weight: w, face: f, upem: float32(f.Upem()), paths: map[uint32]*paint.Path{}}
	if ext, ok := f.FontHExtents(); ok {
		fc.asc = ext.Ascender
		fc.desc = -ext.Descender
	} else {
		fc.asc, fc.desc = fc.upem*0.8, fc.upem*0.2
	}
	return fc
}

// Metrics returns ascent and descent in px at size.
func (f *Face) Metrics(size float32) (ascent, descent float32) {
	return f.asc * size / f.upem, f.desc * size / f.upem
}

// HasRune reports whether the face maps r.
func (f *Face) HasRune(r rune) bool {
	_, ok := f.face.NominalGlyph(r)
	return ok
}

// GlyphPath implements paint.GlyphSource.
func (f *Face) GlyphPath(id uint32, size float32) *paint.Path {
	f.mu.Lock()
	p, ok := f.paths[id]
	if !ok {
		p = f.outline(id)
		f.paths[id] = p
	}
	f.mu.Unlock()
	if p == nil {
		return nil
	}
	return p.Transformed(size/f.upem, 0, 0)
}

func (f *Face) outline(id uint32) *paint.Path {
	if f.unitPath != nil {
		return f.unitPath(id)
	}
	o, ok := f.face.GlyphDataOutline(font.GID(id))
	if !ok || len(o.Segments) == 0 {
		return nil
	}
	p := paint.NewPath()
	open := false
	for _, s := range o.Segments {
		a := s.Args
		switch s.Op {
		case ot.SegmentOpMoveTo:
			if open {
				p.Close()
			}
			p.MoveTo(a[0].X, -a[0].Y)
			open = true
		case ot.SegmentOpLineTo:
			p.LineTo(a[0].X, -a[0].Y)
		case ot.SegmentOpQuadTo:
			p.QuadTo(a[0].X, -a[0].Y, a[1].X, -a[1].Y)
		case ot.SegmentOpCubeTo:
			p.CubeTo(a[0].X, -a[0].Y, a[1].X, -a[1].Y, a[2].X, -a[2].Y)
		}
	}
	if open {
		p.Close()
	}
	return p
}

// family is a named set of faces (one per weight).
type family struct {
	name   string
	class  runeClass // script this family is preferred for
	user   bool      // registered with RegisterFont
	faces  []*Face
	load   func() []*Face // lazy loader for system fonts
	loaded bool
	failed bool
}

func (fm *family) ensure() {
	if fm.loaded {
		return
	}
	fm.loaded = true
	if fm.load != nil {
		fm.faces = append(fm.faces, fm.load()...)
		if len(fm.faces) == 0 {
			fm.failed = true
		}
		sort.Slice(fm.faces, func(i, j int) bool { return fm.faces[i].Weight < fm.faces[j].Weight })
	}
}

// pick implements CSS font matching: faces of the requested slant win
// (italic falls back to upright, never the other way round), then the
// font-weight algorithm chooses among them. synth reports that italic was
// asked for but the family has no italic face, so the caller slants the
// upright outline.
func (fm *family) pick(w Weight, italic bool) (f *Face, synth bool) {
	fm.ensure()
	if italic {
		if f := pickWeight(fm.faces, w, true); f != nil {
			return f, false
		}
		f := pickWeight(fm.faces, w, false)
		return f, f != nil
	}
	if f := pickWeight(fm.faces, w, false); f != nil {
		return f, false
	}
	return pickWeight(fm.faces, w, true), false // italic-only family
}

// pickWeight is the CSS font-weight matching algorithm over the faces of
// one slant.
func pickWeight(faces []*Face, w Weight, italic bool) *Face {
	for _, f := range faces {
		if f.Weight == w && f.Italic == italic {
			return f
		}
	}
	var below, above *Face
	for _, f := range faces {
		if f.Italic != italic {
			continue
		}
		if f.Weight < w && (below == nil || f.Weight > below.Weight) {
			below = f
		}
		if f.Weight > w && (above == nil || f.Weight < above.Weight) {
			above = f
		}
	}
	if w <= 500 {
		if w >= 400 && above != nil && above.Weight <= 500 {
			return above
		}
		if below != nil {
			return below
		}
		return above
	}
	if above != nil {
		return above
	}
	return below
}

type registry struct {
	mu       sync.Mutex
	user     []*family // RegisterFont, highest priority
	system   []*family
	mono     []*family
	version  int
	disabled bool // tests: ignore system fonts

	engineOn  bool         // UseSystemEngine
	sysBuilt  bool         // system/mono built ...
	sysEngine bool         // ... for this engineOn
	engImpl   sysEngine    // the platform engine, opened once
	engOpened bool         //
	eng       *engineState // the engine chain in use; nil: pure-Go system fonts
}

var reg = &registry{engineOn: engineMode}

// RegisterFont adds a TTF/OTF/TTC font under family. Faces registered by the
// app take priority over system fonts; registering several families builds
// your own fallback chain (first registered wins). Variable fonts with a
// wght axis are instantiated at 300-700.
func RegisterFont(familyName string, data []byte) error {
	faces, err := parseFaces(familyName, bytes.NewReader(data), "")
	if err != nil {
		return err
	}
	if len(faces) == 0 {
		return errors.New("text: font has no usable faces")
	}
	// Lock order: layout holds mu and then takes reg.mu (engine fallback),
	// so reg.mu must be released before clearCache takes mu.
	reg.mu.Lock()
	found := false
	for _, fm := range reg.user {
		if fm.name == familyName {
			fm.faces = append(fm.faces, faces...)
			sort.Slice(fm.faces, func(i, j int) bool { return fm.faces[i].Weight < fm.faces[j].Weight })
			found = true
			break
		}
	}
	if !found {
		fm := &family{name: familyName, faces: faces, loaded: true, user: true}
		sort.Slice(fm.faces, func(i, j int) bool { return fm.faces[i].Weight < fm.faces[j].Weight })
		reg.user = append(reg.user, fm)
	}
	reg.version++
	reg.mu.Unlock()
	clearCache()
	return nil
}

// UseSystemFonts enables or disables platform font discovery (enabled by
// default). Disabling is useful for deterministic tests that register their
// own fonts.
func UseSystemFonts(on bool) {
	reg.mu.Lock()
	reg.disabled = !on
	reg.version++
	reg.mu.Unlock()
	clearCache()
}

// Families lists the fallback chain names in priority order (loading system
// fonts if needed); families that failed to load are omitted.
func Families() []string {
	chain := reg.chain(false)
	out := make([]string, 0, len(chain))
	for _, f := range chain {
		f.ensure()
		if !f.failed {
			out = append(out, f.name)
		}
	}
	return out
}

func (r *registry) chain(mono bool) []*family {
	r.mu.Lock()
	defer r.mu.Unlock()
	if !r.sysBuilt || r.sysEngine != r.engineOn {
		r.buildSystem()
	}
	out := make([]*family, 0, len(r.user)+len(r.system)+len(r.mono))
	if mono && !r.disabled {
		out = append(out, r.mono...)
	}
	out = append(out, r.user...)
	if !r.disabled {
		out = append(out, r.system...)
	}
	return out
}

// buildSystem (re)builds the system chains for the current engine setting.
// Each build creates new families and so new faces: a GlyphSource never
// changes rasterization path. r.mu is held.
func (r *registry) buildSystem() {
	r.sysBuilt, r.sysEngine = true, r.engineOn
	r.eng = nil
	if r.engineOn {
		if !r.engOpened {
			r.engOpened = true
			r.engImpl = openEngine()
		}
		if r.engImpl != nil {
			st := newEngineState(r.engImpl)
			if sans, mono := st.chains(); len(sans) > 0 {
				r.eng, r.system, r.mono = st, sans, mono
				return
			}
		}
	}
	r.system, r.mono = systemFamilies()
}

// engineFallback returns the engine state for per-rune fallback, or nil.
func (r *registry) engineFallback() *engineState {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.disabled {
		return nil
	}
	return r.eng
}

// sysFont describes a platform font candidate.
type sysFont struct {
	name     string
	paths    []string // first existing path wins
	families []string // keep only faces whose family is in this list (empty = all)
	extra    []string // further files of the same family (italics), all loaded
}

func (s sysFont) family() *family {
	return &family{name: s.name, load: func() []*Face {
		for _, p := range s.paths {
			if p == "" {
				continue
			}
			f, err := os.Open(p)
			if err != nil {
				continue
			}
			faces, err := parseFaces(s.name, f, strings.Join(s.families, "\x00"))
			if err != nil || len(faces) == 0 {
				f.Close()
				continue
			}
			// The file stays open: go-text reads tables lazily via ReadAt.
			for _, x := range s.extra {
				if xf, err := os.Open(x); err == nil {
					if more, err := parseFaces(s.name, xf, strings.Join(s.families, "\x00")); err == nil && len(more) > 0 {
						faces = append(faces, more...)
					} else {
						xf.Close()
					}
				}
			}
			return faces
		}
		return nil
	}}
}

func parseFaces(name string, r font.Resource, keep string) ([]*Face, error) {
	lds, err := ot.NewLoaders(r)
	if err != nil {
		return nil, fmt.Errorf("text: parse font %s: %w", name, err)
	}
	var keepSet map[string]bool
	if keep != "" {
		keepSet = map[string]bool{}
		for _, k := range strings.Split(keep, "\x00") {
			keepSet[k] = true
		}
	}
	var out []*Face
	type wk struct {
		w      Weight
		italic bool
	}
	seen := map[wk]bool{}
	for _, ld := range lds {
		d, _ := font.Describe(ld, nil)
		if keepSet != nil && !keepSet[d.Family] {
			continue
		}
		italic := d.Aspect.Style == font.StyleItalic // italic or oblique
		if d.Aspect.Style != font.StyleNormal && d.Aspect.Style != 0 && !italic {
			continue
		}
		fn, err := font.NewFont(ld)
		if err != nil {
			continue
		}
		probe := font.NewFace(fn)
		probe.SetVariations([]font.Variation{{Tag: ot.MustNewTag("wght"), Value: 400}})
		if len(probe.Coords()) > 0 {
			for _, w := range []Weight{Light, Regular, Medium, Semibold, Bold} {
				if seen[wk{w, italic}] {
					continue
				}
				// Each weight needs its own *font.Font: go-text's
				// HarfbuzzShaper caches its shaping font per *font.Font, so
				// faces sharing one would all shape (and measure) with the
				// variation coordinates of whichever was shaped first.
				fw, err := font.NewFont(ld)
				if err != nil {
					continue
				}
				fc := font.NewFace(fw)
				vars := []font.Variation{{Tag: ot.MustNewTag("wght"), Value: float32(w)}, {Tag: ot.MustNewTag("opsz"), Value: 16}}
				fc.SetVariations(vars)
				f := newFace(name, w, fc)
				f.Italic = italic
				out = append(out, f)
				seen[wk{w, italic}] = true
			}
			continue
		}
		w := Weight(d.Aspect.Weight)
		if w == 0 {
			w = Regular
		}
		w = Weight((int(w) + 50) / 100 * 100)
		if seen[wk{w, italic}] {
			continue
		}
		seen[wk{w, italic}] = true
		f := newFace(name, w, font.NewFace(fn))
		f.Italic = italic
		out = append(out, f)
	}
	return out, nil
}
