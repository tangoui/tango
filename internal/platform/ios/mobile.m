//go:build ios

// Mobile capabilities of Tango's iOS backend (share sheet, haptics, safe
// area and orientation, biometrics, Keychain, photo picker and camera,
// location, push, permissions, universal links, web content process
// recovery). Compiled by cgo as Objective-C with ARC into the app's Go
// c-archive, like bridge.m. Results go back to Go (mobile.go) on the main
// thread; Go maps codes and decides policy.
//
// Written against Apple's documentation for UIKit, LocalAuthentication,
// Security, PhotosUI, AVFoundation, CoreLocation and UserNotifications.

#import <UIKit/UIKit.h>
#import <WebKit/WebKit.h>
#import <LocalAuthentication/LocalAuthentication.h>
#import <Security/Security.h>
#import <CoreLocation/CoreLocation.h>
#import <UserNotifications/UserNotifications.h>
#import <UniformTypeIdentifiers/UniformTypeIdentifiers.h>
#import <objc/runtime.h>
#include <dlfcn.h>
#include <stdlib.h>
#include <string.h>

#include "bridge.h"
#include "bridge_private.h"
#include "mobile.h"
#include "mobile2.h"
#include "_cgo_export.h"

static NSString *mstr(const char *s) {
	return s ? [NSString stringWithUTF8String:s] ?: @"" : @"";
}

// onMain runs b on the main thread: now if already there, otherwise as a
// run loop block in the common modes, which (unlike the main dispatch
// queue) also runs inside Tango's nested run loops (tg_step).
static void onMain(dispatch_block_t b) {
	if (NSThread.isMainThread) {
		b();
		return;
	}
	CFRunLoopPerformBlock(CFRunLoopGetMain(), kCFRunLoopCommonModes, b);
	CFRunLoopWakeUp(CFRunLoopGetMain());
}

// finish answers token tok. Main thread.
static void finish(int tok, int code, NSString *s) {
	char *c = strdup(s ? s.UTF8String : "");
	tangoIOSMobileDone(tok, code, c);
	free(c);
}

static void appEvent(int kind, NSString *s) {
	char *c = strdup(s ? s.UTF8String : "");
	tangoIOSAppEvent(kind, c);
	free(c);
}

static BOOL hasUsageKey(NSString *key) {
	id v = [NSBundle.mainBundle objectForInfoDictionaryKey:key];
	return [v isKindOfClass:NSString.class] && [v length] > 0;
}

static NSString *missingKey(NSString *key) {
	return [NSString stringWithFormat:@"Info.plist has no %@", key];
}

// ---------------------------------------------------------------------------
// Frameworks loaded on first use. Every framework the executable links is
// loaded before main: PhotosUI, Photos and AVFoundation added about 1.5 s
// to a cold start on the simulator (measured with TANGO_TRACE_STARTUP;
// PhotosUI alone most of it). So they are neither linked (mobile.go) nor
// imported (with -fmodules an #import would link them again): the few
// classes, constants and methods used are looked up when a capability first
// needs them, and declared below from Apple's documentation.

static void *loadFramework(const char *name) {
	char path[256];
	snprintf(path, sizeof path, "/System/Library/Frameworks/%s.framework/%s", name, name);
	void *h = dlopen(path, RTLD_LAZY | RTLD_GLOBAL);
	if (!h) NSLog(@"tango/ios: cannot load %s: %s", name, dlerror());
	return h;
}

static Class lazyClass(const char *framework, NSString *name) {
	Class c = NSClassFromString(name);
	if (c) return c;
	loadFramework(framework);
	return NSClassFromString(name);
}

// lazyString reads an NSString constant (AVMediaTypeVideo, ...).
static NSString *lazyString(const char *framework, const char *symbol) {
	void *h = loadFramework(framework);
	NSString *__unsafe_unretained *p = h ? (NSString *__unsafe_unretained *)dlsym(h, symbol) : NULL;
	return p ? *p : nil;
}

// PhotosUI: PHPickerConfiguration, PHPickerFilter, PHPickerViewController,
// PHPickerResult.
@protocol TGPickerConfiguration <NSObject>
@property(nonatomic) NSInteger selectionLimit;
@property(nonatomic, copy) id filter;
@property(nonatomic) NSInteger preferredAssetRepresentationMode; // PHPickerConfigurationAssetRepresentationMode
@property(nonatomic) NSInteger selection;                        // PHPickerConfigurationSelection
@end
@protocol TGPickerFilterClass <NSObject>
+ (id)imagesFilter;
+ (id)videosFilter;
+ (id)anyFilterMatchingSubfilters:(NSArray *)subfilters;
@end
@protocol TGPickerControllerClass <NSObject>
- (id)initWithConfiguration:(id)configuration;
@end
@protocol TGPickerController <NSObject>
@property(nonatomic, weak) id delegate;
@end
@protocol TGPickerResult <NSObject>
@property(nonatomic, readonly) NSItemProvider *itemProvider;
@end
enum { TGAssetModeCompatible = 2, TGSelectionOrdered = 1 };

// Photos: +[PHPhotoLibrary authorizationStatusForAccessLevel:] and
// +requestAuthorizationForAccessLevel:handler:. AVFoundation:
// +[AVCaptureDevice authorizationStatusForMediaType:] and
// +requestAccessForMediaType:completionHandler:.
@protocol TGPhotoLibraryClass <NSObject>
+ (NSInteger)authorizationStatusForAccessLevel:(NSInteger)level;
+ (void)requestAuthorizationForAccessLevel:(NSInteger)level handler:(void (^)(NSInteger status))handler;
@end
@protocol TGCaptureDeviceClass <NSObject>
+ (NSInteger)authorizationStatusForMediaType:(NSString *)mediaType;
+ (void)requestAccessForMediaType:(NSString *)mediaType completionHandler:(void (^)(BOOL granted))handler;
@end
enum { TGAccessReadWrite = 2 }; // PHAccessLevelReadWrite
// AVAuthorizationStatus and PHAuthorizationStatus share these values;
// limited is Photos only.
enum { TGAuthNotDetermined = 0, TGAuthRestricted = 1, TGAuthDenied = 2, TGAuthAuthorized = 3, TGAuthLimited = 4 };

#define PHPickerConfigurationClass lazyClass("PhotosUI", @"PHPickerConfiguration")
#define PHPickerFilterClass ((Class<TGPickerFilterClass>)lazyClass("PhotosUI", @"PHPickerFilter"))
#define PHPickerViewControllerClass lazyClass("PhotosUI", @"PHPickerViewController")
#define PHPhotoLibraryClass ((Class<TGPhotoLibraryClass>)lazyClass("Photos", @"PHPhotoLibrary"))
#define AVCaptureDeviceClass ((Class<TGCaptureDeviceClass>)lazyClass("AVFoundation", @"AVCaptureDevice"))
#define AVMediaVideo lazyString("AVFoundation", "AVMediaTypeVideo")
#define AVMediaAudio lazyString("AVFoundation", "AVMediaTypeAudio")

// presentSheet shows vc over the top page. Returns NO without a window.
static BOOL presentSheet(UIViewController *vc) {
	UIViewController *from = tg_top_controller();
	if (!from) return NO;
	UIPopoverPresentationController *pop = vc.popoverPresentationController;
	if (pop) { // iPad: anchor at the bottom centre of the screen
		pop.sourceView = from.view;
		CGRect b = from.view.bounds;
		pop.sourceRect = CGRectMake(CGRectGetMidX(b), CGRectGetMaxY(b) - 1, 1, 1);
		pop.permittedArrowDirections = 0;
	}
	[from presentViewController:vc animated:YES completion:nil];
	return YES;
}

static void dismissSheet(UIViewController *vc, dispatch_block_t then) {
	[vc dismissViewControllerAnimated:YES completion:^{
		tg_presented_ended();
		if (then) then();
	}];
}

// ---------------------------------------------------------------------------
// Share sheet

void tgm_share(int tok, const char *text, const char *url, const char *paths, const char *title) {
	NSMutableArray *items = [NSMutableArray array];
	NSString *t = mstr(text), *u = mstr(url);
	if (t.length) [items addObject:t];
	if (u.length) {
		NSURL *nu = [NSURL URLWithString:u];
		[items addObject:nu ?: u];
	}
	for (NSString *p in [mstr(paths) componentsSeparatedByString:@"\n"]) {
		if (p.length) [items addObject:[NSURL fileURLWithPath:p]];
	}
	UIActivityViewController *vc = [[UIActivityViewController alloc] initWithActivityItems:items applicationActivities:nil];
	NSString *subject = mstr(title);
	if (subject.length) [vc setValue:subject forKey:@"subject"]; // mail subject (documented KVC key)
	__block BOOL answered = NO;
	vc.completionWithItemsHandler = ^(UIActivityType type, BOOL completed, NSArray *returned, NSError *err) {
		if (answered) return;
		answered = YES;
		tg_presented_ended();
		if (err) finish(tok, TGM_ERROR, err.localizedDescription);
		else finish(tok, completed ? TGM_OK : TGM_CANCELED, nil);
	};
	if (!presentSheet(vc)) finish(tok, TGM_NOT_AVAILABLE, @"no page on screen");
}

// ---------------------------------------------------------------------------
// Haptics (platform.HapticKind: 1-3 impacts, 4 selection, 5-7 notification)

void tgm_haptic(int kind) {
	UIView *v = tg_window();
	switch (kind) {
	case 1:
	case 2:
	case 3: {
		UIImpactFeedbackStyle st = kind == 1 ? UIImpactFeedbackStyleLight : kind == 2 ? UIImpactFeedbackStyleMedium : UIImpactFeedbackStyleHeavy;
		UIImpactFeedbackGenerator *g;
		if (@available(iOS 17.5, *)) {
			if (v) g = [UIImpactFeedbackGenerator feedbackGeneratorWithStyle:st forView:v];
		}
#pragma clang diagnostic push
#pragma clang diagnostic ignored "-Wdeprecated-declarations"
		if (!g) g = [[UIImpactFeedbackGenerator alloc] initWithStyle:st];
#pragma clang diagnostic pop
		[g impactOccurred];
		break;
	}
	case 4: {
		UISelectionFeedbackGenerator *g;
		if (@available(iOS 17.5, *)) {
			if (v) g = [UISelectionFeedbackGenerator feedbackGeneratorForView:v];
		}
#pragma clang diagnostic push
#pragma clang diagnostic ignored "-Wdeprecated-declarations"
		if (!g) g = [UISelectionFeedbackGenerator new];
#pragma clang diagnostic pop
		[g selectionChanged];
		break;
	}
	default: {
		UINotificationFeedbackType t = kind == 5 ? UINotificationFeedbackTypeSuccess
			: kind == 6 ? UINotificationFeedbackTypeWarning : UINotificationFeedbackTypeError;
		UINotificationFeedbackGenerator *g;
		if (@available(iOS 17.5, *)) {
			if (v) g = [UINotificationFeedbackGenerator feedbackGeneratorForView:v];
		}
#pragma clang diagnostic push
#pragma clang diagnostic ignored "-Wdeprecated-declarations"
		if (!g) g = [UINotificationFeedbackGenerator new];
#pragma clang diagnostic pop
		[g notificationOccurred:t];
		break;
	}
	}
}

// ---------------------------------------------------------------------------
// Safe area and orientation
//
// A transparent tracker view behind everything in the window follows the
// window's bounds and safe area, also while a page is presented full screen
// (the root controller's view leaves the window then).

static UIInterfaceOrientationMask lockMask; // 0 = not locked

static int orientationOf(UIInterfaceOrientation o) {
	switch (o) {
	case UIInterfaceOrientationPortrait: return 1;
	case UIInterfaceOrientationPortraitUpsideDown: return 2;
	case UIInterfaceOrientationLandscapeRight: return 3; // device turned counter-clockwise
	case UIInterfaceOrientationLandscapeLeft: return 4;
	default: return 0;
	}
}

static UIInterfaceOrientationMask maskOf(int m) {
	UIInterfaceOrientationMask r = 0;
	if (m & 1) r |= UIInterfaceOrientationMaskPortrait;
	if (m & 2) r |= UIInterfaceOrientationMaskPortraitUpsideDown;
	if (m & 4) r |= UIInterfaceOrientationMaskLandscapeRight;
	if (m & 8) r |= UIInterfaceOrientationMaskLandscapeLeft;
	return r;
}

int tgm_orientation(void) {
	UIWindowScene *sc = tg_window().windowScene;
	if (!sc) return 0;
	return orientationOf(sc.effectiveGeometry.interfaceOrientation);
}

void tgm_safe_area(double *top, double *bottom, double *left, double *right) {
	UIEdgeInsets in = tg_window() ? tg_window().safeAreaInsets : UIEdgeInsetsZero;
	*top = in.top;
	*bottom = in.bottom;
	*left = in.left;
	*right = in.right;
}

@interface TGTracker : UIView
@property(nonatomic) UIEdgeInsets lastInsets;
@property(nonatomic) int lastOrientation;
@end

@implementation TGTracker

- (void)check {
	if (!self.window) return;
	UIEdgeInsets in = self.window.safeAreaInsets;
	if (!UIEdgeInsetsEqualToEdgeInsets(in, self.lastInsets)) {
		self.lastInsets = in;
		appEvent(TGE_SAFE_AREA, nil);
	}
	int o = tgm_orientation();
	if (o != self.lastOrientation) {
		self.lastOrientation = o;
		appEvent(TGE_ORIENTATION, nil);
	}
}

- (void)layoutSubviews {
	[super layoutSubviews];
	[self check];
}

- (void)safeAreaInsetsDidChange {
	[super safeAreaInsetsDidChange];
	[self check];
}

@end

static TGTracker *tracker;

static void installTracker(UIWindow *w) {
	if (tracker || !w) return;
	tracker = [[TGTracker alloc] initWithFrame:w.bounds];
	tracker.userInteractionEnabled = NO;
	tracker.backgroundColor = UIColor.clearColor;
	tracker.autoresizingMask = UIViewAutoresizingFlexibleWidth | UIViewAutoresizingFlexibleHeight;
	tracker.lastInsets = w.safeAreaInsets;
	tracker.lastOrientation = tgm_orientation();
	[w insertSubview:tracker atIndex:0];
}

// barStyle maps platform.StatusBarStyle: light content is
// UIStatusBarStyleLightContent, dark content UIStatusBarStyleDarkContent.
static UIStatusBarStyle barStyle(void) {
	switch (tgm2_bar_style) {
	case 1: return UIStatusBarStyleLightContent;
	case 2: return UIStatusBarStyleDarkContent;
	default: return UIStatusBarStyleDefault;
	}
}

@interface TGHost : UIViewController
@end

@implementation TGHost

- (void)viewDidLayoutSubviews {
	[super viewDidLayoutSubviews];
	installTracker(self.view.window);
}

- (UIInterfaceOrientationMask)supportedInterfaceOrientations {
	return lockMask ?: UIInterfaceOrientationMaskAll;
}

// Status bar (device.SetStatusBar, mobile2.m): the host decides while the
// root page is embedded in it, a presented page while it is on top.
- (UIStatusBarStyle)preferredStatusBarStyle {
	return barStyle();
}

- (BOOL)prefersStatusBarHidden {
	return tgm2_bar_hidden != 0;
}

@end

UIViewController *tg_new_host(void) {
	return [TGHost new];
}

// Pages presented full screen decide the orientation and the status bar
// while they are on top.
@implementation TGPage (TGMobile)

- (UIInterfaceOrientationMask)supportedInterfaceOrientations {
	return lockMask ?: UIInterfaceOrientationMaskAll;
}

- (UIStatusBarStyle)preferredStatusBarStyle {
	return barStyle();
}

- (BOOL)prefersStatusBarHidden {
	return tgm2_bar_hidden != 0;
}

@end

void tgm_lock_orientation(int m) {
	lockMask = maskOf(m);
	UIWindow *w = tg_window();
	for (UIViewController *vc = w.rootViewController; vc; vc = vc.presentedViewController) {
		[vc setNeedsUpdateOfSupportedInterfaceOrientations];
	}
	UIWindowScene *sc = w.windowScene;
	if (!sc || !lockMask) return;
	UIWindowSceneGeometryPreferencesIOS *pref = [[UIWindowSceneGeometryPreferencesIOS alloc] initWithInterfaceOrientations:lockMask];
	[sc requestGeometryUpdateWithPreferences:pref errorHandler:^(NSError *err) {
		NSLog(@"tango/ios: orientation lock: %@", err.localizedDescription);
	}];
}

// ---------------------------------------------------------------------------
// Biometrics

static int laCode(NSError *err) {
	switch (err.code) {
	case LAErrorUserCancel:
	case LAErrorAppCancel:
	case LAErrorSystemCancel:
	case LAErrorUserFallback:
		return TGM_CANCELED;
	case LAErrorAuthenticationFailed:
		return TGM_AUTH_FAILED;
	case LAErrorBiometryNotEnrolled:
		return TGM_NOT_ENROLLED;
	case LAErrorBiometryLockout:
		return TGM_LOCKED_OUT;
	case LAErrorBiometryNotAvailable:
	case LAErrorPasscodeNotSet:
		return TGM_NOT_AVAILABLE;
	default:
		return TGM_ERROR;
	}
}

static int biometryKind(LABiometryType t) {
	switch (t) {
	case LABiometryTypeTouchID: return 1;
	case LABiometryTypeFaceID: return 2;
	default:
		if (@available(iOS 17.0, *)) {
			if (t == LABiometryTypeOpticID) return 3;
		}
		return 0;
	}
}

int tgm_biometry(int *kind) {
	LAContext *ctx = [LAContext new];
	NSError *err = nil;
	*kind = 0;
	if (![ctx canEvaluatePolicy:LAPolicyDeviceOwnerAuthenticationWithBiometrics error:&err]) {
		return err ? laCode(err) : TGM_NOT_AVAILABLE;
	}
	*kind = biometryKind(ctx.biometryType);
	return TGM_OK;
}

void tgm_authenticate(int tok, const char *reason, const char *cancel, int passcode) {
	LAContext *ctx = [LAContext new];
	NSError *err = nil;
	LAPolicy policy = passcode ? LAPolicyDeviceOwnerAuthentication : LAPolicyDeviceOwnerAuthenticationWithBiometrics;
	if (![ctx canEvaluatePolicy:policy error:&err]) {
		finish(tok, err ? laCode(err) : TGM_NOT_AVAILABLE, err.localizedDescription);
		return;
	}
	if (ctx.biometryType == LABiometryTypeFaceID && !hasUsageKey(@"NSFaceIDUsageDescription")) {
		finish(tok, TGM_NOT_CONFIGURED, missingKey(@"NSFaceIDUsageDescription"));
		return;
	}
	NSString *c = mstr(cancel);
	if (c.length) ctx.localizedCancelTitle = c;
	[ctx evaluatePolicy:policy localizedReason:mstr(reason) reply:^(BOOL ok, NSError *e) {
		(void)ctx; // keep the context alive until it answers
		onMain(^{
			if (ok) finish(tok, TGM_OK, nil);
			else finish(tok, laCode(e), e.localizedDescription);
		});
	}];
}

// ---------------------------------------------------------------------------
// Keychain: generic passwords of this app, this device only.

static NSMutableDictionary *keyQuery(const char *key) {
	NSString *service = [(NSBundle.mainBundle.bundleIdentifier ?: @"tango") stringByAppendingString:@".tango.securestore"];
	return [@{
		(__bridge id)kSecClass: (__bridge id)kSecClassGenericPassword,
		(__bridge id)kSecAttrService: service,
		(__bridge id)kSecAttrAccount: mstr(key),
	} mutableCopy];
}

static int osCode(OSStatus st) {
	switch (st) {
	case errSecSuccess: return TGM_OK;
	case errSecItemNotFound: return TGM_NOT_FOUND;
	case errSecMissingEntitlement: return TGM_NOT_CONFIGURED;
	default: return TGM_ERROR;
	}
}

int tgm_secure_set(const char *key, const void *p, int n) {
	@autoreleasepool {
		NSData *data = [NSData dataWithBytes:p length:(NSUInteger)n];
		NSMutableDictionary *q = keyQuery(key);
		OSStatus st = SecItemUpdate((__bridge CFDictionaryRef)q, (__bridge CFDictionaryRef)@{(__bridge id)kSecValueData: data});
		if (st == errSecItemNotFound) {
			q[(__bridge id)kSecValueData] = data;
			q[(__bridge id)kSecAttrAccessible] = (__bridge id)kSecAttrAccessibleAfterFirstUnlockThisDeviceOnly;
			st = SecItemAdd((__bridge CFDictionaryRef)q, NULL);
		}
		return osCode(st);
	}
}

int tgm_secure_get(const char *key, void **out, int *n) {
	@autoreleasepool {
		*out = NULL;
		*n = 0;
		NSMutableDictionary *q = keyQuery(key);
		q[(__bridge id)kSecReturnData] = @YES;
		q[(__bridge id)kSecMatchLimit] = (__bridge id)kSecMatchLimitOne;
		CFTypeRef res = NULL;
		OSStatus st = SecItemCopyMatching((__bridge CFDictionaryRef)q, &res);
		if (st != errSecSuccess) return osCode(st);
		NSData *d = CFBridgingRelease(res);
		*n = (int)d.length;
		*out = malloc(d.length ? d.length : 1);
		memcpy(*out, d.bytes, d.length);
		return TGM_OK;
	}
}

int tgm_secure_delete(const char *key) {
	@autoreleasepool {
		OSStatus st = SecItemDelete((__bridge CFDictionaryRef)keyQuery(key));
		return st == errSecItemNotFound ? TGM_OK : osCode(st);
	}
}

// ---------------------------------------------------------------------------
// Photos and camera

static NSURL *mediaDir(void) {
	NSURL *caches = [NSFileManager.defaultManager URLsForDirectory:NSCachesDirectory inDomains:NSUserDomainMask].firstObject;
	NSURL *dir = [caches URLByAppendingPathComponent:@"tango-media" isDirectory:YES];
	[NSFileManager.defaultManager createDirectoryAtURL:dir withIntermediateDirectories:YES attributes:nil error:nil];
	return dir;
}

static NSURL *mediaFile(NSString *ext) {
	NSString *name = NSUUID.UUID.UUIDString;
	if (ext.length) name = [name stringByAppendingPathExtension:ext];
	return [mediaDir() URLByAppendingPathComponent:name];
}

@interface TGPicker : NSObject // PHPickerViewControllerDelegate, added at run time
@property(nonatomic) int tok;
@property(nonatomic) BOOL answered;
@end

static NSMutableSet *liveDelegates; // delegates UIKit holds weakly

@implementation TGPicker

- (void)picker:(UIViewController *)picker didFinishPicking:(NSArray<id<TGPickerResult>> *)results {
	if (self.answered) return;
	self.answered = YES;
	int tok = self.tok;
	dismissSheet(picker, nil);
	[liveDelegates removeObject:self];
	if (results.count == 0) {
		finish(tok, TGM_CANCELED, nil);
		return;
	}
	NSUInteger n = results.count;
	NSMutableArray *paths = [NSMutableArray arrayWithCapacity:n];
	for (NSUInteger i = 0; i < n; i++) [paths addObject:@""];
	__block NSString *failure = nil;
	dispatch_group_t g = dispatch_group_create();
	NSObject *lock = [NSObject new];
	[results enumerateObjectsUsingBlock:^(id<TGPickerResult> r, NSUInteger i, BOOL *stop) {
		NSItemProvider *ip = r.itemProvider;
		NSString *type = nil;
		if ([ip hasItemConformingToTypeIdentifier:UTTypeMovie.identifier]) {
			type = UTTypeMovie.identifier;
		} else {
			for (NSString *t in ip.registeredTypeIdentifiers) {
				UTType *ut = [UTType typeWithIdentifier:t];
				if ([ut conformsToType:UTTypeImage]) {
					type = t;
					break;
				}
			}
			if (!type) type = UTTypeImage.identifier;
		}
		dispatch_group_enter(g);
		[ip loadFileRepresentationForTypeIdentifier:type completionHandler:^(NSURL *url, NSError *err) {
			// url is deleted when this handler returns: copy it now.
			NSString *out = nil;
			if (url) {
				NSURL *dst = mediaFile(url.pathExtension);
				NSError *cerr = nil;
				if ([NSFileManager.defaultManager copyItemAtURL:url toURL:dst error:&cerr]) out = dst.path;
				else err = cerr;
			}
			@synchronized(lock) {
				if (out) paths[i] = out;
				else if (!failure) failure = err.localizedDescription ?: @"could not load the item";
			}
			dispatch_group_leave(g);
		}];
	}];
	dispatch_group_notify(g, dispatch_get_global_queue(QOS_CLASS_USER_INITIATED, 0), ^{
		onMain(^{
			NSMutableArray *ok = [NSMutableArray array];
			for (NSString *p in paths) {
				if (p.length) [ok addObject:p];
			}
			if (!ok.count) finish(tok, TGM_ERROR, failure);
			else finish(tok, TGM_OK, [ok componentsJoinedByString:@"\n"]);
		});
	});
}

@end

void tgm_pick_media(int tok, int types, int limit) {
	id<TGPickerConfiguration> cfg = [PHPickerConfigurationClass new];
	cfg.selectionLimit = limit;
	NSMutableArray *filters = [NSMutableArray array];
	Class<TGPickerFilterClass> filter = PHPickerFilterClass;
	if (types & 1) [filters addObject:[filter imagesFilter]];
	if (types & 2) [filters addObject:[filter videosFilter]];
	if (filters.count) cfg.filter = [filter anyFilterMatchingSubfilters:filters];
	cfg.preferredAssetRepresentationMode = TGAssetModeCompatible;
	cfg.selection = TGSelectionOrdered;
	Protocol *del = objc_getProtocol("PHPickerViewControllerDelegate");
	if (del) class_addProtocol(TGPicker.class, del);
	UIViewController<TGPickerController> *vc = (UIViewController<TGPickerController> *)
		[(id<TGPickerControllerClass>)[PHPickerViewControllerClass alloc] initWithConfiguration:cfg];
	TGPicker *d = [TGPicker new];
	d.tok = tok;
	[liveDelegates addObject:d];
	vc.delegate = d;
	if (!presentSheet(vc)) {
		[liveDelegates removeObject:d];
		finish(tok, TGM_NOT_AVAILABLE, @"no page on screen");
	}
}

@interface TGCamera : NSObject <UIImagePickerControllerDelegate, UINavigationControllerDelegate>
@property(nonatomic) int tok;
@end

@implementation TGCamera

- (void)imagePickerController:(UIImagePickerController *)picker didFinishPickingMediaWithInfo:(NSDictionary<UIImagePickerControllerInfoKey, id> *)info {
	int tok = self.tok;
	[liveDelegates removeObject:self];
	UIImage *img = info[UIImagePickerControllerOriginalImage];
	dismissSheet(picker, nil);
	if (!img) {
		finish(tok, TGM_ERROR, @"the camera returned no image");
		return;
	}
	dispatch_async(dispatch_get_global_queue(QOS_CLASS_USER_INITIATED, 0), ^{
		NSData *jpeg = UIImageJPEGRepresentation(img, 0.9);
		NSURL *dst = mediaFile(@"jpg");
		NSError *err = nil;
		BOOL ok = [jpeg writeToURL:dst options:NSDataWritingAtomic error:&err];
		onMain(^{
			if (ok) finish(tok, TGM_OK, dst.path);
			else finish(tok, TGM_ERROR, err.localizedDescription);
		});
	});
}

- (void)imagePickerControllerDidCancel:(UIImagePickerController *)picker {
	[liveDelegates removeObject:self];
	dismissSheet(picker, nil);
	finish(self.tok, TGM_CANCELED, nil);
}

@end

void tgm_capture_photo(int tok) {
	if (![UIImagePickerController isSourceTypeAvailable:UIImagePickerControllerSourceTypeCamera]) {
		finish(tok, TGM_NOT_AVAILABLE, @"this device has no camera");
		return;
	}
	if (!hasUsageKey(@"NSCameraUsageDescription")) {
		finish(tok, TGM_NOT_CONFIGURED, missingKey(@"NSCameraUsageDescription"));
		return;
	}
	NSInteger st = [AVCaptureDeviceClass authorizationStatusForMediaType:AVMediaVideo];
	if (st == TGAuthDenied || st == TGAuthRestricted) {
		finish(tok, TGM_PERMISSION_DENIED, @"camera access was denied");
		return;
	}
	UIImagePickerController *vc = [UIImagePickerController new];
	vc.sourceType = UIImagePickerControllerSourceTypeCamera;
	vc.cameraCaptureMode = UIImagePickerControllerCameraCaptureModePhoto;
	TGCamera *d = [TGCamera new];
	d.tok = tok;
	[liveDelegates addObject:d];
	vc.delegate = d;
	if (!presentSheet(vc)) {
		[liveDelegates removeObject:d];
		finish(tok, TGM_NOT_AVAILABLE, @"no page on screen");
	}
}

// ---------------------------------------------------------------------------
// Location
//
// TGLocator wraps one CLLocationManager: one for single fixes (and
// permission requests), one for continuous updates.

typedef void (^TGAuthDone)(CLAuthorizationStatus st);

@interface TGLocator : NSObject <CLLocationManagerDelegate>
@property(nonatomic, strong) CLLocationManager *mgr;
@property(nonatomic, strong) NSMutableArray<NSNumber *> *once; // tokens waiting for a fix
@property(nonatomic) int updates;                             // token of the update session, 0 = none
@property(nonatomic, strong) NSMutableArray<TGAuthDone> *authWaiters;
@property(nonatomic) BOOL prompted;                          // the app resigned active after a request
@end

static TGLocator *onceLocator, *updateLocator;

static int clCode(NSError *err) {
	if ([err.domain isEqualToString:kCLErrorDomain] && err.code == kCLErrorDenied) return TGM_PERMISSION_DENIED;
	return TGM_ERROR;
}

static void sendLocation(int tok, int code, NSString *msg, CLLocation *l) {
	tg_location loc = {0};
	if (l) {
		loc.lat = l.coordinate.latitude;
		loc.lon = l.coordinate.longitude;
		loc.alt = l.altitude;
		loc.acc = l.horizontalAccuracy;
		loc.alt_acc = l.verticalAccuracy;
		loc.speed = l.speed;
		loc.course = l.course;
		loc.time = l.timestamp.timeIntervalSince1970;
	}
	char *c = strdup(msg ? msg.UTF8String : "");
	tangoIOSLocation(tok, code, c, &loc);
	free(c);
}

static BOOL authorized(CLAuthorizationStatus st) {
	return st == kCLAuthorizationStatusAuthorizedWhenInUse || st == kCLAuthorizationStatusAuthorizedAlways;
}

@implementation TGLocator

- (instancetype)init {
	if ((self = [super init])) {
		_mgr = [CLLocationManager new];
		_mgr.delegate = self;
		_once = [NSMutableArray array];
		_authWaiters = [NSMutableArray array];
	}
	return self;
}

- (void)finishAuth {
	NSArray *ws = [self.authWaiters copy];
	[self.authWaiters removeAllObjects];
	for (TGAuthDone w in ws) w(self.mgr.authorizationStatus);
}

// requestAuth asks for when-in-use (or always) permission and calls done
// with the outcome. When iOS shows no prompt (it already asked, or only
// allows one upgrade to always) nothing reports back, so: no resign-active
// within a second means no prompt; otherwise wait for the app to become
// active again.
- (void)requestAuth:(BOOL)always done:(TGAuthDone)done {
	NSString *key = always ? @"NSLocationAlwaysAndWhenInUseUsageDescription" : @"NSLocationWhenInUseUsageDescription";
	if (!hasUsageKey(@"NSLocationWhenInUseUsageDescription") || !hasUsageKey(key)) {
		done((CLAuthorizationStatus)-1);
		return;
	}
	[self.authWaiters addObject:[done copy]];
	if (self.authWaiters.count > 1) return; // a prompt is already pending
	self.prompted = NO;
	NSNotificationCenter *nc = NSNotificationCenter.defaultCenter;
	__block id resign = nil, active = nil;
	__weak TGLocator *weak = self;
	void (^cleanup)(void) = ^{
		if (resign) [nc removeObserver:resign];
		if (active) [nc removeObserver:active];
		resign = active = nil;
	};
	resign = [nc addObserverForName:UIApplicationWillResignActiveNotification object:nil queue:nil usingBlock:^(NSNotification *n) {
		weak.prompted = YES;
	}];
	active = [nc addObserverForName:UIApplicationDidBecomeActiveNotification object:nil queue:nil usingBlock:^(NSNotification *n) {
		if (!weak.prompted) return;
		cleanup();
		// The authorization callback may follow the activation slightly.
		dispatch_after(dispatch_time(DISPATCH_TIME_NOW, 300 * NSEC_PER_MSEC), dispatch_get_main_queue(), ^{ [weak finishAuth]; });
	}];
	dispatch_after(dispatch_time(DISPATCH_TIME_NOW, NSEC_PER_SEC), dispatch_get_main_queue(), ^{
		if (weak.prompted || !weak.authWaiters.count) return;
		cleanup();
		[weak finishAuth];
	});
	if (always) [self.mgr requestAlwaysAuthorization];
	else [self.mgr requestWhenInUseAuthorization];
}

- (void)locationManagerDidChangeAuthorization:(CLLocationManager *)m {
	if (self.authWaiters.count && m.authorizationStatus != kCLAuthorizationStatusNotDetermined && self.prompted) {
		[self finishAuth];
	}
}

// withAuth runs then once when-in-use permission is settled.
- (void)withAuth:(void (^)(int code))then {
	CLAuthorizationStatus st = self.mgr.authorizationStatus;
	if (authorized(st)) {
		then(TGM_OK);
		return;
	}
	if (st != kCLAuthorizationStatusNotDetermined) {
		then(TGM_PERMISSION_DENIED);
		return;
	}
	[self requestAuth:NO done:^(CLAuthorizationStatus s) {
		if ((int)s == -1) then(TGM_NOT_CONFIGURED);
		else then(authorized(s) ? TGM_OK : TGM_PERMISSION_DENIED);
	}];
}

- (void)locationManager:(CLLocationManager *)m didUpdateLocations:(NSArray<CLLocation *> *)locs {
	CLLocation *l = locs.lastObject;
	if (!l) return;
	if (self.once.count) {
		NSArray *toks = [self.once copy];
		[self.once removeAllObjects];
		for (NSNumber *t in toks) sendLocation(t.intValue, TGM_OK, nil, l);
	}
	if (self.updates) {
		for (CLLocation *x in locs) sendLocation(self.updates, TGM_OK, nil, x);
	}
}

- (void)locationManager:(CLLocationManager *)m didFailWithError:(NSError *)err {
	if (self.updates) {
		if ([err.domain isEqualToString:kCLErrorDomain] && err.code == kCLErrorLocationUnknown) return; // transient
		int tok = self.updates;
		self.updates = 0;
		[m stopUpdatingLocation];
		sendLocation(tok, clCode(err), err.localizedDescription, nil);
		return;
	}
	NSArray *toks = [self.once copy];
	[self.once removeAllObjects];
	for (NSNumber *t in toks) sendLocation(t.intValue, clCode(err), err.localizedDescription, nil);
}

@end

static CLLocationAccuracy accuracyOf(int a) {
	switch (a) {
	case 1: return kCLLocationAccuracyBest;
	case 2: return kCLLocationAccuracyThreeKilometers;
	default: return kCLLocationAccuracyHundredMeters;
	}
}

static NSString *configMsg(void) { return missingKey(@"NSLocationWhenInUseUsageDescription"); }

void tgm_location_once(int tok, int accuracy) {
	if (!onceLocator) onceLocator = [TGLocator new];
	TGLocator *L = onceLocator;
	[L withAuth:^(int code) {
		if (code != TGM_OK) {
			sendLocation(tok, code, code == TGM_NOT_CONFIGURED ? configMsg() : @"location access was denied", nil);
			return;
		}
		L.mgr.desiredAccuracy = accuracyOf(accuracy);
		[L.once addObject:@(tok)];
		[L.mgr requestLocation];
	}];
}

void tgm_location_start(int tok, int accuracy, double distance) {
	if (!updateLocator) updateLocator = [TGLocator new];
	TGLocator *L = updateLocator;
	L.updates = tok;
	[L withAuth:^(int code) {
		if (L.updates != tok) return; // replaced or stopped meanwhile
		if (code != TGM_OK) {
			L.updates = 0;
			sendLocation(tok, code, code == TGM_NOT_CONFIGURED ? configMsg() : @"location access was denied", nil);
			return;
		}
		L.mgr.desiredAccuracy = accuracyOf(accuracy);
		L.mgr.distanceFilter = distance > 0 ? distance : kCLDistanceFilterNone;
		[L.mgr startUpdatingLocation];
	}];
}

void tgm_location_stop(void) {
	if (!updateLocator) return;
	updateLocator.updates = 0;
	[updateLocator.mgr stopUpdatingLocation];
}

// ---------------------------------------------------------------------------
// Push notifications

@interface TGPush : NSObject <UNUserNotificationCenterDelegate>
@end

static TGPush *pushDelegate;
static NSMutableArray<NSNumber *> *pushWaiters;

static NSString *pushJSON(UNNotificationContent *c, NSDictionary *userInfo, NSString *action) {
	NSMutableDictionary *p = [NSMutableDictionary dictionary];
	if (c.title.length) p[@"title"] = c.title;
	if (c.body.length) p[@"body"] = c.body;
	if (action.length) p[@"action"] = action;
	if (userInfo && [NSJSONSerialization isValidJSONObject:userInfo]) p[@"data"] = userInfo;
	NSData *d = [NSJSONSerialization dataWithJSONObject:p options:0 error:nil];
	return d ? [[NSString alloc] initWithData:d encoding:NSUTF8StringEncoding] : @"{}";
}

static BOOL isRemote(UNNotification *n) {
	return [n.request.trigger isKindOfClass:UNPushNotificationTrigger.class];
}

@implementation TGPush

- (void)userNotificationCenter:(UNUserNotificationCenter *)center willPresentNotification:(UNNotification *)n
	withCompletionHandler:(void (^)(UNNotificationPresentationOptions))done {
	if (isRemote(n)) appEvent(TGE_PUSH_RECEIVED, pushJSON(n.request.content, n.request.content.userInfo, nil));
	done(UNNotificationPresentationOptionBanner | UNNotificationPresentationOptionList | UNNotificationPresentationOptionSound);
}

- (void)userNotificationCenter:(UNUserNotificationCenter *)center didReceiveNotificationResponse:(UNNotificationResponse *)r
	withCompletionHandler:(void (^)(void))done {
	if (isRemote(r.notification)) {
		NSString *action = [r.actionIdentifier isEqualToString:UNNotificationDefaultActionIdentifier] ? @"" : r.actionIdentifier;
		appEvent(TGE_PUSH_OPENED, pushJSON(r.notification.request.content, r.notification.request.content.userInfo, action));
	}
	done();
}

@end

static void pushAnswer(int code, NSString *s) {
	NSArray *ws = [pushWaiters copy];
	[pushWaiters removeAllObjects];
	for (NSNumber *t in ws) finish(t.intValue, code, s);
}

// Called by the host when its app delegate implements the push callbacks
// itself (otherwise Tango adds them to the delegate class).
void TangoDidRegisterForRemoteNotifications(NSData *token, NSError *err) {
	onMain(^{
		if (err || !token) {
			pushAnswer(TGM_ERROR, err.localizedDescription ?: @"registration failed");
			return;
		}
		const unsigned char *b = token.bytes;
		NSMutableString *hex = [NSMutableString stringWithCapacity:token.length * 2];
		for (NSUInteger i = 0; i < token.length; i++) [hex appendFormat:@"%02x", b[i]];
		appEvent(TGE_PUSH_TOKEN, hex);
		pushAnswer(TGM_OK, hex);
	});
}

void tgm_push_register(int tok) {
	[pushWaiters addObject:@(tok)];
	if (pushWaiters.count > 1) return;
	UNUserNotificationCenter *c = UNUserNotificationCenter.currentNotificationCenter;
	[c requestAuthorizationWithOptions:UNAuthorizationOptionAlert | UNAuthorizationOptionSound | UNAuthorizationOptionBadge
		completionHandler:^(BOOL granted, NSError *err) {
			// The token is issued whatever the user chose (silent pushes need
			// no permission).
			onMain(^{ [UIApplication.sharedApplication registerForRemoteNotifications]; });
		}];
}

// ---------------------------------------------------------------------------
// Links: custom URL schemes and universal links (NSUserActivity), from the
// scene delegate (scene apps) or the app delegate (no scene manifest).

static void openURL(NSURL *u) {
	if (u.absoluteString.length) appEvent(TGE_OPEN_URL, u.absoluteString);
}

static void continueActivity(NSUserActivity *a) {
	if ([a.activityType isEqualToString:NSUserActivityTypeBrowsingWeb] && a.webpageURL) openURL(a.webpageURL);
}

void tg_scene_connected(UISceneConnectionOptions *options) {
	for (UIOpenURLContext *c in options.URLContexts) openURL(c.URL);
	for (NSUserActivity *a in options.userActivities) continueActivity(a);
}

@implementation TangoSceneDelegate (TGLinks)

- (void)scene:(UIScene *)scene openURLContexts:(NSSet<UIOpenURLContext *> *)contexts {
	for (UIOpenURLContext *c in contexts) openURL(c.URL);
}

- (void)scene:(UIScene *)scene continueUserActivity:(NSUserActivity *)activity {
	continueActivity(activity);
}

@end

// App delegate methods added at run time when the host's delegate lacks
// them, so mobile/ios/main.m (or an Xcode project's AppDelegate) needs no
// forwarding code.

static void appDidRegister(id self, SEL _cmd, UIApplication *app, NSData *token) {
	TangoDidRegisterForRemoteNotifications(token, nil);
}

static void appDidFailToRegister(id self, SEL _cmd, UIApplication *app, NSError *err) {
	TangoDidRegisterForRemoteNotifications(nil, err);
}

static void appDidReceiveRemote(id self, SEL _cmd, UIApplication *app, NSDictionary *info, void (^done)(UIBackgroundFetchResult)) {
	UNMutableNotificationContent *c = [UNMutableNotificationContent new];
	appEvent(TGE_PUSH_RECEIVED, pushJSON(c, info, nil));
	done(UIBackgroundFetchResultNoData);
}

static BOOL appOpenURL(id self, SEL _cmd, UIApplication *app, NSURL *url, NSDictionary *opts) {
	openURL(url);
	return YES;
}

static BOOL appContinue(id self, SEL _cmd, UIApplication *app, NSUserActivity *a, id handler) {
	continueActivity(a);
	return YES;
}

void tg_mobile_install(void) {
	liveDelegates = [NSMutableSet set];
	pushWaiters = [NSMutableArray array];

	// Notification taps that launched the app are delivered to the delegate
	// only if it is set before didFinishLaunching returns (TangoStart runs
	// from there). A host that set its own delegate keeps it.
	UNUserNotificationCenter *c = UNUserNotificationCenter.currentNotificationCenter;
	if (!c.delegate) {
		pushDelegate = [TGPush new];
		c.delegate = pushDelegate;
	}

	Class cls = [UIApplication.sharedApplication.delegate class];
	if (cls) {
		class_addMethod(cls, @selector(application:didRegisterForRemoteNotificationsWithDeviceToken:), (IMP)appDidRegister, "v@:@@");
		class_addMethod(cls, @selector(application:didFailToRegisterForRemoteNotificationsWithError:), (IMP)appDidFailToRegister, "v@:@@");
		class_addMethod(cls, @selector(application:didReceiveRemoteNotification:fetchCompletionHandler:), (IMP)appDidReceiveRemote, "v@:@@@?");
		class_addMethod(cls, @selector(application:openURL:options:), (IMP)appOpenURL, "B@:@@@");
		class_addMethod(cls, @selector(application:continueUserActivity:restorationHandler:), (IMP)appContinue, "B@:@@@?");
	}
	// Background task handlers must be registered before didFinishLaunching
	// returns; keyboard observers.
	tgm2_install();
}

// ---------------------------------------------------------------------------
// Web content process recovery. WebKit calls this when the page's web
// content process died (memory pressure, crash); the web view then shows
// nothing. Go reports it and reloads the page.

static const void *lastURLKey = &lastURLKey;

@implementation TGPage (TGRecovery)

- (void)webView:(WKWebView *)webView didCommitNavigation:(WKNavigation *)navigation {
	objc_setAssociatedObject(self, lastURLKey, webView.URL, OBJC_ASSOCIATION_RETAIN_NONATOMIC);
}

- (void)webViewWebContentProcessDidTerminate:(WKWebView *)webView {
	NSURL *u = webView.URL ?: objc_getAssociatedObject(self, lastURLKey);
	char *c = strdup(u.absoluteString.UTF8String ?: "");
	tangoIOSWebViewTerminated(self.sid, c);
	free(c);
}

@end

// ---------------------------------------------------------------------------
// Permissions

static NSString *avStatus(NSInteger st) {
	switch (st) {
	case TGAuthAuthorized: return @"granted";
	case TGAuthNotDetermined: return @"notDetermined";
	default: return @"denied";
	}
}

static NSString *photoStatus(NSInteger st) {
	switch (st) {
	case TGAuthAuthorized: return @"granted";
	case TGAuthLimited: return @"limited";
	case TGAuthNotDetermined: return @"notDetermined";
	default: return @"denied";
	}
}

static NSString *locationStatus(CLLocationManager *m, BOOL always) {
	switch (m.authorizationStatus) {
	case kCLAuthorizationStatusNotDetermined: return @"notDetermined";
	case kCLAuthorizationStatusAuthorizedAlways:
		return m.accuracyAuthorization == CLAccuracyAuthorizationReducedAccuracy ? @"limited" : @"granted";
	case kCLAuthorizationStatusAuthorizedWhenInUse:
		if (always) return @"limited"; // in use only; Request may offer the upgrade once
		return m.accuracyAuthorization == CLAccuracyAuthorizationReducedAccuracy ? @"limited" : @"granted";
	default: return @"denied";
	}
}

static NSString *notifyStatus(UNAuthorizationStatus st) {
	switch (st) {
	case UNAuthorizationStatusAuthorized: return @"granted";
	case UNAuthorizationStatusProvisional:
	case UNAuthorizationStatusEphemeral: return @"limited";
	case UNAuthorizationStatusNotDetermined: return @"notDetermined";
	default: return @"denied";
	}
}

static void avPermission(int tok, NSString *type, NSString *key, BOOL request) {
	NSInteger st = [AVCaptureDeviceClass authorizationStatusForMediaType:type];
	if (!request || st != TGAuthNotDetermined) {
		finish(tok, TGM_OK, avStatus(st));
		return;
	}
	if (!hasUsageKey(key)) {
		finish(tok, TGM_NOT_CONFIGURED, missingKey(key));
		return;
	}
	[AVCaptureDeviceClass requestAccessForMediaType:type completionHandler:^(BOOL granted) {
		onMain(^{ finish(tok, TGM_OK, avStatus([AVCaptureDeviceClass authorizationStatusForMediaType:type])); });
	}];
}

void tgm_permission(int tok, const char *cname, int request) {
	NSString *name = mstr(cname);
	if ([name isEqualToString:@"camera"]) {
		avPermission(tok, AVMediaVideo, @"NSCameraUsageDescription", request);
	} else if ([name isEqualToString:@"microphone"]) {
		avPermission(tok, AVMediaAudio, @"NSMicrophoneUsageDescription", request);
	} else if ([name isEqualToString:@"photos"]) {
		NSInteger st = [PHPhotoLibraryClass authorizationStatusForAccessLevel:TGAccessReadWrite];
		if (!request || st != TGAuthNotDetermined) {
			finish(tok, TGM_OK, photoStatus(st));
		} else if (!hasUsageKey(@"NSPhotoLibraryUsageDescription")) {
			finish(tok, TGM_NOT_CONFIGURED, missingKey(@"NSPhotoLibraryUsageDescription"));
		} else {
			[PHPhotoLibraryClass requestAuthorizationForAccessLevel:TGAccessReadWrite handler:^(NSInteger s) {
				onMain(^{ finish(tok, TGM_OK, photoStatus(s)); });
			}];
		}
	} else if ([name isEqualToString:@"location"] || [name isEqualToString:@"location-always"]) {
		BOOL always = [name isEqualToString:@"location-always"];
		if (!onceLocator) onceLocator = [TGLocator new];
		TGLocator *L = onceLocator;
		CLAuthorizationStatus st = L.mgr.authorizationStatus;
		BOOL canAsk = st == kCLAuthorizationStatusNotDetermined || (always && st == kCLAuthorizationStatusAuthorizedWhenInUse);
		if (!request || !canAsk) {
			finish(tok, TGM_OK, locationStatus(L.mgr, always));
			return;
		}
		[L requestAuth:always done:^(CLAuthorizationStatus s) {
			if ((int)s == -1) {
				finish(tok, TGM_NOT_CONFIGURED, missingKey(always ? @"NSLocationAlwaysAndWhenInUseUsageDescription" : @"NSLocationWhenInUseUsageDescription"));
				return;
			}
			finish(tok, TGM_OK, locationStatus(L.mgr, always));
		}];
	} else if ([name isEqualToString:@"notifications"]) {
		UNUserNotificationCenter *c = UNUserNotificationCenter.currentNotificationCenter;
		[c getNotificationSettingsWithCompletionHandler:^(UNNotificationSettings *s) {
			if (!request || s.authorizationStatus != UNAuthorizationStatusNotDetermined) {
				onMain(^{ finish(tok, TGM_OK, notifyStatus(s.authorizationStatus)); });
				return;
			}
			[c requestAuthorizationWithOptions:UNAuthorizationOptionAlert | UNAuthorizationOptionSound | UNAuthorizationOptionBadge
				completionHandler:^(BOOL granted, NSError *err) {
					[c getNotificationSettingsWithCompletionHandler:^(UNNotificationSettings *s2) {
						onMain(^{ finish(tok, TGM_OK, notifyStatus(s2.authorizationStatus)); });
					}];
				}];
		}];
	} else {
		finish(tok, TGM_UNSUPPORTED, nil);
	}
}
