//go:build ios

// Canvases attached to WebView pages (docs/specs/mobile-canvas.md §1.4):
// a TGCanvasOverlay is a TGCanvasView placed in the TGPage's view as a
// sibling of the WKWebView, above or below it, so it shows, hides and
// moves with the page. Frames, touches, keys, text input and accessibility
// are the canvas view's (canvas.m, canvas_text.m, canvas_a11y.m); this
// file adds placement, pass-through hit testing and focus on touch.
// Placement and the alpha mask are decided in Go (canvas_attach.go).

#import <UIKit/UIKit.h>
#import <WebKit/WebKit.h>

#include "bridge_private.h"
#include "canvas_attach.h"
#include "canvas_view.h"
#include "_cgo_export.h"

@implementation TGCanvasOverlay

// With pass-through, a point over a pixel that the last frame left
// transparent is not ours: UIKit then asks the views below (the web view).
- (UIView *)hitTest:(CGPoint)p withEvent:(UIEvent *)event {
	UIView *hit = [super hitTest:p withEvent:event];
	if (hit != self || !self.passThrough) return hit;
	return tangoIOSOverlayHit(self.handle, p.x, p.y) ? hit : nil;
}

// A touch on the overlay gives it keyboard focus (the web view had it).
- (void)touchesBegan:(NSSet<UITouch *> *)touches withEvent:(UIEvent *)event {
	if (!self.isFirstResponder) [self becomeFirstResponder];
	[super touchesBegan:touches withEvent:event];
}

- (BOOL)becomeFirstResponder {
	BOOL ok = [super becomeFirstResponder];
	tgc_report_focus(self);
	return ok;
}

- (BOOL)resignFirstResponder {
	BOOL ok = [super resignFirstResponder];
	tgc_report_focus(self);
	return ok;
}

@end

static TGCanvasOverlay *overlayView(uintptr_t h) {
	TGCanvasView *v = tgc_view(h);
	return [v isKindOfClass:TGCanvasOverlay.class] ? (TGCanvasOverlay *)v : nil;
}

// webPage is the WebView page sid with its view loaded and sized.
static TGPage *webPage(uint64_t sid) {
	TGPage *p = tg_page(sid);
	if (!p || !p.web) return nil;
	UIView *v = p.view; // loads it
	if (CGRectIsEmpty(v.bounds)) {
		// Not presented yet: size it as presenting will, so that the
		// overlay's frame and the first frame have the right size.
		UIWindow *w = tg_window();
		v.frame = w ? w.bounds : UIScreen.mainScreen.bounds;
		[v layoutIfNeeded];
	}
	return p;
}

int tgc_page_size(uint64_t sid, double *w, double *hh) {
	TGPage *p = webPage(sid);
	if (!p) return 0;
	CGSize s = p.view.bounds.size;
	*w = s.width;
	*hh = s.height;
	return 1;
}

static void insert(TGPage *p, UIView *v, int above) {
	if (above) {
		[p.view insertSubview:v aboveSubview:p.web];
	} else {
		[p.view insertSubview:v belowSubview:p.web];
	}
}

int tgc_attach(uintptr_t h, uint64_t sid, int above, int passThrough,
	double x, double y, double w, double hh, int mask) {
	TGPage *p = webPage(sid);
	if (!p) return 0;
	TGCanvasOverlay *v = [[TGCanvasOverlay alloc] initWithFrame:CGRectMake(x, y, w, hh)];
	v.passThrough = passThrough != 0;
	v.opaque = NO;
	v.backgroundColor = UIColor.clearColor;
	v.autoresizingMask = (UIViewAutoresizing)mask;
	v.insets = p.view.safeAreaInsets;
	tgc_add_overlay(h, v);
	insert(p, v, above);
	return 1;
}

void tgc_overlay_place(uintptr_t h, int above, int passThrough,
	double x, double y, double w, double hh, int mask) {
	TGCanvasOverlay *v = overlayView(h);
	UIView *page = v.superview;
	if (!v || !page) return;
	v.passThrough = passThrough != 0;
	v.autoresizingMask = (UIViewAutoresizing)mask;
	for (UIView *s in page.subviews) {
		if (![s isKindOfClass:WKWebView.class]) continue;
		NSUInteger iv = [page.subviews indexOfObject:v], iw = [page.subviews indexOfObject:s];
		if ((iv > iw) != (above != 0)) {
			// Reordering siblings keeps the view in its window: first
			// responder and focus stay.
			if (above) {
				[page insertSubview:v aboveSubview:s];
			} else {
				[page insertSubview:v belowSubview:s];
			}
		}
		break;
	}
	v.frame = CGRectMake(x, y, w, hh); // layoutSubviews reports a size change
}

// Scroll view backgrounds of pages made see-through, to restore them.
static NSMutableDictionary<NSNumber *, id> *savedScrollBG;

void tgc_page_underlay(uint64_t sid, int underlay) {
	TGPage *p = tg_page(sid);
	if (!p || !p.web || !p.isViewLoaded) return;
	if (!savedScrollBG) savedScrollBG = [NSMutableDictionary dictionary];
	// The web view itself is never opaque (tg_surface_new); its scroll
	// view, the area under the page and the page view are.
	UIScrollView *sv = p.web.scrollView;
	if (underlay) {
		if (!savedScrollBG[@(sid)]) savedScrollBG[@(sid)] = sv.backgroundColor ?: NSNull.null;
		p.view.backgroundColor = UIColor.clearColor;
		sv.backgroundColor = UIColor.clearColor;
		if (@available(iOS 15.0, *)) p.web.underPageBackgroundColor = UIColor.clearColor;
		return;
	}
	p.view.backgroundColor = UIColor.systemBackgroundColor; // as TGPage loadView
	id old = savedScrollBG[@(sid)];
	if (old) {
		sv.backgroundColor = old == NSNull.null ? nil : old;
		[savedScrollBG removeObjectForKey:@(sid)];
	}
	if (@available(iOS 15.0, *)) p.web.underPageBackgroundColor = nil; // WebKit's default
}
