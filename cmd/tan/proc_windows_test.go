package main

import (
	"strings"
	"testing"
)

// TestShellCommandQuotes: a command line with double quotes reaches
// cmd.exe unchanged (os/exec's own quoting would turn " into \").
func TestShellCommandQuotes(t *testing.T) {
	for line, want := range map[string]string{
		`echo "hello world"`: `"hello world"`,
		`powershell -NoProfile -Command "Write-Output 'a b'"`: `a b`,
	} {
		out, err := shellCommand(line).CombinedOutput()
		if got := strings.TrimSpace(string(out)); err != nil || got != want {
			t.Errorf("%s: %q %v, want %q", line, got, err, want)
		}
	}
}
