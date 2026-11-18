//go:build darwin && (ios || !tango_coretext)

package text

import (
	"runtime"
	"sync"

	"github.com/go-text/typesetting/font"
	ot "github.com/go-text/typesetting/font/opentype"
	"github.com/go-text/typesetting/font/opentype/tables"
	"github.com/tangoui/tango/ui/paint"
)

// go-text faces built from CoreText fonts: go-text shapes with the tables
// CoreText renders from (CTFontCopyTable), so glyph ids and advances agree.

// ctRasterImpl is the go-text part of ctRaster.
type ctRasterImpl struct {
	tables *ot.Loader // immutable shaping tables for fixed-instance specialization
}

func (r *ctRaster) shapingTables() *ot.Loader { return r.tables }

// ctSharedTables holds the tables of one font file: named instances of one
// variable font share them, parsed once, by the first instance built.
type ctSharedTables struct {
	once sync.Once
	fn   *font.Font
	ld   *ot.Loader
	axes []tables.VariationAxisRecord
}

// newCTLazyFaces returns the constructor of the lazy faces of family name
// (ctEngine.load): named instances of one variable font share its tables
// (ctSharedTables).
func newCTLazyFaces(name string) func(w Weight, italic bool, owner *ctFontOwner) *Face {
	shared := map[string]*ctSharedTables{}
	return func(w Weight, italic bool, owner *ctFontOwner) *Face {
		key := tableKey(owner.base)
		sh := shared[key]
		if sh == nil {
			sh = &ctSharedTables{}
			shared[key] = sh
		}
		return ctLazyFace(name, w, italic, owner, sh)
	}
}

// ctLazyFace registers a style of family name; its tables are read when a
// layout first uses it (newLazyEngineFace). owner holds the CTFont.
func ctLazyFace(name string, w Weight, italic bool, owner *ctFontOwner, sh *ctSharedTables) *Face {
	return newLazyEngineFace(name, w, italic, func() (*font.Face, glyphEngine, func(uint32) *paint.Path) {
		// The lazy closure owns the font until its raster takes over the
		// same owner. Raw uintptr handles alone do not keep ownership alive.
		defer runtime.KeepAlive(owner)
		f := owner.base
		sh.once.Do(func() {
			source := ctTables(f)
			ld, err := ot.NewLoader(source)
			if err != nil {
				if source.owner != nil {
					source.owner.release()
				}
				return
			}
			if fn, err := font.NewFont(ld); err == nil {
				sh.fn, sh.ld, sh.axes = fn, ld, fvarAxes(ld)
			} else {
				source.owner.release()
			}
		})
		if sh.fn == nil {
			owner.release()
			return nil, nil, nil
		}
		// Font tables are read-only. Each Face owns its coordinates and
		// glyph caches; runtime shapers key by Face and instance state.
		gt := font.NewFace(sh.fn)
		axes := sh.axes
		var vals map[uint32]float32
		if len(axes) > 0 {
			vals = ctVariation(f)
			if vals == nil {
				vals = map[uint32]float32{}
			}
			// Pin the optical size: CoreText would otherwise follow the
			// point size, drawing other designs than go-text measured.
			for _, a := range axes {
				if uint32(a.Tag) == opszTag {
					if _, ok := vals[opszTag]; !ok {
						vals[opszTag] = 16
					}
				}
			}
			if len(vals) == 0 {
				// keep f: the default instance
			} else if nf := withVariation(f, vals); nf != 0 {
				owner.base = nf
				cfRelease(f)
			}
			setDesignCoords(gt, axes, vals)
		}
		r := newCTRasterOwned(owner, vals)
		r.tables = sh.ld
		return gt, r, r.unitPath
	})
}
