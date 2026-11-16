package editor

import (
	"bytes"
	"image/color"
	"slices"
	"testing"

	"github.com/tangoui/tango/ui/editor/buffer"
	"github.com/tangoui/tango/ui/editor/theme"
	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/paint"
)

// The real renderer/device must not keep a discarded private strip for
// hundreds of frames. Its visible, immutable image values remain valid.
func TestMinimapRetiresDiscardedImages(t *testing.T) {
	goText(t)
	st := NewState(manyLines(80), "a.txt", theme.Base("vs"))
	defer st.Close()
	h := newHintHost(t, st, geom.Size{W: 360, H: 220}, 1)
	for range 4 {
		h.frame()
	}
	check := func(label string) {
		t.Helper()
		want := len(minimapImages(h.vw))
		if got := len(h.dev.Images()); got != want {
			t.Fatalf("%s: device holds %d images for %d current strips", label, got, want)
		}
	}
	if len(h.dev.Images()) < 2 {
		t.Fatal("fixture has too few strips")
	}
	check("warm")
	oldImage := h.vw.vs.mini.strips[0].img
	oldPixels := slices.Clone(oldImage.Pix)
	for range 6 {
		off := st.Snapshot().LineStart(3)
		if err := st.Edit([]buffer.Edit{{Start: off, End: off, Text: "X"}}, Selection{}); err != nil {
			t.Fatal(err)
		}
		h.step("edit")
		check("edited")
	}
	st.Theme = theme.Base("vs-dark")
	h.step("theme")
	check("theme")
	h.w.scale = 2
	h.step("scale")
	check("scale")
	st.Options.Minimap.RenderCharacters = false
	h.step("blocks")
	check("blocks")
	st.Options.Minimap.RenderCharacters = true
	h.step("characters")
	check("characters")
	st.Options.Minimap.Enabled = false
	h.step("disabled")
	check("disabled")
	st.Options.Minimap.Enabled = true
	h.step("enabled")
	check("enabled")
	if !bytes.Equal(oldImage.Pix, oldPixels) {
		t.Fatal("retiring a strip changed its recorded immutable pixels")
	}
	// Cache retirement must not invalidate a saved immutable display list.
	list := &paint.List{Background: color.NRGBA{255, 255, 255, 255}}
	list.DrawImage(oldImage, geom.R(0, 0, 160, 64), geom.Radii{})
	ref := paint.NewCPU()
	ref.Render(list, h.w.size, h.w.scale)
	if err := h.ren.Render(list, h.w.size, h.w.scale); err != nil {
		t.Fatal(err)
	}
	got, want := h.dev.Target(), ref.Image()
	if got.Rect != want.Rect {
		t.Fatal("retired image replay changed target bounds")
	}
	for i, v := range want.Pix {
		if delta := int(got.Pix[i]) - int(v); delta < -1 || delta > 1 {
			t.Fatalf("retired image replay pixel byte %d differs: %d vs %d", i, got.Pix[i], v)
		}
	}
}
