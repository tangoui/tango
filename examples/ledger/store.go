package main

import (
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"time"
)

// Entry kinds.
const (
	Expense = "expense"
	Income  = "income"
)

// Categories offered in the editor, per kind.
var categories = map[string][]string{
	Expense: {"餐饮", "交通", "购物", "住房", "娱乐", "医疗", "教育", "其他"},
	Income:  {"工资", "奖金", "理财", "兼职", "其他"},
}

const dateLayout = "2006-01-02"

// Entry is one bookkeeping record. Amounts are kept in cents so sums never
// pick up floating-point noise.
type Entry struct {
	ID       int64  `json:"id"`
	Date     string `json:"date"` // YYYY-MM-DD
	Kind     string `json:"kind"` // Expense | Income
	Category string `json:"category"`
	Cents    int64  `json:"cents"`
	Note     string `json:"note,omitempty"`
}

// Month returns the entry's month as YYYY-MM.
func (e Entry) Month() string { return e.Date[:7] }

// Signed returns the amount with expenses negative.
func (e Entry) Signed() int64 {
	if e.Kind == Expense {
		return -e.Cents
	}
	return e.Cents
}

// Store is the ledger file: a JSON document written atomically on every change.
type Store struct {
	path    string
	NextID  int64   `json:"nextId"`
	Entries []Entry `json:"entries"`
}

// Open loads the ledger at path. A missing file gives an empty ledger;
// seed, if not nil, fills it on first run.
func Open(path string, seed func(*Store)) (*Store, error) {
	s := &Store{path: path, NextID: 1}
	b, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		if seed != nil {
			seed(s)
			return s, s.save()
		}
		return s, nil
	}
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(b, s); err != nil {
		return nil, fmt.Errorf("读取账本 %s：%w", path, err)
	}
	for _, e := range s.Entries {
		if e.ID >= s.NextID {
			s.NextID = e.ID + 1
		}
	}
	return s, nil
}

// Path returns the file the ledger lives in.
func (s *Store) Path() string { return s.path }

// Put adds e (ID 0) or replaces the entry with e's ID, then saves.
func (s *Store) Put(e Entry) error {
	if e.ID == 0 {
		e.ID = s.NextID
		s.NextID++
		s.Entries = append(s.Entries, e)
	} else if i := s.index(e.ID); i >= 0 {
		s.Entries[i] = e
	} else {
		return fmt.Errorf("记录 %d 不存在", e.ID)
	}
	return s.save()
}

// Delete removes the entry with the given ID, then saves.
func (s *Store) Delete(id int64) error {
	i := s.index(id)
	if i < 0 {
		return nil
	}
	s.Entries = append(s.Entries[:i], s.Entries[i+1:]...)
	return s.save()
}

// Months lists the months that have entries, newest first.
func (s *Store) Months() []string {
	seen := map[string]bool{}
	var out []string
	for _, e := range s.Entries {
		if m := e.Month(); !seen[m] {
			seen[m] = true
			out = append(out, m)
		}
	}
	sort.Sort(sort.Reverse(sort.StringSlice(out)))
	return out
}

func (s *Store) index(id int64) int {
	for i, e := range s.Entries {
		if e.ID == id {
			return i
		}
	}
	return -1
}

// save writes to a temporary file and renames it over the ledger, so a
// crash never leaves a half-written file behind.
func (s *Store) save() error {
	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return err
	}
	b, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, s.path)
}

// WriteCSV writes entries as CSV (UTF-8 with BOM so Excel opens it correctly).
func WriteCSV(w io.Writer, entries []Entry) error {
	if _, err := io.WriteString(w, "\xEF\xBB\xBF"); err != nil {
		return err
	}
	cw := csv.NewWriter(w)
	_ = cw.Write([]string{"日期", "类型", "分类", "金额", "备注"})
	for _, e := range entries {
		kind := "支出"
		if e.Kind == Income {
			kind = "收入"
		}
		_ = cw.Write([]string{e.Date, kind, e.Category, centsString(e.Cents), e.Note})
	}
	cw.Flush()
	return cw.Error()
}

// centsString formats cents as "1234.50".
func centsString(c int64) string {
	sign := ""
	if c < 0 {
		sign, c = "-", -c
	}
	return fmt.Sprintf("%s%d.%02d", sign, c/100, c%100)
}

// money formats cents for display: "¥1,234.50".
func money(c int64) string {
	s := centsString(c)
	neg := s[0] == '-'
	if neg {
		s = s[1:]
	}
	intPart, frac := s[:len(s)-3], s[len(s)-3:]
	var out []byte
	for i := range intPart {
		if i > 0 && (len(intPart)-i)%3 == 0 {
			out = append(out, ',')
		}
		out = append(out, intPart[i])
	}
	r := "¥" + string(out) + frac
	if neg {
		r = "-" + r
	}
	return r
}

// toCents converts a yuan amount from the editor, rounding to the nearest cent.
func toCents(yuan float64) int64 {
	if yuan < 0 {
		return -int64(-yuan*100 + 0.5)
	}
	return int64(yuan*100 + 0.5)
}

// seedSample fills a fresh ledger with a few weeks of plausible records so
// the first launch has something to look at.
func seedSample(now time.Time) func(*Store) {
	return func(s *Store) {
		day := func(monthsAgo, d int) string {
			t := time.Date(now.Year(), now.Month()-time.Month(monthsAgo), d, 0, 0, 0, 0, time.Local)
			if t.After(now) {
				return "" // no records in the future
			}
			return t.Format(dateLayout)
		}
		add := func(date, kind, cat string, yuan float64, note string) {
			if date == "" {
				return
			}
			s.Entries = append(s.Entries, Entry{ID: s.NextID, Date: date, Kind: kind, Category: cat, Cents: toCents(yuan), Note: note})
			s.NextID++
		}
		for m := 1; m >= 0; m-- {
			add(day(m, 1), Expense, "住房", 3200, "房租")
			add(day(m, 3), Expense, "餐饮", 36.5, "午饭")
			add(day(m, 5), Expense, "交通", 120, "地铁卡充值")
			add(day(m, 8), Expense, "购物", 299, "运动鞋")
			add(day(m, 10), Income, "工资", 15800, "")
			add(day(m, 12), Expense, "餐饮", 268, "朋友聚餐")
			add(day(m, 15), Expense, "娱乐", 89, "电影票 ×2")
			add(day(m, 18), Expense, "医疗", 45.8, "感冒药")
			add(day(m, 20), Income, "理财", 312.47, "基金收益")
			add(day(m, 22), Expense, "教育", 199, "在线课程")
			add(day(m, 25), Expense, "餐饮", 52, "外卖")
		}
		add(day(1, 28), Income, "奖金", 3000, "季度奖")
	}
}

// parseCents parses a yuan amount such as "36.5" into cents.
func parseCents(s string) (int64, error) {
	f, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0, err
	}
	return toCents(f), nil
}
