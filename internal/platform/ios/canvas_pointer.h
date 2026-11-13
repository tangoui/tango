// C interface of iPad pointer and trackpad input (canvas_pointer.m) for
// the Go side (canvas_pointer.go). Main thread only. See
// docs/specs/mobile-canvas.md §2.1.

#ifndef TANGO_IOS_CANVAS_POINTER_H
#define TANGO_IOS_CANVAS_POINTER_H

#include <stdint.h>

// Hover phases passed to tangoIOSCanvasHover (hoverEnter... in Go).
enum {
	TGP_HOVER_ENTER = 0,
	TGP_HOVER_MOVE = 1,
	TGP_HOVER_LEAVE = 2,
};

// Pointer styles (pointerSystem... in Go).
enum {
	TGP_STYLE_SYSTEM = 0,
	TGP_STYLE_BEAM = 1,
	TGP_STYLE_RESIZE_EW = 2,
	TGP_STYLE_RESIZE_NS = 3,
};

// Sets the pointer shape over canvas h (a TGP_STYLE_* value).
void tgp_set_cursor(uintptr_t h, int style);

// Self-test (canvas_pointer_selftest.go). Ops run what the gesture
// handlers and the pointer interaction run, without UIKit recognizing a
// gesture. The result, if any, is malloc'd.
enum {
	TGP_TEST_HOVER = 0,   // a: phase; at (x, y)
	TGP_TEST_SCROLL = 1,  // a: 1 discrete; translation (tx, ty) at (x, y)
	TGP_TEST_STYLE = 2,   // describes the pointer style the interaction gives now
	TGP_TEST_HIT = 3,     // hit tests the window at (x, y) in canvas coordinates: the hit view's class
	TGP_TEST_MEMORY = 4,  // posts UIApplicationDidReceiveMemoryWarningNotification
};
char *tgp_selftest(uintptr_t h, int op, int a, double x, double y, double tx, double ty);
// Calls tangoIOSPointerSelftest(h) on the main queue after seconds.
void tgp_after(double seconds, uintptr_t h);

#endif
