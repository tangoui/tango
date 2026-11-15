//go:build linux && !android

package gl

import (
	"errors"
	"os"
	"os/exec"
	"testing"

	"github.com/ebitengine/purego"
)

// Run in a minimal Linux image with EGL, Mesa and libepoxy installed, but
// without libGL/libOpenGL. An unavailable GPU must be an ordinary error,
// never libepoxy's SIGABRT (which prevents the host falling back to CPU).
func TestMissingDesktopGL(t *testing.T) {
	if os.Getenv("TANGO_TEST_MISSING_GL_CHILD") == "1" {
		d, err := NewDevice()
		if d != nil {
			d.Close()
		}
		if !errors.Is(err, ErrNoDevice) {
			t.Fatalf("NewDevice: %v, want ErrNoDevice", err)
		}
		return
	}
	for _, name := range []string{"libOpenGL.so.0", "libGL.so.1"} {
		if h, err := purego.Dlopen(name, purego.RTLD_NOW|purego.RTLD_LOCAL); err == nil {
			purego.Dlclose(h)
			t.Skip("desktop GL installed; regression requires minimal EGL-only image")
		}
	}
	for _, name := range []string{"libEGL.so.1", "libepoxy.so.0"} {
		h, err := purego.Dlopen(name, purego.RTLD_NOW|purego.RTLD_LOCAL)
		if err != nil {
			t.Skipf("%s absent: %v", name, err)
		}
		purego.Dlclose(h)
	}
	cmd := exec.Command(os.Args[0], "-test.run=^TestMissingDesktopGL$", "-test.v")
	cmd.Env = append(os.Environ(), "TANGO_TEST_MISSING_GL_CHILD=1")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("GPU initialization must return, not abort: %v\n%s", err, out)
	}
}
