package editor

import (
	"slices"
	"testing"

	"github.com/tangoui/tango/ui/editor/theme"
)

// walkCmd runs command id repeatedly from caret off and returns the
// carets it stops at.
func walkCmd(t *testing.T, text, id string, off, n int) []int {
	t.Helper()
	st := NewState(text, "a.txt", theme.Base("vs"))
	st.SetSelections(carets(off))
	var out []int
	for range n {
		st.Execute(id, nil)
		out = append(out, st.Primary().Head)
	}
	return out
}

func TestWordPartMovement(t *testing.T) {
	const parts = "fooBarBaz_qux-quux"
	for _, c := range []struct {
		text, id string
		from     int
		want     []int
	}{
		// camelCase, snake_case and kebab-case parts, and the word starts
		// and ends between them.
		{parts, "cursorWordPartRight", 0, []int{3, 6, 9, 13, 14, 18, 18}},
		{parts, "cursorWordPartLeft", 18, []int{14, 13, 10, 6, 3, 0, 0}},
		{parts, "cursorWordPartStartLeft", 18, []int{14}},
		{"HTMLParser", "cursorWordPartRight", 0, []int{4, 10}},
		{"HTMLParser", "cursorWordPartLeft", 10, []int{4, 0}},
		{"get2D_point", "cursorWordPartRight", 0, []int{4, 5, 11}},
		{"ab\ncd", "cursorWordPartLeft", 3, []int{2, 0}},
		{"ab\ncd", "cursorWordPartRight", 2, []int{3, 5}},
		// The Monaco word family.
		{"foo.bar baz", "cursorWordLeft", 11, []int{8, 4, 0}},
		{"foo.bar baz", "cursorWordStartLeft", 11, []int{8, 4, 3, 0}},
		{"foo.bar baz", "cursorWordEndLeft", 11, []int{7, 4, 3, 0}},
		{"foo.bar baz", "cursorWordEndRight", 0, []int{3, 7, 11}},
		{"foo.bar baz", "cursorWordRight", 0, []int{3, 7, 11}},
		{"foo.bar baz", "cursorWordStartRight", 0, []int{3, 4, 8, 11}},
		{"a == b", "cursorWordAccessibilityRight", 0, []int{5, 6}},
		{"a == b", "cursorWordAccessibilityLeft", 6, []int{5, 0}},
		{"ab\n  cd", "cursorWordLeft", 5, []int{3, 0}},
		{"ab\n  cd", "cursorWordEndRight", 2, []int{7}},
		{"中文 ab", "cursorWordEndRight", 0, []int{6, 9}},
	} {
		if got := walkCmd(t, c.text, c.id, c.from, len(c.want)); !slices.Equal(got, c.want) {
			t.Errorf("%s in %q from %d: %v, want %v", c.id, c.text, c.from, got, c.want)
		}
	}
	runSeq(t, []seqCase{
		{"part right select", "a.txt", "|fooBar", []string{"#cursorWordPartRightSelect"}, "[foo]Bar", nil},
		{"part left select", "a.txt", "fooBar|", []string{"#cursorWordPartLeftSelect"}, "foo]Bar[", nil},
		{"word left select extends", "a.txt", "a b c|", []string{"#cursorWordLeftSelect", "#cursorWordLeftSelect"}, "a ]b c[", nil},
		{"carets", "a.txt", "|fooBar\n|bazQux", []string{"#cursorWordPartRight"}, "foo|Bar\nbaz|Qux", nil},
		// Deleting.
		{"delete part left", "a.txt", "fooBar|", []string{"#deleteWordPartLeft"}, "foo|", nil},
		{"delete part right", "a.txt", "|fooBar", []string{"#deleteWordPartRight"}, "|Bar", nil},
		{"delete part left snake", "a.txt", "foo_bar|", []string{"#deleteWordPartLeft"}, "foo_|", nil},
		{"delete part left blanks", "a.txt", "fooBar   |", []string{"#deleteWordPartLeft"}, "fooBar|", nil},
		{"delete word left", "a.txt", "alpha beta |", []string{"#deleteWordLeft"}, "alpha |", nil},
		{"delete word left blanks", "a.txt", "foo   |bar", []string{"#deleteWordLeft"}, "foo|bar", nil},
		{"delete word start left", "a.txt", "foo   |bar", []string{"#deleteWordStartLeft"}, "|bar", nil},
		{"delete word end left", "a.txt", "foo bar|", []string{"#deleteWordEndLeft"}, "foo|", nil},
		{"delete word left at line start", "a.txt", "ab\n|cd", []string{"#deleteWordLeft"}, "ab|cd", nil},
		{"delete word right", "a.txt", "|foo bar", []string{"#deleteWordRight"}, "| bar", nil},
		{"delete word right blanks", "a.txt", "|   foo", []string{"#deleteWordRight"}, "|foo", nil},
		{"delete word start right", "a.txt", "|foo bar", []string{"#deleteWordStartRight"}, "|bar", nil},
		{"delete word end right", "a.txt", "|   foo", []string{"#deleteWordEndRight"}, "|", nil},
		{"delete word right at line end", "a.txt", "ab|\n  cd", []string{"#deleteWordRight"}, "ab|cd", nil},
		{"delete selection", "a.txt", "a[bc]d", []string{"#deleteWordPartLeft"}, "a|d", nil},
		{"delete inside word", "a.txt", "foo b|ar baz", []string{"#deleteInsideWord"}, "foo |baz", nil},
		{"delete inside word at end", "a.txt", "foo ba|r", []string{"#deleteInsideWord"}, "foo|", nil},
		{"delete inside blanks", "a.txt", "foo  |  bar", []string{"#deleteInsideWord"}, "foo|bar", nil},
		{"delete words at carets", "a.txt", "aB|\ncD|", []string{"#deleteWordPartLeft"}, "a|\nc|", nil},
	})
	// deleteInsideWord {onlyWord: true} keeps the blanks.
	st := seqState(t, "a.txt", "foo b|ar baz", nil)
	st.Execute("deleteInsideWord", map[string]any{"onlyWord": true})
	if got := marked(st); got != "foo | baz" {
		t.Errorf("deleteInsideWord onlyWord: %q", got)
	}
	// A word delete is an undo step of its own.
	st = seqState(t, "a.txt", "fooBar|", nil)
	st.Type("x")
	st.Execute("deleteWordPartLeft", nil)
	st.Undo()
	if got := marked(st); got != "fooBarx|" {
		t.Errorf("undo of deleteWordPartLeft: %q", got)
	}
}
