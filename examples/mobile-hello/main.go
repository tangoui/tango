// mobile-hello is the smallest Tango app that runs unchanged on desktop
// (go run .) and on iOS (bash mobile/ios/build.sh examples/mobile-hello).
// The page posts a message when it loads and when the button is tapped; Go
// answers with Surface.Send and the page shows the answer. Its "System
// capabilities" link opens caps.html, which exercises the sys/... packages
// (share, haptics, safe area, biometrics, ...; see caps.go). On desktop
// most of them report "not supported".
package main

import (
	"embed"
	"io/fs"
	"log"
	"net/http"
	"runtime"
	"sync/atomic"

	"github.com/tangoui/tango"
)

//go:embed web
var web embed.FS

func main() {
	assets, _ := fs.Sub(web, "web")
	app := tango.New(tango.Options{
		Name:   "Tango Hello",
		ID:     "dev.tango.mobilehello",
		Assets: apiHandler(http.FileServerFS(assets)),
	})

	var n atomic.Int64
	setupCaps(app)
	app.OnMessage(func(s *tango.Surface, msg string) {
		if handleCaps(app, s, msg) {
			return
		}
		i := n.Add(1)
		log.Printf("mobile-hello: page says %q (#%d)", msg, i)
		s.Send("Go (" + runtime.GOOS + "/" + runtime.GOARCH + ") got: " + msg + " #" + itoa(i))
	})
	app.OnLifecycle(func(ev tango.Lifecycle) {
		log.Printf("mobile-hello: lifecycle %d", ev)
	})
	app.OnReady(func() {
		s, err := app.NewSurface(tango.SurfaceOptions{Title: "Tango Hello", Width: 420, Height: 640, Debug: true, URL: startURL()})
		if err != nil {
			log.Fatal(err)
		}
		w, h := s.Size()
		log.Printf("mobile-hello: surface %d ready, %dx%d", s.ID(), w, h)
	})
	if err := app.Run(); err != nil {
		log.Fatal(err)
	}
	log.Print("mobile-hello: bye")
}

func itoa(i int64) string {
	if i == 0 {
		return "0"
	}
	var b [20]byte
	p := len(b)
	for ; i > 0; i /= 10 {
		p--
		b[p] = byte('0' + i%10)
	}
	return string(b[p:])
}
