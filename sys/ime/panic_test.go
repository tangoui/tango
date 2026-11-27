package ime

import (
	"testing"

	"github.com/tangoui/tango"
	"github.com/tangoui/tango/internal/platform"
)

func TestKeyPanicPassesThrough(t *testing.T) {
	var seen tango.Panic
	a, f := runInputMethod(t, tango.Options{OnPanic: func(p tango.Panic) bool {
		seen = p
		return false
	}})
	if err := Serve(a, Handler{Key: func(*Session, Key) bool { panic("boom") }}); err != nil {
		t.Fatal(err)
	}
	if tango.Await(a, func() bool { return f.sink.IMEKey(7, platform.IMEKey{}) }) {
		t.Fatal("panicking key handler consumed the key")
	}
	if seen.Value != "boom" || seen.Where != "ime key" || len(seen.Stack) == 0 {
		t.Errorf("OnPanic received %+v", seen)
	}
}
