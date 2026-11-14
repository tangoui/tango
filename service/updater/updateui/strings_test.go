package updateui

import (
	"reflect"
	"testing"
)

func TestBuiltinStrings(t *testing.T) {
	cases := map[string]string{
		"zh-CN": "软件更新",
		"zh":    "软件更新",
		"en-US": "Software Update",
		"zh-TW": "軟體更新",
		"zh-HK": "軟體更新",
		"ja":    "ソフトウェア・アップデート",
		"ja-JP": "ソフトウェア・アップデート",
		"fr":    "Software Update",
		"":      "Software Update",
	}
	for loc, want := range cases {
		if got := builtinStrings(loc).Title; got != want {
			t.Errorf("builtinStrings(%q).Title = %q, want %q", loc, got, want)
		}
	}
	for _, loc := range []string{"en", "zh-CN", "zh-TW", "ja"} {
		v := reflect.ValueOf(*builtinStrings(loc))
		for i := 0; i < v.NumField(); i++ {
			if v.Type().Field(i).IsExported() && v.Field(i).String() == "" {
				t.Errorf("%s: field %s is empty", loc, v.Type().Field(i).Name)
			}
		}
	}
	if got := stringsFor(Options{Text: &Strings{Title: "X"}}).Title; got != "X" {
		t.Errorf("Options.Text not used: %q", got)
	}
}
