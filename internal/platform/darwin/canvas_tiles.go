package darwin

// Platform-independent part of partial frames on the canvas view
// (canvas_damage.go): which tiles of the frame a damage list touches.

import "image"

// canvasTileSize is the edge of a tile sublayer in device pixels.
const canvasTileSize = 256

// canvasTileGrid is the tile grid of a frame of size px.
type canvasTileGrid struct {
	px         image.Point
	cols, rows int
}

func newCanvasTileGrid(px image.Point) canvasTileGrid {
	return canvasTileGrid{px: px,
		cols: (px.X + canvasTileSize - 1) / canvasTileSize,
		rows: (px.Y + canvasTileSize - 1) / canvasTileSize}
}

// rect is tile i's rectangle in frame pixels (the last column and row are
// cut at the frame's edge).
func (g canvasTileGrid) rect(i int) image.Rectangle {
	x, y := (i%g.cols)*canvasTileSize, (i/g.cols)*canvasTileSize
	return image.Rect(x, y, min(x+canvasTileSize, g.px.X), min(y+canvasTileSize, g.px.Y))
}

// touched lists, in increasing order and without repeats, the tiles that
// rects (frame pixels, already clipped to the frame) touch, and their
// total area in pixels.
func (g canvasTileGrid) touched(rects []image.Rectangle) (tiles []int, area int) {
	if g.cols == 0 || g.rows == 0 {
		return nil, 0
	}
	seen := make([]bool, g.cols*g.rows)
	for _, r := range rects {
		r = r.Intersect(image.Rectangle{Max: g.px})
		if r.Empty() {
			continue
		}
		for ty := r.Min.Y / canvasTileSize; ty <= (r.Max.Y-1)/canvasTileSize; ty++ {
			for tx := r.Min.X / canvasTileSize; tx <= (r.Max.X-1)/canvasTileSize; tx++ {
				seen[ty*g.cols+tx] = true
			}
		}
	}
	for i, s := range seen {
		if s {
			tiles = append(tiles, i)
			r := g.rect(i)
			area += r.Dx() * r.Dy()
		}
	}
	return tiles, area
}

// canvasTileFrame is the frame of a tile sublayer in its superlayer's
// coordinates (points): tile rectangle r of a frame of height pxH at
// scale, in a superlayer of height layerH points whose geometry is
// flipped (origin at the top) or not.
func canvasTileFrame(r image.Rectangle, scale float64, flipped bool, layerH float64) (x, y, w, h float64) {
	x, y = float64(r.Min.X)/scale, float64(r.Min.Y)/scale
	w, h = float64(r.Dx())/scale, float64(r.Dy())/scale
	if !flipped {
		y = layerH - float64(r.Max.Y)/scale
	}
	return x, y, w, h
}

// canvasCopyTile copies rectangle r of img into dst (tightly packed rows)
// and reports whether every pixel is opaque.
func canvasCopyTile(dst []byte, img *image.RGBA, r image.Rectangle) (opaque bool) {
	opaque = true
	row := r.Dx() * 4
	for y := r.Min.Y; y < r.Max.Y; y++ {
		i := img.PixOffset(r.Min.X, y)
		d := dst[(y-r.Min.Y)*row : (y-r.Min.Y+1)*row]
		copy(d, img.Pix[i:i+row])
		for a := 3; a < row && opaque; a += 4 {
			opaque = d[a] == 0xff
		}
	}
	return opaque
}
