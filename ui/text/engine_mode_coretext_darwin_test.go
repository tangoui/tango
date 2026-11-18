//go:build darwin && !ios && tango_coretext

package text

import "testing"

func TestEngineIsCoreText(t *testing.T) {
	if e := Engine(); e != "coretext" {
		t.Fatalf("Engine() = %q, want coretext", e)
	}
}

func TestUseSystemEngineFalseIsNoop(t *testing.T) {
	v := reg.version
	if prev := UseSystemEngine(false); !prev {
		t.Fatal("UseSystemEngine(false) returned false, want true")
	}
	t.Cleanup(func() { UseSystemEngine(true) })
	if e := Engine(); e != "coretext" {
		t.Fatalf("after UseSystemEngine(false): Engine() = %q, want coretext", e)
	}
	if prev := UseSystemEngine(true); !prev {
		t.Fatal("UseSystemEngine(true) after (false) returned false: the switch changed state")
	}
	if reg.version != v {
		t.Fatalf("reg.version %d -> %d: the no-op switch dropped the caches", v, reg.version)
	}
}
