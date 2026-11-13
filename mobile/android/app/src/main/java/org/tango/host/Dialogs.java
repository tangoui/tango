package org.tango.host;

import android.app.Activity;
import android.app.AlertDialog;
import android.content.ClipData;
import android.content.ContentResolver;
import android.content.DialogInterface;
import android.content.Intent;
import android.database.Cursor;
import android.net.Uri;
import android.provider.OpenableColumns;
import android.webkit.MimeTypeMap;

import java.io.File;
import java.io.FileOutputStream;
import java.io.InputStream;
import java.io.OutputStream;
import java.util.ArrayList;
import java.util.LinkedHashSet;

/** Message and file dialogs (Tango's platform.Dialogs). Main thread. */
final class Dialogs {
    private static final int OPEN_REQUEST = 0x7a6e; // arbitrary, < 0x10000

    // platform.MessageKind values.
    private static final int INFO = 0, WARNING = 1, ERROR = 2, QUESTION = 3;

    /** The callback of the document picker in flight, or 0. */
    private static int openCallback;

    private Dialogs() {}

    static void message(Activity a, int cb, int kind, String title, String message, String[] buttons) {
        if (a == null || a.isFinishing()) {
            Tango.nativeDialogDone(cb, -1, null, "tango: no Activity to show the dialog in");
            return;
        }
        final boolean[] done = {false};
        AlertDialog.Builder b = new AlertDialog.Builder(a);
        if (title != null && !title.isEmpty()) b.setTitle(title);
        switch (kind) {
            case WARNING:
            case ERROR:
                b.setIcon(android.R.drawable.ic_dialog_alert);
                break;
            case INFO:
                b.setIcon(android.R.drawable.ic_dialog_info);
                break;
            default:
                break;
        }
        if (buttons.length <= 3) {
            if (message != null && !message.isEmpty()) b.setMessage(message);
            for (int i = 0; i < buttons.length; i++) {
                final int idx = i;
                DialogInterface.OnClickListener l = (d, w) -> finish(done, cb, idx);
                if (i == 0) b.setPositiveButton(buttons[i], l);
                else if (i == 1) b.setNegativeButton(buttons[i], l);
                else b.setNeutralButton(buttons[i], l);
            }
        } else {
            // AlertDialog has three buttons; show more as a list (the
            // message, if any, goes in the title area).
            if (message != null && !message.isEmpty()) {
                b.setTitle(title == null || title.isEmpty() ? message : title + "\n" + message);
            }
            b.setItems(buttons, (d, w) -> finish(done, cb, w));
        }
        b.setOnCancelListener(d -> finish(done, cb, -1));
        b.setOnDismissListener(d -> finish(done, cb, -1));
        b.show();
    }

    private static void finish(boolean[] done, int cb, int button) {
        if (done[0]) return;
        done[0] = true;
        Tango.nativeDialogDone(cb, button, null, null);
    }

    static void open(Activity a, int cb, String title, String[] patterns, boolean multiple) {
        if (a == null || a.isFinishing()) {
            Tango.nativeDialogDone(cb, 0, null, "tango: no Activity to show the picker in");
            return;
        }
        if (openCallback != 0) {
            Tango.nativeDialogDone(cb, 0, null, "tango: a file dialog is already open");
            return;
        }
        Intent i = new Intent(Intent.ACTION_OPEN_DOCUMENT);
        i.addCategory(Intent.CATEGORY_OPENABLE);
        i.setType("*/*");
        String[] types = mimeTypes(patterns);
        if (types.length > 0) i.putExtra(Intent.EXTRA_MIME_TYPES, types);
        if (multiple) i.putExtra(Intent.EXTRA_ALLOW_MULTIPLE, true);
        if (title != null && !title.isEmpty()) i.putExtra(Intent.EXTRA_TITLE, title);
        try {
            openCallback = cb;
            a.startActivityForResult(i, OPEN_REQUEST);
        } catch (RuntimeException e) {
            openCallback = 0;
            Tango.nativeDialogDone(cb, 0, null, "tango: " + e);
        }
    }

    /** "*.png" → "image/png"; unknown extensions and "*" match everything. */
    private static String[] mimeTypes(String[] patterns) {
        LinkedHashSet<String> out = new LinkedHashSet<>();
        MimeTypeMap map = MimeTypeMap.getSingleton();
        for (String p : patterns) {
            int dot = p.lastIndexOf('.');
            String ext = dot >= 0 ? p.substring(dot + 1).toLowerCase() : "";
            String t = ext.isEmpty() || ext.equals("*") ? null : map.getMimeTypeFromExtension(ext);
            if (t == null) return new String[0];
            out.add(t);
        }
        return out.toArray(new String[0]);
    }

    /** Handles the picker's result; false if the request was not ours. */
    static boolean onActivityResult(Activity a, int request, int result, Intent data) {
        if (request != OPEN_REQUEST || openCallback == 0) return false;
        final int cb = openCallback;
        openCallback = 0;
        if (result != Activity.RESULT_OK || data == null) {
            Tango.nativeDialogDone(cb, 0, null, null);
            return true;
        }
        final ArrayList<Uri> uris = new ArrayList<>();
        ClipData clip = data.getClipData();
        if (clip != null) {
            for (int i = 0; i < clip.getItemCount(); i++) uris.add(clip.getItemAt(i).getUri());
        } else if (data.getData() != null) {
            uris.add(data.getData());
        }
        final ContentResolver cr = a.getContentResolver();
        final File dir = new File(a.getCacheDir(), "tango-picked/" + System.nanoTime());
        // Copy off the main thread: documents may be large or remote.
        new Thread(() -> {
            String[] paths = new String[uris.size()];
            String err = null;
            try {
                if (!dir.mkdirs()) throw new java.io.IOException("cannot create " + dir);
                for (int i = 0; i < uris.size(); i++) paths[i] = copy(cr, uris.get(i), dir, i);
            } catch (Exception e) {
                err = "tango: copying the picked document: " + e;
            }
            final String[] ps = err == null ? paths : null;
            final String fe = err;
            Tango.main.post(() -> Tango.nativeDialogDone(cb, 0, ps, fe));
        }, "tango-picker").start();
        return true;
    }

    private static String copy(ContentResolver cr, Uri uri, File dir, int index) throws java.io.IOException {
        String name = displayName(cr, uri);
        if (name == null || name.isEmpty() || name.contains("/") || name.startsWith(".")) name = "document-" + index;
        File sub = new File(dir, Integer.toString(index));
        if (!sub.mkdirs()) throw new java.io.IOException("cannot create " + sub);
        File out = new File(sub, name);
        try (InputStream in = cr.openInputStream(uri); OutputStream os = new FileOutputStream(out)) {
            if (in == null) throw new java.io.IOException("cannot open " + uri);
            byte[] buf = new byte[64 * 1024];
            for (int n; (n = in.read(buf)) > 0; ) os.write(buf, 0, n);
        }
        return out.getAbsolutePath();
    }

    private static String displayName(ContentResolver cr, Uri uri) {
        try (Cursor c = cr.query(uri, new String[] {OpenableColumns.DISPLAY_NAME}, null, null, null)) {
            if (c != null && c.moveToFirst()) return c.getString(0);
        } catch (RuntimeException ignored) {
            // fall back to a generated name
        }
        return null;
    }
}
