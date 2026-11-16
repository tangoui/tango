//go:build !race

package monarch

import (
	"testing"
)

func TestHTMLTextAllocationBudget(t *testing.T) {
	lx := mustLexer(t, "html")
	assertLineAllocationBudget(t, lx, "hello &amp; world", "", 2)
}

func TestUnusedAttributeCapturesAllocationBudget(t *testing.T) {
	lx := addLexer(t, "unused-capture", `{"tokenizer":{"root":[["\"([^\"]*)\"","value"]]}}`)
	assertLineAllocationBudget(t, lx, `"hello"`, "value.unused-capture", 2)
}
