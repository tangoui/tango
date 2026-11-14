package uihost

import (
	"reflect"
	"runtime"
	"testing"

	"github.com/tangoui/tango/internal/devhost"
)

// TestAttachSymbol pins the symbol tan dev looks for in a built app to tell
// whether it attaches canvases to WebView windows (cmd/tan/dev_hot.go).
func TestAttachSymbol(t *testing.T) {
	got := runtime.FuncForPC(reflect.ValueOf(AttachWith).Pointer()).Name()
	if got != devhost.AttachSymbol {
		t.Fatalf("AttachWith is %q, devhost.AttachSymbol is %q: update the constant (tan dev uses it)", got, devhost.AttachSymbol)
	}
}
