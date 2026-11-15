package diffview

import (
	"fmt"
	"strings"

	"github.com/tangoui/tango/ui/editor/diff"
)

// announceText holds the formats of what a screen reader hears when the
// user moves to a change. They come from the locale (ui.Locale.T with the
// keys below); the fallbacks are Simplified Chinese, the default locale.
type announceText struct {
	change   string // "diffview.a11y.change", args: n (1-based), total
	removed  string // "diffview.a11y.removed", arg: line span of the original
	inserted string // "diffview.a11y.inserted", arg: line span of the modified text
	sep      string // "diffview.a11y.sep", between removed and inserted
	span     string // "diffview.a11y.range", args: first, last line (1-based)
	none     string // "diffview.a11y.none", when there are no changes
}

// announceTexts reads the formats through t (ui.Locale.T; nil uses the
// fallbacks).
func announceTexts(t func(key, fallback string) string) announceText {
	if t == nil {
		t = func(_, fallback string) string { return fallback }
	}
	return announceText{
		change:   t("diffview.a11y.change", "第 %d 处差异，共 %d 处："),
		removed:  t("diffview.a11y.removed", "删除第 %s 行"),
		inserted: t("diffview.a11y.inserted", "新增第 %s 行"),
		sep:      t("diffview.a11y.sep", "，"),
		span:     t("diffview.a11y.range", "%d–%d"),
		none:     t("diffview.a11y.none", "没有差异"),
	}
}

// announceChange returns the announcement for change i of n, e.g.
// "第 3 处差异，共 12 处：删除第 40–41 行，新增第 40–44 行".
func announceChange(tx announceText, i, n int, c diff.Change) string {
	if n == 0 {
		return tx.none
	}
	var b strings.Builder
	b.WriteString(sprintf(tx.change, i+1, n))
	if !c.Original.Empty() {
		b.WriteString(sprintf(tx.removed, tx.lines(c.Original)))
	}
	if !c.Modified.Empty() {
		if !c.Original.Empty() {
			b.WriteString(tx.sep)
		}
		b.WriteString(sprintf(tx.inserted, tx.lines(c.Modified)))
	}
	return b.String()
}

func (tx announceText) lines(r diff.LineRange) string {
	if r.Len() == 1 {
		return fmt.Sprint(r.Start + 1)
	}
	return sprintf(tx.span, r.Start+1, r.End)
}

// sprintf is fmt.Sprintf without the "%!(EXTRA ...)" note, so a locale's
// format may leave out arguments (like ui's locFmt).
func sprintf(format string, args ...any) string {
	s := fmt.Sprintf(format, args...)
	if i := strings.Index(s, "%!(EXTRA "); i >= 0 {
		s = s[:i]
	}
	return s
}
