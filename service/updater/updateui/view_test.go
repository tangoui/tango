package updateui

import (
	"runtime"
	"testing"
	"time"

	"github.com/tangoui/tango/service/updater"
	"github.com/tangoui/tango/ui"
)

// recorder counts the window's actions.
type recorder struct{ calls map[string]int }

func (r *recorder) actions() actions {
	r.calls = map[string]int{}
	hit := func(name string) func() { return func() { r.calls[name]++ } }
	return actions{
		skip: hit("skip"), later: hit("later"), install: hit("install"), cancel: hit("cancel"),
		restartNow: hit("restartNow"), installNextLaunch: hit("installNextLaunch"),
		retry: hit("retry"), close: hit("close"), openDownloadPage: hit("openDownloadPage"),
	}
}

func viewDriver(t *testing.T, m *model, s func() *Strings) (*ui.Driver, *recorder) {
	t.Helper()
	rec := &recorder{}
	d := ui.NewDriver(page(m, s, rec.actions()), ui.DriverOptions{Width: 480, Height: 360})
	d.Settle()
	return d, rec
}

func zh() *Strings { return stringsZhCN }

func mustText(t *testing.T, d *ui.Driver, s string) {
	t.Helper()
	if _, ok := d.FindText(s); !ok {
		t.Fatalf("no text %q", s)
	}
}

func click(t *testing.T, d *ui.Driver, label string) {
	t.Helper()
	if err := d.ClickLabel(label); err != nil {
		t.Fatal(err)
	}
	d.Settle()
}

var installable = runtime.GOOS == "darwin" || runtime.GOOS == "windows" || runtime.GOOS == "linux"

func TestViewAvailable(t *testing.T) {
	m := &model{phase: phaseAvailable, current: "1.2.0",
		rel: &updater.Release{Version: "1.3.0", Notes: "- 修复崩溃", PubDate: time.Date(2026, 11, 14, 0, 0, 0, 0, time.UTC)}}
	d, rec := viewDriver(t, m, zh)
	mustText(t, d, "1.3.0")
	mustText(t, d, "修复崩溃")
	mustText(t, d, "2026-11-14")
	click(t, d, stringsZhCN.Skip)
	click(t, d, stringsZhCN.Later)
	if rec.calls["skip"] != 1 || rec.calls["later"] != 1 {
		t.Fatalf("calls %v", rec.calls)
	}
	if !installable {
		if _, ok := d.Find(stringsZhCN.Install); ok {
			t.Fatal("install offered where the app cannot update itself")
		}
		return
	}
	click(t, d, stringsZhCN.Install)
	if rec.calls["install"] != 1 {
		t.Fatalf("install calls %v", rec.calls)
	}
}

func TestViewDownloadingProgress(t *testing.T) {
	m := &model{phase: phaseDownloading, patch: true, done: 50, total: 200, rel: &updater.Release{Version: "1.3.0"}}
	d, rec := viewDriver(t, m, zh)
	mustText(t, d, stringsZhCN.Patch)
	click(t, d, stringsZhCN.Cancel)
	if rec.calls["cancel"] != 1 {
		t.Fatalf("cancel calls %v", rec.calls)
	}
	m.set(func(m *model) { m.total, m.patch = -1, false })
	d.Settle()
	mustText(t, d, stringsZhCN.Full)
	if _, ok := d.Find(stringsZhCN.Cancel); !ok {
		t.Fatal("cancel gone with an unknown size")
	}
}

func TestViewReady(t *testing.T) {
	m := &model{phase: phaseReady, rel: &updater.Release{Version: "1.3.0"}}
	d, rec := viewDriver(t, m, zh)
	mustText(t, d, stringsZhCN.Ready)
	click(t, d, stringsZhCN.RestartNow)
	click(t, d, stringsZhCN.InstallNextLaunch)
	if rec.calls["restartNow"] != 1 || rec.calls["installNextLaunch"] != 1 {
		t.Fatalf("calls %v", rec.calls)
	}
}

func TestViewDownloadedAtLaunch(t *testing.T) {
	m := &model{phase: phaseDownloaded, rel: &updater.Release{Version: "1.3.0"}}
	d, rec := viewDriver(t, m, zh)
	mustText(t, d, stringsZhCN.Downloaded)
	click(t, d, stringsZhCN.RestartNow)
	click(t, d, stringsZhCN.Later)
	if rec.calls["restartNow"] != 1 || rec.calls["later"] != 1 {
		t.Fatalf("calls %v", rec.calls)
	}
}

func TestViewError(t *testing.T) {
	m := &model{}
	m.applyEvent(updater.Event{Kind: updater.EventError, Error: "signature mismatch"})
	d, rec := viewDriver(t, m, zh)
	mustText(t, d, stringsZhCN.Error)
	mustText(t, d, "signature mismatch")
	click(t, d, stringsZhCN.Retry)
	click(t, d, stringsZhCN.Close)
	if rec.calls["retry"] != 1 || rec.calls["close"] != 1 {
		t.Fatalf("calls %v", rec.calls)
	}
}

func TestViewErrorUnsupported(t *testing.T) {
	m := &model{phase: phaseError, unsupported: true,
		rel: &updater.Release{Version: "1.3.0", Full: updater.File{URL: "https://example.com/app.AppImage"}}}
	d, rec := viewDriver(t, m, zh)
	mustText(t, d, stringsZhCN.Unsupported)
	if _, ok := d.Find(stringsZhCN.Retry); ok {
		t.Fatal("retry offered for an installation that cannot update itself")
	}
	click(t, d, stringsZhCN.OpenDownloadPage)
	if rec.calls["openDownloadPage"] != 1 {
		t.Fatalf("calls %v", rec.calls)
	}
}

func TestViewUpToDateAndRolledBack(t *testing.T) {
	d, rec := viewDriver(t, &model{phase: phaseUpToDate, current: "1.2.0"}, zh)
	mustText(t, d, "1.2.0")
	click(t, d, stringsZhCN.Close)
	d2, _ := viewDriver(t, &model{phase: phaseRolledBack, current: "1.1.0"}, zh)
	mustText(t, d2, "1.1.0")
	if rec.calls["close"] != 1 {
		t.Fatalf("calls %v", rec.calls)
	}
}

func TestViewChecking(t *testing.T) {
	d, _ := viewDriver(t, &model{phase: phaseChecking}, zh)
	mustText(t, d, stringsZhCN.Checking)
}

func TestViewStringsSwitchLive(t *testing.T) {
	cur := stringsZhCN
	m := &model{phase: phaseUpToDate, current: "1.2.0"}
	d, _ := viewDriver(t, m, func() *Strings { return cur })
	if _, ok := d.Find(stringsZhCN.Close); !ok {
		t.Fatal("no Chinese close button")
	}
	cur = stringsEN
	m.set(func(*model) {})
	d.Settle()
	if _, ok := d.Find(stringsEN.Close); !ok {
		t.Fatal("texts did not follow the locale change")
	}
}
