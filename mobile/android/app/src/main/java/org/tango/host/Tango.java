package org.tango.host;

import android.app.Activity;
import android.content.ClipData;
import android.content.ClipboardManager;
import android.content.Context;
import android.os.Handler;
import android.os.Looper;
import android.util.Log;
import android.webkit.WebResourceResponse;

import androidx.webkit.WebViewFeature;

/**
 * The JNI boundary between the Go backend (internal/platform/android) and
 * the Java host. Native methods are implemented in libtango.so; the static
 * methods below are called from Go, on the main thread unless noted.
 */
public final class Tango {
    static final String TAG = "Tango";

    // platform.LifecycleEvent values.
    static final int STARTED = 1, FOREGROUND = 2, BACKGROUND = 3, TERMINATING = 4;
    // platform.SurfaceEventKind values.
    static final int CLOSED = 1, RESIZED = 2, FOCUSED = 3, BLURRED = 4, LOADED = 5,
            NAVIGATING = 6, LOAD_FAILED = 7;

    static final String ASSET_SCHEME = "https";
    static final String ASSET_HOST = "tango.localhost";

    static final Handler main = new Handler(Looper.getMainLooper());
    /** The current Activity instance; null between instances and in background-only runs. */
    static TangoActivity activity;
    /** Go was started in this process (start). */
    static boolean started;

    private static final Runnable drain = Tango::nativeDrain;

    static {
        System.loadLibrary("tango");
    }

    private Tango() {}

    /**
     * Starts Go once per process: from the Activity, or from a component
     * the system starts without one (TangoJobService for a background
     * task). Main thread.
     */
    static void start(Context c) {
        if (Mobile.app == null) Mobile.app = c.getApplicationContext();
        if (started) return;
        started = true;
        nativeInit();
    }

    static boolean isMainThread() {
        return Looper.myLooper() == Looper.getMainLooper();
    }

    // ---- called from Go ----

    /** Schedules nativeDrain on the main thread. Any thread. */
    static void wake() {
        main.post(drain);
    }

    /** Finishes the Activity; its onDestroy reports TERMINATING. */
    static void quit() {
        TangoActivity a = activity;
        if (a != null && !a.isFinishing()) {
            a.finishAndRemoveTask();
        } else if (a == null) {
            nativeLifecycle(TERMINATING);
        }
    }

    /** main.main returned: end the process, as on desktop. Any thread. */
    static void exit() {
        main.post(() -> {
            Activity a = activity;
            if (a != null && !a.isFinishing()) a.finishAndRemoveTask();
            main.post(() -> System.exit(0));
        });
    }

    /**
     * Without an Activity (between instances, or launched for a background
     * task) the page is created on the application context and shown when
     * an Activity comes.
     */
    static Page newPage(long id, String url, boolean visible, boolean debug, String title) {
        Context base = activity != null ? activity : Mobile.app;
        if (base == null) return null;
        Page p = new Page(base, id, debug, title);
        TangoActivity.addPage(p, visible);
        p.navigate(url);
        return p;
    }

    /**
     * Whether pages can use the Android IPC fast lanes: ArrayBuffer messages
     * over a WebMessageListener (WebView 116 and later).
     */
    static boolean laneSupported() {
        return WebViewFeature.isFeatureSupported(WebViewFeature.WEB_MESSAGE_LISTENER)
                && WebViewFeature.isFeatureSupported(WebViewFeature.WEB_MESSAGE_ARRAY_BUFFER);
    }

    static String clipboardRead() {
        TangoActivity a = activity;
        if (a == null) return null;
        ClipboardManager cm = (ClipboardManager) a.getSystemService(Context.CLIPBOARD_SERVICE);
        ClipData clip = cm.getPrimaryClip();
        if (clip == null || clip.getItemCount() == 0) return null;
        CharSequence text = clip.getItemAt(0).coerceToText(a);
        return text == null ? null : text.toString();
    }

    static void clipboardWrite(String text) {
        TangoActivity a = activity;
        if (a == null) throw new IllegalStateException("no Activity");
        ClipboardManager cm = (ClipboardManager) a.getSystemService(Context.CLIPBOARD_SERVICE);
        cm.setPrimaryClip(ClipData.newPlainText("text", text));
    }

    static void messageDialog(int cb, int kind, String title, String message, String[] buttons) {
        Dialogs.message(activity, cb, kind, title, message, buttons);
    }

    static void openDocuments(int cb, String title, String[] patterns, boolean multiple) {
        Dialogs.open(activity, cb, title, patterns, multiple);
    }

    static void log(String msg, Throwable t) {
        Log.e(TAG, msg, t);
    }

    // ---- implemented in Go ----

    /** Starts main.main on a goroutine, once per process. */
    static native void nativeInit();
    /** Runs the Go main-thread queue. */
    static native void nativeDrain();
    static native void nativeLifecycle(int event);
    /** Asks Host.ShouldClose; true means close the page. */
    static native boolean nativeShouldClose(long id);
    static native void nativeSurfaceEvent(long id, int kind, int width, int height, String url, String error);
    /**
     * A main-frame document starts loading: the previous document's bridge
     * token stops working. Stores in script[0] the page script to inject
     * into a trusted document Go did not serve with it inside (else leaves
     * it null). Returns whether the page keeps its lane reply proxy.
     */
    static native boolean nativeBeginPage(long id, String url, String[] script);
    /** JavaBridge thread. The origin of the document token belongs to, or null if it is not current. */
    static native String nativeTokenOrigin(long id, String token);
    /** Delivers a message from origin (established by nativeTokenOrigin). */
    static native void nativeMessage(long id, String origin, String message);
    /**
     * A main-frame WebMessageListener message from sourceOrigin. Returns the
     * reply to post back, or null.
     */
    static native String nativeWebMessage(long id, String sourceOrigin, String data);
    /**
     * An upstream lane message (an ArrayBuffer) from the main frame: 0 drop
     * it, 1 delivered (a probe: keep the reply proxy, answer [0]), 2 refused
     * (answer [1]).
     */
    static native int nativeLanePacket(long id, String sourceOrigin, byte[] data);
    /** WebView request thread. Returns the response for an asset-origin request. */
    static native WebResourceResponse nativeServeAsset(Page page, long id, String method, String url,
            String[] headers, boolean mainFrame);
    /** Reads a streamed response body; -1 at the end. Request thread. */
    static native int nativeStreamRead(long handle, byte[] buf, int off, int len);
    static native void nativeStreamClose(long handle);
    /** JavaBridge thread. Stores a request body for the asset origin; false if refused. */
    static native boolean nativeStashBody(long id, String token, String bodyId, String contentType, String base64);
    /** Completes a dialog: button (-1 dismissed) or paths (null dismissed), or an error. */
    static native void nativeDialogDone(int cb, int button, String[] paths, String error);
}
