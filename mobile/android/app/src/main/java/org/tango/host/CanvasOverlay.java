package org.tango.host;

import android.view.Gravity;
import android.view.PointerIcon;
import android.view.ViewGroup;
import android.widget.FrameLayout;

/**
 * A canvas inside a WebView page (uihost.Attach; canvas_attach_android.go):
 * a translucent {@link CanvasView} in the page's frame, above or below the
 * WebView. Fixed bounds are placed by Go on every layout (Page.Frame asks
 * TangoCanvas.nativePlace), so they keep their anchor when the page is
 * resized. Go owns its lifetime (main thread).
 */
final class CanvasOverlay implements CanvasHost {
    final Page page;
    final long id;
    final CanvasView canvas;
    boolean above;
    boolean closed;

    CanvasOverlay(Page page, long id) {
        this.page = page;
        this.id = id;
        canvas = new CanvasView(page.context, id, true);
    }

    /**
     * Applies a CanvasLayer: stacking, pass-through and bounds in dp (zero
     * width or height fills the page). Fixed bounds get their size here;
     * their position comes from Go at layout.
     */
    void apply(boolean above, boolean passThrough, int x, int y, int w, int h) {
        this.above = above;
        canvas.passThrough = passThrough;
        canvas.fill = w <= 0 || h <= 0;
        FrameLayout.LayoutParams lp;
        if (canvas.fill) {
            lp = new FrameLayout.LayoutParams(ViewGroup.LayoutParams.MATCH_PARENT, ViewGroup.LayoutParams.MATCH_PARENT);
        } else {
            float d = canvas.getResources().getDisplayMetrics().density;
            lp = new FrameLayout.LayoutParams(Math.round(w * d), Math.round(h * d), Gravity.TOP | Gravity.LEFT);
            // Until Go places it (Page.Frame.onLayout).
            lp.leftMargin = Math.round(x * d);
            lp.topMargin = Math.round(y * d);
        }
        canvas.setLayoutParams(lp);
    }

    // ---- called from Go ----

    /** A new CanvasLayer (AttachedCanvas.SetLayer). */
    void setLayer(boolean above, boolean passThrough, int x, int y, int w, int h) {
        if (closed) return;
        boolean moved = above != this.above;
        apply(above, passThrough, x, y, w, h);
        if (moved) page.restackOverlay(this);
        page.overlaysChanged();
    }

    /** Removes the canvas from its page (AttachedCanvas.Detach); its Surface is released on the way. */
    void detach() {
        if (closed) return;
        destroy();
        page.removeOverlay(this);
        TangoActivity.backChanged();
    }

    /** Stops the canvas without touching the page (the page goes away). */
    void destroy() {
        if (closed) return;
        closed = true;
        canvas.release();
        TangoCanvas.forget(id);
    }

    @Override
    public void textInput(boolean on, int inputType, int imeOptions, float x, float y, float w, float h, boolean restart) {
        if (!closed) canvas.textInput(on, inputType, imeOptions, x, y, w, h, restart);
    }

    @Override
    public void mirror(String text, int base, int selStart, int selEnd, int compStart, int compEnd) {
        if (!closed) canvas.mirror(text, base, selStart, selEnd, compStart, compEnd);
    }

    @Override
    public void a11yEvent(int virtualId, int changes) {
        if (!closed) canvas.a11y.contentChanged(virtualId, changes);
    }

    @Override
    public void a11yFocus(int virtualId) {
        if (!closed) canvas.a11y.focusChanged(virtualId);
    }

    @Override
    public void a11yAnnounce(String text, int live) {
        if (!closed) canvas.a11y.announce(text, live);
    }

    @Override
    public void a11ySelfTest(int virtualId, int action) {
        if (!closed) canvas.a11y.selfTest(virtualId, action);
    }

    @Override
    public void setCursor(int type) {
        if (!closed) canvas.setPointerIcon(PointerIcon.getSystemIcon(page.context, type));
    }

    @Override
    public void setBackHandling(boolean on) {
        if (closed || canvas.backHandling == on) return;
        canvas.backHandling = on;
        TangoActivity.backChanged();
    }
}
