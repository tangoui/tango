package highlight

import (
	"github.com/tangoui/tango/ui/editor/monarch"
	"testing"
)

// The oracle must distinguish complete tokens from accepted timeout previews,
// even after the old helper's ten retries have elapsed.
func TestReferenceWaitsForCompleteTokens(t *testing.T) {
	calls := 0
	got, end, err := completeReferenceTokens(func() ([]monarch.Token, monarch.State, monarch.LineInfo) {
		calls++
		if calls <= 11 {
			return []monarch.Token{{Type: "partial"}}, 1, monarch.LineInfo{Degraded: true}
		}
		return []monarch.Token{{Type: "complete"}}, 2, monarch.LineInfo{}
	})
	if err != nil || len(got) != 1 || got[0].Type != "complete" || end != 2 {
		t.Fatalf("got %v, state %d, err %v after %d calls; reference must be complete", got, end, err, calls)
	}
}

func TestReferenceRejectsPersistentDegradation(t *testing.T) {
	calls := 0
	got, _, err := completeReferenceTokens(func() ([]monarch.Token, monarch.State, monarch.LineInfo) {
		calls++
		return []monarch.Token{{Type: "partial"}}, 1, monarch.LineInfo{Degraded: true}
	})
	if err == nil || got != nil || calls > 50 {
		t.Fatalf("got %v, err %v after %d calls: persistent degradation must fail, not become the oracle", got, err, calls)
	}
}
