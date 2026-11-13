package uihost

// Partial presentation of CPU frames (ui.DamagePresenter). The runner
// calls window.PresentDamage instead of Present when a CPU frame carries
// damage hints; the window forwards it to a canvas implementing
// platform.CanvasDamagePresenter, which updates only the dirty rectangles
// of the frame it keeps on screen. The window still presents whole
// frames:
//
//   - when the canvas has no partial path, or the window is on the GPU;
//   - for the first frame, and the first after a resize or scale change
//     (CanvasResized) or a size change of the image;
//   - after a failed present, until a whole frame succeeds;
//   - when PresentDamage fails: the frame is presented again whole (the
//     screen may hold a half-updated frame), and after damageMaxFails
//     failures in a row the window stops trying partial presents.

import (
	"image"
	"log"

	"github.com/tangoui/tango/internal/platform"
)

// damageMaxFails consecutive PresentDamage failures turn partial
// presentation off for the window.
const damageMaxFails = 3

// damageLogf logs the switch to whole frames (a test hook).
var damageLogf = log.Printf

// damageState is a window's partial-presentation state. Main thread.
type damageState struct {
	shown bool        // a frame of size px is on screen
	px    image.Point // device size of the frame on screen
	fails int         // consecutive PresentDamage failures
	off   bool        // partial presentation disabled for good
}

// resized makes the next frame whole. Safe on a nil window (callbacks that
// arrive before the runner's window exists).
func (w *window) resized() {
	if w != nil {
		w.dmg.shown = false
	}
}

// presentFull presents img whole and records what is on screen.
func (w *window) presentFull(img *image.RGBA) error {
	err := w.cv.Present(img)
	if err != nil || img == nil {
		w.dmg.shown = false
		return err
	}
	w.dmg.shown, w.dmg.px = true, img.Bounds().Size()
	return nil
}

// PresentDamage implements ui.DamagePresenter.
func (w *window) PresentDamage(img *image.RGBA, dirty []image.Rectangle) error {
	dp, ok := w.cv.(platform.CanvasDamagePresenter)
	if !ok || img == nil || w.gpu != nil || w.dmg.off || !w.dmg.shown || w.dmg.px != img.Bounds().Size() {
		return w.Present(img)
	}
	if err := dp.PresentDamage(img, dirty); err != nil {
		w.dmg.fails++
		if w.dmg.fails >= damageMaxFails {
			w.dmg.off = true
			damageLogf("tango: uihost: surface %d: partial present failed %d times, presenting whole frames: %v", w.id, w.dmg.fails, err)
		}
		return w.presentFull(img)
	}
	w.dmg.fails = 0
	return nil
}
