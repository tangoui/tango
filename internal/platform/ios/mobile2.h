// C interface of the round-4 mobile capabilities (mobile2.m) for the Go
// side (mobile2.go): background tasks, in-app purchase, store review,
// settings, screen, status bar and keyboard. Main thread only unless
// marked otherwise. Codes are the TGM_* values of mobile.h; asynchronous
// calls answer through tangoIOSMobileDone with their token.

#ifndef TANGO_IOS_MOBILE2_H
#define TANGO_IOS_MOBILE2_H

#include <stdint.h>

// From tg_mobile_install (didFinishLaunching): BGTask registration,
// keyboard observers.
void tgm2_install(void);

// Background tasks. kind: platform.BackgroundKind. err: malloc'd message
// or NULL, freed by the caller. Started and expired tasks are reported
// through tangoIOSBackgroundTask(id, kind, expired).
int tgm2_bg_schedule(int kind, double earliest, int network, int power, char **err);
int tgm2_bg_cancel(int kind);
void tgm2_bg_finish(int64_t tid, int success);

// In-app purchase (StoreKit, loaded on first use). Transaction updates go
// to tangoIOSTransaction(json); products answer with a JSON object
// {"products":[...],"invalid":[...]}; purchases with the transaction JSON
// (also on failure).
int tgm2_can_pay(void);
void tgm2_products(int tok, const char *ids); // "\n"-separated
void tgm2_buy(int tok, const char *product, int quantity);
void tgm2_restore(int tok);
int tgm2_finish_transaction(const char *id);
int tgm2_receipt(void **out, int *n); // *out malloc'd

// Device controls.
void tgm2_review(void);
int tgm2_open_settings(void);
void tgm2_keep_screen_on(int on);
double tgm2_brightness(void);
void tgm2_set_brightness(double v); // < 0: leave it to the system
void tgm2_status_bar(int style, int hidden); // platform.StatusBarStyle
double tgm2_keyboard_height(void);
void tgm2_hide_keyboard(void);

// For mobile.m: the status bar style and visibility the pages report.
extern int tgm2_bar_style, tgm2_bar_hidden;

#endif
