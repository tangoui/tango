package main

import (
	"os"
	"strings"
	"testing"

	"github.com/tangoui/tango/ui"
)

// TestGalleryAccessibility audits every gallery page in the Ant Design,
// Element Plus and shadcn/ui styles (before and after each page's Setup
// interaction): no interactive node may lack an accessible name, no node
// may lack a role, every focusable control must be keyboard reachable, no
// required contrast pair may fail in the six built-in themes (design-system
// defaults and exempt pairs are listed, not counted) and none may stay below
// its minimum in their HighContrast variants. The full report (roles,
// keyboard reachability, contrast) is logged; run
// `go run ./examples/ui-gallery -audit` to print it.
func TestGalleryAccessibility(t *testing.T) {
	if testing.Short() {
		t.Skip("renders every gallery page three times")
	}
	res := auditGallery(os.Getenv("AUDIT_ONLY"))
	var b strings.Builder
	res.write(&b)
	t.Log(b.String())
	if n := res.missingNames(); n > 0 {
		t.Errorf("%d interactive nodes without an accessible name (see log)", n)
	}
	if n := res.count(ui.AuditNoRole); n > 0 {
		t.Errorf("%d nodes without a role (see log)", n)
	}
	if n := res.count(ui.AuditNotKeyboard); n > 0 {
		t.Errorf("%d controls not reachable by keyboard (see log)", n)
	}
	if def, hc := res.contrastFailures(); def > 0 || hc > 0 {
		t.Errorf("contrast: %d required pairs failing in the default themes, %d in the HighContrast themes (see log)", def, hc)
	}
}
