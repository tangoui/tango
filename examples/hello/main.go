// Command hello is the desktop end-to-end example: an embedded page calls a
// bound Go service, consumes a stream, receives an event and reports back.
//
//	go run ./examples/hello          # interactive
//	go run ./examples/hello -smoke   # runs the round trip and exits 0 on success
//
// TANGO_SMOKE_TIMEOUT_SCALE=3 stretches the -smoke timeout for slow machines (CI).
package main

import (
	"context"
	"embed"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io/fs"
	"log"
	"os"
	"strconv"
	"time"

	"github.com/tangoui/tango"
	"github.com/tangoui/tango/asset"
	"github.com/tangoui/tango/bind"
	"github.com/tangoui/tango/ipc"
	"github.com/tangoui/tango/runtime-js/runtimejs"
)

//go:embed frontend
var frontend embed.FS

// Greeter is exposed to JS as "Greeter.*" through bind.Register.
type Greeter struct{}

func (Greeter) Hello(name string) string { return "你好，" + name + "！" }

func (Greeter) Fail() error { return errors.New("故意的错误") }

func main() {
	smoke := flag.Bool("smoke", false, "run the round trip automatically and exit")
	flag.Parse()

	site, _ := fs.Sub(frontend, "frontend")
	app := tango.New(tango.Options{Name: "Tango Hello", ID: "dev.tango.hello", Assets: asset.FS(site, asset.Options{})})

	r := ipc.NewRouter()
	if err := bind.Register(r, "Greeter", Greeter{}); err != nil {
		log.Fatal(err)
	}
	r.HandleStream("Greeter.Count", func(ctx context.Context, args json.RawMessage, w ipc.StreamWriter) error {
		var a []int
		if err := json.Unmarshal(args, &a); err != nil || len(a) != 1 {
			return ipc.NewError("bad_args", "Count(n int)")
		}
		for i := 1; i <= a[0]; i++ {
			if err := w.Write(fmt.Sprintf("tick %d", i)); err != nil {
				return err
			}
		}
		return nil
	})

	app.AddBridgeScript(runtimejs.Script)
	app.OnMessage(func(s *tango.Surface, m string) {
		if os.Getenv("TANGO_DEBUG") != "" {
			log.Printf("msg: %.200s", m)
		}
		r.HandleMessage(s, m)
	})

	r.On("done", func(c ipc.Conn, data json.RawMessage) {
		log.Printf("page finished: %s", data)
		if *smoke {
			var got struct {
				Greeting string `json:"greeting"`
				Ticks    int    `json:"ticks"`
			}
			_ = json.Unmarshal(data, &got)
			if got.Greeting == "你好，世界！" && got.Ticks == 5 {
				log.Print("PASS hello round trip")
				app.Quit()
				return
			}
			log.Print("FAIL unexpected result")
			os.Exit(1)
		}
	})

	app.OnReady(func() {
		url := "/"
		if *smoke {
			url = "/?smoke"
			time.AfterFunc(scaled(15*time.Second), func() { log.Print("FAIL timeout"); os.Exit(1) })
		}
		s, err := app.NewSurface(tango.SurfaceOptions{Title: "Tango Hello", Width: 640, Height: 420, URL: url, Resizable: true, Debug: true})
		if err != nil {
			log.Fatal(err)
		}
		s.OnClosed(func() { r.Drop(s) })
		s.OnLoaded(func() {
			log.Print("page loaded")
			_ = r.Emit(s, "server-time", time.Now().Format("15:04:05"))
		})
	})

	if err := app.Run(); err != nil {
		log.Fatal(err)
	}
}

// scaled stretches d by TANGO_SMOKE_TIMEOUT_SCALE (a factor >= 1, default 1)
// so slow CI machines get more time; unset, the timings are unchanged.
func scaled(d time.Duration) time.Duration {
	if f, err := strconv.ParseFloat(os.Getenv("TANGO_SMOKE_TIMEOUT_SCALE"), 64); err == nil && f > 1 {
		return time.Duration(float64(d) * f)
	}
	return d
}
