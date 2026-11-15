// Command update-demo shows the default update window of
// service/updater/updateui.
//
//	go run ./examples/update-demo                      # serves a fake newer release locally
//	go run ./examples/update-demo -manifest URL -key PUB -version 1.2.0
//
// Without -manifest it starts a local server that publishes a signed
// manifest for version 9.9.9 (with a throwaway key), so the window opens
// at once. Downloading works; installing reports that this copy cannot
// update itself, because `go run` does not run from an app bundle,
// AppImage or install directory. "Check for Updates…" in the app menu and
// the button in the window call updateui.CheckNow.
package main

import (
	"crypto/sha256"
	"encoding/hex"
	"flag"
	"log"
	"net"
	"net/http"
	"runtime"
	"time"

	"github.com/tangoui/tango"
	"github.com/tangoui/tango/service/updater"
	"github.com/tangoui/tango/service/updater/updateui"
	"github.com/tangoui/tango/sys/menu"
	"github.com/tangoui/tango/ui"
	"github.com/tangoui/tango/ui/layout"
	"github.com/tangoui/tango/uihost"
)

func main() {
	manifest := flag.String("manifest", "", "update manifest URL (empty: serve a fake release locally)")
	key := flag.String("key", "", "update public key (tan keygen's .pub)")
	version := flag.String("version", "0.1.0", "the version this copy pretends to be")
	flag.Parse()

	if *manifest == "" {
		*manifest, *key = serveFakeRelease()
	}
	u := &updater.Updater{ManifestURL: *manifest, PublicKey: *key, Version: *version, AppID: "dev.tango.updatedemo"}

	quit := true
	app := tango.New(tango.Options{Name: "Update Demo", ID: "dev.tango.updatedemo", QuitOnLastClose: &quit})
	app.OnReady(func() {
		checkNow := func() { updateui.CheckNow(app, u, updateui.Options{}) }
		m := menu.New()
		sub := m.Submenu("Update Demo")
		sub.Add("Check for Updates…", checkNow)
		sub.Separator()
		sub.Role(menu.Quit)
		if err := menu.SetAppMenu(app, m); err != nil {
			log.Printf("update-demo: menu: %v", err)
		}

		_, _, err := uihost.Open(app, uihost.Options{Surface: tango.SurfaceOptions{
			Title: "Update Demo", Width: 420, Height: 220, Centered: true,
		}}, func(c *ui.Context) {
			ui.Column(c, func() {
				ui.Text(c, "Version "+*version)
				if ui.Button(c, "Check for Updates…").Primary().Clicked() {
					checkNow()
				}
			}, layout.Grow(1), layout.Pad(24), layout.Gap(16))
		}, ui.RunnerOptions{})
		if err != nil {
			log.Fatal(err)
		}

		// A short first delay so the demo finds the update quickly.
		if _, err := updateui.Start(app, u, updateui.Options{FirstDelay: 2 * time.Second}); err != nil {
			log.Printf("update-demo: %v", err)
		}
	})
	if err := app.Run(); err != nil {
		log.Fatal(err)
	}
}

// serveFakeRelease publishes version 9.9.9 on a local server with a
// throwaway key and returns the manifest URL and public key.
func serveFakeRelease() (manifestURL, publicKey string) {
	pub, priv, err := updater.GenerateKey()
	if err != nil {
		log.Fatal(err)
	}
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		log.Fatal(err)
	}
	base := "http://" + ln.Addr().String()
	file := []byte("not a real release")
	sum := sha256.Sum256(file)
	env, err := updater.SignManifest(priv, &updater.Manifest{
		App: "dev.tango.updatedemo", Version: "9.9.9", PubDate: time.Now(),
		Notes: "## What's new\n- A ready-made update window\n- Patches before full downloads\n\n## Fixes\n- Nothing: this release is fake",
		Platforms: map[string]updater.Asset{runtime.GOOS + "-" + runtime.GOARCH: {
			Format: fakeFormat(),
			File:   updater.File{URL: base + "/release", Size: int64(len(file)), SHA256: hex.EncodeToString(sum[:])},
		}},
	})
	if err != nil {
		log.Fatal(err)
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/latest.json", func(w http.ResponseWriter, _ *http.Request) { w.Write(env) })
	mux.HandleFunc("/release", func(w http.ResponseWriter, _ *http.Request) { w.Write(file) })
	go http.Serve(ln, mux)
	return base + "/latest.json", pub
}

// fakeFormat is a single-file format for this platform (no tree digest).
func fakeFormat() string {
	switch runtime.GOOS {
	case "windows":
		return updater.FormatExe
	default:
		return updater.FormatAppImage
	}
}
