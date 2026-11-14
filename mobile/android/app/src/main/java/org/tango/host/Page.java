package org.tango.host;

import android.annotation.SuppressLint;
import android.content.Context;
import android.graphics.Bitmap;
import android.graphics.Color;
import android.graphics.Insets;
import android.net.Uri;
import android.os.Build;
import android.view.View;
import android.view.ViewGroup;
import android.view.WindowInsets;
import android.webkit.CookieManager;
import android.webkit.JavascriptInterface;
import android.webkit.RenderProcessGoneDetail;
import android.webkit.HttpAuthHandler;
import android.webkit.WebResourceError;
import android.webkit.WebResourceRequest;
import android.webkit.WebResourceResponse;
import android.webkit.WebSettings;
import android.webkit.WebView;
import android.webkit.WebViewClient;
import android.widget.FrameLayout;

import androidx.webkit.JavaScriptReplyProxy;
import androidx.webkit.WebMessageCompat;
import androidx.webkit.WebViewCompat;
import androidx.webkit.WebViewFeature;

import java.io.IOException;
import java.io.InputStream;
import java.util.ArrayList;
import java.util.HashMap;
import java.util.LinkedHashSet;
import java.util.Map;
import java.util.Set;

/**
 * One Tango surface: a full-screen WebView in the Activity. Go owns its
 * lifetime through the methods below (main thread).
 *
 * <p>The WebView sits in a {@link Frame} (the view placed in the Activity),
 * which also holds the canvases attached to the page
 * ({@link CanvasOverlay}, uihost.Attach) above or below it. While a canvas
 * is below, the WebView's background is transparent (the page's CSS must
 * not paint one either).
 *
 * <p>The bridge (internal/platform/android, token.go and script.go): only
 * documents of the app's trusted origins, the asset origin and the origin
 * Go last navigated the page to, get the page script, each with its own
 * token. The channel is a WebMessageListener when the WebView supports it:
 * the WebView puts its object only in frames of the trusted origins and
 * says which origin and frame sent each message; only main-frame messages
 * are passed on. Otherwise it is a JavascriptInterface, which every frame
 * sees; there the token alone tells the app's document apart.
 */
final class Page extends PageBase {
    /** Name of the channel object in the page (script.go jsInterface). */
    static final String CHANNEL = "__tangoAndroid";
    static final String ASSET_ORIGIN = Tango.ASSET_SCHEME + "://" + Tango.ASSET_HOST;
    /** Whether the WebView has WebMessageListener; decided with the first page. */
    private static Boolean listenerSupported;

    /** Replaced when the render process dies (see recover). Main thread. */
    WebView web;
    /** Holds the WebView and the attached canvases; the view in the Activity's root. */
    final Frame frame;
    /** Canvases attached to the page, in the order they came. */
    private final ArrayList<CanvasOverlay> overlays = new ArrayList<>();
    /** The WebView's background is transparent for a canvas below it. */
    private boolean webClear;
    private final boolean debug;
    /** Origin of the URL Go last navigated to (trusted besides the asset origin), or null. */
    private String navOrigin;
    private String lastUrl;
    private long lastRecover;
    private int recoveries;
    private int widthDp = -1, heightDp = -1;
    /**
     * The reply proxy of the last probe Go accepted on the lane channel
     * (internal/platform/android/lanes.go): down lane packets go to the page
     * through it (laneSend). Cleared when a document starts (unless Go says
     * the new document's own probe came first), when the WebView is replaced
     * and when the page is destroyed. Main thread.
     */
    private JavaScriptReplyProxy laneReply;
    /**
     * Margins keeping the WebView clear of the system bars, cut-out and
     * keyboard (left, top, right, bottom; px): what the root layout's
     * padding used to do before insets were applied per page.
     */
    private final int[] margins = new int[4];

    @SuppressLint({"SetJavaScriptEnabled", "JavascriptInterface", "AddJavascriptInterface"})
    Page(Context base, long id, boolean debug, String title) {
        super(base, id, title);
        this.debug = debug;
        setMargins(TangoActivity.insets());
        frame = new Frame(context);
        FrameLayout.LayoutParams lp = new FrameLayout.LayoutParams(
                ViewGroup.LayoutParams.MATCH_PARENT, ViewGroup.LayoutParams.MATCH_PARENT);
        lp.setMargins(margins[0], margins[1], margins[2], margins[3]);
        frame.setLayoutParams(lp);
        web = makeWebView();
        frame.addView(web);
    }

    @SuppressLint({"SetJavaScriptEnabled", "JavascriptInterface", "AddJavascriptInterface"})
    private WebView makeWebView() {
        if (debug) WebView.setWebContentsDebuggingEnabled(true);
        WebView w = new WebView(context);
        w.setLayoutParams(new FrameLayout.LayoutParams(
                ViewGroup.LayoutParams.MATCH_PARENT, ViewGroup.LayoutParams.MATCH_PARENT));
        if (webClear) w.setBackgroundColor(Color.TRANSPARENT);
        WebSettings s = w.getSettings();
        s.setJavaScriptEnabled(true);
        s.setDomStorageEnabled(true);
        s.setAllowFileAccess(false);
        s.setAllowContentAccess(false);
        s.setMediaPlaybackRequiresUserGesture(false);
        if (listenerSupported == null) {
            listenerSupported = WebViewFeature.isFeatureSupported(WebViewFeature.WEB_MESSAGE_LISTENER);
        }
        if (listenerSupported) addListener(w);
        else w.addJavascriptInterface(new Channel(), CHANNEL);
        w.setWebViewClient(new Client());
        WebExt.attach(this, w); // WebChromeClient, downloads, find, popups, user agent
        w.addOnLayoutChangeListener((v, l, t, r, b, ol, ot, or, ob) -> reportSize());
        return w;
    }

    /**
     * The render process died (killed for memory or crashed): replace the
     * WebView, which cannot be reused, and reload the page. Go is told with
     * NAVIGATING and an AppWebViewRestored event. A page that keeps crashing
     * (three times within ten seconds) is closed instead.
     */
    private void recover(WebView dead) {
        if (closed || dead != web) return;
        long now = android.os.SystemClock.elapsedRealtime();
        recoveries = now - lastRecover < 10_000 ? recoveries + 1 : 1;
        lastRecover = now;
        if (recoveries > 3) {
            close();
            return;
        }
        String url = lastUrl != null ? lastUrl : dead.getUrl();
        WebView w = makeWebView();
        w.setVisibility(dead.getVisibility());
        ViewGroup parent = (ViewGroup) dead.getParent();
        if (parent != null) {
            int idx = parent.indexOfChild(dead);
            parent.removeView(dead);
            parent.addView(w, idx);
        }
        web = w;
        laneReply = null;
        removeChannel(dead);
        dead.destroy();
        widthDp = heightDp = -1;
        Tango.nativeSurfaceEvent(id, Tango.NAVIGATING, 0, 0, url, null);
        Mobile.event(Mobile.WEBVIEW_RESTORED, url, Long.toString(id));
        if (url != null) w.loadUrl(url);
    }

    // ---- PageBase ----

    @Override
    View view() {
        return frame;
    }

    @Override
    void reportEvent(int kind) {
        Tango.nativeSurfaceEvent(id, kind, 0, 0, null, null);
    }

    @Override
    boolean goBack() {
        if (!web.canGoBack()) return false;
        web.goBack();
        return true;
    }

    /** The WebView (with its canvases) keeps clear of the system bars, cut-out and keyboard, as before. */
    @Override
    void applyInsets(WindowInsets insets) {
        if (!setMargins(insets)) return;
        FrameLayout.LayoutParams lp = (FrameLayout.LayoutParams) frame.getLayoutParams();
        if (lp == null) {
            lp = new FrameLayout.LayoutParams(ViewGroup.LayoutParams.MATCH_PARENT, ViewGroup.LayoutParams.MATCH_PARENT);
        }
        lp.setMargins(margins[0], margins[1], margins[2], margins[3]);
        frame.setLayoutParams(lp);
    }

    /** The top-most attached canvas that takes Back, or null. */
    @Override
    CanvasView backCanvas() {
        for (int i = frame.getChildCount() - 1; i >= 0; i--) {
            View c = frame.getChildAt(i);
            if (c instanceof CanvasView) {
                CanvasView v = (CanvasView) c;
                if (v.backHandling && !v.released && v.getVisibility() == View.VISIBLE) return v;
            }
        }
        return null;
    }

    // ---- attached canvases (CanvasOverlay) ----

    /** Adds a canvas above or below the WebView (TangoCanvas.newOverlay). */
    void addOverlay(CanvasOverlay o) {
        overlays.add(o);
        insertOverlay(o);
        overlaysChanged();
    }

    /**
     * Above: over everything so far; below: right under the WebView, over
     * the canvases below it so far.
     */
    private void insertOverlay(CanvasOverlay o) {
        if (o.above) frame.addView(o.canvas);
        else frame.addView(o.canvas, Math.max(0, frame.indexOfChild(web)));
    }

    /** The canvas changed sides of the WebView; it keeps the keyboard focus. */
    void restackOverlay(CanvasOverlay o) {
        boolean focused = o.canvas.hasFocus();
        frame.removeView(o.canvas);
        insertOverlay(o);
        if (focused) o.canvas.requestFocus();
    }

    void removeOverlay(CanvasOverlay o) {
        overlays.remove(o);
        frame.removeView(o.canvas);
        overlaysChanged();
    }

    /** Canvases came, went or moved: the WebView is see-through while one is below it. */
    void overlaysChanged() {
        if (closed) return;
        boolean under = false;
        for (CanvasOverlay o : overlays) under |= !o.closed && !o.above;
        if (under != webClear) {
            webClear = under;
            web.setBackgroundColor(under ? Color.TRANSPARENT : Color.WHITE);
        }
        frame.requestLayout();
    }

    /**
     * The page's layout: the WebView fills it; canvases with fixed bounds
     * are placed where Go computes (TangoCanvas.nativePlace: the bounds kept
     * at their anchor for the current size).
     */
    static final class Frame extends FrameLayout {
        Frame(Context c) {
            super(c);
        }

        @Override
        protected void onLayout(boolean changed, int l, int t, int r, int b) {
            super.onLayout(changed, l, t, r, b);
            int w = r - l, h = b - t;
            float d = getResources().getDisplayMetrics().density;
            for (int i = 0; i < getChildCount(); i++) {
                View c = getChildAt(i);
                if (!(c instanceof CanvasView) || c.getVisibility() == GONE) continue;
                CanvasView v = (CanvasView) c;
                if (!v.overlay || v.fill || v.released) continue;
                long p = TangoCanvas.nativePlace(v.id, w, h, d);
                if (p == Long.MIN_VALUE) continue;
                int x = (int) (p >> 32), y = (int) p;
                c.layout(x, y, x + c.getMeasuredWidth(), y + c.getMeasuredHeight());
            }
        }
    }

    /** Updates margins from insets; true if they changed. */
    private boolean setMargins(WindowInsets insets) {
        if (insets == null || Build.VERSION.SDK_INT < 30) return false;
        Insets i = insets.getInsets(WindowInsets.Type.systemBars()
                | WindowInsets.Type.displayCutout() | WindowInsets.Type.ime());
        int[] m = {i.left, i.top, i.right, i.bottom};
        if (java.util.Arrays.equals(m, margins)) return false;
        System.arraycopy(m, 0, margins, 0, 4);
        return true;
    }

    // ---- called from Go ----

    /** Destroys the page and reports CLOSED. */
    @Override
    void close() {
        if (closed) return;
        destroy();
        TangoActivity.removePage(this);
        Tango.nativeSurfaceEvent(id, Tango.CLOSED, 0, 0, null, null);
    }

    /**
     * Go navigated the page (Backend NewSurface and Surface.Navigate): like
     * the core, trust url's origin from now on instead of the previous one.
     */
    void navigate(String url) {
        if (closed) return;
        String o = originOf(url);
        if (!java.util.Objects.equals(o, navOrigin)) {
            navOrigin = o;
            if (listenerSupported) {
                // The rules apply to documents created from now on.
                WebViewCompat.removeWebMessageListener(web, CHANNEL);
                addListener(web);
            }
        }
        web.loadUrl(url);
    }

    void eval(String js) {
        if (!closed) web.evaluateJavascript(js, null);
    }

    WebView webView() {
        return web;
    }

    /**
     * Posts a down lane packet (an ArrayBuffer of at least 2 bytes) to the
     * page through the reply proxy of its accepted probe; false if there is
     * none (Go then drops the packet).
     */
    boolean laneSend(byte[] p) {
        if (closed || laneReply == null) return false;
        laneReply.postMessage(p);
        return true;
    }

    /**
     * Builds the WebResourceResponse for an intercepted request (request
     * thread). body is the whole body, or null when it is streamed from Go
     * through stream.
     */
    WebResourceResponse respond(int status, String reason, String mime, String encoding,
            String[] headers, byte[] body, long stream) {
        Map<String, String> h = new HashMap<>();
        for (int i = 0; i + 1 < headers.length; i += 2) h.put(headers[i], headers[i + 1]);
        InputStream in = body != null ? new java.io.ByteArrayInputStream(body) : new GoStream(stream);
        return new WebResourceResponse(mime, encoding, status, reason, h, in);
    }

    // ---- internals ----

    /** Detaches and destroys the WebView without reporting. */
    @Override
    void destroy() {
        if (closed) return;
        closed = true;
        laneReply = null;
        for (CanvasOverlay o : overlays) o.destroy();
        overlays.clear();
        ViewGroup parent = (ViewGroup) frame.getParent();
        if (parent != null) parent.removeView(frame);
        frame.removeView(web);
        web.stopLoading();
        removeChannel(web);
        web.destroy();
    }

    /** Adds the WebMessageListener for the trusted origins. */
    private void addListener(WebView w) {
        Set<String> rules = new LinkedHashSet<>();
        rules.add(ASSET_ORIGIN);
        if (navOrigin != null) rules.add(navOrigin);
        try {
            WebViewCompat.addWebMessageListener(w, CHANNEL, rules, new Listener());
        } catch (IllegalArgumentException e) {
            // A navigated origin the WebView rejects as a rule: the asset origin only.
            Tango.log("bridge origin rule refused: " + rules, e);
            WebViewCompat.addWebMessageListener(w, CHANNEL, java.util.Collections.singleton(ASSET_ORIGIN), new Listener());
        }
    }

    private static void removeChannel(WebView w) {
        if (listenerSupported) WebViewCompat.removeWebMessageListener(w, CHANNEL);
        else w.removeJavascriptInterface(CHANNEL);
    }

    /** scheme://host[:port] of an http(s) URL, lower case, else null (as originOf in Go). */
    static String originOf(String url) {
        if (url == null) return null;
        Uri u = Uri.parse(url);
        String scheme = u.getScheme(), host = u.getHost();
        if (scheme == null || host == null || host.isEmpty()) return null;
        scheme = scheme.toLowerCase(java.util.Locale.ROOT);
        if (!"http".equals(scheme) && !"https".equals(scheme)) return null;
        String o = scheme + "://" + host.toLowerCase(java.util.Locale.ROOT);
        return u.getPort() == -1 ? o : o + ":" + u.getPort();
    }

    /** Whether url's origin gets the bridge. Main thread. */
    private boolean trusted(String url) {
        String o = originOf(url);
        return o != null && (o.equals(ASSET_ORIGIN) || o.equals(navOrigin));
    }

    private void reportSize() {
        if (closed) return;
        float d = web.getResources().getDisplayMetrics().density;
        int w = Math.round(web.getWidth() / d), h = Math.round(web.getHeight() / d);
        if (w == widthDp && h == heightDp) return;
        widthDp = w;
        heightDp = h;
        Tango.nativeSurfaceEvent(id, Tango.RESIZED, w, h, null, null);
    }

    /**
     * WebView ignores Set-Cookie in intercepted responses, and a header map
     * holds one value per name: Go joins the Set-Cookie values with "\n"
     * (asset.go flatHeader); each goes to the cookie jar here, before the
     * page sees the response, so document.cookie and later requests see it.
     */
    private static void storeCookies(String url, WebResourceResponse resp) {
        if (resp == null || resp.getResponseHeaders() == null) return;
        String all = null;
        for (String k : resp.getResponseHeaders().keySet()) {
            if ("set-cookie".equalsIgnoreCase(k)) {
                all = resp.getResponseHeaders().remove(k);
                break;
            }
        }
        if (all == null) return;
        CookieManager cm = CookieManager.getInstance();
        for (String c : all.split("\n")) {
            if (!c.isEmpty()) cm.setCookie(url, c);
        }
        cm.flush();
    }

    static boolean isAsset(Uri u) {
        return u != null && Tango.ASSET_SCHEME.equals(u.getScheme()) && Tango.ASSET_HOST.equals(u.getHost())
                && u.getPort() == -1;
    }

    /**
     * window.__tangoAndroid with a WebMessageListener: in main frames and
     * sub-frames of the trusted origins only. Main thread.
     */
    private final class Listener implements WebViewCompat.WebMessageListener {
        @Override
        public void onPostMessage(WebView view, WebMessageCompat message, Uri sourceOrigin,
                boolean isMainFrame, JavaScriptReplyProxy reply) {
            if (closed || view != web || !isMainFrame || message == null || sourceOrigin == null) return;
            // Check the type first: getData() throws for an ArrayBuffer.
            if (message.getType() == WebMessageCompat.TYPE_ARRAY_BUFFER) {
                lanePacket(message.getArrayBuffer(), sourceOrigin, reply);
                return;
            }
            String data = message.getData();
            if (data == null) return;
            String r = Tango.nativeWebMessage(id, sourceOrigin.toString(), data);
            if (r != null) reply.postMessage(r);
        }
    }

    /**
     * An upstream lane message (internal/platform/android/lanes.go): Go
     * checks the token and origin and hands it to the core. A probe the core
     * accepted makes reply the down lane's proxy and is answered [0]; a
     * message the core refused is answered [1]; a bad token gets nothing.
     */
    private void lanePacket(byte[] b, Uri sourceOrigin, JavaScriptReplyProxy reply) {
        if (b == null) return;
        int r = Tango.nativeLanePacket(id, sourceOrigin.toString(), b);
        if (r == 1 && isProbe(b)) {
            laneReply = reply;
            reply.postMessage(new byte[] {0});
        } else if (r == 2) {
            reply.postMessage(new byte[] {1});
        }
    }

    /** A lane probe: exactly token[16] | gen[8] | 00 00. */
    private static boolean isProbe(byte[] b) {
        return b.length == 26 && b[24] == 0 && b[25] == 0;
    }

    /**
     * window.__tangoAndroid without WebMessageListener: a
     * JavascriptInterface, in every frame. Called on the WebView's
     * JavaBridge thread.
     */
    private final class Channel {
        /**
         * The origin is that of the document the token was issued to,
         * fixed here, when the message arrives, not when the main thread
         * gets to it (the page may have changed by then).
         */
        @JavascriptInterface
        public void post(String token, String message) {
            if (closed) return;
            String origin = Tango.nativeTokenOrigin(id, token);
            if (origin == null) return;
            Tango.main.post(() -> {
                if (!closed) Tango.nativeMessage(id, origin, message);
            });
        }

        /**
         * Stores the body of a request the page is about to send to the
         * asset origin, which shouldInterceptRequest cannot see (Go,
         * internal/platform/android/body.go). Synchronous: the body is
         * stored before the request leaves the page.
         */
        @JavascriptInterface
        public boolean body(String token, String bodyId, String contentType, String base64) {
            return !closed && Tango.nativeStashBody(id, token, bodyId, contentType, base64);
        }
    }

    private final class Client extends WebViewClient {
        @Override
        public WebResourceResponse shouldInterceptRequest(WebView view, WebResourceRequest req) {
            if (closed || !isAsset(req.getUrl())) return null;
            String url = req.getUrl().toString();
            Map<String, String> rh = new HashMap<>(req.getRequestHeaders());
            // Intercepted requests carry no cookies: add the cookie jar's.
            boolean hasCookie = false;
            for (String k : rh.keySet()) hasCookie |= "cookie".equalsIgnoreCase(k);
            if (!hasCookie) {
                String c = CookieManager.getInstance().getCookie(url);
                if (c != null && !c.isEmpty()) rh.put("Cookie", c);
            }
            String[] headers = new String[rh.size() * 2];
            int i = 0;
            for (Map.Entry<String, String> e : rh.entrySet()) {
                headers[i++] = e.getKey();
                headers[i++] = e.getValue();
            }
            WebResourceResponse resp = Tango.nativeServeAsset(Page.this, id, req.getMethod(), url,
                    headers, req.isForMainFrame());
            storeCookies(url, resp);
            return resp;
        }

        @Override
        public boolean shouldOverrideUrlLoading(WebView view, WebResourceRequest req) {
            return WebExt.shouldOverrideUrlLoading(Page.this, view, req);
        }

        @Override
        public void onReceivedHttpAuthRequest(WebView view, HttpAuthHandler handler, String host, String realm) {
            WebExt.onReceivedHttpAuthRequest(Page.this, view, handler, host, realm);
        }

        @Override
        public void onPageStarted(WebView view, String url, Bitmap favicon) {
            if (closed || view != web) return;
            lastUrl = url;
            Tango.nativeSurfaceEvent(id, Tango.NAVIGATING, 0, 0, url, null);
            // A new document: Go retires the previous token. A trusted
            // document Go did not serve with the script inside (the origin Go
            // navigated to, or a response Go could not rewrite) gets it now,
            // with a new token, unless it already has it (restored from
            // history). This may run after the page's own first scripts.
            // Other origins get nothing. The lane's reply proxy belongs to the
            // previous document, unless Go says this document's own probe
            // already came (it can reach the main thread before this).
            String[] script = new String[1];
            boolean keepLane = Tango.nativeBeginPage(id, url, script);
            if (!keepLane) laneReply = null;
            String js = script[0];
            if (js == null || !trusted(url)) return;
            view.evaluateJavascript("Object.prototype.hasOwnProperty.call(window, '__tangoNative')", has -> {
                if (!closed && view == web && !"true".equals(has)) view.evaluateJavascript(js, null);
            });
        }

        @Override
        public void onPageFinished(WebView view, String url) {
            if (!closed) Tango.nativeSurfaceEvent(id, Tango.LOADED, 0, 0, url, null);
        }

        @Override
        public void onReceivedError(WebView view, WebResourceRequest req, WebResourceError err) {
            if (!closed && req.isForMainFrame()) {
                Tango.nativeSurfaceEvent(id, Tango.LOAD_FAILED, 0, 0, req.getUrl().toString(),
                        String.valueOf(err.getDescription()));
            }
        }

        @Override
        public boolean onRenderProcessGone(WebView view, RenderProcessGoneDetail detail) {
            // Without this the whole app is killed with the renderer. The
            // WebView cannot be reused: replace it and reload.
            recover(view);
            return true;
        }
    }

    /** A response body Go is still writing. Read on the WebView's threads. */
    private static final class GoStream extends InputStream {
        private final long handle;
        private volatile boolean closed;

        GoStream(long handle) {
            this.handle = handle;
        }

        @Override
        public int read() throws IOException {
            byte[] b = new byte[1];
            int n = read(b, 0, 1);
            return n < 0 ? -1 : b[0] & 0xff;
        }

        @Override
        public int read(byte[] b, int off, int len) throws IOException {
            if (closed) return -1;
            if (len == 0) return 0;
            return Tango.nativeStreamRead(handle, b, off, len);
        }

        // Not synchronized with read: closing must wake a read blocked in Go.
        @Override
        public void close() {
            if (closed) return;
            closed = true;
            Tango.nativeStreamClose(handle);
        }
    }
}
