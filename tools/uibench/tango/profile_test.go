package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCPUProfileOptIn(t *testing.T) {
	if p := newCPUProfile(func(string) string { return "" }); p != nil {
		t.Fatalf("no %s: profile %+v, want none", profileEnv, p)
	}
	var none *cpuProfile
	none.start() // a nil profile is a no-op
	none.stop()
}

func TestCPUProfileWritesFromStartToStop(t *testing.T) {
	path := filepath.Join(t.TempDir(), "tango-list.pprof")
	p := newCPUProfile(func(k string) string {
		if k == profileEnv {
			return path
		}
		return ""
	})
	if p == nil {
		t.Fatal("no profile for a set " + profileEnv)
	}
	if _, err := os.Stat(path); err == nil {
		t.Fatal("profile file created before ready")
	}
	p.start()
	p.start() // once
	p.stop()
	p.stop()
	p.start() // never after stop
	st, err := os.Stat(path)
	if err != nil || st.Size() == 0 {
		t.Fatalf("profile file %v (err %v), want a non-empty pprof", st, err)
	}
}
