//go:build darwin && !ios

package darwin_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"testing"

	"github.com/tangoui/tango/automation"
)

// The whole-app tests launch examples/duplex many times. automation.Launch
// would run `go build` (and link) for each launch; on a loaded machine that
// dominated the run time (TestLaneFault once took 282 s at load ~48, ~7 s
// of it the test itself). duplexApp builds the program once per test binary
// and every launch reuses the executable.

var duplexBin struct {
	once sync.Once
	dir  string
	path string
	err  error
	out  []byte
}

// duplexApp returns the path of examples/duplex built with -tags
// tango_dev (what automation needs), building it on first use. Like
// automation.Launch it skips the test unless TANGO_E2E is set, and with
// -short, before building anything.
func duplexApp(t *testing.T) string {
	t.Helper()
	if os.Getenv(automation.EnvE2E) == "" {
		t.Skipf("whole-app test: set %s=1 to run it (it opens windows)", automation.EnvE2E)
	}
	if testing.Short() {
		t.Skip("whole-app test skipped with -short")
	}
	b := &duplexBin
	b.once.Do(func() {
		if b.dir, b.err = os.MkdirTemp("", "tango-duplex-e2e-"); b.err != nil {
			return
		}
		b.path = filepath.Join(b.dir, "duplex")
		cmd := exec.Command("go", "build", "-tags", "tango_dev", "-o", b.path, "../../../examples/duplex")
		b.out, b.err = cmd.CombinedOutput()
	})
	if b.err != nil {
		t.Fatalf("go build examples/duplex: %v\n%s", b.err, b.out)
	}
	return b.path
}

func TestMain(m *testing.M) {
	code := m.Run()
	if duplexBin.dir != "" {
		os.RemoveAll(duplexBin.dir)
	}
	os.Exit(code)
}
