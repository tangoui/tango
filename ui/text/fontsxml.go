package text

// Android system font configuration (docs/specs/mobile-canvas.md §4.2).
//
// Android describes its fonts in XML: /system/etc/font_fallback.xml
// (Android 15+), /system/etc/fonts.xml before it, plus product additions in
// /product/etc/fonts_customization.xml. A <familyset> lists <family>
// elements; a family with a name ("sans-serif", "monospace") is a named
// family, families without one form the fallback chain in document order,
// each tagged with the languages or scripts it serves (lang="zh-Hans",
// lang="und-Arab"). A <font> names its file as text content and may carry
// weight, style, a collection index and variation <axis> children:
//
//	<family lang="ja">
//	    <font weight="400" style="normal" index="0">Collection.ttc
//	        <axis tag="wght" stylevalue="400"/>
//	    </font>
//	</family>
//
// This file parses that format and plans Tango's chains from it; it has no
// build tag so it is tested on every OS. sys_android.go reads the files.

import (
	"encoding/xml"
	"io"
	"os"
	"path"
	"strconv"
	"strings"
	"sync"

	"github.com/go-text/typesetting/font"
	ot "github.com/go-text/typesetting/font/opentype"
)

// afAxis is one <axis tag=... stylevalue=...>.
type afAxis struct {
	tag   string
	value float32
}

// afFont is one <font>.
type afFont struct {
	path          string // file joined with the configuration's font directory
	index         int    // face index inside a collection (.ttc)
	weight        int    // 0: not given (400)
	italic        bool
	axes          []afAxis
	fallbackFor   string // "serif": only a fallback for that named family
	supportedAxes string // Android 15+: "wght", "wght,ital": a variable font to instantiate
	postScript    string
}

// afFamily is one <family>.
type afFamily struct {
	name    string
	langs   []string
	variant string // "compact", "elegant" or ""
	ignore  bool
	fonts   []afFont
}

// afAlias is one <alias name=... to=... weight=...>.
type afAlias struct {
	name, to string
	weight   int
}

// afConfig is a parsed font configuration.
type afConfig struct {
	families []afFamily
	aliases  []afAlias
}

// parseFontsXML parses a fonts.xml-style document (a <familyset>, or the
// <fonts-modification> of a customization file, whose new named families
// are kept). dir is the directory font file names are relative to.
func parseFontsXML(r io.Reader, dir string) (*afConfig, error) {
	d := xml.NewDecoder(r)
	d.Strict = false
	cfg := &afConfig{}
	var fam *afFamily
	var fnt *afFont
	var text strings.Builder
	for {
		tok, err := d.Token()
		if err == io.EOF {
			return cfg, nil
		}
		if err != nil {
			return nil, err
		}
		switch t := tok.(type) {
		case xml.StartElement:
			attr := func(name string) string {
				for _, a := range t.Attr {
					if a.Name.Local == name {
						return strings.TrimSpace(a.Value)
					}
				}
				return ""
			}
			switch t.Name.Local {
			case "family":
				if ct := attr("customizationType"); ct != "" && ct != "new-named-family" {
					fam = &afFamily{ignore: true} // a modification Tango does not understand
					continue
				}
				fam = &afFamily{
					name:    attr("name"),
					langs:   splitLangs(attr("lang")),
					variant: attr("variant"),
					ignore:  attr("ignore") == "true" || attr("ignore") == "1",
				}
			case "font":
				if fam == nil {
					continue
				}
				fnt = &afFont{
					index:         atoi(attr("index")),
					weight:        atoi(attr("weight")),
					italic:        attr("style") == "italic",
					fallbackFor:   attr("fallbackFor"),
					supportedAxes: attr("supportedAxes"),
					postScript:    attr("postScriptName"),
				}
				text.Reset()
			case "axis":
				if fnt != nil {
					if v, err := strconv.ParseFloat(attr("stylevalue"), 32); err == nil && len(attr("tag")) == 4 {
						fnt.axes = append(fnt.axes, afAxis{tag: attr("tag"), value: float32(v)})
					}
				}
			case "alias":
				if n, to := attr("name"), attr("to"); n != "" && to != "" {
					cfg.aliases = append(cfg.aliases, afAlias{name: n, to: to, weight: atoi(attr("weight"))})
				}
			}
		case xml.CharData:
			if fnt != nil {
				text.Write(t)
			}
		case xml.EndElement:
			switch t.Name.Local {
			case "font":
				if fnt != nil && fam != nil {
					if file := strings.TrimSpace(text.String()); file != "" {
						fnt.path = path.Join(dir, file) // a device path: always /
						fam.fonts = append(fam.fonts, *fnt)
					}
				}
				fnt = nil
			case "family":
				if fam != nil && !fam.ignore && len(fam.fonts) > 0 {
					cfg.families = append(cfg.families, *fam)
				}
				fam = nil
			}
		}
	}
}

func splitLangs(s string) []string {
	return strings.FieldsFunc(s, func(r rune) bool { return r == ',' || r == ' ' })
}

func atoi(s string) int {
	n, _ := strconv.Atoi(s)
	return n
}

// merge adds the named families and aliases of a customization file.
// A family whose name exists already replaces it.
func (c *afConfig) merge(o *afConfig) {
	if o == nil {
		return
	}
	for _, f := range o.families {
		if f.name == "" {
			continue
		}
		replaced := false
		for i := range c.families {
			if c.families[i].name == f.name {
				c.families[i], replaced = f, true
				break
			}
		}
		if !replaced {
			c.families = append(c.families, f)
		}
	}
	c.aliases = append(c.aliases, o.aliases...)
}

// named returns the named family name, following aliases (an alias that
// also fixes a weight still resolves to the whole family).
func (c *afConfig) named(name string) *afFamily {
	for range 8 { // aliases may chain; bound the walk
		for i := range c.families {
			if c.families[i].name == name {
				return &c.families[i]
			}
		}
		next := ""
		for _, a := range c.aliases {
			if a.name == name {
				next = a.to
				break
			}
		}
		if next == "" {
			return nil
		}
		name = next
	}
	return nil
}

// afFace is one face to load: a file, a collection index, the style it is
// used for and the variation coordinates to apply.
type afFace struct {
	path   string
	index  int
	weight Weight
	italic bool
	vars   []font.Variation
}

// afPlan is one family of a chain.
type afPlan struct {
	name  string
	cjk   bool // the CJK family for the current language: preferred for CJK text
	emoji bool // a symbol or emoji family: preferred for emoji-range runes
	faces []afFace
}

// afWeights are the weights a family keeps: the ones the ui package uses.
// Faces outside the range only stay when a family has nothing inside it.
const afMinWeight, afMaxWeight = 300, 700

// plan builds the sans-serif and monospace chains: the default family
// ("sans-serif", else the first named family), then every fallback family
// in document order. The CJK family of lang (zh-Hans, zh-Hant, ja, ko) is
// moved in front of the other CJK families and marked, so CJK text uses
// the right regional glyph forms. Serif-only fallbacks (fallbackFor) and
// ignored families are left out.
//
// Symbol and emoji families (und-Zsym, und-Zsye) are marked so emoji-range
// runes try them first, symbols before color emoji. Color bitmap emoji
// (CBDT) have no outlines for the pure-Go rasterizer and draw nothing, but
// claiming those runes early spares a walk through, and a load of, every
// other fallback family.
func (c *afConfig) plan(lang string) (sans, mono []afPlan) {
	def := c.named("sans-serif")
	if def == nil {
		for i := range c.families {
			if c.families[i].name != "" {
				def = &c.families[i]
				break
			}
		}
	}
	if def != nil {
		if p, ok := planFamily(def, def.name); ok {
			sans = append(sans, p)
		}
	}
	want := cjkLang(lang)
	cjkAt := -1   // index in sans of the first CJK family
	emojiAt := -1 // index in sans of the first emoji family
	for i := range c.families {
		f := &c.families[i]
		if f.name != "" {
			continue
		}
		p, ok := planFamily(f, "")
		if !ok {
			continue
		}
		isCJK, sym, emoji := false, false, false
		for _, l := range f.langs {
			if cjkLang(l) != "" {
				isCJK = true
			}
			sym = sym || l == "und-Zsym"
			emoji = emoji || l == "und-Zsye"
		}
		if sym || emoji {
			p.emoji = true
			if emojiAt < 0 {
				emojiAt = len(sans)
			}
			if sym {
				// Symbols (drawable outlines) before color emoji.
				sans = append(sans[:emojiAt], append([]afPlan{p}, sans[emojiAt:]...)...)
				if cjkAt >= emojiAt {
					cjkAt++
				}
				emojiAt++
				continue
			}
			sans = append(sans, p)
			continue
		}
		if isCJK && cjkAt < 0 {
			cjkAt = len(sans)
		}
		if isCJK && want != "" && familyServes(f, want) && !anyCJK(sans) {
			p.cjk = true
			sans = append(sans[:cjkAt], append([]afPlan{p}, sans[cjkAt:]...)...)
			if emojiAt >= cjkAt {
				emojiAt++
			}
			continue
		}
		sans = append(sans, p)
	}
	if m := c.named("monospace"); m != nil {
		if p, ok := planFamily(m, "monospace"); ok {
			mono = append(mono, p)
		}
	}
	return sans, mono
}

func anyCJK(ps []afPlan) bool {
	for _, p := range ps {
		if p.cjk {
			return true
		}
	}
	return false
}

// cjkLang returns the CJK family key of a BCP 47 tag ("zh-Hans",
// "zh-Hant", "ja", "ko"), or "" for other languages.
func cjkLang(tag string) string {
	parts := strings.Split(strings.ReplaceAll(tag, "_", "-"), "-")
	switch strings.ToLower(parts[0]) {
	case "ja":
		return "ja"
	case "ko":
		return "ko"
	case "zh":
		for _, p := range parts[1:] {
			switch strings.ToLower(p) {
			case "hant", "tw", "hk", "mo", "bopo":
				return "zh-Hant"
			case "hans", "cn", "sg":
				return "zh-Hans"
			}
		}
		return "zh-Hans"
	}
	return ""
}

func familyServes(f *afFamily, key string) bool {
	for _, l := range f.langs {
		if cjkLang(l) == key {
			return true
		}
	}
	return false
}

// planFamily lists the faces of f to load. ok is false when nothing is
// left (all serif-only).
func planFamily(f *afFamily, name string) (afPlan, bool) {
	var fonts []afFont
	for _, fn := range f.fonts {
		if fn.fallbackFor != "" && fn.fallbackFor != "sans-serif" {
			continue
		}
		fonts = append(fonts, fn)
	}
	if len(fonts) == 0 {
		return afPlan{}, false
	}
	if name == "" {
		name = fonts[0].postScript
		if name == "" {
			name = strings.TrimSuffix(path.Base(fonts[0].path), path.Ext(fonts[0].path))
		}
		if len(f.langs) > 0 {
			name += " (" + strings.Join(f.langs, ",") + ")"
		}
	}
	p := afPlan{name: name}
	type key struct {
		w      Weight
		italic bool
	}
	seen := map[key]bool{}
	add := func(fn afFont, w Weight, italic bool, vars []font.Variation) {
		k := key{w, italic}
		if seen[k] {
			return
		}
		seen[k] = true
		p.faces = append(p.faces, afFace{path: fn.path, index: fn.index, weight: w, italic: italic, vars: vars})
	}
	inRange := false
	for _, fn := range fonts {
		if w := fontWeight(fn); w >= afMinWeight && w <= afMaxWeight {
			inRange = true
		}
	}
	for _, fn := range fonts {
		w := fontWeight(fn)
		if inRange && (w < afMinWeight || w > afMaxWeight) {
			continue
		}
		axes := strings.Split(fn.supportedAxes, ",")
		hasAxis := func(tag string) bool {
			for _, a := range axes {
				if strings.TrimSpace(a) == tag {
					return true
				}
			}
			return false
		}
		if hasAxis("wght") && len(fonts) == 1 && len(fn.axes) == 0 && fn.weight == 0 {
			// One variable font without a weight standing for the whole family (Android 15
			// font_fallback.xml): instantiate the weights the ui uses, and
			// the italics when the font has an ital axis.
			for _, it := range []bool{false, true} {
				if it && !hasAxis("ital") && !fn.italic {
					continue
				}
				for _, w := range []Weight{Light, Regular, Medium, Semibold, Bold} {
					vars := []font.Variation{{Tag: ot.MustNewTag("wght"), Value: float32(w)}}
					if hasAxis("ital") {
						v := float32(0)
						if it {
							v = 1
						}
						vars = append(vars, font.Variation{Tag: ot.MustNewTag("ital"), Value: v})
					}
					add(fn, w, it || fn.italic, vars)
				}
			}
			continue
		}
		var vars []font.Variation
		hasW := false
		for _, a := range fn.axes {
			vars = append(vars, font.Variation{Tag: ot.MustNewTag(a.tag), Value: a.value})
			hasW = hasW || a.tag == "wght"
		}
		if !hasW && hasAxis("wght") {
			vars = append(vars, font.Variation{Tag: ot.MustNewTag("wght"), Value: float32(w)})
		}
		if fn.italic && hasAxis("ital") {
			vars = append(vars, font.Variation{Tag: ot.MustNewTag("ital"), Value: 1})
		}
		add(fn, w, fn.italic, vars)
	}
	return p, len(p.faces) > 0
}

func fontWeight(fn afFont) Weight {
	if fn.weight > 0 {
		return Weight(fn.weight)
	}
	return Regular
}

// afFiles shares opened and parsed fonts between faces and families: the
// faces of one variable font (one per weight) and the four CJK families
// using faces of one collection. Files stay open: go-text reads tables
// lazily with ReadAt.
var afFiles struct {
	sync.Mutex
	loaders map[string][]*ot.Loader  // nil entry: the file failed
	fonts   map[afFontKey]*font.Font // nil entry: the face failed
	files   []*os.File               // open behind loaders
}

// afCloseFiles closes the font files and forgets what was parsed from them.
// For tests: Windows removes a temporary font directory only once its files
// are closed.
func afCloseFiles() {
	afFiles.Lock()
	defer afFiles.Unlock()
	for _, f := range afFiles.files {
		f.Close()
	}
	afFiles.files, afFiles.loaders, afFiles.fonts = nil, nil, nil
}

type afFontKey struct {
	path  string
	index int
}

// afParsed returns the parsed face index of the font file path, parsing
// it once. afFiles is locked.
func afParsed(path string, index int) *font.Font {
	k := afFontKey{path, index}
	if f, ok := afFiles.fonts[k]; ok {
		return f
	}
	if afFiles.fonts == nil {
		afFiles.fonts = map[afFontKey]*font.Font{}
		afFiles.loaders = map[string][]*ot.Loader{}
	}
	lds, ok := afFiles.loaders[path]
	if !ok {
		if f, err := os.Open(path); err == nil {
			if lds, err = ot.NewLoaders(f); err != nil || len(lds) == 0 {
				f.Close()
				lds = nil
			} else {
				afFiles.files = append(afFiles.files, f)
			}
		}
		afFiles.loaders[path] = lds
	}
	var fn *font.Font
	if index >= 0 && index < len(lds) {
		fn, _ = font.NewFont(lds[index])
	}
	afFiles.fonts[k] = fn
	return fn
}

// family makes the lazily loaded family of p.
func (p afPlan) family() *family {
	fam := &family{name: p.name}
	switch {
	case p.cjk:
		fam.class = classCJK
	case p.emoji:
		fam.class = classEmoji
	}
	faces := p.faces
	fam.load = func() []*Face {
		afFiles.Lock()
		defer afFiles.Unlock()
		var out []*Face
		for _, fp := range faces {
			parsed := afParsed(fp.path, fp.index)
			if parsed == nil {
				continue
			}
			// Every face gets its own *font.Font, a shallow copy sharing
			// the parsed (read-only) tables: go-text's shaper caches per
			// font, so faces sharing one would shape with the variation
			// coordinates of whichever ran first.
			fn := new(font.Font)
			*fn = *parsed
			fc := font.NewFace(fn)
			if len(fp.vars) > 0 {
				fc.SetVariations(fp.vars)
			}
			f := newFace(p.name, fp.weight, fc)
			f.Italic = fp.italic
			out = append(out, f)
		}
		return out
	}
	return fam
}

// families makes the families of a chain.
func afFamilies(ps []afPlan) []*family {
	out := make([]*family, 0, len(ps))
	for _, p := range ps {
		out = append(out, p.family())
	}
	return out
}
