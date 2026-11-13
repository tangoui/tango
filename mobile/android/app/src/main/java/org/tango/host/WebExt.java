package org.tango.host;

import android.graphics.Bitmap;
import android.graphics.Canvas;
import android.net.Uri;
import android.os.Build;
import android.os.Message;
import android.util.Base64;
import android.util.Log;
import android.webkit.CookieManager;
import android.webkit.GeolocationPermissions;
import android.webkit.HttpAuthHandler;
import android.webkit.PermissionRequest;
import android.webkit.URLUtil;
import android.webkit.WebChromeClient;
import android.webkit.WebResourceRequest;
import android.webkit.WebSettings;
import android.webkit.WebStorage;
import android.webkit.WebView;
import android.webkit.WebViewClient;
import android.webkit.WebViewDatabase;

import org.json.JSONObject;

import java.io.ByteArrayOutputStream;
import java.io.File;
import java.io.FileOutputStream;
import java.io.IOException;
import java.io.InputStream;
import java.io.OutputStream;
import java.net.HttpURLConnection;
import java.net.URL;
import java.nio.charset.StandardCharsets;
import java.util.ArrayList;
import java.util.HashMap;
import java.util.Iterator;
import java.util.List;
import java.util.WeakHashMap;
import java.util.concurrent.ConcurrentHashMap;
import java.util.concurrent.atomic.AtomicLong;

/**
 * Advanced WebView features (internal/platform/webview_ext.go): page
 * permissions, navigation policy, new windows, HTTP authentication,
 * downloads, cookies and website data, user agent, find in page and
 * screenshots. Go calls {@link #call}; asynchronous operations (cb &gt; 0)
 * answer through {@link #nativeDone} on the main thread. Requests the page
 * makes reach Go through the other native methods; Go answers them with
 * call(..., "permissionDone" / "authDone" / "downloadStart", ...) by request
 * id, now or later. Main thread unless noted.
 */
final class WebExt {
    // platform.PermissionDecision.
    private static final int P_DEFAULT = 0, P_PROMPT = 1, P_ALLOW = 2, P_DENY = 3;
    // platform.AuthAction.
    private static final int A_DEFAULT = 0, A_CREDENTIALS = 1, A_CANCEL = 2;
    // platform.DownloadState.
    private static final int D_PROGRESS = 1, D_DONE = 2, D_FAILED = 3, D_CANCELED = 4;
    // platform.BrowsingData.
    private static final int DATA_COOKIES = 1, DATA_CACHE = 2, DATA_STORAGE = 4;

    private static final long FIND_TIMEOUT_MS = 2000, POPUP_TIMEOUT_MS = 1000, PROGRESS_MS = 250;

    private WebExt() {}

    /** Per-page state; survives WebView replacement (Page.recover). */
    private static final class State {
        String userAgent; // null: the platform's
        int findCb; // pending find answer, 0 none
        boolean findActive, findNoWrap, findBackwardFirst;
        int matches, active;
        String authKey; // host\0realm of the last answered challenge
        int authRetry;
    }

    private static final WeakHashMap<Page, State> states = new WeakHashMap<>();
    /** User agent for the next Page created (WebCreator). */
    private static String presetUserAgent;

    private static State state(Page p) {
        State s = states.get(p);
        if (s == null) {
            s = new State();
            s.userAgent = presetUserAgent;
            presetUserAgent = null;
            states.put(p, s);
        }
        return s;
    }

    /** Hooks a new WebView of p up. From Page.makeWebView. */
    static void attach(Page p, WebView w) {
        State st = state(p);
        WebSettings s = w.getSettings();
        s.setSupportMultipleWindows(true);
        // JavaScriptCanOpenWindowsAutomatically stays false: like WebKit's
        // popup blocker, only window.open calls from a user gesture reach
        // onCreateWindow (and so the core, which may open the browser).
        if (st.userAgent != null) s.setUserAgentString(st.userAgent);
        w.setWebChromeClient(new Chrome(p));
        w.setDownloadListener((url, ua, disposition, mime, length) ->
                onDownload(p, url, ua, disposition, mime, length));
        w.setFindListener((ordinal, count, doneCounting) -> onFind(p, w, ordinal, count, doneCounting));
    }

    // ---- called from Go ----

    static String call(Page p, int cb, String op, String json) {
        if (!Tango.isMainThread() && !op.equals("getCookie")) {
            Tango.main.post(() -> call(p, cb, op, json));
            return "";
        }
        try {
            JSONObject a = new JSONObject(json == null || json.isEmpty() ? "{}" : json);
            switch (op) {
                case "permissionDone": permissionDone(a.getInt("id"), a.optInt("decision")); return "";
                case "authDone":
                    authDone(a.getInt("id"), a.optInt("action"), a.optString("user"), a.optString("password"));
                    return "";
                case "downloadStart": downloadStart(a.getLong("id"), a.optString("path")); return "";
                case "cancelDownload": cancelDownload(a.getLong("id")); return "";
                case "presetUserAgent": presetUserAgent = emptyToNull(a.optString("ua")); return "";
                case "getCookie": {
                    String c = CookieManager.getInstance().getCookie(a.getString("url"));
                    return c == null ? "" : c;
                }
                case "setCookie": setCookie(cb, a.getString("url"), a.getString("value")); return "";
                case "clearCookies": clearCookies(cb, a.optString("domain")); return "";
                default:
                    break;
            }
            if (p == null || p.closed) return "!notAvailable page closed";
            switch (op) {
                case "userAgent": {
                    State st = state(p);
                    st.userAgent = emptyToNull(a.optString("ua"));
                    p.web.getSettings().setUserAgentString(st.userAgent);
                    return "";
                }
                case "clearData": clearData(p, cb, a.optInt("kinds")); return "";
                case "find":
                    find(p, cb, a.optString("text"), a.optBoolean("backward"), a.optBoolean("noWrap"));
                    return "";
                case "findNext": findNext(p, cb, a.optBoolean("backward")); return "";
                case "stopFind": stopFind(p); return "";
                case "screenshot": screenshot(p, cb); return "";
                default: return "!unsupported " + op;
            }
        } catch (Throwable t) {
            Log.e(Tango.TAG, "WebExt." + op, t);
            String msg = "!error " + t;
            if (cb > 0) {
                fail(cb, msg);
                return "";
            }
            return msg;
        }
    }

    private static String emptyToNull(String s) {
        return s == null || s.isEmpty() ? null : s;
    }

    private static void done(int cb, String result, byte[] data) {
        if (cb <= 0) return;
        if (Tango.isMainThread()) nativeDone(cb, result, data, null);
        else Tango.main.post(() -> nativeDone(cb, result, data, null));
    }

    private static void fail(int cb, String err) {
        if (cb <= 0) return;
        if (Tango.isMainThread()) nativeDone(cb, null, null, err);
        else Tango.main.post(() -> nativeDone(cb, null, null, err));
    }

    /** scheme://host[:port] of u, or "". */
    private static String origin(Uri u) {
        if (u == null || u.getScheme() == null || u.getHost() == null) return "";
        String o = u.getScheme() + "://" + u.getHost();
        return u.getPort() != -1 ? o + ":" + u.getPort() : o;
    }

    // ---- WebChromeClient ----

    private static final class Chrome extends WebChromeClient {
        private final Page page;

        Chrome(Page page) {
            this.page = page;
        }

        @Override
        public void onPermissionRequest(PermissionRequest r) {
            if (page.closed) {
                r.deny();
                return;
            }
            List<String> kinds = new ArrayList<>();
            for (String res : r.getResources()) {
                switch (res) {
                    case PermissionRequest.RESOURCE_VIDEO_CAPTURE: kinds.add("camera"); break;
                    case PermissionRequest.RESOURCE_AUDIO_CAPTURE: kinds.add("microphone"); break;
                    case PermissionRequest.RESOURCE_PROTECTED_MEDIA_ID: kinds.add("protected-media"); break;
                    case PermissionRequest.RESOURCE_MIDI_SYSEX: kinds.add("midi"); break;
                    default: kinds.add("other"); break;
                }
            }
            Perm pr = new Perm(page, kinds);
            pr.web = r;
            askPermission(pr, origin(r.getOrigin()));
        }

        @Override
        public void onPermissionRequestCanceled(PermissionRequest r) {
            for (Iterator<Perm> it = perms.values().iterator(); it.hasNext(); ) {
                Perm pr = it.next();
                if (pr.web == r) {
                    pr.canceled = true;
                    it.remove();
                }
            }
        }

        @Override
        public void onGeolocationPermissionsShowPrompt(String origin, GeolocationPermissions.Callback cb) {
            if (page.closed) {
                cb.invoke(origin, false, false);
                return;
            }
            List<String> kinds = new ArrayList<>();
            kinds.add("geolocation");
            Perm pr = new Perm(page, kinds);
            pr.geoOrigin = origin;
            pr.geo = cb;
            askPermission(pr, origin(Uri.parse(origin)));
        }

        @Override
        public void onGeolocationPermissionsHidePrompt() {
            for (Iterator<Perm> it = perms.values().iterator(); it.hasNext(); ) {
                Perm pr = it.next();
                if (pr.page == page && pr.geo != null) {
                    pr.canceled = true;
                    it.remove();
                }
            }
        }

        @Override
        public boolean onCreateWindow(WebView view, boolean isDialog, boolean isUserGesture, Message resultMsg) {
            if (page.closed) return false;
            // A tapped link (target=_blank) names its URL in the hit test:
            // report it and let the platform drop the popup.
            WebView.HitTestResult ht = view.getHitTestResult();
            if (isUserGesture && ht != null && ht.getType() == WebView.HitTestResult.SRC_ANCHOR_TYPE
                    && ht.getExtra() != null) {
                newWindow(page, view, ht.getExtra(), true);
                return false;
            }
            // window.open from script: the URL is only known once the new
            // window starts loading it, so hand the platform a throwaway
            // WebView, catch its first navigation and destroy it.
            new PopupCatcher(page, view, isUserGesture).start(resultMsg);
            return true;
        }

        @Override
        public void onProgressChanged(WebView view, int progress) {
            // A finished load ends any authentication retry sequence.
            if (progress >= 100 && view == page.web) {
                State st = state(page);
                st.authKey = null;
                st.authRetry = 0;
            }
        }
    }

    // ---- permissions ----

    private static final class Perm {
        final Page page;
        final List<String> kinds;
        PermissionRequest web;
        String geoOrigin;
        GeolocationPermissions.Callback geo;
        boolean canceled;

        Perm(Page page, List<String> kinds) {
            this.page = page;
            this.kinds = kinds;
        }

        void answer(boolean allow) {
            if (canceled) return;
            canceled = true;
            if (geo != null) {
                geo.invoke(geoOrigin, allow, false);
            } else if (allow) {
                web.grant(web.getResources());
            } else {
                web.deny();
            }
        }
    }

    private static int nextRequest;
    private static final HashMap<Integer, Perm> perms = new HashMap<>();

    private static int newRequest() {
        if (++nextRequest <= 0) nextRequest = 1;
        return nextRequest;
    }

    private static void askPermission(Perm pr, String origin) {
        int id = newRequest();
        perms.put(id, pr);
        nativePermission(pr.page.id, id, origin, pr.kinds.toArray(new String[0]));
    }

    private static void permissionDone(int id, int decision) {
        Perm pr = perms.remove(id);
        if (pr == null || pr.canceled) return;
        if (decision == P_DENY) {
            pr.answer(false);
            return;
        }
        // The page can only use the camera, microphone or location when the
        // app holds the runtime permission: ask the system first (a no-op
        // when granted, or when AndroidManifest.xml lacks it).
        List<String> sys = new ArrayList<>();
        boolean unbacked = false; // a kind no system prompt covers
        for (String k : pr.kinds) {
            switch (k) {
                case "camera": sys.add("camera"); break;
                case "microphone": sys.add("microphone"); break;
                case "geolocation": sys.add("location"); break;
                default: unbacked = true; break;
            }
        }
        final boolean allow = decision == P_ALLOW;
        final boolean noPrompt = unbacked;
        requestSystem(sys, 0, true, granted -> pr.answer(allow || (granted && !noPrompt)));
    }

    private interface Granted {
        void done(boolean granted);
    }

    private static void requestSystem(List<String> names, int i, boolean ok, Granted g) {
        if (i >= names.size()) {
            g.done(ok);
            return;
        }
        Mobile.requestPermission(names.get(i), (status, err) -> {
            boolean got = err == null && ("granted".equals(status) || "limited".equals(status));
            requestSystem(names, i + 1, ok && got, g);
        });
    }

    // ---- navigation policy and new windows ----

    /** From Page.Client.shouldOverrideUrlLoading: true cancels. */
    static boolean shouldOverrideUrlLoading(Page p, WebView view, WebResourceRequest req) {
        if (p.closed || view != p.web || !req.isForMainFrame()) return false;
        return !nativeShouldNavigate(p.id, req.getUrl().toString(), req.hasGesture());
    }

    private static void newWindow(Page p, WebView view, String url, boolean gesture) {
        if (p.closed) return;
        // Without a WebHost, open the URL in place, as before multiple
        // windows were enabled.
        if (!nativeNewWindow(p.id, url, gesture) && !url.isEmpty() && view == p.web) view.loadUrl(url);
    }

    private static final class PopupCatcher extends WebViewClient {
        private final Page page;
        private final WebView opener;
        private final boolean gesture;
        private WebView popup;
        private boolean caught;

        PopupCatcher(Page page, WebView opener, boolean gesture) {
            this.page = page;
            this.opener = opener;
            this.gesture = gesture;
        }

        void start(Message resultMsg) {
            popup = new WebView(opener.getContext());
            popup.setWebViewClient(this);
            ((WebView.WebViewTransport) resultMsg.obj).setWebView(popup);
            resultMsg.sendToTarget();
            // window.open() without a URL never navigates.
            Tango.main.postDelayed(() -> caught(""), POPUP_TIMEOUT_MS);
        }

        private void caught(String url) {
            if (caught) return;
            caught = true;
            WebView w = popup;
            Tango.main.post(() -> {
                w.stopLoading();
                w.destroy();
            });
            newWindow(page, opener, url, gesture);
        }

        @Override
        public boolean shouldOverrideUrlLoading(WebView view, WebResourceRequest req) {
            caught(req.getUrl().toString());
            return true;
        }

        @Override
        public void onPageStarted(WebView view, String url, Bitmap favicon) {
            if (url == null || url.equals("about:blank")) return;
            view.stopLoading();
            caught(url);
        }
    }

    // ---- HTTP authentication ----

    private static final class Auth {
        final Page page;
        final HttpAuthHandler handler;
        final String host, realm;
        final int retry;

        Auth(Page page, HttpAuthHandler handler, String host, String realm, int retry) {
            this.page = page;
            this.handler = handler;
            this.host = host;
            this.realm = realm;
            this.retry = retry;
        }
    }

    private static final HashMap<Integer, Auth> auths = new HashMap<>();

    /** From Page.Client.onReceivedHttpAuthRequest. */
    static void onReceivedHttpAuthRequest(Page p, WebView view, HttpAuthHandler h, String host, String realm) {
        if (p.closed) {
            h.cancel();
            return;
        }
        State st = state(p);
        String key = host + "\0" + realm;
        Auth a = new Auth(p, h, host, realm, key.equals(st.authKey) ? st.authRetry : 0);
        int id = newRequest();
        auths.put(id, a);
        if (!nativeAuth(p.id, id, host == null ? "" : host, realm == null ? "" : realm, a.retry)) {
            authDone(id, A_DEFAULT, null, null);
        }
    }

    private static void authDone(int id, int action, String user, String password) {
        Auth a = auths.remove(id);
        if (a == null) return;
        State st = state(a.page);
        if (action == A_DEFAULT && a.handler.useHttpAuthUsernamePassword()) {
            // The platform default: credentials the WebView stored, or fail.
            String[] saved = savedCredentials(a.page.web, a.host, a.realm);
            if (saved != null && saved.length == 2 && saved[0] != null && saved[1] != null) {
                action = A_CREDENTIALS;
                user = saved[0];
                password = saved[1];
            }
        }
        if (action == A_CREDENTIALS) {
            st.authKey = a.host + "\0" + a.realm;
            st.authRetry = a.retry + 1;
            a.handler.proceed(user == null ? "" : user, password == null ? "" : password);
        } else {
            st.authKey = null;
            st.authRetry = 0;
            a.handler.cancel();
        }
    }

    @SuppressWarnings("deprecation")
    private static String[] savedCredentials(WebView w, String host, String realm) {
        if (Build.VERSION.SDK_INT >= 26) {
            return WebViewDatabase.getInstance(w.getContext()).getHttpAuthUsernamePassword(host, realm);
        }
        return w.getHttpAuthUsernamePassword(host, realm);
    }

    // ---- downloads ----

    private static final class Pending {
        final long page;
        final String url, userAgent, referer;
        final long size;

        Pending(long page, String url, String userAgent, String referer, long size) {
            this.page = page;
            this.url = url;
            this.userAgent = userAgent;
            this.referer = referer;
            this.size = size;
        }
    }

    private static final AtomicLong nextDownload = new AtomicLong();
    private static final HashMap<Long, Pending> pending = new HashMap<>();
    private static final ConcurrentHashMap<Long, Download> downloads = new ConcurrentHashMap<>();

    private static void onDownload(Page p, String url, String ua, String disposition, String mime, long length) {
        if (p.closed || url == null) return;
        long id = nextDownload.incrementAndGet();
        String name = URLUtil.guessFileName(url, disposition, mime);
        long size = length > 0 ? length : -1;
        pending.put(id, new Pending(p.id, url, ua, p.web.getUrl(), size));
        if (!nativeDownloadStarted(p.id, id, url, name == null ? "" : name, mime == null ? "" : mime, size)) {
            pending.remove(id);
        }
    }

    private static void downloadStart(long id, String path) {
        Pending pd = pending.remove(id);
        if (pd == null || path.isEmpty()) return;
        Download d = new Download(id, pd, path);
        downloads.put(id, d);
        d.start();
    }

    private static void cancelDownload(long id) {
        Download d = downloads.get(id);
        if (d != null) d.cancel();
    }

    /**
     * Downloads to the path Go chose with the page's cookies and user agent,
     * on its own thread. Written next to path and renamed when complete.
     */
    private static final class Download extends Thread {
        private final long id;
        private final Pending pd;
        private final String path;
        private volatile boolean canceled;
        private volatile HttpURLConnection conn;
        private long lastReport;

        Download(long id, Pending pd, String path) {
            super("tango-download-" + id);
            this.id = id;
            this.pd = pd;
            this.path = path;
        }

        void cancel() {
            canceled = true;
            HttpURLConnection c = conn;
            if (c != null) new Thread(c::disconnect).start(); // may touch the network
        }

        private void report(int state, long received, long total, String err) {
            Tango.main.post(() -> {
                if (state != D_PROGRESS) downloads.remove(id);
                nativeDownloadEvent(pd.page, id, state, received, total, path, err);
            });
        }

        @Override
        public void run() {
            File dst = new File(path);
            File tmp = new File(path + ".tgpart");
            long received = 0, total = pd.size;
            String err = null;
            try {
                InputStream in;
                if (pd.url.regionMatches(true, 0, "data:", 0, 5)) {
                    byte[] b = dataURL(pd.url);
                    total = b.length;
                    in = new java.io.ByteArrayInputStream(b);
                } else {
                    HttpURLConnection c = connect();
                    long n = c.getContentLengthLong();
                    if (n >= 0) total = n;
                    in = c.getInputStream();
                }
                try (InputStream src = in; OutputStream out = new FileOutputStream(tmp)) {
                    byte[] buf = new byte[64 << 10];
                    int n;
                    while ((n = src.read(buf)) >= 0) {
                        if (canceled) break;
                        out.write(buf, 0, n);
                        received += n;
                        long now = android.os.SystemClock.elapsedRealtime();
                        if (now - lastReport >= PROGRESS_MS) {
                            lastReport = now;
                            report(D_PROGRESS, received, total, null);
                        }
                    }
                }
                if (!canceled) {
                    if (dst.exists() && !dst.delete()) throw new IOException("cannot replace " + path);
                    if (!tmp.renameTo(dst)) throw new IOException("cannot rename to " + path);
                }
            } catch (Throwable t) {
                if (!canceled) err = t.getMessage() != null ? t.getMessage() : t.toString();
            } finally {
                HttpURLConnection c = conn;
                if (c != null) c.disconnect();
                if (canceled || err != null) tmp.delete();
            }
            if (canceled) report(D_CANCELED, received, total, null);
            else if (err != null) report(D_FAILED, received, total, err);
            else report(D_DONE, received, total, null);
        }

        private HttpURLConnection connect() throws IOException {
            URL u = new URL(pd.url);
            for (int redirects = 0; ; redirects++) {
                String scheme = u.getProtocol();
                if (!"http".equals(scheme) && !"https".equals(scheme)) {
                    throw new IOException("cannot download " + scheme + ": URLs");
                }
                HttpURLConnection c = (HttpURLConnection) u.openConnection();
                conn = c;
                if (canceled) throw new IOException("canceled");
                c.setInstanceFollowRedirects(false);
                c.setConnectTimeout(30_000);
                c.setReadTimeout(60_000);
                if (pd.userAgent != null && !pd.userAgent.isEmpty()) c.setRequestProperty("User-Agent", pd.userAgent);
                String cookie = CookieManager.getInstance().getCookie(u.toString());
                if (cookie != null && !cookie.isEmpty()) c.setRequestProperty("Cookie", cookie);
                if (pd.referer != null && pd.referer.startsWith("http")) c.setRequestProperty("Referer", pd.referer);
                int code = c.getResponseCode();
                if (code >= 300 && code < 400 && code != 304) {
                    String loc = c.getHeaderField("Location");
                    c.disconnect();
                    if (loc == null || redirects >= 10) throw new IOException("HTTP " + code);
                    u = new URL(u, loc);
                    continue;
                }
                if (code < 200 || code >= 300) {
                    c.disconnect();
                    throw new IOException("HTTP " + code);
                }
                return c;
            }
        }

        private static byte[] dataURL(String url) throws IOException {
            int comma = url.indexOf(',');
            if (comma < 0) throw new IOException("bad data: URL");
            String meta = url.substring(5, comma), body = url.substring(comma + 1);
            if (meta.endsWith(";base64")) return Base64.decode(Uri.decode(body), Base64.DEFAULT);
            return Uri.decode(body).getBytes(StandardCharsets.UTF_8);
        }
    }

    // ---- cookies and website data ----

    private static void setCookie(int cb, String url, String value) {
        CookieManager cm = CookieManager.getInstance();
        cm.setCookie(url, value, ok -> {
            cm.flush();
            if (ok != null && ok) done(cb, "", null);
            else fail(cb, "!error the WebView rejected the cookie");
        });
    }

    /**
     * Expires the cookies the WebView would send to http(s)://domain/: the
     * only ones CookieManager can name. Cookies of subdomains or with other
     * paths are not reached; "" clears all.
     */
    private static void clearCookies(int cb, String domain) {
        CookieManager cm = CookieManager.getInstance();
        if (domain.isEmpty()) {
            cm.removeAllCookies(ok -> {
                cm.flush();
                done(cb, "", null);
            });
            return;
        }
        String host = domain.startsWith(".") ? domain.substring(1) : domain;
        String expired = "=; Max-Age=0; Expires=Thu, 01 Jan 1970 00:00:00 GMT; Path=/";
        for (String scheme : new String[] {"https://", "http://"}) {
            String url = scheme + host + "/";
            String all = cm.getCookie(url);
            if (all == null) continue;
            for (String pair : all.split(";")) {
                String name = pair.trim();
                int eq = name.indexOf('=');
                if (eq >= 0) name = name.substring(0, eq);
                if (name.isEmpty()) continue;
                String secure = scheme.equals("https://") ? "; Secure" : "";
                cm.setCookie(url, name + expired + secure); // host-only
                cm.setCookie(url, name + expired + "; Domain=" + host + secure); // domain cookie
            }
        }
        cm.flush();
        done(cb, "", null);
    }

    private static void clearData(Page p, int cb, int kinds) {
        if ((kinds & DATA_CACHE) != 0) p.web.clearCache(true);
        if ((kinds & DATA_STORAGE) != 0) WebStorage.getInstance().deleteAllData();
        if ((kinds & DATA_COOKIES) != 0) {
            CookieManager cm = CookieManager.getInstance();
            cm.removeAllCookies(ok -> {
                cm.flush();
                done(cb, "", null);
            });
            return;
        }
        done(cb, "", null);
    }

    // ---- find in page ----

    private static void findAnswer(State st, boolean found) {
        int cb = st.findCb;
        st.findCb = 0;
        if (cb == 0) return;
        try {
            JSONObject o = new JSONObject();
            o.put("matches", st.matches);
            o.put("active", st.active);
            o.put("found", found);
            done(cb, o.toString(), null);
        } catch (Exception e) {
            fail(cb, "!error " + e);
        }
    }

    private static void findEmpty(int cb) {
        done(cb, "{\"matches\":0,\"active\":0,\"found\":false}", null);
    }

    /** Answers cb with what is known if the listener never reports. */
    private static void findTimeout(Page p, int cb) {
        Tango.main.postDelayed(() -> {
            State st = state(p);
            if (st.findCb == cb) findAnswer(st, st.active > 0);
        }, FIND_TIMEOUT_MS);
    }

    private static void find(Page p, int cb, String text, boolean backward, boolean noWrap) {
        State st = state(p);
        findAnswer(st, false); // a superseded search
        st.matches = st.active = 0;
        if (text.isEmpty()) {
            st.findActive = false;
            p.web.clearMatches();
            findEmpty(cb);
            return;
        }
        st.findActive = true;
        st.findNoWrap = noWrap;
        st.findBackwardFirst = backward;
        st.findCb = cb;
        p.web.findAllAsync(text); // always case-insensitive
        findTimeout(p, cb);
    }

    private static void findNext(Page p, int cb, boolean backward) {
        State st = state(p);
        if (!st.findActive || st.matches == 0) {
            findAnswer(st, st.active > 0);
            findEmpty(cb);
            return;
        }
        findAnswer(st, st.active > 0);
        if (st.findNoWrap && (backward ? st.active <= 1 : st.active >= st.matches)) {
            st.findCb = cb;
            findAnswer(st, false); // at the end: stay
            return;
        }
        st.findCb = cb;
        p.web.findNext(!backward);
        findTimeout(p, cb);
    }

    private static void stopFind(Page p) {
        State st = state(p);
        st.findActive = false;
        findAnswer(st, false);
        st.matches = st.active = 0;
        p.web.clearMatches();
    }

    private static void onFind(Page p, WebView w, int ordinal, int count, boolean doneCounting) {
        if (w != p.web || !doneCounting) return;
        State st = state(p);
        if (!st.findActive) return;
        st.matches = count;
        st.active = count > 0 ? ordinal + 1 : 0;
        if (st.findBackwardFirst) {
            st.findBackwardFirst = false;
            if (count > 1) {
                w.findNext(false); // from the first match to the last
                return;
            }
        }
        findAnswer(st, count > 0);
    }

    // ---- screenshot ----

    private static void screenshot(Page p, int cb) {
        WebView w = p.web;
        int width = w.getWidth(), height = w.getHeight();
        if (width <= 0 || height <= 0) {
            fail(cb, "!notAvailable the page is not laid out");
            return;
        }
        Bitmap bm = Bitmap.createBitmap(width, height, Bitmap.Config.ARGB_8888);
        Canvas c = new Canvas(bm);
        c.translate(-w.getScrollX(), -w.getScrollY());
        w.draw(c);
        new Thread(() -> {
            ByteArrayOutputStream out = new ByteArrayOutputStream();
            boolean ok = bm.compress(Bitmap.CompressFormat.PNG, 100, out);
            bm.recycle();
            if (ok) done(cb, "", out.toByteArray());
            else fail(cb, "!error PNG encoding failed");
        }, "tango-screenshot").start();
    }

    // ---- implemented in Go ----

    /** Completes an asynchronous operation: result and data, or error ("!code message"). */
    static native void nativeDone(int cb, String result, byte[] data, String error);
    /** Go answers with call(null, 0, "permissionDone", {id, decision}). */
    static native void nativePermission(long page, int id, String origin, String[] kinds);
    /** Host.ShouldNavigate; false cancels. */
    static native boolean nativeShouldNavigate(long page, String url, boolean gesture);
    /** Host.OnNewWindow; false when Go has no WebHost. */
    static native boolean nativeNewWindow(long page, String url, boolean gesture);
    /** Go answers with "authDone"; false when Go has no WebHost (nothing will answer). */
    static native boolean nativeAuth(long page, int id, String host, String realm, int retry);
    /** Go answers with "downloadStart"; false when Go has no WebHost. */
    static native boolean nativeDownloadStarted(long page, long id, String url, String filename, String mime, long size);
    static native void nativeDownloadEvent(long page, long id, int state, long received, long total, String path, String error);
}
