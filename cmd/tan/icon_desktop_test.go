package main

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func solidImage(n int, c color.NRGBA) *image.NRGBA {
	img := image.NewNRGBA(image.Rect(0, 0, n, n))
	for i := 0; i < len(img.Pix); i += 4 {
		img.Pix[i], img.Pix[i+1], img.Pix[i+2], img.Pix[i+3] = c.R, c.G, c.B, c.A
	}
	return img
}

func TestDesktopIconShape(t *testing.T) {
	red := color.NRGBA{220, 30, 40, 255}
	img := desktopIcon(solidImage(1024, red))
	if b := img.Bounds(); b.Dx() != 1024 || b.Dy() != 1024 {
		t.Fatalf("size %v", b)
	}
	at := func(x, y int) color.NRGBA { return img.NRGBAAt(x, y) }
	// Inside the square: the artwork, opaque.
	for _, p := range [][2]int{{512, 512}, {101, 512}, {922, 512}, {512, 101}, {512, 922}, {200, 200}} {
		if c := at(p[0], p[1]); c != red {
			t.Errorf("(%d,%d) = %v, want the artwork", p[0], p[1], c)
		}
	}
	// The margin is transparent, except for the shadow below.
	for _, p := range [][2]int{{0, 0}, {512, 20}, {20, 512}, {1003, 512}, {102, 102}, {921, 102}} {
		if c := at(p[0], p[1]); c.A != 0 {
			t.Errorf("(%d,%d) = %v, want transparent", p[0], p[1], c)
		}
	}
	if c := at(512, 930); c.A == 0 || c.A > 100 || c.R != 0 {
		t.Errorf("shadow below the square: %v", c)
	}
	if at(512, 930).A <= at(512, 95).A {
		t.Error("shadow not offset downwards")
	}
	// Anti-aliased corner: partial alpha along the curve at 45°.
	partial := 0
	for d := 120; d < 180; d++ {
		if a := at(d, d).A; a > 0 && a < 255 && at(d, d).R > 0 {
			partial++
		}
	}
	if partial == 0 {
		t.Error("corner is not anti-aliased")
	}
	// The corner is about as deep at 45° as a 185 px circular corner:
	// 100 + 185.4(1-1/√2) ≈ 154.3.
	first := -1
	for d := 100; d < 200; d++ {
		if at(d, d).R > 128 {
			first = d
			break
		}
	}
	if first < 150 || first > 159 {
		t.Errorf("corner reaches the diagonal at %d, want about 154", first)
	}
	// Symmetric left/right.
	for _, y := range []int{110, 300, 512, 900} {
		for _, x := range []int{105, 130, 160} {
			if l, r := at(x, y), at(1023-x, y); l != r {
				t.Errorf("row %d: %v at %d, %v at %d", y, l, x, r, 1023-x)
			}
		}
	}
}

func TestScaleIcon(t *testing.T) {
	// A 2×2 checker of opaque black and transparent white averages to
	// half-transparent black, not grey.
	src := image.NewNRGBA(image.Rect(0, 0, 64, 64))
	for y := range 64 {
		for x := range 64 {
			if (x+y)%2 == 0 {
				src.SetNRGBA(x, y, color.NRGBA{0, 0, 0, 255})
			} else {
				src.SetNRGBA(x, y, color.NRGBA{255, 255, 255, 0})
			}
		}
	}
	got := scaleIcon(src, 16).NRGBAAt(7, 7)
	if got.R != 0 || got.A < 126 || got.A > 129 {
		t.Errorf("downscaled checker = %v", got)
	}
	// Non-integer factor keeps a solid colour solid, edge to edge.
	c := color.NRGBA{10, 200, 90, 255}
	s := scaleIcon(solidImage(1024, c), 824)
	for _, p := range [][2]int{{0, 0}, {823, 823}, {411, 400}} {
		if g := s.NRGBAAt(p[0], p[1]); g != c {
			t.Errorf("824 at %v = %v", p, g)
		}
	}
	// Enlarging works too.
	if g := scaleIcon(solidImage(10, c), 37).NRGBAAt(36, 0); g != c {
		t.Errorf("enlarged = %v", g)
	}
	for m := 1; m < 40; m++ {
		for _, n := range []int{1, 7, 16, 33} {
			for d, taps := range resampleTaps(m, n) {
				var sum float32
				for _, tp := range taps {
					sum += tp.w
					if tp.i < 0 || tp.i >= m {
						t.Fatalf("tap %d out of range for %d→%d", tp.i, m, n)
					}
				}
				if sum < 0.999 || sum > 1.001 {
					t.Fatalf("%d→%d sample %d weights sum to %v", m, n, d, sum)
				}
			}
		}
	}
}

func iconTestConfig(t *testing.T) *Config {
	cfg := testConfig(t)
	cfg.Icon = "icon.png"
	if err := os.WriteFile(cfg.Path("icon.png"), defaultIcon("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	return cfg
}

func TestDesktopIconSources(t *testing.T) {
	cfg := testConfig(t)
	if img, err := desktopIconImage(cfg, "macos", ""); img != nil || err != nil {
		t.Errorf("no icon: %v %v", img, err)
	}
	if p, _, err := macOSIconFile(cfg); p != "" || err != nil {
		t.Errorf("no icon: macOS file %q %v", p, err)
	}
	if m, err := linuxIcons(cfg); m != nil || err != nil {
		t.Errorf("no icon: linux %v %v", m, err)
	}

	cfg = iconTestConfig(t)
	img, err := desktopIconImage(cfg, "macos", "")
	if err != nil {
		t.Fatal(err)
	}
	if a := img.(*image.NRGBA).NRGBAAt(0, 0).A; a != 0 {
		t.Error("derived icon has no transparent margin")
	}
	p, cleanup, err := macOSIconFile(cfg)
	if err != nil || p == "" {
		t.Fatalf("macOS file %q %v", p, err)
	}
	if _, err := os.Stat(p); err != nil {
		t.Fatal(err)
	}
	cleanup()
	if _, err := os.Stat(p); err == nil {
		t.Error("temporary macOS icon left behind")
	}

	// An override is used as is.
	own := solidImage(64, color.NRGBA{1, 2, 3, 255})
	var buf bytes.Buffer
	png.Encode(&buf, own)
	os.MkdirAll(cfg.Path("icons"), 0o755)
	os.WriteFile(cfg.Path("icons/mac.png"), buf.Bytes(), 0o644)
	cfg.Icons.MacOS = "icons/mac.png"
	if p, _, _ := macOSIconFile(cfg); p != cfg.Path("icons/mac.png") {
		t.Errorf("macOS override: %q", p)
	}
	cfg.Icons.Linux.App = "icons/mac.png"
	m, err := linuxIcons(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if len(m) != len(linuxIconSizes) {
		t.Fatalf("%d linux sizes", len(m))
	}
	for n, b := range m {
		img, err := png.Decode(bytes.NewReader(b))
		if err != nil {
			t.Fatal(err)
		}
		if img.Bounds().Dx() != n || img.Bounds().Dy() != n {
			t.Errorf("size %d is %v", n, img.Bounds())
		}
		if c := color.NRGBAModel.Convert(img.At(0, 0)).(color.NRGBA); c != (color.NRGBA{1, 2, 3, 255}) {
			t.Errorf("size %d corner %v: override not used as is", n, c)
		}
	}
	cfg.Icons.Linux.App = "icons/missing.png"
	if _, err := linuxIcons(cfg); err == nil {
		t.Error("missing override: no error")
	}
	if got := hicolorPath("Hello", 48); got != "icons/hicolor/48x48/apps/Hello.png" {
		t.Error(got)
	}
	if got := hicolorSymbolicPath("Hello"); got != "icons/hicolor/scalable/apps/Hello-symbolic.svg" {
		t.Error(got)
	}
}

// TestWriteDesktopIconSamples writes the derived icon for a visual check
// when TAN_ICON_SAMPLES names a directory.
func TestWriteDesktopIconSamples(t *testing.T) {
	dir := os.Getenv("TAN_ICON_SAMPLES")
	if dir == "" {
		t.Skip("TAN_ICON_SAMPLES not set")
	}
	master, err := png.Decode(bytes.NewReader(defaultIcon("Hello")))
	if err != nil {
		t.Fatal(err)
	}
	if err := writePNG(filepath.Join(dir, "master-default.png"), master); err != nil {
		t.Fatal(err)
	}
	d := desktopIcon(master)
	writePNG(filepath.Join(dir, "desktop-1024.png"), d)
	for _, n := range []int{16, 32, 48, 256} {
		writePNG(filepath.Join(dir, fmt.Sprintf("desktop-%d.png", n)), scaleIcon(d, n))
	}
}

// TestLinuxInstallIcons unpacks the Linux tarball and runs its install.sh
// into a temporary prefix: the hicolor icons land in the theme.
func TestLinuxInstallIcons(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("install.sh is the Linux installer; Windows shells mangle its drive-letter paths")
	}
	sh, err := exec.LookPath("sh")
	if err != nil {
		t.Skip("no sh")
	}
	for _, tool := range []string{"install", "sed", "tar"} {
		if _, err := exec.LookPath(tool); err != nil {
			t.Skip("no " + tool)
		}
	}
	cfg := iconTestConfig(t)
	cfg.Icons.Linux.Symbolic = "sym.svg"
	os.WriteFile(cfg.Path("sym.svg"), []byte("<svg/>"), 0o644)
	a := artifact{target: target{"linux", "amd64"}, dir: filepath.Join(cfg.dir, "build", "linux-amd64")}
	os.MkdirAll(a.dir, 0o755)
	a.exe = filepath.Join(a.dir, "HelloApp")
	os.WriteFile(a.exe, []byte("#!/bin/sh\n"), 0o755)
	tarPath, _ := exec.LookPath("tar")
	t.Setenv("PATH", filepath.Dir(tarPath)+string(os.PathListSeparator)+"/usr/bin"+string(os.PathListSeparator)+"/bin")
	made, err := packageLinux(cfg, a)
	if err != nil {
		t.Fatal(err)
	}
	unpack := t.TempDir()
	if out, err := exec.Command("tar", "-xzf", made[0], "-C", unpack).CombinedOutput(); err != nil {
		t.Fatalf("tar: %v %s", err, out)
	}
	prefix := t.TempDir()
	cmd := exec.Command(sh, filepath.Join(unpack, "HelloApp-1.0.0-linux-amd64", "install.sh"))
	cmd.Env = append(os.Environ(), "PREFIX="+prefix)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("install.sh: %v\n%s", err, out)
	}
	for _, n := range linuxIconSizes {
		if _, err := os.Stat(filepath.Join(prefix, "share", hicolorPath("HelloApp", n))); err != nil {
			t.Error(err)
		}
	}
	if _, err := os.Stat(filepath.Join(prefix, "share", hicolorSymbolicPath("HelloApp"))); err != nil {
		t.Error(err)
	}
	d, _ := os.ReadFile(filepath.Join(prefix, "share", "applications", "HelloApp.desktop"))
	if !strings.Contains(string(d), "\nIcon=HelloApp\n") {
		t.Errorf("installed .desktop:\n%s", d)
	}
}
