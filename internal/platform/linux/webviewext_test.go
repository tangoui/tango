package linux

import (
	"net/http"
	"reflect"
	"testing"
	"time"

	"github.com/tangoui/tango/internal/platform"
)

func TestPermissionKinds(t *testing.T) {
	cases := []struct {
		name                  string
		audio, video, display bool
		want                  []platform.WebPermission
	}{
		{"WebKitUserMediaPermissionRequest", true, true, false, []platform.WebPermission{platform.WebPermCamera, platform.WebPermMicrophone}},
		{"WebKitUserMediaPermissionRequest", true, false, false, []platform.WebPermission{platform.WebPermMicrophone}},
		{"WebKitUserMediaPermissionRequest", false, true, true, []platform.WebPermission{platform.WebPermScreen}},
		{"WebKitUserMediaPermissionRequest", false, false, false, []platform.WebPermission{platform.WebPermOther}},
		{"WebKitGeolocationPermissionRequest", false, false, false, []platform.WebPermission{platform.WebPermGeolocation}},
		{"WebKitNotificationPermissionRequest", false, false, false, []platform.WebPermission{platform.WebPermNotifications}},
		{"WebKitClipboardPermissionRequest", false, false, false, []platform.WebPermission{platform.WebPermClipboardRead}},
		{"WebKitMediaKeySystemPermissionRequest", false, false, false, []platform.WebPermission{platform.WebPermMedia}},
		{"WebKitWebsiteDataAccessPermissionRequest", false, false, false, []platform.WebPermission{platform.WebPermStorage}},
		{"WebKitDeviceInfoPermissionRequest", false, false, false, []platform.WebPermission{platform.WebPermOther}},
		{"WebKitPointerLockPermissionRequest", false, false, false, []platform.WebPermission{platform.WebPermOther}},
	}
	for _, c := range cases {
		if got := permissionKinds(c.name, c.audio, c.video, c.display); !reflect.DeepEqual(got, c.want) {
			t.Errorf("%s a=%v v=%v d=%v: got %v want %v", c.name, c.audio, c.video, c.display, got, c.want)
		}
	}
}

func TestPermissionPrompt(t *testing.T) {
	got := permissionPrompt("https://a.example", []platform.WebPermission{platform.WebPermCamera, platform.WebPermMicrophone})
	if got != "https://a.example wants to use your camera and your microphone." {
		t.Errorf("got %q", got)
	}
	if got := permissionPrompt("", []platform.WebPermission{platform.WebPermOther}); got != "This page wants to use a restricted feature." {
		t.Errorf("got %q", got)
	}
}

func TestAuthSchemeName(t *testing.T) {
	for in, want := range map[int32]string{2: "basic", 3: "digest", 5: "ntlm", 6: "negotiate", 1: "", 4: "", 100: ""} {
		if got, ok := authSchemeName(in); !ok || got != want {
			t.Errorf("%d: got %q %v", in, got, ok)
		}
	}
	for _, in := range []int32{7, 8, 9} {
		if _, ok := authSchemeName(in); ok {
			t.Errorf("%d should keep the platform default", in)
		}
	}
}

func TestShouldDownload(t *testing.T) {
	cases := []struct {
		mime bool
		disp string
		main bool
		want bool
	}{
		{false, "", false, true},
		{true, "", true, false},
		{true, `attachment; filename="a.txt"`, true, true},
		{true, "Attachment", true, true},
		{true, "attachment", false, false},
		{true, `inline; filename="attachment.txt"`, true, false},
		{true, "attachments", true, false},
	}
	for _, c := range cases {
		if got := shouldDownload(c.mime, c.disp, c.main); got != c.want {
			t.Errorf("%+v: got %v", c, got)
		}
	}
}

func TestWebsiteDataTypes(t *testing.T) {
	if got := websiteDataTypes(platform.DataCookies); got != 1<<8 {
		t.Errorf("cookies: %#x", got)
	}
	if got := websiteDataTypes(platform.DataCache); got != 3 {
		t.Errorf("cache: %#x", got)
	}
	st := websiteDataTypes(platform.DataStorage)
	if st&(1<<4) == 0 || st&(1<<6) == 0 || st&(1<<12) == 0 || st&(1<<8) != 0 || st&3 != 0 {
		t.Errorf("storage: %#x", st)
	}
	if websiteDataTypes(0) != 0 {
		t.Error("none")
	}
}

func TestFindOptions(t *testing.T) {
	if got := findOptions(platform.FindOptions{}); got != wkFindCaseInsensitive|wkFindWrapAround {
		t.Errorf("default: %#x", got)
	}
	if got := findOptions(platform.FindOptions{MatchCase: true, Backward: true, NoWrap: true}); got != wkFindBackwards {
		t.Errorf("all: %#x", got)
	}
	if findMatches(3) != 3 || findMatches(^uint32(0)) != -1 || findMatches(findMaxMatches+1) != -1 {
		t.Error("findMatches")
	}
}

func TestCookieToFields(t *testing.T) {
	now := time.Unix(1_000_000, 0)
	f := cookieToFields(&http.Cookie{Name: "a", Value: "b", Domain: ".ex.com", Secure: true, HttpOnly: true, SameSite: http.SameSiteStrictMode}, now)
	want := cookieFields{Name: "a", Value: "b", Domain: ".ex.com", Path: "/", Session: true, Secure: true, HTTPOnly: true, SameSite: soupSameSiteStrict}
	if f != want {
		t.Errorf("session: %+v", f)
	}
	f = cookieToFields(&http.Cookie{Name: "a", Domain: "ex.com", Path: "/p", MaxAge: 60, Expires: now.Add(time.Hour)}, now)
	if f.Session || f.Expires != now.Unix()+60 || f.Path != "/p" || f.SameSite != -1 || f.Domain != "ex.com" {
		t.Errorf("max-age: %+v", f)
	}
	f = cookieToFields(&http.Cookie{Name: "a", Domain: "ex.com", Expires: now.Add(time.Hour)}, now)
	if f.Session || f.Expires != now.Unix()+3600 {
		t.Errorf("expires: %+v", f)
	}
	f = cookieToFields(&http.Cookie{Name: "a", Domain: "ex.com", MaxAge: -1}, now)
	if f.Session || f.Expires >= now.Unix() {
		t.Errorf("delete: %+v", f)
	}
	f = cookieToFields(&http.Cookie{Name: "a", Domain: "ex.com", SameSite: http.SameSiteNoneMode}, now)
	if f.SameSite != soupSameSiteNone {
		t.Errorf("none: %+v", f)
	}
}

func TestHTTPCookie(t *testing.T) {
	c := cookieFields{Name: "a", Value: "b", Domain: "ex.com", Path: "/", Expires: 1000, Secure: true, SameSite: soupSameSiteLax}.httpCookie()
	if c.Name != "a" || c.Value != "b" || c.Domain != "ex.com" || !c.Secure || c.HttpOnly ||
		!c.Expires.Equal(time.Unix(1000, 0)) || c.SameSite != http.SameSiteLaxMode {
		t.Errorf("got %+v", c)
	}
	c = cookieFields{Name: "a", Session: true, SameSite: soupSameSiteNone, HTTPOnly: true}.httpCookie()
	if !c.Expires.IsZero() || c.SameSite != http.SameSiteNoneMode || !c.HttpOnly {
		t.Errorf("session: %+v", c)
	}
	// round trip
	in := &http.Cookie{Name: "n", Value: "v", Domain: ".ex.com", Path: "/x", Expires: time.Unix(5000, 0).UTC(), Secure: true, SameSite: http.SameSiteStrictMode}
	out := cookieToFields(in, time.Unix(0, 0)).httpCookie()
	if !reflect.DeepEqual(in, out) {
		t.Errorf("round trip: %+v != %+v", out, in)
	}
}

func TestCookieMatching(t *testing.T) {
	for _, c := range []struct {
		cd, d string
		want  bool
	}{
		{".ex.com", "ex.com", true}, {"a.ex.com", "ex.com", true}, {"ex.com", ".EX.com", true},
		{"badex.com", "ex.com", false}, {"ex.com", "a.ex.com", false}, {"x.org", "", true},
	} {
		if got := cookieDomainMatch(c.cd, c.d); got != c.want {
			t.Errorf("%q in %q: %v", c.cd, c.d, got)
		}
	}
	f := cookieFields{Name: "a", Domain: ".Ex.com", Path: "/"}
	if !sameCookie(f, &http.Cookie{Name: "a", Domain: ".ex.com"}) {
		t.Error("same")
	}
	if sameCookie(f, &http.Cookie{Name: "a", Domain: "ex.com"}) || sameCookie(f, &http.Cookie{Name: "a", Domain: ".ex.com", Path: "/p"}) {
		t.Error("different")
	}
	if got := cookieLookupURL(&http.Cookie{Domain: ".ex.com", Path: "/a b"}); got != "https://ex.com/a%20b" {
		t.Errorf("lookup: %q", got)
	}
}

func TestPDFPaperAndDownload(t *testing.T) {
	if _, _, ok := pdfPaper(platform.PDFOptions{PageWidth: 8.5}); ok {
		t.Error("one dimension")
	}
	if w, h, ok := pdfPaper(platform.PDFOptions{PageWidth: 8.5, PageHeight: 11}); !ok || w != 8.5 || h != 11 {
		t.Error("custom")
	}
	if downloadSize(0) != -1 || downloadSize(10) != 10 {
		t.Error("size")
	}
	if downloadFailState(400, true) != platform.DownloadCanceled || downloadFailState(400, false) != platform.DownloadFailed ||
		downloadFailState(499, true) != platform.DownloadFailed {
		t.Error("fail state")
	}
}
