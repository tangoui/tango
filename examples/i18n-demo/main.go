// Command i18n-demo shows package i18n: the system languages, an app
// catalog, and the native menu roles in the current locale.
//
//	go run ./examples/i18n-demo -print        # print the languages and exit
//	go run ./examples/i18n-demo               # window + localized app menu
//	go run ./examples/i18n-demo -lang ja      # override the system language
package main

import (
	"embed"
	"flag"
	"fmt"
	"html"
	"log"
	"net/http"
	"time"

	"github.com/tangoui/tango"
	"github.com/tangoui/tango/i18n"
	"github.com/tangoui/tango/sys/menu"
)

//go:embed locales
var locales embed.FS

func main() {
	printOnly := flag.Bool("print", false, "print the languages and exit")
	lang := flag.String("lang", "", "override the system language (BCP 47)")
	quitAfter := flag.Duration("quit", 0, "quit after this long (0 = stay open)")
	flag.Parse()

	if err := i18n.Load(locales, "locales"); err != nil {
		log.Fatal(err)
	}
	i18n.SetLocale(*lang)
	fmt.Println("Preferred:", i18n.Preferred())
	fmt.Println("Supported:", i18n.Supported())
	fmt.Println("Locale:   ", i18n.Locale())
	fmt.Println(i18n.T("hello", "name", "Tango"))
	fmt.Println(i18n.N("files", 1), "/", i18n.N("files", 3))
	if *printOnly {
		return
	}

	page := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		fmt.Fprintf(w, `<!doctype html><meta charset="utf-8"><body style="font:16px system-ui;padding:24px">
<h2>%s</h2><p>%s</p><p>%s</p><p>%s</p>`,
			html.EscapeString(i18n.T("title")),
			html.EscapeString(i18n.T("hello", "name", "Tango")),
			html.EscapeString(i18n.N("files", 3)),
			html.EscapeString(i18n.T("locale", "locale", i18n.Locale())))
	})
	app := tango.New(tango.Options{Name: "Tango i18n", ID: "dev.tango.i18n", Assets: page})
	i18n.Init(app) // optional on desktop; shown for completeness

	app.OnReady(func() {
		m := menu.New()
		m.Role(menu.AppMenu)
		m.Role(menu.EditMenu)
		m.Role(menu.WindowMenu)
		if err := menu.SetAppMenu(app, m); err != nil {
			log.Print(err)
		}
		if _, err := app.NewSurface(tango.SurfaceOptions{Title: i18n.T("title"), Width: 560, Height: 320, Resizable: true}); err != nil {
			log.Fatal(err)
		}
		if *quitAfter > 0 {
			time.AfterFunc(*quitAfter, app.Quit)
		}
	})
	if err := app.Run(); err != nil {
		log.Fatal(err)
	}
}
