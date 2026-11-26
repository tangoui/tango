// Command webkey is the native WebView keyboard fixture for tan mcp tests.
package main

import (
	"embed"
	"log"

	"github.com/tangoui/tango"
	"github.com/tangoui/tango/asset"
	"github.com/tangoui/tango/sys/menu"
)

//go:embed index.html
var page embed.FS

func main() {
	app := tango.New(tango.Options{Name: "Web Key Test", ID: "dev.tango.webkey", Assets: asset.FS(page, asset.Options{})})
	app.OnReady(func() {
		w, err := app.NewSurface(tango.SurfaceOptions{Title: "Web Key Test", Width: 710, Height: 390})
		if err != nil {
			log.Fatal(err)
		}
		bar := menu.New()
		file := bar.Submenu("File")
		file.Add("Save", func() {
			log.Print("native Save OnClick")
			w.Eval("document.getElementById('save').textContent = 'Native Save OnClick: 1'")
		}).Accelerator = "CmdOrCtrl+S"
		disabled := file.Add("Disabled", func() {
			w.Eval("window.disabledClicks++")
		})
		disabled.Accelerator = "CmdOrCtrl+B"
		disabled.SetDisabled(true)
		bar.Submenu("Edit").Role(menu.EditMenu)
		if err := menu.SetAppMenu(app, bar); err != nil {
			log.Fatal(err)
		}
	})
	if err := app.Run(); err != nil {
		log.Fatal(err)
	}
}
