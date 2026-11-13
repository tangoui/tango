package i18n

import (
	"fmt"
	"slices"
	"sync"
	"testing"
	"testing/fstest"

	"github.com/tangoui/tango/internal/platform"
)

// setup isolates a test: no registered source, a fixed environment.
func setup(t *testing.T, env map[string]string) {
	t.Helper()
	prev := platform.LocaleSource()
	platform.SetLocaleSource(nil)
	for _, k := range []string{"LANGUAGE", "LC_ALL", "LC_MESSAGES", "LANG"} {
		t.Setenv(k, env[k])
	}
	resetForTest()
	t.Cleanup(func() {
		platform.SetLocaleSource(prev)
		resetForTest()
	})
}

func TestCanonical(t *testing.T) {
	for in, want := range map[string]string{
		"zh_hans_cn.UTF-8": "zh-Hans-CN",
		"EN-us":            "en-US",
		"zh_TW@euro":       "zh-TW",
		"es-419":           "es-419",
		"C":                "",
		"POSIX":            "",
		" ":                "",
		"ja":               "ja",
	} {
		if got := Canonical(in); got != want {
			t.Errorf("Canonical(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestMatch(t *testing.T) {
	builtin := []string{"en", "zh-CN", "zh-TW", "ja"}
	for _, tc := range []struct {
		prefs []string
		avail []string
		want  string
	}{
		{[]string{"zh-Hans-CN"}, builtin, "zh-CN"},
		{[]string{"zh-CN"}, builtin, "zh-CN"},
		{[]string{"zh"}, builtin, "zh-CN"},
		{[]string{"zh-SG"}, builtin, "zh-CN"},
		{[]string{"zh-Hans"}, builtin, "zh-CN"},
		{[]string{"zh-TW"}, builtin, "zh-TW"},
		{[]string{"zh-HK"}, builtin, "zh-TW"},
		{[]string{"zh-Hant"}, builtin, "zh-TW"},
		{[]string{"zh-Hant-HK"}, builtin, "zh-TW"},
		{[]string{"zh_TW.UTF-8"}, builtin, "zh-TW"},
		{[]string{"en-US"}, builtin, "en"},
		{[]string{"en-GB"}, builtin, "en"},
		{[]string{"ja-JP"}, builtin, "ja"},
		{[]string{"fr-FR"}, builtin, "en"},                 // nothing matches: DefaultLocale
		{[]string{"fr-FR", "ja-JP"}, builtin, "ja"},        // next preference
		{[]string{"zh-HK"}, []string{"en", "zh-CN"}, "en"}, // scripts are never swapped
		{[]string{"zh-HK"}, []string{"zh-CN", "zh-HK", "zh-TW"}, "zh-HK"},
		{[]string{"pt-BR"}, []string{"pt-PT", "pt-BR"}, "pt-BR"},
		{[]string{"pt-AO"}, []string{"pt-BR", "pt"}, "pt"},
		{[]string{"de"}, []string{"fr", "it"}, "fr"}, // no default: first
		{nil, nil, "en"},
	} {
		if got := Match(tc.prefs, tc.avail); got != tc.want {
			t.Errorf("Match(%v, %v) = %q, want %q", tc.prefs, tc.avail, got, tc.want)
		}
	}
}

func TestEnvFallback(t *testing.T) {
	setup(t, map[string]string{"LANGUAGE": "zh_TW:en", "LANG": "zh_CN.UTF-8"})
	if got, want := Preferred(), []string{"zh-TW", "en", "zh-CN"}; !slices.Equal(got, want) {
		t.Errorf("Preferred = %v, want %v", got, want)
	}
	if got := Locale(); got != "zh-TW" {
		t.Errorf("Locale = %q", got)
	}

	setup(t, map[string]string{"LANGUAGE": "zh_TW", "LC_ALL": "C"})
	if got := Preferred(); len(got) != 0 {
		t.Errorf("LANGUAGE must be ignored with LC_ALL=C: %v", got)
	}
	if got := Locale(); got != "en" {
		t.Errorf("Locale = %q", got)
	}

	setup(t, map[string]string{"LC_MESSAGES": "ja_JP.UTF-8", "LANG": "en_US.UTF-8"})
	if got := Locale(); got != "ja" {
		t.Errorf("LC_MESSAGES before LANG: %q", got)
	}
}

func TestRegisteredSource(t *testing.T) {
	setup(t, map[string]string{"LANG": "en_US.UTF-8"})
	platform.SetLocaleSource(func() []string { return []string{"zh-Hans-CN", "en-US"} })
	if got := Preferred(); !slices.Equal(got, []string{"zh-Hans-CN", "en-US"}) {
		t.Errorf("Preferred = %v", got)
	}
	if got := Locale(); got != "zh-CN" {
		t.Errorf("Locale = %q", got)
	}
}

type fakeBackend struct {
	platform.Backend
	langs []string
}

func (f fakeBackend) PreferredLocales() []string { return f.langs }

type fakeApp struct{ b platform.Backend }

func (a fakeApp) Backend() platform.Backend { return a.b }

func TestInit(t *testing.T) {
	setup(t, map[string]string{"LANG": "en_US.UTF-8"})
	if Locale() != "en" {
		t.Fatal("env")
	}
	var got []string
	OnChange(func(l string) { got = append(got, l) })
	Init(fakeApp{fakeBackend{langs: []string{"ja-JP"}}})
	if Locale() != "ja" || !slices.Equal(got, []string{"ja"}) {
		t.Errorf("Locale = %q, changes %v", Locale(), got)
	}
	Init(fakeApp{struct{ platform.Backend }{}}) // no Locales: keeps the source
	if Locale() != "ja" {
		t.Errorf("Locale = %q", Locale())
	}
}

func TestSetLocaleAndOnChange(t *testing.T) {
	setup(t, map[string]string{"LANG": "zh_CN.UTF-8"})
	var got []string
	cancel := OnChange(func(l string) { got = append(got, l) })
	if Locale() != "zh-CN" {
		t.Fatalf("Locale = %q", Locale())
	}
	SetLocale("zh-Hant") // matched against Supported
	SetLocale("zh-TW")   // no change, no call
	SetLocale("en-GB")
	SetLocale("")   // back to the system
	SetLocale("fr") // unknown: used as given
	cancel()
	SetLocale("ja")
	if want := []string{"zh-TW", "en", "zh-CN", "fr"}; !slices.Equal(got, want) {
		t.Errorf("changes %v, want %v", got, want)
	}
	if Locale() != "ja" {
		t.Errorf("Locale = %q", Locale())
	}
}

func TestRefresh(t *testing.T) {
	setup(t, map[string]string{"LANG": "en_US.UTF-8"})
	langs := []string{"en-US"}
	platform.SetLocaleSource(func() []string { return langs })
	var got []string
	OnChange(func(l string) { got = append(got, l) })
	if Locale() != "en" {
		t.Fatal(Locale())
	}
	langs = []string{"zh-HK"}
	if Locale() != "en" {
		t.Error("Preferred must be cached until Refresh")
	}
	Refresh()
	if Locale() != "zh-TW" || !slices.Equal(got, []string{"zh-TW"}) {
		t.Errorf("Locale = %q, changes %v", Locale(), got)
	}
}

var testFS = fstest.MapFS{
	"locales/en.json": {Data: []byte(`{
		"hello": "Hello, {name}!",
		"only_en": "English only",
		"files": {"one": "{count} file", "other": "{count} files"},
		"braces": "{{literal}} {missing} {name}",
		"inbox": {"one": "{name} has one message", "other": "{name} has {count} messages"}
	}`)},
	"locales/zh_CN.json": {Data: []byte(`{
		"hello": "你好，{name}！",
		"files": {"other": "{count} 个文件"}
	}`)},
	"locales/ja.json":    {Data: []byte(`{"hello": "こんにちは、{name}さん"}`)},
	"locales/readme.txt": {Data: []byte(`ignored`)},
}

func TestCatalog(t *testing.T) {
	setup(t, map[string]string{"LANG": "zh_CN.UTF-8"})
	if err := Load(testFS, "locales"); err != nil {
		t.Fatal(err)
	}
	if got := Supported(); !slices.Equal(got, []string{"en", "ja", "zh-CN"}) {
		t.Errorf("Supported = %v", got)
	}
	for _, tc := range []struct {
		got, want string
	}{
		{T("hello", "name", "Tom"), "你好，Tom！"},
		{T("only_en"), "English only"}, // default locale
		{T("nope.key"), "nope.key"},    // the key itself
		{T("nope {name}", "name", "x"), "nope x"},
		{N("files", 1), "1 个文件"}, // zh: only "other"
		{N("files", 5), "5 个文件"},
		{T("files"), "{count} 个文件"},
		{T("braces", "name", 7), "{literal} {missing} 7"},
		{N("inbox", 1, "name", "Ann"), "Ann has one message"}, // fell back to en: en rule
		{N("inbox", 2, "name", "Ann"), "Ann has 2 messages"},
		{N("hello", 2, "name", "Bo"), "你好，Bo！"}, // plain string for any count
	} {
		if tc.got != tc.want {
			t.Errorf("got %q, want %q", tc.got, tc.want)
		}
	}
	SetLocale("en-US")
	if got := N("files", 1); got != "1 file" {
		t.Errorf("en one: %q", got)
	}
	if got := N("files", 0); got != "0 files" {
		t.Errorf("en other: %q", got)
	}
	SetLocale("ja")
	if got := T("hello", "name", "Ken"); got != "こんにちは、Kenさん" {
		t.Errorf("ja: %q", got)
	}
}

func TestLoadChangesSupported(t *testing.T) {
	setup(t, map[string]string{"LANG": "zh_TW.UTF-8"})
	var got []string
	OnChange(func(l string) { got = append(got, l) })
	if Locale() != "zh-TW" {
		t.Fatal(Locale())
	}
	// The app ships no zh-TW: the locale falls back to its default.
	if err := Load(testFS, "locales"); err != nil {
		t.Fatal(err)
	}
	if Locale() != "en" || !slices.Equal(got, []string{"en"}) {
		t.Errorf("Locale = %q, changes %v", Locale(), got)
	}
}

func TestLoadErrors(t *testing.T) {
	setup(t, nil)
	for name, data := range map[string]string{
		"syntax":   `{`,
		"type":     `{"a": 1}`,
		"category": `{"a": {"some": "x", "other": "y"}}`,
		"other":    `{"a": {"one": "x"}}`,
	} {
		err := Load(fstest.MapFS{"l/en.json": {Data: []byte(data)}}, "l")
		if err == nil {
			t.Errorf("%s: want error", name)
		}
	}
	if err := Load(fstest.MapFS{}, "missing"); err == nil {
		t.Error("missing dir: want error")
	}
}

func TestPluralCategory(t *testing.T) {
	for _, tc := range []struct {
		loc  string
		n    int
		want string
	}{
		{"zh-CN", 1, "other"}, {"ja", 1, "other"}, {"en", 1, "one"}, {"en", 0, "other"},
		{"en-US", -1, "one"}, {"fr", 0, "one"}, {"fr", 2, "other"}, {"xx", 1, "one"},
	} {
		if got := PluralCategory(tc.loc, tc.n); got != tc.want {
			t.Errorf("PluralCategory(%q, %d) = %q, want %q", tc.loc, tc.n, got, tc.want)
		}
	}
	RegisterPluralRule("ru", func(n int) string {
		switch {
		case n%10 == 1 && n%100 != 11:
			return "one"
		case n%10 >= 2 && n%10 <= 4 && (n%100 < 12 || n%100 > 14):
			return "few"
		}
		return "many"
	})
	if PluralCategory("ru-RU", 3) != "few" || PluralCategory("ru", 11) != "many" {
		t.Error("registered rule not used")
	}
}

func TestConcurrent(t *testing.T) {
	setup(t, map[string]string{"LANG": "en_US.UTF-8"})
	var wg sync.WaitGroup
	cancel := OnChange(func(string) {})
	defer cancel()
	for i := range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := range 200 {
				switch (i + j) % 6 {
				case 0:
					SetLocale([]string{"zh-CN", "en", "ja", ""}[j%4])
				case 1:
					_ = T("hello", "name", "x")
				case 2:
					_ = N("files", j)
				case 3:
					_ = Locale() + fmt.Sprint(Preferred())
				case 4:
					if j%50 == 0 {
						_ = Load(testFS, "locales")
					}
				case 5:
					c := OnChange(func(string) {})
					c()
				}
			}
		}()
	}
	wg.Wait()
}
