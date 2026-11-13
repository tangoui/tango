package text

import (
	"bytes"
	"encoding/binary"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"

	"golang.org/x/image/font/gofont/gobold"
	"golang.org/x/image/font/gofont/goitalic"
	"golang.org/x/image/font/gofont/gomono"
	"golang.org/x/image/font/gofont/goregular"
)

func loadTestConfig(t *testing.T, dir string) *afConfig {
	t.Helper()
	f, err := os.Open("testdata/fonts.xml")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	cfg, err := parseFontsXML(f, dir)
	if err != nil {
		t.Fatal(err)
	}
	return cfg
}

func TestParseFontsXML(t *testing.T) {
	cfg := loadTestConfig(t, "/fonts")
	var names []string
	for _, f := range cfg.families {
		n := f.name
		if n == "" {
			n = strings.Join(f.langs, "+")
		}
		names = append(names, n)
	}
	want := []string{"sans-serif", "serif", "monospace", "und-Arab", "zh-Hans", "zh-Hant+zh-Bopo", "ja", "ko", "und-Zsye", "und-Zsym"}
	if !slices.Equal(names, want) {
		t.Fatalf("families = %q, want %q (the ignored Thai family dropped)", names, want)
	}
	sans := cfg.families[0]
	if len(sans.fonts) != 4 {
		t.Fatalf("sans-serif has %d fonts, want 4", len(sans.fonts))
	}
	if f := sans.fonts[1]; f.path != "/fonts/Sans.ttf" || f.weight != 400 || f.italic || len(f.axes) != 1 || f.axes[0] != (afAxis{"wght", 400}) {
		t.Errorf("sans-serif font 1 = %+v", f)
	}
	if f := sans.fonts[3]; !f.italic || f.path != "/fonts/SansItalic.ttf" {
		t.Errorf("italic font = %+v", f)
	}
	ja := cfg.families[6]
	if f := ja.fonts[0]; f.path != "/fonts/Collection.ttc" || f.index != 1 || f.postScript != "TestCJK-JP" {
		t.Errorf("ja font (file name on its own lines) = %+v", f)
	}
	if ar := cfg.families[3]; ar.variant != "elegant" || len(ar.fonts) != 2 || ar.fonts[1].fallbackFor != "serif" {
		t.Errorf("Arabic family = %+v", ar)
	}
	if len(cfg.aliases) != 3 || cfg.aliases[0] != (afAlias{"sans-serif-medium", "sans-serif", 500}) {
		t.Errorf("aliases = %+v", cfg.aliases)
	}
}

func TestFontsXMLAliases(t *testing.T) {
	cfg := loadTestConfig(t, "/fonts")
	for name, want := range map[string]string{"helvetica": "sans-serif", "sans-serif-medium": "sans-serif", "courier": "monospace", "serif": "serif"} {
		if f := cfg.named(name); f == nil || f.name != want {
			t.Errorf("named(%q) = %v, want %q", name, f, want)
		}
	}
	if f := cfg.named("fantasy"); f != nil {
		t.Errorf("named(fantasy) = %+v, want nil", f)
	}
}

func planNames(ps []afPlan) []string {
	var out []string
	for _, p := range ps {
		n := p.name
		if p.cjk {
			n += "*"
		}
		if p.emoji {
			n += "!"
		}
		out = append(out, n)
	}
	return out
}

func TestFontsXMLPlanOrder(t *testing.T) {
	cfg := loadTestConfig(t, "/fonts")
	for _, tc := range []struct {
		lang string
		want []string
	}{
		{"zh-Hans", []string{"sans-serif", "Arabic (und-Arab)", "TestCJK-SC (zh-Hans)*", "TestCJK-TC (zh-Hant,zh-Bopo)", "TestCJK-JP (ja)", "TestCJK-KR (ko)", "Symbols (und-Zsym)!", "TestColorEmoji (und-Zsye)!"}},
		{"zh-CN", []string{"sans-serif", "Arabic (und-Arab)", "TestCJK-SC (zh-Hans)*", "TestCJK-TC (zh-Hant,zh-Bopo)", "TestCJK-JP (ja)", "TestCJK-KR (ko)", "Symbols (und-Zsym)!", "TestColorEmoji (und-Zsye)!"}},
		{"zh-TW", []string{"sans-serif", "Arabic (und-Arab)", "TestCJK-TC (zh-Hant,zh-Bopo)*", "TestCJK-SC (zh-Hans)", "TestCJK-JP (ja)", "TestCJK-KR (ko)", "Symbols (und-Zsym)!", "TestColorEmoji (und-Zsye)!"}},
		{"ja-JP", []string{"sans-serif", "Arabic (und-Arab)", "TestCJK-JP (ja)*", "TestCJK-SC (zh-Hans)", "TestCJK-TC (zh-Hant,zh-Bopo)", "TestCJK-KR (ko)", "Symbols (und-Zsym)!", "TestColorEmoji (und-Zsye)!"}},
		{"ko", []string{"sans-serif", "Arabic (und-Arab)", "TestCJK-KR (ko)*", "TestCJK-SC (zh-Hans)", "TestCJK-TC (zh-Hant,zh-Bopo)", "TestCJK-JP (ja)", "Symbols (und-Zsym)!", "TestColorEmoji (und-Zsye)!"}},
		{"en-US", []string{"sans-serif", "Arabic (und-Arab)", "TestCJK-SC (zh-Hans)", "TestCJK-TC (zh-Hant,zh-Bopo)", "TestCJK-JP (ja)", "TestCJK-KR (ko)", "Symbols (und-Zsym)!", "TestColorEmoji (und-Zsye)!"}},
	} {
		sans, mono := cfg.plan(tc.lang)
		if got := planNames(sans); !slices.Equal(got, tc.want) {
			t.Errorf("plan(%q) sans = %q\nwant %q", tc.lang, got, tc.want)
		}
		if got := planNames(mono); !slices.Equal(got, []string{"monospace"}) {
			t.Errorf("plan(%q) mono = %q", tc.lang, got)
		}
	}
}

func TestFontsXMLPlanFaces(t *testing.T) {
	cfg := loadTestConfig(t, "/fonts")
	sans, _ := cfg.plan("zh-Hans")
	type face struct {
		file   string
		w      Weight
		italic bool
		vars   string
	}
	got := func(p afPlan) []face {
		var out []face
		for _, f := range p.faces {
			var vs []string
			for _, v := range f.vars {
				vs = append(vs, v.Tag.String()+"="+strconv.FormatFloat(float64(v.Value), 'g', -1, 32))
			}
			out = append(out, face{filepath.Base(f.path), f.weight, f.italic, strings.Join(vs, ",")})
		}
		return out
	}
	// Weight 100 is outside the kept range; the others stay.
	want := []face{{"Sans.ttf", 400, false, "wght=400"}, {"SansBold.ttf", 700, false, ""}, {"SansItalic.ttf", 400, true, ""}}
	if g := got(sans[0]); !slices.Equal(g, want) {
		t.Errorf("sans-serif faces = %+v, want %+v", g, want)
	}
	// The serif-only Arabic font is not part of the sans-serif chain.
	if g := got(sans[1]); len(g) != 1 || g[0].file != "Arabic.ttf" {
		t.Errorf("Arabic faces = %+v", g)
	}
	if cjk := sans[2].faces; len(cjk) != 1 || cjk[0].index != 0 || filepath.Base(cjk[0].path) != "Collection.ttc" {
		t.Errorf("CJK SC faces = %+v", cjk)
	}
	if cjk := sans[3].faces; len(cjk) != 1 || cjk[0].index != 1 {
		t.Errorf("CJK TC faces = %+v", cjk)
	}
}

const variableFamilyXML = `<familyset>
  <family name="sans-serif">
    <font supportedAxes="wght,ital">Flex.ttf</font>
  </family>
  <family lang="und-Latn">
    <font weight="500" supportedAxes="wght">Fallback-VF.ttf</font>
  </family>
</familyset>`

func TestFontsXMLSupportedAxes(t *testing.T) {
	cfg, err := parseFontsXML(strings.NewReader(variableFamilyXML), "/f")
	if err != nil {
		t.Fatal(err)
	}
	sans, _ := cfg.plan("en")
	if len(sans) != 2 {
		t.Fatalf("plan = %q", planNames(sans))
	}
	faces := sans[0].faces
	if len(faces) != 10 {
		t.Fatalf("variable family: %d faces, want 5 weights x 2 slants", len(faces))
	}
	for _, f := range faces {
		if len(f.vars) != 2 || f.vars[0].Tag.String() != "wght" || f.vars[0].Value != float32(f.weight) || f.vars[1].Tag.String() != "ital" {
			t.Errorf("face %+v: vars %+v", f, f.vars)
		}
		if (f.vars[1].Value == 1) != f.italic {
			t.Errorf("face %+v: ital %v", f, f.vars[1].Value)
		}
	}
	// A weighted entry of a variable font gets its weight as the wght axis.
	if fb := sans[1].faces; len(fb) != 1 || fb[0].weight != 500 || len(fb[0].vars) != 1 || fb[0].vars[0].Value != 500 {
		t.Errorf("fallback faces = %+v", fb)
	}
}

const customizationXML = `<fonts-modification version="1">
  <family customizationType="new-named-family" name="google-sans">
    <font weight="400" style="normal">GoogleSans-Regular.ttf</font>
  </family>
  <family customizationType="new-named-family" name="monospace">
    <font weight="400" style="normal">BetterMono.ttf</font>
  </family>
  <family customizationType="something-else" name="sans-serif">
    <font weight="400" style="normal">Nope.ttf</font>
  </family>
  <alias name="gs" to="google-sans" />
</fonts-modification>`

func TestFontsXMLCustomization(t *testing.T) {
	cfg := loadTestConfig(t, "/system/fonts")
	custom, err := parseFontsXML(strings.NewReader(customizationXML), "/product/fonts")
	if err != nil {
		t.Fatal(err)
	}
	cfg.merge(custom)
	if f := cfg.named("gs"); f == nil || f.fonts[0].path != "/product/fonts/GoogleSans-Regular.ttf" {
		t.Errorf("named(gs) = %+v", f)
	}
	if f := cfg.named("monospace"); f == nil || f.fonts[0].path != "/product/fonts/BetterMono.ttf" {
		t.Errorf("monospace not replaced: %+v", f)
	}
	if f := cfg.named("sans-serif"); f == nil || f.fonts[0].path != "/system/fonts/Sans.ttf" {
		t.Errorf("an unknown customization type changed sans-serif: %+v", f)
	}
}

// buildTTC makes a font collection of whole font files: each file is
// copied as is and its table offsets moved by where it lands.
func buildTTC(fonts ...[]byte) []byte {
	head := 12 + 4*len(fonts)
	var body bytes.Buffer
	offsets := make([]uint32, len(fonts))
	for i, f := range fonts {
		for (head+body.Len())%4 != 0 {
			body.WriteByte(0)
		}
		base := uint32(head + body.Len())
		offsets[i] = base
		c := slices.Clone(f)
		n := int(binary.BigEndian.Uint16(c[4:]))
		for k := range n {
			rec := c[12+16*k:]
			binary.BigEndian.PutUint32(rec[8:], binary.BigEndian.Uint32(rec[8:])+base)
		}
		body.Write(c)
	}
	var out bytes.Buffer
	out.WriteString("ttcf")
	binary.Write(&out, binary.BigEndian, uint32(0x00010000))
	binary.Write(&out, binary.BigEndian, uint32(len(fonts)))
	for _, o := range offsets {
		binary.Write(&out, binary.BigEndian, o)
	}
	out.Write(body.Bytes())
	return out.Bytes()
}

func TestFontsXMLLoadFamilies(t *testing.T) {
	dir := t.TempDir()
	files := map[string][]byte{
		"Sans.ttf": goregular.TTF, "SansBold.ttf": gobold.TTF, "SansItalic.ttf": goitalic.TTF,
		"Mono.ttf": gomono.TTF, "Arabic.ttf": goregular.TTF, "Symbols.ttf": goregular.TTF,
		"Collection.ttc": buildTTC(goregular.TTF, gobold.TTF),
	}
	for name, b := range files {
		if err := os.WriteFile(filepath.Join(dir, name), b, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	cfg := loadTestConfig(t, dir)
	sp, mp := cfg.plan("zh-Hant")
	sans, mono := afFamilies(sp), afFamilies(mp)

	def := sans[0]
	def.ensure()
	if def.failed || len(def.faces) != 3 {
		t.Fatalf("sans-serif: failed=%v, %d faces, want 3", def.failed, len(def.faces))
	}
	if f, synth := def.pick(Bold, false); f == nil || f.Weight != Bold || synth {
		t.Errorf("pick(Bold) = %+v", f)
	}
	if f, synth := def.pick(Regular, true); f == nil || !f.Italic || synth {
		t.Errorf("pick(italic) = %+v synth=%v", f, synth)
	}
	mono[0].ensure()
	if len(mono[0].faces) != 1 {
		t.Errorf("monospace: %d faces", len(mono[0].faces))
	}

	// zh-Hant: the TC family (collection index 1, the bold Go font) is the
	// preferred CJK family; SC uses index 0 (regular).
	tc, sc := sans[2], sans[3]
	if tc.class != classCJK || sc.class == classCJK {
		t.Fatalf("CJK classes: tc %v sc %v", tc.class, sc.class)
	}
	adv := func(fam *family) float32 {
		fam.ensure()
		if len(fam.faces) != 1 {
			t.Fatalf("%s: %d faces", fam.name, len(fam.faces))
		}
		f := fam.faces[0]
		gid, ok := f.face.NominalGlyph('m')
		if !ok {
			t.Fatalf("%s: no glyph for m", fam.name)
		}
		return f.face.HorizontalAdvance(gid)
	}
	regular, bold := adv(sc), adv(tc)
	if regular == bold {
		t.Errorf("collection faces 0 and 1 have the same advance (%v): index ignored", regular)
	}
	plainBold := func() float32 {
		fam := afPlan{name: "b", faces: []afFace{{path: filepath.Join(dir, "SansBold.ttf"), weight: Bold}}}.family()
		return adv(fam)
	}()
	if bold != plainBold {
		t.Errorf("collection index 1 advance %v, want the bold font's %v", bold, plainBold)
	}

	// A family whose files are all missing fails and is skipped by Families.
	missing := afPlan{name: "gone", faces: []afFace{{path: filepath.Join(dir, "nope.ttf"), weight: Regular}}}.family()
	missing.ensure()
	if !missing.failed {
		t.Error("missing family did not fail")
	}
}
