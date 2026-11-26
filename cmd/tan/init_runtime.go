package main

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"time"
)

var localRuntimeInputs = []string{"package.json", "package-lock.json", "build.mjs", "tsconfig.json"}

func localRuntimeBuilt(dir string) bool {
	_, ok := localRuntimeBuiltAt(dir)
	return ok
}

// localRuntimeBuiltAt reports the modification time of the oldest output.
func localRuntimeBuiltAt(dir string) (time.Time, bool) {
	var oldest time.Time
	for _, name := range []string{"index.js", "index.d.ts", "tango.iife.js"} {
		info, err := os.Stat(filepath.Join(dir, name))
		if err != nil || !info.Mode().IsRegular() {
			return time.Time{}, false
		}
		if oldest.IsZero() || info.ModTime().Before(oldest) {
			oldest = info.ModTime()
		}
	}
	return oldest, true
}

// localRuntimeCurrent reports whether dist was built after the last change
// to the runtime sources; dist is ignored by git and outlives a pull.
func localRuntimeCurrent(runtimeDir string) bool {
	built, ok := localRuntimeBuiltAt(filepath.Join(runtimeDir, "dist"))
	if !ok {
		return false
	}
	for _, name := range localRuntimeInputs {
		info, err := os.Stat(filepath.Join(runtimeDir, name))
		if err != nil || info.ModTime().After(built) {
			return false
		}
	}
	errStale := errors.New("stale")
	err := filepath.WalkDir(filepath.Join(runtimeDir, "src"), func(_ string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		if info.ModTime().After(built) {
			return errStale
		}
		return nil
	})
	return err == nil
}

// prepareLocalRuntime builds the frontend package in a scratch directory.
// The usual npm build also regenerates Go's embedded runtime, which project
// scaffolding must not write back into the checkout.
func prepareLocalRuntime(ctx context.Context, runtimeDir string) error {
	dist := filepath.Join(runtimeDir, "dist")
	if localRuntimeCurrent(runtimeDir) {
		return nil
	}
	stage, err := os.MkdirTemp(runtimeDir, ".tan-runtime-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(stage)

	for _, name := range localRuntimeInputs {
		data, err := os.ReadFile(filepath.Join(runtimeDir, name))
		if err != nil {
			return err
		}
		if err := os.WriteFile(filepath.Join(stage, name), data, 0o644); err != nil {
			return err
		}
	}
	if err := os.CopyFS(filepath.Join(stage, "src"), os.DirFS(filepath.Join(runtimeDir, "src"))); err != nil {
		return err
	}
	if err := os.Mkdir(filepath.Join(stage, "runtimejs"), 0o755); err != nil {
		return err
	}
	if err := runShell(ctx, stage, nil, "npm ci --ignore-scripts --no-audit --no-fund"); err != nil {
		return err
	}
	if err := runShell(ctx, stage, nil, "npm run build"); err != nil {
		return err
	}
	built := filepath.Join(stage, "dist")
	if !localRuntimeBuilt(built) {
		return fmt.Errorf("npm build did not produce the Tango runtime exports in %s", built)
	}
	// Replace a stale build; a concurrent init may have installed a
	// current one in the meantime.
	if localRuntimeCurrent(runtimeDir) {
		return nil
	}
	if err := os.RemoveAll(dist); err != nil {
		return err
	}
	if err := os.Rename(built, dist); err != nil && !localRuntimeCurrent(runtimeDir) {
		return err
	}
	return nil
}
