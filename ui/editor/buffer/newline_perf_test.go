package buffer

import (
	"fmt"
	"strings"
	"testing"
)

var newlineQuery int

// The previous implementation, kept only as a same-binary reference.
func linearAfterNewline(n *node, k int) int {
	off := 0
	for !n.isLeaf() {
		if k <= n.left.lines {
			n = n.left
		} else {
			k -= n.left.lines
			off += n.left.bytes
			n = n.right
		}
	}
	t := n.text
	for {
		i := strings.IndexByte(t, '\n')
		k--
		if k == 0 {
			return off + i + 1
		}
		off += i + 1
		t = t[i+1:]
	}
}

func BenchmarkLeafNewlineRank(b *testing.B) {
	for _, width := range []int{1, 2, 10, 100, 1024} {
		b.Run(fmt.Sprintf("width%d", width), func(b *testing.B) {
			line := strings.Repeat("x", width-1) + "\n"
			n := newLeaf(strings.Repeat(line, leafTarget/width))
			for _, arm := range []struct {
				name string
				read func(*node, int) int
			}{{"linear", linearAfterNewline}, {"candidate", afterNewline}} {
				b.Run(arm.name, func(b *testing.B) {
					b.ReportAllocs()
					k := 0
					for b.Loop() {
						k = (k + 37) % n.lines
						newlineQuery = arm.read(n, k+1)
					}
				})
			}
		})
	}
}

func TestLeafNewlineRank(t *testing.T) {
	check := func(text string) {
		t.Helper()
		off := 0
		parts := strings.Split(text, "\n")
		for i, part := range parts[:len(parts)-1] {
			off += len(part) + 1
			if got := leafAfterNewline(text, i+1); got != off {
				t.Fatalf("len=%d rank=%d: got %d, want %d", len(text), i+1, got, off)
			}
		}
	}
	// Every short arrangement, then both sides of the counting cutoffs,
	// maximal leaves, non-ASCII, invalid UTF-8 and uneven distributions.
	for mask := range 1 << 10 {
		var a [10]byte
		for i := range a {
			a[i] = 'x'
			if mask&(1<<i) != 0 {
				a[i] = '\n'
			}
		}
		check(string(a[:]))
	}
	for _, n := range []int{63, 64, 65, 127, 128, 129, 1023, 1024, leafMax} {
		check(strings.Repeat("\n", n))
		check(strings.Repeat("a", n) + strings.Repeat("\n", n))
		check(strings.Repeat("\n", n) + strings.Repeat("a", n))
		check(strings.Repeat("世\xff\x00😀\n", n/12+1))
		check(strings.Repeat("x", n) + "\n")
	}
}

// afterNewline's dispatch: a multi-leaf rope whose dense leaves take the
// halving path (k > 8, n.lines >= 32, short lines) at a non-zero leaf offset, and whose
// sparse leaves and early ranks keep the byte search. Both must match an
// independent scan of the whole text.
func TestAfterNewlineDispatch(t *testing.T) {
	for _, text := range []string{
		strings.Repeat("a\n", leafMax*3),                                 // dense leaves only
		strings.Repeat("x", leafMax) + strings.Repeat("\n", 40) + "tail", // sparse then dense
		strings.Repeat("\n", 40) + strings.Repeat("y", leafMax*2) + "\n", // newlines packed at a leaf start
		strings.Repeat("世😀\xff\n", leafMax),                              // non-ASCII, split runes
	} {
		s := NewSnapshot(text)
		want, off := 0, 0
		for k := 1; k <= s.root.lines; k++ {
			i := strings.IndexByte(text[off:], '\n')
			want = off + i + 1
			off = want
			if got := afterNewline(s.root, k); got != want {
				t.Fatalf("len=%d k=%d: got %d, want %d", len(text), k, got, want)
			}
		}
	}
}
