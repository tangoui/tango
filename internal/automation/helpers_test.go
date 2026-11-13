package automation

import (
	"os"
	"testing"
)

// shortTempDir returns a directory with a short path (unix socket paths
// are limited to about 100 bytes; t.TempDir on macOS is longer).
func shortTempDir(t *testing.T) (string, error) {
	d, err := os.MkdirTemp("/tmp", "ta")
	if err != nil {
		return "", err
	}
	t.Cleanup(func() { os.RemoveAll(d) })
	return d, nil
}
