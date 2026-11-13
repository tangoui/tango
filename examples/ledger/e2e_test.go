package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/tangoui/tango/automation"
	"github.com/tangoui/tango/ui/a11y"
)

// TestE2EAddRecord drives the real app (a development build in its own
// process, real window) through the automation channel: open the editor,
// fill amount, category and note, save, and check the table and the data
// file. Skipped unless TANGO_E2E is set:
//
//	TANGO_E2E=1 go test ./examples/ledger -run E2E -v
//
// LEDGER_SHOT=dir keeps the screenshot.
func TestE2EAddRecord(t *testing.T) {
	data := filepath.Join(t.TempDir(), "ledger.json")
	app := automation.Launch(t, ".", "-data", data)
	win := app.Window("记账本")
	if win.Kind() != automation.KindCanvas {
		t.Fatalf("kind %q", win.Kind())
	}
	win.Find(a11y.RoleHeading, "记账本").Wait()

	win.Find(a11y.RoleButton, "记一笔").Click()
	dlg := win.Find(a11y.RoleDialog, "记一笔")
	dlg.Wait()

	// Saving the empty form is refused by validation.
	win.Find(a11y.RoleButton, "保存").Click()
	win.FindContains(a11y.RoleNone, "请输入金额").Wait()

	win.Find(a11y.RoleSpinButton, "金额").SetValue("88.8")
	win.Find(a11y.RoleCombobox, "分类").Expand()
	win.Find(a11y.RoleOption, "交通").Click()
	win.Find(a11y.RoleCombobox, "分类").WaitValue("交通")
	win.Find(a11y.RoleTextbox, "备注").Type("打车回家")
	win.Find(a11y.RoleTextbox, "备注").WaitValue("打车回家")
	win.Find(a11y.RoleButton, "保存").Click()

	dlg.WaitGone()
	row := win.FindContains(a11y.RoleRow, "打车回家").Node()
	if !contains(row.Name, "交通") || !contains(row.Name, "-¥88.80") {
		t.Errorf("new row %q", row.Name)
	}

	shot := filepath.Join(t.TempDir(), "ledger.png")
	if dir := os.Getenv("LEDGER_SHOT"); dir != "" {
		shot = filepath.Join(dir, "ledger-e2e.png")
	}
	win.SaveScreenshot(shot)
	if fi, err := os.Stat(shot); err != nil || fi.Size() < 1000 {
		t.Fatalf("screenshot %v %v", fi, err)
	}

	// The record reached the data file.
	s, err := Open(data, nil)
	if err != nil {
		t.Fatal(err)
	}
	last := s.Entries[len(s.Entries)-1]
	if last.Category != "交通" || last.Cents != 8880 || last.Note != "打车回家" || last.Date != time.Now().Format(dateLayout) {
		t.Fatalf("saved %+v", last)
	}
	app.Quit()
}

func contains(s, sub string) bool { return strings.Contains(s, sub) }
