package org.tango.host;

import android.content.Context;
import android.content.MutableContextWrapper;
import android.view.View;
import android.view.WindowInsets;

/**
 * One entry of the Activity's page stack: a WebView page ({@link Page}) or
 * a self-drawn canvas page ({@link CanvasPage}). Go owns its lifetime
 * (main thread).
 *
 * <p>Pages outlive Activity instances (see TangoActivity): their views are
 * built on {@link #context}, which points at the current Activity, or at
 * the application while there is none.
 */
abstract class PageBase {
    final long id;
    /** The views' context: the current TangoActivity, or the application between instances. */
    final MutableContextWrapper context;
    String title;
    boolean visible;
    boolean closed;
    /** A visible page above covers this one (TangoActivity.pagesChanged). */
    boolean covered;

    PageBase(Context base, long id, String title) {
        this.context = new MutableContextWrapper(base);
        this.id = id;
        this.title = title;
    }

    /** The view placed in the Activity's root layout. */
    abstract View view();

    /** Destroys the page's views without reporting. */
    abstract void destroy();

    /** Reports a page event (Tango.FOCUSED, Tango.BLURRED) to Go. */
    abstract void reportEvent(int kind);

    /** Back: go back inside the page if it can (WebView history). */
    boolean goBack() {
        return false;
    }

    /**
     * The window insets changed (API 30+; the root layout no longer pads
     * itself): keep the page's content clear of what it must avoid.
     */
    void applyInsets(WindowInsets insets) {}

    /** Whether the page hides what is below it (every page fills the window). */
    boolean opaque() {
        return true;
    }

    /** View visibility for the page's state: GONE when hidden. */
    int visibility() {
        return visible ? View.VISIBLE : View.GONE;
    }

    /** The page's covered state changed (see covered). */
    void coveredChanged() {}

    // ---- called from Go (both kinds) ----

    void show() {
        if (closed) return;
        visible = true;
        view().setVisibility(visibility());
        TangoActivity.pagesChanged();
    }

    void hide() {
        if (closed) return;
        visible = false;
        view().setVisibility(visibility());
        TangoActivity.pagesChanged();
    }

    void focus() {
        if (closed) return;
        TangoActivity.raise(this);
        view().requestFocus();
    }

    /** Destroys the page and reports CLOSED. */
    abstract void close();

    void setTitle(String t) {
        title = t;
        TangoActivity.pagesChanged();
    }
}
