package editor

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// withSystemLocales sets the system locales for one test and restores
// the defaults (environment, "en") after it.
func withSystemLocales(t *testing.T, osLocales, ui []string) {
	t.Helper()
	SetSystemLocales(osLocales, ui)
	t.Cleanup(func() { SetSystemLocales(nil, nil) })
}

// pаssword has a Cyrillic "а" (U+0430).
const cyrillicLine = "let pаssword = 1"

func cyrillicMarks(o Options) int { return len(unicodeMarks(cyrillicLine, o.unicodeOpts(), nil, nil)) }

// TestUnicodeLocaleTables: the ambiguous characters follow
// AllowedLocales as Monaco's AmbiguousCharacters.getInstance resolves
// them (testdata/unicode/locales.json: getInstance's own results).
func TestUnicodeLocaleTables(t *testing.T) {
	withSystemLocales(t, []string{"en"}, []string{"en"})
	o := DefaultOptions()
	if !o.UnicodeHighlight.AllowedLocales.IsDefault() {
		t.Fatalf("default AllowedLocales %v", o.UnicodeHighlight.AllowedLocales.Names())
	}
	if n := cyrillicMarks(o); n != 1 {
		t.Fatalf("default (_os en): %d marks, want 1", n)
	}
	o.UnicodeHighlight.AllowedLocales = LocaleSetOf(map[string]bool{"ru": true})
	if n := cyrillicMarks(o); n != 0 {
		t.Fatalf("ru: %d marks, want 0", n)
	}
	// Only true values count, as Monaco's validateBooleanMap.
	o.UnicodeHighlight.AllowedLocales = LocaleSetOf(map[string]bool{"ru": false})
	if n := cyrillicMarks(o); n != 1 {
		t.Fatalf("ru false: %d marks, want 1", n)
	}
	if !LocaleSetOf(nil).IsDefault() || AllowLocales().IsDefault() {
		t.Fatal("nil map must be the default set, AllowLocales() the empty one")
	}
	if AllowLocales("ru", "de", "ru") != AllowLocales("de", "ru") {
		t.Fatal("locale sets must compare by their names")
	}

	data, err := os.ReadFile(filepath.Join("testdata", "unicode", "locales.json"))
	if err != nil {
		t.Fatal(err)
	}
	var cases []struct {
		Locales []string
		Pairs   []uint32
	}
	if err := json.Unmarshal(data, &cases); err != nil {
		t.Fatal(err)
	}
	if len(cases) < 10 {
		t.Fatalf("%d cases", len(cases))
	}
	for _, c := range cases {
		// The names go to getInstance as they are: "_os" and "_vscode"
		// here are literal names (no data key), not the system locales.
		got := ambiguousFor(strings.Join(c.Locales, ",")).pairs
		if !slices.Equal(got, c.Pairs) {
			t.Errorf("%v: %d pairs, want Monaco's %d", c.Locales, len(got)/2, len(c.Pairs)/2)
		}
	}
	// Exact keys only, as Monaco: "en-US" and "pt" name no table.
	if a, b := ambiguousFor("en-US"), ambiguousFor(""); !slices.Equal(a.pairs, b.pairs) {
		t.Error("en-US must fall back to _default")
	}
}

// TestUnicodeSystemLocales: "_os" and "_vscode" are the locales the host
// sets with SetSystemLocales; without them, "_os" is the language of
// LC_ALL, LC_MESSAGES or LANG and "_vscode" is "en".
func TestUnicodeSystemLocales(t *testing.T) {
	t.Setenv("LC_ALL", "")
	t.Setenv("LC_MESSAGES", "")
	t.Setenv("LANG", "en_US.UTF-8")
	SetSystemLocales(nil, nil)
	o := DefaultOptions()
	if n := cyrillicMarks(o); n != 1 {
		t.Fatalf("LANG en_US: %d marks, want 1", n)
	}
	withSystemLocales(t, []string{"ru"}, nil)
	if n := cyrillicMarks(o); n != 0 {
		t.Fatalf("SetSystemLocales(ru, nil): %d marks, want 0", n)
	}
	SetSystemLocales(nil, []string{"ru"})
	if n := cyrillicMarks(o); n != 0 {
		t.Fatalf("SetSystemLocales(nil, ru): %d marks, want 0", n)
	}
	// "_os" alone: the UI locale is not consulted.
	o.UnicodeHighlight.AllowedLocales = AllowLocales("_os")
	if n := cyrillicMarks(o); n != 1 {
		t.Fatalf("_os only (en_US): %d marks, want 1", n)
	}
	o = DefaultOptions()
	SetSystemLocales(nil, nil)
	t.Setenv("LANG", "ru_RU.UTF-8")
	if n := cyrillicMarks(o); n != 0 {
		t.Fatalf("LANG ru_RU: %d marks, want 0", n)
	}
	t.Setenv("LC_MESSAGES", "de_DE.UTF-8") // before LANG; de marks the "а"
	if n := cyrillicMarks(o); n != 1 {
		t.Fatalf("LC_MESSAGES de_DE: %d marks, want 1", n)
	}
	t.Setenv("LC_ALL", "ru_RU") // before both
	if n := cyrillicMarks(o); n != 0 {
		t.Fatalf("LC_ALL ru_RU: %d marks, want 0", n)
	}
	for _, c := range []struct {
		env  string
		want []string
	}{
		{"ru_RU.UTF-8", []string{"ru-RU", "ru"}},
		{"pt_BR.UTF-8@euro", []string{"pt-BR", "pt"}},
		{"de", []string{"de"}},
		{"C", []string{"en"}},
		{"POSIX", []string{"en"}},
		{"C.UTF-8", []string{"en"}},
		{"", nil},
	} {
		if got := envLocaleNames(c.env); !slices.Equal(got, c.want) {
			t.Errorf("envLocaleNames(%q) = %q, want %q", c.env, got, c.want)
		}
	}
}

// TestUnicodeLocalesConcurrent: SetSystemLocales may run on any goroutine
// while views resolve their tables (go test -race).
func TestUnicodeLocalesConcurrent(t *testing.T) {
	t.Cleanup(func() { SetSystemLocales(nil, nil) })
	done := make(chan bool)
	go func() {
		for i := range 200 {
			if i%2 == 0 {
				SetSystemLocales([]string{"ru"}, nil)
			} else {
				SetSystemLocales([]string{"de"}, []string{"fr"})
			}
		}
		done <- true
	}()
	o := DefaultOptions()
	for range 200 {
		_ = cyrillicMarks(o)
	}
	<-done
}
