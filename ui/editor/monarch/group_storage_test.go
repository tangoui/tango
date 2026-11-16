package monarch

import (
	"strings"
	"sync"
	"testing"
)

func TestGroupStoragePreservesCapturesAndFallback(t *testing.T) {
	lx := addLexer(t, "group-storage", `{"defaultToken":"fallback","tokenPostfix":"","tokenizer":{"root":[["(a)(b)",["left",{"token":"$1:$2"}]]]}}`)
	line := "ab?ab世😀\xffab"
	prefix := Token{Start: 7, ByteStart: 9, Type: "prefix"}
	want := []Token{
		prefix,
		{Start: 0, ByteStart: 0, Type: "left"},
		{Start: 1, ByteStart: 1, Type: "a:b"},
		{Start: 2, ByteStart: 2, Type: "fallback"},
		{Start: 3, ByteStart: 3, Type: "left"},
		{Start: 4, ByteStart: 4, Type: "a:b"},
		{Start: 5, ByteStart: 5, Type: "fallback"},
		{Start: 9, ByteStart: 13, Type: "left"},
		{Start: 10, ByteStart: 14, Type: "a:b"},
	}
	for range 3 {
		got, state, info := lx.TokenizeLineInfo(line, lx.Initial(), []Token{prefix})
		if !equalTokenSlices(got, want) || state != lx.Initial() || info.Degraded {
			t.Fatalf("tokens=%v state=%v info=%+v; want %v and initial state", got, state, info, want)
		}
		if _, _, info := lx.TokenizeLineInfo("?ab", lx.Initial(), nil); info.Degraded {
			t.Fatal("subsequent line degraded")
		}
		if !equalTokenSlices(got, want) {
			t.Fatal("a later line overwrote retained tokens")
		}
	}
}

func TestGroupStorageMixedSizes(t *testing.T) {
	lx := addLexer(t, "group-sizes", `{"tokenPostfix":"","tokenizer":{"root":[
		["(a)(b)(c)(d)(e)",["A","B","C","D","E"]],
		["(x)(y)",["X","Y"]],
		["(1)(2)(3)(4)",["one","two","three","four"]],
		["(z)",["Z"]]
	]}}`)
	types := map[byte]string{'a': "A", 'b': "B", 'c': "C", 'd': "D", 'e': "E", 'x': "X", 'y': "Y", '1': "one", '2': "two", '3': "three", '4': "four", 'z': "Z"}
	for _, line := range []string{"zxy1234abcdexy1234z", "abcdexyz1234xyabcde"} {
		var want []Token
		for i := range len(line) {
			want = append(want, Token{Start: i, ByteStart: i, Type: types[line[i]]})
		}
		got, state, info := lx.TokenizeLineInfo(line, lx.Initial(), nil)
		if !equalTokenSlices(got, want) || state != lx.Initial() || info.Degraded {
			t.Fatalf("%q: tokens=%v state=%v info=%+v; want %v", line, got, state, info, want)
		}
	}
}

func TestGroupStorageIsCallLocal(t *testing.T) {
	lx := addLexer(t, "group-concurrent", `{"defaultToken":"fallback","tokenPostfix":"","tokenizer":{"root":[["([a-z]+)(:)",["name",{"token":"$1"}]]]}}`)
	var wg sync.WaitGroup
	for worker := range 8 {
		wg.Go(func() {
			word := strings.Repeat(string(rune('a'+worker)), worker+1)
			line := strings.Repeat(word+":?", 8)
			var want []Token
			for offset := 0; offset < len(line); offset += len(word) + 2 {
				want = append(want,
					Token{Start: offset, ByteStart: offset, Type: "name"},
					Token{Start: offset + len(word), ByteStart: offset + len(word), Type: word},
					Token{Start: offset + len(word) + 1, ByteStart: offset + len(word) + 1, Type: "fallback"})
			}
			for range 20 {
				got, state, info := lx.TokenizeLineInfo(line, lx.Initial(), nil)
				if !equalTokenSlices(got, want) || state != lx.Initial() || info.Degraded {
					t.Errorf("worker %d: tokens=%v state=%v info=%+v; want %v", worker, got, state, info, want)
					return
				}
			}
		})
	}
	wg.Wait()
}
