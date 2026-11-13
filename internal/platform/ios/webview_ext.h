// C interface for the advanced WebView features of the iOS backend
// (internal/platform/webview_ext.go), implemented in webview_ext.m. Same
// rules as bridge.h: main thread only, strings are UTF-8 owned by the
// caller.
//
// Results of asynchronous calls come back through Go exports with the
// caller's token: tangoIOSWebDone (error only), tangoIOSWebBytes (data),
// tangoIOSWebCookies (JSON array) and tangoIOSWebFound. code is one of
// TGW_OK / TGW_UNSUPPORTED / TGW_FAILED (msg says why).

#ifndef TANGO_IOS_WEBVIEW_EXT_H
#define TANGO_IOS_WEBVIEW_EXT_H

#include <stdint.h>

enum {
	TGW_OK = 0,
	TGW_UNSUPPORTED = 1,
	TGW_FAILED = 2,
};

// Permission kinds passed to tangoIOSPermission (bit set).
enum {
	TGW_PERM_CAMERA = 1,
	TGW_PERM_MICROPHONE = 2,
	TGW_PERM_MOTION = 4,
};

// Download states passed to tangoIOSDownloadEvent; equal to
// platform.DownloadState values.
enum {
	TGW_DL_PROGRESS = 1,
	TGW_DL_DONE = 2,
	TGW_DL_FAILED = 3,
	TGW_DL_CANCELED = 4,
};

// Answers to the delegate callbacks (tok is the one the export received).
void tg_web_permission_done(uint64_t tok, int decision);  // platform.PermissionDecision
void tg_web_auth_done(uint64_t tok, int action, const char *user, const char *password); // platform.AuthAction
void tg_web_download_dest(uint64_t did, const char *path); // "" cancels
void tg_web_download_cancel(uint64_t did);

// Surface controls.
void tg_web_set_user_agent(uint64_t sid, const char *ua); // "" = platform default
void tg_web_find(uint64_t sid, uint64_t tok, const char *text, int matchCase, int backward, int noWrap);
void tg_web_find_next(uint64_t sid, uint64_t tok, int backward);
void tg_web_find_stop(uint64_t sid);
void tg_web_cookies(uint64_t sid, uint64_t tok);
void tg_web_set_cookie(uint64_t sid, uint64_t tok, const char *name, const char *value, const char *domain,
	const char *path, double expires, int secure, int httpOnly, int sameSite); // expires: Unix seconds, 0 = session; sameSite: 0 none/default, 1 lax, 2 strict
void tg_web_delete_cookie(uint64_t sid, uint64_t tok, const char *name, const char *domain, const char *path);
void tg_web_clear_cookies(uint64_t sid, uint64_t tok, const char *domain);
void tg_web_clear_data(uint64_t sid, uint64_t tok, uint32_t kinds); // platform.BrowsingData
void tg_web_pdf(uint64_t sid, uint64_t tok, int noBackground);
void tg_web_snapshot(uint64_t sid, uint64_t tok);

#endif
