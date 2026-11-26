//go:build darwin && !ios && tango_dev

package darwin

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"testing"
	"time"

	"github.com/ebitengine/purego"
	"github.com/ebitengine/purego/objc"
	"github.com/tangoui/tango/internal/platform"
)

// AppKit requires the initial OS thread, before the test runner starts.
func init() {
	if os.Getenv("TANGO_WEB_KEY_MENU_SUBPROCESS") != "1" {
		return
	}
	runtime.LockOSThread()
	if err := testNestedWebKeyMenu(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	os.Exit(0)
}

func TestNestedWebKeyMenu(t *testing.T) {
	if os.Getenv("TANGO_E2E") != "1" || testing.Short() {
		t.Skip("set TANGO_E2E=1 to test AppKit menu dispatch")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestNestedWebKeyMenu$")
	cmd.Env = append(os.Environ(), "TANGO_WEB_KEY_MENU_SUBPROCESS=1")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("native menu subprocess: %v\n%s", err, out)
	}
}

func testNestedWebKeyMenu() error {
	b := New().(*backend)
	if err := b.Init(&shortcutTestHost{}, platform.AppOptions{Name: "Tango nested menu test"}); err != nil {
		return err
	}
	var result error
	withPool(func() {
		var first, second id
		var firstCalls, secondCalls int
		classDef("TangoNestedWebKeyResponder", "NSResponder", nil, []objc.MethodDef{
			method("selectAll:", func(self id, _ objc.SEL, _ id) {
				if self == first {
					firstCalls++
				}
				if self == second {
					secondCalls++
				}
			}),
		})
		first = autorelease(send(alloc("TangoNestedWebKeyResponder"), "init"))
		second = autorelease(send(alloc("TangoNestedWebKeyResponder"), "init"))
		var makeKey func(id, objc.SEL, uint, nsPoint, uint, float64, int, id, id, id, bool, uint16) id
		purego.RegisterFunc(&makeKey, msgSendAddr)
		event := func(text string, code uint16) id {
			return makeKey(class("NSEvent"), sel("keyEventWithType:location:modifierFlags:timestamp:windowNumber:context:characters:charactersIgnoringModifiers:isARepeat:keyCode:"),
				10, nsPoint{}, nsCommandKeyMask, 0, 0, 0, nsString(text), nsString(text), false, code)
		}
		s := &surface{b: b}
		var nestedHandled bool
		b.SetAppMenu(0, []platform.MenuItem{
			{Label: "File", Submenu: []platform.MenuItem{{ID: 1, Label: "Save", Accelerator: "Cmd+S"}}},
			{Label: "Edit", Submenu: []platform.MenuItem{{Role: "selectAll"}}},
		}, func(uint32) {
			// A native menu action may run another main-thread request.
			nestedHandled = s.performWebKeyEquivalent(0, second, event("a", 0))
		})
		handled := s.performWebKeyEquivalent(0, first, event("s", 1))
		if !handled || !nestedHandled || firstCalls != 0 || secondCalls != 1 {
			result = fmt.Errorf("menu handled %v/%v, Select All called first=%d second=%d", handled, nestedHandled, firstCalls, secondCalls)
		}
	})
	return result
}
