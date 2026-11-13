// C interface of the system settings the canvas UI follows (system.m) for
// the Go side (system.go): light/dark appearance, reduce motion, increase
// contrast and the preferred languages (docs/specs/mobile-canvas.md §3.3),
// plus the system's memory warnings (§3.4).
// Values are cached on the main thread, so the getters may be called from
// any thread. Changes are reported through tangoIOSSystemChanged with one
// of the TGS_* kinds, on the main thread.

#ifndef TANGO_IOS_SYSTEM_H
#define TANGO_IOS_SYSTEM_H

enum {
	TGS_THEME = 1,
	TGS_MOTION = 2,
	TGS_CONTRAST = 3,
	TGS_LOCALE = 4,
	TGS_MEMORY = 5, // UIApplicationDidReceiveMemoryWarningNotification (an event, not a setting)
};

void tgs_install(void);       // main thread, from TangoStart: read the settings, observe changes
void tgs_refresh(void);       // main thread: read again and report what changed
int tgs_dark(void);           // any thread
int tgs_reduce_motion(void);  // any thread
int tgs_high_contrast(void);  // any thread
char *tgs_locales(void);      // any thread: "\n"-separated BCP 47 tags, malloc'd
void tgs_watch_locales(void); // any thread: start reporting TGS_LOCALE (idempotent)

#endif
