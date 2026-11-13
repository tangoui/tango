// Command runtime-demo shows the page-side runtime API (package jsapi,
// window.tango.Window / Screens / Theme / Clipboard / Dialog ...): the page
// retitles and resizes its window, reads the displays and the theme, round
// trips the clipboard, listens to window and theme events, and shows a
// message box and a context menu.
//
//	go run ./examples/runtime-demo          # interactive
//	go run ./examples/runtime-demo -smoke   # runs the checks and exits 0 on success
//
// Notifications are deliberately left disabled, so the page also shows
// what a forbidden capability looks like.
package main

import (
	"embed"
	"encoding/json"
	"flag"
	"io/fs"
	"log"
	"os"
	"time"

	"github.com/tangoui/tango"
	"github.com/tangoui/tango/asset"
	"github.com/tangoui/tango/ipc"
	"github.com/tangoui/tango/jsapi"
	"github.com/tangoui/tango/runtime-js/runtimejs"
	"github.com/tangoui/tango/sys/single"
)

//go:embed frontend
var frontend embed.FS

func main() {
	smoke := flag.Bool("smoke", false, "run the checks automatically and exit")
	flag.Parse()

	site, _ := fs.Sub(frontend, "frontend")
	app := tango.New(tango.Options{
		Name: "Tango Runtime Demo", ID: "dev.tango.runtime-demo",
		Assets:     asset.FS(site, asset.Options{}),
		URLSchemes: []string{"tango-demo"},
	})

	r := ipc.NewRouter()
	app.AddBridgeScript(runtimejs.Script)
	app.OnMessage(func(s *tango.Surface, m string) {
		if os.Getenv("TANGO_DEBUG") != "" {
			log.Printf("msg: %.300s", m)
		}
		r.HandleMessage(s, m)
	})
	jsapi.Install(app, r, jsapi.Options{
		Allow: jsapi.ClipboardRead | jsapi.ClipboardWrite | jsapi.Dialogs | jsapi.OpenURL | jsapi.AppControl,
	})

	if !*smoke {
		// jsapi forwards later launches to the pages (App.onSecondInstance).
		primary, err := single.Claim(app, single.Options{})
		if !primary {
			return
		}
		if err != nil {
			log.Print("single instance disabled: ", err)
		}
	}

	r.On("smoke-log", func(_ ipc.Conn, data json.RawMessage) { log.Printf("page: %s", data) })
	r.On("done", func(_ ipc.Conn, data json.RawMessage) {
		log.Printf("page finished: %s", data)
		if !*smoke {
			return
		}
		var got struct {
			OK       bool     `json:"ok"`
			Failures []string `json:"failures"`
		}
		_ = json.Unmarshal(data, &got)
		if got.OK && len(got.Failures) == 0 {
			log.Print("PASS runtime API")
			app.Quit()
			return
		}
		log.Printf("FAIL %v", got.Failures)
		os.Exit(1)
	})

	app.OnReady(func() {
		url := "/"
		if *smoke {
			url = "/?smoke"
			time.AfterFunc(20*time.Second, func() { log.Print("FAIL timeout"); os.Exit(1) })
		}
		s, err := app.NewSurface(tango.SurfaceOptions{
			Title: "Tango Runtime Demo", Width: 760, Height: 560, URL: url,
			Resizable: true, Debug: true, AcceptFiles: true,
		})
		if err != nil {
			log.Fatal(err)
		}
		s.OnClosed(func() { r.Drop(s) })
	})

	if err := app.Run(); err != nil {
		log.Fatal(err)
	}
}
