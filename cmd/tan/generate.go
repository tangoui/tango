package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/tangoui/tango/cmd/tan/internal/bindgen"
)

func runGenerate(args []string) error {
	if len(args) == 0 || args[0] != "bindings" {
		fmt.Fprintln(os.Stderr, "Usage: tan generate bindings [flags]")
		return errors.New("generate: the only generator is bindings")
	}
	fs := newFlags("generate bindings", "[flags]")
	dir := projectFlag(fs)
	out := fs.String("o", "", "output `dir` (default <frontend.dir>/src/bindings, or bindings without tan.json)")
	pkg := fs.String("pkg", ".", "Go package `dir` with the bind.Register calls, relative to the project")
	rt := fs.String("runtime", "@tangoui/runtime", "`module` that exports call")
	if err := fs.Parse(args[1:]); err != nil {
		return err
	}
	root, err := filepath.Abs(*dir)
	if err != nil {
		return err
	}
	outDir := *out
	if outDir == "" {
		outDir = filepath.Join(root, "bindings")
		if cfg, err := LoadConfig(root); err == nil && cfg.HasFrontend() {
			outDir = filepath.Join(cfg.FrontendDir(), "src", "bindings")
		}
	} else if !filepath.IsAbs(outDir) {
		outDir = filepath.Join(root, outDir)
	}

	res, err := bindgen.Analyze(filepath.Join(root, *pkg))
	if err != nil {
		return err
	}
	for _, w := range res.Warnings {
		warnf("%s", w)
	}
	files, err := bindgen.Generate(res, *rt)
	if err != nil {
		return err
	}
	if err := writeGenerated(outDir, files); err != nil {
		return err
	}
	logf("Wrote bindings for %d service(s) to %s", len(res.Services), outDir)
	return nil
}

// writeGenerated replaces the generated .ts files in dir (those starting
// with bindgen.Header) with files, leaving anything else alone.
func writeGenerated(dir string, files map[string][]byte) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".ts") || files[e.Name()] != nil {
			continue
		}
		p := filepath.Join(dir, e.Name())
		if b, err := os.ReadFile(p); err == nil && strings.HasPrefix(string(b), bindgen.Header) {
			os.Remove(p)
		}
	}
	for name, b := range files {
		if err := os.WriteFile(filepath.Join(dir, name), b, 0o644); err != nil {
			return err
		}
	}
	return nil
}
