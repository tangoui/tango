//go:build !tango_dev

package devfeat_test

import (
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/tangoui/tango"
	"github.com/tangoui/tango/internal/platform/fake/faketest"
)

// In release builds TANGO_AUTOMATION is ignored: no control channel, no
// token file.
func TestAutomationReleaseOff(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := ln.Addr().String()
	ln.Close() // a free port the app would take if it listened
	tokenFile := filepath.Join(t.TempDir(), "auto.json")
	t.Setenv("TANGO_AUTOMATION", addr)
	t.Setenv("TANGO_AUTOMATION_TOKEN_FILE", tokenFile)
	faketest.Run(t, tango.Options{})
	time.Sleep(100 * time.Millisecond)
	if _, err := os.Stat(tokenFile); err == nil {
		t.Fatal("release build wrote a token file")
	}
	if c, err := net.DialTimeout("tcp", addr, 200*time.Millisecond); err == nil {
		c.Close()
		t.Fatal("release build listens on TANGO_AUTOMATION")
	}
	if os.Getenv("TANGO_AUTOMATION") != addr {
		t.Fatal("release build touched the environment")
	}
}
