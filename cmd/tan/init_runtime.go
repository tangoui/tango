package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
)

func localRuntimeBuilt(dir string) bool {
	for _, name := range []string{"index.js", "index.d.ts", "tango.iife.js"} {
		info, err := os.Stat(filepath.Join(dir, name))
		if err != nil || !info.Mode().IsRegular() {
			return false
		}
	}
	return true
}

// prepareLocalRuntime builds the frontend package in a scratch directory.
// The usual npm build also regenerates Go's embedded runtime, which project
// scaffolding must not write back into the checkout.
func prepareLocalRuntime(ctx context.Context, runtimeDir string) error {
	dist := filepath.Join(runtimeDir, "dist")
	if localRuntimeBuilt(dist) {
		return nil
	}
	stage, err := os.MkdirTemp(runtimeDir, ".tan-runtime-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(stage)

	for _, name := range []string{"package.json", "package-lock.json", "build.mjs", "tsconfig.json"} {
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
	if err := os.Rename(built, dist); err != nil && !localRuntimeBuilt(dist) {
		return err
	}
	return nil
}
