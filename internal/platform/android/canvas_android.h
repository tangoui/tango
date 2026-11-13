// Native-window presentation of the Android canvas (canvas_android.go):
// ANativeWindow from a Surface, buffer geometry and the CPU frame copy.

#ifndef TANGO_ANDROID_CANVAS_H
#define TANGO_ANDROID_CANVAS_H

#include <jni.h>
#include <stdint.h>

// tg_cv_window returns the ANativeWindow of a android.view.Surface (one
// reference, released with tg_cv_release), or NULL.
void *tg_cv_window(JNIEnv *env, jobject surface);
void tg_cv_release(void *win);

// tg_cv_geometry sets the buffer size (device pixels) and an opaque
// RGBX_8888 format. 0 on success.
int tg_cv_geometry(void *win, int w, int h);

// tg_cv_present copies a premultiplied RGBA image (pix, stride bytes per
// row, iw x ih pixels) into the next buffer and posts it.
//
// dirty (x0, y0, x1, y1 in device pixels) asks for a partial update; NULL
// or an empty rectangle means the whole buffer. On return it holds the
// region actually copied, which the system may have grown (it copies the
// rest back from the previous buffer). Buffer pixels outside the image
// repeat its last column / row, so a 1 px size mismatch shows no garbage.
//
// timing, when not NULL, receives the nanoseconds spent waiting for a
// buffer (lock), copying and posting.
//
// Returns 0 on success, 1 if the buffer format needed the slow path
// (converted; logged by the caller once), negative on failure.
int tg_cv_present(void *win, const uint8_t *pix, int stride, int iw, int ih, int32_t *dirty, int64_t *timing);

// tg_cv_floats copies up to max elements of a float[] into out and
// returns how many there are (which may exceed max: nothing is copied then).
int tg_cv_floats(JNIEnv *env, jfloatArray a, float *out, int max);

// tg_cv_debug returns the debug level the system property
// debug.tango.canvas sets (adb shell setprop debug.tango.canvas 1): 1 logs
// input and frame-time summaries, 2 also every frame; 0 when unset.
int tg_cv_debug(void);

#endif
