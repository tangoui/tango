// Thin C wrappers over the JNI function table, which cgo cannot call
// directly. Object arguments and results are JNI references; strings cross
// as UTF-16 (JNI's "modified UTF-8" mangles characters outside the BMP).

#ifndef TANGO_ANDROID_JNI_H
#define TANGO_ANDROID_JNI_H

#include <jni.h>
#include <stdint.h>

int tg_init(JNIEnv *env);                 // remember the JavaVM
JNIEnv *tg_env(int *attached);            // env for this thread, attaching it if needed
void tg_detach(void);
int tg_is_main_thread(void);
void tg_log(int prio, const char *msg);

int tg_exception(JNIEnv *env);            // 1 if an exception was pending (logged and cleared)
int tg_push_frame(JNIEnv *env, int capacity);
void tg_pop_frame(JNIEnv *env);

jclass tg_find_class(JNIEnv *env, const char *name);   // global ref, or NULL
jmethodID tg_method(JNIEnv *env, jclass cls, const char *name, const char *sig, int is_static);

void tg_call_static_void(JNIEnv *env, jclass cls, jmethodID m, const jvalue *args);
jobject tg_call_static_object(JNIEnv *env, jclass cls, jmethodID m, const jvalue *args);
void tg_call_void(JNIEnv *env, jobject obj, jmethodID m, const jvalue *args);
jobject tg_call_object(JNIEnv *env, jobject obj, jmethodID m, const jvalue *args);

jobject tg_new_global(JNIEnv *env, jobject obj);
void tg_delete_local(JNIEnv *env, jobject obj);
void tg_delete_global(JNIEnv *env, jobject obj);

jstring tg_new_string(JNIEnv *env, const jchar *s, jsize n);
jsize tg_string_length(JNIEnv *env, jstring s);
void tg_string_chars(JNIEnv *env, jstring s, jsize n, jchar *out);

jobjectArray tg_new_string_array(JNIEnv *env, jclass string_class, jsize n);
jsize tg_array_length(JNIEnv *env, jarray a);
jobject tg_array_get(JNIEnv *env, jobjectArray a, jsize i);
void tg_array_set(JNIEnv *env, jobjectArray a, jsize i, jobject v);

jbyteArray tg_new_bytes(JNIEnv *env, const void *p, jsize n);
void tg_set_bytes(JNIEnv *env, jbyteArray a, jsize off, jsize n, const void *p);

#endif
