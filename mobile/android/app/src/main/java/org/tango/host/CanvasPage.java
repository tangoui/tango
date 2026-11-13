package org.tango.host;

import android.content.Context;
import android.graphics.Insets;
import android.os.Build;
import android.view.PointerIcon;
import android.view.View;
import android.view.ViewGroup;
import android.view.WindowInsets;

/**
 * A page drawn by Tango's own UI (uihost.Open): a full-screen
 * {@link CanvasView}. It stays full-bleed under the system bars and
 * reports them, with the display cut-out, as its safe area, and the soft
 * keyboard as its keyboard inset. Go owns its lifetime (main thread).
 */
final class CanvasPage extends PageBase implements CanvasHost {
    final CanvasView canvas;

    CanvasPage(Context base, long id, String title) {
        super(base, id, title);
        canvas = new CanvasView(context, id, false);
    }

    @Override
    View view() {
        return canvas;
    }

    @Override
    void reportEvent(int kind) {
        TangoCanvas.nativeSurfaceEvent(id, kind);
        if (kind == Tango.FOCUSED) canvas.requestFocus();
    }

    /** A covered page keeps its place but drops its Surface (INVISIBLE). */
    @Override
    int visibility() {
        if (!visible) return View.GONE;
        return covered ? View.INVISIBLE : View.VISIBLE;
    }

    @Override
    void coveredChanged() {
        if (!closed) canvas.setVisibility(visibility());
    }

    @Override
    void applyInsets(WindowInsets insets) {
        if (insets == null || Build.VERSION.SDK_INT < 30) return;
        Insets safe = insets.getInsets(WindowInsets.Type.systemBars() | WindowInsets.Type.displayCutout());
        int ime = insets.isVisible(WindowInsets.Type.ime()) ? insets.getInsets(WindowInsets.Type.ime()).bottom : 0;
        canvas.setInsets(safe.top, safe.right, safe.bottom, safe.left, ime);
    }

    // ---- called from Go ----

    @Override
    void close() {
        if (closed) return;
        destroy();
        TangoActivity.removePage(this);
        TangoCanvas.forget(id);
        TangoCanvas.nativeSurfaceEvent(id, Tango.CLOSED);
    }

    /**
     * Text input on or off, with the EditorInfo inputType and imeOptions
     * and the caret in device pixels (see CanvasView.textInput).
     */
    @Override
    public void textInput(boolean on, int inputType, int imeOptions, float x, float y, float w, float h, boolean restart) {
        if (!closed) canvas.textInput(on, inputType, imeOptions, x, y, w, h, restart);
    }

    /** The input method's document (see CanvasView.mirror). */
    @Override
    public void mirror(String text, int base, int selStart, int selEnd, int compStart, int compEnd) {
        if (!closed) canvas.mirror(text, base, selStart, selEnd, compStart, compEnd);
    }

    /** A content change of a virtual view (CanvasA11y.C_* bits; 0 undefined). */
    @Override
    public void a11yEvent(int virtualId, int changes) {
        if (!closed) canvas.a11y.contentChanged(virtualId, changes);
    }

    /** The virtual view with input focus (CanvasA11y.NO_ID: none). */
    @Override
    public void a11yFocus(int virtualId) {
        if (!closed) canvas.a11y.focusChanged(virtualId);
    }

    /** Text to speak once that no live region carries (ui.Context.Announce). */
    @Override
    public void a11yAnnounce(String text, int live) {
        if (!closed) canvas.a11y.announce(text, live);
    }

    /** Debug self-test: performs an action on a virtual view through the provider. */
    @Override
    public void a11ySelfTest(int virtualId, int action) {
        if (!closed) canvas.a11y.selfTest(virtualId, action);
    }

    /** Sets the mouse pointer shape (a PointerIcon type). */
    @Override
    public void setCursor(int type) {
        if (closed) return;
        canvas.setPointerIcon(PointerIcon.getSystemIcon(context, type));
    }

    @Override
    public void setBackHandling(boolean on) {
        if (closed || canvas.backHandling == on) return;
        canvas.backHandling = on;
        TangoActivity.backChanged();
    }

    @Override
    CanvasView backCanvas() {
        return !closed && canvas.backHandling ? canvas : null;
    }

    // ---- internals ----

    /** Detaches the view without reporting; its Surface is released on the way. */
    @Override
    void destroy() {
        if (closed) return;
        closed = true;
        canvas.release();
        ViewGroup parent = (ViewGroup) canvas.getParent();
        if (parent != null) parent.removeView(canvas);
    }
}
