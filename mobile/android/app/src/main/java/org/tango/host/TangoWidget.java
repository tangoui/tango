package org.tango.host;

import android.app.PendingIntent;
import android.appwidget.AppWidgetManager;
import android.appwidget.AppWidgetProvider;
import android.content.Context;
import android.content.Intent;
import android.content.SharedPreferences;
import android.widget.RemoteViews;

/**
 * The app's home-screen widget: a title and a text that Go sets with
 * sys/widget (Device.updateWidget), stored in SharedPreferences so the
 * widget shows them without the app running. Tapping it opens the app.
 *
 * tan build declares it in the manifest when tan.json "permissions" lists
 * "widget" (res/xml/tango_widget_info.xml, res/layout/tango_widget.xml).
 */
public class TangoWidget extends AppWidgetProvider {
    @Override
    public void onUpdate(Context c, AppWidgetManager m, int[] ids) {
        m.updateAppWidget(ids, views(c));
    }

    static RemoteViews views(Context c) {
        SharedPreferences p = c.getSharedPreferences(Device.WIDGET_PREFS, Context.MODE_PRIVATE);
        RemoteViews v = new RemoteViews(c.getPackageName(), R.layout.tango_widget);
        String title = p.getString("title", "");
        if (title.isEmpty()) title = c.getString(R.string.app_name);
        v.setTextViewText(R.id.tango_widget_title, title);
        v.setTextViewText(R.id.tango_widget_text, p.getString("text", ""));
        Intent open = c.getPackageManager().getLaunchIntentForPackage(c.getPackageName());
        if (open != null) {
            v.setOnClickPendingIntent(R.id.tango_widget_root,
                    PendingIntent.getActivity(c, 0, open, PendingIntent.FLAG_IMMUTABLE | PendingIntent.FLAG_UPDATE_CURRENT));
        }
        return v;
    }
}
