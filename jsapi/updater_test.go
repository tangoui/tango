package jsapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/tangoui/tango/service/updater"
)

// updateServer publishes version 2.0.0 of a single-file app.
func updateServer(t *testing.T) *updater.Updater {
	t.Helper()
	pub, priv, err := updater.GenerateKey()
	if err != nil {
		t.Fatal(err)
	}
	body := []byte(strings.Repeat("new version ", 1000))
	file := filepath.Join(t.TempDir(), "App-2.0.0.exe")
	os.WriteFile(file, body, 0o644)
	full, err := updater.FileInfo(file, "App-2.0.0.exe")
	if err != nil {
		t.Fatal(err)
	}
	env, err := updater.SignManifest(priv, &updater.Manifest{
		Version: "2.0.0", PubDate: time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC),
		Platforms: map[string]updater.Asset{"testos-testarch": {Format: updater.FormatExe, File: full}},
	})
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, ".json") {
			w.Write(env)
			return
		}
		w.Write(body)
	}))
	t.Cleanup(srv.Close)
	return &updater.Updater{
		ManifestURL: srv.URL + "/latest.json", PublicKey: pub, Version: "1.0.0",
		Platform: "testos-testarch", Client: srv.Client(), StateDir: t.TempDir(),
	}
}

func TestUpdaterOffByDefault(t *testing.T) {
	e := setup(t, Options{})
	e.api.InstallUpdater(updateServer(t), UpdaterOptions{AllowRestart: true})
	p := e.open()
	for _, m := range []string{"updater.check", "updater.download", "updater.restart"} {
		p.fails(CodeForbidden, m)
	}
}

func TestUpdaterCheckAndDownload(t *testing.T) {
	e := setup(t, Options{Allow: Updates})
	u := updateServer(t)
	hooked := 0
	u.OnEvent = func(updater.Event) { hooked++ }
	var got *updater.Payload
	e.api.InstallUpdater(u, UpdaterOptions{OnDownloaded: func(p *updater.Payload) { got = p }})
	p := e.open()

	p.fails("not_found", "updater.download")
	var rel struct {
		Version string `json:"version"`
		Format  string `json:"format"`
		Full    struct {
			URL string `json:"url"`
		} `json:"full"`
	}
	json.Unmarshal(p.ok("updater.check"), &rel)
	if rel.Version != "2.0.0" || rel.Format != "exe" || !strings.HasSuffix(rel.Full.URL, "/App-2.0.0.exe") {
		t.Fatalf("check = %+v", rel)
	}
	jsonEq(t, p.event("updater.event", 1), `{"kind":"checking"}`)
	var ev struct {
		Kind    string `json:"kind"`
		Release struct {
			Version string `json:"version"`
		} `json:"release"`
	}
	json.Unmarshal(p.event("updater.event", 2), &ev)
	if ev.Kind != "available" || ev.Release.Version != "2.0.0" {
		t.Fatalf("event %+v", ev)
	}

	jsonEq(t, p.ok("updater.download"), `{"version":"2.0.0","patched":false}`)
	if got == nil || filepath.Base(got.Path) != "App-2.0.0.exe" {
		t.Fatalf("OnDownloaded got %+v", got)
	}
	p.wait(func(m map[string]json.RawMessage) bool {
		return strings.Contains(string(m["data"]), `"verified"`)
	})
	if hooked == 0 {
		t.Fatal("the app's own OnEvent hook was dropped")
	}
	// Restart was not allowed.
	p.fails(CodeForbidden, "updater.restart")
}
