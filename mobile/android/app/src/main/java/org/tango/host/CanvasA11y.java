package org.tango.host;

import android.content.Context;
import android.graphics.Rect;
import android.os.Build;
import android.os.Bundle;
import android.os.Handler;
import android.os.Looper;
import android.os.SystemClock;
import android.util.Log;
import android.view.MotionEvent;
import android.view.View;
import android.view.ViewParent;
import android.view.accessibility.AccessibilityEvent;
import android.view.accessibility.AccessibilityManager;
import android.view.accessibility.AccessibilityNodeInfo;
import android.view.accessibility.AccessibilityNodeInfo.AccessibilityAction;
import android.view.accessibility.AccessibilityNodeProvider;

import java.nio.ByteBuffer;
import java.nio.ByteOrder;
import java.nio.charset.StandardCharsets;
import java.util.LinkedHashMap;
import java.util.Map;

/**
 * The accessibility node provider of a {@link CanvasView}
 * (canvas_a11y_android.go). The view stands for the root of Tango's
 * semantics tree; every other node is a virtual view whose id Go assigns.
 * Each query asks Go synchronously (TangoCanvas.nativeA11yNode) for a
 * packed record of the node, built from the last tree Go received, and
 * fills the AccessibilityNodeInfo from it. Actions go back to Go
 * (nativeA11yAction); accessibility focus and hover are kept here.
 *
 * <p>Go's notifications (content changes, input focus, announcements)
 * arrive during a frame; they are queued and sent as AccessibilityEvents
 * once the frame is over.
 *
 * <p>Activation: Go builds the tree only while an assistive technology
 * reads the canvas: while AccessibilityManager is enabled, and for 30
 * seconds after any query of the provider (uiautomator and other
 * automation read it without enabling a service for long).
 */
final class CanvasA11y extends AccessibilityNodeProvider {
    // AndroidState bits (a11ybridge/android.go).
    static final int S_CHECKABLE = 1, S_CHECKED = 1 << 1, S_MIXED = 1 << 2, S_ENABLED = 1 << 3,
            S_FOCUSABLE = 1 << 4, S_FOCUSED = 1 << 5, S_SELECTED = 1 << 6, S_CLICKABLE = 1 << 7,
            S_SCROLLABLE = 1 << 8, S_EDITABLE = 1 << 9, S_PASSWORD = 1 << 10, S_MULTILINE = 1 << 11,
            S_HEADING = 1 << 12, S_VISIBLE = 1 << 13, S_INVALID = 1 << 14, S_DISMISSABLE = 1 << 15,
            S_EXPANDED = 1 << 16, S_COLLAPSED = 1 << 17, S_REQUIRED = 1 << 18;
    // AndroidAction bits.
    static final int A_CLICK = 1, A_FOCUS = 1 << 1, A_SCROLL_FORWARD = 1 << 2, A_SCROLL_BACKWARD = 1 << 3,
            A_EXPAND = 1 << 4, A_COLLAPSE = 1 << 5, A_SET_TEXT = 1 << 6, A_SET_PROGRESS = 1 << 7,
            A_SHOW_ON_SCREEN = 1 << 8, A_DISMISS = 1 << 9;
    // AndroidChange bits.
    static final int C_SUBTREE = 1, C_TEXT = 1 << 1, C_CONTENT_DESCRIPTION = 1 << 2, C_STATE = 1 << 3,
            C_PANE_TITLE = 1 << 4;
    /** a11ybridge.AndroidNoID: no virtual view. */
    static final int NO_ID = Integer.MIN_VALUE;
    private static final int RECORD_VERSION = 1;
    /** How long the tree stays built after the last query without an enabled service. */
    private static final long IDLE_MS = 30_000;
    /** Least time between "content moved" events of the whole view. */
    private static final long MOVED_MS = 100;

    private final CanvasView view;
    private final long id;
    private final AccessibilityManager am;
    private final Handler handler = new Handler(Looper.getMainLooper());
    private final AccessibilityManager.AccessibilityStateChangeListener stateListener = this::stateChanged;

    private boolean active, listening, released;
    private long lastQuery;
    private int a11yFocus = NO_ID, hovered = NO_ID, inputFocus = NO_ID;

    /** Content changes to send, by virtual view (insertion order). */
    private final LinkedHashMap<Integer, Integer> pending = new LinkedHashMap<>();
    private boolean focusPending, flushPosted, movedPending;
    private long lastMoved;
    private final Runnable flush = this::flush;
    private final Runnable idleCheck = this::idleCheck;

    CanvasA11y(CanvasView view, long id) {
        this.view = view;
        this.id = id;
        am = (AccessibilityManager) view.getContext().getSystemService(Context.ACCESSIBILITY_SERVICE);
    }

    // ---- activation ----

    /** The view was attached: follow the accessibility state. */
    void attach() {
        if (released || am == null || listening) return;
        listening = true;
        am.addAccessibilityStateChangeListener(stateListener);
        stateChanged(am.isEnabled());
    }

    void release() {
        released = true;
        if (listening && am != null) am.removeAccessibilityStateChangeListener(stateListener);
        listening = false;
        handler.removeCallbacksAndMessages(null);
        pending.clear();
    }

    private void stateChanged(boolean enabled) {
        if (released) return;
        if (enabled) {
            setActive(true, false);
        } else {
            handler.removeCallbacks(idleCheck);
            handler.postDelayed(idleCheck, IDLE_MS);
        }
    }

    private void idleCheck() {
        if (released || !active) return;
        boolean enabled = am != null && am.isEnabled();
        long idle = SystemClock.uptimeMillis() - lastQuery;
        if (enabled) return;
        if (idle < IDLE_MS) {
            handler.postDelayed(idleCheck, IDLE_MS - idle);
            return;
        }
        setActive(false, false);
    }

    /** Any query of the provider: an assistive technology or automation reads the canvas. */
    void queried() {
        if (released) return;
        lastQuery = SystemClock.uptimeMillis();
        if (!active) {
            setActive(true, false);
            if (am == null || !am.isEnabled()) {
                handler.removeCallbacks(idleCheck);
                handler.postDelayed(idleCheck, IDLE_MS);
            }
        }
    }

    /** Tells Go; prime draws a frame now when Go has no tree yet. */
    private void setActive(boolean on, boolean prime) {
        active = on;
        if (!on) {
            a11yFocus = hovered = NO_ID;
            pending.clear();
        }
        view.apply(TangoCanvas.nativeA11yActive(id, on, prime, SystemClock.uptimeMillis()));
    }

    private boolean serviceOn() {
        return am != null && am.isEnabled();
    }

    // ---- AccessibilityNodeProvider ----

    @Override
    public AccessibilityNodeInfo createAccessibilityNodeInfo(int vid) {
        if (released) return null;
        lastQuery = SystemClock.uptimeMillis();
        byte[] r = TangoCanvas.nativeA11yNode(id, vid);
        if (r == null && vid == HOST_VIEW_ID) {
            // The first query after activation: build the tree now.
            setActive(true, true);
            r = TangoCanvas.nativeA11yNode(id, vid);
        }
        if (vid == HOST_VIEW_ID) {
            @SuppressWarnings("deprecation")
            AccessibilityNodeInfo info = AccessibilityNodeInfo.obtain(view);
            view.onInitializeAccessibilityNodeInfo(info);
            if (r != null) fill(info, vid, r, true);
            return info;
        }
        if (r == null) return null;
        @SuppressWarnings("deprecation")
        AccessibilityNodeInfo info = AccessibilityNodeInfo.obtain(view, vid);
        fill(info, vid, r, false);
        return info;
    }

    private static String str(ByteBuffer b) {
        int n = b.getInt();
        String s = new String(b.array(), b.position(), n, StandardCharsets.UTF_8);
        b.position(b.position() + n);
        return s;
    }

    /** A node record of Go (a11ybridge.PackAndroidNode), decoded. */
    private static final class Rec {
        int parent, st, acts, live, flags;
        float x, y, w, h;
        int rangeType;
        float lo, hi, cur;
        int rows, cols, selMode;
        boolean hierarchical;
        int row, col;
        boolean itemHeading, itemSelected;
        int[] kids;
        String cls, text, desc, state, hint, error, pane, roleDesc;

        /** Decodes rec; null for a record of another version. */
        static Rec parse(byte[] rec) {
            ByteBuffer b = ByteBuffer.wrap(rec).order(ByteOrder.LITTLE_ENDIAN);
            if (b.get() != RECORD_VERSION) return null;
            Rec r = new Rec();
            b.getInt(); // its own virtual id
            r.parent = b.getInt();
            r.st = b.getInt();
            r.acts = b.getShort() & 0xffff;
            r.live = b.get();
            r.flags = b.get();
            r.x = b.getFloat();
            r.y = b.getFloat();
            r.w = b.getFloat();
            r.h = b.getFloat();
            if ((r.flags & 1) != 0) {
                r.rangeType = b.get();
                r.lo = b.getFloat();
                r.hi = b.getFloat();
                r.cur = b.getFloat();
            }
            if ((r.flags & 2) != 0) {
                r.rows = b.getInt();
                r.cols = b.getInt();
                r.hierarchical = b.get() != 0;
                r.selMode = b.get();
            }
            if ((r.flags & 4) != 0) {
                r.row = b.getInt();
                r.col = b.getInt();
                r.itemHeading = b.get() != 0;
                r.itemSelected = b.get() != 0;
            }
            int n = b.getInt();
            r.kids = new int[n];
            for (int i = 0; i < n; i++) r.kids[i] = b.getInt();
            r.cls = str(b);
            r.text = str(b);
            r.desc = str(b);
            r.state = str(b);
            r.hint = str(b);
            r.error = str(b);
            r.pane = str(b);
            r.roleDesc = str(b);
            return r;
        }

        @SuppressWarnings("deprecation")
        AccessibilityNodeInfo.CollectionInfo collection() {
            if ((flags & 2) == 0) return null;
            int mode = selMode == 2 ? AccessibilityNodeInfo.CollectionInfo.SELECTION_MODE_MULTIPLE
                    : selMode == 1 ? AccessibilityNodeInfo.CollectionInfo.SELECTION_MODE_SINGLE
                    : AccessibilityNodeInfo.CollectionInfo.SELECTION_MODE_NONE;
            return AccessibilityNodeInfo.CollectionInfo.obtain(rows, cols, hierarchical, mode);
        }
    }

    @SuppressWarnings("deprecation")
    private void fill(AccessibilityNodeInfo info, int vid, byte[] bytes, boolean host) {
        Rec r = Rec.parse(bytes);
        if (r == null) return;
        int st = r.st, acts = r.acts;
        // The root node is the view itself: it keeps the view's own
        // properties and gets the tree's top-level nodes as children.
        for (int k : r.kids) info.addChild(view, k);
        AccessibilityNodeInfo.CollectionInfo coll = r.collection();
        if (host) {
            if (coll != null) info.setCollectionInfo(coll);
            return;
        }

        info.setPackageName(view.getContext().getPackageName());
        info.setClassName(r.cls);
        if (r.parent == HOST_VIEW_ID || r.parent == NO_ID) {
            info.setParent(view);
        } else {
            info.setParent(view, r.parent);
        }
        int[] loc = new int[2];
        view.getLocationOnScreen(loc);
        float d = view.getResources().getDisplayMetrics().density;
        // Bounds are already where the node is drawn (mirrored for RTL by
        // the layout): only scale and offset them.
        Rect sr = new Rect(loc[0] + Math.round(r.x * d), loc[1] + Math.round(r.y * d),
                loc[0] + Math.round((r.x + r.w) * d), loc[1] + Math.round((r.y + r.h) * d));
        info.setBoundsInScreen(sr);
        Rect pr = new Rect(sr);
        pr.offset(-loc[0], -loc[1]);
        info.setBoundsInParent(pr);
        info.setVisibleToUser((st & S_VISIBLE) != 0 && view.isShown());

        if (!r.text.isEmpty()) info.setText(r.text);
        if (!r.desc.isEmpty()) info.setContentDescription(r.desc);
        if (!r.state.isEmpty() && Build.VERSION.SDK_INT >= 30) info.setStateDescription(r.state);
        if (!r.hint.isEmpty() && Build.VERSION.SDK_INT >= 26) info.setHintText(r.hint);
        if (!r.error.isEmpty()) info.setError(r.error);
        if (!r.pane.isEmpty() && Build.VERSION.SDK_INT >= 28) info.setPaneTitle(r.pane);
        if (!r.roleDesc.isEmpty()) {
            info.getExtras().putCharSequence("AccessibilityNodeInfo.roleDescription", r.roleDesc);
        }

        info.setImportantForAccessibility(true);
        info.setEnabled((st & S_ENABLED) != 0);
        info.setCheckable((st & S_CHECKABLE) != 0);
        if (Build.VERSION.SDK_INT >= 36) {
            info.setChecked((st & S_MIXED) != 0 ? AccessibilityNodeInfo.CHECKED_STATE_PARTIAL
                    : (st & S_CHECKED) != 0 ? AccessibilityNodeInfo.CHECKED_STATE_TRUE
                    : AccessibilityNodeInfo.CHECKED_STATE_FALSE);
            if ((st & (S_EXPANDED | S_COLLAPSED)) != 0) {
                info.setExpandedState((st & S_EXPANDED) != 0 ? AccessibilityNodeInfo.EXPANDED_STATE_FULL
                        : AccessibilityNodeInfo.EXPANDED_STATE_COLLAPSED);
            }
            info.setFieldRequired((st & S_REQUIRED) != 0);
        } else {
            info.setChecked((st & S_CHECKED) != 0);
        }
        info.setFocusable((st & S_FOCUSABLE) != 0);
        info.setFocused((st & S_FOCUSED) != 0 && view.hasFocus());
        info.setSelected((st & S_SELECTED) != 0);
        info.setClickable((st & S_CLICKABLE) != 0);
        info.setScrollable((st & S_SCROLLABLE) != 0);
        info.setEditable((st & S_EDITABLE) != 0);
        info.setPassword((st & S_PASSWORD) != 0);
        info.setMultiLine((st & S_MULTILINE) != 0);
        info.setContentInvalid((st & S_INVALID) != 0);
        info.setDismissable((st & S_DISMISSABLE) != 0);
        if (Build.VERSION.SDK_INT >= 28) info.setHeading((st & S_HEADING) != 0);
        info.setLiveRegion(r.live == 2 ? View.ACCESSIBILITY_LIVE_REGION_ASSERTIVE
                : r.live == 1 ? View.ACCESSIBILITY_LIVE_REGION_POLITE : View.ACCESSIBILITY_LIVE_REGION_NONE);
        if ((r.flags & 1) != 0) {
            int type = r.rangeType == 0 ? AccessibilityNodeInfo.RangeInfo.RANGE_TYPE_INT
                    : AccessibilityNodeInfo.RangeInfo.RANGE_TYPE_FLOAT;
            info.setRangeInfo(AccessibilityNodeInfo.RangeInfo.obtain(type, r.lo, r.hi, r.cur));
        }
        if (coll != null) info.setCollectionInfo(coll);
        if ((r.flags & 4) != 0) {
            info.setCollectionItemInfo(AccessibilityNodeInfo.CollectionItemInfo.obtain(r.row, 1, r.col, 1,
                    r.itemHeading, r.itemSelected));
        }

        boolean focused = vid == a11yFocus;
        info.setAccessibilityFocused(focused);
        info.addAction(focused ? AccessibilityAction.ACTION_CLEAR_ACCESSIBILITY_FOCUS
                : AccessibilityAction.ACTION_ACCESSIBILITY_FOCUS);
        if ((acts & A_CLICK) != 0) info.addAction(AccessibilityAction.ACTION_CLICK);
        if ((acts & A_FOCUS) != 0) {
            info.addAction(info.isFocused() ? AccessibilityAction.ACTION_CLEAR_FOCUS : AccessibilityAction.ACTION_FOCUS);
        }
        if ((acts & A_SCROLL_FORWARD) != 0) info.addAction(AccessibilityAction.ACTION_SCROLL_FORWARD);
        if ((acts & A_SCROLL_BACKWARD) != 0) info.addAction(AccessibilityAction.ACTION_SCROLL_BACKWARD);
        if ((acts & A_EXPAND) != 0) info.addAction(AccessibilityAction.ACTION_EXPAND);
        if ((acts & A_COLLAPSE) != 0) info.addAction(AccessibilityAction.ACTION_COLLAPSE);
        if ((acts & A_SET_TEXT) != 0) info.addAction(AccessibilityAction.ACTION_SET_TEXT);
        if ((acts & A_SET_PROGRESS) != 0) info.addAction(AccessibilityAction.ACTION_SET_PROGRESS);
        if ((acts & A_SHOW_ON_SCREEN) != 0) info.addAction(AccessibilityAction.ACTION_SHOW_ON_SCREEN);
        if ((acts & A_DISMISS) != 0) info.addAction(AccessibilityAction.ACTION_DISMISS);
    }

    @Override
    public boolean performAction(int vid, int action, Bundle args) {
        if (released) return false;
        lastQuery = SystemClock.uptimeMillis();
        if (vid == HOST_VIEW_ID) return view.performAccessibilityAction(action, args);
        switch (action) {
            case AccessibilityNodeInfo.ACTION_ACCESSIBILITY_FOCUS:
                return setA11yFocus(vid);
            case AccessibilityNodeInfo.ACTION_CLEAR_ACCESSIBILITY_FOCUS:
                if (a11yFocus != vid) return false;
                a11yFocus = NO_ID;
                view.invalidate();
                send(vid, AccessibilityEvent.TYPE_VIEW_ACCESSIBILITY_FOCUS_CLEARED, 0);
                return true;
            case AccessibilityNodeInfo.ACTION_CLICK:
                if (!go(vid, A_CLICK, null, 0)) return false;
                send(vid, AccessibilityEvent.TYPE_VIEW_CLICKED, 0);
                return true;
            case AccessibilityNodeInfo.ACTION_FOCUS:
                if (!view.hasFocus()) view.requestFocus();
                return go(vid, A_FOCUS, null, 0);
            case AccessibilityNodeInfo.ACTION_SCROLL_FORWARD:
                return go(vid, A_SCROLL_FORWARD, null, 0);
            case AccessibilityNodeInfo.ACTION_SCROLL_BACKWARD:
                return go(vid, A_SCROLL_BACKWARD, null, 0);
            case AccessibilityNodeInfo.ACTION_EXPAND:
                return go(vid, A_EXPAND, null, 0);
            case AccessibilityNodeInfo.ACTION_COLLAPSE:
                return go(vid, A_COLLAPSE, null, 0);
            case AccessibilityNodeInfo.ACTION_DISMISS:
                return go(vid, A_DISMISS, null, 0);
            case AccessibilityNodeInfo.ACTION_SET_TEXT: {
                CharSequence t = args == null ? null
                        : args.getCharSequence(AccessibilityNodeInfo.ACTION_ARGUMENT_SET_TEXT_CHARSEQUENCE);
                return go(vid, A_SET_TEXT, t == null ? "" : t.toString(), 0);
            }
            default:
                break;
        }
        if (action == AccessibilityAction.ACTION_SHOW_ON_SCREEN.getId()) {
            return go(vid, A_SHOW_ON_SCREEN, null, 0);
        }
        if (action == AccessibilityAction.ACTION_SET_PROGRESS.getId() && args != null
                && args.containsKey(AccessibilityNodeInfo.ACTION_ARGUMENT_PROGRESS_VALUE)) {
            return go(vid, A_SET_PROGRESS, null, args.getFloat(AccessibilityNodeInfo.ACTION_ARGUMENT_PROGRESS_VALUE));
        }
        return false;
    }

    private boolean go(int vid, int act, String text, float progress) {
        return TangoCanvas.nativeA11yAction(id, vid, act, text, progress);
    }

    private boolean setA11yFocus(int vid) {
        if (a11yFocus == vid) return false;
        if (TangoCanvas.nativeA11yNode(id, vid) == null) return false;
        int old = a11yFocus;
        a11yFocus = vid;
        if (old != NO_ID) send(old, AccessibilityEvent.TYPE_VIEW_ACCESSIBILITY_FOCUS_CLEARED, 0);
        view.invalidate();
        send(vid, AccessibilityEvent.TYPE_VIEW_ACCESSIBILITY_FOCUSED, 0);
        return true;
    }

    @Override
    public AccessibilityNodeInfo findFocus(int focus) {
        if (released) return null;
        if (focus == AccessibilityNodeInfo.FOCUS_ACCESSIBILITY) {
            return a11yFocus == NO_ID ? null : createAccessibilityNodeInfo(a11yFocus);
        }
        if (focus == AccessibilityNodeInfo.FOCUS_INPUT && view.hasFocus()) {
            int v = TangoCanvas.nativeA11yFocus(id);
            return v == NO_ID ? null : createAccessibilityNodeInfo(v);
        }
        return null;
    }

    // ---- touch exploration ----

    /** A hover event of touch exploration; false leaves it to the view. */
    boolean hover(MotionEvent e) {
        if (released || am == null || !am.isEnabled() || !am.isTouchExplorationEnabled()) return false;
        switch (e.getActionMasked()) {
            case MotionEvent.ACTION_HOVER_ENTER:
            case MotionEvent.ACTION_HOVER_MOVE: {
                int v = TangoCanvas.nativeA11yHit(id, e.getX(), e.getY());
                if (v == HOST_VIEW_ID) v = NO_ID;
                setHovered(v);
                return v != NO_ID;
            }
            case MotionEvent.ACTION_HOVER_EXIT:
                if (hovered == NO_ID) return false;
                setHovered(NO_ID);
                return true;
            default:
                return false;
        }
    }

    private void setHovered(int v) {
        if (v == hovered) return;
        int old = hovered;
        hovered = v;
        if (v != NO_ID) send(v, AccessibilityEvent.TYPE_VIEW_HOVER_ENTER, 0);
        if (old != NO_ID) send(old, AccessibilityEvent.TYPE_VIEW_HOVER_EXIT, 0);
    }

    // ---- notifications from Go (during a frame) ----

    /** A content change of virtual view vid (C_* bits; 0 undefined). */
    void contentChanged(int vid, int changes) {
        if (released || !active) return;
        if (vid == HOST_VIEW_ID && changes == 0) {
            movedPending = true;
        } else {
            Integer old = pending.get(vid);
            pending.put(vid, old == null ? changes : old | changes);
        }
        post();
    }

    /** The virtual view with input focus (NO_ID: none). */
    void focusChanged(int vid) {
        inputFocus = vid;
        if (released || !active || vid == NO_ID) return;
        focusPending = true;
        post();
    }

    @SuppressWarnings("deprecation")
    void announce(String text, int live) {
        if (released || !serviceOn()) return;
        // Not tied to a node (ui.Context.Announce): live regions cover the
        // others (canvas_a11y_android.go).
        handler.post(() -> {
            if (!released) view.announceForAccessibility(text);
        });
    }

    private void post() {
        if (flushPosted) return;
        flushPosted = true;
        handler.post(flush);
    }

    private void flush() {
        flushPosted = false;
        if (released) return;
        if (!serviceOn()) {
            pending.clear();
            focusPending = movedPending = false;
            return;
        }
        if (a11yFocus != NO_ID && TangoCanvas.nativeA11yNode(id, a11yFocus) == null) a11yFocus = NO_ID;
        if (hovered != NO_ID && TangoCanvas.nativeA11yNode(id, hovered) == null) hovered = NO_ID;
        for (Map.Entry<Integer, Integer> e : pending.entrySet()) {
            send(e.getKey(), AccessibilityEvent.TYPE_WINDOW_CONTENT_CHANGED, changeTypes(e.getValue()));
        }
        pending.clear();
        if (movedPending) {
            long now = SystemClock.uptimeMillis();
            if (now - lastMoved >= MOVED_MS) {
                movedPending = false;
                lastMoved = now;
                send(HOST_VIEW_ID, AccessibilityEvent.TYPE_WINDOW_CONTENT_CHANGED,
                        AccessibilityEvent.CONTENT_CHANGE_TYPE_UNDEFINED);
            } else if (!flushPosted) {
                flushPosted = true;
                handler.postDelayed(flush, MOVED_MS - (now - lastMoved));
            }
        }
        if (focusPending) {
            focusPending = false;
            if (inputFocus != NO_ID && view.hasFocus()) send(inputFocus, AccessibilityEvent.TYPE_VIEW_FOCUSED, 0);
        }
    }

    private static int changeTypes(int c) {
        int t = 0;
        if ((c & C_SUBTREE) != 0) t |= AccessibilityEvent.CONTENT_CHANGE_TYPE_SUBTREE;
        if ((c & C_TEXT) != 0) t |= AccessibilityEvent.CONTENT_CHANGE_TYPE_TEXT;
        if ((c & C_CONTENT_DESCRIPTION) != 0) t |= AccessibilityEvent.CONTENT_CHANGE_TYPE_CONTENT_DESCRIPTION;
        if ((c & C_STATE) != 0 && Build.VERSION.SDK_INT >= 30) t |= AccessibilityEvent.CONTENT_CHANGE_TYPE_STATE_DESCRIPTION;
        if ((c & C_PANE_TITLE) != 0 && Build.VERSION.SDK_INT >= 28) t |= AccessibilityEvent.CONTENT_CHANGE_TYPE_PANE_TITLE;
        return t;
    }

    /** Sends an event whose source is virtual view vid (or the view itself). */
    @SuppressWarnings("deprecation")
    private void send(int vid, int type, int changeTypes) {
        ViewParent p = view.getParent();
        if (p == null || !serviceOn()) return;
        AccessibilityEvent e = Build.VERSION.SDK_INT >= 30 ? new AccessibilityEvent(type) : AccessibilityEvent.obtain(type);
        e.setPackageName(view.getContext().getPackageName());
        if (vid == HOST_VIEW_ID) {
            view.onInitializeAccessibilityEvent(e);
            e.setEventType(type);
        } else {
            e.setSource(view, vid);
            byte[] b = TangoCanvas.nativeA11yNode(id, vid);
            Rec r = b == null ? null : Rec.parse(b);
            if (r == null) return; // gone already
            // What a View sends about itself (onInitializeAccessibilityEvent
            // and onPopulateAccessibilityEvent).
            e.setClassName(r.cls);
            e.setEnabled((r.st & S_ENABLED) != 0);
            e.setChecked((r.st & S_CHECKED) != 0);
            e.setPassword((r.st & S_PASSWORD) != 0);
            if (!r.desc.isEmpty()) e.setContentDescription(r.desc);
            if (!r.text.isEmpty() && (r.st & S_PASSWORD) == 0) e.getText().add(r.text);
        }
        if (type == AccessibilityEvent.TYPE_WINDOW_CONTENT_CHANGED) e.setContentChangeTypes(changeTypes);
        p.requestSendAccessibilityEvent(view, e);
    }

    // ---- debug self-test (debug.tango.a11y.click) ----

    /** Performs act (A_CLICK) on vid through the provider, as a screen reader would. */
    void selfTest(int vid, int act) {
        handler.postDelayed(() -> {
            if (released) return;
            AccessibilityNodeInfo info = createAccessibilityNodeInfo(vid);
            Log.i(Tango.TAG, "a11y self-test: node " + vid + ": " + info);
            if (info != null) {
                // Touch exploration's lookup: the node's center, in view pixels.
                Rect r = new Rect();
                info.getBoundsInScreen(r);
                int[] loc = new int[2];
                view.getLocationOnScreen(loc);
                int hit = TangoCanvas.nativeA11yHit(id, r.exactCenterX() - loc[0], r.exactCenterY() - loc[1]);
                Log.i(Tango.TAG, "a11y self-test: hit test at the center of " + r + " -> " + hit);
            }
            boolean ok = info != null && act == A_CLICK
                    && info.getActionList().contains(AccessibilityAction.ACTION_CLICK)
                    && performAction(vid, AccessibilityNodeInfo.ACTION_CLICK, null);
            Log.i(Tango.TAG, "a11y self-test: ACTION_CLICK on " + vid + " -> " + ok);
        }, 300);
    }
}
