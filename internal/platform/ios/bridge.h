// C interface between the Go half of the iOS backend and its UIKit half
// (bridge.m). Every function is main-thread only unless marked otherwise.
// Strings are UTF-8 and owned by the caller; functions returning char*
// return malloc'd memory the caller frees.

#ifndef TANGO_IOS_BRIDGE_H
#define TANGO_IOS_BRIDGE_H

#include <stdint.h>
#include <stddef.h>

// Lifecycle codes passed to tangoIOSLifecycle (Go). They are not
// platform.LifecycleEvent values: Go maps them.
enum {
	TG_DID_BECOME_ACTIVE = 1,
	TG_WILL_RESIGN_ACTIVE = 2,
	TG_DID_ENTER_BACKGROUND = 3,
	TG_WILL_ENTER_FOREGROUND = 4,
};

// App.
void tg_install(void);           // from TangoStart: wake source, observers, window
void tg_wake(void);              // any thread
int tg_step(int block);          // runs the main run loop once
int tg_app_active(void);         // UIApplicationStateActive?
void tg_exit_on_main(int code);  // any thread: exit(code) from the main queue

// Surfaces (pages). sid is the core's SurfaceID.
void tg_surface_new(uint64_t sid, const char *shim, const char *bridge, int debug, const char *title);
void tg_surface_show(uint64_t sid);
void tg_surface_hide(uint64_t sid);
void tg_surface_close(uint64_t sid);
void tg_surface_set_title(uint64_t sid, const char *title);
void tg_surface_size(uint64_t sid, int *w, int *h);
void tg_surface_navigate(uint64_t sid, const char *url);
void tg_surface_eval(uint64_t sid, const char *js);
uintptr_t tg_surface_native(uint64_t sid);

// Asset scheme tasks (tango://). task is the handle given to tangoIOSTaskStart.
void tg_task_respond(uint64_t task, int status, const char *headers); // "Key: value\n" lines
void tg_task_data(uint64_t task, const void *p, size_t n);
void tg_task_finish(uint64_t task);

// Clipboard.
char *tg_clipboard_read(void); // NULL when the pasteboard holds no text
void tg_clipboard_write(const char *text);

// Dialogs. Results come back through tangoIOSAlertDone / tangoIOSPickerDone
// with the same token. Both return 0 when the dialog is showing, -1 when
// there is no view controller to present it from (the callback then never
// runs).
int tg_alert(int token, const char *title, const char *message, const char *buttons); // buttons: "\n"-separated
int tg_pick(int token, const char *exts, int multiple, int folders, const char *dir);  // exts: "\n"-separated, "" = any
void tg_dismiss_dialogs(void);

#endif
