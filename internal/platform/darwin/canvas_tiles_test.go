package darwin

import (
	"image"
	"slices"
	"testing"
)

func TestCanvasTileGrid(t *testing.T) {
	g := newCanvasTileGrid(image.Pt(600, 300))
	if g.cols != 3 || g.rows != 2 {
		t.Fatalf("grid %dx%d", g.cols, g.rows)
	}
	if r := g.rect(5); r != image.Rect(512, 256, 600, 300) {
		t.Fatalf("last tile %v", r)
	}
	tiles, area := g.touched([]image.Rectangle{
		image.Rect(10, 10, 20, 20),     // tile 0
		image.Rect(250, 250, 260, 260), // tiles 0, 1, 3, 4
		image.Rect(256, 0, 257, 1),     // tile 1 again
		image.Rect(700, 0, 800, 10),    // outside
	})
	if !slices.Equal(tiles, []int{0, 1, 3, 4}) {
		t.Fatalf("tiles %v", tiles)
	}
	if want := 256*256*2 + 256*44*2; area != want {
		t.Fatalf("area %d, want %d", area, want)
	}
	if tiles, _ := newCanvasTileGrid(image.Point{}).touched([]image.Rectangle{image.Rect(0, 0, 1, 1)}); tiles != nil {
		t.Fatalf("empty grid touched %v", tiles)
	}
}

func TestCanvasTileFrame(t *testing.T) {
	r := image.Rect(256, 256, 600, 300)
	if x, y, w, h := canvasTileFrame(r, 2, true, 150); x != 128 || y != 128 || w != 172 || h != 22 {
		t.Fatalf("flipped: %v %v %v %v", x, y, w, h)
	}
	// Unflipped superlayer: origin at the bottom.
	if x, y, w, h := canvasTileFrame(r, 2, false, 150); x != 128 || y != 0 || w != 172 || h != 22 {
		t.Fatalf("unflipped: %v %v %v %v", x, y, w, h)
	}
}

func TestCanvasCopyTile(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 4, 4))
	for i := range img.Pix {
		img.Pix[i] = 0xff
	}
	img.Pix[img.PixOffset(1, 1)] = 7
	dst := make([]byte, 2*2*4)
	if !canvasCopyTile(dst, img, image.Rect(1, 1, 3, 3)) || dst[0] != 7 || dst[4] != 0xff {
		t.Fatalf("copy %v", dst)
	}
	img.Pix[img.PixOffset(2, 2)+3] = 0x80 // translucent pixel
	if canvasCopyTile(dst, img, image.Rect(1, 1, 3, 3)) {
		t.Fatal("translucent tile reported opaque")
	}
}
