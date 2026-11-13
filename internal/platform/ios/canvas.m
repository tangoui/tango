//go:build ios

// UIKit half of the canvas pages (docs/specs/mobile-canvas.md §1.2): a
// TGCanvasPage is a TGPage without a web view, so the page stack in
// bridge.m (syncStack, presentation, Hide/Close) handles it unchanged; its
// view, a TGCanvasView, shows frames drawn by package ui. Every decision
// that is not about UIKit (frame scheduling, touch IDs, key mapping) is
// made in Go (canvas.go, canvas_keys.go, package mobilecanvas).
//
// Presentation uses sublayers of the view's own layer, since a UIView's
// layer class is fixed when it is created: CPU frames become the contents
// of the view's layer; after tgc_use_metal a CAMetalLayer sublayer fills
// the view and CPU fallback frames go to a second sublayer above it.

#import <UIKit/UIKit.h>
#import <QuartzCore/QuartzCore.h>
#include <stdlib.h>
#include <string.h>

#include "bridge_private.h"
#include "canvas.h"
#include "canvas_view.h"
#include "system.h"
#include "_cgo_export.h"

static char *ccopy(NSString *s) {
	return strdup(s ? s.UTF8String : "");
}

// Implicit animations off for every property a frame or a resize changes.
static NSDictionary *noActions(void) {
	static NSDictionary *d;
	if (!d) {
		NSNull *n = NSNull.null;
		d = @{@"bounds": n, @"position": n, @"frame": n, @"contents": n, @"contentsScale": n,
			@"hidden": n, @"opacity": n, @"sublayers": n, @"onOrderIn": n, @"onOrderOut": n};
	}
	return d;
}

@class TGCanvasView;

// TGCanvasLinkTarget is the display link's target: CADisplayLink retains
// its target, so the view is referenced weakly to avoid a cycle.
@interface TGCanvasLinkTarget : NSObject
@property(nonatomic, weak) TGCanvasView *view;
@end

@interface TGCanvasPage : TGPage
@property(nonatomic, strong) TGCanvasView *canvas;
@end

static NSMutableDictionary<NSNumber *, TGCanvasPage *> *canvasPages;
// Canvases attached to WebView pages (canvas_attach.m), by handle.
static NSMutableDictionary<NSNumber *, TGCanvasView *> *canvasOverlays;
static CFRunLoopSourceRef canvasSource;

static TGCanvasView *canvasView(uintptr_t h) {
	TGCanvasView *v = canvasPages[@(h)].canvas;
	return v ?: canvasOverlays[@(h)];
}

// allViews lists every open canvas view: pages and overlays.
static NSArray<TGCanvasView *> *allViews(void) {
	NSMutableArray<TGCanvasView *> *a = [NSMutableArray array];
	for (TGCanvasPage *p in canvasPages.allValues) [a addObject:p.canvas];
	[a addObjectsFromArray:canvasOverlays.allValues];
	return a;
}

TGCanvasView *tgc_view(uintptr_t h) {
	return canvasView(h);
}

static int appActive(void) {
	return UIApplication.sharedApplication.applicationState == UIApplicationStateActive;
}

static void reportFocus(TGCanvasView *v, BOOL f);

// focusNow reports whether v has focus: a canvas page while it is in a
// window, an overlay while it is also first responder (the web view
// beside it takes keyboard focus otherwise); either only while the app is
// active.
static BOOL focusNow(TGCanvasView *v) {
	if (!v.window || !appActive()) return NO;
	return !v.tgOverlay || v.isFirstResponder;
}

void tgc_report_focus(TGCanvasView *v) {
	reportFocus(v, focusNow(v));
}

static void reportFocus(TGCanvasView *v, BOOL f) {
	if (v.tgFocused == f) return;
	v.tgFocused = f;
	tangoIOSCanvasFocus(v.handle, f);
}

// ---------------------------------------------------------------------------
// Install: the wake source (Invalidate from any goroutine) and the app
// state observers shared by every canvas.

static void canvasPerform(void *info) {
	@autoreleasepool {
		tangoIOSCanvasWake();
	}
}

static void installCanvas(void) {
	if (canvasPages) return;
	canvasPages = [NSMutableDictionary dictionary];
	canvasOverlays = [NSMutableDictionary dictionary];
	CFRunLoopSourceContext ctx = {0};
	ctx.perform = canvasPerform;
	canvasSource = CFRunLoopSourceCreate(NULL, 0, &ctx);
	CFRunLoopAddSource(CFRunLoopGetMain(), canvasSource, kCFRunLoopCommonModes);

	NSNotificationCenter *nc = NSNotificationCenter.defaultCenter;
	[nc addObserverForName:UIApplicationDidEnterBackgroundNotification object:nil queue:nil usingBlock:^(NSNotification *n) {
		tangoIOSCanvasBackground(1);
	}];
	[nc addObserverForName:UIApplicationWillEnterForegroundNotification object:nil queue:nil usingBlock:^(NSNotification *n) {
		tangoIOSCanvasBackground(0);
	}];
	[nc addObserverForName:UIApplicationWillResignActiveNotification object:nil queue:nil usingBlock:^(NSNotification *n) {
		for (TGCanvasView *v in allViews()) reportFocus(v, NO);
	}];
	[nc addObserverForName:UIApplicationDidBecomeActiveNotification object:nil queue:nil usingBlock:^(NSNotification *n) {
		for (TGCanvasView *v in allViews()) reportFocus(v, focusNow(v));
	}];
}

void tgc_signal(void) {
	CFRunLoopSourceRef s = canvasSource;
	if (!s) return;
	CFRunLoopSourceSignal(s);
	CFRunLoopWakeUp(CFRunLoopGetMain());
}

int tgc_in_background(void) {
	return UIApplication.sharedApplication.applicationState == UIApplicationStateBackground;
}

// ---------------------------------------------------------------------------
// TGCanvasView

@implementation TGCanvasLinkTarget

- (void)tick:(CADisplayLink *)link {
	TGCanvasView *v = self.view;
	if (!v) return;
	@autoreleasepool {
		tangoIOSCanvasTick(v.handle);
	}
}

@end

@implementation TGCanvasView

- (CGFloat)scale {
	CGFloat s = self.traitCollection.displayScale;
	if (s <= 0) s = self.window.screen.scale;
	if (s <= 0) s = UIScreen.mainScreen.scale;
	return s > 0 ? s : 1;
}

// Keeps the sublayers on the bounds and the device scale (CALayer has no
// autoresizing on iOS) and reports the geometry to Go, which redraws
// synchronously after a change.
- (void)layoutSubviews {
	[super layoutSubviews];
	CGFloat s = self.scale;
	[CATransaction begin];
	[CATransaction setDisableActions:YES];
	self.layer.contentsScale = s;
	if (self.metalLayer) {
		self.metalLayer.frame = self.layer.bounds;
		self.metalLayer.contentsScale = s;
	}
	if (self.cpuLayer) {
		self.cpuLayer.frame = self.layer.bounds;
		self.cpuLayer.contentsScale = s;
	}
	[CATransaction commit];
	CGSize sz = self.bounds.size;
	tangoIOSCanvasLayout(self.handle, sz.width, sz.height, s);
	[self updateKeyboardInset];
}

- (void)safeAreaInsetsDidChange {
	[super safeAreaInsetsDidChange];
	UIEdgeInsets in = self.safeAreaInsets;
	if (UIEdgeInsetsEqualToEdgeInsets(in, self.insets)) return;
	self.insets = in;
	tangoIOSCanvasInsets(self.handle, in.top, in.left, in.bottom, in.right);
}

// watchTraits relayouts on a scale change (layoutSubviews reports it) and
// lets system.m notice a light/dark switch.
- (void)watchTraits {
	[self registerForTraitChanges:@[UITraitDisplayScale.class] withAction:@selector(setNeedsLayout)];
	[self registerForTraitChanges:@[UITraitUserInterfaceStyle.class]
		withHandler:^(id<UITraitEnvironment> env, UITraitCollection *previous) { tgs_refresh(); }];
}

- (void)didMoveToWindow {
	[super didMoveToWindow];
	if (self.window) {
		// Hardware keys (pressesBegan:). An overlay waits for a touch: the
		// web view beside it keeps the keyboard until then.
		if (!self.tgOverlay) [self becomeFirstResponder];
		[self safeAreaInsetsDidChange];
	}
	reportFocus(self, focusNow(self));
}

// The canvas is first responder while it is in a window, for hardware
// key presses; whether that shows the soft keyboard depends on text
// input (inputView, canvas_text.m).
- (BOOL)canBecomeFirstResponder {
	return YES;
}

// ---- touches ----

static tgc_sample sampleOf(UITouch *t, UIView *v) {
	CGPoint p = [t locationInView:v];
	tgc_sample s = {p.x, p.y, t.force, t.maximumPossibleForce, t.timestamp};
	return s;
}

// maxHistory bounds the coalesced samples passed to Go per move; Go keeps
// at most mobilecanvas.MaxCoalesced of them.
enum { maxHistory = 32 };

- (void)send:(NSSet<UITouch *> *)touches event:(UIEvent *)event phase:(int)phase {
	for (UITouch *t in touches) {
		tgc_sample buf[maxHistory + 1];
		int n = 0;
		if (phase == TGC_TOUCH_MOVED) {
			NSArray<UITouch *> *co = [event coalescedTouchesForTouch:t];
			NSInteger last = (NSInteger)co.count - 1; // co.lastObject is the current sample
			for (NSInteger i = MAX(0, last - maxHistory); i < last; i++) buf[n++] = sampleOf(co[i], self);
		}
		buf[n++] = sampleOf(t, self);
		tangoIOSCanvasTouch(self.handle, phase, (uintptr_t)(__bridge void *)t, buf, n);
	}
}

- (void)touchesBegan:(NSSet<UITouch *> *)touches withEvent:(UIEvent *)event {
	[self send:touches event:event phase:TGC_TOUCH_BEGAN];
}

- (void)touchesMoved:(NSSet<UITouch *> *)touches withEvent:(UIEvent *)event {
	[self send:touches event:event phase:TGC_TOUCH_MOVED];
}

- (void)touchesEnded:(NSSet<UITouch *> *)touches withEvent:(UIEvent *)event {
	[self send:touches event:event phase:TGC_TOUCH_ENDED];
}

- (void)touchesCancelled:(NSSet<UITouch *> *)touches withEvent:(UIEvent *)event {
	[self send:touches event:event phase:TGC_TOUCH_CANCELLED];
}

// ---- hardware keys ----

// press reports one key press or release to Go, which says whether it
// took it (canvas_keys.go keyRoute); the others go to super.
- (BOOL)press:(UIPress *)p down:(BOOL)down {
	UIKey *k = p.key;
	if (!k) return NO;
	char *c = ccopy(k.characters), *ci = ccopy(k.charactersIgnoringModifiers);
	int took = tangoIOSCanvasPress(self.handle, down ? 1 : 0, (int)k.keyCode, (int64_t)k.modifierFlags, c, ci);
	free(c);
	free(ci);
	return took != 0;
}

- (NSSet<UIPress *> *)route:(NSSet<UIPress *> *)presses down:(BOOL)down {
	NSMutableSet<UIPress *> *rest = [NSMutableSet set];
	for (UIPress *p in presses) {
		if (![self press:p down:down]) [rest addObject:p];
	}
	return rest;
}

- (void)pressesBegan:(NSSet<UIPress *> *)presses withEvent:(UIPressesEvent *)event {
	NSSet *rest = [self route:presses down:YES];
	if (rest.count) [super pressesBegan:rest withEvent:event];
}

- (void)pressesEnded:(NSSet<UIPress *> *)presses withEvent:(UIPressesEvent *)event {
	NSSet *rest = [self route:presses down:NO];
	if (rest.count) [super pressesEnded:rest withEvent:event];
}

- (void)pressesCancelled:(NSSet<UIPress *> *)presses withEvent:(UIPressesEvent *)event {
	NSSet *rest = [self route:presses down:NO];
	if (rest.count) [super pressesCancelled:rest withEvent:event];
}

@end

// ---------------------------------------------------------------------------
// TGCanvasPage: the page stack only needs a TGPage; the web view stays nil
// (messages to it are no-ops).

@implementation TGCanvasPage

- (void)loadView {
	self.view = self.canvas;
}

@end

// ---------------------------------------------------------------------------
// Lifecycle

void tgc_new(uintptr_t h, uint64_t sid, const char *title) {
	installCanvas();
	TGCanvasPage *p = [TGCanvasPage new];
	p.sid = sid;
	p.title = title ? [NSString stringWithUTF8String:title] ?: @"" : @"";

	UIWindow *w = tg_window();
	CGRect frame = w ? w.bounds : UIScreen.mainScreen.bounds;
	TGCanvasView *v = [[TGCanvasView alloc] initWithFrame:frame];
	v.opaque = YES;
	v.backgroundColor = UIColor.systemBackgroundColor; // until the first frame
	v.autoresizingMask = UIViewAutoresizingFlexibleWidth | UIViewAutoresizingFlexibleHeight;
	v.insets = w ? w.safeAreaInsets : UIEdgeInsetsZero;
	tgc_setup_view(v, h);
	p.canvas = v;
	canvasPages[@(h)] = p;
	tg_page_register(p);
}

void tgc_add_overlay(uintptr_t h, TGCanvasView *v) {
	installCanvas();
	v.tgOverlay = YES;
	tgc_setup_view(v, h);
	canvasOverlays[@(h)] = v;
}

void tgc_setup_view(TGCanvasView *v, uintptr_t h) {
	installCanvas();
	v.handle = h;
	[v watchTraits];
	v.multipleTouchEnabled = YES;
	v.layer.actions = noActions();
	// A frame whose size is not the view's yet (ceil vs. round, or a
	// resize in progress) stays unscaled at the top-left corner.
	v.layer.contentsGravity = v.layer.contentsAreFlipped ? kCAGravityBottomLeft : kCAGravityTopLeft;
	v.layer.contentsScale = v.scale;
	[v setupText];
	tgp_install(v); // canvas_pointer.m: iPad pointer and trackpad

	TGCanvasLinkTarget *target = [TGCanvasLinkTarget new];
	target.view = v;
	v.link = [CADisplayLink displayLinkWithTarget:target selector:@selector(tick:)];
	v.link.preferredFrameRateRange = CAFrameRateRangeMake(80, 120, 120);
	v.link.paused = YES;
	[v.link addToRunLoop:NSRunLoop.mainRunLoop forMode:NSRunLoopCommonModes];

	const CFAbsoluteTime never = 1.0e12;
	v.timer = CFRunLoopTimerCreateWithHandler(NULL, never, never, 0, 0, ^(CFRunLoopTimerRef t) {
		@autoreleasepool {
			tangoIOSCanvasTimer(h);
		}
	});
	CFRunLoopAddTimer(CFRunLoopGetMain(), v.timer, kCFRunLoopCommonModes);
}

void tgc_destroy(uintptr_t h) {
	TGCanvasView *v = canvasView(h);
	if (!v) return;
	[v.link invalidate];
	v.link = nil;
	if (v.timer) {
		CFRunLoopTimerInvalidate(v.timer);
		CFRelease(v.timer);
		v.timer = NULL;
	}
	v.tgFocused = NO; // no focus report after close
	v.textOn = NO;
	v.tgTokenizer = nil;
	tga_teardown(v);
	[NSNotificationCenter.defaultCenter removeObserver:v];
	if (v.isFirstResponder) [v resignFirstResponder];
	tgp_teardown(v);
	if (v.tgOverlay) {
		[v removeFromSuperview];
		[canvasOverlays removeObjectForKey:@(h)];
		return;
	}
	[canvasPages removeObjectForKey:@(h)];
}

void tgc_geometry(uintptr_t h, double *w, double *hh, double *scale,
	double *top, double *left, double *bottom, double *right) {
	TGCanvasView *v = canvasView(h);
	CGSize s = v.bounds.size;
	UIEdgeInsets in = v.window ? v.safeAreaInsets : v.insets;
	*w = s.width;
	*hh = s.height;
	*scale = v ? v.scale : 1;
	*top = in.top;
	*left = in.left;
	*bottom = in.bottom;
	*right = in.right;
}

int tgc_focused(uintptr_t h) {
	TGCanvasView *v = canvasView(h);
	return v && focusNow(v);
}

// ---------------------------------------------------------------------------
// Scheduling

void tgc_link(uintptr_t h, int run) {
	canvasView(h).link.paused = run ? NO : YES;
}

void tgc_timer(uintptr_t h, double in) {
	TGCanvasView *v = canvasView(h);
	if (!v.timer) return;
	CFRunLoopTimerSetNextFireDate(v.timer, in < 0 ? 1.0e12 : CFAbsoluteTimeGetCurrent() + in);
}

// ---------------------------------------------------------------------------
// Presentation

static CGColorSpaceRef srgb(void) {
	static CGColorSpaceRef cs;
	if (!cs) cs = CGColorSpaceCreateWithName(kCGColorSpaceSRGB);
	return cs;
}

int tgc_present(uintptr_t h, const void *pix, int w, int hh, int stride) {
	TGCanvasView *v = canvasView(h);
	if (!v) return -1;
	CFDataRef data = CFDataCreate(NULL, pix, (CFIndex)stride * (hh - 1) + (CFIndex)w * 4); // copies
	if (!data) return -1;
	CGDataProviderRef prov = CGDataProviderCreateWithCFData(data);
	CFRelease(data);
	if (!prov) return -1;
	CGImageRef img = CGImageCreate(w, hh, 8, 32, stride, srgb(),
		kCGImageAlphaPremultipliedLast | kCGBitmapByteOrder32Big, prov, NULL, false, kCGRenderingIntentDefault);
	CGDataProviderRelease(prov);
	if (!img) return -1;
	CALayer *layer = v.layer;
	[CATransaction begin];
	[CATransaction setDisableActions:YES];
	if (v.metalLayer) {
		if (!v.cpuLayer) {
			CALayer *c = [CALayer layer];
			c.actions = noActions();
			c.opaque = YES;
			c.frame = v.layer.bounds;
			c.contentsGravity = v.layer.contentsGravity;
			[v.layer addSublayer:c]; // above the Metal layer
			v.cpuLayer = c;
		}
		v.cpuLayer.hidden = NO;
		layer = v.cpuLayer;
	}
	layer.contents = (__bridge id)img;
	layer.contentsScale = v.scale;
	[CATransaction commit];
	CGImageRelease(img);
	return 0;
}

uintptr_t tgc_use_metal(uintptr_t h) {
	TGCanvasView *v = canvasView(h);
	if (!v) return 0;
	if (!v.metalLayer) {
		CAMetalLayer *m = [CAMetalLayer layer];
		if (!m) return 0;
		[CATransaction begin];
		[CATransaction setDisableActions:YES];
		m.actions = noActions();
		m.opaque = YES;
		m.frame = v.layer.bounds;
		m.contentsScale = v.scale;
		if (v.cpuLayer) {
			[v.layer insertSublayer:m below:v.cpuLayer];
		} else {
			[v.layer addSublayer:m];
		}
		v.metalLayer = m;
		[CATransaction commit];
	}
	return (uintptr_t)(__bridge void *)v.metalLayer;
}

void tgc_hide_cpu(uintptr_t h) {
	TGCanvasView *v = canvasView(h);
	if (!v.cpuLayer || v.cpuLayer.hidden) return;
	[CATransaction begin];
	[CATransaction setDisableActions:YES];
	v.cpuLayer.hidden = YES;
	v.cpuLayer.contents = nil;
	[CATransaction commit];
}
