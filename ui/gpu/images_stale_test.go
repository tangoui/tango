package gpu

import (
	"image"
	"testing"
)

// TestImageCacheStaleBudget: images re-created every frame (a zoomed
// preview) do not pile up to imageCacheBytes: textures the latest frame
// does not draw are held up to imageStaleBytes only, so an idle window
// keeps at most that much besides what its last frame drew.
func TestImageCacheStaleBudget(t *testing.T) {
	c := newImageCache()
	var f Frame
	const side = 1024 // 4 MB per texture
	var kept *image.RGBA
	for k := range 60 {
		c.beginFrame()
		img := image.NewRGBA(image.Rect(0, 0, side, side))
		if k == 0 {
			kept = img
		}
		if _, miss := c.get(img, 0, 0, defaultImageBudget); miss != imageOK {
			t.Fatalf("frame %d: miss %v", k, miss)
		}
		if k < 3 {
			// The first image is drawn in the first frames only.
			if _, miss := c.get(kept, 0, 0, defaultImageBudget); miss != imageOK {
				t.Fatal(miss)
			}
		}
		c.frameLists(&f)
		c.executed(true)
		cur := 4 * side * side
		if c.held > cur+imageStaleBytes {
			t.Fatalf("frame %d: %d MB held, want at most %d MB", k, c.held>>20, (cur+imageStaleBytes)>>20)
		}
	}
	if c.held < imageStaleBytes/2 {
		t.Fatalf("only %d MB held: recently drawn images should stay cached up to the stale budget", c.held>>20)
	}
	// Textures the current frame draws are never evicted by the stale
	// budget, however many.
	c.beginFrame()
	var imgs []*image.RGBA
	for range 20 {
		img := image.NewRGBA(image.Rect(0, 0, side, side))
		imgs = append(imgs, img)
		c.get(img, 0, 0, 1<<30)
	}
	c.frameLists(&f)
	c.executed(true)
	for _, img := range imgs {
		if c.byKey[imageKey{img, 0, 0}] == nil {
			t.Fatal("texture drawn this frame evicted")
		}
	}
}
