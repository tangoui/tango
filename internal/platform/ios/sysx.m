//go:build ios

// Desktop-style system capabilities of Tango's iOS backend (sysx.h): open a
// URL or preview a file, local notifications, connectivity and power.
// Go half: sysx.go.

#import <UIKit/UIKit.h>
#import <Network/Network.h>
#import <UserNotifications/UserNotifications.h>
#include <stdatomic.h>
#include <stdlib.h>
#include <string.h>

#include "bridge_private.h"
#include "mobile.h"
#include "sysx.h"
#include "_cgo_export.h"

static void finishX(int tok, int code, NSString *s) {
	char *c = strdup(s ? s.UTF8String ?: "" : "");
	tangoIOSMobileDone(tok, code, c);
	free(c);
}

// onMainX runs b on the main thread in the common modes (see onMain in
// mobile.m), so answers also arrive while a nested wait pumps events.
static void onMainX(dispatch_block_t b) {
	if (NSThread.isMainThread) {
		b();
		return;
	}
	CFRunLoopPerformBlock(CFRunLoopGetMain(), kCFRunLoopCommonModes, b);
	CFRunLoopWakeUp(CFRunLoopGetMain());
}

// ---------------------------------------------------------------------------
// Shell

int tgx_open_url(const char *url) {
	NSString *s = [NSString stringWithUTF8String:url];
	NSURL *u = s.length ? [NSURL URLWithString:s] : nil;
	if (!u || !u.scheme.length) return TGM_ERROR;
	[UIApplication.sharedApplication openURL:u options:@{} completionHandler:^(BOOL ok) {
		// openURL answers asynchronously; there is nobody left to tell.
		if (!ok) NSLog(@"tango/ios: no app opened %@", u.scheme);
	}];
	return TGM_OK;
}

// The preview being shown; UIDocumentInteractionController must be kept
// alive while its UI is up.
@interface TGPreview : NSObject <UIDocumentInteractionControllerDelegate>
@property(nonatomic, strong) UIDocumentInteractionController *doc;
@end

static TGPreview *preview;

@implementation TGPreview

- (UIViewController *)documentInteractionControllerViewControllerForPreview:(UIDocumentInteractionController *)c {
	return tg_top_controller();
}

- (void)documentInteractionControllerDidEndPreview:(UIDocumentInteractionController *)c {
	if (preview == self) preview = nil;
	tg_presented_ended();
}

- (void)documentInteractionControllerDidDismissOpenInMenu:(UIDocumentInteractionController *)c {
	if (preview == self) preview = nil;
	tg_presented_ended();
}

@end

int tgx_open_path(const char *path) {
	NSString *p = [NSString stringWithUTF8String:path];
	BOOL dir = NO;
	if (!p.length || ![NSFileManager.defaultManager fileExistsAtPath:p isDirectory:&dir]) return TGM_NOT_FOUND;
	// Folders have no viewer on iOS (the Files app cannot be pointed at
	// an app's sandbox).
	if (dir) return TGM_UNSUPPORTED;
	UIViewController *top = tg_top_controller();
	if (!top) return TGM_NOT_AVAILABLE;
	TGPreview *pv = [TGPreview new];
	pv.doc = [UIDocumentInteractionController interactionControllerWithURL:[NSURL fileURLWithPath:p]];
	pv.doc.delegate = pv;
	preview = pv;
	if ([pv.doc presentPreviewAnimated:YES]) return TGM_OK;
	// QuickLook cannot show the type: offer the apps that can open it.
	CGRect r = CGRectMake(CGRectGetMidX(top.view.bounds), CGRectGetMidY(top.view.bounds), 1, 1);
	if ([pv.doc presentOpenInMenuFromRect:r inView:top.view animated:YES]) return TGM_OK;
	preview = nil;
	return TGM_UNSUPPORTED;
}

// ---------------------------------------------------------------------------
// Local notifications. The delegate (TGPush in mobile.m) shows them while
// the app is in the foreground and reports taps.

// The authorization status last read (UNAuthorizationStatus), -1 before
// the first read. Main thread.
static NSInteger notifyStatus = -1;

static void readNotifyStatus(void (^then)(UNAuthorizationStatus)) {
	[UNUserNotificationCenter.currentNotificationCenter getNotificationSettingsWithCompletionHandler:^(UNNotificationSettings *s) {
		UNAuthorizationStatus st = s.authorizationStatus;
		onMainX(^{
			notifyStatus = st;
			if (then) then(st);
		});
	}];
}

static BOOL allowed(UNAuthorizationStatus st) {
	return st == UNAuthorizationStatusAuthorized || st == UNAuthorizationStatusProvisional
		|| st == UNAuthorizationStatusEphemeral;
}

// ensureAuthorized asks the user when they were never asked and calls
// done with the outcome, on the main thread.
static void ensureAuthorized(void (^done)(BOOL ok)) {
	readNotifyStatus(^(UNAuthorizationStatus st) {
		if (st != UNAuthorizationStatusNotDetermined) {
			done(allowed(st));
			return;
		}
		[UNUserNotificationCenter.currentNotificationCenter
			requestAuthorizationWithOptions:UNAuthorizationOptionAlert | UNAuthorizationOptionSound | UNAuthorizationOptionBadge
			completionHandler:^(BOOL granted, NSError *err) {
				readNotifyStatus(^(UNAuthorizationStatus st2) { done(allowed(st2)); });
			}];
	});
}

static BOOL notifyObserved;

// observeNotifySettings re-reads the status whenever the app becomes
// active (the user may have changed it in Settings meanwhile).
static void observeNotifySettings(void) {
	if (notifyObserved) return;
	notifyObserved = YES;
	[NSNotificationCenter.defaultCenter addObserverForName:UIApplicationDidBecomeActiveNotification object:nil
		queue:NSOperationQueue.mainQueue usingBlock:^(NSNotification *n) { readNotifyStatus(nil); }];
}

void tgx_notify_permission(int tok) {
	observeNotifySettings();
	ensureAuthorized(^(BOOL ok) { finishX(tok, TGM_OK, ok ? @"1" : @"0"); });
}

int tgx_notify(const char *ident, const char *title, const char *subtitle, const char *body, int silent) {
	observeNotifySettings();
	if (notifyStatus == UNAuthorizationStatusDenied) return TGM_PERMISSION_DENIED;
	UNMutableNotificationContent *c = [UNMutableNotificationContent new];
	c.title = [NSString stringWithUTF8String:title] ?: @"";
	c.subtitle = [NSString stringWithUTF8String:subtitle] ?: @"";
	c.body = [NSString stringWithUTF8String:body] ?: @"";
	if (!silent) c.sound = UNNotificationSound.defaultSound;
	NSString *rid = [NSString stringWithUTF8String:ident];
	// No trigger: delivered right away.
	UNNotificationRequest *req = [UNNotificationRequest requestWithIdentifier:rid content:c trigger:nil];
	ensureAuthorized(^(BOOL ok) {
		if (!ok) {
			NSLog(@"tango/ios: notification %@ not shown: notifications are off for this app (Settings > Notifications)", rid);
			return;
		}
		[UNUserNotificationCenter.currentNotificationCenter addNotificationRequest:req withCompletionHandler:^(NSError *err) {
			if (err) NSLog(@"tango/ios: notification %@ not shown: %@", rid, err.localizedDescription);
		}];
	});
	return TGM_OK;
}

// ---------------------------------------------------------------------------
// Network: one path monitor for the process, on its own queue. The latest
// path's facts are kept in atomics so the status can be read from any
// thread; changes are reported on the main thread.

static nw_path_monitor_t monitor;
static dispatch_semaphore_t firstPath;
static atomic_int netOnline, netKind, netExpensive, netConstrained, netHave, netWatch;

// platform.NetworkKind values.
enum { NK_UNKNOWN = 0, NK_NONE = 1, NK_WIRED = 2, NK_WIFI = 3, NK_CELLULAR = 4, NK_OTHER = 5 };

static int pathKind(nw_path_t p, BOOL online) {
	if (!online) return NK_NONE;
	if (nw_path_uses_interface_type(p, nw_interface_type_wifi)) return NK_WIFI;
	if (nw_path_uses_interface_type(p, nw_interface_type_cellular)) return NK_CELLULAR;
	if (nw_path_uses_interface_type(p, nw_interface_type_wired)) return NK_WIRED;
	return NK_OTHER;
}

static void startMonitor(void) {
	static dispatch_once_t once;
	dispatch_once(&once, ^{
		firstPath = dispatch_semaphore_create(0);
		monitor = nw_path_monitor_create();
		dispatch_queue_t q = dispatch_queue_create("tango.network", DISPATCH_QUEUE_SERIAL);
		nw_path_monitor_set_queue(monitor, q);
		nw_path_monitor_set_update_handler(monitor, ^(nw_path_t p) {
			BOOL online = nw_path_get_status(p) == nw_path_status_satisfied;
			int changed = 0;
			changed |= atomic_exchange(&netOnline, online ? 1 : 0) != (online ? 1 : 0);
			int k = pathKind(p, online);
			changed |= atomic_exchange(&netKind, k) != k;
			int e = nw_path_is_expensive(p) ? 1 : 0, c = nw_path_is_constrained(p) ? 1 : 0;
			changed |= atomic_exchange(&netExpensive, e) != e;
			changed |= atomic_exchange(&netConstrained, c) != c;
			if (!atomic_exchange(&netHave, 1)) {
				dispatch_semaphore_signal(firstPath);
				return;
			}
			if (changed && atomic_load(&netWatch)) onMainX(^{ tangoIOSNetworkChanged(); });
		});
		nw_path_monitor_start(monitor);
	});
}

void tgx_network_status(int *online, int *kind, int *expensive, int *constrained) {
	startMonitor();
	if (!atomic_load(&netHave)) {
		// The first path arrives within milliseconds of the start.
		if (dispatch_semaphore_wait(firstPath, dispatch_time(DISPATCH_TIME_NOW, NSEC_PER_SEC)) == 0) {
			dispatch_semaphore_signal(firstPath); // for other waiters
		}
	}
	if (!atomic_load(&netHave)) {
		*online = 0;
		*kind = NK_UNKNOWN;
		*expensive = *constrained = 0;
		return;
	}
	*online = atomic_load(&netOnline);
	*kind = atomic_load(&netKind);
	*expensive = atomic_load(&netExpensive);
	*constrained = atomic_load(&netConstrained);
}

void tgx_network_watch(int on) {
	atomic_store(&netWatch, on ? 1 : 0);
	if (on) startMonitor();
}

// ---------------------------------------------------------------------------
// Power

static BOOL powerObserved, powerWatch;

static void enableBattery(void) {
	UIDevice *d = UIDevice.currentDevice;
	if (!d.batteryMonitoringEnabled) d.batteryMonitoringEnabled = YES;
}

void tgx_power_status(int *state, double *level, int *lowPower) {
	enableBattery();
	UIDevice *d = UIDevice.currentDevice;
	switch (d.batteryState) {
	case UIDeviceBatteryStateUnplugged: *state = 1; break;
	case UIDeviceBatteryStateCharging: *state = 2; break;
	case UIDeviceBatteryStateFull: *state = 3; break;
	default: *state = 0; break;
	}
	float l = d.batteryLevel; // -1 when unknown (the simulator)
	*level = l < 0 ? -1 : (double)l;
	*lowPower = NSProcessInfo.processInfo.isLowPowerModeEnabled ? 1 : 0;
}

void tgx_power_watch(int on) {
	powerWatch = on != 0;
	if (!on || powerObserved) return;
	powerObserved = YES;
	enableBattery();
	void (^changed)(NSNotification *) = ^(NSNotification *n) {
		onMainX(^{
			if (powerWatch) tangoIOSPowerChanged();
		});
	};
	NSNotificationCenter *nc = NSNotificationCenter.defaultCenter;
	[nc addObserverForName:UIDeviceBatteryStateDidChangeNotification object:nil queue:nil usingBlock:changed];
	[nc addObserverForName:UIDeviceBatteryLevelDidChangeNotification object:nil queue:nil usingBlock:changed];
	// Posted on an arbitrary thread.
	[nc addObserverForName:NSProcessInfoPowerStateDidChangeNotification object:nil queue:nil usingBlock:changed];
}
