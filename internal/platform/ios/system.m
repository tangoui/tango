//go:build ios

// System settings followed by the canvas UI (system.h). The values are read
// on the main thread and kept in atomics: SystemTheme and friends may be
// asked from any goroutine, UIKit only answers on the main thread.

#import <UIKit/UIKit.h>
#include <stdatomic.h>
#include <stdlib.h>
#include <string.h>

#include "bridge_private.h"
#include "system.h"
#include "_cgo_export.h"

static atomic_int sysDark, sysMotion, sysContrast, sysWatchLocales;
static BOOL sysInstalled;

// The window's traits follow the system appearance (Tango never overrides
// the interface style); before the scene connected the screen's do.
static UITraitCollection *traits(void) {
	UIWindow *w = tg_window();
	return w ? w.traitCollection : UIScreen.mainScreen.traitCollection;
}

static void update(atomic_int *v, int now, int kind) {
	if (atomic_exchange(v, now) != now) tangoIOSSystemChanged(kind);
}

static UIWindow *watched;

static void watchWindow(void) {
	UIWindow *w = tg_window();
	if (!w || w == watched) return;
	watched = w;
	[w registerForTraitChanges:@[UITraitUserInterfaceStyle.class]
		withHandler:^(id<UITraitEnvironment> env, UITraitCollection *previous) { tgs_refresh(); }];
}

void tgs_refresh(void) {
	if (!sysInstalled) return;
	update(&sysDark, traits().userInterfaceStyle == UIUserInterfaceStyleDark, TGS_THEME);
	update(&sysMotion, UIAccessibilityIsReduceMotionEnabled() ? 1 : 0, TGS_MOTION);
	update(&sysContrast, UIAccessibilityDarkerSystemColorsEnabled() ? 1 : 0, TGS_CONTRAST);
}

void tgs_install(void) {
	if (sysInstalled) return;
	sysInstalled = YES;
	atomic_store(&sysDark, traits().userInterfaceStyle == UIUserInterfaceStyleDark);
	atomic_store(&sysMotion, UIAccessibilityIsReduceMotionEnabled() ? 1 : 0);
	atomic_store(&sysContrast, UIAccessibilityDarkerSystemColorsEnabled() ? 1 : 0);

	NSNotificationCenter *nc = NSNotificationCenter.defaultCenter;
	void (^refresh)(NSNotification *) = ^(NSNotification *n) { tgs_refresh(); };
	[nc addObserverForName:UIAccessibilityReduceMotionStatusDidChangeNotification object:nil queue:nil usingBlock:refresh];
	[nc addObserverForName:UIAccessibilityDarkerSystemColorsStatusDidChangeNotification object:nil queue:nil usingBlock:refresh];
	// The appearance can change while the app is in the background (the
	// automatic light/dark schedule): read it again when it comes back.
	// The window's trait changes cover a switch while the app is on
	// screen; it exists once the scene connected, before the app is
	// first active.
	[nc addObserverForName:UIApplicationDidBecomeActiveNotification object:nil queue:nil usingBlock:^(NSNotification *n) {
		watchWindow();
		tgs_refresh();
	}];
}

int tgs_dark(void) { return atomic_load(&sysDark); }
int tgs_reduce_motion(void) { return atomic_load(&sysMotion); }
int tgs_high_contrast(void) { return atomic_load(&sysContrast); }

char *tgs_locales(void) {
	NSString *s = [NSLocale.preferredLanguages componentsJoinedByString:@"\n"];
	return strdup(s.UTF8String ?: "");
}

void tgs_watch_locales(void) {
	if (atomic_exchange(&sysWatchLocales, 1)) return;
	[NSNotificationCenter.defaultCenter addObserverForName:NSCurrentLocaleDidChangeNotification object:nil
		queue:NSOperationQueue.mainQueue usingBlock:^(NSNotification *n) {
			tangoIOSSystemChanged(TGS_LOCALE);
		}];
}
