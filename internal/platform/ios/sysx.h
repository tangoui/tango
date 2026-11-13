// C interface of the desktop-style system capabilities of the iOS backend
// (sysx.m) for the Go side (sysx.go): opening URLs and files (Shell), local
// notifications (Notifier), connectivity (Network) and battery / Low Power
// Mode (Power). Main thread only unless marked otherwise. Codes are the
// TGM_* values of mobile.h; asynchronous calls answer through
// tangoIOSMobileDone with their token.

#ifndef TANGO_IOS_SYSX_H
#define TANGO_IOS_SYSX_H

// Shell. tgx_open_url hands url to UIApplication (Safari, Mail, Phone,
// other apps' schemes); the system answers later and failures are logged.
// tgx_open_path shows a QuickLook preview of a file (or the "Open in…"
// menu for types QuickLook cannot show).
int tgx_open_url(const char *url);
int tgx_open_path(const char *path);

// Local notifications. tgx_notify_permission answers with "1" (allowed)
// or "0"; it asks the user first when they were never asked.
// tgx_notify returns TGM_PERMISSION_DENIED when the user turned
// notifications off; otherwise it asks for permission when needed and
// posts the notification asynchronously. Clicks on local notifications
// are reported as TGE_NOTIFICATION_OPENED app events with the request id.
void tgx_notify_permission(int tok);
int tgx_notify(const char *ident, const char *title, const char *subtitle, const char *body, int silent);

// Network (Network.framework path monitor, started on first use). Any
// thread. kind: platform.NetworkKind values. Changes are reported through
// tangoIOSNetworkChanged on the main thread once tgx_network_watch(1) ran.
void tgx_network_status(int *online, int *kind, int *expensive, int *constrained);
void tgx_network_watch(int on);

// Power: battery state (UIDevice battery monitoring) and Low Power Mode.
// state: 0 unknown, 1 unplugged, 2 charging, 3 full. level: [0,1] or -1.
// Changes are reported through tangoIOSPowerChanged on the main thread
// once tgx_power_watch(1) ran.
void tgx_power_status(int *state, double *level, int *lowPower);
void tgx_power_watch(int on);

#endif
