package org.tango.host;

import android.content.Context;
import android.graphics.Matrix;
import android.graphics.PixelFormat;
import android.graphics.RectF;
import android.graphics.SurfaceTexture;
import android.os.Build;
import android.os.Handler;
import android.os.Looper;
import android.os.SystemClock;
import android.text.InputType;
import android.text.Selection;
import android.text.SpannableStringBuilder;
import android.text.TextUtils;
import android.view.Choreographer;
import android.view.InputDevice;
import android.view.KeyEvent;
import android.view.MotionEvent;
import android.view.Surface;
import android.view.SurfaceHolder;
import android.view.SurfaceView;
import android.view.TextureView;
import android.view.ViewConfiguration;
import android.view.View;
import android.view.ViewGroup;
import android.view.WindowInsets;
import android.view.accessibility.AccessibilityNodeProvider;
import android.view.WindowInsetsController;
import android.view.inputmethod.CursorAnchorInfo;
import android.view.inputmethod.EditorInfo;
import android.view.inputmethod.InputConnection;
import android.view.inputmethod.InputMethodManager;
import android.widget.FrameLayout;

/**
 * The view of a canvas: a SurfaceView filling the page of a
 * {@link CanvasPage}, or a translucent TextureView for a
 * {@link CanvasOverlay} inside a WebView page (any z-order and alpha). Go
 * draws into the native window of either (canvas_android.go,
 * canvas_attach_android.go); this view adds input forwarding and the frame
 * clock.
 *
 * <p>Overlays with pass-through (CanvasLayer.PassThrough) refuse a gesture
 * that starts on a pixel of the last frame below the hit threshold
 * ({@link #dispatchTouchEvent}), so the page layout gives the whole gesture
 * to the WebView under it.
 *
 * <p>Frames: Go's scheduler decides; this view carries out its answers
 * ({@link #apply}): a Choreographer frame callback that runs while frames
 * are wanted, and a deadline timer on the main Handler.
 *
 * <p>Input: touch-screen events go to Go whole (all pointers, historical
 * points included) as one float array; mouse, hover and wheel events one by
 * one; hardware keys with their key code, meta state and characters.
 *
 * <p>Text: while a Tango text field has focus ({@link #textInput}) the view
 * is a text editor whose input connection is a {@link CanvasInput}. Go
 * keeps the document; the view holds a mirror of it for the input method's
 * queries and reports selection changes and the caret to the input method.
 */
final class CanvasView extends FrameLayout
        implements SurfaceHolder.Callback, TextureView.SurfaceTextureListener, Choreographer.FrameCallback {
    // nativePointer kinds (canvas_input.go).
    static final int PTR_MOVE = 0, PTR_PRESS = 1, PTR_RELEASE = 2, PTR_LEAVE = 3, PTR_CANCEL = 4, PTR_SCROLL = 5;

    final long id;
    /** A canvas inside a WebView page (TextureView), not a page of its own (SurfaceView). */
    final boolean overlay;
    /** The SurfaceView of a page canvas, or null. */
    private final SurfaceView surface;
    /** The TextureView of an overlay, or null, and the Surface made on its SurfaceTexture. */
    private final TextureView texture;
    private Surface textureSurface;
    /** Overlay placement (CanvasOverlay): fills the page, or Go places it (Page.Frame). */
    boolean fill = true;
    /** Overlay: touches over pixels Go reports transparent pass to the views below. */
    boolean passThrough;
    /** Back arrives as Escape while this canvas is the top-most one asking (CanvasHost.setBackHandling). */
    boolean backHandling;
    private final Handler handler = new Handler(Looper.getMainLooper());
    private final Runnable timer = this::timerFired;

    /** Go wants the frame callback running. */
    private boolean running;
    /** A frame callback is posted. */
    private boolean posted;
    boolean released;
    private int mouseButton = MotionEvent.BUTTON_PRIMARY;
    private final int[] insets = {-1, -1, -1, -1, -1};
    private float[] packed = new float[64];

    // Text input (CanvasInput; canvas_ime_android.go).
    private boolean textOn;
    private int inputType = InputType.TYPE_CLASS_TEXT;
    private int imeOptions;
    /** Caret of the focused field, device pixels in this view. */
    private final RectF caret = new RectF();
    /** The document Go keeps, as last sent (CanvasPage.mirror). */
    final SpannableStringBuilder mirror = new SpannableStringBuilder();
    private int mirrorBase, selStart, selEnd, compStart = -1, compEnd = -1;
    /** Selection and composing span last reported to the input method. */
    private final int[] reported = {-1, -1, -1, -1};
    private CanvasInput input;
    private int batch;
    private boolean selectionPending;
    private int cursorMode;
    private boolean restartPosted, showWanted, tapPending;
    private float downX, downY;
    private final Runnable restart = this::restartNow;

    /** Accessibility: the provider of Tango's semantics tree. */
    final CanvasA11y a11y;

    CanvasView(Context context, long id, boolean overlay) {
        super(context);
        this.id = id;
        this.overlay = overlay;
        setLayoutParams(new FrameLayout.LayoutParams(
                ViewGroup.LayoutParams.MATCH_PARENT, ViewGroup.LayoutParams.MATCH_PARENT));
        setFocusable(true);
        setFocusableInTouchMode(true);
        View content;
        if (overlay) {
            // Translucent and in the view tree's order: Go fills premultiplied
            // RGBA_8888 buffers, which the TextureView blends over (or under)
            // the WebView.
            surface = null;
            texture = new TextureView(context);
            texture.setOpaque(false);
            texture.setSurfaceTextureListener(this);
            content = texture;
        } else {
            texture = null;
            surface = new SurfaceView(context);
            // Opaque: the compositor skips blending (Go asks for RGBX buffers too).
            surface.getHolder().setFormat(PixelFormat.RGBX_8888);
            surface.getHolder().addCallback(this);
            content = surface;
        }
        content.setLayoutParams(new FrameLayout.LayoutParams(
                ViewGroup.LayoutParams.MATCH_PARENT, ViewGroup.LayoutParams.MATCH_PARENT));
        // The tree's nodes are virtual views of this view (CanvasA11y);
        // the SurfaceView itself means nothing to a screen reader.
        content.setImportantForAccessibility(View.IMPORTANT_FOR_ACCESSIBILITY_NO);
        addView(content);
        a11y = new CanvasA11y(this, id);
    }

    private float density() {
        return getResources().getDisplayMetrics().density;
    }

    // ---- frame clock ----

    /** Go asked for a frame (TangoCanvas.requestFrame). */
    void wake() {
        if (!released) apply(TangoCanvas.nativeWake(id, SystemClock.uptimeMillis()));
    }

    @Override
    public void doFrame(long frameTimeNanos) {
        posted = false;
        if (released || !running) return;
        apply(TangoCanvas.nativeFrame(id, SystemClock.uptimeMillis()));
        if (tapPending) {
            // A tap was handled by this frame: if it left a text field
            // focused (tapped again after the keyboard was dismissed),
            // bring the keyboard back.
            tapPending = false;
            if (textOn && !showWanted && !imeVisible()) showIme();
        }
        if (running && !posted && !released) {
            posted = true;
            Choreographer.getInstance().postFrameCallback(this);
        }
    }

    private void timerFired() {
        if (!released) apply(TangoCanvas.nativeTimer(id, SystemClock.uptimeMillis()));
    }

    /** Carries out a scheduler answer (see TangoCanvas). */
    void apply(long a) {
        if (released) return;
        switch ((int) (a & 3)) {
            case 1:
                running = true;
                if (!posted) {
                    posted = true;
                    Choreographer.getInstance().postFrameCallback(this);
                }
                break;
            case 2:
                running = false;
                if (posted) {
                    posted = false;
                    Choreographer.getInstance().removeFrameCallback(this);
                }
                break;
            default:
                break;
        }
        if ((a & 4) != 0) {
            handler.removeCallbacks(timer);
            long at = a >>> 4;
            if (at > 0) handler.postAtTime(timer, at);
        }
    }

    /** Stops the clock for good (the page closed). */
    void release() {
        released = true;
        a11y.release();
        running = false;
        if (posted) Choreographer.getInstance().removeFrameCallback(this);
        posted = false;
        handler.removeCallbacks(timer);
        handler.removeCallbacks(restart);
        if (textOn) {
            textOn = false;
            hideIme();
        }
    }

    // ---- Surface ----

    @Override
    public void surfaceCreated(SurfaceHolder holder) {
        // surfaceChanged follows with the size.
    }

    @Override
    public void surfaceChanged(SurfaceHolder holder, int format, int width, int height) {
        if (released) return;
        apply(TangoCanvas.nativeSurfaceChanged(id, holder.getSurface(), width, height, density(),
                SystemClock.uptimeMillis()));
    }

    @Override
    public void surfaceDestroyed(SurfaceHolder holder) {
        // Also after release: Go must let go of the native window now.
        apply(TangoCanvas.nativeSurfaceDestroyed(id, SystemClock.uptimeMillis()));
    }

    // ---- TextureView (overlays) ----

    @Override
    public void onSurfaceTextureAvailable(SurfaceTexture st, int width, int height) {
        if (released) return;
        if (textureSurface == null) textureSurface = new Surface(st);
        apply(TangoCanvas.nativeSurfaceChanged(id, textureSurface, width, height, density(),
                SystemClock.uptimeMillis()));
    }

    @Override
    public void onSurfaceTextureSizeChanged(SurfaceTexture st, int width, int height) {
        if (released || textureSurface == null) return;
        apply(TangoCanvas.nativeSurfaceChanged(id, textureSurface, width, height, density(),
                SystemClock.uptimeMillis()));
    }

    @Override
    public boolean onSurfaceTextureDestroyed(SurfaceTexture st) {
        // Also after release: Go lets go of the native window first.
        apply(TangoCanvas.nativeSurfaceDestroyed(id, SystemClock.uptimeMillis()));
        if (textureSurface != null) {
            textureSurface.release();
            textureSurface = null;
        }
        return true; // the TextureView releases the SurfaceTexture
    }

    @Override
    public void onSurfaceTextureUpdated(SurfaceTexture st) {}

    // ---- Back (CanvasHost.setBackHandling) ----

    /**
     * Delivers the system Back action, down then up, as KEYCODE_BACK through
     * the key path; Go reports it as Escape marked with Code KeyBack while
     * back handling is on. The only path Back takes into the canvas: key()
     * leaves KEYCODE_BACK to the Activity, which calls this.
     */
    void deliverBack() {
        if (released) return;
        TangoCanvas.nativeKey(id, true, KeyEvent.KEYCODE_BACK, 0, 0, 0, 0);
        if (!released) TangoCanvas.nativeKey(id, false, KeyEvent.KEYCODE_BACK, 0, 0, 0, 0);
    }

    // ---- pass-through (overlays) ----

    /** Whether a pointer at (x, y), view pixels, belongs to this canvas rather than to the views below. */
    private boolean hits(float x, float y) {
        return !overlay || !passThrough || released || TangoCanvas.nativeHit(id, x, y);
    }

    /**
     * A gesture starting over a transparent pixel is refused whole: the
     * parent then offers ACTION_DOWN to the views below (the WebView), and
     * the rest of the gesture follows it there.
     */
    @Override
    public boolean dispatchTouchEvent(MotionEvent e) {
        if (e.getActionMasked() == MotionEvent.ACTION_DOWN && !hits(e.getX(), e.getY())) return false;
        return super.dispatchTouchEvent(e);
    }

    /**
     * Pointer streams without a press (mouse and trackpad hover, wheel and
     * two-finger scroll) are decided like touch gestures: once, where the
     * stream starts, and the rest follows that decision. A hover stream runs
     * from HOVER_ENTER (or the first move) to HOVER_EXIT; a scroll stream
     * is a run of SCROLL events less than SCROLL_STREAM_MS apart. Asking per
     * event let a scroll hop between the overlay and the page as it crossed
     * transparent pixels.
     */
    static final long SCROLL_STREAM_MS = 300;
    /** The current hover stream: 0 none, 1 this overlay's, -1 the page's. */
    private int hoverOwner;
    /** The current scroll stream's owner as hoverOwner, and its last event time. */
    private int scrollOwner;
    private long scrollAt;

    @Override
    public boolean dispatchGenericMotionEvent(MotionEvent e) {
        if (!e.isFromSource(InputDevice.SOURCE_CLASS_POINTER)) return super.dispatchGenericMotionEvent(e);
        if (!pointerHits(e)) return false;
        return super.dispatchGenericMotionEvent(e);
    }

    /** Whether a pointer event without a press belongs to this canvas (see SCROLL_STREAM_MS). */
    private boolean pointerHits(MotionEvent e) {
        switch (e.getActionMasked()) {
            case MotionEvent.ACTION_HOVER_ENTER:
                hoverOwner = hits(e.getX(), e.getY()) ? 1 : -1;
                return hoverOwner > 0;
            case MotionEvent.ACTION_HOVER_MOVE:
                if (hoverOwner == 0) hoverOwner = hits(e.getX(), e.getY()) ? 1 : -1;
                return hoverOwner > 0;
            case MotionEvent.ACTION_HOVER_EXIT: {
                boolean mine = hoverOwner > 0 || (hoverOwner == 0 && hits(e.getX(), e.getY()));
                hoverOwner = 0;
                return mine;
            }
            case MotionEvent.ACTION_SCROLL: {
                long t = e.getEventTime();
                if (scrollOwner == 0 || t - scrollAt > SCROLL_STREAM_MS) {
                    scrollOwner = hits(e.getX(), e.getY()) ? 1 : -1;
                }
                scrollAt = t;
                return scrollOwner > 0;
            }
            default:
                return hits(e.getX(), e.getY());
        }
    }

    /**
     * Hidden (GONE) or covered (INVISIBLE) pages hide the SurfaceView itself
     * too: a SurfaceView releases its Surface when its own visibility is not
     * VISIBLE, not reliably when only an ancestor's is.
     */
    @Override
    public void setVisibility(int v) {
        super.setVisibility(v);
        if (surface != null) surface.setVisibility(v == VISIBLE ? VISIBLE : INVISIBLE);
    }

    // ---- accessibility ----

    @Override
    protected void onAttachedToWindow() {
        super.onAttachedToWindow();
        if (!released) a11y.attach();
    }

    @Override
    public AccessibilityNodeProvider getAccessibilityNodeProvider() {
        if (released) return super.getAccessibilityNodeProvider();
        a11y.queried();
        return a11y;
    }

    /** Touch exploration (a screen reader's hover events) finds the virtual node under the finger. */
    @Override
    protected boolean dispatchHoverEvent(MotionEvent e) {
        if (!hits(e.getX(), e.getY())) return false;
        if (!released && a11y.hover(e)) return true;
        return super.dispatchHoverEvent(e);
    }

    // ---- insets ----

    void setInsets(int top, int right, int bottom, int left, int ime) {
        int[] v = {top, right, bottom, left, ime};
        if (java.util.Arrays.equals(v, insets)) return;
        System.arraycopy(v, 0, insets, 0, v.length);
        if (!released) TangoCanvas.nativeInsets(id, top, right, bottom, left, ime, density());
    }

    // ---- pointer input ----

    private static boolean isMouse(MotionEvent e) {
        return e.getPointerCount() > 0 && e.getToolType(0) == MotionEvent.TOOL_TYPE_MOUSE;
    }

    @Override
    public boolean onTouchEvent(MotionEvent e) {
        if (released) return false;
        if (e.getActionMasked() == MotionEvent.ACTION_DOWN && !hasFocus()) requestFocus();
        trackTap(e);
        if (isMouse(e)) {
            mouse(e);
            return true;
        }
        TangoCanvas.nativeTouch(id, e.getEventTime(), pack(e));
        return true;
    }

    /**
     * Packs a touch-screen event (canvas_input.go): action, action index,
     * pointer count n, history size h, n × (id, x, y, pressure), then
     * h × (time offset in ms, n × (x, y, pressure)). The array is reused and
     * may be longer than that; Go reads the counts.
     */
    private float[] pack(MotionEvent e) {
        int n = e.getPointerCount();
        int h = e.getActionMasked() == MotionEvent.ACTION_MOVE ? e.getHistorySize() : 0;
        int size = 4 + n * 4 + h * (1 + n * 3);
        float[] d = size <= packed.length ? packed : (packed = new float[Math.max(size, packed.length * 2)]);
        d[0] = e.getActionMasked();
        d[1] = e.getActionIndex();
        d[2] = n;
        d[3] = h;
        int o = 4;
        for (int i = 0; i < n; i++) {
            d[o++] = e.getPointerId(i);
            d[o++] = e.getX(i);
            d[o++] = e.getY(i);
            d[o++] = e.getPressure(i);
        }
        long t = e.getEventTime();
        for (int k = 0; k < h; k++) {
            d[o++] = e.getHistoricalEventTime(k) - t;
            for (int i = 0; i < n; i++) {
                d[o++] = e.getHistoricalX(i, k);
                d[o++] = e.getHistoricalY(i, k);
                d[o++] = e.getHistoricalPressure(i, k);
            }
        }
        return d;
    }

    /** Mouse buttons through the touch stream: the button held at the press. */
    private void mouse(MotionEvent e) {
        int kind;
        switch (e.getActionMasked()) {
            case MotionEvent.ACTION_DOWN:
                int b = e.getButtonState();
                mouseButton = (b & MotionEvent.BUTTON_SECONDARY) != 0 ? MotionEvent.BUTTON_SECONDARY
                        : (b & MotionEvent.BUTTON_TERTIARY) != 0 ? MotionEvent.BUTTON_TERTIARY
                        : MotionEvent.BUTTON_PRIMARY;
                kind = PTR_PRESS;
                break;
            case MotionEvent.ACTION_MOVE:
                kind = PTR_MOVE;
                break;
            case MotionEvent.ACTION_UP:
                kind = PTR_RELEASE;
                break;
            case MotionEvent.ACTION_CANCEL:
                kind = PTR_CANCEL;
                break;
            default:
                return;
        }
        TangoCanvas.nativePointer(id, kind, e.getX(), e.getY(), mouseButton, e.getMetaState(), 0, 0);
    }

    @Override
    public boolean onGenericMotionEvent(MotionEvent e) {
        if (released || !e.isFromSource(InputDevice.SOURCE_CLASS_POINTER)) return super.onGenericMotionEvent(e);
        switch (e.getActionMasked()) {
            case MotionEvent.ACTION_HOVER_ENTER:
            case MotionEvent.ACTION_HOVER_MOVE:
                TangoCanvas.nativePointer(id, PTR_MOVE, e.getX(), e.getY(), 0, e.getMetaState(), 0, 0);
                return true;
            case MotionEvent.ACTION_HOVER_EXIT:
                TangoCanvas.nativePointer(id, PTR_LEAVE, e.getX(), e.getY(), 0, e.getMetaState(), 0, 0);
                return true;
            case MotionEvent.ACTION_SCROLL: {
                // AXIS_VSCROLL > 0 is the wheel turned away (content moves
                // down); Tango's positive DY reveals content below.
                float v = e.getAxisValue(MotionEvent.AXIS_VSCROLL);
                float hz = e.getAxisValue(MotionEvent.AXIS_HSCROLL);
                ViewConfiguration vc = ViewConfiguration.get(getContext());
                float vf, hf;
                if (Build.VERSION.SDK_INT >= 26) {
                    vf = vc.getScaledVerticalScrollFactor();
                    hf = vc.getScaledHorizontalScrollFactor();
                } else {
                    vf = hf = 48 * density();
                }
                TangoCanvas.nativePointer(id, PTR_SCROLL, e.getX(), e.getY(), 0, e.getMetaState(), hz * hf, -v * vf);
                return true;
            }
            default:
                return super.onGenericMotionEvent(e);
        }
    }

    // ---- keys and focus ----

    @Override
    public boolean onKeyDown(int keyCode, KeyEvent e) {
        return key(true, keyCode, e) || super.onKeyDown(keyCode, e);
    }

    @Override
    public boolean onKeyUp(int keyCode, KeyEvent e) {
        return key(false, keyCode, e) || super.onKeyUp(keyCode, e);
    }

    private boolean key(boolean down, int keyCode, KeyEvent e) {
        if (released || keyCode == KeyEvent.KEYCODE_BACK) return false;
        return TangoCanvas.nativeKey(id, down, keyCode, e.getUnicodeChar(0), e.getUnicodeChar(), e.getMetaState(),
                e.getRepeatCount());
    }

    @Override
    protected void onFocusChanged(boolean gained, int direction, android.graphics.Rect previous) {
        super.onFocusChanged(gained, direction, previous);
        if (!released) TangoCanvas.nativeFocus(id, gained && hasWindowFocus());
    }

    @Override
    public void onWindowFocusChanged(boolean hasWindowFocus) {
        super.onWindowFocusChanged(hasWindowFocus);
        if (!released) TangoCanvas.nativeFocus(id, hasWindowFocus && hasFocus());
    }

    // ---- text input ----

    /**
     * Go's text input state (canvas_ime_android.go): on or off, the
     * EditorInfo inputType and imeOptions, the caret in device pixels, and
     * whether the input method must start over (another field, other hints,
     * text changed behind its back). Restarts are coalesced and run after
     * the current frame, so the mirror is up to date when the input method
     * asks for its initial state.
     */
    void textInput(boolean on, int type, int options, float x, float y, float w, float h, boolean restartIme) {
        if (released) return;
        boolean changed = on != textOn;
        boolean editor = type != inputType || options != imeOptions;
        boolean moved = caret.left != x || caret.top != y || caret.width() != w || caret.height() != h;
        textOn = on;
        inputType = type;
        imeOptions = options;
        caret.set(x, y, x + w, y + h);
        if (changed) {
            if (on) {
                if (!hasFocus()) requestFocus();
                showWanted = true;
            } else {
                showWanted = false;
                hideIme();
            }
            scheduleRestart();
        } else if (on && (editor || restartIme)) {
            scheduleRestart();
        }
        if (on && moved && (cursorMode & InputConnection.CURSOR_UPDATE_MONITOR) != 0) sendCursorAnchor();
    }

    /** Go's document (CanvasPage.mirror): text, base, UTF-16 selection and composing span. */
    void mirror(String text, int base, int ss, int se, int cs, int ce) {
        if (!TextUtils.equals(mirror, text)) mirror.replace(0, mirror.length(), text);
        int n = mirror.length();
        mirrorBase = base;
        selStart = clamp(ss, n);
        selEnd = clamp(se, n);
        compStart = cs < 0 ? -1 : clamp(cs, n);
        compEnd = cs < 0 ? -1 : clamp(ce, n);
        Selection.setSelection(mirror, selStart, selEnd);
        if (batch > 0) {
            selectionPending = true;
        } else {
            reportSelection();
        }
    }

    private static int clamp(int i, int n) {
        return Math.max(0, Math.min(i, n));
    }

    int mirrorSelStart() {
        return Math.min(selStart, mirror.length());
    }

    int mirrorSelEnd() {
        return Math.min(selEnd, mirror.length());
    }

    boolean liveInput(CanvasInput c) {
        return c == input && textOn && !released;
    }

    /** An edit of the input method, carried out by Go synchronously. */
    void ime(int op, String text, int a, int b) {
        if (!released) TangoCanvas.nativeIME(id, op, text, a, b);
    }

    void beginBatch() {
        batch++;
    }

    /** Ends one batch level; true while still inside a batch. */
    boolean endBatch() {
        if (batch > 0) batch--;
        if (batch == 0 && selectionPending) {
            selectionPending = false;
            reportSelection();
        }
        return batch > 0;
    }

    void endAllBatches() {
        batch = 0;
        selectionPending = false;
    }

    /** Tells the input method where the selection and composing span are now. */
    private void reportSelection() {
        if (!textOn || input == null) return;
        int a = mirrorBase + selStart, b = mirrorBase + selEnd;
        int c = compStart < 0 ? -1 : mirrorBase + compStart, d = compStart < 0 ? -1 : mirrorBase + compEnd;
        if (a == reported[0] && b == reported[1] && c == reported[2] && d == reported[3]) return;
        reported[0] = a;
        reported[1] = b;
        reported[2] = c;
        reported[3] = d;
        imm().updateSelection(this, a, b, c, d);
        if ((cursorMode & InputConnection.CURSOR_UPDATE_MONITOR) != 0) sendCursorAnchor();
    }

    void cursorUpdates(int mode) {
        cursorMode = mode;
        if ((mode & InputConnection.CURSOR_UPDATE_IMMEDIATE) != 0) sendCursorAnchor();
    }

    /** The caret and composing text for the input method's candidate window, in screen pixels. */
    private void sendCursorAnchor() {
        if (!textOn || input == null) return;
        int[] loc = new int[2];
        getLocationOnScreen(loc);
        Matrix m = new Matrix();
        m.setTranslate(loc[0], loc[1]);
        CursorAnchorInfo.Builder b = new CursorAnchorInfo.Builder();
        b.setMatrix(m);
        b.setSelectionRange(mirrorBase + selStart, mirrorBase + selEnd);
        if (compStart >= 0 && compEnd >= compStart) {
            b.setComposingText(mirrorBase + compStart, mirror.subSequence(compStart, compEnd));
        }
        b.setInsertionMarkerLocation(caret.left, caret.top, caret.bottom, caret.bottom,
                CursorAnchorInfo.FLAG_HAS_VISIBLE_REGION);
        imm().updateCursorAnchorInfo(this, b.build());
    }

    @Override
    public boolean onCheckIsTextEditor() {
        return textOn && !released;
    }

    @Override
    public InputConnection onCreateInputConnection(EditorInfo out) {
        if (!textOn || released) {
            input = null;
            return null;
        }
        out.inputType = inputType;
        out.imeOptions = imeOptions;
        out.initialSelStart = mirrorBase + selStart;
        out.initialSelEnd = mirrorBase + selEnd;
        if ((inputType & InputType.TYPE_MASK_CLASS) == InputType.TYPE_CLASS_TEXT) {
            out.initialCapsMode = TextUtils.getCapsMode(mirror, mirrorSelStart(), inputType);
        }
        if (Build.VERSION.SDK_INT >= 30) out.setInitialSurroundingSubText(mirror, mirrorBase);
        reported[0] = out.initialSelStart;
        reported[1] = out.initialSelEnd;
        reported[2] = compStart < 0 ? -1 : mirrorBase + compStart;
        reported[3] = compStart < 0 ? -1 : mirrorBase + compEnd;
        batch = 0;
        selectionPending = false;
        cursorMode = 0;
        input = new CanvasInput(this);
        return input;
    }

    private InputMethodManager imm() {
        return (InputMethodManager) getContext().getSystemService(Context.INPUT_METHOD_SERVICE);
    }

    private void scheduleRestart() {
        if (restartPosted) return;
        restartPosted = true;
        handler.post(restart);
    }

    private void restartNow() {
        restartPosted = false;
        if (released) return;
        if (!textOn) input = null;
        imm().restartInput(this);
        if (textOn && showWanted) {
            showWanted = false;
            showIme();
        }
    }

    private boolean imeVisible() {
        if (Build.VERSION.SDK_INT >= 30) {
            WindowInsets in = getRootWindowInsets();
            return in != null && in.isVisible(WindowInsets.Type.ime());
        }
        return false; // unknown; showing again is harmless
    }

    private void showIme() {
        if (!hasFocus()) requestFocus();
        if (Build.VERSION.SDK_INT >= 30) {
            WindowInsetsController c = getWindowInsetsController();
            if (c != null) {
                c.show(WindowInsets.Type.ime());
                return;
            }
        }
        imm().showSoftInput(this, 0);
    }

    private void hideIme() {
        if (Build.VERSION.SDK_INT >= 30) {
            WindowInsetsController c = getWindowInsetsController();
            if (c != null) {
                c.hide(WindowInsets.Type.ime());
                return;
            }
        }
        imm().hideSoftInputFromWindow(getWindowToken(), 0);
    }

    /** Remembers a tap (a touch that did not move past the touch slop) for doFrame. */
    private void trackTap(MotionEvent e) {
        switch (e.getActionMasked()) {
            case MotionEvent.ACTION_DOWN:
                downX = e.getX();
                downY = e.getY();
                break;
            case MotionEvent.ACTION_UP: {
                float slop = ViewConfiguration.get(getContext()).getScaledTouchSlop();
                float dx = e.getX() - downX, dy = e.getY() - downY;
                if (dx * dx + dy * dy <= slop * slop) tapPending = true;
                break;
            }
            default:
                break;
        }
    }
}
