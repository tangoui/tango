package main

import (
	"bytes"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
	"testing"
)

var updateGolden = flag.Bool("update", false, "rewrite testdata golden files")

// cliHelp returns the help text of tan and every command, the way
// "tan help" and "tan <command> -h" print it.
func cliHelp(t *testing.T) string {
	t.Helper()
	var b strings.Builder
	capture := func(args ...string) {
		out := captureStd(t, func() {
			if err := run(args); err != nil && !errors.Is(err, flag.ErrHelp) {
				t.Fatalf("tan %s: %v", strings.Join(args, " "), err)
			}
		})
		fmt.Fprintf(&b, "$ tan %s\n%s\n", strings.Join(args, " "), strings.TrimRight(out, "\n"))
	}
	capture("help")
	for _, c := range commands() {
		switch c.name {
		case "version":
			continue
		case "generate":
			b.WriteString("\n")
			capture("generate", "bindings", "-h")
		default:
			b.WriteString("\n")
			capture(c.name, "-h")
		}
	}
	return b.String()
}

// captureStd runs f with os.Stdout and os.Stderr sent to one buffer.
func captureStd(t *testing.T, f func()) string {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	stdout, stderr := os.Stdout, os.Stderr
	os.Stdout, os.Stderr = w, w
	done := make(chan []byte)
	go func() {
		data, _ := io.ReadAll(r)
		done <- data
	}()
	defer func() {
		os.Stdout, os.Stderr = stdout, stderr
	}()
	f()
	w.Close()
	return string(<-done)
}

// TestCLIGolden pins tan's commands and flags (docs/release.md, "CLI 与
// tan.json"): commands and flags are not removed or renamed; a changed
// default goes in CHANGELOG.md. Run "go test -run TestCLIGolden -update"
// after an intended change and review the diff.
func TestCLIGolden(t *testing.T) {
	got := cliHelp(t)
	const path = "testdata/cli.golden"
	if *updateGolden {
		if err := os.WriteFile(path, []byte(got), 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("%v (run go test -run TestCLIGolden -update)", err)
	}
	if !bytes.Equal(bytes.ReplaceAll(want, []byte("\r\n"), []byte("\n")), []byte(got)) {
		t.Errorf("tan help changed; if intended, run go test -run TestCLIGolden -update, review the diff and note it in CHANGELOG.md\n%s", lineDiff(string(want), got))
	}
}

// lineDiff lists the lines only in want (-) or only in got (+).
func lineDiff(want, got string) string {
	in := func(s string) map[string]bool {
		m := map[string]bool{}
		for _, l := range strings.Split(s, "\n") {
			m[l] = true
		}
		return m
	}
	w, g := in(want), in(got)
	var b strings.Builder
	for _, l := range strings.Split(want, "\n") {
		if !g[l] {
			fmt.Fprintf(&b, "- %s\n", l)
		}
	}
	for _, l := range strings.Split(got, "\n") {
		if !w[l] {
			fmt.Fprintf(&b, "+ %s\n", l)
		}
	}
	return b.String()
}
