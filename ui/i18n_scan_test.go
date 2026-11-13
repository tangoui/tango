package ui

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"unicode"
)

// i18nPackFiles may contain CJK text: they are the language packs.
var i18nPackFiles = map[string]bool{"locale_packs.go": true}

// isCJK reports Han, kana, Hangul and full-width / CJK punctuation.
func isCJK(r rune) bool {
	return unicode.In(r, unicode.Han, unicode.Hiragana, unicode.Katakana, unicode.Hangul) ||
		(r >= 0x3000 && r <= 0x303f) || // CJK symbols and punctuation
		(r >= 0xff00 && r <= 0xffef) // full-width forms
}

// TestNoHardcodedCJK fails when a component file writes CJK interface text
// literally instead of reading it from c.Locale(). Comments, tests and the
// language packs are not checked. A literal that is not interface text
// (parsing separators, line-breaking tables, a probe glyph) is allowed with
// a "// i18n:ok <reason>" comment on the same line.
func TestNoHardcodedCJK(t *testing.T) {
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	fset := token.NewFileSet()
	checked := 0
	for _, path := range files {
		if strings.HasSuffix(path, "_test.go") || i18nPackFiles[path] {
			continue
		}
		f, err := parser.ParseFile(fset, path, nil, parser.ParseComments)
		if err != nil {
			t.Fatal(err)
		}
		checked++
		allowed := map[int]bool{}
		for _, cg := range f.Comments {
			for _, cm := range cg.List {
				if strings.Contains(cm.Text, "i18n:ok") {
					allowed[fset.Position(cm.Slash).Line] = true
				}
			}
		}
		ast.Inspect(f, func(n ast.Node) bool {
			lit, ok := n.(*ast.BasicLit)
			if !ok || (lit.Kind != token.STRING && lit.Kind != token.CHAR) {
				return true
			}
			var s string
			if lit.Kind == token.CHAR {
				r, _, _, err := strconv.UnquoteChar(lit.Value[1:len(lit.Value)-1], '\'')
				if err != nil {
					return true
				}
				s = string(r)
			} else if s, err = strconv.Unquote(lit.Value); err != nil {
				return true
			}
			if strings.IndexFunc(s, isCJK) < 0 {
				return true
			}
			pos := fset.Position(lit.Pos())
			if !allowed[pos.Line] {
				t.Errorf("%s:%d: hard-coded CJK text %q; read it from c.Locale() (add a field to Locale and the packs in locale_packs.go), or mark a non-interface literal with // i18n:ok <reason>", pos.Filename, pos.Line, s)
			}
			return true
		})
	}
	if checked < 50 {
		t.Fatalf("only %d files scanned", checked)
	}
}

// TestLocalePacksComplete checks that the built-in packs translate every
// text (nothing left for FillLocale to borrow from EnUS by accident) and
// that the format helpers produce clean output.
func TestLocalePacksComplete(t *testing.T) {
	for _, l := range []*Locale{ZhCN, EnUS, ZhTW, Ja} {
		raw := localeEmptyFields(l)
		if len(raw) > 0 {
			t.Errorf("%s: empty texts %v", l.Code, raw)
		}
		for _, s := range []string{l.Total(85), l.RangeTotal(1, 10, 85), l.PageSizeLabel(10), l.Selected(3),
			l.Year(2026), l.Month(11), l.YearMonth(2026, 11), l.Items(3), locFmt(l.ProTable.LoadErrorFmt, "x"),
			locFmt(l.MarkdownEditor.TableColumnFmt, 1),
			locFmt(l.DatePicker.WeekFmt, 2026, 3, "rd"), locFmt(l.DatePicker.QuarterFmt, 2026, 4), locFmt(l.DatePicker.QuarterCellFmt, 4),
			locFmt(l.Upload.DoneFmt, "a"), locFmt(l.Upload.FailedFmt, "a"), locFmt(l.Upload.TooManyFmt, "a"), locFmt(l.Upload.BadTypeFmt, "a"),
			locFmt(l.Upload.TooLargeFmt, "a", "1 KB"), locFmt(l.Upload.IsDirFmt, "a"),
			locFmt(l.ProField.StartFmt, "a"), locFmt(l.ProField.EndFmt, "a"), locFmt(l.ProField.RangeEndFmt, "a", "b"),
			locFmt(l.ProDescriptions.EditFmt, "a"), locFmt(l.LightFilter.ClearFmt, "a"), locFmt(l.LightFilter.PillFmt, "a", "b"),
			locFmt(l.LightFilter.MoreFmt, 2), locFmt(l.LoginForm.ResendFmt, 60), locFmt(l.LoginForm.CodeSentFmt, 60)} {
			if strings.Contains(s, "%!") || s == "" {
				t.Errorf("%s: bad format output %q", l.Code, s)
			}
		}
	}
	checks := []struct{ got, want string }{
		{ZhCN.YearMonth(2026, 11), "2026年11月"},
		{EnUS.YearMonth(2026, 11), "Nov 2026"},
		{ZhTW.YearMonth(2026, 11), "2026年11月"},
		{Ja.YearMonth(2026, 11), "2026年11月"},
		{EnUS.Month(11), "Nov"},
		{ZhCN.Month(11), "11月"},
		{Ja.Month(3), "3月"},
		{EnUS.Year(2026), "2026"},
		{ZhCN.Year(2026), "2026年"},
		{ZhCN.Total(85), "共 85 条"},
		{ZhTW.Total(85), "共 85 筆"},
		{Ja.Total(85), "全 85 件"},
		{EnUS.Selected(2), "2 selected"},
		{ZhCN.WithColon("姓名"), "姓名："},
		{EnUS.WithColon("Name"), "Name:"},
		{EnUS.WithColon(""), ""},
		{strings.Join(ZhCN.WeekdayNames(), ""), "日一二三四五六"},
		{strings.Join(EnUS.WeekdayNames(), " "), "Su Mo Tu We Th Fr Sa"},
		{strings.Join(Ja.WeekdayNames(), ""), "日月火水木金土"},
		{locFmt(ZhCN.DatePicker.WeekFmt, 2026, 46, "th"), "2026-第46周"},
		{locFmt(EnUS.DatePicker.WeekFmt, 2026, 46, "th"), "2026-46th"},
		{locFmt(ZhTW.DatePicker.WeekFmt, 2026, 46, "th"), "2026-第46週"},
		{locFmt(Ja.DatePicker.WeekFmt, 2026, 46, "th"), "2026-第46週"},
		{locFmt(Ja.DatePicker.QuarterFmt, 2026, 4), "2026-Q4"},
		{locFmt(EnUS.LightFilter.MoreFmt, 2), "More filters (2)"},
	}
	for _, c := range checks {
		if c.got != c.want {
			t.Errorf("got %q, want %q", c.got, c.want)
		}
	}
}

func TestLocaleByCodeAliases(t *testing.T) {
	for code, want := range map[string]*Locale{
		"zh-CN": ZhCN, "zh": ZhCN, "zh-Hans": ZhCN, "zh-Hans-CN": ZhCN, "zh_SG": ZhCN, "zh_CN.UTF-8": ZhCN,
		"zh-TW": ZhTW, "zh-HK": ZhTW, "zh-MO": ZhTW, "zh-Hant": ZhTW, "zh-Hant-HK": ZhTW, "zh_TW.UTF-8": ZhTW,
		"ja": Ja, "ja-JP": Ja, "ja_JP": Ja,
		"en": EnUS, "en-GB": EnUS, "EN_us": EnUS,
	} {
		if got, ok := LocaleByCode(code); !ok || got != want {
			name := "<nil>"
			if got != nil {
				name = got.Code
			}
			t.Errorf("LocaleByCode(%q) = %s, want %s", code, name, want.Code)
		}
	}
	if _, ok := LocaleByCode("xx-YY"); ok {
		t.Error("unknown language matched")
	}
	codes := []string{}
	for _, l := range Locales() {
		codes = append(codes, l.Code)
	}
	for _, c := range []string{"en-US", "ja-JP", "zh-CN", "zh-TW"} {
		if !strings.Contains(strings.Join(codes, ","), c) {
			t.Errorf("%s not registered: %v", c, codes)
		}
	}
}

// localeEmptyFields lists the empty texts of l by field path.
func localeEmptyFields(l *Locale) []string {
	var out []string
	var walk func(v reflect.Value, path string)
	walk = func(v reflect.Value, path string) {
		switch v.Kind() {
		case reflect.String:
			if v.String() == "" {
				out = append(out, path)
			}
		case reflect.Array:
			for i := 0; i < v.Len(); i++ {
				walk(v.Index(i), path+"["+strconv.Itoa(i)+"]")
			}
		case reflect.Struct:
			for i := 0; i < v.NumField(); i++ {
				walk(v.Field(i), strings.TrimPrefix(path+"."+v.Type().Field(i).Name, "."))
			}
		}
	}
	walk(reflect.ValueOf(l).Elem(), "")
	return out
}
