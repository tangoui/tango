// C interface of canvases attached to WebView pages (canvas_attach.m) for
// the Go side (canvas_attach.go). An attached canvas is a TGCanvasOverlay
// placed in a TGPage's view next to its WKWebView; it is otherwise a
// canvas like those of canvas.h, named by a handle chosen by Go. Main
// thread only. See docs/specs/mobile-canvas.md §1.4.

#ifndef TANGO_IOS_CANVAS_ATTACH_H
#define TANGO_IOS_CANVAS_ATTACH_H

#include <stdint.h>

// Size of page sid's view in points (its content area). A page that is
// not on screen yet is sized to the window first, as presenting it will.
// Returns 0 when there is no WebView page sid.
int tgc_page_size(uint64_t sid, double *w, double *hh);
// Creates the overlay view for handle h in page sid, above or below the
// web view, at (x, y, w, hh) points with UIViewAutoresizing mask. Returns
// 0 when there is no WebView page sid.
int tgc_attach(uintptr_t h, uint64_t sid, int above, int passThrough,
	double x, double y, double w, double hh, int mask);
// Moves the overlay: frame, mask, stacking and pass-through. Changing the
// stacking keeps first responder.
void tgc_overlay_place(uintptr_t h, int above, int passThrough,
	double x, double y, double w, double hh, int mask);
// underlay 1 makes page sid see-through (clear view and web view
// backgrounds, also under the page) for canvases below the web view; 0
// restores the defaults.
void tgc_page_underlay(uint64_t sid, int underlay);

#endif
