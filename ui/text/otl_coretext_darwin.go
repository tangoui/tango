//go:build darwin && !ios && tango_coretext

package text

import (
	"runtime"
	"slices"
	"strings"
	"unicode"
	"unsafe"
)

// What the exact fast path (linefast_coretext_darwin.go) needs to know
// about a face's OpenType layout tables to set a rune by itself: whether
// any lookup CoreText applies by default could make the rune's glyphs
// depend on the runes around it. The tables are read once per face and
// language (otlFor) and kept with the fast path's memo.
//
// A lookup only ever starts at a glyph in its first coverage table (the
// glyph itself for single, multiple, alternate, ligature and positioning
// lookups, the first input glyph for contextual ones), so a rune whose
// glyph is in no such coverage of a lookup reachable from a default
// feature is untouched by that lookup, whatever surrounds it. Single and
// multiple substitutions change the glyph without looking at neighbours:
// they are allowed when the run's script cannot change whether they apply
// (otlInfo.allows); the probe then measures what CoreText does with the
// rune. Everything else covering the rune (ligatures, contextual lookups,
// any positioning, as kerning) sends its line to the general path.
//
// The language system CoreText picks for a language is not documented: the
// analysis takes the union over every candidate (otlLangCandidates) and
// the script's default, and for scripts it cannot tell (Common and
// Inherited runes, scripts the face has no table for) every script of the
// face. AAT tables (morx, kerx, kern) leave the face to the general path.

// otlOff lists the OpenType features CoreText does not apply by default in
// horizontal text, plus liga, clig and calt, which the fast path's lines
// turn off (Options.noLiga). Every other feature counts as applied.
var otlOff = func() map[string]bool {
	m := map[string]bool{}
	for _, t := range strings.Fields(`aalt afrc c2pc c2sc case cpct cpsp cswh dlig dnom expt falt frac fwid
		halt hist hkna hlig hngl hojo hwid ital jalt jp04 jp78 jp83 jp90 lnum mgrk nalt nlck numr onum
		ordn ornm palt pcap pkna pnum pwid qwid ruby salt sinf smcp smpl subs sups swsh titl tnum trad
		twid unic valt vert vhal vkna vkrn vpal vrt2 vrtr zero liga clig calt`) {
		m[t] = true
	}
	for i := 1; i <= 20; i++ {
		m["ss"+string(rune('0'+i/10))+string(rune('0'+i%10))] = true
	}
	for i := 1; i <= 99; i++ {
		m["cv"+string(rune('0'+i/10))+string(rune('0'+i%10))] = true
	}
	return m
}()

// otlLangCandidates returns the OpenType language system tags CoreText
// may pick for BCP 47 tag lang; nil means any of the face's.
func otlLangCandidates(lang string) []string {
	p := strings.ToLower(lang)
	if i := strings.IndexAny(p, "-_"); i >= 0 {
		p = p[:i]
	}
	switch p {
	case "zh":
		// Simplified unless a Traditional script or region says otherwise;
		// for those, every Traditional system (CoreText's choice between
		// them is not documented).
		l := strings.ToLower(lang)
		for _, s := range []string{"hant", "-tw", "_tw", "-hk", "_hk", "-mo", "_mo"} {
			if strings.Contains(l, s) {
				return []string{"ZHT ", "ZHH ", "ZHTM"}
			}
		}
		return []string{"ZHS "}
	case "en":
		return []string{"ENG "}
	case "ja":
		return []string{"JAN "}
	case "ko":
		return []string{"KOR "}
	}
	return nil
}

// otlScriptOf is the OpenType script tag of r when its script does not
// depend on the text around it; "" otherwise.
func otlScriptOf(r rune) string {
	switch {
	case r < 0x80:
		if 'a' <= r && r <= 'z' || 'A' <= r && r <= 'Z' {
			return "latn"
		}
		return ""
	case unicode.Is(unicode.Han, r):
		return "hani"
	case unicode.Is(unicode.Hiragana, r), unicode.Is(unicode.Katakana, r):
		return "kana"
	case unicode.Is(unicode.Hangul, r):
		return "hang"
	case unicode.Is(unicode.Latin, r):
		return "latn"
	case unicode.Is(unicode.Greek, r):
		return "grek"
	case unicode.Is(unicode.Cyrillic, r):
		return "cyrl"
	case unicode.Is(unicode.Bopomofo, r):
		return "bopo"
	}
	return ""
}

// otlInfo is the analysis of one face's GSUB and GPOS for one language.
type otlInfo struct {
	ok      bool     // false: AAT tables or unreadable tables, general path only
	scripts []string // script tags of the face (GSUB and GPOS)
	lookups []otlLookup
}

// otlLookup is a lookup reachable from a default feature: its resolved
// type, the first coverage of its subtables, and for every script of the
// face, a bit per language candidate telling whether the lookup applies.
type otlLookup struct {
	gpos  bool
	kind  uint16
	cov   []glyphRange
	masks map[string]uint64
}

type glyphRange struct{ lo, hi uint16 }

func (l *otlLookup) covers(g uint16) bool {
	lo, hi := 0, len(l.cov)
	for lo < hi {
		m := (lo + hi) / 2
		switch r := l.cov[m]; {
		case g < r.lo:
			hi = m
		case g > r.hi:
			lo = m + 1
		default:
			return true
		}
	}
	return false
}

// allows reports whether glyph g of a rune of script sc ("" unknown) can
// be set by itself: no lookup that may apply to it in that script could
// make it depend on its neighbours or on the script CoreText gives its run.
func (o *otlInfo) allows(g uint32, sc string) bool {
	if !o.ok || g > 0xffff {
		return false
	}
	scripts := o.scripts
	if sc != "" {
		for _, s := range o.scripts {
			if s == sc {
				scripts = []string{sc}
				break
			}
		}
	}
	for i := range o.lookups {
		l := &o.lookups[i]
		var any, first uint64
		same := true
		for k, s := range scripts {
			m := l.masks[s]
			any |= m
			if k == 0 {
				first = m
			} else if m != first {
				same = false
			}
		}
		if any == 0 || !l.covers(uint16(g)) {
			continue
		}
		if l.gpos || l.kind != 1 && l.kind != 2 || !same {
			return false
		}
	}
	return true
}

// otlFor reads f's layout tables for language lang.
func otlFor(f *Face, lang string) *otlInfo {
	o := &otlInfo{}
	f.ready()
	if f.own == nil {
		return o
	}
	base := f.own.base
	defer runtime.KeepAlive(f)
	if arr := ctFontCopyAvailableTables(base, 0); arr != 0 {
		for i := int64(0); i < cfArrayGetCount(arr); i++ {
			switch uint32(cfArrayGetValueAtIndex(arr, i)) {
			case tag4("morx"), tag4("mort"), tag4("kerx"), tag4("kern"):
				cfRelease(arr)
				return o
			}
		}
		cfRelease(arr)
	}
	cands := otlLangCandidates(lang)
	seen := map[string]bool{}
	for _, t := range []string{"GSUB", "GPOS"} {
		d := ctFontCopyTable(base, tag4(t), 0)
		if d == 0 {
			continue
		}
		b := append([]byte(nil), unsafe.Slice((*byte)(cfDataGetBytePtr(d)), cfDataGetLength(d))...)
		cfRelease(d)
		ls, ok := parseOTL(b, t == "GPOS", cands)
		if !ok {
			return &otlInfo{}
		}
		o.lookups = append(o.lookups, ls.lookups...)
		for _, s := range ls.scripts {
			if !seen[s] {
				seen[s] = true
				o.scripts = append(o.scripts, s)
			}
		}
	}
	// A lookup's masks name only the scripts of its own table: the other
	// table's scripts get no bits, which is what OpenType does there.
	o.ok = true
	return o
}

type otlTable struct {
	scripts []string
	lookups []otlLookup
}

// otlReader reads big-endian values with bounds checks; any read out of
// range sets bad.
type otlReader struct {
	b   []byte
	bad bool
}

func (r *otlReader) u16(o int) int {
	if o < 0 || o+2 > len(r.b) {
		r.bad = true
		return 0
	}
	return int(r.b[o])<<8 | int(r.b[o+1])
}

func (r *otlReader) u32(o int) int {
	return r.u16(o)<<16 | r.u16(o+2)
}

func (r *otlReader) tag(o int) string {
	if o < 0 || o+4 > len(r.b) {
		r.bad = true
		return "    "
	}
	return string(r.b[o : o+4])
}

// parseOTL reads a GSUB or GPOS table: the lookups reachable from default
// features (otlOff) of every script's candidate language systems, with
// their membership masks and first coverages.
func parseOTL(b []byte, gpos bool, cands []string) (otlTable, bool) {
	r := &otlReader{b: b}
	var out otlTable
	if len(b) < 10 {
		return out, false
	}
	minor := r.u16(2)
	sl, fl, ll := r.u16(4), r.u16(6), r.u16(8)
	fv := 0
	if minor >= 1 {
		fv = r.u32(10)
	}
	if sl == 0 || fl == 0 || ll == 0 {
		return out, !r.bad
	}
	nf := r.u16(fl)
	featTag := func(i int) string { return r.tag(fl + 2 + 6*i) }
	featLookups := func(off int) []int {
		n := r.u16(off + 2)
		ids := make([]int, 0, n)
		for k := 0; k < n; k++ {
			ids = append(ids, r.u16(off+4+2*k))
		}
		return ids
	}
	featOff := func(i int) int { return fl + r.u16(fl+6+6*i) }

	// The candidate language systems: the given tags, or every tag of the
	// table; bit len(tags) is "the script's default".
	tags := cands
	if tags == nil {
		set := map[string]bool{}
		for i, n := 0, r.u16(sl); i < n; i++ {
			so := sl + r.u16(sl+6+6*i)
			for k, m := 0, r.u16(so+2); k < m; k++ {
				set[r.tag(so+4+6*k)] = true
			}
		}
		for t := range set {
			tags = append(tags, t)
		}
	}
	if len(tags) > 62 {
		return out, false
	}
	// feature index -> per script mask.
	featMasks := make([]map[string]uint64, nf)
	langFeats := func(lo int) []int {
		if r.bad {
			return nil
		}
		var fs []int
		if req := r.u16(lo + 2); req != 0xffff {
			fs = append(fs, req)
		}
		for k, n := 0, r.u16(lo+4); k < n; k++ {
			fs = append(fs, r.u16(lo+6+2*k))
		}
		return fs
	}
	for i, n := 0, r.u16(sl); i < n; i++ {
		st := r.tag(sl + 2 + 6*i)
		so := sl + r.u16(sl+6+6*i)
		out.scripts = append(out.scripts, st)
		def := -1
		if d := r.u16(so); d != 0 {
			def = so + d
		}
		sys := map[string]int{}
		for k, m := 0, r.u16(so+2); k < m; k++ {
			sys[r.tag(so+4+6*k)] = so + r.u16(so+8+6*k)
		}
		eff := func(t string) int {
			if o, ok := sys[t]; ok {
				return o
			}
			return def
		}
		for bit := 0; bit <= len(tags); bit++ {
			lo := def
			if bit < len(tags) {
				lo = eff(tags[bit])
			}
			if lo < 0 {
				continue
			}
			for _, fi := range langFeats(lo) {
				if fi >= nf || otlOff[featTag(fi)] {
					continue
				}
				if featMasks[fi] == nil {
					featMasks[fi] = map[string]uint64{}
				}
				featMasks[fi][st] |= 1 << bit
			}
		}
	}
	// lookup index -> masks, from the features and their variations.
	nl := r.u16(ll)
	lookMasks := make([]map[string]uint64, nl)
	addLookups := func(fi int, ids []int) {
		for _, li := range ids {
			if li >= nl {
				r.bad = true
				return
			}
			if lookMasks[li] == nil {
				lookMasks[li] = map[string]uint64{}
			}
			for s, m := range featMasks[fi] {
				lookMasks[li][s] |= m
			}
		}
	}
	for fi := 0; fi < nf; fi++ {
		if featMasks[fi] != nil {
			addLookups(fi, featLookups(featOff(fi)))
		}
	}
	if fv != 0 {
		for k, n := 0, r.u32(fv+4); k < n && !r.bad; k++ {
			fts := r.u32(fv + 8 + 8*k + 4)
			if fts == 0 {
				continue
			}
			fts += fv
			for j, m := 0, r.u16(fts+4); j < m && !r.bad; j++ {
				fi := r.u16(fts + 6 + 6*j)
				alt := fts + r.u32(fts+6+6*j+2)
				if fi < nf && featMasks[fi] != nil {
					addLookups(fi, featLookups(alt)) // a Feature table
				}
			}
		}
	}
	for li := 0; li < nl && !r.bad; li++ {
		if lookMasks[li] == nil {
			continue
		}
		lo := ll + r.u16(ll+2+2*li)
		kind := r.u16(lo)
		l := otlLookup{gpos: gpos, kind: uint16(kind), masks: lookMasks[li]}
		ext := 7
		if gpos {
			ext = 9
		}
		for s, n := 0, r.u16(lo+4); s < n && !r.bad; s++ {
			so := lo + r.u16(lo+6+2*s)
			k := kind
			if kind == ext {
				k = r.u16(so + 2)
				so += r.u32(so + 4)
				l.kind = uint16(k)
			}
			cov := otlFirstCoverage(r, so, k, gpos)
			if cov < 0 {
				return out, false
			}
			l.cov = append(l.cov, readCoverage(r, cov)...)
		}
		l.cov = mergeRanges(l.cov)
		out.lookups = append(out.lookups, l)
	}
	return out, !r.bad
}

// otlFirstCoverage returns the offset of the coverage of the first glyph
// a subtable at so of lookup type k matches; -1 for an unknown type.
func otlFirstCoverage(r *otlReader, so, k int, gpos bool) int {
	context, chain := 5, 6
	if gpos {
		context, chain = 7, 8
	}
	switch {
	case k == context:
		if r.u16(so) == 3 {
			return so + r.u16(so+6)
		}
		return so + r.u16(so+2)
	case k == chain:
		if r.u16(so) == 3 {
			nb := r.u16(so + 2)
			in := so + 4 + 2*nb
			if r.u16(in) == 0 {
				return -1
			}
			return so + r.u16(in+2)
		}
		return so + r.u16(so+2)
	case k >= 1 && k <= 8:
		return so + r.u16(so+2)
	}
	return -1
}

func readCoverage(r *otlReader, c int) []glyphRange {
	var out []glyphRange
	switch r.u16(c) {
	case 1:
		for i, n := 0, r.u16(c+2); i < n && !r.bad; i++ {
			g := uint16(r.u16(c + 4 + 2*i))
			out = append(out, glyphRange{g, g})
		}
	case 2:
		for i, n := 0, r.u16(c+2); i < n && !r.bad; i++ {
			out = append(out, glyphRange{uint16(r.u16(c + 4 + 6*i)), uint16(r.u16(c + 6 + 6*i))})
		}
	default:
		r.bad = true
	}
	return out
}

func mergeRanges(rs []glyphRange) []glyphRange {
	if len(rs) < 2 {
		return rs
	}
	slices.SortFunc(rs, func(a, b glyphRange) int { return int(a.lo) - int(b.lo) })
	out := rs[:1]
	for _, g := range rs[1:] {
		last := &out[len(out)-1]
		if int(g.lo) <= int(last.hi)+1 {
			last.hi = max(last.hi, g.hi)
		} else {
			out = append(out, g)
		}
	}
	return out
}
