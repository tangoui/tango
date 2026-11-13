//go:build ios

// Advanced WebView features of the iOS backend: page permission requests,
// navigation policy, new-window requests, HTTP authentication, downloads,
// cookies and website data, user agent, find in page, PDF and snapshots
// (internal/platform/webview_ext.go). Go half: webview_ext.go.
//
// The WKNavigationDelegate / WKUIDelegate methods are added to TGPage in a
// category; bridge.m and mobile.m implement the others. Every decision is
// made in Go: handlers WebKit gives us are kept by token until Go answers
// through tg_web_*_done, which may happen before the Go export returns or
// later (always on the main thread).

#import <UIKit/UIKit.h>
#import <WebKit/WebKit.h>
#import <objc/runtime.h>
#include <stdlib.h>
#include <string.h>

#include "bridge_private.h"
#include "webview_ext.h"
#include "_cgo_export.h"

static NSString *wstr(const char *s) {
	return s ? [NSString stringWithUTF8String:s] ?: @"" : @"";
}

static char *wcstr(NSString *s) {
	return strdup(s ? s.UTF8String ?: "" : "");
}

static NSString *originOf(WKSecurityOrigin *so) {
	if (!so || !so.protocol.length) return @"";
	NSString *o = [NSString stringWithFormat:@"%@://%@", so.protocol, so.host];
	if (so.port != 0) o = [o stringByAppendingFormat:@":%ld", (long)so.port];
	return o;
}

// Pending handlers by token. Main thread only.
static NSMutableDictionary<NSNumber *, id> *pending;
static uint64_t lastToken;

static uint64_t keep(id block) {
	if (!pending) pending = [NSMutableDictionary dictionary];
	uint64_t t = ++lastToken;
	pending[@(t)] = block;
	return t;
}

static id take(uint64_t tok) {
	id b = pending[@(tok)];
	if (b) [pending removeObjectForKey:@(tok)];
	return b;
}

// Result helpers.
static void doneErr(uint64_t tok, int code, NSString *msg) {
	char *m = wcstr(msg);
	tangoIOSWebDone(tok, code, m);
	free(m);
}

static void doneOK(uint64_t tok) { doneErr(tok, TGW_OK, nil); }

static void doneBytes(uint64_t tok, NSData *d, NSError *err, NSString *fallback) {
	if (!d) {
		char *m = wcstr(err.localizedDescription ?: fallback);
		tangoIOSWebBytes(tok, TGW_FAILED, m, NULL, 0);
		free(m);
		return;
	}
	tangoIOSWebBytes(tok, TGW_OK, (char *)"", (void *)d.bytes, (int)d.length);
}

static WKWebView *webOf(uint64_t sid) {
	return tg_page(sid).web;
}

static NSString *const noPage = @"tango/ios: surface is closed";

// ---------------------------------------------------------------------------
// Downloads (iOS 14.5+). One record per WKDownload, by Tango download id.

@interface TGDownload : NSObject
@property(nonatomic) uint64_t did, sid;
@property(nonatomic, strong) WKDownload *download API_AVAILABLE(ios(14.5));
@property(nonatomic, copy) void (^decide)(NSURL *);
@property(nonatomic, copy) NSString *path;
@property(nonatomic) BOOL accepted, canceled;
@property(nonatomic) int64_t lastReported;
@property(nonatomic, strong) NSTimer *timer;
@end

@implementation TGDownload
@end

API_AVAILABLE(ios(14.5))
@interface TGDownloads : NSObject <WKDownloadDelegate>
@end

static NSMutableDictionary<NSNumber *, TGDownload *> *downloads;
static uint64_t lastDownload;
static TGDownloads *downloadDelegate API_AVAILABLE(ios(14.5));

static TGDownload *recordFor(id dl) {
	for (TGDownload *r in downloads.allValues) {
		if (@available(iOS 14.5, *)) {
			if (r.download == dl) return r;
		}
	}
	return nil;
}

static void reportProgress(TGDownload *r, BOOL force) API_AVAILABLE(ios(14.5)) {
	NSProgress *p = r.download.progress;
	int64_t got = p.completedUnitCount;
	if (!force && got == r.lastReported) return;
	r.lastReported = got;
	char *path = wcstr(r.path);
	tangoIOSDownloadEvent(r.sid, r.did, TGW_DL_PROGRESS, got, p.totalUnitCount > 0 ? p.totalUnitCount : -1, path, (char *)"");
	free(path);
}

// endDownload sends the one final event of an accepted download and forgets
// it. Calls after the first are ignored.
static void endDownload(TGDownload *r, int state, NSString *err) API_AVAILABLE(ios(14.5)) {
	if (!downloads[@(r.did)]) return;
	[downloads removeObjectForKey:@(r.did)];
	[r.timer invalidate];
	r.timer = nil;
	if (!r.accepted) return;
	NSProgress *p = r.download.progress;
	int64_t got = p.completedUnitCount;
	if (state == TGW_DL_DONE && got <= 0) {
		NSDictionary *a = [NSFileManager.defaultManager attributesOfItemAtPath:r.path error:nil];
		if (a) got = (int64_t)a.fileSize;
	}
	int64_t total = p.totalUnitCount > 0 ? p.totalUnitCount : (state == TGW_DL_DONE ? got : -1);
	char *path = wcstr(r.path), *e = wcstr(err);
	tangoIOSDownloadEvent(r.sid, r.did, state, got, total, path, e);
	free(path);
	free(e);
}

static void adoptDownload(TGPage *page, WKDownload *dl) API_AVAILABLE(ios(14.5)) {
	if (!downloads) downloads = [NSMutableDictionary dictionary];
	if (!downloadDelegate) downloadDelegate = [TGDownloads new];
	TGDownload *r = [TGDownload new];
	r.did = ++lastDownload;
	r.sid = page.sid;
	r.download = dl;
	r.lastReported = -1;
	downloads[@(r.did)] = r;
	dl.delegate = downloadDelegate; // weak; downloadDelegate lives forever
}

@implementation TGDownloads

- (void)download:(WKDownload *)download decideDestinationUsingResponse:(NSURLResponse *)response
	suggestedFilename:(NSString *)suggestedFilename completionHandler:(void (^)(NSURL *))completionHandler {
	TGDownload *r = recordFor(download);
	if (!r) {
		completionHandler(nil);
		return;
	}
	r.decide = completionHandler;
	NSURL *u = response.URL ?: download.originalRequest.URL;
	char *url = wcstr(u.absoluteString), *name = wcstr(suggestedFilename), *mime = wcstr(response.MIMEType);
	int64_t size = response.expectedContentLength >= 0 ? response.expectedContentLength : -1;
	tangoIOSDownloadStart(r.sid, r.did, url, name, mime, size);
	free(url);
	free(name);
	free(mime);
}

- (void)downloadDidFinish:(WKDownload *)download {
	TGDownload *r = recordFor(download);
	if (r) endDownload(r, TGW_DL_DONE, nil);
}

- (void)download:(WKDownload *)download didFailWithError:(NSError *)error resumeData:(NSData *)resumeData {
	TGDownload *r = recordFor(download);
	if (!r) return;
	BOOL canceled = r.canceled || ([error.domain isEqualToString:NSURLErrorDomain] && error.code == NSURLErrorCancelled);
	endDownload(r, canceled ? TGW_DL_CANCELED : TGW_DL_FAILED, canceled ? nil : error.localizedDescription);
}

@end

void tg_web_download_dest(uint64_t did, const char *cpath) {
	if (@available(iOS 14.5, *)) {
		TGDownload *r = downloads[@(did)];
		void (^decide)(NSURL *) = r.decide;
		if (!r || !decide) return;
		r.decide = nil;
		NSString *path = wstr(cpath);
		if (!path.length) {
			// Not accepted: no events. WebKit cancels the download.
			[downloads removeObjectForKey:@(did)];
			decide(nil);
			return;
		}
		// WKDownload fails when the destination exists; the contract says
		// existing files are replaced.
		[NSFileManager.defaultManager removeItemAtPath:path error:nil];
		r.path = path;
		r.accepted = YES;
		__weak TGDownload *weak = r;
		r.timer = [NSTimer timerWithTimeInterval:0.5 repeats:YES block:^(NSTimer *t) {
			TGDownload *s = weak;
			if (s && downloads[@(s.did)]) reportProgress(s, NO);
			else [t invalidate];
		}];
		[NSRunLoop.mainRunLoop addTimer:r.timer forMode:NSRunLoopCommonModes];
		decide([NSURL fileURLWithPath:path isDirectory:NO]);
	}
}

void tg_web_download_cancel(uint64_t did) {
	if (@available(iOS 14.5, *)) {
		TGDownload *r = downloads[@(did)];
		if (!r || !r.accepted || r.canceled) return;
		r.canceled = YES;
		[r.download cancel:^(NSData *resumeData) {
			// WebKit may or may not also call download:didFailWithError:;
			// endDownload reports only once.
			endDownload(r, TGW_DL_CANCELED, nil);
		}];
	}
}

// ---------------------------------------------------------------------------
// Delegate methods on TGPage.

static int isUserNav(WKNavigationAction *a) {
	return a.navigationType == WKNavigationTypeLinkActivated || a.navigationType == WKNavigationTypeFormSubmitted;
}

@implementation TGPage (TGWebExt)

// WKNavigationDelegate: policy

- (void)webView:(WKWebView *)webView decidePolicyForNavigationAction:(WKNavigationAction *)action
	decisionHandler:(void (^)(WKNavigationActionPolicy))decisionHandler {
	if (@available(iOS 14.5, *)) {
		if (action.shouldPerformDownload) {
			decisionHandler(WKNavigationActionPolicyDownload);
			return;
		}
	}
	// targetFrame nil: a new-window request; createWebView... reports it.
	if (action.targetFrame.isMainFrame) {
		char *u = wcstr(action.request.URL.absoluteString);
		int ok = tangoIOSShouldNavigate(self.sid, u, isUserNav(action));
		free(u);
		if (!ok) {
			decisionHandler(WKNavigationActionPolicyCancel);
			return;
		}
	}
	decisionHandler(WKNavigationActionPolicyAllow);
}

- (void)webView:(WKWebView *)webView decidePolicyForNavigationResponse:(WKNavigationResponse *)resp
	decisionHandler:(void (^)(WKNavigationResponsePolicy))decisionHandler {
	if (@available(iOS 14.5, *)) {
		BOOL attachment = NO;
		if ([resp.response isKindOfClass:NSHTTPURLResponse.class]) {
			NSString *cd = [(NSHTTPURLResponse *)resp.response valueForHTTPHeaderField:@"Content-Disposition"];
			NSString *t = [cd stringByTrimmingCharactersInSet:NSCharacterSet.whitespaceCharacterSet];
			attachment = t && [t.lowercaseString hasPrefix:@"attachment"];
		}
		if (!resp.canShowMIMEType || attachment) {
			decisionHandler(WKNavigationResponsePolicyDownload);
			return;
		}
	}
	decisionHandler(WKNavigationResponsePolicyAllow);
}

- (void)webView:(WKWebView *)webView navigationAction:(WKNavigationAction *)action
	didBecomeDownload:(WKDownload *)download API_AVAILABLE(ios(14.5)) {
	adoptDownload(self, download);
}

- (void)webView:(WKWebView *)webView navigationResponse:(WKNavigationResponse *)resp
	didBecomeDownload:(WKDownload *)download API_AVAILABLE(ios(14.5)) {
	adoptDownload(self, download);
}

// WKNavigationDelegate: HTTP authentication. Only the HTTP schemes go to
// Go; server trust and client certificates keep the default.

- (void)webView:(WKWebView *)webView didReceiveAuthenticationChallenge:(NSURLAuthenticationChallenge *)ch
	completionHandler:(void (^)(NSURLSessionAuthChallengeDisposition, NSURLCredential *))completionHandler {
	NSURLProtectionSpace *ps = ch.protectionSpace;
	NSString *m = ps.authenticationMethod, *scheme = nil;
	if ([m isEqualToString:NSURLAuthenticationMethodHTTPBasic]) scheme = @"basic";
	else if ([m isEqualToString:NSURLAuthenticationMethodHTTPDigest]) scheme = @"digest";
	else if ([m isEqualToString:NSURLAuthenticationMethodNTLM]) scheme = @"ntlm";
	else if ([m isEqualToString:NSURLAuthenticationMethodNegotiate]) scheme = @"negotiate";
	if (!scheme) {
		completionHandler(NSURLSessionAuthChallengePerformDefaultHandling, nil);
		return;
	}
	uint64_t tok = keep(completionHandler);
	char *url = wcstr(ch.failureResponse.URL.absoluteString), *host = wcstr(ps.host), *realm = wcstr(ps.realm),
		*sch = wcstr(scheme);
	tangoIOSAuth(self.sid, tok, url, host, (int)ps.port, realm, sch, ps.isProxy ? 1 : 0, (int)ch.previousFailureCount);
	free(url);
	free(host);
	free(realm);
	free(sch);
}

// WKUIDelegate: new windows. The page's window.open gets null; Go decides.

- (WKWebView *)webView:(WKWebView *)webView createWebViewWithConfiguration:(WKWebViewConfiguration *)cfg
	forNavigationAction:(WKNavigationAction *)action windowFeatures:(WKWindowFeatures *)features {
	NSString *s = action.request.URL.absoluteString ?: @"";
	if ([s isEqualToString:@"about:blank"]) s = @"";
	char *u = wcstr(s);
	tangoIOSNewWindow(self.sid, u, isUserNav(action), features.width.intValue, features.height.intValue);
	free(u);
	return nil;
}

// WKUIDelegate: permissions (iOS 15+).

- (void)webView:(WKWebView *)webView requestMediaCapturePermissionForOrigin:(WKSecurityOrigin *)origin
	initiatedByFrame:(WKFrameInfo *)frame type:(WKMediaCaptureType)type
	decisionHandler:(void (^)(WKPermissionDecision))decisionHandler API_AVAILABLE(ios(15.0)) {
	int kinds = 0;
	switch (type) {
	case WKMediaCaptureTypeCamera: kinds = TGW_PERM_CAMERA; break;
	case WKMediaCaptureTypeMicrophone: kinds = TGW_PERM_MICROPHONE; break;
	case WKMediaCaptureTypeCameraAndMicrophone: kinds = TGW_PERM_CAMERA | TGW_PERM_MICROPHONE; break;
	}
	uint64_t tok = keep(decisionHandler);
	char *o = wcstr(originOf(origin));
	tangoIOSPermission(self.sid, tok, o, kinds);
	free(o);
}

- (void)webView:(WKWebView *)webView requestDeviceOrientationAndMotionPermissionForOrigin:(WKSecurityOrigin *)origin
	initiatedByFrame:(WKFrameInfo *)frame decisionHandler:(void (^)(WKPermissionDecision))decisionHandler API_AVAILABLE(ios(15.0)) {
	uint64_t tok = keep(decisionHandler);
	char *o = wcstr(originOf(origin));
	tangoIOSPermission(self.sid, tok, o, TGW_PERM_MOTION);
	free(o);
}

@end

void tg_web_permission_done(uint64_t tok, int decision) {
	if (@available(iOS 15.0, *)) {
		void (^h)(WKPermissionDecision) = take(tok);
		if (!h) return;
		switch (decision) {
		case 2: h(WKPermissionDecisionGrant); break;  // PermissionAllow
		case 3: h(WKPermissionDecisionDeny); break;   // PermissionDeny
		default: h(WKPermissionDecisionPrompt); break; // Default, Prompt
		}
	}
}

void tg_web_auth_done(uint64_t tok, int action, const char *user, const char *password) {
	void (^h)(NSURLSessionAuthChallengeDisposition, NSURLCredential *) = take(tok);
	if (!h) return;
	switch (action) {
	case 1: // AuthCredentials
		h(NSURLSessionAuthChallengeUseCredential, [NSURLCredential credentialWithUser:wstr(user)
			password:wstr(password) persistence:NSURLCredentialPersistenceForSession]);
		break;
	case 2: // AuthCancel
		h(NSURLSessionAuthChallengeCancelAuthenticationChallenge, nil);
		break;
	default:
		h(NSURLSessionAuthChallengePerformDefaultHandling, nil);
		break;
	}
}

// ---------------------------------------------------------------------------
// User agent

void tg_web_set_user_agent(uint64_t sid, const char *ua) {
	NSString *s = wstr(ua);
	webOf(sid).customUserAgent = s.length ? s : nil;
}

// ---------------------------------------------------------------------------
// Find in page (iOS 14+). WKFindResult only says whether a match was
// found; the number of matches is counted in the page's text.

@interface TGFindState : NSObject
@property(nonatomic, copy) NSString *text;
@property(nonatomic) BOOL matchCase, noWrap;
@end

@implementation TGFindState
@end

static const void *findKey = &findKey;

static NSString *jsString(NSString *s) {
	NSData *d = [NSJSONSerialization dataWithJSONObject:@[ s ] options:0 error:nil];
	NSString *a = d ? [[NSString alloc] initWithData:d encoding:NSUTF8StringEncoding] : @"[\"\"]";
	return [a substringWithRange:NSMakeRange(1, a.length - 2)];
}

static void runFind(WKWebView *web, uint64_t tok, TGFindState *st, BOOL backward) API_AVAILABLE(ios(14.0)) {
	WKFindConfiguration *cfg = [WKFindConfiguration new];
	cfg.backwards = backward;
	cfg.caseSensitive = st.matchCase;
	cfg.wraps = !st.noWrap;
	NSString *count = [NSString stringWithFormat:
		@"(function(t,cs){var s=document.body?document.body.innerText:'';if(!t)return 0;"
		@"if(!cs){s=s.toLowerCase();t=t.toLowerCase();}var n=0,i=0;"
		@"while((i=s.indexOf(t,i))!==-1){n++;i+=t.length;}return n;})(%@,%@)",
		jsString(st.text), st.matchCase ? @"true" : @"false"];
	[web findString:st.text withConfiguration:cfg completionHandler:^(WKFindResult *res) {
		BOOL found = res.matchFound;
		[web evaluateJavaScript:count completionHandler:^(id n, NSError *err) {
			int matches = [n isKindOfClass:NSNumber.class] ? [n intValue] : -1;
			if (found && matches == 0) matches = -1; // innerText missed it (e.g. form fields)
			tangoIOSWebFound(tok, TGW_OK, (char *)"", found ? 1 : 0, matches);
		}];
	}];
}

void tg_web_find(uint64_t sid, uint64_t tok, const char *text, int matchCase, int backward, int noWrap) {
	TGPage *p = tg_page(sid);
	if (!p) {
		char *m = wcstr(noPage);
		tangoIOSWebFound(tok, TGW_FAILED, m, 0, 0);
		free(m);
		return;
	}
	if (@available(iOS 14.0, *)) {
		TGFindState *st = [TGFindState new];
		st.text = wstr(text);
		st.matchCase = matchCase != 0;
		st.noWrap = noWrap != 0;
		objc_setAssociatedObject(p, findKey, st, OBJC_ASSOCIATION_RETAIN_NONATOMIC);
		runFind(p.web, tok, st, backward != 0);
	} else {
		tangoIOSWebFound(tok, TGW_UNSUPPORTED, (char *)"", 0, 0);
	}
}

void tg_web_find_next(uint64_t sid, uint64_t tok, int backward) {
	TGPage *p = tg_page(sid);
	TGFindState *st = p ? objc_getAssociatedObject(p, findKey) : nil;
	if (!st || !st.text.length) {
		tangoIOSWebFound(tok, TGW_OK, (char *)"", 0, 0); // no current search
		return;
	}
	if (@available(iOS 14.0, *)) {
		runFind(p.web, tok, st, backward != 0);
	} else {
		tangoIOSWebFound(tok, TGW_UNSUPPORTED, (char *)"", 0, 0);
	}
}

void tg_web_find_stop(uint64_t sid) {
	TGPage *p = tg_page(sid);
	if (!p) return;
	objc_setAssociatedObject(p, findKey, nil, OBJC_ASSOCIATION_RETAIN_NONATOMIC);
	[p.web evaluateJavaScript:@"window.getSelection&&window.getSelection().removeAllRanges()" completionHandler:nil];
}

// ---------------------------------------------------------------------------
// Cookies. The store is the web view's website data store, shared by every
// page (they all use the default store).

static WKHTTPCookieStore *cookieStore(uint64_t sid) {
	return webOf(sid).configuration.websiteDataStore.httpCookieStore;
}

void tg_web_cookies(uint64_t sid, uint64_t tok) {
	WKHTTPCookieStore *cs = cookieStore(sid);
	if (!cs) {
		char *m = wcstr(noPage);
		tangoIOSWebCookies(tok, TGW_FAILED, m, NULL, 0);
		free(m);
		return;
	}
	[cs getAllCookies:^(NSArray<NSHTTPCookie *> *cookies) {
		NSMutableArray *out = [NSMutableArray arrayWithCapacity:cookies.count];
		for (NSHTTPCookie *c in cookies) {
			int ss = 0;
			if ([c.sameSitePolicy isEqualToString:NSHTTPCookieSameSiteLax]) ss = 1;
			else if ([c.sameSitePolicy isEqualToString:NSHTTPCookieSameSiteStrict]) ss = 2;
			[out addObject:@{
				@"n" : c.name ?: @"",
				@"v" : c.value ?: @"",
				@"d" : c.domain ?: @"",
				@"p" : c.path ?: @"",
				@"e" : @(c.sessionOnly || !c.expiresDate ? 0 : c.expiresDate.timeIntervalSince1970),
				@"s" : @(c.secure),
				@"h" : @(c.HTTPOnly),
				@"ss" : @(ss),
			}];
		}
		NSData *d = [NSJSONSerialization dataWithJSONObject:out options:0 error:nil] ?: [@"[]" dataUsingEncoding:NSUTF8StringEncoding];
		tangoIOSWebCookies(tok, TGW_OK, (char *)"", (char *)d.bytes, (int)d.length);
	}];
}

void tg_web_set_cookie(uint64_t sid, uint64_t tok, const char *name, const char *value, const char *domain,
	const char *path, double expires, int secure, int httpOnly, int sameSite) {
	WKHTTPCookieStore *cs = cookieStore(sid);
	if (!cs) {
		doneErr(tok, TGW_FAILED, noPage);
		return;
	}
	NSString *p = wstr(path);
	NSMutableDictionary *props = [NSMutableDictionary dictionary];
	props[NSHTTPCookieName] = wstr(name);
	props[NSHTTPCookieValue] = wstr(value);
	props[NSHTTPCookieDomain] = wstr(domain);
	props[NSHTTPCookiePath] = p.length ? p : @"/";
	if (expires != 0) props[NSHTTPCookieExpires] = [NSDate dateWithTimeIntervalSince1970:expires];
	if (secure) props[NSHTTPCookieSecure] = @"TRUE";
	// Foundation has no public key constant for HttpOnly; NSHTTPCookie
	// reads the attribute name.
	if (httpOnly) props[@"HttpOnly"] = @"TRUE";
	if (sameSite == 1) props[NSHTTPCookieSameSitePolicy] = NSHTTPCookieSameSiteLax;
	else if (sameSite == 2) props[NSHTTPCookieSameSitePolicy] = NSHTTPCookieSameSiteStrict;
	NSHTTPCookie *c = [NSHTTPCookie cookieWithProperties:props];
	if (!c) {
		doneErr(tok, TGW_FAILED, @"tango/ios: invalid cookie");
		return;
	}
	[cs setCookie:c completionHandler:^{ doneOK(tok); }];
}

// deleteMatching deletes every cookie for which match returns YES, then
// answers tok.
static void deleteMatching(uint64_t sid, uint64_t tok, BOOL (^match)(NSHTTPCookie *)) {
	WKHTTPCookieStore *cs = cookieStore(sid);
	if (!cs) {
		doneErr(tok, TGW_FAILED, noPage);
		return;
	}
	[cs getAllCookies:^(NSArray<NSHTTPCookie *> *cookies) {
		dispatch_group_t g = dispatch_group_create();
		for (NSHTTPCookie *c in cookies) {
			if (!match(c)) continue;
			dispatch_group_enter(g);
			[cs deleteCookie:c completionHandler:^{ dispatch_group_leave(g); }];
		}
		dispatch_group_notify(g, dispatch_get_main_queue(), ^{ doneOK(tok); });
	}];
}

void tg_web_delete_cookie(uint64_t sid, uint64_t tok, const char *name, const char *domain, const char *path) {
	NSString *n = wstr(name), *d = wstr(domain), *p = wstr(path);
	if (!p.length) p = @"/";
	deleteMatching(sid, tok, ^BOOL(NSHTTPCookie *c) {
		return [c.name isEqualToString:n] && [c.domain caseInsensitiveCompare:d] == NSOrderedSame &&
			[(c.path.length ? c.path : @"/") isEqualToString:p];
	});
}

static NSString *bareDomain(NSString *d) {
	d = d.lowercaseString;
	return [d hasPrefix:@"."] ? [d substringFromIndex:1] : d;
}

void tg_web_clear_cookies(uint64_t sid, uint64_t tok, const char *domain) {
	NSString *want = bareDomain(wstr(domain));
	deleteMatching(sid, tok, ^BOOL(NSHTTPCookie *c) {
		if (!want.length) return YES;
		NSString *d = bareDomain(c.domain);
		return [d isEqualToString:want] || [d hasSuffix:[@"." stringByAppendingString:want]];
	});
}

// ---------------------------------------------------------------------------
// Website data

void tg_web_clear_data(uint64_t sid, uint64_t tok, uint32_t kinds) {
	WKWebsiteDataStore *ds = webOf(sid).configuration.websiteDataStore;
	if (!ds) {
		doneErr(tok, TGW_FAILED, noPage);
		return;
	}
	NSMutableSet<NSString *> *types = [NSMutableSet set];
	if (kinds & 1) [types addObject:WKWebsiteDataTypeCookies];
	if (kinds & 2) [types addObjectsFromArray:@[ WKWebsiteDataTypeDiskCache, WKWebsiteDataTypeMemoryCache ]];
	if (kinds & 4) {
		[types addObjectsFromArray:@[
			WKWebsiteDataTypeLocalStorage, WKWebsiteDataTypeSessionStorage, WKWebsiteDataTypeIndexedDBDatabases,
			WKWebsiteDataTypeWebSQLDatabases, WKWebsiteDataTypeServiceWorkerRegistrations, WKWebsiteDataTypeFetchCache
		]];
		if (@available(iOS 16.0, *)) [types addObject:WKWebsiteDataTypeFileSystem];
	}
	if (!types.count) {
		doneOK(tok);
		return;
	}
	[ds removeDataOfTypes:types modifiedSince:NSDate.distantPast completionHandler:^{ doneOK(tok); }];
}

// ---------------------------------------------------------------------------
// PDF (iOS 14+) and snapshot

void tg_web_pdf(uint64_t sid, uint64_t tok, int noBackground) {
	WKWebView *web = webOf(sid);
	if (!web) {
		doneBytes(tok, nil, nil, noPage);
		return;
	}
	if (@available(iOS 14.0, *)) {
		WKPDFConfiguration *cfg = [WKPDFConfiguration new];
		if (@available(iOS 17.0, *)) cfg.allowTransparentBackground = noBackground != 0;
		[web createPDFWithConfiguration:cfg completionHandler:^(NSData *pdf, NSError *err) {
			doneBytes(tok, pdf, err, @"tango/ios: PDF creation failed");
		}];
	} else {
		tangoIOSWebBytes(tok, TGW_UNSUPPORTED, (char *)"", NULL, 0);
	}
}

void tg_web_snapshot(uint64_t sid, uint64_t tok) {
	WKWebView *web = webOf(sid);
	if (!web) {
		doneBytes(tok, nil, nil, noPage);
		return;
	}
	[web takeSnapshotWithConfiguration:nil completionHandler:^(UIImage *img, NSError *err) {
		NSData *png = img ? UIImagePNGRepresentation(img) : nil;
		doneBytes(tok, png, err, @"tango/ios: snapshot failed");
	}];
}
