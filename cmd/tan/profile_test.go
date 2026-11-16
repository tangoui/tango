package main

import (
	"os"
	"strings"
	"testing"
)

func TestParseFeatures(t *testing.T) {
	for _, tc := range []struct{ input, want string }{
		{"", "all"}, {"all", "all"}, {"webview", "webview"}, {"canvas", "canvas"}, {"webview,canvas", "webview,canvas"}, {"canvas,webview", "webview,canvas"},
	} {
		got, err := parseFeatures(tc.input)
		if err != nil || got.String() != tc.want {
			t.Errorf("parseFeatures(%q) = %v, %v; want %s", tc.input, got, err, tc.want)
		}
	}
	for _, input := range []string{" ", "webview,webview", "canvas,canvas", "webview,all", "", "gpu", "webview,"} {
		if strings.TrimSpace(input) == "" {
			continue
		}
		if _, err := parseFeatures(input); err == nil {
			t.Errorf("parseFeatures(%q) accepted invalid input", input)
		}
	}
}

func TestConfigFeatures(t *testing.T) {
	for _, tc := range []struct {
		got  []string
		want string
	}{
		{nil, "all"}, {[]string{"webview"}, "webview"}, {[]string{"canvas", "webview"}, "webview,canvas"}, {[]string{"webview", "canvas"}, "webview,canvas"},
	} {
		f, err := configFeatures(tc.got)
		if err != nil || f.String() != tc.want {
			t.Errorf("configFeatures(%v)=%v,%v want %s", tc.got, f, err, tc.want)
		}
	}
	for _, got := range [][]string{{}, {"all"}, {"webview", "webview"}, {"nope"}} {
		if _, err := configFeatures(got); err == nil {
			t.Errorf("configFeatures(%v) accepted invalid", got)
		}
	}
}

func TestFeatureTagsAndPlatformRejection(t *testing.T) {
	cases := []struct {
		f    buildFeatures
		os   string
		tags string
		want []string
	}{
		{featureAll, "darwin", "", nil},
		{featureWebView, "darwin", "", []string{"tango_backend_webview"}},
		{featureCanvas, "darwin", "", []string{"tango_backend_canvas"}},
		{featureMixed, "darwin", "", []string{"tango_backend_webview", "tango_backend_canvas"}},
	}
	for _, tc := range cases {
		got, err := featureTags(tc.f, target{OS: tc.os, Arch: "arm64"})
		if err != nil || !sameStrings(got, tc.want) {
			t.Errorf("tags=%v err=%v want %v", got, err, tc.want)
		}
	}
	for _, osName := range []string{"windows", "linux"} {
		if _, err := featureTags(featureCanvas, target{OS: osName, Arch: "amd64"}); err == nil {
			t.Errorf("canvas accepted on %s", osName)
		}
	}
}

func TestFeatureGOFLAGSConflict(t *testing.T) {
	old := os.Getenv("GOFLAGS")
	t.Cleanup(func() { os.Setenv("GOFLAGS", old) })
	os.Setenv("GOFLAGS", "-tags=tango_backend_canvas")
	if _, err := featureTags(featureWebView, target{OS: "darwin", Arch: "arm64"}); err == nil {
		t.Fatal("conflicting GOFLAGS backend tag accepted")
	}
}

func sameStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
