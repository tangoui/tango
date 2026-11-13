package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"html"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
)

// Mobile targets. tan build -platform takes them next to the desktop ones:
//
//	ios                 device build (arm64), signed, with an .ipa
//	ios-simulator       Simulator build (arm64, Apple silicon Macs)
//	android             APK/AAB with arm64-v8a and x86_64 libraries
//	android/arm64,...   only these ABIs (arm64, amd64, arm, 386)
const (
	osIOS          = "ios"
	osIOSSimulator = "ios-simulator"
	osAndroid      = "android"
)

// androidABIs maps GOARCH to the Android ABI name and the NDK clang
// target prefix (the API level is appended).
var androidABIs = []struct{ goarch, abi, clang string }{
	{"arm64", "arm64-v8a", "aarch64-linux-android"},
	{"amd64", "x86_64", "x86_64-linux-android"},
	{"arm", "armeabi-v7a", "armv7a-linux-androideabi"},
	{"386", "x86", "i686-linux-android"},
}

// defaultAndroidArches are built for a bare "android": phones and the
// emulator on both kinds of host.
var defaultAndroidArches = []string{"arm64", "amd64"}

// parseAndroidArch accepts a GOARCH or an ABI name.
func parseAndroidArch(s string) (string, bool) {
	for _, a := range androidABIs {
		if s == a.goarch || s == a.abi {
			return a.goarch, true
		}
	}
	return "", false
}

func androidABI(goarch string) string {
	for _, a := range androidABIs {
		if a.goarch == goarch {
			return a.abi
		}
	}
	return ""
}

// tangoModuleDir finds the Tango module the project builds against, which
// holds the native hosts (mobile/ios, mobile/android). It follows the
// project's go.mod, so replace directives and workspaces are honoured.
func tangoModuleDir(ctx context.Context, cfg *Config) (string, error) {
	const mod = "github.com/tangoui/tango"
	cmd := exec.CommandContext(ctx, "go", "list", "-m", "-f", "{{.Dir}}", mod)
	cmd.Dir = cfg.dir
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	dir := strings.TrimSpace(string(out))
	if err != nil || dir == "" {
		if dir == "" && err == nil {
			err = errors.New("module not downloaded (run go mod download)")
		}
		return "", fmt.Errorf("locating %s from %s: %v %s", mod, cfg.dir, err, strings.TrimSpace(stderr.String()))
	}
	if _, err := os.Stat(filepath.Join(dir, "mobile")); err != nil {
		return "", fmt.Errorf("%s has no mobile/ directory; update github.com/tangoui/tango", dir)
	}
	return dir, nil
}

// cgoLinkFlags returns the #cgo LDFLAGS of the main package and its
// dependencies. A c-archive does not carry them, so the iOS link step
// adds them (frameworks the capability packages use, for example).
func cgoLinkFlags(ctx context.Context, cfg *Config, env []string) ([]string, error) {
	cmd := exec.CommandContext(ctx, "go", "list", "-deps", "-f", "{{range .CgoLDFLAGS}}{{.}}\n{{end}}", ".")
	cmd.Dir = cfg.dir
	cmd.Env = append(os.Environ(), env...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("go list: %v: %s", err, strings.TrimSpace(stderr.String()))
	}
	return dedupeLinkFlags(strings.Split(string(out), "\n")), nil
}

// dedupeLinkFlags keeps the first of each flag, treating "-framework X"
// and "-weak_framework X" as one flag.
func dedupeLinkFlags(tokens []string) []string {
	var out []string
	seen := map[string]bool{}
	for i := 0; i < len(tokens); i++ {
		tok := strings.TrimSpace(tokens[i])
		if tok == "" {
			continue
		}
		flag := []string{tok}
		if (tok == "-framework" || tok == "-weak_framework") && i+1 < len(tokens) {
			i++
			flag = append(flag, strings.TrimSpace(tokens[i]))
		}
		key := strings.Join(flag, " ")
		if !seen[key] {
			seen[key] = true
			out = append(out, flag...)
		}
	}
	return out
}

// loadIcon decodes the project icon, or returns nil when there is none.
func loadIcon(cfg *Config) (image.Image, error) {
	if cfg.Icon == "" {
		return nil, nil
	}
	f, err := os.Open(cfg.Path(cfg.Icon))
	if err != nil {
		return nil, err
	}
	defer f.Close()
	img, err := png.Decode(f)
	if err != nil {
		return nil, fmt.Errorf("icon %s: %w", cfg.Icon, err)
	}
	if b := img.Bounds(); b.Dx() != b.Dy() {
		warnf("Icon %s is %dx%d, not square; it will be stretched", cfg.Icon, b.Dx(), b.Dy())
	}
	return img, nil
}

// resizeIcon scales src to n×n by averaging the source pixels each target
// pixel covers (good for the downscaling icons need), optionally onto an
// opaque background (iOS app icons must not be transparent).
func resizeIcon(src image.Image, n int, bg color.Color) *image.NRGBA {
	b := src.Bounds()
	dst := image.NewNRGBA(image.Rect(0, 0, n, n))
	sw, sh := float64(b.Dx())/float64(n), float64(b.Dy())/float64(n)
	for y := range n {
		y0, y1 := int(float64(y)*sh), max(int(float64(y+1)*sh), int(float64(y)*sh)+1)
		for x := range n {
			x0, x1 := int(float64(x)*sw), max(int(float64(x+1)*sw), int(float64(x)*sw)+1)
			var r, g, bl, a, cnt uint64
			for sy := y0; sy < y1 && sy < b.Dy(); sy++ {
				for sx := x0; sx < x1 && sx < b.Dx(); sx++ {
					// Premultiplied, so transparent pixels do not darken edges.
					pr, pg, pb, pa := src.At(b.Min.X+sx, b.Min.Y+sy).RGBA()
					r, g, bl, a = r+uint64(pr), g+uint64(pg), bl+uint64(pb), a+uint64(pa)
					cnt++
				}
			}
			if cnt == 0 {
				continue
			}
			c := color.RGBA64{uint16(r / cnt), uint16(g / cnt), uint16(bl / cnt), uint16(a / cnt)}
			dst.Set(x, y, c)
		}
	}
	if bg == nil {
		return dst
	}
	flat := image.NewNRGBA(dst.Bounds())
	draw.Draw(flat, flat.Bounds(), image.NewUniform(bg), image.Point{}, draw.Src)
	draw.Draw(flat, flat.Bounds(), dst, image.Point{}, draw.Over)
	return flat
}

func writePNG(path string, img image.Image) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		return err
	}
	return os.WriteFile(path, buf.Bytes(), 0o644)
}

// copyTree copies the regular files under src to dst, skipping paths for
// which skip returns true. File modes are kept (but at least 0644).
func copyTree(src, dst string, skip func(rel string, d fs.DirEntry) bool) error {
	return filepath.WalkDir(src, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(src, p)
		if rel != "." && skip != nil && skip(filepath.ToSlash(rel), d) {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		target := filepath.Join(dst, rel)
		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		if !d.Type().IsRegular() {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		// Files from the module cache are read-only; the copies must not be.
		return copyFile(p, target, info.Mode().Perm()|0o644)
	})
}

// plist is an ordered property-list dictionary.
type plist []plistEntry

type plistEntry struct {
	key   string
	value any // string, bool, int, float64, []any, []string, plist, map[string]any
}

func (p *plist) set(key string, v any) {
	for i := range *p {
		if (*p)[i].key == key {
			(*p)[i].value = v
			return
		}
	}
	*p = append(*p, plistEntry{key, v})
}

func (p plist) get(key string) any {
	for _, e := range p {
		if e.key == key {
			return e.value
		}
	}
	return nil
}

// encodePlist renders an XML property list (Apple's plist DTD).
func encodePlist(root plist) []byte {
	var b bytes.Buffer
	b.WriteString(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
`)
	writePlistValue(&b, "", root)
	b.WriteString("</plist>\n")
	return b.Bytes()
}

func writePlistValue(b *bytes.Buffer, indent string, v any) {
	switch v := v.(type) {
	case string:
		fmt.Fprintf(b, "%s<string>%s</string>\n", indent, html.EscapeString(v))
	case bool:
		if v {
			fmt.Fprintf(b, "%s<true/>\n", indent)
		} else {
			fmt.Fprintf(b, "%s<false/>\n", indent)
		}
	case int:
		fmt.Fprintf(b, "%s<integer>%d</integer>\n", indent, v)
	case float64:
		if v == float64(int64(v)) {
			fmt.Fprintf(b, "%s<integer>%d</integer>\n", indent, int64(v))
		} else {
			fmt.Fprintf(b, "%s<real>%g</real>\n", indent, v)
		}
	case []string:
		vs := make([]any, len(v))
		for i, s := range v {
			vs[i] = s
		}
		writePlistValue(b, indent, vs)
	case []any:
		if len(v) == 0 {
			fmt.Fprintf(b, "%s<array/>\n", indent)
			return
		}
		fmt.Fprintf(b, "%s<array>\n", indent)
		for _, e := range v {
			writePlistValue(b, indent+"\t", e)
		}
		fmt.Fprintf(b, "%s</array>\n", indent)
	case map[string]any:
		var p plist
		for _, k := range sortedKeys(v) {
			p = append(p, plistEntry{k, v[k]})
		}
		writePlistValue(b, indent, p)
	case plist:
		if len(v) == 0 {
			fmt.Fprintf(b, "%s<dict/>\n", indent)
			return
		}
		fmt.Fprintf(b, "%s<dict>\n", indent)
		for _, e := range v {
			fmt.Fprintf(b, "%s\t<key>%s</key>\n", indent, html.EscapeString(e.key))
			writePlistValue(b, indent+"\t", e.value)
		}
		fmt.Fprintf(b, "%s</dict>\n", indent)
	case plistRaw: // <date>/<data> read by decodePlist (applinks.go)
		fmt.Fprintf(b, "%s<%s>%s</%s>\n", indent, v.tag, html.EscapeString(v.text), v.tag)
	default:
		panic(fmt.Sprintf("plist: unsupported value %T", v))
	}
}

func isMobileOS(goos string) bool {
	return goos == osIOS || goos == osIOSSimulator || goos == osAndroid
}

func isMobile(t target) bool { return isMobileOS(t.OS) }

// parseMobileTarget parses one -platform entry for a mobile OS; hasArch
// says whether it had a "/arch" part.
func parseMobileTarget(goos, arch string, hasArch bool) ([]target, error) {
	switch goos {
	case osIOS, osIOSSimulator:
		if hasArch && arch != "arm64" {
			return nil, errors.New("iOS builds are arm64 only")
		}
		return []target{{goos, "arm64"}}, nil
	default:
		if !hasArch {
			var ts []target
			for _, a := range defaultAndroidArches {
				ts = append(ts, target{osAndroid, a})
			}
			return ts, nil
		}
		goarch, ok := parseAndroidArch(arch)
		if !ok {
			return nil, errors.New("unsupported Android architecture (want arm64, amd64, arm or 386, or an ABI name such as arm64-v8a)")
		}
		return []target{{osAndroid, goarch}}, nil
	}
}

// mobileTargets are the mobile part of a build: iOS targets are built one
// by one, Android ones together (one APK holds every ABI).
type mobileTargets struct {
	ios           []target
	androidArches []string
}

func (m mobileTargets) empty() bool { return len(m.ios) == 0 && len(m.androidArches) == 0 }

func splitTargets(ts []target) (desktop []target, m mobileTargets) {
	for _, t := range ts {
		switch t.OS {
		case osIOS, osIOSSimulator:
			m.ios = append(m.ios, t)
		case osAndroid:
			if !slices.Contains(m.androidArches, t.Arch) {
				m.androidArches = append(m.androidArches, t.Arch)
			}
		default:
			desktop = append(desktop, t)
		}
	}
	return
}

// checkMobileBuild checks tools and signing setup before anything is
// built, so a missing keystore fails in seconds.
func checkMobileBuild(cfg *Config, m mobileTargets, o buildOptions) error {
	if o.xcode && len(m.ios) == 0 {
		return errors.New("-xcode needs -platform ios or ios-simulator")
	}
	for _, t := range m.ios {
		if err := checkIOSBuild(cfg, t, o.debug); err != nil {
			return fmt.Errorf("%s: %w", t.OS, err)
		}
	}
	if len(m.androidArches) > 0 {
		if err := checkAndroidBuild(cfg, o.debug); err != nil {
			return fmt.Errorf("android: %w", err)
		}
	}
	return nil
}

// buildMobile builds the mobile targets into <out>/ios, <out>/ios-simulator
// and <out>/android.
func buildMobile(ctx context.Context, cfg *Config, m mobileTargets, out string, o buildOptions) ([]artifact, error) {
	var arts []artifact
	report := func(a artifact) {
		for _, p := range append([]string{a.path}, a.extra...) {
			logf("  → %s", p)
		}
		arts = append(arts, a)
	}
	for _, t := range m.ios {
		logf("Building %s %s for %s", cfg.Name, cfg.Version, t.OS)
		a, err := buildIOS(ctx, cfg, t, filepath.Join(out, t.OS), o.debug, false)
		if err != nil {
			return arts, fmt.Errorf("%s: %w", t.OS, err)
		}
		report(a)
	}
	if o.xcode {
		proj, err := writeXcodeProject(ctx, cfg, filepath.Join(out, "xcode"))
		if err != nil {
			return arts, fmt.Errorf("xcode: %w", err)
		}
		logf("  → %s", proj)
	}
	if len(m.androidArches) > 0 {
		logf("Building %s %s for android (%s)", cfg.Name, cfg.Version, strings.Join(m.androidArches, ", "))
		a, err := buildAndroid(ctx, cfg, m.androidArches, filepath.Join(out, osAndroid), o.debug, false)
		if err != nil {
			return arts, fmt.Errorf("android: %w", err)
		}
		report(a)
	}
	return arts, nil
}
