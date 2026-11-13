package main

import (
	"bytes"
	"encoding/csv"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/tangoui/tango/ui"
	"github.com/tangoui/tango/ui/geom"
)

func TestStoreRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sub", "ledger.json")
	s, err := Open(path, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Put(Entry{Date: "2026-11-01", Kind: Expense, Category: "餐饮", Cents: 3650, Note: "午饭"}); err != nil {
		t.Fatal(err)
	}
	if err := s.Put(Entry{Date: "2026-10-10", Kind: Income, Category: "工资", Cents: 1580000}); err != nil {
		t.Fatal(err)
	}
	e := s.Entries[0]
	e.Cents = 4000
	if err := s.Put(e); err != nil {
		t.Fatal(err)
	}

	r, err := Open(path, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Entries) != 2 || r.Entries[0].Cents != 4000 || r.NextID != 3 {
		t.Fatalf("reloaded %+v", r)
	}
	if got := r.Months(); strings.Join(got, ",") != "2026-11,2026-10" {
		t.Fatalf("months %v", got)
	}
	if err := r.Delete(1); err != nil || len(r.Entries) != 1 {
		t.Fatalf("delete: %v %+v", err, r.Entries)
	}
}

func TestMoneyAndCSV(t *testing.T) {
	for c, want := range map[int64]string{0: "¥0.00", 5: "¥0.05", 123456789: "¥1,234,567.89", -100000: "-¥1,000.00"} {
		if got := money(c); got != want {
			t.Errorf("money(%d) = %q, want %q", c, got, want)
		}
	}
	if toCents(0.1+0.2) != 30 || toCents(19.995) != 2000 {
		t.Error("toCents rounding")
	}
	var b bytes.Buffer
	if err := WriteCSV(&b, []Entry{{Date: "2026-11-01", Kind: Expense, Category: "餐饮", Cents: 3650, Note: "午饭, 两人"}}); err != nil {
		t.Fatal(err)
	}
	recs, err := csv.NewReader(strings.NewReader(strings.TrimPrefix(b.String(), "\xEF\xBB\xBF"))).ReadAll()
	if err != nil {
		t.Fatal(err)
	}
	if len(recs) != 2 || recs[1][1] != "支出" || recs[1][3] != "36.50" || recs[1][4] != "午饭, 两人" {
		t.Fatalf("csv %q", recs)
	}
	if c, _ := parseCents(recs[1][3]); c != 3650 {
		t.Fatalf("parse back %d", c)
	}
}

// TestAddThroughUI drives the real page: open the editor, fill in the form,
// save, and check the record reached the file and the totals.
func TestAddThroughUI(t *testing.T) {
	path := filepath.Join(t.TempDir(), "ledger.json")
	s, err := Open(path, seedSample(time.Now()))
	if err != nil {
		t.Fatal(err)
	}
	before := len(s.Entries)
	l := newLedger(s)
	d := ui.NewDriver(l.Page, ui.DriverOptions{Width: 1180, Height: 780})
	d.Settle()

	// Saving an empty form is refused by validation.
	must(t, d.ClickLabel("记一笔"))
	d.Settle()
	must(t, d.ClickLabel("保存"))
	d.Settle()
	if !l.editing || len(s.Entries) != before {
		t.Fatal("empty form was saved")
	}
	for _, msg := range []string{"请输入金额", "请选择分类"} {
		if _, ok := d.FindText(msg); !ok {
			t.Errorf("no validation message %q", msg)
		}
	}
	if dir := os.Getenv("LEDGER_SHOT"); dir != "" {
		_ = d.SavePNG(filepath.Join(dir, "invalid.png"))
	}

	// Fill amount, category, note.
	r, ok := d.FindText("金额")
	if !ok {
		t.Fatal("no amount field")
	}
	d.Click(r.Center().Add(geom.Pt(160, 0))) // the input right of the label
	d.Settle()
	d.Type("88.8")
	d.Settle()
	r, _ = d.FindText("分类")
	d.Click(r.Center().Add(geom.Pt(160, 0)))
	d.Settle()
	must(t, d.ClickLabel("交通"))
	d.Settle()
	r, _ = d.FindText("备注")
	d.Click(r.Center().Add(geom.Pt(160, 0)))
	d.Settle()
	d.Type("打车回家")
	d.Settle()
	must(t, d.ClickLabel("保存"))
	d.Settle()

	if l.editing {
		_ = d.SavePNG(filepath.Join(t.TempDir(), "fail.png"))
		t.Fatalf("editor still open: amt=%v cat=%q note=%q", l.editAmt, l.editCat, l.editNote)
	}
	if len(s.Entries) != before+1 {
		t.Fatalf("entries %d, want %d", len(s.Entries), before+1)
	}
	got := s.Entries[len(s.Entries)-1]
	if got.Kind != Expense || got.Category != "交通" || got.Cents != 8880 || got.Note != "打车回家" || got.Date != time.Now().Format(dateLayout) {
		t.Fatalf("saved %+v", got)
	}
	r2, err := Open(path, nil)
	if err != nil || len(r2.Entries) != before+1 {
		t.Fatalf("file not updated: %v", err)
	}
	if _, ok := d.FindText("打车回家"); !ok {
		t.Error("new record not in table")
	}
}

func must(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}

func TestCheckAmount(t *testing.T) {
	for v, bad := range map[string]bool{"": false, "36.50": false, "0.01": false, "0.00": true, "-5.00": true} {
		if got := checkAmount(v) != ""; got != bad {
			t.Errorf("checkAmount(%q) rejected=%v, want %v", v, got, bad)
		}
	}
}

// TestShortcutActions checks what the window shortcuts do (setupShortcuts
// binds them; uihost tests cover the key matching).
func TestShortcutActions(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "ledger.json"), seedSample(time.Now()))
	if err != nil {
		t.Fatal(err)
	}
	l := newLedger(s)
	d := ui.NewDriver(l.Page, ui.DriverOptions{Width: 1180, Height: 780})
	d.Settle()

	l.findEntries() // CmdOrCtrl+F: typing goes to the search box
	d.Settle()
	d.Type("午饭")
	d.Settle()
	if l.search != "午饭" {
		t.Fatalf("search %q after CmdOrCtrl+F and typing", l.search)
	}

	l.newEntry() // CmdOrCtrl+N
	d.Settle()
	if !l.editing || l.edit.ID != 0 {
		t.Fatal("editor not open for a new record")
	}
	if _, ok := d.FindText("记一笔"); !ok {
		t.Error("no editor title")
	}
	l.editNote = "draft"
	l.newEntry() // again while open: keeps the draft
	l.findEntries()
	if l.editNote != "draft" || l.focusSearch {
		t.Fatal("shortcuts acted behind the open editor")
	}
}
