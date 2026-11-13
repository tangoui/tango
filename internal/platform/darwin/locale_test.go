//go:build darwin && !ios

package darwin

import (
	"testing"

	"github.com/tangoui/tango/i18n"
)

// The menu and dialog tests expect English labels whatever the machine's
// language is.
func init() { i18n.SetLocale("en") }

func TestPreferredLocales(t *testing.T) {
	got := preferredLocales()
	if len(got) == 0 || got[0] == "" {
		t.Fatalf("preferredLocales() = %v", got)
	}
	t.Logf("preferred languages: %v", got)
}

func TestRoleLabelLocalized(t *testing.T) {
	defer i18n.SetLocale("en")
	i18n.SetLocale("zh-CN")
	for key, want := range map[string]string{
		menuRoles["copy"].label:     "拷贝",
		menuRoles["front"].label:    "前置全部窗口",
		menuRoles["quit"].label:     "退出 Demo",
		roleMenus["editMenu"].title: "编辑",
		roleMenus["appMenu"].title:  "Demo",
		"Custom %s":                 "Custom Demo",
	} {
		if got := roleLabel(key, "Demo"); got != want {
			t.Errorf("roleLabel(%q) = %q, want %q", key, got, want)
		}
	}
}
