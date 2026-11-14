// Package asset serves a front end to Tango surfaces: static files from an
// fs.FS, or, in development builds (-tags tango_dev, which tan dev adds), a
// reverse proxy to a dev server (Vite).
package asset

import (
	"bytes"
	"errors"
	"io"
	"io/fs"
	"mime"
	"net/http"
	"net/http/httputil"
	"net/netip"
	"net/url"
	"os"
	"path"
	"strings"
	"time"
)

// DevURLEnv is the environment variable FromEnv checks in development
// builds (-tags tango_dev). Production builds, the default, ignore it.
const DevURLEnv = "TANGO_DEV_URL"

// Options configures FS.
type Options struct {
	_ [0]func()

	// Index is the file served for directory paths. Default "index.html".
	Index string
	// SPA serves the root index for unknown paths whose last segment has no
	// file extension (client-side routes such as /settings/profile).
	SPA bool
	// Headers are added to every successful response (e.g. a CSP).
	Headers http.Header
}

// FS returns a handler serving files from fsys. Use fs.Sub to serve a
// subdirectory of an embed.FS. Directory listings are never produced, and
// paths that try to escape fsys are rejected.
func FS(fsys fs.FS, opts Options) http.Handler {
	if opts.Index == "" {
		opts.Index = "index.html"
	}
	return &fileHandler{fsys: fsys, opts: opts}
}

type fileHandler struct {
	fsys fs.FS
	opts Options
}

func (h *fileHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		w.Header().Set("Allow", "GET, HEAD")
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	name, ok := cleanPath(r.URL.Path)
	if !ok {
		http.Error(w, "bad path", http.StatusBadRequest)
		return
	}
	if name == "" || strings.HasSuffix(r.URL.Path, "/") {
		name = path.Join(name, h.opts.Index)
	}
	if h.serve(w, r, name) {
		return
	}
	if h.opts.SPA && path.Ext(name) == "" {
		if h.serve(w, r, h.opts.Index) {
			return
		}
	}
	http.NotFound(w, r)
}

// cleanPath maps a URL path to an fs.FS name ("" for the root).
func cleanPath(p string) (string, bool) {
	if strings.ContainsAny(p, "\\\x00") {
		return "", false
	}
	for _, seg := range strings.Split(p, "/") {
		if seg == ".." {
			return "", false
		}
	}
	name := strings.TrimPrefix(path.Clean("/"+p), "/")
	if name == "" {
		return "", true
	}
	return name, fs.ValidPath(name)
}

// serve writes name if it is a regular file and reports whether it did.
// Directories are served through their index file.
func (h *fileHandler) serve(w http.ResponseWriter, r *http.Request, name string) bool {
	f, err := h.fsys.Open(name)
	if err != nil {
		return false
	}
	st, err := f.Stat()
	if err != nil {
		f.Close()
		return false
	}
	if st.IsDir() {
		f.Close()
		return h.serveIndexOf(w, r, name)
	}
	defer f.Close()
	if !st.Mode().IsRegular() {
		return false
	}
	var rs io.ReadSeeker
	if s, ok := f.(io.ReadSeeker); ok {
		rs = s
	} else {
		b, err := io.ReadAll(f)
		if err != nil {
			http.Error(w, "read error", http.StatusInternalServerError)
			return true
		}
		rs = bytes.NewReader(b)
	}
	hd := w.Header()
	for k, v := range h.opts.Headers {
		hd[k] = append([]string(nil), v...)
	}
	if ct := ContentType(name); ct != "" {
		hd.Set("Content-Type", ct)
	}
	hd.Set("X-Content-Type-Options", "nosniff")
	modTime := st.ModTime()
	if modTime.IsZero() || modTime.Equal(time.Unix(0, 0)) {
		hd.Set("Cache-Control", "no-cache")
	}
	// ServeContent sniffs when Content-Type is unset and handles Range,
	// If-Modified-Since and HEAD.
	http.ServeContent(w, r, name, modTime, rs)
	return true
}

func (h *fileHandler) serveIndexOf(w http.ResponseWriter, r *http.Request, dir string) bool {
	idx := path.Join(dir, h.opts.Index)
	f, err := h.fsys.Open(idx)
	if err != nil {
		return false
	}
	st, err := f.Stat()
	f.Close()
	if err != nil || st.IsDir() {
		return false
	}
	return h.serve(w, r, idx)
}

// types fills gaps and fixes OS-dependent entries of the mime database.
var types = map[string]string{
	".html":        "text/html; charset=utf-8",
	".htm":         "text/html; charset=utf-8",
	".js":          "text/javascript; charset=utf-8",
	".mjs":         "text/javascript; charset=utf-8",
	".cjs":         "text/javascript; charset=utf-8",
	".css":         "text/css; charset=utf-8",
	".json":        "application/json",
	".map":         "application/json",
	".webmanifest": "application/manifest+json",
	".wasm":        "application/wasm",
	".svg":         "image/svg+xml",
	".png":         "image/png",
	".jpg":         "image/jpeg",
	".jpeg":        "image/jpeg",
	".gif":         "image/gif",
	".webp":        "image/webp",
	".avif":        "image/avif",
	".ico":         "image/x-icon",
	".woff":        "font/woff",
	".woff2":       "font/woff2",
	".ttf":         "font/ttf",
	".otf":         "font/otf",
	".txt":         "text/plain; charset=utf-8",
	".xml":         "application/xml",
	".pdf":         "application/pdf",
	".mp4":         "video/mp4",
	".webm":        "video/webm",
	".mp3":         "audio/mpeg",
	".wav":         "audio/wav",
}

// ContentType returns the media type for a file name, or "" if unknown
// (the caller then sniffs the content).
func ContentType(name string) string {
	ext := strings.ToLower(path.Ext(name))
	if t, ok := types[ext]; ok {
		return t
	}
	return mime.TypeByExtension(ext)
}

// DevProxyOptions configures DevProxyWith.
type DevProxyOptions struct {
	_ [0]func()

	// AllowRemote lets the target be any host. By default only loopback
	// targets are accepted: localhost, 127.0.0.0/8 and ::1. (Android's adb
	// reverse and the iOS Simulator both reach the dev machine through
	// localhost.)
	AllowRemote bool
}

// DevProxy returns a reverse proxy to a dev server such as Vite
// (e.g. "http://localhost:5173"). WebSocket upgrades (HMR) are forwarded.
// The Host header is rewritten to the target so dev-server host checks pass.
// The target must be a loopback address (DevProxyWith can allow remote
// targets); a rejected target answers every request with 502.
func DevProxy(target string) http.Handler {
	return DevProxyWith(target, DevProxyOptions{})
}

// DevProxyWith is DevProxy with options. The zero DevProxyOptions behaves
// like DevProxy.
func DevProxyWith(target string, o DevProxyOptions) http.Handler {
	u, err := url.Parse(target)
	if err == nil && (u.Scheme != "http" && u.Scheme != "https" || u.Host == "") {
		err = errors.New("target must be an absolute http(s) URL")
	}
	if err == nil && !o.AllowRemote && !isLoopback(u.Hostname()) {
		err = errors.New("target is not a loopback address (localhost, 127.0.0.0/8, ::1); see DevProxyOptions.AllowRemote")
	}
	if err != nil {
		msg := "tango: bad dev server URL " + target + ": " + err.Error()
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			http.Error(w, msg, http.StatusBadGateway)
		})
	}
	return &httputil.ReverseProxy{
		Rewrite: func(pr *httputil.ProxyRequest) {
			pr.SetURL(u)
			pr.Out.Host = u.Host
			// Requests from custom schemes (tango://app) carry an Origin the
			// dev server would reject on WebSocket upgrades.
			if o := pr.In.Header.Get("Origin"); o != "" && !strings.HasPrefix(o, "http") {
				pr.Out.Header.Set("Origin", u.Scheme+"://"+u.Host)
			}
		},
		ErrorHandler: func(w http.ResponseWriter, r *http.Request, err error) {
			http.Error(w, "tango: dev server unreachable at "+target+": "+err.Error(), http.StatusBadGateway)
		},
	}
}

// isLoopback reports whether host (a host name or IP literal, without
// port) names this machine: "localhost" (or a name under .localhost),
// 127.0.0.0/8 or ::1. Names are not resolved.
func isLoopback(host string) bool {
	host = strings.TrimSuffix(strings.ToLower(strings.Trim(host, "[]")), ".")
	if host == "localhost" || strings.HasSuffix(host, ".localhost") {
		return true
	}
	if i := strings.IndexByte(host, '%'); i >= 0 { // zone: fe80::1%lo0
		host = host[:i]
	}
	ip, err := netip.ParseAddr(host)
	return err == nil && ip.Unmap().IsLoopback()
}

// FromEnv returns FS(fsys, opts), or, in development builds (-tags
// tango_dev, as tan dev builds) with $TANGO_DEV_URL set, DevProxy to that
// URL, which must be a loopback address. Production builds, the default,
// never read the variable. opts is optional; at most one is used.
func FromEnv(fsys fs.FS, opts ...Options) http.Handler {
	if devBuild {
		if dev := os.Getenv(DevURLEnv); dev != "" {
			return DevProxy(dev)
		}
	}
	var o Options
	if len(opts) > 0 {
		o = opts[0]
	}
	return FS(fsys, o)
}
