package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"html"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
)

// target is a GOOS/GOARCH pair. Arch "universal" (darwin only) builds
// amd64 and arm64 and joins them with lipo.
type target struct{ OS, Arch string }

func (t target) String() string { return t.OS + "/" + t.Arch }

var hostTarget = target{runtime.GOOS, runtime.GOARCH}

// parseTargets parses "darwin/arm64,windows/amd64". An OS alone means the
// host architecture. Empty means the host. Mobile targets (ios,
// ios-simulator, android[/arch]) are parsed by parseMobileTarget.
func parseTargets(s string) ([]target, error) {
	if strings.TrimSpace(s) == "" {
		return []target{hostTarget}, nil
	}
	var ts []target
	for _, part := range strings.Split(s, ",") {
		part = strings.TrimSpace(part)
		goos, arch, ok := strings.Cut(part, "/")
		if isMobileOS(goos) {
			mts, err := parseMobileTarget(goos, arch, ok)
			if err != nil {
				return nil, fmt.Errorf("%s: %w", part, err)
			}
			for _, t := range mts {
				if !slices.Contains(ts, t) {
					ts = append(ts, t)
				}
			}
			continue
		}
		if !ok {
			arch = runtime.GOARCH
		}
		t := target{goos, arch}
		switch goos {
		case "darwin", "windows", "linux":
		default:
			return nil, fmt.Errorf("%s: unsupported OS (want darwin, windows, linux, ios, ios-simulator or android)", part)
		}
		switch {
		case arch == "amd64" || arch == "arm64":
		case arch == "universal" && goos == "darwin":
		default:
			return nil, fmt.Errorf("%s: unsupported architecture", part)
		}
		if !slices.Contains(ts, t) {
			ts = append(ts, t)
		}
	}
	return ts, nil
}

type buildOptions struct {
	targets []target
	out     string // output root; each target goes to <out>/<os>-<arch>
	debug   bool
	xcode   bool // ios: also write an Xcode project
}

func runBuild(args []string) error {
	fs := newFlags("build", "[flags]")
	dir := projectFlag(fs)
	platform := fs.String("platform", "", "comma-separated `targets` such as darwin/arm64,windows/amd64,linux/amd64 (default: this machine; darwin/universal joins both Mac architectures); mobile: ios, ios-simulator, android, android/arm64,android/amd64")
	out := fs.String("o", "build", "output `dir`")
	debug := fs.Bool("debug", false, "keep symbols and the console window; let the page open the inspector; android: debug APK instead of the signed release")
	xcode := fs.Bool("xcode", false, "ios: also generate an Xcode project in <out>/xcode for debugging")
	if err := fs.Parse(args); err != nil {
		return err
	}
	cfg, err := LoadConfig(*dir)
	if err != nil {
		return err
	}
	ts, err := parseTargets(*platform)
	if err != nil {
		return err
	}
	_, err = build(context.Background(), cfg, buildOptions{targets: ts, out: *out, debug: *debug, xcode: *xcode})
	return err
}

// artifact is one target's build output.
type artifact struct {
	target target
	dir    string   // <out>/<os>-<arch>
	path   string   // the .app bundle, the executable, or the APK
	exe    string   // the executable itself
	extra  []string // more outputs (.ipa, .aab)
}

// build runs the front-end build once, then builds every target.
func build(ctx context.Context, cfg *Config, o buildOptions) ([]artifact, error) {
	desktop, mobile := splitTargets(o.targets)
	if err := checkMobileBuild(cfg, mobile, o); err != nil {
		return nil, err
	}
	if err := buildFrontend(ctx, cfg); err != nil {
		return nil, err
	}
	out := o.out
	if !filepath.IsAbs(out) {
		out = cfg.Path(out)
	}
	arts, err := buildMobile(ctx, cfg, mobile, out, o)
	if err != nil {
		return arts, err
	}
	for _, t := range desktop {
		logf("Building %s %s for %s", cfg.Name, cfg.Version, t)
		dir := filepath.Join(out, t.OS+"-"+t.Arch)
		a, err := buildTarget(ctx, cfg, t, dir, o.debug)
		if err != nil {
			os.RemoveAll(dir)
			return arts, fmt.Errorf("%s: %w", t, err)
		}
		logf("  → %s", a.path)
		arts = append(arts, a)
	}
	return arts, nil
}

func buildTarget(ctx context.Context, cfg *Config, t target, dir string, debug bool) (artifact, error) {
	a := artifact{target: t, dir: dir}
	if err := os.RemoveAll(dir); err != nil {
		return a, err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return a, err
	}
	switch t.OS {
	case "darwin":
		a.path = filepath.Join(dir, cfg.Name+".app")
		a.exe = filepath.Join(a.path, "Contents", "MacOS", cfg.Exe())
		if err := goBuildDarwin(ctx, cfg, t.Arch, a.exe, debug); err != nil {
			return a, err
		}
		if err := writeBundle(cfg, a.path); err != nil {
			return a, err
		}
		return a, adhocSign(ctx, a.path)
	case "windows":
		// The icon, manifest and version information go in as a .syso
		// that exists only while go build runs (winres.go).
		a.exe = filepath.Join(dir, cfg.Exe()+".exe")
		a.path = a.exe
		return a, withWindowsResources(cfg, t.Arch, func() error {
			return goBuild(ctx, cfg, t, a.exe, debug)
		})
	default:
		a.exe = filepath.Join(dir, cfg.Exe())
	}
	a.path = a.exe
	return a, goBuild(ctx, cfg, t, a.exe, debug)
}

func goBuildDarwin(ctx context.Context, cfg *Config, arch, exe string, debug bool) error {
	if arch != "universal" {
		return goBuild(ctx, cfg, target{"darwin", arch}, exe, debug)
	}
	if _, err := exec.LookPath("lipo"); err != nil {
		return errors.New("darwin/universal needs lipo (Xcode command line tools)")
	}
	var parts []string
	for _, arch := range []string{"amd64", "arm64"} {
		p := exe + "-" + arch
		if err := goBuild(ctx, cfg, target{"darwin", arch}, p, debug); err != nil {
			return err
		}
		defer os.Remove(p)
		parts = append(parts, p)
	}
	return runIn(ctx, "", nil, "lipo", append([]string{"-create", "-output", exe}, parts...)...)
}

// goBuildArgs returns the go build arguments for a release (or debug)
// build of the project's main package.
func goBuildArgs(t target, out string, debug bool) []string {
	args := []string{"build", "-tags", buildTags(false)} // build_tags.go
	var ldflags []string
	if !debug {
		args = append(args, "-trimpath")
		ldflags = append(ldflags, "-s", "-w")
		if t.OS == "windows" {
			ldflags = append(ldflags, "-H", "windowsgui")
		}
	}
	if len(ldflags) > 0 {
		args = append(args, "-ldflags", strings.Join(ldflags, " "))
	}
	return append(args, "-o", out, ".")
}

// goBuild compiles without cgo: the desktop backends call the OS through
// purego/syscall, so cross-compiling needs no C toolchain.
func goBuild(ctx context.Context, cfg *Config, t target, out string, debug bool) error {
	env := []string{"CGO_ENABLED=0", "GOOS=" + t.OS, "GOARCH=" + t.Arch}
	return runIn(ctx, cfg.dir, env, "go", goBuildArgs(t, out, debug)...)
}

// buildFrontend installs dependencies when needed and runs frontend.build.
func buildFrontend(ctx context.Context, cfg *Config) error {
	if !cfg.HasFrontend() {
		return nil // self-drawn UI app
	}
	if err := installFrontend(ctx, cfg); err != nil {
		return err
	}
	if cfg.Frontend.Build != "" {
		logf("Building the front end: %s", cfg.Frontend.Build)
		if err := runShell(ctx, cfg.FrontendDir(), nil, cfg.Frontend.Build); err != nil {
			return fmt.Errorf("front-end build: %w", err)
		}
	}
	if st, err := os.Stat(cfg.DistDir()); err != nil || !st.IsDir() {
		return fmt.Errorf("front-end output %s is missing", cfg.DistDir())
	}
	return nil
}

// installFrontend runs frontend.install (default npm install) when the
// front end has a package.json but no node_modules yet.
func installFrontend(ctx context.Context, cfg *Config) error {
	dir := cfg.FrontendDir()
	if _, err := os.Stat(filepath.Join(dir, "package.json")); err != nil {
		return nil
	}
	if _, err := os.Stat(filepath.Join(dir, "node_modules")); err == nil {
		return nil
	}
	line := cfg.Frontend.Install
	if line == "" {
		line = "npm install"
	}
	logf("Installing front-end dependencies: %s", line)
	if err := runShell(ctx, dir, nil, line); err != nil {
		return fmt.Errorf("front-end install: %w", err)
	}
	return nil
}

// writeBundle completes a macOS .app around its executable: Info.plist,
// PkgInfo and, when the tools are there, the icon.
func writeBundle(cfg *Config, app string) error {
	contents := filepath.Join(app, "Contents")
	res := filepath.Join(contents, "Resources")
	if err := os.MkdirAll(res, 0o755); err != nil {
		return err
	}
	icon := ""
	if cfg.Icon != "" || cfg.Icons.MacOS != "" {
		// "icons.macos" as is, or the desktop shape derived from the
		// master icon (icon_desktop.go).
		png, cleanup, err := macOSIconFile(cfg)
		if err == nil {
			defer cleanup()
			err = makeICNS(png, filepath.Join(res, "icon.icns"))
		}
		switch {
		case err == nil:
			icon = "icon"
		case errors.Is(err, errNoIconTools):
			warnf("No sips/iconutil here; the app has no icon")
		default:
			warnf("Icon skipped: %v", err)
		}
	}
	if err := os.WriteFile(filepath.Join(contents, "Info.plist"), infoPlist(cfg, icon), 0o644); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(contents, "PkgInfo"), []byte("APPL????"), 0o644)
}

// adhocSign signs a bundle ad hoc, on macOS, so its code signature covers
// Info.plist and names the bundle id: the Go linker signs only the bare
// executable (as "a.out"). tan sign replaces this with a real identity.
func adhocSign(ctx context.Context, app string) error {
	if runtime.GOOS != "darwin" || !lookTool("codesign") {
		return nil
	}
	out, err := exec.CommandContext(ctx, "codesign", "--force", "--sign", "-", app).CombinedOutput()
	if err != nil {
		return fmt.Errorf("codesign: %v: %s", err, out)
	}
	return nil
}

// macLocalizations is CFBundleLocalizations: the languages the app declares.
// AppKit localizes the items it adds or names itself (Undo/Redo, Start
// Dictation, Emoji & Symbols, Enter Full Screen, ...) only into languages
// the bundle declares; without the key a Chinese system still gets English
// there. These are the languages i18n's rolenames ship (docs/specs/i18n.md);
// tan.json has no field for them yet.
var macLocalizations = []string{"en", "zh-Hans", "zh-Hant", "ja"}

// infoPlist renders Contents/Info.plist. icon is the .icns base name in
// Resources, or "" for none.
func infoPlist(cfg *Config, icon string) []byte {
	minOS := cfg.MacOS.MinVersion
	if minOS == "" {
		minOS = "11.0"
	}
	type kv struct{ k, v string }
	entries := []kv{
		{"CFBundleDevelopmentRegion", "en"},
		{"CFBundleDisplayName", cfg.Name},
		{"CFBundleExecutable", cfg.Exe()},
		{"CFBundleIdentifier", cfg.ID},
		{"CFBundleInfoDictionaryVersion", "6.0"},
		{"CFBundleName", cfg.Name},
		{"CFBundlePackageType", "APPL"},
		{"CFBundleShortVersionString", cfg.Version},
		{"CFBundleVersion", cfg.Version},
		{"LSMinimumSystemVersion", minOS},
	}
	if icon != "" {
		entries = append(entries, kv{"CFBundleIconFile", icon})
	}
	if cfg.Copyright != "" {
		entries = append(entries, kv{"NSHumanReadableCopyright", cfg.Copyright})
	}
	slices.SortFunc(entries, func(a, b kv) int { return strings.Compare(a.k, b.k) })

	var b bytes.Buffer
	b.WriteString(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
`)
	for _, e := range entries {
		fmt.Fprintf(&b, "\t<key>%s</key>\n\t<string>%s</string>\n", e.k, html.EscapeString(e.v))
	}
	b.WriteString("\t<key>NSHighResolutionCapable</key>\n\t<true/>\n")
	plistStrings(&b, "\t", "CFBundleLocalizations", cfg.AppleLocalizations())
	writeURLTypes(&b, cfg)
	writeDocumentTypes(&b, cfg)
	b.WriteString("</dict>\n</plist>\n")
	return b.Bytes()
}

// writeURLTypes declares the deep-link schemes (CFBundleURLTypes), one
// entry named after the bundle id listing every scheme.
func writeURLTypes(b *bytes.Buffer, cfg *Config) {
	if len(cfg.URLSchemes) == 0 {
		return
	}
	b.WriteString("\t<key>CFBundleURLTypes</key>\n\t<array>\n\t\t<dict>\n")
	fmt.Fprintf(b, "\t\t\t<key>CFBundleURLName</key>\n\t\t\t<string>%s</string>\n", html.EscapeString(cfg.ID))
	b.WriteString("\t\t\t<key>CFBundleTypeRole</key>\n\t\t\t<string>Viewer</string>\n")
	plistStrings(b, "\t\t\t", "CFBundleURLSchemes", cfg.URLSchemes)
	b.WriteString("\t\t</dict>\n\t</array>\n")
}

// writeDocumentTypes declares the file associations (CFBundleDocumentTypes);
// Launch Services also uses them for the Open Recent menu. Editors rank
// Default (the app may be the default handler), viewers Alternate.
func writeDocumentTypes(b *bytes.Buffer, cfg *Config) {
	if len(cfg.FileAssociations) == 0 {
		return
	}
	b.WriteString("\t<key>CFBundleDocumentTypes</key>\n\t<array>\n")
	for _, f := range cfg.FileAssociations {
		role, rank := "Editor", "Default"
		if f.Viewer() {
			role, rank = "Viewer", "Alternate"
		}
		b.WriteString("\t\t<dict>\n")
		for _, kv := range [][2]string{{"CFBundleTypeName", f.Name}, {"CFBundleTypeRole", role}, {"LSHandlerRank", rank}} {
			fmt.Fprintf(b, "\t\t\t<key>%s</key>\n\t\t\t<string>%s</string>\n", kv[0], html.EscapeString(kv[1]))
		}
		plistStrings(b, "\t\t\t", "CFBundleTypeExtensions", []string{f.Ext})
		plistStrings(b, "\t\t\t", "CFBundleTypeMIMETypes", []string{f.MIMEType(cfg)})
		b.WriteString("\t\t</dict>\n")
	}
	b.WriteString("\t</array>\n")
}

// plistStrings writes key and an array of strings at indent.
func plistStrings(b *bytes.Buffer, indent, key string, vals []string) {
	fmt.Fprintf(b, "%s<key>%s</key>\n%s<array>\n", indent, key, indent)
	for _, v := range vals {
		fmt.Fprintf(b, "%s\t<string>%s</string>\n", indent, html.EscapeString(v))
	}
	fmt.Fprintf(b, "%s</array>\n", indent)
}

var errNoIconTools = errors.New("sips or iconutil not found")

// makeICNS converts a PNG to .icns with the macOS tools sips (resize) and
// iconutil (pack an .iconset).
func makeICNS(png, icns string) error {
	if _, err := os.Stat(png); err != nil {
		return err
	}
	sips, err1 := exec.LookPath("sips")
	iconutil, err2 := exec.LookPath("iconutil")
	if err1 != nil || err2 != nil {
		return errNoIconTools
	}
	tmp, err := os.MkdirTemp("", "tango-icon-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmp)
	set := filepath.Join(tmp, "icon.iconset")
	if err := os.Mkdir(set, 0o755); err != nil {
		return err
	}
	for _, size := range []int{16, 32, 128, 256, 512} {
		for _, scale := range []int{1, 2} {
			name := fmt.Sprintf("icon_%dx%d.png", size, size)
			if scale == 2 {
				name = fmt.Sprintf("icon_%dx%d@2x.png", size, size)
			}
			px := fmt.Sprint(size * scale)
			out, err := exec.Command(sips, "-s", "format", "png", "-z", px, px, png, "--out", filepath.Join(set, name)).CombinedOutput()
			if err != nil {
				return fmt.Errorf("sips: %v: %s", err, out)
			}
		}
	}
	if out, err := exec.Command(iconutil, "-c", "icns", "-o", icns, set).CombinedOutput(); err != nil {
		return fmt.Errorf("iconutil: %v: %s", err, out)
	}
	return nil
}

// runIn runs name with args in dir, with env added to the environment and
// the output passed through.
func runIn(ctx context.Context, dir string, env []string, name string, args ...string) error {
	cmd := exec.CommandContext(ctx, name, args...)
	return runCmd(cmd, dir, env)
}

// runShell runs a configured command line through the shell.
func runShell(ctx context.Context, dir string, env []string, line string) error {
	cmd := shellCommand(line)
	newGroup(cmd)
	done := make(chan struct{})
	defer close(done)
	go func() {
		select {
		case <-ctx.Done():
			signalGroup(cmd, true)
		case <-done:
		}
	}()
	return runCmd(cmd, dir, env)
}

func runCmd(cmd *exec.Cmd, dir string, env []string) error {
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), env...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("%s: %w", strings.Join(cmd.Args, " "), err)
	}
	return nil
}
