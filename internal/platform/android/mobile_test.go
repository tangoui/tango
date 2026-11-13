package android

import (
	"errors"
	"testing"

	"github.com/tangoui/tango/internal/platform"
)

func TestJavaErr(t *testing.T) {
	if javaErr("") != nil {
		t.Fatal("empty")
	}
	for s, want := range map[string]error{
		"!canceled":                        platform.ErrCanceled,
		"!notConfigured AndroidManifest x": platform.ErrNotConfigured,
		"!permissionDenied":                platform.ErrPermissionDenied,
		"!notFound":                        platform.ErrNotFound,
		"!lockedOut too many":              platform.ErrLockedOut,
	} {
		if err := javaErr(s); !errors.Is(err, want) {
			t.Errorf("%q: %v", s, err)
		}
	}
	if err := javaErr("!boom it broke"); err == nil || err.Error() != "tango/android: boom it broke" {
		t.Fatal(err)
	}
	if v, err := splitResult("abc"); v != "abc" || err != nil {
		t.Fatal(v, err)
	}
	if _, err := splitResult("!notAvailable"); !errors.Is(err, platform.ErrNotAvailable) {
		t.Fatal(err)
	}
}

func TestDecode(t *testing.T) {
	l, err := decodeLocation(`{"lat":1.5,"lon":-2,"acc":10,"speed":-1,"heading":-1,"time":1700000000000}`)
	if err != nil || l.Latitude != 1.5 || l.Longitude != -2 || l.Accuracy != 10 || l.Time.UnixMilli() != 1700000000000 {
		t.Fatal(l, err)
	}
	ps, err := decodePaths(`["/a","/b"]`)
	if err != nil || len(ps) != 2 {
		t.Fatal(ps, err)
	}
	var q eventQueue
	q.add(platform.AppEvent{Kind: platform.AppOpenURL, URL: "x"})
	if got := q.take(); len(got) != 1 || len(q.take()) != 0 {
		t.Fatal(got)
	}
}

func TestDecodeBackground(t *testing.T) {
	ts := decodeBackground(`[{"id":3,"kind":1,"deadline":1700000000000,"expired":false},{"id":2,"kind":2,"expired":true}]`)
	if len(ts) != 2 || ts[0].ID != 3 || ts[0].Kind != platform.BackgroundRefresh || ts[0].Deadline.UnixMilli() != 1700000000000 ||
		!ts[1].Expired || !ts[1].Deadline.IsZero() {
		t.Fatalf("%+v", ts)
	}
	if decodeBackground("bad") != nil {
		t.Fatal("bad input decoded")
	}
}
