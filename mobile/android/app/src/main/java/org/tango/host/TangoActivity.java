package org.tango.host;

import android.app.Activity;
import android.content.ComponentCallbacks2;
import android.content.Intent;
import android.content.res.Configuration;
import android.os.Build;
import android.os.Bundle;
import android.view.View;
import android.view.ViewGroup;
import android.view.WindowInsets;
import android.view.WindowInsetsController;
import android.widget.FrameLayout;
import android.window.OnBackInvokedCallback;
import android.window.OnBackInvokedDispatcher;

import java.util.ArrayList;

/**
 * The Activity hosting a Tango app. Its content is a stack of pages (a
 * WebView or a self-drawn canvas per Tango surface): the first fills the
 * screen, later ones are stacked over it, the last visible one is on top.
 *
 * The page stack belongs to the process, not to an Activity instance: Go's
 * state lives as long as the process, so when the system destroys the
 * Activity without the app finishing (a configuration change the manifest
 * does not handle, "Don't keep activities", reclaiming a stopped Activity)
 * the WebViews are detached, kept alive with their pages' state, and moved
 * into the next instance; Go sees Background/Foreground only. Terminating
 * is reported when the Activity really finishes (back on the last page,
 * App.Quit, finish()).
 *
 * Use it as is (see AndroidManifest.xml) or subclass it.
 */
public class TangoActivity extends Activity {
    /** The page stack, bottom first. Main thread. */
    private static final ArrayList<PageBase> pages = new ArrayList<>();
    private static PageBase top;
    /** The last window insets (API 30+), applied to pages as they come. */
    private static WindowInsets lastInsets;
    /** Height of the status bar area (px) under the pages, for Device's strip color. */
    private static int statusInset;

    private FrameLayout root;
    private OnBackInvokedCallback backCallback;
    /**
     * Registered above backCallback while the top page has a canvas that
     * takes Back (API 33+; see canvasBack).
     */
    private OnBackInvokedCallback canvasBackCallback;

    @Override
    protected void onCreate(Bundle state) {
        super.onCreate(state);
        root = new FrameLayout(this);
        setContentView(root);
        fitSystemBars();
        styleSystemBars();
        if (Build.VERSION.SDK_INT >= 33) {
            backCallback = this::back;
            getOnBackInvokedDispatcher().registerOnBackInvokedCallback(
                    OnBackInvokedDispatcher.PRIORITY_DEFAULT, backCallback);
        }
        Tango.activity = this;
        Mobile.attach(this);
        // Pages of the previous instance (the process survived it).
        for (PageBase p : pages) {
            View v = p.view();
            ViewGroup parent = (ViewGroup) v.getParent();
            if (parent != null) parent.removeView(v);
            p.context.setBaseContext(this);
            root.addView(v);
        }
        top = null;
        pagesChanged();
        Tango.start(this);
        Tango.nativeLifecycle(Tango.STARTED);
        // The deep link / notification that launched the app. An instance
        // the system recreated (it has a saved state) gets the same Intent
        // again: that was handled already.
        if (state == null) Mobile.handleIntent(getIntent());
        Mobile.activityCreated(this);
    }

    @Override
    protected void onNewIntent(Intent intent) {
        super.onNewIntent(intent);
        setIntent(intent);
        Mobile.handleIntent(intent);
    }

    @Override
    public void onRequestPermissionsResult(int request, String[] permissions, int[] results) {
        if (!Mobile.onRequestPermissionsResult(request, permissions, results)) {
            super.onRequestPermissionsResult(request, permissions, results);
        }
    }

    @Override
    protected void onResume() {
        super.onResume();
        styleSystemBars();
        Tango.nativeLifecycle(Tango.FOREGROUND);
    }

    @Override
    protected void onPause() {
        Tango.nativeLifecycle(Tango.BACKGROUND);
        super.onPause();
    }

    @Override
    protected void onDestroy() {
        boolean current = Tango.activity == this;
        if (current && isFinishing() && !isChangingConfigurations()) {
            // The app ends.
            Tango.nativeLifecycle(Tango.TERMINATING);
            for (PageBase p : new ArrayList<>(pages)) p.destroy();
            pages.clear();
            top = null;
        } else if (current) {
            // Destroyed by the system: a new instance takes the pages over.
            // Until then their WebViews live detached, on the application
            // context (no reference to this instance).
            for (PageBase p : pages) {
                if (p.view().getParent() == root) root.removeView(p.view());
                p.context.setBaseContext(getApplicationContext());
            }
        }
        if (Build.VERSION.SDK_INT >= 33 && backCallback != null) {
            getOnBackInvokedDispatcher().unregisterOnBackInvokedCallback(backCallback);
        }
        if (Build.VERSION.SDK_INT >= 33 && canvasBackCallback != null) {
            getOnBackInvokedDispatcher().unregisterOnBackInvokedCallback(canvasBackCallback);
            canvasBackCallback = null;
        }
        Mobile.activityDestroyed(this);
        if (current) Tango.activity = null;
        super.onDestroy();
    }

    @Override
    public void onConfigurationChanged(Configuration c) {
        super.onConfigurationChanged(c);
        styleSystemBars(); // bar icons follow night mode
        Mobile.configurationChanged(c);
    }

    @Override
    @SuppressWarnings("deprecation")
    public void onBackPressed() {
        back(); // before API 33
    }

    /**
     * Memory pressure: Go hears platform.AppMemoryWarning and drops caches
     * it can rebuild (GPU atlases, glyph caches).
     */
    @Override
    public void onTrimMemory(int level) {
        super.onTrimMemory(level);
        // Memory pressure only: TRIM_MEMORY_UI_HIDDEN (20) just means the UI
        // went to the background, which is not a low-memory condition.
        if (level >= ComponentCallbacks2.TRIM_MEMORY_RUNNING_LOW && level != ComponentCallbacks2.TRIM_MEMORY_UI_HIDDEN) {
            Mobile.event(Mobile.MEMORY_WARNING, null, null);
        }
    }

    @Override
    public void onLowMemory() {
        super.onLowMemory();
        Mobile.event(Mobile.MEMORY_WARNING, null, null);
    }

    @Override
    protected void onActivityResult(int request, int result, Intent data) {
        if (!Dialogs.onActivityResult(this, request, result, data)
                && !Mobile.onActivityResult(this, request, result, data)) {
            super.onActivityResult(request, result, data);
        }
    }

    /**
     * Back: go back in the top page's history (WebView pages); with none, ask
     * Go whether the page may close (Host.ShouldClose). Closing the last page
     * ends the app.
     */
    void back() {
        CanvasView c = canvasBackTarget();
        if (c != null) {
            c.deliverBack();
            return;
        }
        PageBase p = top;
        if (p == null) {
            finish();
            return;
        }
        if (p.goBack()) return;
        if (!Tango.nativeShouldClose(p.id)) return;
        p.close();
        if (pages.isEmpty()) finish();
    }

    /**
     * The canvas that takes Back now: the top-most one asking for it
     * (CanvasHost.setBackHandling) on the top page, or null.
     */
    private static CanvasView canvasBackTarget() {
        PageBase p = top;
        return p == null || p.closed ? null : p.backCanvas();
    }

    /**
     * A canvas started or stopped asking for Back, or the top page changed.
     * From API 33 a callback of overlay priority is registered while a
     * canvas takes Back, so the system routes the gesture to it (and shows
     * no back-to-home preview); before, back() checks the same target.
     */
    static void backChanged() {
        TangoActivity a = Tango.activity;
        if (a == null || Build.VERSION.SDK_INT < 33) return;
        boolean want = canvasBackTarget() != null;
        if (want == (a.canvasBackCallback != null)) return;
        if (want) {
            a.canvasBackCallback = a::back;
            a.getOnBackInvokedDispatcher().registerOnBackInvokedCallback(
                    OnBackInvokedDispatcher.PRIORITY_OVERLAY, a.canvasBackCallback);
        } else {
            a.getOnBackInvokedDispatcher().unregisterOnBackInvokedCallback(a.canvasBackCallback);
            a.canvasBackCallback = null;
        }
    }

    // ---- pages (main thread; work without an Activity too) ----

    /** The current instance's root, or null between instances. */
    private static FrameLayout currentRoot() {
        TangoActivity a = Tango.activity;
        return a == null ? null : a.root;
    }

    static void addPage(PageBase p, boolean visible) {
        p.visible = visible;
        p.view().setVisibility(p.visibility());
        if (lastInsets != null) p.applyInsets(lastInsets);
        pages.add(p);
        FrameLayout root = currentRoot();
        if (root != null) root.addView(p.view());
        pagesChanged();
    }

    static void removePage(PageBase p) {
        pages.remove(p);
        pagesChanged();
    }

    /** Moves p to the top of the stack. */
    static void raise(PageBase p) {
        if (!pages.remove(p)) return;
        pages.add(p);
        p.view().bringToFront();
        FrameLayout root = currentRoot();
        if (root != null) {
            root.requestLayout();
            root.invalidate();
        }
        pagesChanged();
    }

    /** The top visible page, or null. */
    static PageBase topPage() {
        return top;
    }

    /**
     * Recomputes the top page: reports focus changes and shows its title.
     * Canvas pages covered by an opaque visible page above hide their
     * SurfaceView (its Surface is released): SurfaceViews of one window have
     * no reliable order among themselves, and a covered page has nothing to
     * show anyway.
     */
    static void pagesChanged() {
        PageBase next = null;
        boolean coveredFromAbove = false;
        for (int i = pages.size() - 1; i >= 0; i--) {
            PageBase p = pages.get(i);
            if (p.closed) continue;
            if (p.covered != coveredFromAbove) {
                p.covered = coveredFromAbove;
                p.coveredChanged();
            }
            if (!p.visible) continue;
            if (next == null) next = p;
            if (p.opaque()) coveredFromAbove = true;
        }
        TangoActivity a = Tango.activity;
        if (a != null && next != null && next.title != null && !next.title.isEmpty()) a.setTitle(next.title);
        if (next != top) {
            PageBase prev = top;
            top = next;
            if (prev != null && !prev.closed) prev.reportEvent(Tango.BLURRED);
            if (next != null) next.reportEvent(Tango.FOCUSED);
        }
        backChanged();
    }

    /**
     * Keeps pages clear of the status and navigation bars and the keyboard.
     * Android 15+ draws apps edge to edge, where adjustResize no longer
     * shrinks the window for the keyboard. Each page applies the insets
     * itself (PageBase.applyInsets): WebView pages get margins (what the
     * root's padding used to be), canvas pages stay full-bleed and report
     * the insets to Go as their safe area and keyboard inset.
     */
    private void fitSystemBars() {
        root.setOnApplyWindowInsetsListener((v, insets) -> {
            if (Build.VERSION.SDK_INT >= 30) {
                lastInsets = insets;
                statusInset = insets.getInsets(WindowInsets.Type.systemBars()
                        | WindowInsets.Type.displayCutout() | WindowInsets.Type.ime()).top;
                for (PageBase p : pages) p.applyInsets(insets);
                v.invalidate(); // Device's status bar strip
                Device.insetsChanged(v, insets);
                return WindowInsets.CONSUMED;
            }
            return v.onApplyWindowInsets(insets);
        });
        if (Build.VERSION.SDK_INT < 30) root.setFitsSystemWindows(true);
    }

    /** The last window insets (API 30+), or null. */
    static WindowInsets insets() {
        return lastInsets;
    }

    /**
     * Height (px) of the area at the top that pages keep clear of: the root's
     * top padding before API 30, the status bar inset after.
     */
    static int statusInset(View root) {
        return Build.VERSION.SDK_INT >= 30 ? statusInset : root.getPaddingTop();
    }

    /**
     * Pages are drawn edge to edge under transparent system bars, so the bar
     * icons must contrast with the page: dark icons in light mode, light icons
     * in dark mode (following the system night mode).
     */
    @SuppressWarnings("deprecation")
    void styleSystemBars() {
        int night = getResources().getConfiguration().uiMode & Configuration.UI_MODE_NIGHT_MASK;
        boolean lightBars = night != Configuration.UI_MODE_NIGHT_YES;
        // sys/device may have set the status bar's style (Device.statusBar).
        boolean lightStatus = Device.lightStatusBar(lightBars);
        if (Build.VERSION.SDK_INT >= 30) {
            WindowInsetsController c = getWindow().getInsetsController();
            if (c == null) return;
            int status = WindowInsetsController.APPEARANCE_LIGHT_STATUS_BARS;
            int nav = WindowInsetsController.APPEARANCE_LIGHT_NAVIGATION_BARS;
            c.setSystemBarsAppearance((lightStatus ? status : 0) | (lightBars ? nav : 0), status | nav);
        } else if (Build.VERSION.SDK_INT >= 23) {
            View decor = getWindow().getDecorView();
            int flags = decor.getSystemUiVisibility() & ~View.SYSTEM_UI_FLAG_LIGHT_STATUS_BAR;
            if (lightStatus) flags |= View.SYSTEM_UI_FLAG_LIGHT_STATUS_BAR;
            if (Build.VERSION.SDK_INT >= 26) {
                flags &= ~View.SYSTEM_UI_FLAG_LIGHT_NAVIGATION_BAR;
                if (lightBars) flags |= View.SYSTEM_UI_FLAG_LIGHT_NAVIGATION_BAR;
            }
            decor.setSystemUiVisibility(flags);
        }
    }

    /** The root layout holding the pages (Device paints the status bar strip on it). */
    FrameLayout root() {
        return root;
    }
}
