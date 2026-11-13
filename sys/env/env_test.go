package env_test

import (
	"runtime"
	"testing"

	"github.com/tangoui/tango"
	"github.com/tangoui/tango/internal/platform/fake/faketest"
	"github.com/tangoui/tango/sys/env"
)

func TestInfo(t *testing.T) {
	in := env.Info(nil)
	if in.OS != runtime.GOOS || in.Arch != runtime.GOARCH || in.Debug != wantDebug || in.OSName == "" || in.WebView != "" {
		t.Fatalf("%+v", in)
	}
	if runtime.GOOS == "darwin" && in.OSVersion == "" {
		t.Fatal("no macOS version")
	}
	a, _ := faketest.Run(t, tango.Options{})
	if in := env.Info(a); in.WebView != "FakeView" || in.WebViewVersion != "1.0" {
		t.Fatalf("%+v", in)
	}
	b, _ := faketest.RunMinimal(t, tango.Options{})
	if in := env.Info(b); in.WebView != "" {
		t.Fatalf("%+v", in)
	}
}
