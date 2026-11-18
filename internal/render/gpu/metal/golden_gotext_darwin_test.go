//go:build darwin && (ios || tango_gotext)

package metal

import (
	"testing"

	"github.com/tangoui/tango/internal/render/gpu/internal/gputest"
	"github.com/tangoui/tango/ui"
)

// Compared with ui's golden images, which are exact pixels of the pure-Go
// (go-text) text engine: the default macOS build has only CoreText, so
// this runs with -tags tango_gotext.

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
