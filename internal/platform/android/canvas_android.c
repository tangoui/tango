//go:build android

#include <android/native_window.h>
#include <android/native_window_jni.h>
#include <string.h>
#include <sys/system_properties.h>
#include <time.h>

#include "canvas_android.h"

// ANativeWindow_LegacyFormat / AHardwareBuffer formats this file handles.
enum {
	TG_FMT_RGBA_8888 = 1,
	TG_FMT_RGBX_8888 = 2,
	TG_FMT_RGB_565 = 4,
};

void *tg_cv_window(JNIEnv *env, jobject surface) {
	if (surface == NULL) return NULL;
	return ANativeWindow_fromSurface(env, surface);
}

void tg_cv_release(void *win) {
	if (win != NULL) ANativeWindow_release((ANativeWindow *)win);
}

int tg_cv_geometry(void *win, int w, int h) {
	if (win == NULL) return -1;
	return ANativeWindow_setBuffersGeometry((ANativeWindow *)win, w, h, TG_FMT_RGBX_8888);
}

// row32 copies one row of n pixels from src (iw pixels) to a 32-bit
// buffer row, repeating the last source pixel past iw.
static void row32(uint32_t *dst, const uint32_t *src, int x0, int x1, int iw) {
	int end = x1 < iw ? x1 : iw;
	if (end > x0) memcpy(dst + x0, src + x0, (size_t)(end - x0) * 4);
	if (x1 > end && iw > 0) {
		uint32_t last = src[iw - 1];
		for (int x = end > x0 ? end : x0; x < x1; x++) dst[x] = last;
	}
}

// row565 is row32 converting to RGB_565 (slow path: some emulator images).
static void row565(uint16_t *dst, const uint8_t *src, int x0, int x1, int iw) {
	for (int x = x0; x < x1; x++) {
		const uint8_t *p = src + (size_t)(x < iw ? x : iw - 1) * 4;
		dst[x] = (uint16_t)(((p[0] >> 3) << 11) | ((p[1] >> 2) << 5) | (p[2] >> 3));
	}
}

static int64_t now_ns(void) {
	struct timespec ts;
	clock_gettime(CLOCK_MONOTONIC, &ts);
	return (int64_t)ts.tv_sec * 1000000000 + ts.tv_nsec;
}

int tg_cv_present(void *win, const uint8_t *pix, int stride, int iw, int ih, int32_t *dirty, int64_t *timing) {
	if (win == NULL) return -1;
	if (iw <= 0 || ih <= 0) return -2;
	ANativeWindow *w = (ANativeWindow *)win;
	ANativeWindow_Buffer buf;
	ARect r, *rp = NULL;
	if (dirty != NULL && dirty[2] > dirty[0] && dirty[3] > dirty[1]) {
		r.left = dirty[0];
		r.top = dirty[1];
		r.right = dirty[2];
		r.bottom = dirty[3];
		rp = &r;
	}
	int64_t t0 = timing != NULL ? now_ns() : 0;
	if (ANativeWindow_lock(w, &buf, rp) != 0) return -3;
	int64_t t1 = timing != NULL ? now_ns() : 0;
	int x0 = 0, y0 = 0, x1 = buf.width, y1 = buf.height;
	if (rp != NULL) {
		// The system may have grown the region (copy-back): copy what it says.
		x0 = r.left < 0 ? 0 : r.left;
		y0 = r.top < 0 ? 0 : r.top;
		x1 = r.right > buf.width ? buf.width : r.right;
		y1 = r.bottom > buf.height ? buf.height : r.bottom;
	}
	int ret = 0;
	for (int y = y0; y < y1; y++) {
		const uint8_t *src = pix + (size_t)(y < ih ? y : ih - 1) * (size_t)stride;
		switch (buf.format) {
		case TG_FMT_RGBA_8888:
		case TG_FMT_RGBX_8888:
			row32((uint32_t *)buf.bits + (size_t)y * (size_t)buf.stride, (const uint32_t *)src, x0, x1, iw);
			break;
		case TG_FMT_RGB_565:
			row565((uint16_t *)buf.bits + (size_t)y * (size_t)buf.stride, src, x0, x1, iw);
			ret = 1;
			break;
		default:
			ret = -4;
			y = y1;
			break;
		}
	}
	if (dirty != NULL) {
		dirty[0] = x0;
		dirty[1] = y0;
		dirty[2] = x1;
		dirty[3] = y1;
	}
	int64_t t2 = timing != NULL ? now_ns() : 0;
	ANativeWindow_unlockAndPost(w);
	if (timing != NULL) {
		timing[0] = t1 - t0;          // waiting for a free buffer
		timing[1] = t2 - t1;          // copying
		timing[2] = now_ns() - t2;    // posting
	}
	return ret;
}

int tg_cv_floats(JNIEnv *env, jfloatArray a, float *out, int max) {
	if (a == NULL) return 0;
	jsize n = (*env)->GetArrayLength(env, a);
	if (n <= max && n > 0) (*env)->GetFloatArrayRegion(env, a, 0, n, out);
	return n;
}

int tg_cv_debug(void) {
	char v[PROP_VALUE_MAX];
	if (__system_property_get("debug.tango.canvas", v) <= 0) return 0;
	return v[0] >= '1' && v[0] <= '9' ? v[0] - '0' : 0;
}
