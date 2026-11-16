package monarch

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
)

// A same-input stream from old/new fixed binaries pins observable states and
// tokens, including script/style MIME switches and malformed continuations.
func TestHTMLDifferentialDump(t *testing.T) {
	output := os.Getenv("TANGO_HTML_TEST_DUMP")
	if output == "" {
		t.Skip("differential output not requested")
	}
	f, err := os.Create(output)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	enc := json.NewEncoder(f)
	reg := NewRegistry()
	lx, err := reg.Lexer("html")
	if err != nil {
		t.Fatal(err)
	}
	cases := [][]string{
		{`<!DOCTYPE html>`, `<DIV a="x" B='y'>é😀</DIV>`},
		{`<!-- open`, `x-y--`, `-->tail`},
		{`<script`, `type="module">const x = 1;`, `</script>`},
		{`<script type='application/json'>`, `{"x":1}`, `</script>`},
		{`<style type="text/css">.a{color:red}</style>`},
		{`<script type="unknown/thing">`, `x`, `</script>`},
		{`<scripture>`, `</script>`},
		{`<x:a/>`, `<script />`, `<style/>`, `<x:a:b q="x">`},
		{`<div a="unfinished`, `next 'value'>`, `</div>`},
	}
	parts := []string{"<", ">", "/", "div", "SCRIPT", "style", "type", "=", " ", "\t", "'", "\"", "-", ":", "_", "a0", "<!--", "-->", "é", "😀", "\xff", "&amp;", "text/css", "module", "</script", "</style"}
	var seed uint32 = 1947
	for n := 0; n < 4000; n++ {
		line := ""
		for range n % 16 {
			seed = seed*1664525 + 1013904223
			line += parts[int(seed>>8)%len(parts)]
		}
		if n%31 == 0 {
			line += strings.Repeat(" a='b'", 100)
		}
		cases = append(cases, []string{line, "tail </script></style> -->", "<div a='1'>end</div>"})
	}
	for ci, lines := range cases {
		state := lx.Initial()
		for li, line := range lines {
			tokens, next, info := lx.TokenizeLineInfo(line, state, []Token{{Start: 9, ByteStart: 9, Type: "prefix"}})
			if err := enc.Encode(struct {
				Case, Line int
				Input      string
				Tokens     []Token
				State      State
				Info       LineInfo
			}{ci, li, line, tokens, next, info}); err != nil {
				t.Fatal(err)
			}
			state = next
		}
	}
}
