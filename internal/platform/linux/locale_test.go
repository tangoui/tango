package linux

import (
	"testing"

	"github.com/tangoui/tango/i18n"
	"github.com/tangoui/tango/internal/platform"
)

// The menu and dialog spec tests expect English labels whatever the
// machine's language is.
func init() { i18n.SetLocale("en") }

func TestMenuEntriesLocalized(t *testing.T) {
	defer i18n.SetLocale("en")
	i18n.SetLocale("zh-CN")
	got := buildMenuEntries([]platform.MenuItem{{Role: "appMenu"}, {Role: "editMenu"}}, "")
	if got[0].Label != "文件(_F)" || got[0].Children[2].Label != "退出(_Q)" {
		t.Errorf("app = %+v", got[0])
	}
	if got[1].Label != "编辑(_E)" || got[1].Children[4].Label != "复制(_C)" {
		t.Errorf("edit = %+v", got[1])
	}
	if b := messageButtons(platform.MessageOptions{}); b[0] != "确定(_O)" {
		t.Errorf("buttons = %v", b)
	}
}
