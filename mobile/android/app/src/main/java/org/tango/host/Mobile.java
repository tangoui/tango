package org.tango.host;

import android.Manifest;
import android.app.Activity;
import android.app.NotificationManager;
import android.content.ClipData;
import android.content.ContentResolver;
import android.content.Context;
import android.content.Intent;
import android.content.SharedPreferences;
import android.content.pm.ActivityInfo;
import android.content.pm.PackageInfo;
import android.content.pm.PackageManager;
import android.content.res.Configuration;
import android.database.Cursor;
import android.graphics.Insets;
import android.hardware.biometrics.BiometricManager;
import android.hardware.biometrics.BiometricPrompt;
import android.location.Location;
import android.location.LocationListener;
import android.location.LocationManager;
import android.net.Uri;
import android.os.Build;
import android.os.Bundle;
import android.os.CancellationSignal;
import android.os.Looper;
import android.os.ext.SdkExtensions;
import android.provider.MediaStore;
import android.provider.OpenableColumns;
import android.security.keystore.KeyGenParameterSpec;
import android.security.keystore.KeyProperties;
import android.util.Base64;
import android.util.Log;
import android.view.DisplayCutout;
import android.view.HapticFeedbackConstants;
import android.view.View;
import android.view.ViewGroup;
import android.view.WindowInsets;
import android.webkit.MimeTypeMap;

import org.json.JSONArray;
import org.json.JSONObject;

import java.io.File;
import java.io.FileOutputStream;
import java.io.InputStream;
import java.io.OutputStream;
import java.nio.charset.StandardCharsets;
import java.security.KeyStore;
import java.util.ArrayDeque;
import java.util.ArrayList;
import java.util.Arrays;
import java.util.HashSet;
import java.util.List;
import java.util.Locale;

import javax.crypto.Cipher;
import javax.crypto.KeyGenerator;
import javax.crypto.SecretKey;
import javax.crypto.spec.GCMParameterSpec;

/**
 * Mobile system capabilities for the Go backend (mobile_android.go):
 * share sheet, haptics, safe area, orientation, biometrics, secure storage,
 * photo picker, camera, location, push, runtime permissions and app events.
 *
 * Go calls {@link #call}. Synchronous operations return their result;
 * failures are "!code message" with the codes the Go side maps to platform
 * errors (canceled, unsupported, notAvailable, notEnrolled, lockedOut,
 * authFailed, permissionDenied, notFound, notConfigured). Asynchronous
 * operations (cb &gt; 0) answer through {@link #nativeDone} on the main
 * thread.
 */
public final class Mobile {
    // platform.AppEventKind values.
    static final int OPEN_URL = 1, SAFE_AREA = 100, ORIENTATION = 101, PUSH_TOKEN = 102,
            PUSH_RECEIVED = 103, PUSH_OPENED = 104, WEBVIEW_RESTORED = 105;
    // platform.AppThemeChanged, AppReduceMotionChanged, AppContrastChanged.
    static final int THEME_CHANGED = 3, REDUCE_MOTION_CHANGED = 200, CONTRAST_CHANGED = 201;

    // platform.HapticKind values.
    private static final int H_LIGHT = 1, H_MEDIUM = 2, H_HEAVY = 3, H_SELECTION = 4,
            H_SUCCESS = 5, H_WARNING = 6, H_ERROR = 7;

    // platform.Orientation values and OrientationMask bits.
    private static final int O_UNKNOWN = 0, O_PORTRAIT = 1, O_UPSIDE_DOWN = 2, O_LANDSCAPE_LEFT = 3,
            O_LANDSCAPE_RIGHT = 4;
    private static final int M_PORTRAIT = 1, M_UPSIDE_DOWN = 2, M_LEFT = 4, M_RIGHT = 8;

    // platform.BiometryKind values.
    private static final int B_NONE = 0, B_FINGER = 1, B_FACE = 2, B_OTHER = 4;

    private static final int PICK_REQUEST = 0x7a70, CAPTURE_REQUEST = 0x7a71, PERMISSION_REQUEST = 0x7a72;

    static Context app;
    private static String lastSafeArea;
    private static int lastOrientation = -1;
    /** Night mode and language list last reported (configurationChanged). */
    private static int lastNight = -1;
    private static String lastLocales;
    private static boolean settingsObserved;

    private Mobile() {}

    // ---- wiring from TangoActivity ----

    /** Called from onCreate, before Tango.nativeInit. */
    static void attach(TangoActivity a) {
        app = a.getApplicationContext();
        a.getWindow().getDecorView().getViewTreeObserver().addOnGlobalLayoutListener(Mobile::layoutChanged);
        Configuration c = a.getResources().getConfiguration();
        lastNight = c.uiMode & Configuration.UI_MODE_NIGHT_MASK;
        lastLocales = locales(c);
        observeSettings();
    }

    // ---- system settings (system_android.go) ----

    /** From TangoActivity.onConfigurationChanged: night mode and language changes. */
    static void configurationChanged(Configuration c) {
        int night = c.uiMode & Configuration.UI_MODE_NIGHT_MASK;
        if (night != lastNight) {
            lastNight = night;
            event(THEME_CHANGED, null, null);
        }
        String l = locales(c);
        if (!l.equals(lastLocales)) {
            lastLocales = l;
            nativeLocalesChanged();
        }
    }

    /**
     * Watches the animator duration scale (Developer options, and
     * Accessibility "Remove animations", which sets it to 0) and, on
     * Android 14+, the contrast level. Once per process.
     */
    private static void observeSettings() {
        if (settingsObserved || app == null) return;
        settingsObserved = true;
        android.database.ContentObserver o = new android.database.ContentObserver(Tango.main) {
            @Override
            public void onChange(boolean selfChange) {
                event(REDUCE_MOTION_CHANGED, null, null);
            }
        };
        app.getContentResolver().registerContentObserver(
                android.provider.Settings.Global.getUriFor(android.provider.Settings.Global.ANIMATOR_DURATION_SCALE),
                false, o);
        if (Build.VERSION.SDK_INT >= 34) {
            android.app.UiModeManager um = (android.app.UiModeManager) app.getSystemService(Context.UI_MODE_SERVICE);
            if (um != null) um.addContrastChangeListener(app.getMainExecutor(), contrast -> event(CONTRAST_CHANGED, null, null));
        }
    }

    /** {"dark":…,"accent":"#RRGGBB" or ""} (the accent of the Android 12+ system palette). */
    private static String systemTheme() {
        Context c = app;
        if (c == null) return "{\"dark\":false,\"accent\":\"\"}";
        boolean dark = (c.getResources().getConfiguration().uiMode & Configuration.UI_MODE_NIGHT_MASK)
                == Configuration.UI_MODE_NIGHT_YES;
        String accent = "";
        if (Build.VERSION.SDK_INT >= 31) {
            int argb = c.getResources().getColor(android.R.color.system_accent1_500, null);
            accent = String.format(Locale.ROOT, "#%06X", argb & 0xFFFFFF);
        }
        return "{\"dark\":" + dark + ",\"accent\":\"" + accent + "\"}";
    }

    private static String reduceMotion() {
        if (app == null) return "0";
        float scale = android.provider.Settings.Global.getFloat(app.getContentResolver(),
                android.provider.Settings.Global.ANIMATOR_DURATION_SCALE, 1f);
        return scale == 0f ? "1" : "0";
    }

    /** Android 14+ contrast level: medium or high counts as high contrast. */
    private static String highContrast() {
        if (app == null || Build.VERSION.SDK_INT < 34) return "0";
        android.app.UiModeManager um = (android.app.UiModeManager) app.getSystemService(Context.UI_MODE_SERVICE);
        return um != null && um.getContrast() >= 0.5f ? "1" : "0";
    }

    /** The language list as a JSON array of BCP 47 tags. */
    private static String locales(Configuration c) {
        JSONArray a = new JSONArray();
        if (Build.VERSION.SDK_INT >= 24) {
            android.os.LocaleList l = c.getLocales();
            for (int i = 0; i < l.size(); i++) a.put(l.get(i).toLanguageTag());
        }
        return a.toString();
    }

    /** A TangoActivity instance was created (also a recreated one). */
    static void activityCreated(TangoActivity a) {
        Device.apply(a);
    }

    /** A TangoActivity instance is being destroyed. */
    static void activityDestroyed(TangoActivity a) {
        lastSafeArea = null;
        lastOrientation = -1;
    }

    /** Deep links / App Links and FCM notification taps. onCreate and onNewIntent. */
    static void handleIntent(Intent i) {
        if (i == null) return;
        Uri data = i.getData();
        if (Intent.ACTION_VIEW.equals(i.getAction()) && data != null) {
            event(OPEN_URL, data.toString(), null);
        }
        Bundle extras = i.getExtras();
        if (extras != null && extras.containsKey("google.message_id")) {
            event(PUSH_OPENED, TangoPush.fromExtras(extras), null);
            i.removeExtra("google.message_id"); // report a tap once
        }
    }

    private static void layoutChanged() {
        if (Tango.activity == null) return;
        String s = safeArea();
        if (!s.equals(lastSafeArea)) {
            boolean first = lastSafeArea == null;
            lastSafeArea = s;
            if (!first) event(SAFE_AREA, null, null);
        }
        int o = orientation();
        if (o != lastOrientation) {
            boolean first = lastOrientation < 0;
            lastOrientation = o;
            if (!first) event(ORIENTATION, null, null);
        }
    }

    /** Reports an app event to Go on the main thread. Any thread. */
    static void event(int kind, String url, String path) {
        if (Tango.isMainThread()) nativeEvent(kind, url, path);
        else Tango.main.post(() -> nativeEvent(kind, url, path));
    }

    private static void done(int cb, String result) {
        finish(cb, result, null);
    }

    private static void fail(int cb, String err) {
        finish(cb, null, err.startsWith("!") ? err : "!" + err);
    }

    private static void finish(int cb, String result, String err) {
        if (cb <= 0) return;
        if (Tango.isMainThread()) nativeDone(cb, result, err);
        else Tango.main.post(() -> nativeDone(cb, result, err));
    }

    /**
     * The front-end dev server URL tan dev -platform android passes as the
     * launch Intent's TANGO_DEV_URL extra. Only debuggable APKs (tan dev,
     * tan build -debug) honour it; release builds always get "".
     */
    private static String devURL() {
        Activity a = Tango.activity;
        if (a == null || (a.getApplicationInfo().flags & android.content.pm.ApplicationInfo.FLAG_DEBUGGABLE) == 0) {
            return "";
        }
        Intent i = a.getIntent();
        String u = i == null ? null : i.getStringExtra("TANGO_DEV_URL");
        return u == null ? "" : u;
    }

    // ---- called from Go ----

    static String call(int cb, String op, String json) {
        try {
            JSONObject a = new JSONObject(json == null || json.isEmpty() ? "{}" : json);
            switch (op) {
                case "share": share(cb, a); return "";
                case "haptic": return haptic(a.optInt("kind"));
                case "safeArea": return safeArea();
                case "systemTheme": return systemTheme();
                case "reduceMotion": return reduceMotion();
                case "highContrast": return highContrast();
                case "locales": return locales(app != null ? app.getResources().getConfiguration()
                        : android.content.res.Resources.getSystem().getConfiguration());
                case "orientation": return Integer.toString(orientation());
                case "lockOrientation": return lockOrientation(a.optInt("mask"));
                case "biometryKind": return biometryKind();
                case "authenticate": authenticate(cb, a); return "";
                case "secureSet": return secureSet(a.getString("key"), a.getString("value"));
                case "secureGet": return secureGet(a.getString("key"));
                case "secureDelete": return secureDelete(a.getString("key"));
                case "pickMedia": pickMedia(cb, a.optInt("types", 1), a.optInt("limit", 1)); return "";
                case "capturePhoto": capturePhoto(cb); return "";
                case "currentLocation": currentLocation(cb, a); return "";
                case "startLocation": startLocation(cb, a); return "";
                case "stopLocation": stopLocation(); return "";
                case "registerPush": registerPush(cb); return "";
                case "checkPermission": done(cb, checkPermission(a.getString("name"))); return "";
                case "devURL": return devURL();
                case "requestPermission": requestPermission(a.getString("name"), (s, e) -> {
                    if (e != null) fail(cb, e);
                    else done(cb, s);
                });
                    return "";
                default: return Device.call(cb, op, a);
            }
        } catch (Throwable t) {
            Log.e(Tango.TAG, "Mobile." + op, t);
            String msg = "!error " + t;
            fail(cb, msg);
            return cb > 0 ? "" : msg;
        }
    }

    private static Activity activity() {
        Activity a = Tango.activity;
        return a == null || a.isFinishing() ? null : a;
    }

    private static final String NO_ACTIVITY = "!notAvailable no Activity";

    // ---- share ----

    private static void share(int cb, JSONObject a) throws Exception {
        Activity act = activity();
        if (act == null) {
            fail(cb, NO_ACTIVITY);
            return;
        }
        ArrayList<Uri> uris = new ArrayList<>();
        String mime = null;
        JSONArray paths = a.optJSONArray("paths");
        if (paths != null) {
            for (int i = 0; i < paths.length(); i++) {
                File f = new File(paths.getString(i));
                uris.add(TangoFileProvider.uriFor(act, f));
                String m = TangoFileProvider.mimeOf(f.getName());
                if (mime == null) mime = m;
                else if (!mime.equals(m)) {
                    String major = mime.split("/")[0];
                    mime = major.equals(m.split("/")[0]) ? major + "/*" : "*/*";
                }
            }
        }
        String text = a.optString("text"), url = a.optString("url"), title = a.optString("title");
        String body = text.isEmpty() ? url : url.isEmpty() ? text : text + "\n" + url;
        Intent i;
        if (uris.size() > 1) {
            i = new Intent(Intent.ACTION_SEND_MULTIPLE);
            i.putParcelableArrayListExtra(Intent.EXTRA_STREAM, uris);
        } else {
            i = new Intent(Intent.ACTION_SEND);
            if (uris.size() == 1) i.putExtra(Intent.EXTRA_STREAM, uris.get(0));
        }
        i.setType(uris.isEmpty() ? "text/plain" : mime);
        if (!body.isEmpty()) i.putExtra(Intent.EXTRA_TEXT, body);
        if (!title.isEmpty()) {
            i.putExtra(Intent.EXTRA_SUBJECT, title);
            i.putExtra(Intent.EXTRA_TITLE, title);
        }
        if (!uris.isEmpty()) {
            ContentResolver cr = act.getContentResolver();
            ClipData clip = ClipData.newUri(cr, "file", uris.get(0));
            for (int k = 1; k < uris.size(); k++) clip.addItem(new ClipData.Item(uris.get(k)));
            i.setClipData(clip);
            i.addFlags(Intent.FLAG_GRANT_READ_URI_PERMISSION);
        }
        act.startActivity(Intent.createChooser(i, title.isEmpty() ? null : title));
        done(cb, ""); // Android does not report what the user chose
    }

    // ---- haptics ----

    private static String haptic(int kind) {
        Activity a = activity();
        if (a == null) return NO_ACTIVITY;
        int sdk = Build.VERSION.SDK_INT;
        int c;
        switch (kind) {
            case H_LIGHT: c = HapticFeedbackConstants.CLOCK_TICK; break;
            case H_MEDIUM: c = HapticFeedbackConstants.VIRTUAL_KEY; break;
            case H_HEAVY: c = HapticFeedbackConstants.LONG_PRESS; break;
            case H_SELECTION:
                c = sdk >= 27 ? HapticFeedbackConstants.TEXT_HANDLE_MOVE : HapticFeedbackConstants.CLOCK_TICK;
                break;
            case H_SUCCESS: c = sdk >= 30 ? HapticFeedbackConstants.CONFIRM : HapticFeedbackConstants.VIRTUAL_KEY; break;
            case H_WARNING: c = HapticFeedbackConstants.LONG_PRESS; break;
            case H_ERROR: c = sdk >= 30 ? HapticFeedbackConstants.REJECT : HapticFeedbackConstants.LONG_PRESS; break;
            default: return "!unsupported haptic " + kind;
        }
        a.getWindow().getDecorView().performHapticFeedback(c);
        return "";
    }

    // ---- safe area / orientation ----

    /** The topmost visible page's WebView, or the content view. */
    private static View topView(Activity a) {
        ViewGroup content = a.findViewById(android.R.id.content);
        if (content == null) return a.getWindow().getDecorView();
        if (content.getChildCount() == 0 || !(content.getChildAt(0) instanceof ViewGroup)) return content;
        ViewGroup root = (ViewGroup) content.getChildAt(0);
        for (int i = root.getChildCount() - 1; i >= 0; i--) {
            View v = root.getChildAt(i);
            if (v.getVisibility() == View.VISIBLE) return v;
        }
        return root;
    }

    /** Insets of the top page covered by system UI, in dp, as JSON. */
    static String safeArea() {
        double t = 0, b = 0, l = 0, r = 0;
        Activity a = Tango.activity;
        if (a != null) {
            View decor = a.getWindow().getDecorView();
            WindowInsets wi = decor.getRootWindowInsets();
            View v = topView(a);
            if (wi != null && v.isAttachedToWindow()) {
                int it, ib, il, ir;
                if (Build.VERSION.SDK_INT >= 30) {
                    Insets i = wi.getInsets(WindowInsets.Type.systemBars()
                            | WindowInsets.Type.displayCutout() | WindowInsets.Type.ime());
                    it = i.top; ib = i.bottom; il = i.left; ir = i.right;
                } else {
                    it = wi.getSystemWindowInsetTop();
                    ib = wi.getSystemWindowInsetBottom();
                    il = wi.getSystemWindowInsetLeft();
                    ir = wi.getSystemWindowInsetRight();
                    if (Build.VERSION.SDK_INT >= 28 && wi.getDisplayCutout() != null) {
                        DisplayCutout dc = wi.getDisplayCutout();
                        it = Math.max(it, dc.getSafeInsetTop());
                        ib = Math.max(ib, dc.getSafeInsetBottom());
                        il = Math.max(il, dc.getSafeInsetLeft());
                        ir = Math.max(ir, dc.getSafeInsetRight());
                    }
                }
                int[] loc = new int[2];
                v.getLocationInWindow(loc);
                int w = decor.getWidth(), h = decor.getHeight();
                float d = a.getResources().getDisplayMetrics().density;
                t = Math.max(0, it - loc[1]) / d;
                l = Math.max(0, il - loc[0]) / d;
                b = Math.max(0, loc[1] + v.getHeight() - (h - ib)) / d;
                r = Math.max(0, loc[0] + v.getWidth() - (w - ir)) / d;
            }
        }
        return String.format(Locale.ROOT, "{\"top\":%.2f,\"bottom\":%.2f,\"left\":%.2f,\"right\":%.2f}", t, b, l, r);
    }

    @SuppressWarnings("deprecation")
    static int orientation() {
        Activity a = Tango.activity;
        if (a == null) return O_UNKNOWN;
        int rot = Build.VERSION.SDK_INT >= 30 && a.getDisplay() != null ? a.getDisplay().getRotation()
                : a.getWindowManager().getDefaultDisplay().getRotation();
        boolean landscape = a.getResources().getConfiguration().orientation == Configuration.ORIENTATION_LANDSCAPE;
        boolean sideways = rot == 1 || rot == 3;
        if (landscape == sideways) { // naturally tall (phones)
            switch (rot) {
                case 0: return O_PORTRAIT;
                case 1: return O_LANDSCAPE_LEFT; // turned counter-clockwise
                case 2: return O_UPSIDE_DOWN;
                default: return O_LANDSCAPE_RIGHT;
            }
        }
        switch (rot) { // naturally wide (many tablets)
            case 0: return O_LANDSCAPE_LEFT;
            case 1: return O_UPSIDE_DOWN;
            case 2: return O_LANDSCAPE_RIGHT;
            default: return O_PORTRAIT;
        }
    }

    private static String lockOrientation(int m) {
        Activity a = activity();
        if (a == null) return NO_ACTIVITY;
        int o;
        boolean portrait = (m & (M_PORTRAIT | M_UPSIDE_DOWN)) != 0, land = (m & (M_LEFT | M_RIGHT)) != 0;
        if (m == 0) o = ActivityInfo.SCREEN_ORIENTATION_UNSPECIFIED;
        else if (portrait && land) o = (m & 15) == 15 ? ActivityInfo.SCREEN_ORIENTATION_FULL_SENSOR
                : ActivityInfo.SCREEN_ORIENTATION_UNSPECIFIED;
        else if (portrait) o = m == M_PORTRAIT ? ActivityInfo.SCREEN_ORIENTATION_PORTRAIT
                : m == M_UPSIDE_DOWN ? ActivityInfo.SCREEN_ORIENTATION_REVERSE_PORTRAIT
                : ActivityInfo.SCREEN_ORIENTATION_SENSOR_PORTRAIT;
        else o = m == M_LEFT ? ActivityInfo.SCREEN_ORIENTATION_LANDSCAPE
                : m == M_RIGHT ? ActivityInfo.SCREEN_ORIENTATION_REVERSE_LANDSCAPE
                : ActivityInfo.SCREEN_ORIENTATION_SENSOR_LANDSCAPE;
        a.setRequestedOrientation(o);
        return "";
    }

    // ---- biometrics ----

    private static int hardwareKind(Context c) {
        PackageManager pm = c.getPackageManager();
        boolean finger = pm.hasSystemFeature(PackageManager.FEATURE_FINGERPRINT);
        boolean face = Build.VERSION.SDK_INT >= 29 && pm.hasSystemFeature(PackageManager.FEATURE_FACE);
        boolean iris = Build.VERSION.SDK_INT >= 29 && pm.hasSystemFeature(PackageManager.FEATURE_IRIS);
        if (finger && !face && !iris) return B_FINGER;
        if (face && !finger && !iris) return B_FACE;
        return finger || face || iris ? B_OTHER : B_NONE;
    }

    @SuppressWarnings("deprecation")
    private static String biometryKind() throws Exception {
        if (Build.VERSION.SDK_INT < 28) return "!unsupported BiometricPrompt needs Android 9";
        Context c = app;
        int kind = hardwareKind(c);
        String err = "";
        if (Build.VERSION.SDK_INT >= 29) {
            BiometricManager bm = c.getSystemService(BiometricManager.class);
            int r = Build.VERSION.SDK_INT >= 30
                    ? bm.canAuthenticate(BiometricManager.Authenticators.BIOMETRIC_WEAK)
                    : bm.canAuthenticate();
            switch (r) {
                case BiometricManager.BIOMETRIC_SUCCESS: break;
                case BiometricManager.BIOMETRIC_ERROR_NONE_ENROLLED: err = "!notEnrolled"; break;
                default: err = "!notAvailable"; break;
            }
            if (r == BiometricManager.BIOMETRIC_SUCCESS && kind == B_NONE) kind = B_OTHER;
        } else if (kind == B_NONE) {
            err = "!notAvailable";
        }
        if (!err.isEmpty()) kind = B_NONE;
        return new JSONObject().put("kind", kind).put("error", err).toString();
    }

    @SuppressWarnings("deprecation")
    private static void authenticate(int cb, JSONObject o) {
        if (Build.VERSION.SDK_INT < 28) {
            fail(cb, "!unsupported BiometricPrompt needs Android 9");
            return;
        }
        Activity a = activity();
        if (a == null) {
            fail(cb, NO_ACTIVITY);
            return;
        }
        String title = o.optString("title");
        if (title.isEmpty()) title = String.valueOf(a.getApplicationInfo().loadLabel(a.getPackageManager()));
        String reason = o.optString("reason"), cancel = o.optString("cancel");
        if (cancel.isEmpty()) cancel = a.getString(android.R.string.cancel);
        boolean passcode = o.optBoolean("passcode");
        BiometricPrompt.Builder b = new BiometricPrompt.Builder(a).setTitle(title);
        if (!reason.isEmpty()) b.setSubtitle(reason);
        final boolean[] answered = {false};
        if (passcode && Build.VERSION.SDK_INT >= 30) {
            b.setAllowedAuthenticators(BiometricManager.Authenticators.BIOMETRIC_WEAK
                    | BiometricManager.Authenticators.DEVICE_CREDENTIAL);
        } else if (passcode && Build.VERSION.SDK_INT == 29) {
            b.setDeviceCredentialAllowed(true);
        } else {
            if (Build.VERSION.SDK_INT >= 30) {
                b.setAllowedAuthenticators(BiometricManager.Authenticators.BIOMETRIC_WEAK);
            }
            b.setNegativeButton(cancel, a.getMainExecutor(), (d, w) -> {
                if (!answered[0]) {
                    answered[0] = true;
                    fail(cb, "!canceled");
                }
            });
        }
        BiometricPrompt p;
        try {
            p = b.build();
        } catch (RuntimeException e) {
            fail(cb, "!notAvailable " + e.getMessage());
            return;
        }
        p.authenticate(new CancellationSignal(), a.getMainExecutor(), new BiometricPrompt.AuthenticationCallback() {
            @Override
            public void onAuthenticationSucceeded(BiometricPrompt.AuthenticationResult r) {
                if (answered[0]) return;
                answered[0] = true;
                done(cb, "");
            }

            @Override
            public void onAuthenticationError(int code, CharSequence msg) {
                if (answered[0]) return;
                answered[0] = true;
                String m = msg == null ? "" : " " + msg;
                switch (code) {
                    case BiometricPrompt.BIOMETRIC_ERROR_USER_CANCELED:
                    case BiometricPrompt.BIOMETRIC_ERROR_CANCELED:
                    case BiometricPrompt.BIOMETRIC_ERROR_TIMEOUT:
                        fail(cb, "!canceled" + m);
                        break;
                    case BiometricPrompt.BIOMETRIC_ERROR_LOCKOUT:
                    case BiometricPrompt.BIOMETRIC_ERROR_LOCKOUT_PERMANENT:
                        fail(cb, "!lockedOut" + m);
                        break;
                    case BiometricPrompt.BIOMETRIC_ERROR_NO_BIOMETRICS:
                    case BiometricPrompt.BIOMETRIC_ERROR_NO_DEVICE_CREDENTIAL:
                        fail(cb, "!notEnrolled" + m);
                        break;
                    case BiometricPrompt.BIOMETRIC_ERROR_HW_NOT_PRESENT:
                    case BiometricPrompt.BIOMETRIC_ERROR_HW_UNAVAILABLE:
                        fail(cb, "!notAvailable" + m);
                        break;
                    default:
                        fail(cb, "!authFailed" + m);
                }
            }
            // onAuthenticationFailed: one attempt failed; the prompt stays up.
        });
    }

    // ---- secure storage (any thread) ----

    private static final String KEY_ALIAS = "tango.securestore";

    private static SharedPreferences securePrefs() {
        return app.getSharedPreferences("tango.securestore", Context.MODE_PRIVATE);
    }

    private static synchronized SecretKey secureKey() throws Exception {
        KeyStore ks = KeyStore.getInstance("AndroidKeyStore");
        ks.load(null);
        java.security.Key k = ks.getKey(KEY_ALIAS, null);
        if (k instanceof SecretKey) return (SecretKey) k;
        KeyGenerator g = KeyGenerator.getInstance(KeyProperties.KEY_ALGORITHM_AES, "AndroidKeyStore");
        g.init(new KeyGenParameterSpec.Builder(KEY_ALIAS,
                KeyProperties.PURPOSE_ENCRYPT | KeyProperties.PURPOSE_DECRYPT)
                .setBlockModes(KeyProperties.BLOCK_MODE_GCM)
                .setEncryptionPaddings(KeyProperties.ENCRYPTION_PADDING_NONE)
                .setKeySize(256)
                .build());
        return g.generateKey();
    }

    private static synchronized String secureSet(String key, String b64) throws Exception {
        if (app == null) return NO_ACTIVITY;
        Cipher c = Cipher.getInstance("AES/GCM/NoPadding");
        c.init(Cipher.ENCRYPT_MODE, secureKey());
        c.updateAAD(key.getBytes(StandardCharsets.UTF_8)); // binds the value to its key
        byte[] ct = c.doFinal(Base64.decode(b64, Base64.NO_WRAP));
        byte[] iv = c.getIV();
        byte[] out = new byte[1 + iv.length + ct.length];
        out[0] = (byte) iv.length;
        System.arraycopy(iv, 0, out, 1, iv.length);
        System.arraycopy(ct, 0, out, 1 + iv.length, ct.length);
        if (!securePrefs().edit().putString(key, Base64.encodeToString(out, Base64.NO_WRAP)).commit()) {
            return "!error cannot write secure storage";
        }
        return "";
    }

    private static synchronized String secureGet(String key) throws Exception {
        if (app == null) return NO_ACTIVITY;
        String s = securePrefs().getString(key, null);
        if (s == null) return "!notFound";
        byte[] d = Base64.decode(s, Base64.NO_WRAP);
        int n = d[0] & 0xff;
        Cipher c = Cipher.getInstance("AES/GCM/NoPadding");
        c.init(Cipher.DECRYPT_MODE, secureKey(), new GCMParameterSpec(128, d, 1, n));
        c.updateAAD(key.getBytes(StandardCharsets.UTF_8));
        byte[] pt = c.doFinal(d, 1 + n, d.length - 1 - n);
        return Base64.encodeToString(pt, Base64.NO_WRAP);
    }

    private static synchronized String secureDelete(String key) {
        if (app == null) return NO_ACTIVITY;
        securePrefs().edit().remove(key).commit();
        return "";
    }

    // ---- photo picker and camera ----

    private static int pickCB, pickLimit, captureCB;
    private static File captureFile;

    private static boolean hasPhotoPicker() {
        return Build.VERSION.SDK_INT >= 33
                || (Build.VERSION.SDK_INT >= 30 && SdkExtensions.getExtensionVersion(Build.VERSION_CODES.R) >= 2);
    }

    private static void pickMedia(int cb, int types, int limit) {
        Activity a = activity();
        if (a == null) {
            fail(cb, NO_ACTIVITY);
            return;
        }
        if (pickCB != 0) {
            fail(cb, "!error a media picker is already open");
            return;
        }
        boolean images = (types & 1) != 0 || types == 0, videos = (types & 2) != 0;
        String type = images && videos ? null : videos ? "video/*" : "image/*";
        Intent i;
        if (hasPhotoPicker()) {
            i = new Intent(MediaStore.ACTION_PICK_IMAGES);
            if (type != null) i.setType(type);
            if (limit > 1) {
                int max = Build.VERSION.SDK_INT >= 33 ? MediaStore.getPickImagesMaxLimit() : 100;
                i.putExtra(MediaStore.EXTRA_PICK_IMAGES_MAX, Math.min(limit, max));
            }
        } else {
            i = new Intent(Intent.ACTION_GET_CONTENT);
            i.addCategory(Intent.CATEGORY_OPENABLE);
            if (type != null) {
                i.setType(type);
            } else {
                i.setType("*/*");
                i.putExtra(Intent.EXTRA_MIME_TYPES, new String[] {"image/*", "video/*"});
            }
            if (limit > 1) i.putExtra(Intent.EXTRA_ALLOW_MULTIPLE, true);
        }
        try {
            pickCB = cb;
            pickLimit = Math.max(1, limit);
            a.startActivityForResult(i, PICK_REQUEST);
        } catch (RuntimeException e) {
            pickCB = 0;
            fail(cb, "!notAvailable " + e.getMessage());
        }
    }

    private static void capturePhoto(int cb) {
        Activity a = activity();
        if (a == null) {
            fail(cb, NO_ACTIVITY);
            return;
        }
        if (!a.getPackageManager().hasSystemFeature(PackageManager.FEATURE_CAMERA_ANY)) {
            fail(cb, "!notAvailable this device has no camera");
            return;
        }
        if (captureCB != 0) {
            fail(cb, "!error the camera is already open");
            return;
        }
        // An app that declares CAMERA must hold it to start the camera app.
        if (declared(a, Manifest.permission.CAMERA)
                && a.checkSelfPermission(Manifest.permission.CAMERA) != PackageManager.PERMISSION_GRANTED) {
            requestPermission("camera", (s, e) -> {
                if (e != null) fail(cb, e);
                else if ("granted".equals(s)) startCapture(cb);
                else fail(cb, "!permissionDenied camera");
            });
            return;
        }
        startCapture(cb);
    }

    private static void startCapture(int cb) {
        Activity a = activity();
        if (a == null) {
            fail(cb, NO_ACTIVITY);
            return;
        }
        try {
            File dir = new File(a.getCacheDir(), "tango-media");
            if (!dir.isDirectory() && !dir.mkdirs()) throw new java.io.IOException("cannot create " + dir);
            File f = new File(dir, "photo-" + System.currentTimeMillis() + ".jpg");
            Uri out = TangoFileProvider.uriFor(a, f);
            Intent i = new Intent(MediaStore.ACTION_IMAGE_CAPTURE);
            i.putExtra(MediaStore.EXTRA_OUTPUT, out);
            i.setClipData(ClipData.newRawUri("output", out));
            i.addFlags(Intent.FLAG_GRANT_WRITE_URI_PERMISSION | Intent.FLAG_GRANT_READ_URI_PERMISSION);
            captureCB = cb;
            captureFile = f;
            a.startActivityForResult(i, CAPTURE_REQUEST);
        } catch (android.content.ActivityNotFoundException e) {
            captureCB = 0;
            fail(cb, "!notAvailable no camera app");
        } catch (Exception e) {
            captureCB = 0;
            fail(cb, "!error " + e);
        }
    }

    /** Activity results of the picker and the camera; false if not ours. */
    static boolean onActivityResult(Activity a, int request, int result, Intent data) {
        if (request == CAPTURE_REQUEST && captureCB != 0) {
            int cb = captureCB;
            File f = captureFile;
            captureCB = 0;
            captureFile = null;
            if (result == Activity.RESULT_OK && f != null && f.length() > 0) done(cb, f.getAbsolutePath());
            else {
                if (f != null) f.delete();
                fail(cb, "!canceled");
            }
            return true;
        }
        if (request != PICK_REQUEST || pickCB == 0) return false;
        int cb = pickCB;
        pickCB = 0;
        if (result != Activity.RESULT_OK || data == null) {
            fail(cb, "!canceled");
            return true;
        }
        ArrayList<Uri> uris = new ArrayList<>();
        ClipData clip = data.getClipData();
        if (clip != null) {
            for (int i = 0; i < clip.getItemCount(); i++) uris.add(clip.getItemAt(i).getUri());
        } else if (data.getData() != null) {
            uris.add(data.getData());
        }
        while (uris.size() > pickLimit) uris.remove(uris.size() - 1); // GET_CONTENT has no limit
        if (uris.isEmpty()) {
            fail(cb, "!canceled");
            return true;
        }
        ContentResolver cr = a.getContentResolver();
        File dir = new File(a.getCacheDir(), "tango-media/" + System.nanoTime());
        new Thread(() -> {
            try {
                if (!dir.mkdirs()) throw new java.io.IOException("cannot create " + dir);
                JSONArray out = new JSONArray();
                for (int i = 0; i < uris.size(); i++) out.put(copyMedia(cr, uris.get(i), dir, i));
                done(cb, out.toString());
            } catch (Exception e) {
                fail(cb, "!error copying the picked media: " + e);
            }
        }, "tango-media").start();
        return true;
    }

    private static String copyMedia(ContentResolver cr, Uri uri, File dir, int index) throws java.io.IOException {
        String name = null;
        try (Cursor c = cr.query(uri, new String[] {OpenableColumns.DISPLAY_NAME}, null, null, null)) {
            if (c != null && c.moveToFirst()) name = c.getString(0);
        } catch (RuntimeException ignored) {
            // generated name below
        }
        if (name == null || name.isEmpty() || name.contains("/") || name.startsWith(".")) {
            String ext = MimeTypeMap.getSingleton().getExtensionFromMimeType(cr.getType(uri));
            name = "media-" + index + (ext != null ? "." + ext : "");
        }
        File out = new File(dir, index + "-" + name);
        try (InputStream in = cr.openInputStream(uri); OutputStream os = new FileOutputStream(out)) {
            if (in == null) throw new java.io.IOException("cannot open " + uri);
            byte[] buf = new byte[64 * 1024];
            for (int n; (n = in.read(buf)) > 0; ) os.write(buf, 0, n);
        }
        return out.getAbsolutePath();
    }

    // ---- location ----

    private static final long LOCATION_TIMEOUT_MS = 30_000;
    private static Listener updates;

    private static LocationManager locationManager() {
        return (LocationManager) app.getSystemService(Context.LOCATION_SERVICE);
    }

    /** Runs ok once location permission is held, asking first if never asked. */
    private static void withLocation(int cb, Runnable ok) {
        String s = checkPermission("location");
        if (s.startsWith("!")) {
            fail(cb, s);
            return;
        }
        if (s.equals("granted") || s.equals("limited")) {
            ok.run();
            return;
        }
        requestPermission("location", (st, e) -> {
            if (e != null) fail(cb, e);
            else if (st.equals("granted") || st.equals("limited")) ok.run();
            else fail(cb, "!permissionDenied location");
        });
    }

    /** The best enabled provider for accuracy, or null when location is off. */
    private static String provider(int accuracy) {
        LocationManager lm = locationManager();
        if (Build.VERSION.SDK_INT >= 28 && !lm.isLocationEnabled()) return null;
        List<String> enabled = lm.getProviders(true); // only those this app may use
        String fused = Build.VERSION.SDK_INT >= 31 ? LocationManager.FUSED_PROVIDER : "fused";
        String[] order;
        switch (accuracy) {
            case 1: // high
                order = new String[] {LocationManager.GPS_PROVIDER, fused, LocationManager.NETWORK_PROVIDER};
                break;
            case 2: // low
                order = new String[] {LocationManager.NETWORK_PROVIDER, fused, LocationManager.PASSIVE_PROVIDER,
                        LocationManager.GPS_PROVIDER};
                break;
            default:
                order = new String[] {fused, LocationManager.NETWORK_PROVIDER, LocationManager.GPS_PROVIDER};
        }
        for (String p : order) if (enabled.contains(p)) return p;
        return null;
    }

    private static String json(Location l) {
        try {
            JSONObject o = new JSONObject();
            o.put("lat", l.getLatitude()).put("lon", l.getLongitude()).put("acc", l.getAccuracy());
            if (l.hasAltitude()) {
                o.put("alt", l.getAltitude());
                if (Build.VERSION.SDK_INT >= 26 && l.hasVerticalAccuracy()) o.put("altAcc", l.getVerticalAccuracyMeters());
            }
            o.put("speed", l.hasSpeed() ? l.getSpeed() : -1);
            o.put("heading", l.hasBearing() ? l.getBearing() : -1);
            o.put("time", l.getTime());
            return o.toString();
        } catch (org.json.JSONException e) {
            return "{}";
        }
    }

    @SuppressWarnings({"deprecation", "MissingPermission"})
    private static void currentLocation(int cb, JSONObject o) {
        int acc = o.optInt("accuracy");
        withLocation(cb, () -> {
            String p = provider(acc);
            if (p == null) {
                fail(cb, "!notAvailable location services are off");
                return;
            }
            LocationManager lm = locationManager();
            final boolean[] answered = {false};
            CancellationSignal cancel = new CancellationSignal();
            Listener single = new Listener(0);
            java.util.function.Consumer<Location> answer = l -> {
                if (answered[0]) return;
                answered[0] = true;
                if (Build.VERSION.SDK_INT < 30) lm.removeUpdates(single);
                if (l == null) {
                    try {
                        l = lm.getLastKnownLocation(p);
                    } catch (SecurityException ignored) {
                        l = null;
                    }
                }
                if (l == null) fail(cb, "!notAvailable no location fix");
                else done(cb, json(l));
            };
            single.onFix = answer;
            Tango.main.postDelayed(() -> {
                if (!answered[0]) {
                    cancel.cancel();
                    answer.accept(null);
                }
            }, LOCATION_TIMEOUT_MS);
            try {
                if (Build.VERSION.SDK_INT >= 30) {
                    lm.getCurrentLocation(p, cancel, app.getMainExecutor(), answer);
                } else {
                    lm.requestSingleUpdate(p, single, Looper.getMainLooper());
                }
            } catch (SecurityException e) {
                answered[0] = true;
                fail(cb, "!permissionDenied " + e.getMessage());
            }
        });
    }

    @SuppressWarnings("MissingPermission")
    private static void startLocation(int cb, JSONObject o) {
        stopLocation();
        int acc = o.optInt("accuracy");
        float dist = (float) o.optDouble("distance", 0);
        withLocation(cb, () -> {
            String p = provider(acc);
            if (p == null) {
                fail(cb, "!notAvailable location services are off");
                return;
            }
            Listener l = new Listener(cb);
            updates = l;
            long every = acc == 1 ? 1000 : acc == 2 ? 30_000 : 5000;
            try {
                locationManager().requestLocationUpdates(p, every, dist, l, Looper.getMainLooper());
            } catch (SecurityException e) {
                updates = null;
                fail(cb, "!permissionDenied " + e.getMessage());
            }
        });
    }

    private static void stopLocation() {
        Listener l = updates;
        updates = null;
        if (l != null) {
            l.stopped = true;
            locationManager().removeUpdates(l);
        }
    }

    /** All methods implemented: older Android has no default methods here. */
    private static final class Listener implements LocationListener {
        final int cb;
        boolean stopped;
        java.util.function.Consumer<Location> onFix;

        Listener(int cb) {
            this.cb = cb;
        }

        @Override
        public void onLocationChanged(Location l) {
            if (onFix != null) {
                onFix.accept(l);
                return;
            }
            if (!stopped) finish(cb, json(l), null);
        }

        @Override
        public void onProviderDisabled(String provider) {
            if (onFix != null || stopped) return;
            stopLocation();
            fail(cb, "!notAvailable location provider " + provider + " was turned off");
        }

        @Override
        public void onProviderEnabled(String provider) {}

        @Override
        @SuppressWarnings("deprecation")
        public void onStatusChanged(String provider, int status, Bundle extras) {}
    }

    // ---- push ----

    private static void registerPush(int cb) {
        String t = TangoPush.token;
        if (t != null) done(cb, t);
        else fail(cb, "!unsupported Firebase Cloud Messaging is not wired in (see org.tango.host.TangoPush)");
    }

    // ---- permissions ----

    interface PermissionResult {
        void done(String status, String err);
    }

    private static HashSet<String> declaredPerms;

    private static boolean declared(Context c, String perm) {
        if (declaredPerms == null) {
            declaredPerms = new HashSet<>();
            try {
                PackageInfo pi = c.getPackageManager().getPackageInfo(c.getPackageName(), PackageManager.GET_PERMISSIONS);
                if (pi.requestedPermissions != null) declaredPerms.addAll(Arrays.asList(pi.requestedPermissions));
            } catch (PackageManager.NameNotFoundException ignored) {
                // cannot happen for our own package
            }
        }
        return declaredPerms.contains(perm);
    }

    /** The runtime permissions behind name on this Android version (all, declared or not). */
    private static String[] runtimePerms(String name) {
        int sdk = Build.VERSION.SDK_INT;
        switch (name) {
            case "camera": return new String[] {Manifest.permission.CAMERA};
            case "microphone": return new String[] {Manifest.permission.RECORD_AUDIO};
            case "location":
                return new String[] {Manifest.permission.ACCESS_FINE_LOCATION, Manifest.permission.ACCESS_COARSE_LOCATION};
            case "location-always":
                return sdk >= 29 ? new String[] {Manifest.permission.ACCESS_BACKGROUND_LOCATION}
                        : new String[] {Manifest.permission.ACCESS_FINE_LOCATION, Manifest.permission.ACCESS_COARSE_LOCATION};
            case "photos":
                if (sdk >= 34) return new String[] {Manifest.permission.READ_MEDIA_IMAGES,
                        Manifest.permission.READ_MEDIA_VIDEO, Manifest.permission.READ_MEDIA_VISUAL_USER_SELECTED};
                if (sdk >= 33) return new String[] {Manifest.permission.READ_MEDIA_IMAGES, Manifest.permission.READ_MEDIA_VIDEO};
                return new String[] {Manifest.permission.READ_EXTERNAL_STORAGE};
            case "notifications":
                return sdk >= 33 ? new String[] {Manifest.permission.POST_NOTIFICATIONS} : new String[0];
            default:
                return null;
        }
    }

    private static SharedPreferences askedPrefs() {
        return app.getSharedPreferences("tango.permissions", Context.MODE_PRIVATE);
    }

    private static boolean granted(Context c, String p) {
        return c.checkSelfPermission(p) == PackageManager.PERMISSION_GRANTED;
    }

    /** Current status of name, or "!code message". Main thread. */
    static String checkPermission(String name) {
        Activity a = Tango.activity;
        Context c = a != null ? a : app;
        if (c == null) return NO_ACTIVITY;
        String[] all = runtimePerms(name);
        if (all == null) return "!unsupported unknown permission " + name;
        if (name.equals("notifications") && all.length == 0) {
            NotificationManager nm = c.getSystemService(NotificationManager.class);
            return nm.areNotificationsEnabled() ? "granted" : "denied";
        }
        ArrayList<String> decl = new ArrayList<>();
        for (String p : all) if (declared(c, p)) decl.add(p);
        if (decl.isEmpty()) return "!notConfigured AndroidManifest.xml lacks " + all[0];
        switch (name) {
            case "location":
                if (granted(c, Manifest.permission.ACCESS_FINE_LOCATION)) return "granted";
                if (granted(c, Manifest.permission.ACCESS_COARSE_LOCATION)) {
                    return decl.contains(Manifest.permission.ACCESS_FINE_LOCATION) ? "limited" : "granted";
                }
                break;
            case "location-always":
                if (Build.VERSION.SDK_INT >= 29) {
                    boolean fg = granted(c, Manifest.permission.ACCESS_FINE_LOCATION)
                            || granted(c, Manifest.permission.ACCESS_COARSE_LOCATION);
                    if (fg && granted(c, Manifest.permission.ACCESS_BACKGROUND_LOCATION)) return "granted";
                } else if (granted(c, Manifest.permission.ACCESS_FINE_LOCATION)
                        || granted(c, Manifest.permission.ACCESS_COARSE_LOCATION)) {
                    return "granted";
                }
                break;
            case "photos":
                if (Build.VERSION.SDK_INT >= 33) {
                    if (granted(c, Manifest.permission.READ_MEDIA_IMAGES) || granted(c, Manifest.permission.READ_MEDIA_VIDEO)) {
                        return "granted";
                    }
                    if (Build.VERSION.SDK_INT >= 34 && granted(c, Manifest.permission.READ_MEDIA_VISUAL_USER_SELECTED)) {
                        return "limited";
                    }
                } else if (granted(c, all[0])) {
                    return "granted";
                }
                break;
            default:
                if (granted(c, decl.get(0))) return "granted";
        }
        return askedPrefs().getBoolean(name, false) ? "denied" : "notDetermined";
    }

    private static final class PendingRequest {
        final String name;
        final PermissionResult result;

        PendingRequest(String name, PermissionResult result) {
            this.name = name;
            this.result = result;
        }
    }

    private static final ArrayDeque<PendingRequest> permQueue = new ArrayDeque<>();
    private static PendingRequest permInFlight;

    /** Prompts for name if the system still allows it, then reports the status. Main thread. */
    static void requestPermission(String name, PermissionResult r) {
        permQueue.add(new PendingRequest(name, r));
        nextPermission();
    }

    private static void nextPermission() {
        while (permInFlight == null && !permQueue.isEmpty()) {
            PendingRequest pr = permQueue.poll();
            String s = checkPermission(pr.name);
            if (s.startsWith("!")) {
                pr.result.done(null, s);
                continue;
            }
            Activity a = activity();
            if (s.equals("granted") || a == null || (pr.name.equals("notifications") && Build.VERSION.SDK_INT < 33)) {
                pr.result.done(s, null);
                continue;
            }
            ArrayList<String> ask = new ArrayList<>();
            if (pr.name.equals("location-always") && Build.VERSION.SDK_INT >= 29) {
                String fg = checkPermission("location");
                if (!fg.equals("granted") && !fg.equals("limited")) {
                    // Background location needs foreground location first.
                    for (String p : runtimePerms("location")) if (declared(a, p)) ask.add(p);
                    permInFlight = new PendingRequest(pr.name, (st, e) -> {
                        String now = checkPermission("location");
                        if (now.equals("granted") || now.equals("limited")) requestPermission("location-always", pr.result);
                        else pr.result.done(checkPermission("location-always"), null);
                    });
                    askedPrefs().edit().putBoolean("location", true).apply();
                    a.requestPermissions(ask.toArray(new String[0]), PERMISSION_REQUEST);
                    return;
                }
            }
            for (String p : runtimePerms(pr.name)) {
                if (declared(a, p) && !granted(a, p)) ask.add(p);
            }
            boolean canPrompt = !askedPrefs().getBoolean(pr.name, false);
            for (String p : ask) canPrompt |= a.shouldShowRequestPermissionRationale(p);
            if (ask.isEmpty() || !canPrompt) {
                pr.result.done(s, null);
                continue;
            }
            permInFlight = pr;
            askedPrefs().edit().putBoolean(pr.name, true).apply();
            a.requestPermissions(ask.toArray(new String[0]), PERMISSION_REQUEST);
        }
    }

    /** From TangoActivity.onRequestPermissionsResult; false if not ours. */
    static boolean onRequestPermissionsResult(int request, String[] perms, int[] results) {
        if (request != PERMISSION_REQUEST || permInFlight == null) return false;
        PendingRequest pr = permInFlight;
        permInFlight = null;
        String s = checkPermission(pr.name);
        if (s.startsWith("!")) pr.result.done(null, s);
        else pr.result.done(s, null);
        nextPermission();
        return true;
    }

    // ---- implemented in Go ----

    static native void nativeDone(int cb, String result, String error);

    static native void nativeEvent(int kind, String url, String path);

    /** The system language list changed (LocaleWatcher). Main thread. */
    static native void nativeLocalesChanged();
}
