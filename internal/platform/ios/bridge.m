//go:build ios

// UIKit half of Tango's iOS backend. Compiled by cgo as Objective-C with ARC
// into the app's Go c-archive. It only translates: every decision that is
// not purely about UIKit is made in Go (backend.go and friends).
//
// Pages: every Tango surface is a TGPage (a UIViewController whose view is a
// full-screen WKWebView). The shown pages form a stack: the first is the
// window's rootViewController, each later one is presented full screen over
// the previous. syncStack() reconciles UIKit's presentation chain with that
// stack one transition at a time, so callers never present while UIKit is
// still animating.

#import <UIKit/UIKit.h>
#import <WebKit/WebKit.h>
#import <UniformTypeIdentifiers/UniformTypeIdentifiers.h>
#include <stdlib.h>
#include <string.h>

#include "bridge.h"
#include "bridge_private.h"
#include "startup.h"
#include "_cgo_export.h"

static NSString *str(const char *s) {
	return s ? [NSString stringWithUTF8String:s] ?: @"" : @"";
}

static char *cstr(NSString *s) {
	return strdup(s ? s.UTF8String : "");
}

// ---------------------------------------------------------------------------
// Wake source: a version-0 run loop source in the common modes, so Drain
// also runs while UIKit tracks a scroll (UITrackingRunLoopMode) and inside
// nested run loops started by tg_step — which a block queued on the main
// dispatch queue would not, as long as an earlier main-queue block (or this
// drain itself) is still running.

static CFRunLoopSourceRef wakeSource;

static void wakePerform(void *info) {
	@autoreleasepool {
		tangoIOSDrain();
	}
}

void tg_wake(void) {
	CFRunLoopSourceSignal(wakeSource);
	CFRunLoopWakeUp(CFRunLoopGetMain());
}

int tg_step(int block) {
	@autoreleasepool {
		CFRunLoopRunInMode(kCFRunLoopDefaultMode, block ? 1.0e10 : 0, true);
	}
	return 1;
}

int tg_app_active(void) {
	return UIApplication.sharedApplication.applicationState == UIApplicationStateActive;
}

void tg_exit_on_main(int code) {
	dispatch_async(dispatch_get_main_queue(), ^{
		exit(code);
	});
}

// ---------------------------------------------------------------------------
// Pages

// TGPage is declared in bridge_private.h.

static UIWindow *window;
static NSMutableDictionary<NSNumber *, TGPage *> *pages;
static NSMutableArray<TGPage *> *stack;
static BOOL transitioning, resync;

static void syncStack(void);

static TGPage *page(uint64_t sid) {
	return pages[@(sid)];
}

// host is the window's rootViewController for the app's lifetime. The
// root page (stack[0]) is embedded in it as a child; later pages are
// presented over it. Never swapping rootViewController avoids UIKit's
// unbalanced appearance transitions when the first page arrives right after
// the window appeared.
static UIViewController *host;

static void embedRoot(UIViewController *p) {
	UIViewController *old = host.childViewControllers.firstObject;
	if (old == p) return;
	if (old) {
		[old willMoveToParentViewController:nil];
		[old.view removeFromSuperview];
		[old removeFromParentViewController];
	}
	if (!p) return;
	[host addChildViewController:p];
	p.view.frame = host.view.bounds;
	p.view.autoresizingMask = UIViewAutoresizingFlexibleWidth | UIViewAutoresizingFlexibleHeight;
	[host.view addSubview:p.view];
	[p didMoveToParentViewController:host];
}

// attachWindow puts host into w and shows it. Called once a window exists:
// from the scene delegate, or from tg_install without scenes.
static void attachWindow(UIWindow *w) {
	window = w;
	window.rootViewController = host;
	[window makeKeyAndVisible];
	syncStack();
}

// topController is where dialogs are presented from.
static UIViewController *topController(void) {
	UIViewController *vc = window.rootViewController;
	while (vc.presentedViewController && !vc.presentedViewController.isBeingDismissed)
		vc = vc.presentedViewController;
	return vc;
}

static void transitionDone(void) {
	transitioning = NO;
	syncStack();
}

static void syncStack(void) {
	if (!window) return;
	if (transitioning) {
		resync = YES;
		return;
	}
	resync = NO;
	UIViewController *root = stack.firstObject;
	if (host.childViewControllers.firstObject != root) {
		// The new root may be a page that was presented: take every page
		// down first, the loop below presents the rest again.
		[host dismissViewControllerAnimated:NO completion:nil];
		embedRoot(root);
	}
	UIViewController *parent = host;
	for (NSUInteger i = 1; i < stack.count; i++) {
		TGPage *want = stack[i];
		UIViewController *have = parent.presentedViewController;
		if (have == want && !want.isBeingDismissed) {
			parent = want;
			continue;
		}
		if (!parent.view.window) return; // not on screen yet; retried on the next change
		if (have && ![have isKindOfClass:TGPage.class]) return; // a dialog is up: wait for it
		transitioning = YES;
		if (have) {
			[parent dismissViewControllerAnimated:YES completion:^{ transitionDone(); }];
		} else {
			want.modalPresentationStyle = UIModalPresentationFullScreen;
			[parent presentViewController:want animated:YES completion:^{ transitionDone(); }];
		}
		return;
	}
	UIViewController *extra = parent.presentedViewController;
	if ([extra isKindOfClass:TGPage.class] && !extra.isBeingDismissed) {
		transitioning = YES;
		[parent dismissViewControllerAnimated:YES completion:^{ transitionDone(); }];
	} else if (resync) {
		syncStack();
	}
}

// Accessors for mobile.m.

UIViewController *tg_top_controller(void) { return window ? topController() : nil; }
UIWindow *tg_window(void) { return window; }
void tg_presented_ended(void) { syncStack(); }
TGPage *tg_page(uint64_t sid) { return page(sid); }
void tg_page_register(TGPage *p) { pages[@(p.sid)] = p; }

@implementation TGPage

- (void)loadView {
	UIView *v = [UIView new];
	v.backgroundColor = UIColor.systemBackgroundColor;
	self.web.frame = v.bounds;
	self.web.autoresizingMask = UIViewAutoresizingFlexibleWidth | UIViewAutoresizingFlexibleHeight;
	[v addSubview:self.web];
	self.view = v;
}

- (void)viewDidLayoutSubviews {
	[super viewDidLayoutSubviews];
	CGSize s = self.view.bounds.size;
	if (!CGSizeEqualToSize(s, self.reported) && pages[@(self.sid)]) {
		self.reported = s;
		tangoIOSResized(self.sid, (int)lround(s.width), (int)lround(s.height));
	}
	if (self.web && pages[@(self.sid)]) tangoIOSPageLayout(self.sid, s.width, s.height); // canvas_attach.go
}

// WKNavigationDelegate

- (void)webView:(WKWebView *)webView didFinishNavigation:(WKNavigation *)navigation {
	tg_trace("didFinishNavigation (first page loaded)");
	char *u = cstr(webView.URL.absoluteString);
	tangoIOSLoaded(self.sid, u);
	free(u);
}

// WKScriptMessageHandler: only the main frame (where the shim runs) may talk
// to Go; the core further checks the origin.

- (void)userContentController:(WKUserContentController *)ucc didReceiveScriptMessage:(WKScriptMessage *)msg {
	if (!msg.frameInfo.isMainFrame || ![msg.body isKindOfClass:NSString.class]) return;
	WKSecurityOrigin *so = msg.frameInfo.securityOrigin;
	NSString *origin = [NSString stringWithFormat:@"%@://%@", so.protocol, so.host];
	if (so.port != 0) origin = [origin stringByAppendingFormat:@":%ld", (long)so.port];
	// Pass the length: messages may contain NUL (the core's internal
	// messages start with "\x00tango-int:").
	char *o = cstr(origin);
	NSData *m = [(NSString *)msg.body dataUsingEncoding:NSUTF8StringEncoding];
	tangoIOSMessage(self.sid, o, (char *)m.bytes, (int)m.length);
	free(o);
}

// WKUIDelegate: WKWebView shows nothing for alert()/confirm() unless the app
// does it.

- (void)webView:(WKWebView *)webView runJavaScriptAlertPanelWithMessage:(NSString *)message
	initiatedByFrame:(WKFrameInfo *)frame completionHandler:(void (^)(void))done {
	UIAlertController *a = [UIAlertController alertControllerWithTitle:nil message:message
		preferredStyle:UIAlertControllerStyleAlert];
	[a addAction:[UIAlertAction actionWithTitle:@"OK" style:UIAlertActionStyleDefault handler:^(UIAlertAction *x) { done(); }]];
	[topController() presentViewController:a animated:YES completion:nil];
}

- (void)webView:(WKWebView *)webView runJavaScriptConfirmPanelWithMessage:(NSString *)message
	initiatedByFrame:(WKFrameInfo *)frame completionHandler:(void (^)(BOOL))done {
	UIAlertController *a = [UIAlertController alertControllerWithTitle:nil message:message
		preferredStyle:UIAlertControllerStyleAlert];
	[a addAction:[UIAlertAction actionWithTitle:@"Cancel" style:UIAlertActionStyleCancel handler:^(UIAlertAction *x) { done(NO); }]];
	[a addAction:[UIAlertAction actionWithTitle:@"OK" style:UIAlertActionStyleDefault handler:^(UIAlertAction *x) { done(YES); }]];
	[topController() presentViewController:a animated:YES completion:nil];
}

@end

// ---------------------------------------------------------------------------
// tango:// scheme handler. WebKit calls it on the main thread. Go
// (scheme.go) runs the request's handler on a goroutine; the response is
// queued in Go and handed to the task on the main thread (tg_task_kick ->
// tangoIOSTaskDrain -> tg_task_respond / _data / _finish). tasks is touched
// on the main thread only, so a task WebKit stopped (or that finished) is
// never called again.

@interface TGSchemeHandler : NSObject <WKURLSchemeHandler>
@end

static TGSchemeHandler *schemeHandler;
static NSMutableDictionary<NSNumber *, id<WKURLSchemeTask>> *tasks;
static NSMapTable<id<WKURLSchemeTask>, NSNumber *> *taskIDs;
static uint64_t lastTask;

static NSData *requestBody(NSURLRequest *r) {
	if (r.HTTPBody) return r.HTTPBody;
	NSInputStream *in = r.HTTPBodyStream;
	if (!in) return nil;
	NSMutableData *d = [NSMutableData data];
	uint8_t buf[65536];
	[in open];
	for (;;) {
		NSInteger n = [in read:buf maxLength:sizeof buf];
		if (n <= 0) break;
		[d appendBytes:buf length:(NSUInteger)n];
	}
	[in close];
	return d;
}

// Streaming probe (test hook). With TANGO_SCHEME_PROBE=1 in the
// environment (simctl: SIMCTL_CHILD_TANGO_SCHEME_PROBE=1), GET
// /__tango/probe/stream is answered here, without Host.ServeAsset: the
// response head and a first 1 KiB chunk at once, then one more 1 KiB chunk
// every 200 ms (8 in all), then didFinish. A page that records when each
// chunk reaches its fetch() ReadableStream shows whether WebKit delivers
// the didReceiveData: calls incrementally (examples/duplex-ios,
// scripts/smoke.sh ios-probe). The chunks come from an NSTimer on the main
// run loop in the common modes, so they are sent on the main thread and
// also while UIKit tracks a touch or Tango runs a nested run loop. Without
// the variable nothing here runs.

static BOOL schemeProbe(void) {
#ifndef TANGO_TEST_HOOKS
	return NO; // compiled in only with the self-test hooks (testhooks_on.go)
#else
	static BOOL on;
	static dispatch_once_t once;
	dispatch_once(&once, ^{
		const char *v = getenv("TANGO_SCHEME_PROBE");
		on = v && strcmp(v, "1") == 0;
	});
	return on;
#endif
}

// serveProbe streams the probe response to task h (already in tasks). Once
// WebKit stopped the task (stopURLSchemeTask: removed it from tasks)
// nothing more is sent.
static void serveProbe(uint64_t h) {
	enum { probeChunks = 8, probeChunk = 1024 };
	static uint8_t chunk[probeChunk];
	for (int i = 0; i < probeChunk; i++) chunk[i] = (uint8_t)i;
	tg_task_respond(h, 200, "Content-Type: application/octet-stream\nCache-Control: no-store\nX-Content-Type-Options: nosniff\n");
	tg_task_data(h, chunk, probeChunk);
	__block int sent = 1;
	NSTimer *timer = [NSTimer timerWithTimeInterval:0.2 repeats:YES block:^(NSTimer *t) {
		if (!tasks[@(h)]) { // stopped by WebKit
			[t invalidate];
			return;
		}
		tg_task_data(h, chunk, probeChunk);
		if (++sent == probeChunks) {
			[t invalidate];
			tg_task_finish(h);
		}
	}];
	[NSRunLoop.mainRunLoop addTimer:timer forMode:NSRunLoopCommonModes];
}

@implementation TGSchemeHandler

- (void)webView:(WKWebView *)webView startURLSchemeTask:(id<WKURLSchemeTask>)task {
	tg_trace("first tango:// request");
	uint64_t h = ++lastTask;
	tasks[@(h)] = task;
	[taskIDs setObject:@(h) forKey:task];
	if (schemeProbe() && [task.request.URL.path isEqualToString:@"/__tango/probe/stream"]) {
		serveProbe(h);
		return;
	}

	uint64_t sid = 0;
	for (TGPage *p in pages.allValues) {
		if (p.web == webView) sid = p.sid;
	}
	NSURLRequest *r = task.request;
	NSMutableString *hdr = [NSMutableString string];
	__block BOOL hasOrigin = NO;
	[r.allHTTPHeaderFields enumerateKeysAndObjectsUsingBlock:^(NSString *k, NSString *v, BOOL *stop) {
		[hdr appendFormat:@"%@: %@\n", k, v];
		if ([k caseInsensitiveCompare:@"Origin"] == NSOrderedSame) hasOrigin = YES;
	}];
	// WebKit sends no Origin on custom-scheme requests; the core's CSRF
	// check (package tango, asset_origin.go) uses the main document's.
	NSURL *doc = r.mainDocumentURL;
	if (!hasOrigin && doc.scheme.length && doc.host.length) {
		NSString *o = [NSString stringWithFormat:@"%@://%@", doc.scheme.lowercaseString, doc.host.lowercaseString];
		if (doc.port) o = [o stringByAppendingFormat:@":%@", doc.port];
		[hdr appendFormat:@"Origin: %@\n", o];
	}
	NSData *body = requestBody(r);
	char *method = cstr(r.HTTPMethod ?: @"GET"), *url = cstr(r.URL.absoluteString), *headers = cstr(hdr);
	tangoIOSTaskStart(sid, h, method, url, headers, (void *)body.bytes, (int)body.length);
	free(method);
	free(url);
	free(headers);
}

- (void)webView:(WKWebView *)webView stopURLSchemeTask:(id<WKURLSchemeTask>)task {
	NSNumber *h = [taskIDs objectForKey:task];
	if (!h) return;
	[tasks removeObjectForKey:h];
	[taskIDs removeObjectForKey:task];
	tangoIOSTaskStop(h.unsignedLongLongValue);
}

@end

static void taskRespond(id<WKURLSchemeTask> task, int status, NSString *headers) {
	NSMutableDictionary *fields = [NSMutableDictionary dictionary];
	for (NSString *line in [headers componentsSeparatedByString:@"\n"]) {
		NSRange c = [line rangeOfString:@": "];
		if (c.location == NSNotFound) continue;
		fields[[line substringToIndex:c.location]] = [line substringFromIndex:c.location + 2];
	}
	NSHTTPURLResponse *resp = [[NSHTTPURLResponse alloc] initWithURL:task.request.URL statusCode:status
		HTTPVersion:@"HTTP/1.1" headerFields:fields];
	[task didReceiveResponse:resp];
}

static void taskFinish(uint64_t h, id<WKURLSchemeTask> task) {
	[task didFinish];
	[tasks removeObjectForKey:@(h)];
	[taskIDs removeObjectForKey:task];
}

void tg_task_respond(uint64_t h, int status, const char *headers) {
	id<WKURLSchemeTask> task = tasks[@(h)];
	if (task) taskRespond(task, status, str(headers));
}

void tg_task_data(uint64_t h, const void *p, size_t n) {
	id<WKURLSchemeTask> task = tasks[@(h)];
	if (task && n > 0) [task didReceiveData:[[NSData alloc] initWithBytes:p length:n]];
}

void tg_task_finish(uint64_t h) {
	id<WKURLSchemeTask> task = tasks[@(h)];
	if (task) taskFinish(h, task);
}

// tg_task_kick: see bridge.h. The block goes on the main run loop in the
// common modes, like mobile.m's onMain, so it also runs while UIKit tracks
// a touch and inside nested run loops (tg_step: AwaitAsync on the main
// thread) — blocks on the main dispatch queue would stall there, and a
// wait that needs a tango:// response would never end. The block only says
// "drain this task": the response's order is kept by the Go writer's own
// queue (scheme_writer.go), not by the order in which the run loop runs
// blocks, which is not strict (CFRunLoop detaches its block list for a
// pass, so a nested run loop started by one block runs later blocks before
// the rest of that pass). Called on Go threads, which have no autorelease
// pool of their own; the pool keeps anything autoreleased here from living
// as long as the thread. No bytes cross this call: the drain hands them
// to tg_task_data on the main thread, which copies them.
void tg_task_kick(uint64_t h) {
	@autoreleasepool {
		CFRunLoopPerformBlock(CFRunLoopGetMain(), kCFRunLoopCommonModes, ^{
			@autoreleasepool {
				tangoIOSTaskDrain(h);
			}
		});
		CFRunLoopWakeUp(CFRunLoopGetMain());
	}
}

// ---------------------------------------------------------------------------
// Surfaces

static void addScript(WKUserContentController *ucc, const char *src) {
	if (!src || !*src) return;
	WKUserScript *s = [[WKUserScript alloc] initWithSource:str(src)
		injectionTime:WKUserScriptInjectionTimeAtDocumentStart forMainFrameOnly:YES];
	[ucc addUserScript:s];
}

// pageWebView creates a page's web view; its user content controller is
// empty. Every page-wide WKWebViewConfiguration setting belongs here, since
// the first page gets the prewarmed web view (tg_prewarm_webkit).
static WKWebView *pageWebView(void) {
	WKWebViewConfiguration *cfg = [WKWebViewConfiguration new];
	[cfg setURLSchemeHandler:schemeHandler forURLScheme:@"tango"];
	cfg.allowsInlineMediaPlayback = YES;
	cfg.userContentController = [WKUserContentController new];
	// The web view fills the page, under the status bar and home indicator.
	// WKWebView insets the content by the safe area by itself unless the page
	// opts out with <meta name="viewport" content="viewport-fit=cover"> and
	// uses env(safe-area-inset-*).
	return [[WKWebView alloc] initWithFrame:UIScreen.mainScreen.bounds configuration:cfg];
}

// Prewarming. On a cold start most of the time to the first page went
// into creating the first WKWebView (WebKit's own setup) and launching its
// web content process before WebKit asked for the first tango:// URL.
// TangoStart therefore makes the first page's web view right away and loads
// an empty document into it, which launches the process while the Go
// program starts; the first tg_surface_new takes it over (the user content
// controller's scripts apply from the next document on).
static WKWebView *spare;

void tg_prewarm_webkit(void) {
	const char *e = getenv("TANGO_NO_PREWARM"); // for measurements
	if (spare || pages.count || (e && *e && strcmp(e, "0") != 0)) return;
	tg_trace("prewarm begins");
	spare = pageWebView();
	[spare loadHTMLString:@"" baseURL:nil];
	tg_trace("prewarm web view created");
	// An app that shows no page soon (a background launch) gives it back.
	dispatch_after(dispatch_time(DISPATCH_TIME_NOW, 30 * NSEC_PER_SEC), dispatch_get_main_queue(), ^{
		[spare stopLoading];
		spare = nil;
	});
}

void tg_surface_new(uint64_t sid, const char *shim, const char *bridge, int debug, const char *title) {
	tg_trace("tg_surface_new begins");
	TGPage *p = [TGPage new];
	p.sid = sid;
	p.title = str(title);

	if (spare) {
		p.web = spare;
		spare = nil;
	} else {
		p.web = pageWebView();
	}
	WKUserContentController *ucc = p.web.configuration.userContentController;
	[ucc addScriptMessageHandler:p name:@"tango"];
	addScript(ucc, shim);
	addScript(ucc, bridge);
	p.web.navigationDelegate = p;
	p.web.UIDelegate = p;
	p.web.opaque = NO;
	p.web.backgroundColor = UIColor.clearColor;
	if (@available(iOS 16.4, *)) p.web.inspectable = debug != 0;
	pages[@(sid)] = p;
	tg_trace("WKWebView created");
}

void tg_surface_show(uint64_t sid) {
	TGPage *p = page(sid);
	if (!p || [stack containsObject:p]) return;
	[stack addObject:p];
	syncStack();
}

void tg_surface_hide(uint64_t sid) {
	TGPage *p = page(sid);
	if (!p) return;
	[stack removeObject:p];
	syncStack();
}

void tg_surface_close(uint64_t sid) {
	TGPage *p = page(sid);
	if (!p) return;
	[pages removeObjectForKey:@(sid)];
	[stack removeObject:p];
	// Break WKUserContentController -> handler (p) -> web view -> ucc.
	[p.web.configuration.userContentController removeScriptMessageHandlerForName:@"tango"];
	[p.web stopLoading];
	p.web.navigationDelegate = nil;
	p.web.UIDelegate = nil;
	syncStack();
}

void tg_surface_set_title(uint64_t sid, const char *title) {
	page(sid).title = str(title);
}

void tg_surface_size(uint64_t sid, int *w, int *h) {
	TGPage *p = page(sid);
	CGSize s = p.isViewLoaded && p.view.window ? p.view.bounds.size
		: window ? window.bounds.size : UIScreen.mainScreen.bounds.size;
	*w = (int)lround(s.width);
	*h = (int)lround(s.height);
}

void tg_surface_navigate(uint64_t sid, const char *url) {
	NSURL *u = [NSURL URLWithString:str(url)] ?: [NSURL URLWithString:@"about:blank"];
	[page(sid).web loadRequest:[NSURLRequest requestWithURL:u]];
}

void tg_surface_eval(uint64_t sid, const char *js) {
	[page(sid).web evaluateJavaScript:str(js) completionHandler:nil];
}

uintptr_t tg_surface_native(uint64_t sid) {
	return (uintptr_t)(__bridge void *)page(sid);
}

// ---------------------------------------------------------------------------
// Clipboard

char *tg_clipboard_read(void) {
	NSString *s = UIPasteboard.generalPasteboard.string;
	return s ? cstr(s) : NULL;
}

void tg_clipboard_write(const char *text) {
	UIPasteboard.generalPasteboard.string = str(text);
}

// ---------------------------------------------------------------------------
// Dialogs

static NSMutableDictionary<NSNumber *, UIViewController *> *dialogs;

@interface TGPickerDelegate : NSObject <UIDocumentPickerDelegate>
@property(nonatomic) int token;
@end

static void dialogEnded(int token) {
	[dialogs removeObjectForKey:@(token)];
	syncStack(); // a page transition may have waited for the dialog
}

@implementation TGPickerDelegate

- (void)documentPicker:(UIDocumentPickerViewController *)picker didPickDocumentsAtURLs:(NSArray<NSURL *> *)urls {
	NSMutableData *buf = [NSMutableData data];
	for (NSURL *u in urls) {
		const char *p = u.fileSystemRepresentation;
		[buf appendBytes:p length:strlen(p) + 1]; // NUL-separated
	}
	dialogEnded(self.token);
	tangoIOSPickerDone(self.token, (char *)buf.bytes, (int)buf.length, 0);
}

- (void)documentPickerWasCancelled:(UIDocumentPickerViewController *)picker {
	dialogEnded(self.token);
	tangoIOSPickerDone(self.token, NULL, 0, 1);
}

@end

static NSMutableDictionary<NSNumber *, TGPickerDelegate *> *pickerDelegates;

int tg_alert(int token, const char *title, const char *message, const char *buttons) {
	UIViewController *from = topController();
	if (!from) return -1;
	NSString *t = str(title), *m = str(message);
	UIAlertController *a = [UIAlertController alertControllerWithTitle:t.length ? t : nil
		message:m.length ? m : nil preferredStyle:UIAlertControllerStyleAlert];
	NSArray<NSString *> *labels = [str(buttons) componentsSeparatedByString:@"\n"];
	[labels enumerateObjectsUsingBlock:^(NSString *label, NSUInteger i, BOOL *stop) {
		[a addAction:[UIAlertAction actionWithTitle:label style:UIAlertActionStyleDefault handler:^(UIAlertAction *x) {
			dialogEnded(token);
			tangoIOSAlertDone(token, (int)i);
		}]];
	}];
	dialogs[@(token)] = a;
	[from presentViewController:a animated:YES completion:nil];
	return 0;
}

int tg_pick(int token, const char *exts, int multiple, int folders, const char *dir) {
	UIViewController *from = topController();
	if (!from) return -1;
	NSMutableArray<UTType *> *types = [NSMutableArray array];
	if (folders) {
		[types addObject:UTTypeFolder];
	} else {
		for (NSString *e in [str(exts) componentsSeparatedByString:@"\n"]) {
			if (!e.length) continue;
			UTType *t = [UTType typeWithFilenameExtension:e];
			if (t) [types addObject:t];
		}
		if (!types.count) [types addObject:UTTypeItem];
	}
	// Files are imported as copies into the app's sandbox (tmp/), so the
	// returned paths stay readable without security-scoped access. Folders
	// cannot be copied; their URLs need startAccessingSecurityScopedResource.
	UIDocumentPickerViewController *pk = [[UIDocumentPickerViewController alloc]
		initForOpeningContentTypes:types asCopy:!folders];
	pk.allowsMultipleSelection = multiple != 0;
	NSString *d = str(dir);
	if (d.length) pk.directoryURL = [NSURL fileURLWithPath:d isDirectory:YES];
	TGPickerDelegate *del = [TGPickerDelegate new];
	del.token = token;
	pickerDelegates[@(token)] = del; // the picker holds its delegate weakly
	pk.delegate = del;
	dialogs[@(token)] = pk;
	[from presentViewController:pk animated:YES completion:nil];
	return 0;
}

// tg_dismiss_dialogs closes every dialog without reporting (the Go side has
// already answered them).
void tg_dismiss_dialogs(void) {
	for (UIViewController *vc in dialogs.allValues) {
		[vc.presentingViewController dismissViewControllerAnimated:NO completion:nil];
	}
	[dialogs removeAllObjects];
	[pickerDelegates removeAllObjects];
}

// ---------------------------------------------------------------------------
// Scene delegate. Hosts with a UIApplicationSceneManifest name this class as
// the scene delegate (see mobile/ios/Info.plist); the window is created when
// UIKit connects the scene. Without a manifest tg_install creates it.

// TangoSceneDelegate is declared in bridge_private.h; mobile.m adds the
// URL and user-activity (universal link) methods in a category.

@implementation TangoSceneDelegate

- (void)scene:(UIScene *)scene willConnectToSession:(UISceneSession *)session options:(UISceneConnectionOptions *)options {
	if (![scene isKindOfClass:UIWindowScene.class] || window) return;
	tg_trace("scene connected");
	self.window = [[UIWindow alloc] initWithWindowScene:(UIWindowScene *)scene];
	attachWindow(self.window);
	tg_scene_connected(options);
}

@end

// ---------------------------------------------------------------------------
// Install

void tg_install(void) {
	pages = [NSMutableDictionary dictionary];
	stack = [NSMutableArray array];
	tasks = [NSMutableDictionary dictionary];
	taskIDs = [NSMapTable weakToStrongObjectsMapTable];
	dialogs = [NSMutableDictionary dictionary];
	pickerDelegates = [NSMutableDictionary dictionary];
	schemeHandler = [TGSchemeHandler new];
	host = tg_new_host();
	host.view.backgroundColor = UIColor.systemBackgroundColor;

	CFRunLoopSourceContext ctx = {0};
	ctx.perform = wakePerform;
	wakeSource = CFRunLoopSourceCreate(NULL, 0, &ctx);
	CFRunLoopAddSource(CFRunLoopGetMain(), wakeSource, kCFRunLoopCommonModes);

	NSNotificationCenter *nc = NSNotificationCenter.defaultCenter;
	void (^on)(NSNotificationName, int) = ^(NSNotificationName name, int code) {
		[nc addObserverForName:name object:nil queue:nil usingBlock:^(NSNotification *n) {
			tangoIOSLifecycle(code);
		}];
	};
	on(UIApplicationDidBecomeActiveNotification, TG_DID_BECOME_ACTIVE);
	on(UIApplicationWillResignActiveNotification, TG_WILL_RESIGN_ACTIVE);
	on(UIApplicationDidEnterBackgroundNotification, TG_DID_ENTER_BACKGROUND);
	on(UIApplicationWillEnterForegroundNotification, TG_WILL_ENTER_FOREGROUND);
	[nc addObserverForName:UIApplicationWillTerminateNotification object:nil queue:nil usingBlock:^(NSNotification *n) {
		tangoIOSTerminate();
	}];

	tg_mobile_install();

	if (![NSBundle.mainBundle objectForInfoDictionaryKey:@"UIApplicationSceneManifest"]) {
		attachWindow([[UIWindow alloc] initWithFrame:UIScreen.mainScreen.bounds]);
	}
}
