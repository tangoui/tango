// WebView Notes saves an embedded editor's text through a native dialog.
package main

import (
	"context"
	"embed"
	"errors"
	"flag"
	"io/fs"
	"log"
	"os"
	"path/filepath"

	"github.com/tangoui/tango"
	"github.com/tangoui/tango/asset"
	"github.com/tangoui/tango/bind"
	"github.com/tangoui/tango/ipc"
	"github.com/tangoui/tango/runtime-js/runtimejs"
	"github.com/tangoui/tango/sys/dialog"
	"github.com/tangoui/tango/sys/menu"
	"github.com/tangoui/tango/sys/windowstate"
)

//go:embed page
var frontend embed.FS

type notes struct {
	app *tango.App
}

// Save asks for a destination in the window that invoked the Go method.
func (n *notes) Save(ctx context.Context, text string) (string, error) {
	conn, _ := ipc.ConnFrom(ctx)
	s, ok := conn.(*tango.Surface)
	if !ok {
		return "", errors.New("saving a note requires a window")
	}
	dest, err := dialog.Save(n.app, dialog.FileOptions{
		Parent: s, Title: "Save note", Filename: "note.md",
	})
	if errors.Is(err, dialog.ErrCanceled) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	if err := os.WriteFile(dest, []byte(text), 0600); err != nil {
		return "", err
	}
	return filepath.Base(dest), nil
}

func installMenu(app *tango.App, s *tango.Surface, router *ipc.Router) error {
	bar := menu.New()
	bar.Submenu("WebView Notes").Role(menu.Quit)
	file := bar.Submenu("File")
	file.Add("Save As…", func() {
		if err := router.Emit(s, "menu:save", nil); err != nil {
			log.Print(err)
		}
	}).Accelerator = "CmdOrCtrl+S"
	bar.Submenu("Edit").Role(menu.EditMenu)
	return menu.SetAppMenu(app, bar)
}

func main() {
	statePath := flag.String("window-state", "", "path to the window-state file")
	flag.Parse()
	files, err := fs.Sub(frontend, "page")
	if err != nil {
		log.Fatal(err)
	}
	app := tango.New(tango.Options{
		Name: "WebView Notes", ID: "io.tangoui.examples.webviewnotes",
		Assets: asset.FromEnv(files),
	})
	router := ipc.NewRouter()
	if err := bind.Register(router, "Notes", &notes{app: app}); err != nil {
		log.Fatal(err)
	}
	app.AddBridgeScript(runtimejs.Script)
	app.OnMessage(func(s *tango.Surface, msg string) { router.HandleMessage(s, msg) })
	app.OnBinary(func(s *tango.Surface, gen uint64, data []byte) { router.HandleBinarySession(s, gen, data) })
	app.OnReady(func() {
		s, err := app.NewSurface(tango.SurfaceOptions{
			Title: "WebView Notes", Width: 700, Height: 500, Resizable: true, Hidden: true,
		})
		if err != nil {
			log.Fatal(err)
		}
		s.OnClosed(func() { router.Drop(s) })
		var store windowstate.Store
		if *statePath != "" {
			store = windowstate.NewFileStore(*statePath)
		}
		if err := windowstate.Track(s, "main", store); err != nil {
			log.Print(err)
		}
		if err := installMenu(app, s, router); err != nil {
			log.Print(err)
		}
		s.Show()
	})
	if err := app.Run(); err != nil {
		log.Fatal(err)
	}
}
