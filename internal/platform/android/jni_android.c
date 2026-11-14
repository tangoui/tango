//go:build android

#include <android/log.h>
#include <sys/types.h>
#include <unistd.h>

#include "jni_android.h"

static JavaVM *tg_vm;

int tg_init(JNIEnv *env) {
	if (tg_vm != NULL) return 1;
	return (*env)->GetJavaVM(env, &tg_vm) == JNI_OK;
}

JNIEnv *tg_env(int *attached) {
	*attached = 0;
	if (tg_vm == NULL) return NULL;
	JNIEnv *env = NULL;
	jint r = (*tg_vm)->GetEnv(tg_vm, (void **)&env, JNI_VERSION_1_6);
	if (r == JNI_OK) return env;
	if (r != JNI_EDETACHED) return NULL;
	if ((*tg_vm)->AttachCurrentThread(tg_vm, &env, NULL) != JNI_OK) return NULL;
	*attached = 1;
	return env;
}

void tg_detach(void) {
	if (tg_vm != NULL) (*tg_vm)->DetachCurrentThread(tg_vm);
}

int tg_is_main_thread(void) { return gettid() == getpid(); }

void tg_log(int prio, const char *msg) { __android_log_write(prio, "Tango", msg); }

int tg_exception(JNIEnv *env) {
	if (!(*env)->ExceptionCheck(env)) return 0;
	(*env)->ExceptionDescribe(env);
	(*env)->ExceptionClear(env);
	return 1;
}

int tg_push_frame(JNIEnv *env, int capacity) { return (*env)->PushLocalFrame(env, capacity) == JNI_OK; }
void tg_pop_frame(JNIEnv *env) { (*env)->PopLocalFrame(env, NULL); }

jclass tg_find_class(JNIEnv *env, const char *name) {
	jclass local = (*env)->FindClass(env, name);
	if (local == NULL) {
		tg_exception(env);
		return NULL;
	}
	jclass global = (*env)->NewGlobalRef(env, local);
	(*env)->DeleteLocalRef(env, local);
	return global;
}

jmethodID tg_method(JNIEnv *env, jclass cls, const char *name, const char *sig, int is_static) {
	jmethodID m = is_static ? (*env)->GetStaticMethodID(env, cls, name, sig)
	                        : (*env)->GetMethodID(env, cls, name, sig);
	if (m == NULL) tg_exception(env);
	return m;
}

void tg_call_static_void(JNIEnv *env, jclass cls, jmethodID m, const jvalue *args) {
	(*env)->CallStaticVoidMethodA(env, cls, m, args);
}

jobject tg_call_static_object(JNIEnv *env, jclass cls, jmethodID m, const jvalue *args) {
	return (*env)->CallStaticObjectMethodA(env, cls, m, args);
}

void tg_call_void(JNIEnv *env, jobject obj, jmethodID m, const jvalue *args) {
	(*env)->CallVoidMethodA(env, obj, m, args);
}

jobject tg_call_object(JNIEnv *env, jobject obj, jmethodID m, const jvalue *args) {
	return (*env)->CallObjectMethodA(env, obj, m, args);
}

jboolean tg_call_boolean(JNIEnv *env, jobject obj, jmethodID m, const jvalue *args) {
	return (*env)->CallBooleanMethodA(env, obj, m, args);
}

jboolean tg_call_static_boolean(JNIEnv *env, jclass cls, jmethodID m, const jvalue *args) {
	return (*env)->CallStaticBooleanMethodA(env, cls, m, args);
}

jobject tg_new_global(JNIEnv *env, jobject obj) { return obj == NULL ? NULL : (*env)->NewGlobalRef(env, obj); }
void tg_delete_local(JNIEnv *env, jobject obj) { if (obj != NULL) (*env)->DeleteLocalRef(env, obj); }
void tg_delete_global(JNIEnv *env, jobject obj) { if (obj != NULL) (*env)->DeleteGlobalRef(env, obj); }

jstring tg_new_string(JNIEnv *env, const jchar *s, jsize n) { return (*env)->NewString(env, s, n); }
jsize tg_string_length(JNIEnv *env, jstring s) { return (*env)->GetStringLength(env, s); }
void tg_string_chars(JNIEnv *env, jstring s, jsize n, jchar *out) { (*env)->GetStringRegion(env, s, 0, n, out); }

jobjectArray tg_new_string_array(JNIEnv *env, jclass string_class, jsize n) {
	return (*env)->NewObjectArray(env, n, string_class, NULL);
}
jsize tg_array_length(JNIEnv *env, jarray a) { return (*env)->GetArrayLength(env, a); }
jobject tg_array_get(JNIEnv *env, jobjectArray a, jsize i) { return (*env)->GetObjectArrayElement(env, a, i); }
void tg_array_set(JNIEnv *env, jobjectArray a, jsize i, jobject v) { (*env)->SetObjectArrayElement(env, a, i, v); }

jbyteArray tg_new_bytes(JNIEnv *env, const void *p, jsize n) {
	jbyteArray a = (*env)->NewByteArray(env, n);
	if (a != NULL && n > 0) (*env)->SetByteArrayRegion(env, a, 0, n, (const jbyte *)p);
	return a;
}

void tg_set_bytes(JNIEnv *env, jbyteArray a, jsize off, jsize n, const void *p) {
	(*env)->SetByteArrayRegion(env, a, off, n, (const jbyte *)p);
}
