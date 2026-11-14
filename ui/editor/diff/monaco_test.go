package diff

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"testing"
)

// monacoCase is one fixture of testdata/monaco/cases.json, produced by
// running Monaco's DefaultLinesDiffComputer (see gen.mjs there).
type monacoCase struct {
	Name                 string
	Original, Modified   []string
	IgnoreTrimWhitespace bool
	ComputeMoves         bool
	ExtendToSubwords     bool
	Changes              []json.RawMessage
	Moves                []json.RawMessage
}

// formatChanges renders changes in the fixture's compact array form.
func formatChanges(cs []Change) []string {
	out := make([]string, len(cs))
	for i, c := range cs {
		var inner []string
		for _, m := range c.Inner {
			o, md := m.Original, m.Modified
			inner = append(inner, fmt.Sprintf("[%d,%d,%d,%d,%d,%d,%d,%d]",
				o.Start.Line, o.Start.Col, o.End.Line, o.End.Col, md.Start.Line, md.Start.Col, md.End.Line, md.End.Col))
		}
		out[i] = fmt.Sprintf("[%d,%d,%d,%d,[%s]]", c.Original.Start, c.Original.End, c.Modified.Start, c.Modified.End, strings.Join(inner, ","))
	}
	return out
}

func formatMoves(ms []Move) []string {
	out := make([]string, len(ms))
	for i, m := range ms {
		out[i] = fmt.Sprintf("[%d,%d,%d,%d,[%s]]", m.Original.Start, m.Original.End, m.Modified.Start, m.Modified.End,
			strings.Join(formatChanges(m.Changes), ","))
	}
	return out
}

func rawStrings(rs []json.RawMessage) []string {
	out := make([]string, len(rs))
	for i, r := range rs {
		out[i] = string(r)
	}
	return out
}

func TestMatchesMonaco(t *testing.T) {
	data, err := os.ReadFile("testdata/monaco/cases.json")
	if err != nil {
		t.Fatal(err)
	}
	var cases []monacoCase
	if err := json.Unmarshal(data, &cases); err != nil {
		t.Fatal(err)
	}
	// Cases where this package deliberately differs from Monaco, with the
	// result it gives instead.
	deviations := map[string]string{
		// Monaco diffs UTF-16 code units, so 😀 -> 😃 (which share their
		// high surrogate) becomes a change of half a character. Runes keep
		// the whole emoji: bytes [6,10) of line 1.
		"unicode": "[0,2,0,2,[[0,9,0,15,0,9,0,15],[1,6,1,10,1,6,1,10],[1,15,1,15,1,15,1,16]]]",
	}
	for _, tc := range cases {
		t.Run(tc.Name, func(t *testing.T) {
			res := Lines(context.Background(), tc.Original, tc.Modified, Options{
				IgnoreTrimWhitespace: tc.IgnoreTrimWhitespace,
				ComputeMoves:         tc.ComputeMoves,
				ExtendToSubwords:     tc.ExtendToSubwords,
			})
			got, want := strings.Join(formatChanges(res.Changes), "\n"), strings.Join(rawStrings(tc.Changes), "\n")
			if dev, ok := deviations[tc.Name]; ok {
				want = dev
			}
			if got != want {
				t.Errorf("changes differ\noriginal: %q\nmodified: %q\ngot:\n%s\nwant (Monaco):\n%s", tc.Original, tc.Modified, got, want)
			}
			got, want = strings.Join(formatMoves(res.Moves), "\n"), strings.Join(rawStrings(tc.Moves), "\n")
			if got != want {
				t.Errorf("moves differ\noriginal: %q\nmodified: %q\ngot:\n%s\nwant (Monaco):\n%s", tc.Original, tc.Modified, got, want)
			}
		})
	}
}
