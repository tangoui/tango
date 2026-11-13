package org.tango.host;

import android.app.Activity;
import android.app.job.JobInfo;
import android.app.job.JobParameters;
import android.app.job.JobScheduler;
import android.appwidget.AppWidgetManager;
import android.content.ActivityNotFoundException;
import android.content.ComponentName;
import android.content.Context;
import android.content.Intent;
import android.content.SharedPreferences;
import android.content.pm.PackageManager;
import android.graphics.Canvas;
import android.graphics.ColorFilter;
import android.graphics.Insets;
import android.graphics.Paint;
import android.graphics.PixelFormat;
import android.graphics.drawable.Drawable;
import android.net.Uri;
import android.os.Build;
import android.provider.Settings;
import android.util.Log;
import android.view.View;
import android.view.Window;
import android.view.WindowInsets;
import android.view.WindowInsetsController;
import android.view.WindowManager;
import android.view.inputmethod.InputMethodManager;
import android.widget.FrameLayout;
import android.widget.RemoteViews;

import org.json.JSONArray;
import org.json.JSONObject;

import java.util.ArrayList;
import java.util.HashMap;

/**
 * Round-4 capabilities for the Go backend (internal/platform/android
 * mobile2_android.go): device controls (store page, settings, keep screen
 * on, brightness, status bar, keyboard), background jobs (JobScheduler,
 * TangoJobService) and the home-screen widget (TangoWidget). Reached
 * through {@link Mobile#call}, same conventions: "!code message" errors.
 *
 * Window state (keep on, brightness, status bar) is kept here and applied
 * again to every new TangoActivity instance.
 */
final class Device {
    // platform.AppEventKind values.
    static final int KEYBOARD = 106, BACKGROUND_TASK = 107;

    // platform.StatusBarStyle values.
    private static final int SB_DEFAULT = 0, SB_LIGHT = 1, SB_DARK = 2;

    private static boolean keepOn;
    private static float brightness = WindowManager.LayoutParams.BRIGHTNESS_OVERRIDE_NONE;
    private static int barStyle = SB_DEFAULT;
    private static boolean barHidden;
    private static Integer barColor; // null: none (transparent)
    private static float keyboardDp = -1;

    private Device() {}

    /** Mobile.call's fallback: the operations below. Main thread. */
    static String call(int cb, String op, JSONObject a) throws Exception {
        switch (op) {
            case "review": return review();
            case "openSettings": return openSettings();
            case "keepScreenOn": keepOn = a.optBoolean("on"); apply(Tango.activity); return "";
            case "brightness": return brightness();
            case "setBrightness": return setBrightness(a.optDouble("value", -1));
            case "statusBar": return statusBar(a.optInt("style"), a.optBoolean("hidden"), a.optString("color", ""));
            case "keyboardHeight": return Float.toString(Math.max(keyboardDp, 0));
            case "hideKeyboard": return hideKeyboard();
            case "scheduleBackground": return schedule(a.optInt("kind"), a.optLong("earliestMs"),
                    a.optBoolean("network"), a.optBoolean("charging"));
            case "cancelBackground": return cancel(a.optInt("kind"));
            case "takeBackground": return takeBackground();
            case "finishBackground": finishBackground(a.optLong("id"), a.optBoolean("success")); return "";
            case "updateWidget": return updateWidget(a.optString("title", ""), a.optString("text", ""));
            default: return Sysx.call(cb, op, a);
        }
    }

    private static Context app() {
        return Mobile.app;
    }

    // ---- store page and settings ----

    /** Google Play's in-app review API needs the Play Core library: open the store page instead. */
    private static String review() {
        String pkg = app().getPackageName();
        Context from = Tango.activity != null ? Tango.activity : app();
        try {
            from.startActivity(newTask(from, new Intent(Intent.ACTION_VIEW, Uri.parse("market://details?id=" + pkg))));
        } catch (ActivityNotFoundException e) {
            try {
                from.startActivity(newTask(from, new Intent(Intent.ACTION_VIEW,
                        Uri.parse("https://play.google.com/store/apps/details?id=" + pkg))));
            } catch (ActivityNotFoundException e2) {
                return "!notAvailable no store or browser";
            }
        }
        return "";
    }

    private static String openSettings() {
        Context from = Tango.activity != null ? Tango.activity : app();
        Intent i = new Intent(Settings.ACTION_APPLICATION_DETAILS_SETTINGS,
                Uri.fromParts("package", app().getPackageName(), null));
        try {
            from.startActivity(newTask(from, i));
        } catch (ActivityNotFoundException e) {
            return "!notAvailable " + e.getMessage();
        }
        return "";
    }

    private static Intent newTask(Context from, Intent i) {
        if (!(from instanceof Activity)) i.addFlags(Intent.FLAG_ACTIVITY_NEW_TASK);
        return i;
    }

    // ---- window state: keep on, brightness, status bar ----

    /** Applies the window state to a (new) Activity instance. Main thread. */
    static void apply(TangoActivity a) {
        if (a == null) return;
        Window w = a.getWindow();
        if (keepOn || Sysx.keepScreenOn()) w.addFlags(WindowManager.LayoutParams.FLAG_KEEP_SCREEN_ON);
        else w.clearFlags(WindowManager.LayoutParams.FLAG_KEEP_SCREEN_ON);
        WindowManager.LayoutParams lp = w.getAttributes();
        if (lp.screenBrightness != brightness) {
            lp.screenBrightness = brightness;
            w.setAttributes(lp);
        }
        applyBars(a);
    }

    private static String brightness() {
        if (brightness >= 0) return Float.toString(brightness);
        try {
            int v = Settings.System.getInt(app().getContentResolver(), Settings.System.SCREEN_BRIGHTNESS);
            return Float.toString(Math.min(Math.max(v / 255f, 0f), 1f));
        } catch (Settings.SettingNotFoundException e) {
            return "!notAvailable " + e.getMessage();
        }
    }

    private static String setBrightness(double v) {
        brightness = v < 0 ? WindowManager.LayoutParams.BRIGHTNESS_OVERRIDE_NONE : (float) Math.min(Math.max(v, 0.0), 1.0);
        // 0 would turn the backlight off on some devices.
        if (brightness == 0f) brightness = 0.01f;
        apply(Tango.activity);
        return "";
    }

    private static String statusBar(int style, boolean hidden, String color) {
        if (!color.isEmpty()) {
            try {
                int c = android.graphics.Color.parseColor(color);
                barColor = android.graphics.Color.alpha(c) == 0 ? null : c;
            } catch (IllegalArgumentException e) {
                return "!error bad color " + color;
            }
        }
        barStyle = style;
        barHidden = hidden;
        applyBars(Tango.activity);
        return "";
    }

    /**
     * Whether the status bar should have dark content ("light bars"):
     * TangoActivity.styleSystemBars asks, so its night-mode default does
     * not override an explicit style.
     */
    static boolean lightStatusBar(boolean byNightMode) {
        switch (barStyle) {
            case SB_LIGHT: return false;
            case SB_DARK: return true;
            default:
                if (barColor != null) return luminance(barColor) > 0.5;
                return byNightMode;
        }
    }

    private static double luminance(int c) {
        return (0.2126 * android.graphics.Color.red(c) + 0.7152 * android.graphics.Color.green(c)
                + 0.0722 * android.graphics.Color.blue(c)) / 255;
    }

    @SuppressWarnings("deprecation")
    private static void applyBars(TangoActivity a) {
        if (a == null) return;
        a.styleSystemBars();
        if (Build.VERSION.SDK_INT >= 30) {
            WindowInsetsController c = a.getWindow().getInsetsController();
            if (c != null) {
                if (barHidden) {
                    c.setSystemBarsBehavior(WindowInsetsController.BEHAVIOR_SHOW_TRANSIENT_BARS_BY_SWIPE);
                    c.hide(WindowInsets.Type.statusBars());
                } else {
                    c.show(WindowInsets.Type.statusBars());
                }
            }
        } else {
            if (barHidden) a.getWindow().addFlags(WindowManager.LayoutParams.FLAG_FULLSCREEN);
            else a.getWindow().clearFlags(WindowManager.LayoutParams.FLAG_FULLSCREEN);
        }
        // Pages are drawn edge to edge and WebView pages keep clear of the
        // status bar (TangoActivity.fitSystemBars): the color fills that strip.
        FrameLayout root = a.root();
        if (root != null) root.setBackground(barColor == null ? null : new TopStrip(root, barColor));
    }

    /** Paints the area under the status bar, which WebView pages leave free. */
    private static final class TopStrip extends Drawable {
        private final View owner;
        private final Paint paint = new Paint();

        TopStrip(View owner, int color) {
            this.owner = owner;
            paint.setColor(color);
        }

        @Override
        public void draw(Canvas c) {
            int h = TangoActivity.statusInset(owner);
            if (h > 0) c.drawRect(0, 0, getBounds().width(), h, paint);
        }

        @Override public void setAlpha(int alpha) {}
        @Override public void setColorFilter(ColorFilter f) {}
        @Override public int getOpacity() { return PixelFormat.TRANSLUCENT; }
    }

    // ---- keyboard ----

    /** From TangoActivity's insets listener. Reports keyboard height changes (dp). */
    static void insetsChanged(View v, WindowInsets insets) {
        if (Build.VERSION.SDK_INT < 30) return;
        boolean visible = insets.isVisible(WindowInsets.Type.ime());
        Insets ime = insets.getInsets(WindowInsets.Type.ime());
        float dp = visible ? ime.bottom / v.getResources().getDisplayMetrics().density : 0;
        dp = Math.round(dp * 10) / 10f;
        if (dp == keyboardDp) return;
        boolean first = keyboardDp < 0;
        keyboardDp = dp;
        if (!first || dp > 0) Mobile.event(KEYBOARD, Float.toString(dp), null);
    }

    private static String hideKeyboard() {
        TangoActivity a = Tango.activity;
        if (a == null) return "";
        View focus = a.getCurrentFocus();
        if (Build.VERSION.SDK_INT >= 30) {
            WindowInsetsController c = a.getWindow().getInsetsController();
            if (c != null) c.hide(WindowInsets.Type.ime());
        }
        InputMethodManager imm = (InputMethodManager) a.getSystemService(Context.INPUT_METHOD_SERVICE);
        View v = focus != null ? focus : a.getWindow().getDecorView();
        imm.hideSoftInputFromWindow(v.getWindowToken(), 0);
        return "";
    }

    // ---- background jobs ----
    //
    // One JobScheduler job per platform.BackgroundKind; the job id is the
    // kind (1 refresh, 2 processing). TangoJobService reports started and
    // stopped jobs as updates; Go takes them (takeBackground) on the
    // AppBackgroundTask event and finishes them.

    /** Android gives a job about 10 minutes; report a little less. */
    private static final long JOB_BUDGET_MS = 9 * 60_000;

    private static final ArrayList<JSONObject> bgUpdates = new ArrayList<>();
    private static final HashMap<Long, JobParameters> running = new HashMap<>();
    private static final HashMap<Long, TangoJobService> runningService = new HashMap<>();
    private static long nextTask;

    private static String schedule(int kind, long earliestMs, boolean network, boolean charging) {
        if (kind != 1 && kind != 2) return "!unsupported background kind " + kind;
        Context c = app();
        JobInfo.Builder b = new JobInfo.Builder(kind, new ComponentName(c, TangoJobService.class))
                .setMinimumLatency(Math.max(earliestMs, 0))
                .setRequiredNetworkType(network ? JobInfo.NETWORK_TYPE_ANY : JobInfo.NETWORK_TYPE_NONE)
                .setRequiresCharging(charging);
        if (kind == 2) b.setRequiresBatteryNotLow(true);
        if (c.checkSelfPermission(android.Manifest.permission.RECEIVE_BOOT_COMPLETED) == PackageManager.PERMISSION_GRANTED) {
            b.setPersisted(true); // survives reboots
        }
        try {
            JobScheduler js = c.getSystemService(JobScheduler.class);
            if (js.schedule(b.build()) != JobScheduler.RESULT_SUCCESS) return "!error JobScheduler refused the job";
        } catch (IllegalArgumentException e) {
            return "!notConfigured " + e.getMessage(); // TangoJobService not in the manifest
        }
        return "";
    }

    private static String cancel(int kind) {
        app().getSystemService(JobScheduler.class).cancel(kind);
        return "";
    }

    /** TangoJobService.onStartJob. Main thread. */
    static void jobStarted(TangoJobService svc, JobParameters p) {
        long id = ++nextTask;
        running.put(id, p);
        runningService.put(id, svc);
        JSONObject u = new JSONObject();
        try {
            u.put("id", id).put("kind", p.getJobId())
                    .put("deadline", System.currentTimeMillis() + JOB_BUDGET_MS).put("expired", false);
        } catch (Exception ignored) {
        }
        bgUpdates.add(u);
        Log.i(Tango.TAG, "background job " + p.getJobId() + " started as task " + id);
        Mobile.event(BACKGROUND_TASK, null, null);
    }

    /** TangoJobService.onStopJob: the system wants the job to stop. Main thread. */
    static void jobStopped(JobParameters p) {
        for (java.util.Map.Entry<Long, JobParameters> e : running.entrySet()) {
            if (e.getValue() != p) continue;
            JSONObject u = new JSONObject();
            try {
                u.put("id", e.getKey()).put("kind", p.getJobId()).put("expired", true);
            } catch (Exception ignored) {
            }
            bgUpdates.add(u);
            running.remove(e.getKey());
            runningService.remove(e.getKey());
            Mobile.event(BACKGROUND_TASK, null, null);
            return;
        }
    }

    private static String takeBackground() {
        JSONArray a = new JSONArray(bgUpdates);
        bgUpdates.clear();
        return a.toString();
    }

    private static void finishBackground(long id, boolean success) {
        JobParameters p = running.remove(id);
        TangoJobService svc = runningService.remove(id);
        if (p != null && svc != null) {
            Log.i(Tango.TAG, "background task " + id + " finished, success " + success);
            svc.jobFinished(p, false); // one-shot: Go schedules the next one
        }
    }

    // ---- widget ----

    static final String WIDGET_PREFS = "tango.widget";

    private static String updateWidget(String title, String text) {
        Context c = app();
        ComponentName cn = new ComponentName(c, TangoWidget.class);
        try {
            c.getPackageManager().getReceiverInfo(cn, 0);
        } catch (PackageManager.NameNotFoundException e) {
            return "!notConfigured the app does not declare its widget (tan.json \"permissions\": [\"widget\"])";
        }
        c.getSharedPreferences(WIDGET_PREFS, Context.MODE_PRIVATE).edit()
                .putString("title", title).putString("text", text).apply();
        AppWidgetManager m = AppWidgetManager.getInstance(c);
        int[] ids = m.getAppWidgetIds(cn);
        if (ids.length > 0) m.updateAppWidget(ids, TangoWidget.views(c));
        return "";
    }
}
