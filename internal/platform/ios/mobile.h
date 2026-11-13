// C interface of the mobile capabilities (mobile.m) for the Go side
// (mobile.go). Main thread only unless marked otherwise. Asynchronous
// functions take a token and answer through tangoIOSMobileDone (or
// tangoIOSLocation) on the main thread with one of the TGM_* codes.

#ifndef TANGO_IOS_MOBILE_H
#define TANGO_IOS_MOBILE_H

#include <stdint.h>
#include <stddef.h>

// Result codes; Go maps them to platform errors.
enum {
	TGM_OK = 0,
	TGM_CANCELED = 1,
	TGM_AUTH_FAILED = 2,
	TGM_NOT_ENROLLED = 3,
	TGM_LOCKED_OUT = 4,
	TGM_NOT_AVAILABLE = 5,
	TGM_NOT_CONFIGURED = 6,
	TGM_PERMISSION_DENIED = 7,
	TGM_UNSUPPORTED = 8,
	TGM_ERROR = 9,
	TGM_NOT_FOUND = 10,
};

// App events tangoIOSAppEvent reports (Go maps them to AppEventKind).
enum {
	TGE_OPEN_URL = 1,         // str = URL
	TGE_SAFE_AREA = 2,
	TGE_ORIENTATION = 3,
	TGE_PUSH_TOKEN = 4,       // str = token
	TGE_PUSH_RECEIVED = 5,    // str = payload JSON
	TGE_PUSH_OPENED = 6,      // str = payload JSON
	TGE_NOTIFICATION_OPENED = 7, // str = local notification request id (sysx.m)
};

typedef struct {
	double lat, lon, alt, acc, alt_acc, speed, course, time; // time: Unix seconds
} tg_location;

// Share sheet. paths: "\n"-separated.
void tgm_share(int tok, const char *text, const char *url, const char *paths, const char *title);

// Haptics: platform.HapticKind values.
void tgm_haptic(int kind);

// Safe area (points) and orientation (platform.Orientation values).
void tgm_safe_area(double *top, double *bottom, double *left, double *right);
int tgm_orientation(void);
void tgm_lock_orientation(int mask); // platform.OrientationMask bits; 0 unlocks

// Biometrics. kind: platform.BiometryKind; returns a TGM_* code.
int tgm_biometry(int *kind);
void tgm_authenticate(int tok, const char *reason, const char *cancel, int passcode);

// Keychain (any thread). Return TGM_* codes. tgm_secure_get sets *out to a
// malloc'd buffer the caller frees.
int tgm_secure_set(const char *key, const void *p, int n);
int tgm_secure_get(const char *key, void **out, int *n);
int tgm_secure_delete(const char *key);

// Photos and camera. Results: "\n"-separated paths in str.
void tgm_pick_media(int tok, int types, int limit);
void tgm_capture_photo(int tok);

// Location. Answers through tangoIOSLocation; the updates token stays
// registered until an error or tgm_location_stop.
void tgm_location_once(int tok, int accuracy);
void tgm_location_start(int tok, int accuracy, double distance);
void tgm_location_stop(void);

// Push.
void tgm_push_register(int tok); // str = token

// Permissions: name as in platform.Permission; str = platform.PermissionStatus.
void tgm_permission(int tok, const char *name, int request);

#endif
