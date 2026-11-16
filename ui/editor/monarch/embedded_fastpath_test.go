package monarch

import "testing"

func TestFixedEmbeddedBoundaryRequiresSimpleState(t *testing.T) {
	lx := mustLexer(t, "html")
	root := lx.mustState(lx.Initial()).stack
	if got := lx.stackElem(root, "scriptEmbedded").boundary; got != "</script" {
		t.Fatalf("script boundary = %q, want </script", got)
	}
	if got := lx.stackElem(root, "styleEmbedded").boundary; got != "</style" {
		t.Fatalf("style boundary = %q, want </style", got)
	}

	custom := addLexer(t, "boundary-custom", `{"tokenizer":{"root":[["<",{"token":"x","next":"@embed","nextEmbedded":"javascript"}]],"embed":[["<\\/x",{"token":"@rematch","next":"@pop","nextEmbedded":"@pop"}],["[^<]+",""]]}}`)
	customRoot := custom.mustState(custom.Initial()).stack
	if got := custom.stackElem(customRoot, "embed").boundary; got != "</x" {
		t.Fatalf("custom boundary = %q, want </x", got)
	}

	complex := addLexer(t, "boundary-complex", `{"tokenizer":{"root":[["<",{"token":"x","next":"@embed","nextEmbedded":"javascript"}]],"embed":[["<\\/x",{"token":"@rematch","next":"@pop","nextEmbedded":"@pop"}],["foo","foo"],["[^<]+",""]]}}`)
	complexRoot := complex.mustState(complex.Initial()).stack
	if got := complex.stackElem(complexRoot, "embed").boundary; got != "" {
		t.Fatalf("complex boundary = %q, want generic fallback", got)
	}
}
