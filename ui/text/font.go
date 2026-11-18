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
	"errors"
	"slices"
	"sort"
	"sync"

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
	Family   string
	Weight   Weight
	Italic   bool // a designed italic or oblique face
	faceImpl      // the text engine's font (face_<engine>.go)

	mu    sync.Mutex
	paths map[uint32]*paint.Path // unit outline (font units, y down)

	// System text engine faces (engine.go); nil for pure-Go faces.
	ras      glyphEngine
	noRaster map[rasterKey]bool // keys the engine failed on (fall back forever)

	// Lazily built engine faces (newLazyEngineFace): a family registers all
	// its styles, but only the ones a layout picks parse their tables.
	once   sync.Once
	broken bool // build failed: the face cannot be used
}

// usable reports whether f is not a lazy face that failed to build.
func (f *Face) usable() bool {
	f.ready()
	return !f.broken
}

// Metrics returns ascent and descent in px at size.
func (f *Face) Metrics(size float32) (ascent, descent float32) {
	upem, asc, desc := f.metricsUnits()
	return asc * size / upem, desc * size / upem
}

// HasRune reports whether the face maps r.
func (f *Face) HasRune(r rune) bool { return f.hasRune(r) }

// GlyphPath implements paint.GlyphSource.
func (f *Face) GlyphPath(id uint32, size float32) *paint.Path {
	upem, _, _ := f.metricsUnits()
	f.mu.Lock()
	p, ok := f.paths[id]
	if !ok {
		p = f.unitOutline(id)
		f.paths[id] = p
	}
	f.mu.Unlock()
	if p == nil {
		return nil
	}
	return p.Transformed(size/upem, 0, 0)
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
	for {
		f, synth = fm.pickBuilt(w, italic)
		if f == nil || f.usable() {
			return f, synth
		}
		// A lazy face whose tables could not be read: drop it, pick again.
		fm.faces = slices.DeleteFunc(fm.faces, func(x *Face) bool { return x == f })
		if len(fm.faces) == 0 {
			fm.failed = true
		}
	}
}

func (fm *family) pickBuilt(w Weight, italic bool) (f *Face, synth bool) {
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
	faces, err := parseFaces(familyName, data)
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
