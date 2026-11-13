package windows

import (
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/tangoui/tango/internal/platform"
)

func TestWebPermission(t *testing.T) {
	want := map[int32]platform.WebPermission{
		wvPermUnknown:       platform.WebPermOther,
		wvPermMicrophone:    platform.WebPermMicrophone,
		wvPermCamera:        platform.WebPermCamera,
		wvPermGeolocation:   platform.WebPermGeolocation,
		wvPermNotifications: platform.WebPermNotifications,
		wvPermOtherSensors:  platform.WebPermMotion,
		wvPermClipboardRead: platform.WebPermClipboardRead,
		wvPermMIDISysex:     platform.WebPermMIDI,
		wvPermAutoplay:      platform.WebPermOther,
		wvPermLocalFonts:    platform.WebPermOther,
		99:                  platform.WebPermOther,
	}
	for k, w := range want {
		if got := webPermission(k); got != w {
			t.Errorf("webPermission(%d) = %q, want %q", k, got, w)
		}
	}
}

func TestPermissionState(t *testing.T) {
	cases := map[platform.PermissionDecision]int32{
		platform.PermissionDefault: wvPermStateDefault,
		platform.PermissionPrompt:  wvPermStateDefault,
		platform.PermissionAllow:   wvPermStateAllow,
		platform.PermissionDeny:    wvPermStateDeny,
	}
	for d, w := range cases {
		if got := permissionState(d); got != w {
			t.Errorf("permissionState(%d) = %d, want %d", d, got, w)
		}
	}
}

func TestRequestOriginAndNewWindowURL(t *testing.T) {
	if got := requestOrigin("https://Example.com:8443/a?b"); got != "https://example.com:8443" {
		t.Errorf("origin = %q", got)
	}
	if got := requestOrigin("about:blank"); got != "" {
		t.Errorf("origin of about:blank = %q", got)
	}
	if newWindowURL("about:blank") != "" || newWindowURL("https://x/") != "https://x/" {
		t.Error("newWindowURL")
	}
}

func TestParseAuthChallenge(t *testing.T) {
	cases := []struct{ in, scheme, realm string }{
		{`Basic realm="Staging"`, "basic", "Staging"},
		{`Basic realm="a \"quoted\" realm", charset="UTF-8"`, "basic", `a "quoted" realm`},
		{`basic charset="UTF-8", realm="R"`, "basic", "R"},
		{`Digest realm=plain, nonce="x"`, "digest", "plain"},
		{`NTLM`, "ntlm", ""},
		{`  Negotiate  `, "negotiate", ""},
		{``, "", ""},
	}
	for _, c := range cases {
		s, r := parseAuthChallenge(c.in)
		if s != c.scheme || r != c.realm {
			t.Errorf("parseAuthChallenge(%q) = %q, %q; want %q, %q", c.in, s, r, c.scheme, c.realm)
		}
	}
}

func TestAuthChallenge(t *testing.T) {
	ch := authChallenge("https://user.example.com/private", `Basic realm="R"`)
	if ch.Host != "user.example.com" || ch.Port != 443 || ch.Scheme != "basic" || ch.Realm != "R" || ch.URL == "" {
		t.Errorf("got %+v", ch)
	}
	ch = authChallenge("http://h:8080/", "")
	if ch.Port != 8080 || ch.Scheme != "basic" {
		t.Errorf("got %+v", ch)
	}
	ch = authChallenge("http://h/", "Bearer realm=x")
	if ch.Port != 80 || ch.Scheme != "" || ch.Realm != "x" {
		t.Errorf("got %+v", ch)
	}
}

func TestBrowsingDataKinds(t *testing.T) {
	if got := browsingDataKinds(platform.DataAll); got != wvDataCookies|wvDataDiskCache|wvDataAllDOMStorage {
		t.Errorf("all = %d", got)
	}
	if got := browsingDataKinds(platform.DataCookies); got != 64 {
		t.Errorf("cookies = %d", got)
	}
	if got := browsingDataKinds(0); got != 0 {
		t.Errorf("none = %d", got)
	}
}

func TestDownloadState(t *testing.T) {
	cases := []struct {
		state, reason int32
		want          platform.DownloadState
		end           bool
	}{
		{wvDownloadInProgress, 0, platform.DownloadProgress, false},
		{wvDownloadCompleted, 0, platform.DownloadDone, true},
		{wvDownloadInterrupted, wvInterruptUserCanceled, platform.DownloadCanceled, true},
		{wvDownloadInterrupted, wvInterruptUserShutdown, platform.DownloadCanceled, true},
		{wvDownloadInterrupted, wvInterruptUserPaused, platform.DownloadProgress, false},
		{wvDownloadInterrupted, 12, platform.DownloadFailed, true},
	}
	for _, c := range cases {
		got, end := downloadState(c.state, c.reason)
		if got != c.want || end != c.end {
			t.Errorf("downloadState(%d, %d) = %d, %v", c.state, c.reason, got, end)
		}
	}
	if downloadError(12) != "download interrupted: network failed" || downloadError(77) == "" {
		t.Error("downloadError")
	}
}

func TestDownloadFilename(t *testing.T) {
	cases := []struct{ cd, path, uri, want string }{
		{`attachment; filename="report.pdf"`, `C:\Users\u\Downloads\x.pdf`, "https://h/a", "report.pdf"},
		{`attachment; filename*=UTF-8''%E6%8A%A5%E5%91%8A.txt`, "", "", "报告.txt"},
		{`attachment; filename="..\..\evil.exe"`, "", "", "evil.exe"},
		{"", `C:\Users\u\Downloads\file (1).zip`, "https://h/file.zip", "file (1).zip"},
		{"", "", "https://h/dir/my%20file.bin?x=1", "my file.bin"},
		{"", "", "https://h/", ""},
	}
	for _, c := range cases {
		if got := downloadFilename(c.cd, c.path, c.uri); got != c.want {
			t.Errorf("downloadFilename(%q, %q, %q) = %q, want %q", c.cd, c.path, c.uri, got, c.want)
		}
	}
}

func TestSameSite(t *testing.T) {
	for _, m := range []http.SameSite{http.SameSiteNoneMode, http.SameSiteLaxMode, http.SameSiteStrictMode} {
		v, ok := sameSiteToWV(m)
		if !ok || sameSiteFromWV(v) != m {
			t.Errorf("round trip of %d", m)
		}
	}
	if _, ok := sameSiteToWV(http.SameSiteDefaultMode); ok {
		t.Error("default should keep the runtime's")
	}
}

func TestCookieExpiry(t *testing.T) {
	now := time.Unix(1000, 0)
	if e, del := cookieExpiry(&http.Cookie{MaxAge: -1}, now); !del || !e.IsZero() {
		t.Error("MaxAge < 0 deletes")
	}
	if e, _ := cookieExpiry(&http.Cookie{MaxAge: 60}, now); !e.Equal(now.Add(time.Minute)) {
		t.Errorf("MaxAge 60 = %v", e)
	}
	exp := time.Unix(5000, 0)
	if e, _ := cookieExpiry(&http.Cookie{Expires: exp}, now); !e.Equal(exp) {
		t.Errorf("Expires = %v", e)
	}
	if e, _ := cookieExpiry(&http.Cookie{}, now); !e.IsZero() {
		t.Error("session cookie")
	}
	ts := time.Unix(1700000000, 500000000)
	if got := fromUnixSeconds(unixSeconds(ts)); got.Sub(ts).Abs() > time.Millisecond {
		t.Errorf("seconds round trip: %v", got)
	}
}

func TestCookieInDomain(t *testing.T) {
	cases := []struct {
		c, d string
		want bool
	}{
		{".example.com", "example.com", true},
		{"www.example.com", "example.com", true},
		{"example.com", ".Example.com", true},
		{"badexample.com", "example.com", false},
		{"example.org", "example.com", false},
	}
	for _, c := range cases {
		if got := cookieInDomain(c.c, c.d); got != c.want {
			t.Errorf("cookieInDomain(%q, %q) = %v", c.c, c.d, got)
		}
	}
}

func TestCDPSetCookieParams(t *testing.T) {
	var p map[string]any
	exp := time.Unix(2000000000, 0)
	_ = json.Unmarshal([]byte(cdpSetCookieParams(&http.Cookie{Name: "a", Value: "b", Domain: ".ex.com", Secure: true, SameSite: http.SameSiteStrictMode}, exp)), &p)
	if p["domain"] != ".ex.com" || p["path"] != "/" || p["sameSite"] != "Strict" || p["expires"] != float64(2000000000) || p["url"] != nil {
		t.Errorf("domain cookie params: %v", p)
	}
	p = nil
	_ = json.Unmarshal([]byte(cdpSetCookieParams(&http.Cookie{Name: "a", Domain: "ex.com", Path: "/x"}, time.Time{})), &p)
	if p["url"] != "http://ex.com/x" || p["domain"] != nil || p["expires"] != nil || p["sameSite"] != nil {
		t.Errorf("host-only cookie params: %v", p)
	}
}

func TestIsAssetURI(t *testing.T) {
	yes := []string{assetOrigin, assetOrigin + "/", assetOrigin + "/index.html", "HTTPS://TANGO.LOCALHOST/a", assetOrigin + "?q"}
	no := []string{"https://tango.localhost.evil.com/", "https://example.com/", "http://tango.localhost/", ""}
	for _, u := range yes {
		if !isAssetURI(u) {
			t.Errorf("isAssetURI(%q) = false", u)
		}
	}
	for _, u := range no {
		if isAssetURI(u) {
			t.Errorf("isAssetURI(%q) = true", u)
		}
	}
}

func TestHeaderValues(t *testing.T) {
	got := headerValues(http.Header{"x-a": {"1", "2"}, "": {"x"}, "X-Empty": nil, "X-Inj": {"a\r\nB: c"}})
	m := map[string]string{}
	for _, kv := range got {
		m[kv[0]] = kv[1]
	}
	if len(m) != 2 || m["X-A"] != "1, 2" || m["X-Inj"] != "a  B: c" {
		t.Errorf("headerValues = %v", got)
	}
}

func TestFindResult(t *testing.T) {
	if r := findResult(0, -1); r != (platform.FindResult{}) {
		t.Errorf("no match: %+v", r)
	}
	if r := findResult(3, -1); r != (platform.FindResult{Matches: 3, Active: 1, Found: true}) {
		t.Errorf("started: %+v", r)
	}
	if r := findResult(3, 2); r != (platform.FindResult{Matches: 3, Active: 2, Found: true}) {
		t.Errorf("active: %+v", r)
	}
}

func TestNextFindIndex(t *testing.T) {
	cases := []struct {
		count, active    int
		backward, noWrap bool
		next             int
		move             bool
	}{
		{0, 0, false, false, 0, false},
		{3, 1, false, false, 2, true},
		{3, 3, false, false, 1, true},
		{3, 3, false, true, 3, false},
		{3, 1, true, false, 3, true},
		{3, 1, true, true, 1, false},
		{3, 2, true, false, 1, true},
		{3, -1, true, false, 3, true},
		{3, -1, false, false, 1, true},
	}
	for _, c := range cases {
		n, m := nextFindIndex(c.count, c.active, c.backward, c.noWrap)
		if n != c.next || m != c.move {
			t.Errorf("nextFindIndex(%d, %d, %v, %v) = %d, %v", c.count, c.active, c.backward, c.noWrap, n, m)
		}
	}
}
