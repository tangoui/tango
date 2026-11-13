//go:build ios

package main

// Touch self-test for the simulator, where simctl cannot tap
// (docs/specs/mobile-canvas.md §7.3). With TANGO_MOBILEUI_SELFTEST=1
// (simctl launch: SIMCTL_CHILD_TANGO_MOBILEUI_SELFTEST=1) the app feeds
// scripted touches straight to the canvas view's touchesBegan/Moved/Ended
// methods a few seconds after launch: everything from the UIKit callback
// down (touch IDs, coalescing, the runner, the widgets) runs as for a
// finger; only UIKit's own hit testing is skipped.

/*
#cgo CFLAGS: -x objective-c -fobjc-arc
#cgo LDFLAGS: -framework UIKit
#import <UIKit/UIKit.h>

@interface MUIFakeTouch : UITouch
@property(nonatomic) CGPoint at;
@property(nonatomic) NSTimeInterval when;
@end

@implementation MUIFakeTouch
- (CGPoint)locationInView:(UIView *)v { return self.at; }
- (CGPoint)previousLocationInView:(UIView *)v { return self.at; }
- (NSTimeInterval)timestamp { return self.when; }
- (CGFloat)force { return 0; }
- (CGFloat)maximumPossibleForce { return 0; }
@end

static UIView *findCanvas(UIView *v) {
	if ([NSStringFromClass(v.class) isEqualToString:@"TGCanvasView"]) return v;
	for (UIView *s in v.subviews) {
		UIView *f = findCanvas(s);
		if (f) return f;
	}
	return nil;
}

static UIView *canvasView(void) {
	for (UIScene *sc in UIApplication.sharedApplication.connectedScenes) {
		if (![sc isKindOfClass:UIWindowScene.class]) continue;
		for (UIWindow *w in ((UIWindowScene *)sc).windows) {
			UIView *v = findCanvas(w);
			if (v) return v;
		}
	}
	return nil;
}

// muiTouch reports one phase (0 began, 1 moved, 2 ended) of a single
// scripted finger at (x, y) points.
static MUIFakeTouch *finger;

static void muiTouch(int phase, double x, double y) {
	UIView *v = canvasView();
	if (!v) { NSLog(@"mobile-ui selftest: no canvas view"); return; }
	if (phase == 0 || !finger) finger = [MUIFakeTouch new];
	finger.at = CGPointMake(x, y);
	finger.when = NSProcessInfo.processInfo.systemUptime;
	NSSet *s = [NSSet setWithObject:finger];
	switch (phase) {
	case 0: [v touchesBegan:s withEvent:nil]; break;
	case 1: [v touchesMoved:s withEvent:nil]; break;
	default: [v touchesEnded:s withEvent:nil]; finger = nil;
	}
}
*/
import "C"

import (
	"log"
	"os"
	"time"
)

func init() {
	if os.Getenv("TANGO_MOBILEUI_SELFTEST") != "1" {
		return
	}
	go selfTest()
}

func selfTest() {
	for app == nil {
		time.Sleep(100 * time.Millisecond)
	}
	touch := func(phase int, x, y float64) {
		app.Post(func() { C.muiTouch(C.int(phase), C.double(x), C.double(y)) })
		time.Sleep(16 * time.Millisecond) // one event per frame, as a finger
	}
	tap := func(what string, x, y float64) {
		log.Printf("mobile-ui selftest: %s", what)
		touch(0, x, y)
		time.Sleep(50 * time.Millisecond)
		touch(2, x, y)
		time.Sleep(700 * time.Millisecond)
	}
	time.Sleep(3 * time.Second)
	tap("tap the counter row", 200, 370)
	tap("tap the counter row again", 200, 370)
	tap("tap the bluetooth switch", 370, 225)
	log.Printf("mobile-ui selftest: drag the list up")
	touch(0, 200, 700)
	for i := 1; i <= 20; i++ {
		touch(1, 200, 700-float64(i)*20)
	}
	touch(2, 200, 300)
	time.Sleep(2 * time.Second)
	if os.Getenv("TANGO_MOBILEUI_SELFTEST_STAY") == "" {
		tap("tap the settings tab", 301, 817)
	}
	log.Printf("mobile-ui selftest: done")
}
