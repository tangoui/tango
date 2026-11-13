package mobilecanvas

import (
	"reflect"
	"testing"

	"github.com/tangoui/tango/ui/input"
)

func comp(s string, cur int) input.Composition  { return input.Composition{Text: s, Cursor: cur} }
func text(s string) input.TextInput             { return input.TextInput{Text: s} }
func key(k input.Key, down bool) input.KeyEvent { return input.KeyEvent{Key: k, Down: down} }

var (
	endComp = input.Composition{}
	bsDown  = key(input.KeyBackspace, true)
	bsUp    = key(input.KeyBackspace, false)
	enDown  = key(input.KeyEnter, true)
	enUp    = key(input.KeyEnter, false)
)

func TestIMEDoc(t *testing.T) {
	type op struct {
		name string // marked, insert, unmark, delete, enter, reset
		s    string
		sel  Range
		n    int
		want []input.Event
		snap IMESnapshot
	}
	tests := []struct {
		name string
		ops  []op
	}{
		{"pinyin compose then commit", []op{
			{name: "marked", s: "zh", sel: Range{2, 0}, want: []input.Event{comp("zh", 2)},
				snap: IMESnapshot{Marked: "zh", Len: 2, Sel: Range{2, 0}, Caret: 2}},
			{name: "marked", s: "zhong", sel: Range{5, 0}, want: []input.Event{comp("zhong", 5)},
				snap: IMESnapshot{Marked: "zhong", Len: 5, Sel: Range{5, 0}, Caret: 5}},
			{name: "insert", s: "中", want: []input.Event{text("中"), endComp}},
		}},
		{"insert without composition", []op{
			{name: "insert", s: "hi", want: []input.Event{text("hi")}},
		}},
		{"marked selection in UTF-16 becomes a rune cursor", []op{
			{name: "marked", s: "😀ab", sel: Range{3, 0}, want: []input.Event{comp("😀ab", 2)},
				snap: IMESnapshot{Marked: "😀ab", Len: 4, Sel: Range{3, 0}, Caret: 4}},
			{name: "marked", s: "😀ab", sel: Range{2, 2}, want: []input.Event{comp("😀ab", 1)},
				snap: IMESnapshot{Marked: "😀ab", Len: 4, Sel: Range{2, 2}, Caret: 4}},
		}},
		{"marked selection is clamped", []op{
			{name: "marked", s: "ab", sel: Range{9, 3}, want: []input.Event{comp("ab", 2)},
				snap: IMESnapshot{Marked: "ab", Len: 2, Sel: Range{2, 0}, Caret: 2}},
			{name: "marked", s: "ab", sel: Range{-4, 5}, want: []input.Event{comp("ab", 0)},
				snap: IMESnapshot{Marked: "ab", Len: 2, Sel: Range{0, 1}, Caret: 2}},
		}},
		{"empty marked text cancels", []op{
			{name: "marked", s: "", want: nil},
			{name: "marked", s: "ka", sel: Range{2, 0}, want: []input.Event{comp("ka", 2)},
				snap: IMESnapshot{Marked: "ka", Len: 2, Sel: Range{2, 0}, Caret: 2}},
			{name: "marked", s: "", want: []input.Event{endComp}},
			{name: "unmark", want: nil},
		}},
		{"unmark commits the marked text", []op{
			{name: "marked", s: "にほん", sel: Range{3, 0}, want: []input.Event{comp("にほん", 3)},
				snap: IMESnapshot{Marked: "にほん", Len: 3, Sel: Range{3, 0}, Caret: 3}},
			{name: "unmark", want: []input.Event{text("にほん"), endComp}},
			{name: "unmark", want: nil},
		}},
		{"delete without composition presses backspace", []op{
			{name: "delete", n: 1, want: []input.Event{bsDown, bsUp}},
			{name: "delete", n: 3, want: []input.Event{bsDown, bsUp, bsDown, bsUp, bsDown, bsUp}},
			{name: "delete", n: 0, want: nil},
			{name: "delete", n: -2, want: nil},
		}},
		{"delete inside composition shortens it by characters", []op{
			{name: "marked", s: "aé👨‍👩‍👧", sel: Range{11, 0},
				want: []input.Event{comp("aé👨‍👩‍👧", 8)},
				snap: IMESnapshot{Marked: "aé👨‍👩‍👧", Len: 11, Sel: Range{11, 0}, Caret: 11}},
			{name: "delete", n: 1, want: []input.Event{comp("aé", 3)},
				snap: IMESnapshot{Marked: "aé", Len: 3, Sel: Range{3, 0}, Caret: 3}},
			{name: "delete", n: 1, want: []input.Event{comp("a", 1)},
				snap: IMESnapshot{Marked: "a", Len: 1, Sel: Range{1, 0}, Caret: 1}},
			{name: "delete", n: 1, want: []input.Event{endComp}},
			{name: "delete", n: 1, want: []input.Event{bsDown, bsUp}},
		}},
		{"delete more than the composition ends it", []op{
			{name: "marked", s: "ab", sel: Range{2, 0}, want: []input.Event{comp("ab", 2)},
				snap: IMESnapshot{Marked: "ab", Len: 2, Sel: Range{2, 0}, Caret: 2}},
			{name: "delete", n: 5, want: []input.Event{endComp}},
		}},
		{"enter commits then presses enter", []op{
			{name: "marked", s: "ni", sel: Range{2, 0}, want: []input.Event{comp("ni", 2)},
				snap: IMESnapshot{Marked: "ni", Len: 2, Sel: Range{2, 0}, Caret: 2}},
			{name: "enter", want: []input.Event{text("ni"), endComp, enDown, enUp}},
			{name: "enter", want: []input.Event{enDown, enUp}},
		}},
		{"line breaks in inserted text become enter", []op{
			{name: "insert", s: "\n", want: []input.Event{enDown, enUp}},
			{name: "insert", s: "a\nb", want: []input.Event{text("a"), enDown, enUp, text("b")}},
			{name: "insert", s: "a\r\n", want: []input.Event{text("a"), enDown, enUp}},
			{name: "insert", s: "x\ry", want: []input.Event{text("x"), enDown, enUp, text("y")}},
		}},
		{"control characters are dropped", []op{
			{name: "insert", s: "a\tb\x7f", want: []input.Event{text("ab")}},
			{name: "insert", s: "\x00", want: nil},
			{name: "marked", s: "\x01", want: nil},
		}},
		{"insert replaces the composition with nothing", []op{
			{name: "marked", s: "ab", sel: Range{2, 0}, want: []input.Event{comp("ab", 2)},
				snap: IMESnapshot{Marked: "ab", Len: 2, Sel: Range{2, 0}, Caret: 2}},
			{name: "insert", s: "", want: []input.Event{endComp}},
		}},
		{"reset drops the composition silently", []op{
			{name: "marked", s: "ab", sel: Range{2, 0}, want: []input.Event{comp("ab", 2)},
				snap: IMESnapshot{Marked: "ab", Len: 2, Sel: Range{2, 0}, Caret: 2}},
			{name: "reset"},
			{name: "unmark", want: nil},
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var d IMEDoc
			for i, o := range tt.ops {
				var evs []input.Event
				var snap IMESnapshot
				switch o.name {
				case "marked":
					evs, snap = d.SetMarked(o.s, o.sel)
				case "insert":
					evs, snap = d.Insert(o.s)
				case "unmark":
					evs, snap = d.Unmark()
				case "delete":
					evs, snap = d.DeleteBackward(o.n)
				case "enter":
					evs, snap = d.Enter()
				case "reset":
					d.Reset()
					snap = d.Snapshot()
				default:
					t.Fatalf("bad op %q", o.name)
				}
				if len(evs) == 0 {
					evs = nil
				}
				if !reflect.DeepEqual(evs, o.want) {
					t.Fatalf("op %d %s(%q): events\n got %#v\nwant %#v", i, o.name, o.s, evs, o.want)
				}
				if snap != o.snap {
					t.Fatalf("op %d %s(%q): snapshot %+v, want %+v", i, o.name, o.s, snap, o.snap)
				}
				if snap != d.Snapshot() || d.Marked() != (snap.Marked != "") {
					t.Fatalf("op %d: Snapshot/Marked disagree with the returned snapshot", i)
				}
			}
		})
	}
}

func TestRangeEnd(t *testing.T) {
	if (Range{3, 4}).End() != 7 {
		t.Fatal("End")
	}
}
