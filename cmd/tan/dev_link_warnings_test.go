package main

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
)

func TestDevGoBuildLinkWarnings(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("Apple linker diagnostics")
	}
	const deployment = "ld: warning: object file (/tmp/go-link/go.o) was built for newer 'macOS' version (13.0) than being linked (11.0)\n"
	const duplicate = "ld: warning: ignoring duplicate libraries: '-lobjc'\n"
	const other = "ld: warning: ignoring duplicate libraries: '-lm'\nld: warning: another warning\n"
	const failure = "ld: symbol(s) not found for architecture arm64"
	const full = deployment + duplicate + other + failure
	dir := t.TempDir()
	script := "#!/bin/sh\ncat >&2 <<'END'\n" + deployment + duplicate + other + "END\nprintf '%s' '" + failure + "' >&2\nexit 7\n"
	if err := os.WriteFile(filepath.Join(dir, "go"), []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	for _, tc := range []struct{ name, env, want string }{
		{"quiet by default", "", other + failure},
		{"raw diagnostics", "1", full},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("TANGO_DEV_RAW_BUILD_LOGS", tc.env)
			s := devSession{cfg: &Config{dir: dir}}
			var stderr bytes.Buffer
			err := s.goBuild(context.Background(), filepath.Join(dir, "app"), false, &stderr)
			if exit, ok := err.(*exec.ExitError); !ok || exit.ExitCode() != 7 {
				t.Fatalf("build error = %v, want exit 7", err)
			}
			if got := stderr.String(); got != tc.want {
				t.Fatalf("stderr = %q, want %q", got, tc.want)
			}
		})
	}
}
