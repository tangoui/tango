package rolenames

import (
	"strings"
	"testing"

	"github.com/tangoui/tango/i18n"
)

func TestTableComplete(t *testing.T) {
	en := table["en"]
	for _, loc := range Locales {
		m := table[loc]
		if len(m) != len(en) {
			t.Errorf("%s: %d keys, en has %d", loc, len(m), len(en))
		}
		for k, v := range en {
			lv, ok := m[k]
			if !ok || lv == "" {
				t.Errorf("%s: missing %q", loc, k)
				continue
			}
			if strings.Contains(v, "%s") != strings.Contains(lv, "%s") {
				t.Errorf("%s %q: placeholder mismatch %q vs %q", loc, k, lv, v)
			}
			if strings.HasPrefix(k, "gtk.") && !strings.Contains(lv, "_") {
				t.Errorf("%s %q: GTK label without mnemonic: %q", loc, k, lv)
			}
		}
	}
}

func TestLookup(t *testing.T) {
	for _, tc := range []struct{ loc, key, want string }{
		{"zh-Hans-CN", "mac.copy", "拷贝"},
		{"zh-CN", "win.copy", "复制"},
		{"zh-HK", "mac.front", "將此程式所有視窗移至最前"},
		{"ja-JP", "mac.quit", "%sを終了"},
		{"fr", "mac.quit", "Quit %s"},
		{"en-GB", "gtk.cut", "Cu_t"},
	} {
		if got, ok := Lookup(tc.loc, tc.key); !ok || got != tc.want {
			t.Errorf("Lookup(%q, %q) = %q, %v; want %q", tc.loc, tc.key, got, ok, tc.want)
		}
	}
	if _, ok := Lookup("en", "nope"); ok {
		t.Error("unknown key")
	}
}

func TestCurrentLocale(t *testing.T) {
	i18n.SetLocale("zh-CN")
	defer i18n.SetLocale("")
	if got := Format("mac.quit", "Demo"); got != "退出 Demo" {
		t.Errorf("Format = %q", got)
	}
	if got := Text("Custom"); got != "Custom" {
		t.Errorf("unknown key = %q", got)
	}
}
