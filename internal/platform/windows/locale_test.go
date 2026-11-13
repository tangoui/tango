package windows

import (
	"testing"

	"github.com/tangoui/tango/i18n"
	"github.com/tangoui/tango/internal/platform"
)

// The menu and dialog plan tests expect English labels whatever the
// machine's language is.
func init() { i18n.SetLocale("en") }

func TestMenuPlanLocalized(t *testing.T) {
	defer i18n.SetLocale("en")
	i18n.SetLocale("zh-CN")
	p := buildMenuPlan([]platform.MenuItem{{Role: "appMenu"}, {Role: "editMenu"}}, "Demo")
	app, edit := p.Entries[0], p.Entries[1]
	if app.Children[0].Label != "关于 Demo" || app.Children[2].Label != "退出" {
		t.Errorf("app = %+v", app.Children)
	}
	if edit.Label != "编辑" || edit.Children[4].Label != "复制\tCtrl+C" {
		t.Errorf("edit = %+v", edit)
	}
	if got := messageButtons(nil); got[0] != "确定" {
		t.Errorf("buttons = %v", got)
	}
}
