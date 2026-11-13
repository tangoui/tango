package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"sort"
	"strings"
	"time"
)

// HTTP endpoints the capability demo calls with fetch/XMLHttpRequest, to
// check that request bodies and cookies reach Go through the asset handler
// (Android WebView does not expose request bodies to shouldInterceptRequest;
// the backend recovers them, see docs/mobile.md).
//
//	POST /api/echo     → method, content type, body size and SHA-256; form fields and files for forms
//	GET  /api/cookies  → sets two cookies with two Set-Cookie headers
//	GET  /api/whoami   → the Cookie header the request carried
func apiHandler(next http.Handler) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/echo", func(w http.ResponseWriter, r *http.Request) {
		out := map[string]any{"method": r.Method, "contentType": r.Header.Get("Content-Type")}
		ct := r.Header.Get("Content-Type")
		switch {
		case strings.HasPrefix(ct, "multipart/form-data"):
			if err := r.ParseMultipartForm(32 << 20); err != nil {
				out["error"] = err.Error()
				break
			}
			fields := map[string]string{}
			for k, v := range r.MultipartForm.Value {
				fields[k] = strings.Join(v, ",")
			}
			files := map[string]any{}
			for k, fhs := range r.MultipartForm.File {
				for _, fh := range fhs {
					f, err := fh.Open()
					if err != nil {
						continue
					}
					b, _ := io.ReadAll(f)
					f.Close()
					sum := sha256.Sum256(b)
					files[k] = map[string]any{"name": fh.Filename, "size": len(b), "type": fh.Header.Get("Content-Type"), "sha256": hex.EncodeToString(sum[:8])}
				}
			}
			out["fields"], out["files"] = fields, files
		case strings.HasPrefix(ct, "application/x-www-form-urlencoded"):
			r.ParseForm()
			out["fields"] = r.PostForm
		default:
			b, _ := io.ReadAll(r.Body)
			sum := sha256.Sum256(b)
			out["size"], out["sha256"] = len(b), hex.EncodeToString(sum[:8])
			if len(b) <= 200 {
				out["body"] = string(b)
			}
		}
		log.Printf("api: %s /api/echo %v", r.Method, out)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(out)
	})
	mux.HandleFunc("/api/cookies", func(w http.ResponseWriter, r *http.Request) {
		stamp := time.Now().Format("150405")
		http.SetCookie(w, &http.Cookie{Name: "tango_a", Value: "alpha-" + stamp, Path: "/"})
		http.SetCookie(w, &http.Cookie{Name: "tango_b", Value: "beta-" + stamp, Path: "/", HttpOnly: true})
		w.Header().Add("X-Tango-Multi", "one")
		w.Header().Add("X-Tango-Multi", "two")
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"set": []string{"tango_a", "tango_b (HttpOnly)"}, "setCookieHeaders": len(w.Header()["Set-Cookie"])})
	})
	mux.HandleFunc("/api/whoami", func(w http.ResponseWriter, r *http.Request) {
		var names []string
		for _, c := range r.Cookies() {
			names = append(names, c.Name+"="+c.Value)
		}
		sort.Strings(names)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"cookies": names})
	})
	mux.Handle("/", next)
	return mux
}
