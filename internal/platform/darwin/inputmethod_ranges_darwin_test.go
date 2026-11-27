//go:build darwin && !ios

package darwin

import (
	"os"
	"os/exec"
	"runtime"
	"testing"
)

const imeRangeProbe = "TANGO_TEST_IME_NATIVE_RANGES"

func init() {
	if os.Getenv(imeRangeProbe) != "1" {
		return
	}
	runtime.LockOSThread()
	previous := MainThreadTestHook
	MainThreadTestHook = func() (int, bool) {
		if previous != nil {
			if code, handled := previous(); handled {
				return code, true
			}
		}
		runNativeIMERanges()
		return 0, true
	}
}

func TestIMENativeRanges(t *testing.T) {
	cmd := exec.Command(os.Args[0])
	cmd.Env = append(os.Environ(), imeRangeProbe+"=1")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("native marked-text call: %v\n%s", err, out)
	}
}

func runNativeIMERanges() {
	withPool(func() {
		if err := loadIME(); err != nil {
			panic(err)
		}
		send(class("NSApplication"), "sharedApplication")
		view := alloc("NSTextView")
		defer release(view)
		send(view, "setString:", uintptr(nsString("prefix suffix")))
		// NSTextView uses selectedRange: where InputMethodKit uses
		// selectionRange:. Both selectors use the same two NSRange arguments.
		imeRuntime.mark(view, sel("setMarkedText:selectedRange:replacementRange:"),
			nsString("你好"), nsRange{Location: 2}, nsRange{Location: 7, Length: 6})
		if got := goString(send(view, "string")); got != "prefix 你好" {
			panic("unexpected text after marking replacement range: " + got)
		}
	})
}
