//go:build tango_dev

package asset

import "testing"

func TestFromEnvDev(t *testing.T) {
	t.Setenv(DevURLEnv, "")
	if _, ok := FromEnv(testFS).(*fileHandler); !ok {
		t.Error("want FS handler without env")
	}
	t.Setenv(DevURLEnv, "http://localhost:5173")
	if _, ok := FromEnv(testFS).(*fileHandler); ok {
		t.Error("want proxy with env")
	}
	// Even in development builds the variable cannot point at another
	// machine.
	t.Setenv(DevURLEnv, "http://evil.example")
	if w := get(t, FromEnv(testFS), "GET", "/"); w.Code != 502 {
		t.Errorf("remote dev URL: %d, want 502", w.Code)
	}
}
