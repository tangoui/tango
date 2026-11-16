package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestStartupDocumentUsesReadableSample(t *testing.T) {
	name, content, err := startupDocument("", 10000, false)
	if err != nil {
		t.Fatal(err)
	}
	if name != "welcome.ts" || strings.Count(content, "\n") > 70 || !strings.Contains(content, "Welcome to Tango") {
		t.Fatalf("unexpected default sample %q (%d lines)", name, strings.Count(content, "\n"))
	}
}
func TestStartupDocumentPreservesExplicitGenerator(t *testing.T) {
	name, content, err := startupDocument("", 123, true)
	if err != nil {
		t.Fatal(err)
	}
	if name != "generated.ts" || content != genTS(123) {
		t.Fatal("explicit generator changed")
	}
}
func TestStartupDocumentPrefersFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "hello.go")
	if err := os.WriteFile(path, []byte("package main\r\n"), 0600); err != nil {
		t.Fatal(err)
	}
	name, content, err := startupDocument(path, 123, true)
	if err != nil || name != path || content != "package main\r\n" {
		t.Fatalf("file=%q content=%q err=%v", name, content, err)
	}
	if _, _, err := startupDocument(path+"-missing", 1, false); err == nil {
		t.Fatal("missing file accepted")
	}
}
