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

func TestEndPanicKeepsServing(t *testing.T) {
	var seen tango.Panic
	var keys []*Session
	a, f := runInputMethod(t, tango.Options{OnPanic: func(p tango.Panic) bool {
		seen = p
		return false
	}})
	if err := Serve(a, Handler{
		Key: func(s *Session, _ Key) bool { keys = append(keys, s); return true },
		End: func(*Session) { panic("end panic") },
	}); err != nil {
		t.Fatal(err)
	}
	tango.Await(a, func() bool {
		f.sink.IMEKey(7, platform.IMEKey{})
		f.sink.IMEEnd(7)
		return f.sink.IMEKey(7, platform.IMEKey{})
	})
	if len(keys) != 2 || keys[0] == keys[1] {
		t.Fatalf("keys after End = %v", keys)
	}
	if seen.Value != "end panic" || seen.Where != "ime end" || len(seen.Stack) == 0 {
		t.Errorf("OnPanic received %+v", seen)
	}
}
