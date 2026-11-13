// JNI helpers used by webview_ext_android.go only.

#ifndef TANGO_ANDROID_WEBVIEW_EXT_H
#define TANGO_ANDROID_WEBVIEW_EXT_H

#include <jni.h>

// Copies the contents of a byte[] (n = its length) into out.
void tg_get_bytes(JNIEnv *env, jbyteArray a, jsize n, void *out);

#endif
