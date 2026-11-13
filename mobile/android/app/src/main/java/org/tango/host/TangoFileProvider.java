package org.tango.host;

import android.content.ContentProvider;
import android.content.ContentValues;
import android.content.Context;
import android.database.Cursor;
import android.database.MatrixCursor;
import android.net.Uri;
import android.os.ParcelFileDescriptor;
import android.provider.OpenableColumns;
import android.webkit.MimeTypeMap;

import java.io.File;
import java.io.FileInputStream;
import java.io.FileNotFoundException;
import java.io.FileOutputStream;
import java.io.IOException;
import java.io.InputStream;
import java.io.OutputStream;
import java.util.List;

/**
 * Hands files in the app's cache and files directories to other apps as
 * content:// URIs (sharing, the camera's output file) without androidx.
 * It is not exported: other apps reach a URI only through a permission
 * grant on the Intent that carries it.
 *
 * URIs look like content://&lt;applicationId&gt;.tango.files/cache/some/file.jpg
 * ("cache" or "files" names the root directory).
 */
public final class TangoFileProvider extends ContentProvider {
    static String authority(Context c) {
        return c.getPackageName() + ".tango.files";
    }

    /**
     * The content URI of f. Files outside the cache and files directories
     * are copied into cache/tango-share first.
     */
    static Uri uriFor(Context c, File f) throws IOException {
        File cf = f.getCanonicalFile();
        File cache = c.getCacheDir().getCanonicalFile(), files = c.getFilesDir().getCanonicalFile();
        String root;
        String rel;
        if ((rel = relative(cache, cf)) != null) {
            root = "cache";
        } else if ((rel = relative(files, cf)) != null) {
            root = "files";
        } else {
            File dir = new File(cache, "tango-share/" + System.nanoTime());
            if (!dir.mkdirs()) throw new IOException("cannot create " + dir);
            File out = new File(dir, cf.getName());
            try (InputStream in = new FileInputStream(cf); OutputStream os = new FileOutputStream(out)) {
                byte[] buf = new byte[64 * 1024];
                for (int n; (n = in.read(buf)) > 0; ) os.write(buf, 0, n);
            }
            root = "cache";
            rel = relative(cache, out.getCanonicalFile());
        }
        Uri.Builder b = new Uri.Builder().scheme("content").authority(authority(c)).appendPath(root);
        for (String seg : rel.split("/")) if (!seg.isEmpty()) b.appendPath(seg);
        return b.build();
    }

    /** path of f below dir, or null when f is not inside dir. */
    private static String relative(File dir, File f) {
        String d = dir.getPath(), p = f.getPath();
        if (!p.startsWith(d + "/")) return null;
        return p.substring(d.length() + 1);
    }

    static String mimeOf(String name) {
        int dot = name.lastIndexOf('.');
        String t = dot < 0 ? null
                : MimeTypeMap.getSingleton().getMimeTypeFromExtension(name.substring(dot + 1).toLowerCase());
        return t != null ? t : "application/octet-stream";
    }

    private File fileFor(Uri uri) throws FileNotFoundException {
        Context c = getContext();
        List<String> segs = uri.getPathSegments();
        if (c == null || segs.size() < 2) throw new FileNotFoundException(uri.toString());
        File root;
        switch (segs.get(0)) {
            case "cache":
                root = c.getCacheDir();
                break;
            case "files":
                root = c.getFilesDir();
                break;
            default:
                throw new FileNotFoundException(uri.toString());
        }
        try {
            File r = root.getCanonicalFile();
            File f = new File(r, String.join("/", segs.subList(1, segs.size()))).getCanonicalFile();
            if (relative(r, f) == null) throw new FileNotFoundException(uri.toString());
            return f;
        } catch (IOException e) {
            throw new FileNotFoundException(uri + ": " + e);
        }
    }

    @Override
    public boolean onCreate() {
        return true;
    }

    @Override
    public ParcelFileDescriptor openFile(Uri uri, String mode) throws FileNotFoundException {
        File f = fileFor(uri);
        int m = ParcelFileDescriptor.parseMode(mode);
        if ((m & ParcelFileDescriptor.MODE_CREATE) != 0) {
            File dir = f.getParentFile();
            if (dir != null) dir.mkdirs();
        }
        return ParcelFileDescriptor.open(f, m);
    }

    @Override
    public Cursor query(Uri uri, String[] projection, String selection, String[] args, String sort) {
        File f;
        try {
            f = fileFor(uri);
        } catch (FileNotFoundException e) {
            return null;
        }
        if (projection == null) projection = new String[] {OpenableColumns.DISPLAY_NAME, OpenableColumns.SIZE};
        MatrixCursor c = new MatrixCursor(projection, 1);
        Object[] row = new Object[projection.length];
        for (int i = 0; i < projection.length; i++) {
            if (OpenableColumns.DISPLAY_NAME.equals(projection[i])) row[i] = f.getName();
            else if (OpenableColumns.SIZE.equals(projection[i])) row[i] = f.length();
        }
        c.addRow(row);
        return c;
    }

    @Override
    public String getType(Uri uri) {
        String last = uri.getLastPathSegment();
        return mimeOf(last == null ? "" : last);
    }

    @Override
    public Uri insert(Uri uri, ContentValues values) {
        throw new UnsupportedOperationException("read-only");
    }

    @Override
    public int delete(Uri uri, String selection, String[] args) {
        return 0;
    }

    @Override
    public int update(Uri uri, ContentValues values, String selection, String[] args) {
        return 0;
    }
}
