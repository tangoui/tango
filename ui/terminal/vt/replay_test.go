package vt

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"
	"unicode/utf8"
)

// The replay test feeds the samples in testdata/replay to a Terminal and
// compares the screen with reference snapshots that headless xterm.js
// produced from the same input (internal/refgen; see its README.md).
// Intentional differences from xterm.js are listed in
// testdata/replay/DIFFERENCES.md and kept out of the samples.

// replaySnap is a reference snapshot, in refgen's JSON format.
type replaySnap struct {
	Cols    int          `json:"cols"`
	Rows    int          `json:"rows"`
	Cursor  [2]int       `json:"cursor"` // x is cols while a wrap is pending
	Lines   []replayLine `json:"lines"`
	Replies string       `json:"replies,omitempty"`
}

// replayLine is one row of a snapshot. Text is the row's characters with
// right halves of wide characters skipped, blanks as spaces and trailing
// blanks (cells never written) trimmed; Cells lists runs of cells in a
// non-default style as [x, count, fg, bg, attrs]; Wide lists the columns
// of wide characters; Chars lists the cells that hold anything but one
// printable ASCII character as [x, "text"]; Wrapped is set when the row
// continues the one above it.
type replayLine struct {
	Text    string       `json:"text"`
	Cells   [][5]uint32  `json:"cells,omitempty"`
	Wide    []int        `json:"wide,omitempty"`
	Chars   []replayChar `json:"chars,omitempty"`
	Wrapped bool         `json:"wrapped,omitempty"`
}

// replayChar is the content of one cell, [x, "text"] in JSON.
type replayChar struct {
	X    int
	Text string
}

func (c *replayChar) UnmarshalJSON(b []byte) error {
	var a [2]json.RawMessage
	if err := json.Unmarshal(b, &a); err != nil {
		return err
	}
	if err := json.Unmarshal(a[0], &c.X); err != nil {
		return err
	}
	return json.Unmarshal(a[1], &c.Text)
}

// replayStep is one step of a .ops sample.
type replayStep struct {
	Write  *string `json:"write"`
	Resize []int   `json:"resize"`
}

func TestReplay(t *testing.T) {
	vts, _ := filepath.Glob(filepath.Join("testdata", "replay", "*.vt"))
	ops, _ := filepath.Glob(filepath.Join("testdata", "replay", "*.ops"))
	files := append(vts, ops...)
	if len(files) == 0 {
		t.Fatal("no samples in testdata/replay")
	}
	for _, f := range files {
		base := strings.TrimSuffix(strings.TrimSuffix(f, ".vt"), ".ops")
		t.Run(filepath.Base(f), func(t *testing.T) {
			want := readSnap(t, base+".json")
			steps := readSteps(t, f)
			cols, rows := readSize(t, base+".size")
			// Whole writes, then the same bytes in small chunks that
			// split escape sequences and UTF-8 characters.
			for _, chunk := range []int{0, 7} {
				got := replay(steps, cols, rows, chunk)
				if msg := diffSnap(want, got); msg != "" {
					t.Fatalf("chunk size %d: %s", chunk, msg)
				}
			}
		})
	}
}

func readSnap(t *testing.T, path string) replaySnap {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("%v (regenerate with internal/refgen)", err)
	}
	var s replaySnap
	if err := json.Unmarshal(b, &s); err != nil {
		t.Fatalf("%s: %v", path, err)
	}
	return s
}

func readSteps(t *testing.T, path string) []replayStep {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.HasSuffix(path, ".vt") {
		s := string(b)
		return []replayStep{{Write: &s}}
	}
	var steps []replayStep
	if err := json.Unmarshal(b, &steps); err != nil {
		t.Fatalf("%s: %v", path, err)
	}
	for i, st := range steps {
		if (st.Write == nil) == (len(st.Resize) != 2) {
			t.Fatalf("%s: step %d: want one of write or resize [cols, rows]", path, i)
		}
	}
	return steps
}

func readSize(t *testing.T, path string) (cols, rows int) {
	t.Helper()
	b, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return 80, 24
	}
	if err != nil {
		t.Fatal(err)
	}
	c, r, ok := strings.Cut(strings.TrimSpace(string(b)), "x")
	cols, err1 := strconv.Atoi(c)
	rows, err2 := strconv.Atoi(r)
	if !ok || err1 != nil || err2 != nil {
		t.Fatalf("%s: want COLSxROWS, got %q", path, b)
	}
	return cols, rows
}

// replay runs steps on a new terminal, writing in chunks of chunk bytes
// (0: each write whole), and returns its snapshot.
func replay(steps []replayStep, cols, rows, chunk int) replaySnap {
	var replies bytes.Buffer
	term := New(Options{Cols: cols, Rows: rows, Reply: func(b []byte) { replies.Write(b) }})
	for _, st := range steps {
		if st.Write == nil {
			term.Resize(st.Resize[0], st.Resize[1])
			continue
		}
		b := []byte(*st.Write)
		for len(b) > 0 {
			n := len(b)
			if chunk > 0 {
				n = min(n, chunk)
			}
			term.Write(b[:n])
			b = b[n:]
		}
	}
	return snapOf(term, replies.String())
}

// snapOf takes a terminal's snapshot in refgen's format.
func snapOf(t *Terminal, replies string) replaySnap {
	s := t.Screen()
	snap := replaySnap{Cols: t.cols, Rows: t.rows, Cursor: [2]int{t.cx, t.cy}, Replies: replies}
	if t.pending {
		snap.Cursor[0] = t.cols // xterm.js's representation
	}
	tb := s.Styles()
	for y := range t.rows {
		l := s.Line(y)
		var rl replayLine
		if y > 0 {
			rl.Wrapped = s.Line(y - 1).Wrapped
		} else if s.sbLen > 0 {
			rl.Wrapped = s.at(s.sbLen - 1).Wrapped
		}
		end := 0 // columns up to the last written cell
		for x, c := range l.Cells {
			if c.Rune != 0 {
				end = x + max(int(c.Width), 1)
			}
		}
		var text strings.Builder
		for x, c := range l.Cells {
			if c.Width == 2 {
				rl.Wide = append(rl.Wide, x)
			}
			if c.Rune != 0 && (c.Rune > 0x7E || len(l.Combining(x)) > 0) {
				ch := string(c.Rune)
				for _, m := range l.Combining(x) {
					ch += string(m)
				}
				rl.Chars = append(rl.Chars, replayChar{x, ch})
			}
			if x < end && c.Width != 0 {
				if c.Rune == 0 {
					text.WriteByte(' ')
				} else {
					text.WriteRune(c.Rune)
					for _, m := range l.Combining(x) {
						text.WriteRune(m)
					}
				}
			}
			st := tb.Get(c.Style)
			if st == (Style{}) {
				continue
			}
			seg := [5]uint32{uint32(x), 1, uint32(st.Fg), uint32(st.Bg), uint32(st.Attr)}
			if n := len(rl.Cells); n > 0 {
				p := &rl.Cells[n-1]
				if p[0]+p[1] == uint32(x) && p[2] == seg[2] && p[3] == seg[3] && p[4] == seg[4] {
					p[1]++
					continue
				}
			}
			rl.Cells = append(rl.Cells, seg)
		}
		rl.Text = text.String()
		snap.Lines = append(snap.Lines, rl)
	}
	return snap
}

// replayCol is one column of a snapshot row, for reporting differences.
type replayCol struct {
	text       string // "" for the right half of a wide character
	fg, bg, at uint32
}

func (c replayCol) String() string {
	s := fmt.Sprintf("%+q", c.text)
	if c.text == "" {
		s = "(right half)"
	}
	if c.fg|c.bg|c.at != 0 {
		s += fmt.Sprintf(" fg=%#x bg=%#x attrs=%#x", c.fg, c.bg, c.at)
	}
	return s
}

// columns expands a snapshot row into its cols columns.
func columns(l replayLine, cols int) []replayCol {
	out := make([]replayCol, cols)
	for i := range out {
		out[i].text = " "
	}
	text := l.Text
	chars := l.Chars
	for x := 0; x < cols && text != ""; x++ {
		if len(chars) > 0 && chars[0].X == x && strings.HasPrefix(text, chars[0].Text) {
			out[x].text = chars[0].Text
			text = text[len(chars[0].Text):]
			chars = chars[1:]
		} else {
			_, n := utf8.DecodeRuneInString(text)
			out[x].text = text[:n]
			text = text[n:]
		}
		if slices.Contains(l.Wide, x) && x+1 < cols {
			x++
			out[x].text = ""
		}
	}
	for _, s := range l.Cells {
		for x := s[0]; x < s[0]+s[1] && int(x) < cols; x++ {
			out[x].fg, out[x].bg, out[x].at = s[2], s[3], s[4]
		}
	}
	return out
}

// diffSnap describes the first difference between the reference want and
// got, or returns "".
func diffSnap(want, got replaySnap) string {
	if want.Cols != got.Cols || want.Rows != got.Rows {
		return fmt.Sprintf("size: got %dx%d, want %dx%d", got.Cols, got.Rows, want.Cols, want.Rows)
	}
	for y := range want.Rows {
		w, g := want.Lines[y], got.Lines[y]
		wc, gc := columns(w, want.Cols), columns(g, got.Cols)
		for x := range wc {
			if wc[x] != gc[x] {
				return fmt.Sprintf("row %d col %d: got %v, want %v\n got row: %q\nwant row: %q",
					y, x, gc[x], wc[x], g.Text, w.Text)
			}
		}
		if w.Text != g.Text || !slices.Equal(w.Wide, g.Wide) || !slices.Equal(w.Chars, g.Chars) {
			return fmt.Sprintf("row %d: got text %q wide %v chars %+v, want %q wide %v chars %+v",
				y, g.Text, g.Wide, g.Chars, w.Text, w.Wide, w.Chars)
		}
		if w.Wrapped != g.Wrapped {
			return fmt.Sprintf("row %d: got wrapped (continues the row above) %v, want %v", y, g.Wrapped, w.Wrapped)
		}
	}
	if want.Cursor != got.Cursor {
		return fmt.Sprintf("cursor: got %v, want %v (x = cols means a wrap is pending)", got.Cursor, want.Cursor)
	}
	if want.Replies != got.Replies {
		return fmt.Sprintf("replies: got %+q, want %+q", got.Replies, want.Replies)
	}
	return ""
}

// TestReplaySamplesSmall keeps the samples small and free of private
// data markers.
func TestReplaySamplesSmall(t *testing.T) {
	files, _ := filepath.Glob(filepath.Join("testdata", "replay", "*"))
	for _, f := range files {
		b, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		if len(b) > 200<<10 {
			t.Errorf("%s: %d bytes, want at most 200 KB", f, len(b))
		}
		if strings.HasSuffix(f, ".vt") && !utf8.Valid(b) {
			t.Logf("%s: not valid UTF-8", f) // allowed, but worth knowing
		}
		if bytes.Contains(b, []byte("/Users/")) {
			t.Errorf("%s: contains a home directory path", f)
		}
	}
}
