package org.tango.host;

import android.app.Activity;
import android.app.Notification;
import android.app.NotificationChannel;
import android.app.NotificationManager;
import android.app.PendingIntent;
import android.content.ActivityNotFoundException;
import android.content.BroadcastReceiver;
import android.content.Context;
import android.content.Intent;
import android.content.IntentFilter;
import android.content.pm.PackageManager;
import android.net.ConnectivityManager;
import android.net.Network;
import android.net.NetworkCapabilities;
import android.net.Uri;
import android.os.BatteryManager;
import android.os.Build;
import android.os.PowerManager;
import android.util.Log;

import org.json.JSONObject;

import java.io.File;

/**
 * Desktop-style system capabilities for the Go backend (internal/platform/
 * android sysx_android.go): open a URL or a file with another app (Shell),
 * local notifications (Notifier), connectivity (Network) and battery /
 * power-saver state (Power). Reached through {@link Mobile#call}, same
 * conventions: "!code message" errors, asynchronous answers through
 * Mobile.nativeDone, watchers answer repeatedly with their callback id.
 */
final class Sysx {
    private Sysx() {}

    /** platform.AppNotificationOpened. */
    static final int NOTIFICATION_OPENED = 6;
    /** The extra carrying the notification id on the tap Intent. */
    static final String EXTRA_NOTIFICATION = "org.tango.notification";

    private static final String CHANNEL = "tango.notify", CHANNEL_SILENT = "tango.notify.silent";

    // platform.NetworkKind values.
    private static final int NK_NONE = 1, NK_WIRED = 2, NK_WIFI = 3, NK_CELLULAR = 4, NK_OTHER = 5;

    static String call(int cb, String op, JSONObject a) throws Exception {
        switch (op) {
            case "openURL": return openURL(a.getString("url"));
            case "openPath": return openPath(a.getString("path"));
            case "notifyPermission":
                Mobile.requestPermission("notifications", (s, e) -> {
                    if (e != null) Mobile.fail(cb, e);
                    else Mobile.done(cb, s.equals("granted") && notificationsEnabled() ? "1" : "0");
                });
                return "";
            case "notify": return notify(a);
            case "networkStatus": return networkStatus();
            case "networkWatch": return networkWatch(cb);
            case "networkUnwatch": networkUnwatch(); return "";
            case "powerStatus": return powerStatus();
            case "powerWatch": powerWatch(cb); return "";
            case "powerUnwatch": powerUnwatch(); return "";
            case "sleepBlocks": return sleepBlocks(a.optInt("display"), a.optInt("system"));
            default: return "!unsupported " + op;
        }
    }

    private static Context app() {
        return Mobile.app;
    }

    // ---- Shell ----

    /** The context to start activities from, with FLAG_ACTIVITY_NEW_TASK when it is not an Activity. */
    private static void start(Intent i) {
        Activity a = Tango.activity;
        if (a != null && !a.isFinishing()) {
            a.startActivity(i);
            return;
        }
        i.addFlags(Intent.FLAG_ACTIVITY_NEW_TASK);
        app().startActivity(i);
    }

    private static String openURL(String url) {
        Uri u = Uri.parse(url);
        if (u.getScheme() == null) return "!error no scheme in " + url;
        Intent i = new Intent(Intent.ACTION_VIEW, u);
        i.addCategory(Intent.CATEGORY_BROWSABLE);
        try {
            start(i);
        } catch (ActivityNotFoundException e) {
            return "!notAvailable no app opens " + u.getScheme() + ": URLs";
        }
        return "";
    }

    /** Opens a file with the app the user picks for its type, through TangoFileProvider. */
    private static String openPath(String path) throws Exception {
        File f = new File(path);
        if (!f.exists()) return "!notFound " + path;
        if (f.isDirectory()) return "!unsupported Android has no standard folder viewer";
        Context c = Tango.activity != null ? Tango.activity : app();
        Uri u = TangoFileProvider.uriFor(c, f);
        String mime = TangoFileProvider.mimeOf(f.getName());
        Intent i = new Intent(Intent.ACTION_VIEW);
        i.setDataAndType(u, mime);
        i.addFlags(Intent.FLAG_GRANT_READ_URI_PERMISSION);
        try {
            start(i);
        } catch (ActivityNotFoundException e) {
            return "!notAvailable no app opens " + mime;
        }
        return "";
    }

    // ---- Notifier ----

    private static boolean notificationsEnabled() {
        NotificationManager nm = app().getSystemService(NotificationManager.class);
        return nm != null && nm.areNotificationsEnabled();
    }

    private static void ensureChannels(NotificationManager nm) {
        if (Build.VERSION.SDK_INT < 26 || nm.getNotificationChannel(CHANNEL) != null) return;
        // Channel names show in the app's notification settings.
        nm.createNotificationChannel(new NotificationChannel(CHANNEL, "Notifications", NotificationManager.IMPORTANCE_DEFAULT));
        NotificationChannel silent = new NotificationChannel(CHANNEL_SILENT, "Silent notifications",
                NotificationManager.IMPORTANCE_LOW);
        silent.setSound(null, null);
        nm.createNotificationChannel(silent);
    }

    /**
     * Shows a notification now. When the user was never asked (Android
     * 13+), asks first and shows it if they allow; a refusal known
     * beforehand is an error.
     */
    private static String notify(JSONObject a) {
        String s = Mobile.checkPermission("notifications");
        if (s.startsWith("!")) return s;
        if (s.equals("granted")) return post(a);
        if (s.equals("denied")) return "!permissionDenied notifications are turned off for this app";
        Mobile.requestPermission("notifications", (st, e) -> {
            if (e == null && "granted".equals(st)) post(a);
            else Log.w(Tango.TAG, "notification " + a.optString("id") + " not shown: not allowed");
        });
        return "";
    }

    private static String post(JSONObject a) {
        Context c = app();
        NotificationManager nm = c.getSystemService(NotificationManager.class);
        if (nm == null) return "!notAvailable no NotificationManager";
        if (!nm.areNotificationsEnabled()) return "!permissionDenied notifications are turned off for this app";
        ensureChannels(nm);
        String id = a.optString("id");
        boolean silent = a.optBoolean("silent");
        String title = a.optString("title"), sub = a.optString("subtitle"), body = a.optString("body");

        Intent tap = new Intent(c, TangoActivity.class);
        tap.setAction("org.tango.NOTIFICATION");
        tap.putExtra(EXTRA_NOTIFICATION, id);
        tap.addFlags(Intent.FLAG_ACTIVITY_NEW_TASK | Intent.FLAG_ACTIVITY_SINGLE_TOP);
        PendingIntent pi = PendingIntent.getActivity(c, id.hashCode(), tap,
                PendingIntent.FLAG_IMMUTABLE | PendingIntent.FLAG_UPDATE_CURRENT);

        Notification.Builder b = Build.VERSION.SDK_INT >= 26
                ? new Notification.Builder(c, silent ? CHANNEL_SILENT : CHANNEL)
                : new Notification.Builder(c);
        b.setSmallIcon(smallIcon(c))
                .setContentTitle(title)
                .setContentText(body)
                .setContentIntent(pi)
                .setAutoCancel(true)
                .setWhen(System.currentTimeMillis())
                .setShowWhen(true);
        if (!sub.isEmpty()) b.setSubText(sub);
        if (!body.isEmpty()) b.setStyle(new Notification.BigTextStyle().bigText(body));
        if (Build.VERSION.SDK_INT < 26 && silent) b.setSound(null);
        nm.notify(id, 1, b.build());
        return "";
    }

    /** tan.json "icons.notification" (drawable ic_stat_tango), else the app icon. */
    private static int smallIcon(Context c) {
        int r = c.getResources().getIdentifier("ic_stat_tango", "drawable", c.getPackageName());
        if (r != 0) return r;
        r = c.getApplicationInfo().icon;
        return r != 0 ? r : android.R.drawable.ic_dialog_info;
    }

    /** From Mobile.handleIntent: a tap on one of our notifications. */
    static void handleIntent(Intent i) {
        String id = i.getStringExtra(EXTRA_NOTIFICATION);
        if (id == null) return;
        i.removeExtra(EXTRA_NOTIFICATION); // report a tap once
        Mobile.event(NOTIFICATION_OPENED, id, null);
    }

    // ---- Network ----

    private static ConnectivityManager cm() {
        return app().getSystemService(ConnectivityManager.class);
    }

    private static boolean networkStateDeclared() {
        return app().checkSelfPermission(android.Manifest.permission.ACCESS_NETWORK_STATE) == PackageManager.PERMISSION_GRANTED;
    }

    private static final String NO_NETWORK_STATE = "!notConfigured AndroidManifest.xml lacks android.permission.ACCESS_NETWORK_STATE";

    /** {"online":…,"kind":…,"metered":…,"constrained":…} of the default network. */
    private static String networkStatus() throws Exception {
        if (!networkStateDeclared()) return NO_NETWORK_STATE;
        ConnectivityManager m = cm();
        if (m == null) return "!notAvailable no ConnectivityManager";
        return describe(m, m.getActiveNetwork());
    }

    private static String describe(ConnectivityManager m, Network n) throws Exception {
        NetworkCapabilities caps = n == null ? null : m.getNetworkCapabilities(n);
        boolean online = caps != null && caps.hasCapability(NetworkCapabilities.NET_CAPABILITY_INTERNET)
                && caps.hasCapability(NetworkCapabilities.NET_CAPABILITY_VALIDATED);
        int kind = NK_NONE;
        boolean metered = false;
        if (caps != null && online) {
            if (caps.hasTransport(NetworkCapabilities.TRANSPORT_WIFI)) kind = NK_WIFI;
            else if (caps.hasTransport(NetworkCapabilities.TRANSPORT_CELLULAR)) kind = NK_CELLULAR;
            else if (caps.hasTransport(NetworkCapabilities.TRANSPORT_ETHERNET)) kind = NK_WIRED;
            else kind = NK_OTHER;
            metered = !caps.hasCapability(NetworkCapabilities.NET_CAPABILITY_NOT_METERED);
        }
        // Data Saver restricting this app counts as constrained (Low Data Mode).
        boolean constrained = metered
                && m.getRestrictBackgroundStatus() == ConnectivityManager.RESTRICT_BACKGROUND_STATUS_ENABLED;
        JSONObject o = new JSONObject();
        o.put("online", online);
        o.put("kind", kind);
        o.put("metered", metered);
        o.put("constrained", constrained);
        return o.toString();
    }

    private static ConnectivityManager.NetworkCallback netCallback;
    private static int netCB;

    /** Reports the default network's status to cb whenever it changes. */
    private static String networkWatch(int cb) {
        if (!networkStateDeclared()) return NO_NETWORK_STATE;
        networkUnwatch();
        ConnectivityManager m = cm();
        if (m == null) return "!notAvailable no ConnectivityManager";
        netCB = cb;
        netCallback = new ConnectivityManager.NetworkCallback() {
            @Override
            public void onAvailable(Network n) {
                report();
            }

            @Override
            public void onCapabilitiesChanged(Network n, NetworkCapabilities caps) {
                report();
            }

            @Override
            public void onLost(Network n) {
                report();
            }

            @Override
            public void onBlockedStatusChanged(Network n, boolean blocked) {
                report();
            }

            private void report() {
                Tango.main.post(() -> {
                    if (netCallback != this || netCB <= 0) return;
                    try {
                        Mobile.done(netCB, describe(m, m.getActiveNetwork()));
                    } catch (Exception e) {
                        Log.w(Tango.TAG, "network status", e);
                    }
                });
            }
        };
        m.registerDefaultNetworkCallback(netCallback, Tango.main);
        return "";
    }

    private static void networkUnwatch() {
        if (netCallback == null) return;
        try {
            cm().unregisterNetworkCallback(netCallback);
        } catch (IllegalArgumentException ignored) {
            // not registered
        }
        netCallback = null;
        netCB = 0;
    }

    // ---- Power ----

    private static Intent batteryIntent() {
        // Sticky broadcast: a null receiver only reads the last value.
        return app().registerReceiver(null, new IntentFilter(Intent.ACTION_BATTERY_CHANGED));
    }

    /** {"present":…,"plugged":…,"charging":…,"level":…,"lowPower":…}. */
    private static String powerStatus() throws Exception {
        Intent b = batteryIntent();
        JSONObject o = new JSONObject();
        boolean present = b != null && b.getBooleanExtra(BatteryManager.EXTRA_PRESENT, true);
        int status = b == null ? -1 : b.getIntExtra(BatteryManager.EXTRA_STATUS, -1);
        int plugged = b == null ? 0 : b.getIntExtra(BatteryManager.EXTRA_PLUGGED, 0);
        int level = b == null ? -1 : b.getIntExtra(BatteryManager.EXTRA_LEVEL, -1);
        int scale = b == null ? -1 : b.getIntExtra(BatteryManager.EXTRA_SCALE, -1);
        o.put("present", present);
        o.put("plugged", plugged != 0);
        o.put("charging", status == BatteryManager.BATTERY_STATUS_CHARGING);
        o.put("level", present && level >= 0 && scale > 0 ? (double) level / scale : -1);
        PowerManager pm = app().getSystemService(PowerManager.class);
        o.put("lowPower", pm != null && pm.isPowerSaveMode());
        return o.toString();
    }

    private static BroadcastReceiver powerReceiver;
    private static int powerCB;
    private static String lastPower;

    /** Reports the power status to cb when it changes (battery broadcasts, power-saver mode). */
    private static void powerWatch(int cb) {
        powerUnwatch();
        powerCB = cb;
        lastPower = null;
        powerReceiver = new BroadcastReceiver() {
            @Override
            public void onReceive(Context c, Intent i) {
                if (powerReceiver != this || powerCB <= 0) return;
                try {
                    String s = powerStatus();
                    // ACTION_BATTERY_CHANGED also fires for temperature and voltage.
                    if (s.equals(lastPower)) return;
                    lastPower = s;
                    Mobile.done(powerCB, s);
                } catch (Exception e) {
                    Log.w(Tango.TAG, "power status", e);
                }
            }
        };
        IntentFilter f = new IntentFilter();
        f.addAction(Intent.ACTION_BATTERY_CHANGED);
        f.addAction(Intent.ACTION_POWER_CONNECTED);
        f.addAction(Intent.ACTION_POWER_DISCONNECTED);
        f.addAction(PowerManager.ACTION_POWER_SAVE_MODE_CHANGED);
        // System broadcasts reach receivers that are not exported.
        if (Build.VERSION.SDK_INT >= 33) app().registerReceiver(powerReceiver, f, null, Tango.main, Context.RECEIVER_NOT_EXPORTED);
        else app().registerReceiver(powerReceiver, f, null, Tango.main);
    }

    private static void powerUnwatch() {
        if (powerReceiver == null) return;
        try {
            app().unregisterReceiver(powerReceiver);
        } catch (IllegalArgumentException ignored) {
            // not registered
        }
        powerReceiver = null;
        powerCB = 0;
    }

    // ---- sleep blocks ----

    private static int displayBlocks, systemBlocks;
    private static PowerManager.WakeLock wakeLock;

    /** Whether a sleep block keeps the screen on (Device.apply). */
    static boolean keepScreenOn() {
        return displayBlocks > 0 || (systemBlocks > 0 && !wakeLockDeclared());
    }

    private static boolean wakeLockDeclared() {
        return app().checkSelfPermission(android.Manifest.permission.WAKE_LOCK) == PackageManager.PERMISSION_GRANTED;
    }

    /**
     * The number of held sleep blocks of each kind. A system block holds a
     * partial wake lock when the app declares WAKE_LOCK, otherwise it
     * keeps the screen on like a display block.
     */
    private static String sleepBlocks(int display, int system) {
        displayBlocks = display;
        systemBlocks = system;
        if (system > 0 && wakeLockDeclared()) {
            if (wakeLock == null) {
                PowerManager pm = app().getSystemService(PowerManager.class);
                wakeLock = pm.newWakeLock(PowerManager.PARTIAL_WAKE_LOCK, "tango:sleep-block");
                wakeLock.acquire();
            }
        } else if (wakeLock != null) {
            if (wakeLock.isHeld()) wakeLock.release();
            wakeLock = null;
        }
        Device.apply(Tango.activity);
        return "";
    }
}
