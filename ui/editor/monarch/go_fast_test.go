package monarch

import "testing"

func TestManualGoLineSeparatesAdjacentNumbersAndOperators(t *testing.T) {
	toks, ok := manualGoLine("x = 1+2", nil)
	if !ok {
		t.Fatal("manual Go path rejected a basic expression")
	}
	if len(toks) != 7 {
		t.Fatalf("got %v tokens, want 7", toks)
	}
	if toks[4].ByteStart != 4 || toks[4].Type != "number.go" || toks[5].ByteStart != 5 || toks[5].Type != "delimiter.go" || toks[6].ByteStart != 6 || toks[6].Type != "number.go" {
		t.Fatalf("number/operator split = %v", toks)
	}
}
