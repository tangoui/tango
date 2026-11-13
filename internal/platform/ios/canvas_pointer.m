//go:build ios

// iPad pointer and trackpad input on canvas views
// (docs/specs/mobile-canvas.md §2.1): a UIHoverGestureRecognizer reports
// the pointer moving over the view (mouse, trackpad, Apple Pencil
// hovering), two scroll-only UIPanGestureRecognizers report trackpad
// (continuous) and mouse wheel (discrete) scrolling, and a
// UIPointerInteraction shows the pointer shape Go asked for. Clicks are
// touches (canvas.m); the decisions (event kinds, deltas, pass-through)
// are made in Go (canvas_pointer.go, canvas_attach_plan.go).

#import <UIKit/UIKit.h>
#include <stdlib.h>
#include <string.h>

#include "canvas_pointer.h"
#include "canvas_view.h"
#include "_cgo_export.h"

static void hover(TGCanvasView *v, int phase, CGPoint p, int64_t flags) {
	tangoIOSCanvasHover(v.handle, phase, p.x, p.y, flags);
}

static void scroll(TGCanvasView *v, BOOL discrete, CGPoint t, CGPoint p, int64_t flags) {
	if (t.x == 0 && t.y == 0) return;
	tangoIOSCanvasScroll(v.handle, discrete ? 1 : 0, t.x, t.y, p.x, p.y, flags);
}

// A double-headed arrow centred on the pointer, horizontal or vertical.
static UIBezierPath *doubleArrow(BOOL vertical) {
	const CGFloat len = 11, head = 5, shaft = 1.5;
	UIBezierPath *b = [UIBezierPath bezierPath];
	CGPoint pts[] = {
		{-len, 0}, {-len + head, -head}, {-len + head, -shaft}, {len - head, -shaft},
		{len - head, -head}, {len, 0}, {len - head, head}, {len - head, shaft},
		{-len + head, shaft}, {-len + head, head},
	};
	int n = sizeof pts / sizeof pts[0];
	for (int i = 0; i < n; i++) {
		CGPoint q = vertical ? CGPointMake(pts[i].y, pts[i].x) : pts[i];
		if (i == 0) {
			[b moveToPoint:q];
		} else {
			[b addLineToPoint:q];
		}
	}
	[b closePath];
	return b;
}

// styleFor returns the pointer style for a TGP_STYLE_* value; nil means
// the system pointer.
static UIPointerStyle *styleFor(int style) {
	switch (style) {
	case TGP_STYLE_BEAM:
		return [UIPointerStyle styleWithShape:[UIPointerShape beamWithPreferredLength:22 axis:UIAxisVertical]
			constrainedAxes:UIAxisNeither];
	case TGP_STYLE_RESIZE_EW:
		return [UIPointerStyle styleWithShape:[UIPointerShape shapeWithPath:doubleArrow(NO)] constrainedAxes:UIAxisNeither];
	case TGP_STYLE_RESIZE_NS:
		return [UIPointerStyle styleWithShape:[UIPointerShape shapeWithPath:doubleArrow(YES)] constrainedAxes:UIAxisNeither];
	}
	return nil;
}

@implementation TGCanvasView (Pointer)

- (void)tgHover:(UIHoverGestureRecognizer *)g {
	int phase;
	switch (g.state) {
	case UIGestureRecognizerStateBegan:
		phase = TGP_HOVER_ENTER;
		break;
	case UIGestureRecognizerStateChanged:
		phase = TGP_HOVER_MOVE;
		break;
	case UIGestureRecognizerStateEnded:
	case UIGestureRecognizerStateCancelled:
	case UIGestureRecognizerStateFailed:
		phase = TGP_HOVER_LEAVE;
		break;
	default:
		return;
	}
	hover(self, phase, [g locationInView:self], (int64_t)g.modifierFlags);
}

// Each update reports the translation since the previous one, then
// resets it.
- (void)tgScroll:(UIPanGestureRecognizer *)g {
	if (g.state != UIGestureRecognizerStateBegan && g.state != UIGestureRecognizerStateChanged) return;
	CGPoint t = [g translationInView:self];
	[g setTranslation:CGPointZero inView:self];
	scroll(self, g.allowedScrollTypesMask == UIScrollTypeMaskDiscrete, t, [g locationInView:self], (int64_t)g.modifierFlags);
}

// UIPointerInteractionDelegate. The region is the whole view, named by
// the current style, so that tgp_set_cursor's invalidate makes UIKit ask
// for the style again. Over a transparent pixel of a pass-through
// overlay there is no region: the views below choose the pointer.
- (UIPointerRegion *)pointerInteraction:(UIPointerInteraction *)interaction
	regionForRequest:(UIPointerRegionRequest *)request defaultRegion:(UIPointerRegion *)defaultRegion {
	CGPoint p = request.location;
	if (self.tgOverlay && !tangoIOSOverlayHit(self.handle, p.x, p.y)) return nil;
	return [UIPointerRegion regionWithRect:self.bounds identifier:@(self.tgCursor)];
}

- (UIPointerStyle *)pointerInteraction:(UIPointerInteraction *)interaction styleForRegion:(UIPointerRegion *)region {
	return styleFor(self.tgCursor);
}

@end

void tgp_install(TGCanvasView *v) {
	UIHoverGestureRecognizer *h = [[UIHoverGestureRecognizer alloc] initWithTarget:v action:@selector(tgHover:)];
	h.cancelsTouchesInView = NO;
	[v addGestureRecognizer:h];
	UIScrollTypeMask masks[] = {UIScrollTypeMaskContinuous, UIScrollTypeMaskDiscrete};
	for (int i = 0; i < 2; i++) {
		UIPanGestureRecognizer *s = [[UIPanGestureRecognizer alloc] initWithTarget:v action:@selector(tgScroll:)];
		s.allowedScrollTypesMask = masks[i];
		s.allowedTouchTypes = @[]; // scroll events only: fingers stay touches
		s.cancelsTouchesInView = NO;
		s.delaysTouchesBegan = NO;
		s.delaysTouchesEnded = NO;
		[v addGestureRecognizer:s];
	}
	UIPointerInteraction *pi = [[UIPointerInteraction alloc] initWithDelegate:v];
	[v addInteraction:pi];
	v.tgPointer = pi;
}

void tgp_teardown(TGCanvasView *v) {
	for (UIGestureRecognizer *g in [v.gestureRecognizers copy]) {
		if ([g isKindOfClass:UIHoverGestureRecognizer.class] || [g isKindOfClass:UIPanGestureRecognizer.class]) {
			[g removeTarget:v action:NULL];
			[v removeGestureRecognizer:g];
		}
	}
	if (v.tgPointer) {
		[v removeInteraction:v.tgPointer];
		v.tgPointer = nil;
	}
}

void tgp_set_cursor(uintptr_t h, int style) {
	TGCanvasView *v = tgc_view(h);
	if (!v || v.tgCursor == style) return;
	v.tgCursor = style;
	[v.tgPointer invalidate];
}

// ---------------------------------------------------------------------------
// Self-test

static char *ccopy(NSString *s) {
	return strdup(s ? s.UTF8String : "");
}

static NSString *describeStyle(TGCanvasView *v) {
	UIPointerRegion *r = [UIPointerRegion regionWithRect:v.bounds identifier:@(v.tgCursor)];
	UIPointerStyle *st = [v pointerInteraction:v.tgPointer styleForRegion:r];
	NSString *kind = @"system";
	switch (v.tgCursor) {
	case TGP_STYLE_BEAM: kind = @"beam"; break;
	case TGP_STYLE_RESIZE_EW: kind = @"resize-ew"; break;
	case TGP_STYLE_RESIZE_NS: kind = @"resize-ns"; break;
	}
	return [NSString stringWithFormat:@"%@ (UIPointerStyle %@, interaction %@)", kind,
		st ? @"set" : @"nil", v.tgPointer ? @"installed" : @"missing"];
}

char *tgp_selftest(uintptr_t h, int op, int a, double x, double y, double tx, double ty) {
	TGCanvasView *v = tgc_view(h);
	if (!v) return NULL;
	CGPoint p = CGPointMake(x, y);
	switch (op) {
	case TGP_TEST_HOVER:
		hover(v, a, p, 0);
		return NULL;
	case TGP_TEST_SCROLL:
		scroll(v, a != 0, CGPointMake(tx, ty), p, 0);
		return NULL;
	case TGP_TEST_STYLE:
		return ccopy(describeStyle(v));
	case TGP_TEST_HIT: {
		UIWindow *w = v.window;
		if (!w) return ccopy(@"(no window)");
		UIView *hit = [w hitTest:[v convertPoint:p toView:w] withEvent:nil];
		NSString *s = hit ? NSStringFromClass(hit.class) : @"nil";
		for (UIView *u = hit.superview; u; u = u.superview) {
			if ([u isKindOfClass:NSClassFromString(@"WKWebView")]) {
				s = [s stringByAppendingString:@" in WKWebView"];
				break;
			}
		}
		return ccopy(s);
	}
	case TGP_TEST_MEMORY:
		[NSNotificationCenter.defaultCenter postNotificationName:UIApplicationDidReceiveMemoryWarningNotification
			object:UIApplication.sharedApplication];
		return NULL;
	}
	return NULL;
}

void tgp_after(double seconds, uintptr_t h) {
	dispatch_after(dispatch_time(DISPATCH_TIME_NOW, (int64_t)(seconds * NSEC_PER_SEC)), dispatch_get_main_queue(), ^{
		@autoreleasepool {
			tangoIOSPointerSelftest(h);
		}
	});
}
