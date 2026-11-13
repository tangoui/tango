package highlight

import (
	"strings"
	"testing"
	"time"

	"github.com/tangoui/tango/ui/editor/buffer"
	"github.com/tangoui/tango/ui/editor/monarch"
)

// 2 万行 TS 从头全量高亮（规格 6.6：≤ 2 s）。
func BenchmarkHighlightTS20k(b *testing.B) {
	lx := tsLexer(b)
	s := buffer.NewSnapshot(tsDoc(b, 20000))
	b.ResetTimer()
	for range b.N {
		h := New(lx)
		if !h.Highlight(s, s.LineCount(), time.Hour) {
			b.Fatal("not ready")
		}
	}
}

// 100 万行文件末尾的编辑：Edit（作废、行号平移）加上对可见区（前 100 行）的 Highlight。
// "char" 只改行内字符，"newline" 交替插入 / 删除换行（行状态数组整体平移）。
func BenchmarkEditInvalidate1M(b *testing.B) {
	lx := tsLexer(b)
	src := tsLines(b)[:40]
	var sb strings.Builder
	for i := range 1_000_000 {
		if i > 0 {
			sb.WriteByte('\n')
		}
		sb.WriteString(src[i%len(src)])
	}
	base := buffer.NewSnapshot(sb.String())
	for _, mode := range []string{"char", "newline", "middle-newline"} {
		b.Run(mode, func(b *testing.B) {
			h := New(lx)
			s := base
			h.Highlight(s, 100, time.Hour)
			b.ResetTimer()
			for i := range b.N {
				var e buffer.Edit
				switch {
				case mode == "char":
					e = buffer.Edit{Start: s.Len(), End: s.Len(), Text: "x"}
				case mode == "newline" && i%2 == 0:
					e = buffer.Edit{Start: s.Len(), End: s.Len(), Text: "\n"}
				case mode == "newline":
					e = buffer.Edit{Start: s.Len() - 1, End: s.Len()}
				case i%2 == 0:
					off := s.LineStart(500_000)
					e = buffer.Edit{Start: off, End: off, Text: "\n"}
				default:
					off := s.LineStart(500_000)
					e = buffer.Edit{Start: off - 1, End: off}
				}
				edits := []buffer.Edit{e}
				ns, err := s.Apply(edits)
				if err != nil {
					b.Fatal(err)
				}
				h.Edit(s, ns, edits)
				s = ns
				if !h.Highlight(s, 100, time.Hour) {
					b.Fatal("top not ready")
				}
			}
		})
	}
}

// 在可见区（token 窗口内）的光标处连续输入：Edit 原地作废该行的槽（不重建约 4001 槽的窗口），
// 再对可见区 Highlight 并取该行 token。"edit" 只计 Edit（应为 0 allocs/op）；
// "edit+highlight" 含重新分词与取 token（分配来自 Lexer 的匹配，不来自窗口）。
// 快照的 Apply 不计入（StopTimer）。
func BenchmarkTypeAtCursor(b *testing.B) {
	lx := tsLexer(b)
	base := buffer.NewSnapshot(tsDoc(b, 10000))
	for _, mode := range []string{"edit", "edit+highlight"} {
		b.Run(mode, func(b *testing.B) {
			h := New(lx)
			s := base
			h.Highlight(s, 100, time.Hour)
			var dst []monarch.Token
			b.ReportAllocs()
			b.ResetTimer()
			for range b.N {
				b.StopTimer()
				off := s.LineStart(50)
				edits := []buffer.Edit{{Start: off, End: off, Text: "x"}}
				ns, err := s.Apply(edits)
				if err != nil {
					b.Fatal(err)
				}
				b.StartTimer()
				h.Edit(s, ns, edits)
				s = ns
				if mode == "edit+highlight" {
					h.Highlight(s, 100, time.Hour)
					dst, _ = h.Tokens(s, 50, dst)
				}
			}
		})
	}
}
