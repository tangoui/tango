//go:build darwin && !ios && !tango_gotext

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
// feature is untouched by that lookup, whatever surrounds it. An isolated
// single substitution is allowed when the run's script cannot change
// whether it applies and its output cannot feed another lookup (allows).
// The probe then measures what CoreText does with the rune. All other
// substitutions and positioning lookups covering the rune send its line
// to the general path.
//
// The language system CoreText picks for a language is not documented: the
// analysis takes the union over every candidate (otlLangCandidates) and
// each script's default language. For known scripts it also considers
// DFLT, and for scripts it cannot tell (Common or Inherited runes, scripts
// absent from the layout table) it considers every script of that table.
// AAT tables (morx, mort, kerx, kern) leave the face to the general path.

// otlOff lists the optional OpenType features CoreText does not apply by
// default in horizontal text. The monospace line path also turns off liga,
// clig and calt (Options.noLiga); the numeric path does not.
var otlOff = func() map[string]bool {
	m := map[string]bool{}
	for _, t := range strings.Fields(`aalt afrc c2pc c2sc case cpct cpsp cswh dlig dnom expt falt frac fwid
		halt hist hkna hlig hngl hojo hwid ital jalt jp04 jp78 jp83 jp90 lnum mgrk nalt nlck numr onum
		ordn ornm palt pcap pkna pnum pwid qwid ruby salt sinf smcp smpl subs sups swsh titl tnum trad
		twid unic valt vert vhal vkna vkrn vpal vrt2 vrtr zero`) {
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
	ok      bool // false: AAT tables or unreadable tables, general path only
	lookups []otlLookup
}

// otlLookup is a lookup reachable from a default feature: its resolved
// type, the first coverage of its subtables, and for every script of the
// face, a bit per language candidate telling whether the lookup applies.
type otlLookup struct {
	gpos    bool
	kind    uint16
	cov     []glyphRange
	masks   map[string]uint64
	scripts []string            // script tags of this lookup's layout table
	outputs map[uint16][]uint16 // all outputs of a single substitution, by input
	ligs    []otlLigature       // numeric analysis only
}

type otlLigature struct {
	first uint16
	rest  []uint16
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
	for i := range o.lookups {
		l := &o.lookups[i]
		if !l.covers(uint16(g)) {
			continue
		}
		any, same := l.applicability(sc)
		if !any {
			continue
		}
		if l.gpos || l.kind != 1 || !same || len(l.outputs[uint16(g)]) == 0 {
			return false
		}
		// The cmap and probe see only the endpoints. An intermediate glyph
		// can be in a contextual lookup's coverage, or in an unsupported
		// chain. Ignore lookup order and reject any such intersection.
		for _, out := range l.outputs[uint16(g)] {
			for j := range o.lookups {
				next := &o.lookups[j]
				if next.covers(out) {
					if applies, _ := next.applicability(sc); applies {
						return false
					}
				}
			}
		}
	}
	return true
}

// restrictLigatures removes numeric ligatures that cannot occur in the
// label alphabet, even after single substitutions. If any other
// substitution can change an input glyph, allows rejects it separately.
func (o *otlInfo) restrictLigatures(alphabet map[uint16]bool) {
	var outputs []uint16
	for g := range alphabet {
		for i := range o.lookups {
			outputs = append(outputs, o.lookups[i].outputs[g]...)
		}
	}
	for _, g := range outputs {
		alphabet[g] = true
	}
	for i := range o.lookups {
		l := &o.lookups[i]
		if l.kind == 4 {
			l.cov = l.cov[:0]
			for _, lig := range l.ligs {
				if !slices.ContainsFunc(lig.rest, func(g uint16) bool { return !alphabet[g] }) {
					l.cov = append(l.cov, glyphRange{lig.first, lig.first})
				}
			}
			l.cov = mergeRanges(l.cov)
		}
	}
}

// applicability reports whether the lookup might apply and whether its
// language mask is the same for all candidate scripts.
func (l *otlLookup) applicability(sc string) (bool, bool) {
	if sc != "" && slices.Contains(l.scripts, sc) {
		m := l.masks[sc]
		if sc != "DFLT" && slices.Contains(l.scripts, "DFLT") {
			d := l.masks["DFLT"]
			return m|d != 0, m == d
		}
		return m != 0, true
	}
	var any, first uint64
	same := true
	for i, s := range l.scripts {
		m := l.masks[s]
		any |= m
		if i == 0 {
			first = m
		} else if m != first {
			same = false
		}
	}
	return any != 0, same
}

// otlFor reads f's layout tables for a monospace line (ligatures off).
func otlFor(f *Face, lang string) *otlInfo {
	return readOTL(f, lang, false)
}

// For numeric labels, inspect GSUB with ligatures on. Positioning is
// handled separately by their pair-advance model (including legacy kern).
func readOTL(f *Face, lang string, numeric bool) *otlInfo {
	o := &otlInfo{}
	f.ready()
	if f.own == nil {
		return o
	}
	base := f.own.base
	defer runtime.KeepAlive(f)
	arr := ctFontCopyAvailableTables(base, 0)
	if arr == 0 {
		return o
	}
	defer cfRelease(arr)
	cands := otlLangCandidates(lang)
	for i := int64(0); i < cfArrayGetCount(arr); i++ {
		tag := uint32(cfArrayGetValueAtIndex(arr, i))
		switch tag {
		case tag4("morx"), tag4("mort"), tag4("kerx"):
			return &otlInfo{}
		case tag4("kern"):
			if !numeric {
				return &otlInfo{}
			}
		case tag4("GPOS"), tag4("GSUB"):
			gpos := tag == tag4("GPOS")
			if gpos && numeric {
				continue
			}
			d := ctFontCopyTable(base, tag, 0)
			if d == 0 {
				return &otlInfo{}
			}
			b := unsafe.Slice((*byte)(cfDataGetBytePtr(d)), cfDataGetLength(d))
			ls, ok := parseOTL(b, gpos, !numeric, cands)
			cfRelease(d)
			if !ok {
				return &otlInfo{}
			}
			o.lookups = append(o.lookups, ls.lookups...)
		}
	}
	o.ok = true
	return o
}

type otlTable struct {
	scripts []string
	lookups []otlLookup
}

// otlReader reads big-endian values with bounds checks and shares a work
// budget across all lookups in the table. Aliased offsets are charged on
// every traversal, before expanding records or allocating output.
type otlReader struct {
	b    []byte
	left int
	bad  bool
}

func (r *otlReader) take(n int) bool {
	if r.bad || n > r.left {
		r.bad = true
		return false
	}
	r.left -= n
	return true
}

// count charges for visiting n records and for any per-record storage.
func (r *otlReader) count(off int) int {
	n := r.u16(off)
	if !r.take(n) {
		return 0
	}
	return n
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
func parseOTL(b []byte, gpos, noLiga bool, cands []string) (otlTable, bool) {
	// Allow ordinary compact range encodings, but bound the total work
	// even for very large or heavily aliased layout tables.
	r := &otlReader{b: b, left: max(4096, 4*min(len(b), 1<<18))}
	var out otlTable
	if len(b) < 10 || r.u16(0) != 1 || r.u16(2) > 1 {
		return out, false
	}
	minor := r.u16(2)
	sl, fl, ll := r.u16(4), r.u16(6), r.u16(8)
	fv := 0
	if minor >= 1 {
		fv = r.u32(10)
	}
	if sl == 0 || fl == 0 || ll == 0 {
		return out, false
	}
	nf := r.count(fl)
	featTag := func(i int) string { return r.tag(fl + 2 + 6*i) }
	featLookups := func(off int) []int {
		n := r.count(off + 2)
		ids := make([]int, 0, n)
		for k := 0; k < n && !r.bad; k++ {
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
		for i, n := 0, r.count(sl); i < n && !r.bad; i++ {
			so := sl + r.u16(sl+6+6*i)
			for k, m := 0, r.count(so+2); k < m && !r.bad; k++ {
				set[r.tag(so+4+6*k)] = true
				if len(set) > 62 {
					return out, false
				}
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
		for k, n := 0, r.count(lo+4); k < n && !r.bad; k++ {
			fs = append(fs, r.u16(lo+6+2*k))
		}
		return fs
	}
	for i, n := 0, r.count(sl); i < n && !r.bad; i++ {
		st := r.tag(sl + 2 + 6*i)
		so := sl + r.u16(sl+6+6*i)
		out.scripts = append(out.scripts, st)
		def := -1
		if d := r.u16(so); d != 0 {
			def = so + d
		}
		sys := map[string]int{}
		for k, m := 0, r.count(so+2); k < m && !r.bad; k++ {
			sys[r.tag(so+4+6*k)] = so + r.u16(so+8+6*k)
		}
		eff := func(t string) int {
			if o, ok := sys[t]; ok {
				return o
			}
			return def
		}
		if !r.take(len(tags) + 1) {
			return out, false
		}
		for bit := 0; bit <= len(tags) && !r.bad; bit++ {
			lo := def
			if bit < len(tags) {
				lo = eff(tags[bit])
			}
			if lo < 0 {
				continue
			}
			req := r.u16(lo + 2)
			for _, fi := range langFeats(lo) {
				if fi >= nf {
					return out, false
				}
				tag := featTag(fi)
				if fi != req && (otlOff[tag] || noLiga && (tag == "liga" || tag == "clig" || tag == "calt")) {
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
	nl := r.count(ll)
	lookMasks := make([]map[string]uint64, nl)
	addLookups := func(fi int, ids []int) {
		for _, li := range ids {
			if li >= nl {
				r.bad = true
				return
			}
			if !r.take(len(featMasks[fi])) {
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
	for fi := 0; fi < nf && !r.bad; fi++ {
		if featMasks[fi] != nil {
			addLookups(fi, featLookups(featOff(fi)))
		}
	}
	if fv != 0 {
		n := r.u32(fv + 4)
		if !r.take(n) {
			return out, false
		}
		for k := 0; k < n && !r.bad; k++ {
			fts := r.u32(fv + 8 + 8*k + 4)
			if fts == 0 {
				continue
			}
			fts += fv
			for j, m := 0, r.count(fts+4); j < m && !r.bad; j++ {
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
		l := otlLookup{gpos: gpos, kind: uint16(kind), masks: lookMasks[li], scripts: out.scripts}
		ext := 7
		if gpos {
			ext = 9
		}
		for s, n := 0, r.count(lo+4); s < n && !r.bad; s++ {
			so := lo + r.u16(lo+6+2*s)
			k := kind
			if kind == ext {
				k = r.u16(so + 2)
				d := r.u32(so + 4)
				if r.u16(so) != 1 || k == ext || d == 0 || s != 0 && l.kind != uint16(k) {
					return out, false
				}
				so += d
				l.kind = uint16(k)
			}
			cov := otlFirstCoverage(r, so, k, gpos)
			if cov < 0 {
				return out, false
			}
			rs := readCoverage(r, cov)
			if !gpos && k == 1 {
				if l.outputs == nil {
					l.outputs = map[uint16][]uint16{}
				}
				readSingleOutputs(r, so, rs, l.outputs)
			} else if !gpos && !noLiga && k == 4 {
				l.ligs = append(l.ligs, readLigatures(r, so, rs)...)
			}
			l.cov = append(l.cov, rs...)
		}
		if r.bad {
			return out, false
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
	format := r.u16(so)
	covOff := so + 2
	switch {
	case k == context || k == chain:
		switch format {
		case 1, 2:
		case 3:
			in := so + 2
			covOff = so + 6
			if k == chain {
				in = so + 4 + 2*r.u16(so+2)
				covOff = in + 2
			}
			if r.u16(in) == 0 {
				return -1
			}
		default:
			return -1
		}
	case k == 1 || gpos && k == 2:
		if format != 1 && format != 2 {
			return -1
		}
	case k >= 2 && k <= 4 || gpos && (k == 5 || k == 6) || !gpos && k == 8:
		if format != 1 {
			return -1
		}
	default:
		return -1
	}
	if d := r.u16(covOff); d != 0 {
		return so + d
	}
	return -1
}

// readSingleOutputs reads both SingleSubst formats. Coverage iteration
// preserves index order (readCoverage validates the range indices).
func readSingleOutputs(r *otlReader, so int, rs []glyphRange, outputs map[uint16][]uint16) {
	format, value := r.u16(so), r.u16(so+4)
	if format != 1 && format != 2 {
		r.bad = true
		return
	}
	i := 0
	for _, g := range rs {
		if !r.take(int(g.hi) - int(g.lo) + 1) {
			return
		}
		for in := int(g.lo); in <= int(g.hi) && !r.bad; in++ {
			out := uint16(in + value) // format 1: signed delta modulo 65536
			if format == 2 {
				if i >= value {
					r.bad = true
					return
				}
				out = uint16(r.u16(so + 6 + 2*i))
			}
			outputs[uint16(in)] = append(outputs[uint16(in)], out)
			i++
		}
	}
	if format == 2 && i != value {
		r.bad = true
	}
}

// readLigatures reads only the inputs of each rule, in coverage order.
func readLigatures(r *otlReader, so int, rs []glyphRange) []otlLigature {
	if r.u16(so) != 1 {
		r.bad = true
		return nil
	}
	n, i := r.u16(so+4), 0
	var ligs []otlLigature
	for _, g := range rs {
		if !r.take(int(g.hi) - int(g.lo) + 1) {
			return nil
		}
		for in := int(g.lo); in <= int(g.hi) && !r.bad; in++ {
			if i >= n {
				r.bad = true
				return nil
			}
			set := so + r.u16(so+6+2*i)
			i++
			for k, count := 0, r.count(set); k < count && !r.bad; k++ {
				lo := set + r.u16(set+2+2*k)
				c := r.u16(lo + 2)
				if c < 2 || lo+2+2*c > len(r.b) || !r.take(c-1) {
					r.bad = true
					return nil
				}
				l := otlLigature{first: uint16(in), rest: make([]uint16, c-1)}
				for j := range l.rest {
					l.rest[j] = uint16(r.u16(lo + 4 + 2*j))
				}
				ligs = append(ligs, l)
			}
		}
	}
	if i != n {
		r.bad = true
	}
	return ligs
}

func readCoverage(r *otlReader, c int) []glyphRange {
	var out []glyphRange
	index := 0
	appendRange := func(lo, hi int) {
		if lo > hi || len(out) > 0 && lo <= int(out[len(out)-1].hi) {
			r.bad = true
			return
		}
		out = append(out, glyphRange{uint16(lo), uint16(hi)})
		index += hi - lo + 1
	}
	switch r.u16(c) {
	case 1:
		for i, n := 0, r.count(c+2); i < n && !r.bad; i++ {
			g := r.u16(c + 4 + 2*i)
			appendRange(g, g)
		}
	case 2:
		for i, n := 0, r.count(c+2); i < n && !r.bad; i++ {
			o := c + 4 + 6*i
			if r.u16(o+4) != index {
				r.bad = true
				break
			}
			appendRange(r.u16(o), r.u16(o+2))
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
