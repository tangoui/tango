package org.tango.host;

import android.os.Bundle;

import org.json.JSONObject;

import java.util.Map;

/**
 * Entry points for an app that wires in Firebase Cloud Messaging itself.
 * Tango does not bundle Google services; to get push on Android, add the
 * Firebase dependency and google-services.json to your copy of
 * mobile/android and a FirebaseMessagingService such as:
 *
 * <pre>
 * public class MyFcm extends FirebaseMessagingService {
 *     public void onNewToken(String t) { TangoPush.token(t); }
 *     public void onMessageReceived(RemoteMessage m) {
 *         RemoteMessage.Notification n = m.getNotification();
 *         TangoPush.received(m.getData(), n != null ? n.getTitle() : null, n != null ? n.getBody() : null);
 *     }
 * }
 * </pre>
 *
 * and at startup report the current token:
 * FirebaseMessaging.getInstance().getToken().addOnSuccessListener(TangoPush::token).
 * Taps on FCM notifications that launch or resume the app are reported
 * automatically (the launch Intent carries "google.message_id"); call
 * {@link #opened} for notifications your app builds itself.
 *
 * All methods are safe from any thread.
 */
public final class TangoPush {
    private TangoPush() {}

    static volatile String token;

    /** A new FCM registration token. */
    public static void token(String t) {
        if (t == null || t.isEmpty()) return;
        token = t;
        Mobile.event(Mobile.PUSH_TOKEN, t, null);
    }

    /** A message arrived while the app runs. */
    public static void received(Map<String, String> data, String title, String body) {
        Mobile.event(Mobile.PUSH_RECEIVED, payload(data, title, body, null), null);
    }

    /** The user tapped a notification; action is the action id or null. */
    public static void opened(Map<String, String> data, String title, String body, String action) {
        Mobile.event(Mobile.PUSH_OPENED, payload(data, title, body, action), null);
    }

    static String payload(Map<String, String> data, String title, String body, String action) {
        JSONObject o = new JSONObject();
        try {
            if (title != null && !title.isEmpty()) o.put("title", title);
            if (body != null && !body.isEmpty()) o.put("body", body);
            if (action != null && !action.isEmpty()) o.put("action", action);
            JSONObject d = new JSONObject();
            if (data != null) for (Map.Entry<String, String> e : data.entrySet()) d.put(e.getKey(), e.getValue());
            o.put("data", d);
        } catch (org.json.JSONException ignored) {
            // keys are strings: cannot happen
        }
        return o.toString();
    }

    /** Payload of a notification tap from the launch Intent's extras. */
    static String fromExtras(Bundle extras) {
        java.util.HashMap<String, String> data = new java.util.HashMap<>();
        for (String k : extras.keySet()) {
            Object v = extras.get(k);
            if (v != null) data.put(k, String.valueOf(v));
        }
        return payload(data, null, null, null);
    }
}
