package main

import (
	"cmp"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"slices"
	"strings"
)

// Flatpak: tan writes a flatpak-builder manifest (JSON, the format in the
// flatpak-manifest(5) man page) whose single module installs the already
// built program and its desktop files from a local "dir" source, and, when
// flatpak-builder is installed, builds a repository and a single-file
// .flatpak bundle from it.
//
// Exported files must carry the application ID (Flatpak documentation,
// "Desktop integration"): the desktop file is <id>.desktop with Icon=<id>,
// icons <id>.png and <id>-symbolic.svg, MIME definitions <id>.xml.

// defaultFlatpakRuntime is the org.gnome.Platform branch the manifest
// uses unless "linux.flatpak.runtimeVersion" names another. The GNOME
// runtime ships GTK 3 and WebKitGTK 4.1. GNOME branches are supported for
// about a year; Flathub served 49, 50 and 51 when this was set.
const defaultFlatpakRuntime = "50"

// flatpakFinishArgs are the default sandbox permissions: the X11 shared
// memory (ipc), Wayland with X11 as a fallback, GPU access (dri) for
// WebKit's compositing, and the network, which web content usually needs.
// The file system is not shared: GTK's file chooser goes through the
// document portal.
var flatpakFinishArgs = []string{"--share=ipc", "--socket=wayland", "--socket=fallback-x11", "--device=dri", "--share=network"}

type flatpakManifest struct {
	ID             string          `json:"id"`
	Runtime        string          `json:"runtime"`
	RuntimeVersion string          `json:"runtime-version"`
	SDK            string          `json:"sdk"`
	Command        string          `json:"command"`
	FinishArgs     []string        `json:"finish-args"`
	Modules        []flatpakModule `json:"modules"`
}

type flatpakModule struct {
	Name          string          `json:"name"`
	Buildsystem   string          `json:"buildsystem"`
	BuildCommands []string        `json:"build-commands"`
	Sources       []flatpakSource `json:"sources"`
}

type flatpakSource struct {
	Type string `json:"type"`
	Path string `json:"path"`
}

// flatpakArch maps GOARCH to the Flatpak architecture.
func flatpakArch(goarch string) string {
	return map[string]string{"amd64": "x86_64", "arm64": "aarch64", "386": "i386", "arm": "arm"}[goarch]
}

var desktopIconLine = regexp.MustCompile(`(?m)^Icon=.*$`)

// flatpakTree lists the files of the "dir" source, keyed by their path under
// /app: bin/<exe>, then the exported files named after the app id.
func flatpakTree(cfg *Config, icons map[string][]byte) map[string][]byte {
	exe, id := cfg.Exe(), cfg.ID
	tree := map[string][]byte{
		"share/applications/" + id + ".desktop": []byte(desktopIconLine.ReplaceAllLiteralString(desktopEntry(cfg), "Icon="+id)),
	}
	for p, b := range icons {
		// icons/hicolor/<dir>/apps/<exe>[-symbolic].<ext> → <id>[-symbolic].<ext>
		dir, file := filepath.Split(p)
		tree["share/"+dir+id+strings.TrimPrefix(file, exe)] = b
	}
	if x := mimePackage(cfg); x != nil {
		tree["share/mime/packages/"+id+".xml"] = x
	}
	return tree
}

// flatpakManifestFor renders the manifest for the tree from flatpakTree.
func flatpakManifestFor(cfg *Config, tree map[string][]byte) flatpakManifest {
	exe := cfg.Exe()
	cmds := []string{"install -Dm755 bin/" + exe + " ${FLATPAK_DEST}/bin/" + exe}
	for _, p := range sortedKeys(tree) {
		cmds = append(cmds, "install -Dm644 "+p+" ${FLATPAK_DEST}/"+p)
	}
	args := append([]string(nil), flatpakFinishArgs...)
	for _, a := range cfg.Linux.Flatpak.FinishArgs {
		if !slices.Contains(args, a) {
			args = append(args, a)
		}
	}
	return flatpakManifest{
		ID:             cfg.ID,
		Runtime:        "org.gnome.Platform",
		RuntimeVersion: cmp.Or(cfg.Linux.Flatpak.RuntimeVersion, defaultFlatpakRuntime),
		SDK:            "org.gnome.Sdk",
		Command:        exe,
		FinishArgs:     args,
		Modules: []flatpakModule{{
			Name:          linuxPackageName(cfg),
			Buildsystem:   "simple",
			BuildCommands: cmds,
			Sources:       []flatpakSource{{Type: "dir", Path: "files"}},
		}},
	}
}

// packageFlatpak writes <Exe>-<arch>.flatpak/ next to the build directory:
// the manifest <id>.json and the module's files/. With flatpak-builder on
// PATH (Linux) it builds the repository there and <base>.flatpak beside it.
func packageFlatpak(ctx context.Context, cfg *Config, a artifact, icons map[string][]byte) ([]string, error) {
	parent := filepath.Dir(a.dir)
	dir := filepath.Join(parent, cfg.Exe()+"-"+a.target.Arch+".flatpak")
	os.RemoveAll(dir)
	files := filepath.Join(dir, "files")
	if err := os.MkdirAll(filepath.Join(files, "bin"), 0o755); err != nil {
		return nil, err
	}
	if err := copyFile(a.exe, filepath.Join(files, "bin", cfg.Exe()), 0o755); err != nil {
		return nil, err
	}
	tree := flatpakTree(cfg, icons)
	for p, b := range tree {
		dst := filepath.Join(files, filepath.FromSlash(p))
		if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
			return nil, err
		}
		if err := os.WriteFile(dst, b, 0o644); err != nil {
			return nil, err
		}
	}
	m := flatpakManifestFor(cfg, tree)
	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return nil, err
	}
	manifest := filepath.Join(dir, cfg.ID+".json")
	if err := os.WriteFile(manifest, append(data, '\n'), 0o644); err != nil {
		return nil, err
	}
	made := []string{manifest}

	arch := flatpakArch(a.target.Arch)
	bundle := filepath.Join(parent, artifactBase(cfg, a.target)+".flatpak")
	repo, buildDir := filepath.Join(dir, "repo"), filepath.Join(dir, "build")
	builderArgs := []string{"--arch=" + arch, "--force-clean", "--repo=" + repo, buildDir, manifest}
	bundleArgs := []string{"build-bundle", "--arch=" + arch, repo, bundle, cfg.ID}
	hint := fmt.Sprintf("flatpak install --user flathub org.gnome.Platform//%[1]s org.gnome.Sdk//%[1]s\n  flatpak-builder %[2]s\n  flatpak %[3]s",
		m.RuntimeVersion, strings.Join(builderArgs, " "), strings.Join(bundleArgs, " "))
	builder, err := exec.LookPath("flatpak-builder")
	if runtime.GOOS != "linux" || err != nil {
		warnf("flatpak-builder is not on PATH: wrote %s; on Linux build the bundle with\n  %s", manifest, hint)
		return made, nil
	}
	if err := runIn(ctx, dir, nil, builder, builderArgs...); err != nil {
		return made, fmt.Errorf("%w (the runtime and SDK must be installed:\n  %s)", err, hint)
	}
	if err := runIn(ctx, dir, nil, "flatpak", bundleArgs...); err != nil {
		return made, err
	}
	return append(made, bundle), nil
}
