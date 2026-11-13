package main

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestConfigWithoutFrontend(t *testing.T) {
	dir := t.TempDir()
	const js = `{"name": "Ledger", "id": "com.example.ledger", "version": "1.0.0", "icon": "icon.png"}`
	if err := os.WriteFile(filepath.Join(dir, ConfigFile), []byte(js), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := LoadConfig(dir)
	if err != nil {
		t.Fatalf("self-drawn app config rejected: %v", err)
	}
	if cfg.HasFrontend() || cfg.FrontendDir() != "" {
		t.Fatalf("HasFrontend=%v FrontendDir=%q", cfg.HasFrontend(), cfg.FrontendDir())
	}
	if err := buildFrontend(context.Background(), cfg); err != nil {
		t.Fatalf("buildFrontend without a front end: %v", err)
	}
}

func TestConfigPartialFrontendStillChecked(t *testing.T) {
	dir := t.TempDir()
	const js = `{"name": "X", "id": "com.example.x", "version": "1.0.0", "frontend": {"dir": "web"}}`
	if err := os.WriteFile(filepath.Join(dir, ConfigFile), []byte(js), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadConfig(dir); err == nil || !strings.Contains(err.Error(), "frontend.dist") {
		t.Fatalf("missing frontend.dist not reported: %v", err)
	}
}
