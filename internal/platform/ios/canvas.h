// C interface of the canvas pages (canvas.m) for the Go side (canvas.go).
// A canvas page is a TGCanvasPage (a TGPage without a web view) whose view
// is a TGCanvasView drawn by package ui. Each canvas is named by a handle
// chosen by Go. Every function is main-thread only unless marked
// otherwise. See docs/specs/mobile-canvas.md §1.2.

#ifndef TANGO_IOS_CANVAS_H
#define TANGO_IOS_CANVAS_H

#include <stdint.h>

// One report of a finger (UITouch), in the canvas view's coordinates.
typedef struct {
	double x, y;          // points (logical pixels)
	double force, maxForce;
	double time;          // UITouch.timestamp: seconds of system uptime
} tgc_sample;

// Touch phases passed to tangoIOSCanvasTouch.
enum {
	TGC_TOUCH_BEGAN = 0,
	TGC_TOUCH_MOVED = 1,
	TGC_TOUCH_ENDED = 2,
	TGC_TOUCH_CANCELLED = 3,
};

// Creates the page and its view for surface sid (not shown yet: the
// surface's Show pushes it on the page stack). The view's display link
// starts paused and its deadline timer disarmed.
void tgc_new(uintptr_t h, uint64_t sid, const char *title);
// Releases the display link and timer and forgets the page; the surface
// then closes it like a WebView page (tg_surface_close).
void tgc_destroy(uintptr_t h);
// Current logical size, device scale and safe-area insets.
void tgc_geometry(uintptr_t h, double *w, double *hh, double *scale,
	double *top, double *left, double *bottom, double *right);
// 1: the view is in a window and the app is active (the canvas has focus).
int tgc_focused(uintptr_t h);
// 1: the app is in the background.
int tgc_in_background(void);

// Frame scheduling. tgc_signal may be called from any thread: it wakes
// the main thread, which then calls tangoIOSCanvasWake.
void tgc_signal(void);
void tgc_link(uintptr_t h, int run);   // unpause (1) or pause (0) the display link
void tgc_timer(uintptr_t h, double in); // arm the deadline timer to fire in `in` seconds; < 0 disarms

// Presentation. tgc_present copies premultiplied RGBA (stride bytes per
// row) into a CGImage shown by the view's layer, or, after tgc_use_metal,
// by a sublayer above the Metal layer. tgc_use_metal adds a CAMetalLayer
// sublayer once and returns it (owned by the view). tgc_hide_cpu hides
// that CPU sublayer again.
int tgc_present(uintptr_t h, const void *pix, int w, int hh, int stride);
uintptr_t tgc_use_metal(uintptr_t h);
void tgc_hide_cpu(uintptr_t h);

#endif
