package codehl

import (
	"strings"
	"testing"
	"time"

	"github.com/tangoui/tango/ui"
)

func TestLanguageID(t *testing.T) {
	for in, want := range map[string]string{
		"go": "go", "Go": "go", "golang": "go", "js": "javascript", "ts": "typescript", ".py": "python",
		"python": "python", "json": "javascript", "yaml": "yaml", "yml": "yaml", "sh": "shell", "bash": "shell",
		"rs": "rust", "Dockerfile": "dockerfile", "nope-lang": "", "": "",
	} {
		if got := LanguageID(in); got != want {
			t.Errorf("LanguageID(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestDocumentSpans(t *testing.T) {
	h := New()
	code := "package main\n\n// hi\nfunc main() { x := \"s\" + 42 }"
	d := h.Document("go", code)
	if d == nil {
		t.Fatal("no document for go")
	}
	kinds := func(i int) map[string]ui.CodeTokenKind {
		line := strings.Split(code, "\n")[i]
		spans, final := d.Line(i, nil)
		if !final {
			t.Fatalf("line %d not final", i)
		}
		m := map[string]ui.CodeTokenKind{}
		prev := 0
		for _, s := range spans {
			if s.Start < prev || s.End <= s.Start || s.End > len(line) {
				t.Fatalf("line %d: bad span %+v", i, s)
			}
			prev = s.End
			m[line[s.Start:s.End]] = s.Kind
		}
		return m
	}
	if k := kinds(0)["package"]; k != ui.CodeKeyword {
		t.Errorf("package: %v", k)
	}
	if k := kinds(2)["// hi"]; k != ui.CodeComment {
		t.Errorf("comment: %v", kinds(2))
	}
	m := kinds(3)
	if m["func"] != ui.CodeKeyword || m[`"s"`] != ui.CodeString || m["42"] != ui.CodeNumber {
		t.Errorf("line 3: %v", m)
	}
	if h.Document("go", code) != d {
		t.Error("the same code must reuse its document")
	}
	if h.Document("no-such-language", code) != nil {
		t.Error("unknown language must give nil")
	}
	if s, ok := d.Line(99, nil); len(s) != 0 || !ok {
		t.Error("out of range line")
	}
}

func TestLargeDocumentCatchesUp(t *testing.T) {
	var b strings.Builder
	for i := 0; i < 5000; i++ {
		b.WriteString("var x = \"a\" // c\n")
	}
	d := New().Document("js", b.String())
	deadline := time.Now().Add(10 * time.Second)
	for {
		if _, final := d.Line(4999, nil); final {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("the end of a long document never became final")
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func TestKind(t *testing.T) {
	for in, want := range map[string]ui.CodeTokenKind{
		"keyword.go": ui.CodeKeyword, "string.key.json": ui.CodeProperty, "string.value.json": ui.CodeString,
		"type.identifier.ts": ui.CodeType, "identifier.go": ui.CodePlain, "delimiter.bracket": ui.CodePunctuation,
		"attribute.value.html": ui.CodeString, "attribute.name.html": ui.CodeAttribute, "": ui.CodePlain,
	} {
		if got := Kind(in); got != want {
			t.Errorf("Kind(%q) = %v, want %v", in, got, want)
		}
	}
}

func BenchmarkDocumentLine(b *testing.B) {
	var sb strings.Builder
	for i := 0; i < 2000; i++ {
		sb.WriteString("func f(a int) string { return fmt.Sprint(a + 1) } // tail\n")
	}
	d := New().Document("go", sb.String())
	var dst []ui.CodeSpan
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		dst, _ = d.Line(i%2000, dst)
	}
}
