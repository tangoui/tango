//go:build android

#include "webview_ext_android.h"

void tg_get_bytes(JNIEnv *env, jbyteArray a, jsize n, void *out) {
	(*env)->GetByteArrayRegion(env, a, 0, n, (jbyte *)out);
}
