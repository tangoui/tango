//go:build ios

// Round-4 mobile capabilities of Tango's iOS backend: background tasks
// (BGTaskScheduler), in-app purchase (StoreKit's SKPaymentQueue API, the one
// Objective-C can use), store review, the app's settings page, screen
// (idle timer, brightness), status bar and keyboard. Go half: mobile2.go.
//
// StoreKit and BackgroundTasks are not linked: StoreKit alone added about
// two seconds to a cold start on the simulator (every linked framework is
// loaded before main). They are loaded when first needed and their API is
// declared below from Apple's documentation instead of imported (with
// -fmodules an #import would link the framework again).

#import <UIKit/UIKit.h>
#import <objc/runtime.h>
#include <dlfcn.h>
#include <stdlib.h>
#include <string.h>

#include "bridge_private.h"
#include "mobile.h"
#include "mobile2.h"
#include "sysx.h"
#include "_cgo_export.h"

static void *loadFramework2(const char *name) {
	char path[256];
	snprintf(path, sizeof path, "/System/Library/Frameworks/%s.framework/%s", name, name);
	void *h = dlopen(path, RTLD_LAZY | RTLD_GLOBAL);
	if (!h) NSLog(@"tango/ios: cannot load %s: %s", name, dlerror());
	return h;
}

static Class fwClass(const char *framework, NSString *name) {
	Class c = NSClassFromString(name);
	if (c) return c;
	loadFramework2(framework);
	return NSClassFromString(name);
}

static void finish2(int tok, int code, NSString *s) {
	char *c = strdup(s ? s.UTF8String ?: "" : "");
	tangoIOSMobileDone(tok, code, c);
	free(c);
}

// onMain2 runs b on the main thread in the common modes (see onMain in
// mobile.m).
static void onMain2(dispatch_block_t b) {
	if (NSThread.isMainThread) {
		b();
		return;
	}
	CFRunLoopPerformBlock(CFRunLoopGetMain(), kCFRunLoopCommonModes, b);
	CFRunLoopWakeUp(CFRunLoopGetMain());
}

static NSString *jsonString(id obj) {
	NSData *d = [NSJSONSerialization dataWithJSONObject:obj options:0 error:nil];
	return d ? [[NSString alloc] initWithData:d encoding:NSUTF8StringEncoding] : @"{}";
}

// ---------------------------------------------------------------------------
// Background tasks (BackgroundTasks framework)

@protocol TGBGTask <NSObject>
@property(nonatomic, readonly, copy) NSString *identifier;
@property(nonatomic, strong) void (^expirationHandler)(void);
- (void)setTaskCompletedWithSuccess:(BOOL)success;
@end
@protocol TGBGRequest <NSObject>
@property(nonatomic, copy) NSDate *earliestBeginDate;
@end
@protocol TGBGProcessingRequest <TGBGRequest>
@property(nonatomic) BOOL requiresNetworkConnectivity;
@property(nonatomic) BOOL requiresExternalPower;
@end
@protocol TGBGRequestClass <NSObject>
- (id)initWithIdentifier:(NSString *)identifier;
@end
@protocol TGBGScheduler <NSObject>
- (BOOL)registerForTaskWithIdentifier:(NSString *)identifier usingQueue:(dispatch_queue_t)queue
	launchHandler:(void (^)(id<TGBGTask> task))launchHandler;
- (BOOL)submitTaskRequest:(id)request error:(NSError **)error;
- (void)cancelTaskRequestWithIdentifier:(NSString *)identifier;
@end
@protocol TGBGSchedulerClass <NSObject>
+ (id<TGBGScheduler>)sharedScheduler;
@end
// BGTaskSchedulerErrorCode
enum { TGBGUnavailable = 1, TGBGTooManyPending = 2, TGBGNotPermitted = 3 };

static NSMutableDictionary<NSNumber *, id<TGBGTask>> *bgTasks; // running, by id
static int64_t lastBGTask;
static BOOL bgRegistered[3];

static NSString *bgIdentifier(int kind) {
	NSString *bid = NSBundle.mainBundle.bundleIdentifier ?: @"";
	return [bid stringByAppendingString:kind == 2 ? @".tango.processing" : @".tango.refresh"];
}

static BOOL bgPermitted(NSString *ident) {
	NSArray *ids = [NSBundle.mainBundle objectForInfoDictionaryKey:@"BGTaskSchedulerPermittedIdentifiers"];
	return [ids isKindOfClass:NSArray.class] && [ids containsObject:ident];
}

static id<TGBGScheduler> bgScheduler(void) {
	Class c = fwClass("BackgroundTasks", @"BGTaskScheduler");
	return c ? [(Class<TGBGSchedulerClass>)c sharedScheduler] : nil;
}

static void bgReport(int64_t tid, int kind, int expired) {
	tangoIOSBackgroundTask(tid, kind, expired);
}

// bgRegister registers the launch handlers. It must run before
// didFinishLaunching returns, and only for identifiers the Info.plist
// permits (BGTaskScheduler raises otherwise).
static void bgRegister(void) {
	bgTasks = [NSMutableDictionary dictionary];
	for (int kind = 1; kind <= 2; kind++) {
		NSString *ident = bgIdentifier(kind);
		if (!bgPermitted(ident)) continue;
		id<TGBGScheduler> s = bgScheduler();
		if (!s) return;
		bgRegistered[kind] = [s registerForTaskWithIdentifier:ident usingQueue:nil launchHandler:^(id<TGBGTask> task) {
			onMain2(^{
				int64_t tid = ++lastBGTask;
				bgTasks[@(tid)] = task;
				task.expirationHandler = ^{
					onMain2(^{
						if (bgTasks[@(tid)]) bgReport(tid, kind, 1);
					});
				};
				bgReport(tid, kind, 0);
			});
		}];
	}
}

int tgm2_bg_schedule(int kind, double earliest, int network, int power, char **err) {
	*err = NULL;
	if (kind != 1 && kind != 2) return TGM_UNSUPPORTED;
	NSString *ident = bgIdentifier(kind);
	if (!bgPermitted(ident) || !bgRegistered[kind]) {
		*err = strdup([NSString stringWithFormat:@"Info.plist BGTaskSchedulerPermittedIdentifiers lacks %@ "
			"(declare \"background\" in tan.json \"permissions\")", ident].UTF8String);
		return TGM_NOT_CONFIGURED;
	}
	Class cls = fwClass("BackgroundTasks", kind == 2 ? @"BGProcessingTaskRequest" : @"BGAppRefreshTaskRequest");
	id<TGBGProcessingRequest> req = (id<TGBGProcessingRequest>)[(id<TGBGRequestClass>)[cls alloc] initWithIdentifier:ident];
	if (earliest > 0) req.earliestBeginDate = [NSDate dateWithTimeIntervalSinceNow:earliest];
	if (kind == 2) {
		req.requiresNetworkConnectivity = network != 0;
		req.requiresExternalPower = power != 0;
	}
	NSError *e = nil;
	if ([bgScheduler() submitTaskRequest:req error:&e]) return TGM_OK;
	*err = strdup((e.localizedDescription ?: @"submit failed").UTF8String);
	switch (e.code) {
	case TGBGUnavailable: return TGM_NOT_AVAILABLE; // simulator, Background App Refresh off
	case TGBGNotPermitted: return TGM_NOT_CONFIGURED;
	default: return TGM_ERROR;
	}
}

int tgm2_bg_cancel(int kind) {
	if (kind != 1 && kind != 2) return TGM_UNSUPPORTED;
	if (bgRegistered[kind]) [bgScheduler() cancelTaskRequestWithIdentifier:bgIdentifier(kind)];
	return TGM_OK;
}

void tgm2_bg_finish(int64_t tid, int success) {
	id<TGBGTask> t = bgTasks[@(tid)];
	if (!t) return;
	[bgTasks removeObjectForKey:@(tid)];
	[t setTaskCompletedWithSuccess:success != 0];
}

// ---------------------------------------------------------------------------
// In-app purchase (StoreKit: SKProductsRequest, SKPaymentQueue)

@protocol TGSKProduct <NSObject>
@property(nonatomic, readonly) NSString *productIdentifier;
@property(nonatomic, readonly) NSString *localizedTitle;
@property(nonatomic, readonly) NSString *localizedDescription;
@property(nonatomic, readonly) NSDecimalNumber *price;
@property(nonatomic, readonly) NSLocale *priceLocale;
@property(nonatomic, readonly) id subscriptionPeriod;
@end
@protocol TGSKProductsResponse <NSObject>
@property(nonatomic, readonly) NSArray<id<TGSKProduct>> *products;
@property(nonatomic, readonly) NSArray<NSString *> *invalidProductIdentifiers;
@end
@protocol TGSKProductsRequest <NSObject>
@property(nonatomic, weak) id delegate;
- (id)initWithProductIdentifiers:(NSSet<NSString *> *)ids;
- (void)start;
@end
@protocol TGSKPayment <NSObject>
@property(nonatomic, readonly) NSString *productIdentifier;
@property(nonatomic, readonly) NSInteger quantity;
@end
@protocol TGSKMutablePayment <TGSKPayment>
@property(nonatomic, readwrite) NSInteger quantity;
@end
@protocol TGSKMutablePaymentClass <NSObject>
+ (id)paymentWithProduct:(id)product;
@end
@protocol TGSKTransaction <NSObject>
@property(nonatomic, readonly) NSError *error;
@property(nonatomic, readonly) id<TGSKTransaction> originalTransaction;
@property(nonatomic, readonly) id<TGSKPayment> payment;
@property(nonatomic, readonly) NSDate *transactionDate;
@property(nonatomic, readonly) NSString *transactionIdentifier;
@property(nonatomic, readonly) NSInteger transactionState; // SKPaymentTransactionState
@end
@protocol TGSKPaymentQueue <NSObject>
- (void)addTransactionObserver:(id)observer;
- (void)addPayment:(id)payment;
- (void)finishTransaction:(id)transaction;
- (void)restoreCompletedTransactions;
@end
@protocol TGSKPaymentQueueClass <NSObject>
+ (id<TGSKPaymentQueue>)defaultQueue;
+ (BOOL)canMakePayments;
@end
@protocol TGSKReviewClass <NSObject>
+ (void)requestReviewInScene:(UIWindowScene *)scene;
@end
// SKPaymentTransactionState; platform.TransactionState is one more.
enum { TGSKPurchasing = 0, TGSKPurchased = 1, TGSKFailed = 2, TGSKRestored = 3, TGSKDeferred = 4 };
enum { TGSKErrorPaymentCancelled = 2 };

@interface TGStore : NSObject // SKPaymentTransactionObserver, SKProductsRequestDelegate (added at run time)
@end

static TGStore *store;
static NSMutableDictionary<NSString *, id<TGSKProduct>> *knownProducts;
static NSMapTable<id, NSNumber *> *buyTokens;       // SKPayment -> token
static NSMapTable<id, NSNumber *> *productRequests; // SKProductsRequest -> token
static NSMutableDictionary<NSString *, id> *unfinished; // transactionIdentifier -> transaction
static NSMutableArray<NSNumber *> *restoreTokens;

static Class skClass(NSString *name) { return fwClass("StoreKit", name); }

static Class<TGSKPaymentQueueClass> skQueueClass(void) {
	return (Class<TGSKPaymentQueueClass>)skClass(@"SKPaymentQueue");
}

// storeReady loads StoreKit and installs the transaction observer once.
// Main thread. From then on transactions left over from earlier runs
// (interrupted purchases, Ask to Buy approvals, renewals) are reported.
static BOOL storeReady(void) {
	if (store) return YES;
	Class<TGSKPaymentQueueClass> q = skQueueClass();
	if (!q) return NO;
	Protocol *p1 = objc_getProtocol("SKPaymentTransactionObserver"), *p2 = objc_getProtocol("SKProductsRequestDelegate");
	if (p1) class_addProtocol(TGStore.class, p1);
	if (p2) class_addProtocol(TGStore.class, p2);
	knownProducts = [NSMutableDictionary dictionary];
	buyTokens = [NSMapTable strongToStrongObjectsMapTable];
	productRequests = [NSMapTable strongToStrongObjectsMapTable];
	unfinished = [NSMutableDictionary dictionary];
	restoreTokens = [NSMutableArray array];
	store = [TGStore new];
	[[q defaultQueue] addTransactionObserver:store];
	return YES;
}

// withStore runs then on the main thread with StoreKit loaded; the first
// time it loads the framework on a background queue (it takes long).
static void withStore(int tok, void (^then)(void)) {
	if (store) {
		then();
		return;
	}
	dispatch_async(dispatch_get_global_queue(QOS_CLASS_USER_INITIATED, 0), ^{
		loadFramework2("StoreKit");
		onMain2(^{
			if (!storeReady()) {
				finish2(tok, TGM_UNSUPPORTED, @"StoreKit not available");
				return;
			}
			then();
		});
	});
}

static NSDictionary *txDict(id<TGSKTransaction> t) {
	NSMutableDictionary *d = [NSMutableDictionary dictionary];
	d[@"id"] = t.transactionIdentifier ?: @"";
	d[@"originalId"] = t.originalTransaction.transactionIdentifier ?: @"";
	d[@"productId"] = t.payment.productIdentifier ?: @"";
	d[@"quantity"] = @(t.payment.quantity);
	d[@"state"] = @(t.transactionState + 1);
	d[@"time"] = @(t.transactionDate ? t.transactionDate.timeIntervalSince1970 : 0);
	if (t.transactionState == TGSKFailed) {
		d[@"error"] = t.error.localizedDescription ?: @"failed";
		d[@"canceled"] = @([t.error.domain isEqualToString:@"SKErrorDomain"] && t.error.code == TGSKErrorPaymentCancelled);
	}
	return d;
}

@implementation TGStore

// SKPaymentTransactionObserver (called on the main thread for the default
// queue observer, but do not rely on it).
- (void)paymentQueue:(id)queue updatedTransactions:(NSArray *)transactions {
	onMain2(^{
		for (id<TGSKTransaction> t in transactions) {
			NSDictionary *d = txDict(t);
			NSString *js = jsonString(d);
			char *c = strdup(js.UTF8String);
			tangoIOSTransaction(c);
			free(c);
			NSInteger st = t.transactionState;
			if ((st == TGSKPurchased || st == TGSKRestored) && t.transactionIdentifier) {
				unfinished[t.transactionIdentifier] = t;
			}
			if (st == TGSKFailed) [[skQueueClass() defaultQueue] finishTransaction:t];
			if (st == TGSKPurchasing || st == TGSKRestored) continue;
			NSNumber *tok = [buyTokens objectForKey:t.payment];
			if (!tok) continue;
			[buyTokens removeObjectForKey:t.payment];
			int code = TGM_OK;
			if (st == TGSKFailed) code = [d[@"canceled"] boolValue] ? TGM_CANCELED : TGM_ERROR;
			finish2(tok.intValue, code, js);
		}
	});
}

- (void)paymentQueueRestoreCompletedTransactionsFinished:(id)queue {
	onMain2(^{
		for (NSNumber *tok in restoreTokens) finish2(tok.intValue, TGM_OK, nil);
		[restoreTokens removeAllObjects];
	});
}

- (void)paymentQueue:(id)queue restoreCompletedTransactionsFailedWithError:(NSError *)error {
	onMain2(^{
		int code = [error.domain isEqualToString:@"SKErrorDomain"] && error.code == TGSKErrorPaymentCancelled ? TGM_CANCELED : TGM_ERROR;
		for (NSNumber *tok in restoreTokens) finish2(tok.intValue, code, error.localizedDescription);
		[restoreTokens removeAllObjects];
	});
}

// SKProductsRequestDelegate

- (void)productsRequest:(id)request didReceiveResponse:(id<TGSKProductsResponse>)response {
	onMain2(^{
		NSNumber *tok = [productRequests objectForKey:request];
		[productRequests removeObjectForKey:request];
		NSMutableArray *ps = [NSMutableArray array];
		for (id<TGSKProduct> p in response.products) {
			knownProducts[p.productIdentifier] = p;
			NSNumberFormatter *f = [NSNumberFormatter new];
			f.numberStyle = NSNumberFormatterCurrencyStyle;
			f.locale = p.priceLocale;
			BOOL sub = [p respondsToSelector:@selector(subscriptionPeriod)] && p.subscriptionPeriod != nil;
			[ps addObject:@{
				@"id": p.productIdentifier ?: @"",
				@"title": p.localizedTitle ?: @"",
				@"description": p.localizedDescription ?: @"",
				@"price": [f stringFromNumber:p.price] ?: @"",
				@"priceValue": @(p.price.doubleValue),
				@"currency": p.priceLocale.currencyCode ?: @"",
				@"kind": @(sub ? 3 : 0), // StoreKit 1 does not tell consumables from the rest
			}];
		}
		if (tok) finish2(tok.intValue, TGM_OK, jsonString(@{@"products": ps, @"invalid": response.invalidProductIdentifiers ?: @[]}));
	});
}

- (void)request:(id)request didFailWithError:(NSError *)error {
	onMain2(^{
		NSNumber *tok = [productRequests objectForKey:request];
		[productRequests removeObjectForKey:request];
		if (tok) finish2(tok.intValue, TGM_ERROR, error.localizedDescription ?: @"product request failed");
	});
}

- (void)requestDidFinish:(id)request {
}

@end

int tgm2_can_pay(void) {
	Class<TGSKPaymentQueueClass> q = skQueueClass();
	if (!q) return 0;
	storeReady();
	return [q canMakePayments] ? 1 : 0;
}

void tgm2_products(int tok, const char *ids) {
	NSString *s = [NSString stringWithUTF8String:ids ?: ""] ?: @"";
	NSMutableSet *set = [NSMutableSet set];
	for (NSString *i in [s componentsSeparatedByString:@"\n"]) {
		if (i.length) [set addObject:i];
	}
	withStore(tok, ^{
		id<TGSKProductsRequest> r = [(id<TGSKProductsRequest>)[skClass(@"SKProductsRequest") alloc] initWithProductIdentifiers:set];
		r.delegate = store;
		[productRequests setObject:@(tok) forKey:r];
		[r start];
	});
}

void tgm2_buy(int tok, const char *product, int quantity) {
	NSString *pid = [NSString stringWithUTF8String:product ?: ""] ?: @"";
	withStore(tok, ^{
		id p = knownProducts[pid];
		if (!p) {
			finish2(tok, TGM_NOT_FOUND, [NSString stringWithFormat:@"product %@ was not returned by Products", pid]);
			return;
		}
		if (![skQueueClass() canMakePayments]) {
			finish2(tok, TGM_NOT_AVAILABLE, @"payments are disabled on this device");
			return;
		}
		id<TGSKMutablePayment> pay = [(Class<TGSKMutablePaymentClass>)skClass(@"SKMutablePayment") paymentWithProduct:p];
		if (quantity > 1) pay.quantity = quantity;
		[buyTokens setObject:@(tok) forKey:pay];
		[[skQueueClass() defaultQueue] addPayment:pay];
	});
}

void tgm2_restore(int tok) {
	withStore(tok, ^{
		[restoreTokens addObject:@(tok)];
		if (restoreTokens.count == 1) [[skQueueClass() defaultQueue] restoreCompletedTransactions];
	});
}

int tgm2_finish_transaction(const char *cid) {
	NSString *tid = [NSString stringWithUTF8String:cid ?: ""] ?: @"";
	id t = store ? unfinished[tid] : nil;
	if (!t) return TGM_NOT_FOUND;
	[unfinished removeObjectForKey:tid];
	[[skQueueClass() defaultQueue] finishTransaction:t];
	return TGM_OK;
}

int tgm2_receipt(void **out, int *n) {
	NSURL *u = NSBundle.mainBundle.appStoreReceiptURL;
	NSData *d = u ? [NSData dataWithContentsOfURL:u] : nil;
	if (!d.length) return TGM_NOT_FOUND;
	*out = malloc(d.length);
	memcpy(*out, d.bytes, d.length);
	*n = (int)d.length;
	return TGM_OK;
}

// ---------------------------------------------------------------------------
// Store review and settings

void tgm2_review(void) {
	UIWindowScene *sc = tg_window().windowScene;
	if (!sc) return;
	// StoreKit loads in the background; the system decides whether the
	// prompt shows (at most three times a year, never in TestFlight).
	dispatch_async(dispatch_get_global_queue(QOS_CLASS_USER_INITIATED, 0), ^{
		loadFramework2("StoreKit");
		onMain2(^{
			Class c = NSClassFromString(@"SKStoreReviewController");
			if (c && sc.activationState != UISceneActivationStateUnattached) {
				[(Class<TGSKReviewClass>)c requestReviewInScene:sc];
			}
		});
	});
}

int tgm2_open_settings(void) {
	int code = tgx_open_url(UIApplicationOpenSettingsURLString.UTF8String); // sysx.m
	return code == TGM_OK ? TGM_OK : TGM_UNSUPPORTED;
}

// ---------------------------------------------------------------------------
// Screen

void tgm2_keep_screen_on(int on) {
	UIApplication.sharedApplication.idleTimerDisabled = on != 0;
}

static UIScreen *screen(void) {
	UIWindowScene *sc = tg_window().windowScene;
	return sc ? sc.screen : nil;
}

double tgm2_brightness(void) {
	UIScreen *s = screen();
	return s ? s.brightness : -1;
}

void tgm2_set_brightness(double v) {
	if (v < 0) return; // iOS restores the user's brightness when the device locks
	screen().brightness = v > 1 ? 1 : v;
}

// ---------------------------------------------------------------------------
// Status bar: TGHost and the pages (mobile.m) report these.

int tgm2_bar_style, tgm2_bar_hidden;

void tgm2_status_bar(int style, int hidden) {
	tgm2_bar_style = style;
	tgm2_bar_hidden = hidden;
	UIWindow *w = tg_window();
	for (UIViewController *vc = w.rootViewController; vc; vc = vc.presentedViewController) {
		[vc setNeedsStatusBarAppearanceUpdate];
		for (UIViewController *c in vc.childViewControllers) [c setNeedsStatusBarAppearanceUpdate];
	}
}

// ---------------------------------------------------------------------------
// Keyboard

static double keyboardHeight;

static void keyboardChanged(double h) {
	h = round(h);
	if (h == keyboardHeight) return;
	keyboardHeight = h;
	tangoIOSKeyboard(h);
}

double tgm2_keyboard_height(void) { return keyboardHeight; }

void tgm2_hide_keyboard(void) {
	[tg_window() endEditing:YES];
}

void tgm2_install(void) {
	bgRegister();
	NSNotificationCenter *nc = NSNotificationCenter.defaultCenter;
	[nc addObserverForName:UIKeyboardWillChangeFrameNotification object:nil queue:nil usingBlock:^(NSNotification *n) {
		UIWindow *w = tg_window();
		if (!w) return;
		CGRect end = [n.userInfo[UIKeyboardFrameEndUserInfoKey] CGRectValue];
		// The frame is in screen coordinates; the overlap with the window
		// is what the keyboard covers.
		CGRect kb = [w convertRect:end fromCoordinateSpace:w.screen.coordinateSpace];
		CGRect hit = CGRectIntersection(w.bounds, kb);
		keyboardChanged(CGRectIsNull(hit) ? 0 : hit.size.height);
	}];
	[nc addObserverForName:UIKeyboardWillHideNotification object:nil queue:nil usingBlock:^(NSNotification *n) {
		keyboardChanged(0);
	}];
}
