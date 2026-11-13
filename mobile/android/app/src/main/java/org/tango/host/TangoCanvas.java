package org.tango.host;

import android.content.Context;
import android.view.Surface;

import java.util.HashMap;

/**
 * The JNI boundary of canvas pages (internal/platform/android,
 * canvas_android.go): pages whose content Tango's own UI draws. Native
 * methods are implemented in libtango.so; the static methods are called
 * from Go, on the main thread unless noted.
 *
 * <p>The frame scheduler lives in Go. Its answers come back as the long
 * result of nativeWake, nativeFrame, nativeTimer and the Surface calls,
 * which {@link CanvasView#apply} carries out: bits 0-1 the frame callback
 * (0 keep, 1 resume, 2 pause), bit 2 "set the deadline timer" and bits 4
 * and up its uptime in milliseconds (0 cancels it).
 */
public final class TangoCanvas {
    /** Live canvas pages by surface id. Main thread. */
    private static final HashMap<Long, CanvasPage> pages = new HashMap<>();

    private TangoCanvas() {}

    // ---- called from Go ----

    /**
     * Creates a canvas page on top of the stack. Without an Activity it is
     * built on the application context and shown when one comes.
     */
    static CanvasPage newPage(long id, boolean visible, String title) {
        Context base = Tango.activity != null ? Tango.activity : Mobile.app;
        if (base == null) return null;
        CanvasPage p = new CanvasPage(base, id, title);
        pages.put(id, p);
        TangoActivity.addPage(p, visible);
        return p;
    }

    /** Asks for a scheduler wake on the main thread. Any thread. */
    static void requestFrame(long id) {
        Tango.main.post(() -> {
            CanvasPage p = pages.get(id);
            if (p != null && !p.closed) p.canvas.wake();
        });
    }

    static void forget(long id) {
        pages.remove(id);
    }

    // ---- implemented in Go ----

    static native long nativeWake(long id, long uptimeMs);
    /** A vsync tick of the frame callback; draws a frame when one is due. */
    static native long nativeFrame(long id, long uptimeMs);
    static native long nativeTimer(long id, long uptimeMs);
    /** The Surface was created or resized: Go takes its native window and draws at once. */
    static native long nativeSurfaceChanged(long id, Surface surface, int width, int height, float density, long uptimeMs);
    /** The Surface is going away: Go releases its native window before returning. */
    static native long nativeSurfaceDestroyed(long id, long uptimeMs);
    /** Safe area (system bars, cut-out) and keyboard inset in pixels. */
    static native void nativeInsets(long id, int top, int right, int bottom, int left, int ime, float density);
    /** A touch-screen MotionEvent packed by CanvasView.pack. */
    static native void nativeTouch(long id, long eventTimeMs, float[] data);
    /** A mouse, hover or wheel event (CanvasView.PTR_*). */
    static native void nativePointer(long id, int kind, float x, float y, int button, int meta, float dx, float dy);
    /** A hardware key; false leaves it to the system. */
    static native boolean nativeKey(long id, boolean down, int keyCode, int baseChar, int unicodeChar, int meta, int repeat);
    static native void nativeFocus(long id, boolean focused);
    /** An edit of the input method (CanvasInput.OP_*); Go answers with CanvasPage.mirror before returning. */
    static native void nativeIME(long id, int op, String text, int a, int b);
    /** A page event (Tango.CLOSED, FOCUSED, BLURRED). */
    static native void nativeSurfaceEvent(long id, int kind);

    // Accessibility (CanvasA11y; canvas_a11y_android.go). Main thread.

    /**
     * An assistive technology started or stopped reading the canvas; with
     * prime Go draws a frame now if it has no tree yet. Returns a scheduler
     * answer.
     */
    static native long nativeA11yActive(long id, boolean on, boolean prime, long uptimeMs);
    /** The packed record of a node (a11ybridge.PackAndroidNode), or null. */
    static native byte[] nativeA11yNode(long id, int virtualId);
    /** The virtual view at a point in view pixels, or CanvasA11y.NO_ID. */
    static native int nativeA11yHit(long id, float x, float y);
    /** The virtual view with input focus, or CanvasA11y.NO_ID. */
    static native int nativeA11yFocus(long id);
    /** Performs an action (CanvasA11y.A_*); true when Go accepted it. */
    static native boolean nativeA11yAction(long id, int virtualId, int action, String text, float progress);
}
