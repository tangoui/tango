//go:build darwin

package metal

import (
	"fmt"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tangoui/tango/ui"
	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/gpu"
	"github.com/tangoui/tango/ui/gpu/internal/gputest"
	"github.com/tangoui/tango/ui/paint"
	"github.com/tangoui/tango/ui/text"
	"github.com/tangoui/tango/ui/theme"
)

// goldenThemes are the themes of ui's golden test (ui/golden_test.go
// TestGolden), with the names of their images in ui/testdata.
var goldenThemes = []struct {
	name string
	th   func() *theme.Theme
}{
	{"basic_antd", func() *theme.Theme { return theme.AntDesign() }},
	{"basic_element", func() *theme.Theme { return theme.ElementPlus() }},
	{"basic_dark", func() *theme.Theme { return theme.AntDesign(theme.Dark()) }},
}

// goldenSize is the logical size of ui's golden page.
var goldenSize = geom.Size{W: 640, H: 300}

// goldenThr is the per-channel threshold of the golden pages: they draw
// text, so the text threshold of spec 6.8 applies to the whole page.
const goldenThr = 4

// TestGoldenPagesGPU renders ui's golden page (rebuilt as galleryPage) in
// the three golden themes at scale 1 and 2 with paint.CPU and with Metal
// and compares them (≤ goldenThr per channel on ≥ 99.5% of the pixels).
// The geometry threshold (2) is logged alongside for the record. On
// failure the CPU image, the Metal image and a diff image are written to
// t.TempDir() (or $TANGO_GPU_DIFF_DIR, which survives the test) and their
// paths logged.
func TestGoldenPagesGPU(t *testing.T) {
	d := newDevice(t)
	r := gpu.NewRenderer(d)
	for _, tc := range goldenThemes {
		for _, scale := range []float32{1, 2} {
			t.Run(fmt.Sprintf("%s@%v", tc.name, scale), func(t *testing.T) {
				drv := ui.NewDriver(galleryPage, ui.DriverOptions{Width: goldenSize.W, Height: goldenSize.H, Scale: scale, Theme: tc.th()})
				drv.Settle()
				l := drv.Runner().List()
				gputest.CheckListEngine(t, l)
				ref := paint.NewCPU()
				if err := ref.Render(l, goldenSize, scale); err != nil {
					t.Fatal(err)
				}
				if err := r.Render(l, goldenSize, scale); err != nil {
					t.Fatal(err)
				}
				want, got := ref.Image(), readPixels(t, d)
				geo := gputest.Compare(got, want, 2)
				t.Logf("%s scale %v: at thr 2: over %d (%.3f%%)", tc.name, scale, geo.Over, geo.Frac*100)
				res := gputest.Check(t, tc.name, l, scale, want, got, goldenThr)
				if scale == 1 {
					// The link to ui's golden images: the Metal frame
					// itself against ui/testdata (TestGolden's tolerance).
					if golden, ok := goldenImage(t, tc.name); ok {
						gr := gputest.Compare(got, golden, goldenFileThr)
						t.Logf("%s metal vs ui/testdata/%s.png: max diff %d, over %d: %d (%.3f%%)", tc.name, tc.name, gr.MaxDiff, goldenFileThr, gr.Over, gr.Frac*100)
						if gr.Frac > gputest.MaxFrac {
							t.Errorf("%s: Metal frame differs from ui's golden image (%.3f%% pixels over %d)", tc.name, gr.Frac*100, goldenFileThr)
						}
					}
				}
				if res.Frac > gputest.MaxFrac {
					dir := diffDir(t)
					base := fmt.Sprintf("%s@%vx", tc.name, scale)
					for _, f := range []struct {
						suffix string
						img    image.Image
					}{{"cpu", want}, {"metal", got}, {"diff", diffImage(want, got, goldenThr)}} {
						p := filepath.Join(dir, base+"-"+f.suffix+".png")
						if err := writePNG(p, f.img); err != nil {
							t.Errorf("writing %s: %v", p, err)
							continue
						}
						t.Logf("%s image: %s", f.suffix, p)
					}
				}
			})
		}
	}
}

// goldenFileThr is TestGolden's per-channel tolerance against the
// recorded images.
const goldenFileThr = 24

// goldenFonts reports whether ui's golden images were recorded with the
// fonts this machine has, as ui's TestGolden decides (it skips otherwise).
// The images are recorded with the pure-Go text engine, which TestMain
// pins (gputest.Main); with another engine the comparison would only
// measure the engines' difference, so that is a failure, not a skip.
func goldenFonts(t *testing.T) bool {
	t.Helper()
	if e := text.Engine(); e != "go" {
		t.Fatalf("text engine %q: the golden comparisons need the pure-Go engine (gputest.Main in TestMain)", e)
	}
	fonts, err := os.ReadFile(filepath.Join(goldenDir, "golden_fonts.txt"))
	return err == nil && string(fonts) == strings.Join(text.Families(), "\n")
}

var goldenDir = filepath.Join("..", "..", "testdata")

// goldenImage returns ui/testdata/<name>.png, or false (logged) when the
// goldens were recorded with other fonts.
func goldenImage(t *testing.T, name string) (*image.RGBA, bool) {
	t.Helper()
	if !goldenFonts(t) {
		t.Logf("ui/testdata goldens were recorded with different fonts: not compared")
		return nil, false
	}
	img, err := readPNG(filepath.Join(goldenDir, name+".png"))
	if err != nil {
		t.Fatal(err)
	}
	return img, true
}

// TestGalleryPageIsGoldenPage checks that galleryPage (this package's copy
// of ui's goldenPage) still draws ui's golden images at scale 1 with
// paint.CPU, with TestGolden's tolerance, so TestGoldenPagesGPU compares
// the page the golden test records. It is skipped, like TestGolden, when
// the goldens were recorded with other fonts.
func TestGalleryPageIsGoldenPage(t *testing.T) {
	if !goldenFonts(t) {
		t.Skip("golden images were recorded with different fonts")
	}
	for _, tc := range goldenThemes {
		t.Run(tc.name, func(t *testing.T) {
			drv := ui.NewDriver(galleryPage, ui.DriverOptions{Width: goldenSize.W, Height: goldenSize.H, Scale: 1, Theme: tc.th()})
			got := drv.Settle()
			gputest.CheckListEngine(t, drv.Runner().List())
			want, ok := goldenImage(t, tc.name)
			if !ok {
				t.Skip("golden images were recorded with different fonts")
			}
			if got.Rect != want.Rect {
				t.Fatalf("size %v, want %v", got.Rect, want.Rect)
			}
			res := gputest.Compare(got, want, goldenFileThr)
			t.Logf("%s: max diff %d, over %d: %d (%.3f%%)", tc.name, res.MaxDiff, goldenFileThr, res.Over, res.Frac*100)
			if res.Frac > gputest.MaxFrac {
				t.Errorf("%s: galleryPage differs from ui's golden image (%.3f%% pixels over %d)", tc.name, res.Frac*100, goldenFileThr)
			}
		})
	}
}

// diffDir is where failing comparisons write their images.
func diffDir(t *testing.T) string {
	if d := os.Getenv("TANGO_GPU_DIFF_DIR"); d != "" {
		if err := os.MkdirAll(d, 0o755); err == nil {
			return d
		}
	}
	return t.TempDir()
}

// diffImage shows a faded copy of want with the pixels differing by more
// than thr in red and the smaller differences in amplified blue.
func diffImage(want, got *image.RGBA, thr int) *image.RGBA {
	out := image.NewRGBA(want.Rect)
	for i := 0; i < len(want.Pix); i += 4 {
		d := 0
		for c := 0; c < 4; c++ {
			v := int(want.Pix[i+c]) - int(got.Pix[i+c])
			d = max(d, v, -v)
		}
		var px color.RGBA
		switch {
		case d > thr:
			px = color.RGBA{255, 0, 0, 255}
		case d > 0:
			px = color.RGBA{0, 0, uint8(min(255, 128+d*32)), 255}
		default:
			g := uint8(192 + (int(want.Pix[i])+int(want.Pix[i+1])+int(want.Pix[i+2]))/12)
			px = color.RGBA{g, g, g, 255}
		}
		out.Pix[i], out.Pix[i+1], out.Pix[i+2], out.Pix[i+3] = px.R, px.G, px.B, px.A
	}
	return out
}

func writePNG(path string, img image.Image) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	if err := png.Encode(f, img); err != nil {
		f.Close()
		return err
	}
	return f.Close()
}

// readPNG decodes path into RGBA (premultiplied, as paint.CPU's images).
func readPNG(path string) (*image.RGBA, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	src, err := png.Decode(f)
	if err != nil {
		return nil, err
	}
	if rgba, ok := src.(*image.RGBA); ok {
		return rgba, nil
	}
	b := src.Bounds()
	out := image.NewRGBA(image.Rectangle{Max: b.Size()})
	for y := 0; y < b.Dy(); y++ {
		for x := 0; x < b.Dx(); x++ {
			out.Set(x, y, src.At(b.Min.X+x, b.Min.Y+y))
		}
	}
	return out, nil
}

// TestGoldenPagesGPUSystemText is TestGoldenPagesGPU with the system text
// engine (CoreText): the golden pages in the three themes at scale 1 and
// 2, Metal against paint.CPU with the text threshold. There is no link to
// ui/testdata here (the goldens are recorded with the pure-Go engine).
func TestGoldenPagesGPUSystemText(t *testing.T) {
	gputest.SystemText(t)
	d := newDevice(t)
	r := gpu.NewRenderer(d)
	for _, tc := range goldenThemes {
		for _, scale := range []float32{1, 2} {
			t.Run(fmt.Sprintf("%s@%v", tc.name, scale), func(t *testing.T) {
				drv := ui.NewDriver(galleryPage, ui.DriverOptions{Width: goldenSize.W, Height: goldenSize.H, Scale: scale, Theme: tc.th()})
				drv.Settle()
				l := drv.Runner().List()
				gputest.CheckListEngine(t, l)
				ref := paint.NewCPU()
				if err := ref.Render(l, goldenSize, scale); err != nil {
					t.Fatal(err)
				}
				if err := r.Render(l, goldenSize, scale); err != nil {
					t.Fatal(err)
				}
				want, got := ref.Image(), readPixels(t, d)
				geo := gputest.Compare(got, want, 2)
				t.Logf("%s scale %v (system text): at thr 2: over %d (%.3f%%)", tc.name, scale, geo.Over, geo.Frac*100)
				gputest.Check(t, tc.name, l, scale, want, got, goldenThr)
				if dir := os.Getenv("TANGO_GPU_DIFF_DIR"); dir != "" {
					base := fmt.Sprintf("%s@%vx-system", tc.name, scale)
					for suffix, img := range map[string]image.Image{"cpu": want, "metal": got, "diff": diffImage(want, got, goldenThr)} {
						if err := writePNG(filepath.Join(dir, base+"-"+suffix+".png"), img); err != nil {
							t.Error(err)
						}
					}
				}
			})
		}
	}
}
