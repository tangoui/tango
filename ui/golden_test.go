package ui

import (
	"flag"
	"fmt"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/tangoui/tango/ui/icon"
	"github.com/tangoui/tango/ui/layout"
	"github.com/tangoui/tango/ui/text"
	"github.com/tangoui/tango/ui/theme"
)

var update = flag.Bool("update", false, "rewrite golden images in testdata/")

// Golden images depend on the fonts found on the machine that recorded them;
// the font list is stored next to them and the comparison is skipped on
// machines with a different list. They are drawn with the pure-Go text path
// (the system text engine is off), which renders identically everywhere.
func goldenFonts(t *testing.T) {
	t.Helper()
	prev := text.UseSystemEngine(false)
	t.Cleanup(func() { text.UseSystemEngine(prev) })
	got := strings.Join(text.Families(), "\n")
	path := filepath.Join("testdata", "golden_fonts.txt")
	if *update {
		os.MkdirAll("testdata", 0o755)
		os.WriteFile(path, []byte(got), 0o644)
		return
	}
	want, err := os.ReadFile(path)
	if err != nil || string(want) != got {
		t.Skipf("golden images were recorded with different fonts")
	}
}

func goldenPage(c *Context) {
	open := true
	val, empty := "Tango", ""
	ck := true
	date := time.Date(2026, 1, 15, 0, 0, 0, 0, time.UTC)
	_ = date
	Column(c, func() {
		Row(c, func() {
			Button(c, "主按钮").Primary()
			Button(c, "默认")
			Button(c, "虚线").Dashed()
			Button(c, "危险").Danger()
			Button(c, "搜索").Icon(icon.Search)
			Button(c, "禁用").Disabled(true)
		}, layout.Gap(8))
		Row(c, func() {
			Input(c, &val).Width(160)
			Input(c, &empty).Placeholder("请输入").Status("error").Width(160)
			Checkbox(c, "复选框", &ck)
			Switch(c, &open)
		}, layout.Gap(12), layout.Middle())
		Row(c, func() {
			Tag(c, "默认")
			Tag(c, "成功").Color("success")
			Badge(c).Status("processing", "运行中")
			Badge(c).Count(5).Body(func() { Avatar(c, "U").Square() })
		}, layout.Gap(12), layout.Middle())
		Alert(c, "info", "这是一条提示信息，中文渲染与断行。").Show()
		Progress(c, 60).Layout(layout.W(300))
	}, layout.Pad(16), layout.Gap(16))
}

func TestGolden(t *testing.T) {
	goldenFonts(t)
	for _, tc := range []struct {
		name string
		th   *theme.Theme
	}{{"basic_antd", theme.AntDesign()}, {"basic_element", theme.ElementPlus()}, {"basic_dark", theme.AntDesign(theme.Dark())}} {
		t.Run(tc.name, func(t *testing.T) {
			d := NewDriver(goldenPage, DriverOptions{Width: 640, Height: 300, Scale: 1, Theme: tc.th})
			img := d.Settle()
			path := filepath.Join("testdata", tc.name+".png")
			if *update {
				if err := d.SavePNG(path); err != nil {
					t.Fatal(err)
				}
				return
			}
			want, err := loadPNG(path)
			if err != nil {
				t.Fatalf("missing golden (run go test ./ui -run Golden -update): %v", err)
			}
			if diff := compare(img, want, 24); diff > 0.005 {
				out := filepath.Join(t.TempDir(), tc.name+".png")
				d.SavePNG(out)
				t.Fatalf("%.2f%% pixels differ; got %s", diff*100, out)
			}
		})
	}
}

func loadPNG(p string) (image.Image, error) {
	f, err := os.Open(p)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return png.Decode(f)
}

// compare returns the fraction of pixels whose channels differ by more than tol.
func compare(a, b image.Image, tol int) float64 {
	if a.Bounds() != b.Bounds() {
		fmt.Println("size mismatch", a.Bounds(), b.Bounds())
		return 1
	}
	bad, total := 0, 0
	r := a.Bounds()
	for y := r.Min.Y; y < r.Max.Y; y++ {
		for x := r.Min.X; x < r.Max.X; x++ {
			total++
			r1, g1, b1, a1 := a.At(x, y).RGBA()
			r2, g2, b2, a2 := b.At(x, y).RGBA()
			d := max(absDiff(r1, r2), absDiff(g1, g2), absDiff(b1, b2), absDiff(a1, a2)) >> 8
			if int(d) > tol {
				bad++
			}
		}
	}
	return float64(bad) / float64(total)
}

func absDiff(a, b uint32) uint32 {
	if a > b {
		return a - b
	}
	return b - a
}
