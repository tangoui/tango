// Command sysx-demo exercises the desktop system packages: securestore,
// power, network, a tray flyout panel, the app shortcut menu (Dock menu /
// Jump List) and dragging a file out of a window.
//
//	go run ./examples/sysx-demo          # interactive: tray icon with a flyout, a page with a draggable file
//	go run ./examples/sysx-demo -smoke   # runs the checks, logs PASS/FAIL, exits 0 when all pass
//
// TANGO_SMOKE_TIMEOUT_SCALE=3 stretches the -smoke timeout for slow machines (CI).
package main

import (
	"bytes"
	"errors"
	"flag"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strconv"
	"time"

	"github.com/tangoui/tango"
	"github.com/tangoui/tango/sys/flyout"
	"github.com/tangoui/tango/sys/jumplist"
	"github.com/tangoui/tango/sys/network"
	"github.com/tangoui/tango/sys/power"
	"github.com/tangoui/tango/sys/securestore"
	"github.com/tangoui/tango/sys/tray"
)

const mainPage = `<!doctype html><meta charset=utf-8><title>sysx demo</title>
<style>body{font:14px system-ui;margin:24px}#file{display:inline-block;padding:16px 20px;border:2px dashed #888;border-radius:8px;cursor:grab;user-select:none}</style>
<h3>Tango sysx demo</h3>
<p>Drag this to Finder / Explorer / your desktop:</p>
<div id=file draggable=true>📄 tango-dragout.txt</div>
<pre id=log></pre>
<script>
const f = document.getElementById('file');
f.addEventListener('dragstart', e => e.preventDefault());
f.addEventListener('mousedown', e => { if (e.button === 0) window.__tango.post('drag'); });
window.__tango.onMessage(m => document.getElementById('log').textContent += m + '\n');
</script>`

const panelPage = `<!doctype html><meta charset=utf-8>
<style>body{font:13px system-ui;margin:0;padding:16px;background:Canvas;color:CanvasText}</style>
<b>Tango flyout</b><p>Click elsewhere to hide me.</p><pre id=s></pre>
<script>window.__tango.onMessage(m => document.getElementById('s').textContent = m);</script>`

var failed int

func check(name string, err error) {
	if err != nil {
		failed++
		log.Printf("FAIL %s: %v", name, err)
		return
	}
	log.Printf("PASS %s", name)
}

func main() {
	smoke := flag.Bool("smoke", false, "run the checks and exit")
	flag.Parse()
	log.SetFlags(log.Ltime | log.Lmicroseconds)

	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.Write([]byte(mainPage))
	})
	mux.HandleFunc("/panel.html", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.Write([]byte(panelPage))
	})
	app := tango.New(tango.Options{Name: "Sysx Demo", ID: "dev.tango.sysxdemo", Assets: mux})

	dragFile := filepath.Join(os.TempDir(), "tango-dragout.txt")
	if err := os.WriteFile(dragFile, []byte("dragged out of a Tango window\n"), 0o644); err != nil {
		log.Fatal(err)
	}

	clicks := make(chan string, 4)
	jumplist.OnClick(app, func(id string) {
		log.Printf("shortcut %q", id)
		select {
		case clicks <- id:
		default:
		}
	})
	power.OnChange(app, func(s power.Status) { log.Printf("power changed: %+v", s) })
	power.OnSuspend(app, func() { log.Print("system going to sleep") })
	power.OnResume(app, func() { log.Print("system woke up") })
	network.OnChange(app, func(s network.Status) { log.Printf("network changed: %+v (%s)", s, network.KindString(s.Kind)) })

	var win *tango.Surface
	app.OnMessage(func(s *tango.Surface, msg string) {
		if msg == "drag" {
			err := s.DragFiles([]string{dragFile}, nil, func(op tango.DragOperation) { s.Send(fmt.Sprintf("drag ended: op=%d", op)) })
			if err != nil {
				s.Send("drag error: " + err.Error())
			}
		}
	})

	app.OnReady(func() {
		var err error
		win, err = app.NewSurface(tango.SurfaceOptions{Title: "Sysx Demo", Width: 520, Height: 360, Resizable: true})
		if err != nil {
			log.Fatal(err)
		}
		go func() {
			run(app, win, dragFile, clicks, *smoke)
			if *smoke {
				app.Quit()
			}
		}()
	})
	if *smoke {
		time.AfterFunc(scaled(15*time.Second), func() {
			log.Print("FAIL timeout")
			os.Exit(2)
		})
	}
	if err := app.Run(); err != nil {
		log.Fatal(err)
	}
	if failed > 0 {
		log.Printf("%d check(s) failed", failed)
		os.Exit(1)
	}
	log.Print("all checks passed")
}

func run(app *tango.App, win *tango.Surface, dragFile string, clicks chan string, smoke bool) {
	// securestore
	key := fmt.Sprintf("sysx-demo-%d", os.Getpid())
	err := securestore.SetString(app, key, "s3cret")
	check("securestore.Set", err)
	v, err := securestore.GetString(app, key)
	if err == nil && v != "s3cret" {
		err = fmt.Errorf("got %q", v)
	}
	check("securestore.Get", err)
	check("securestore.Delete", securestore.Delete(app, key))
	if _, err := securestore.Get(app, key); !errors.Is(err, securestore.ErrNotFound) {
		check("securestore.Get after delete", fmt.Errorf("got %v", err))
	} else {
		check("securestore.Get after delete", nil)
	}

	// power
	st, err := power.Get(app)
	log.Printf("power: %+v", st)
	if err == nil && st.Source == power.Unknown {
		err = errors.New("unknown power source")
	}
	check("power.Get", err)
	blk, err := power.KeepDisplayOn(app, "sysx-demo smoke")
	check("power.KeepDisplayOn", err)
	type sleepBlocks interface{ SmokeSleepBlocks() int }
	sb, _ := app.Backend().(sleepBlocks)
	count := func() int { return tango.Await(app, func() int { return sb.SmokeSleepBlocks() }) }
	if err == nil && sb != nil {
		if n := count(); n != 1 {
			check("power block active", fmt.Errorf("%d blocks", n))
		}
		blk.Release()
		app.Post(func() {})
		time.Sleep(50 * time.Millisecond)
		if n := count(); n != 0 {
			check("power.Release", fmt.Errorf("%d blocks after release", n))
		} else {
			check("power.Release", nil)
		}
	} else {
		blk.Release()
	}

	// network
	ns, err := network.Get(app)
	log.Printf("network: %+v (%s)", ns, network.KindString(ns.Kind))
	if err == nil && ns.Online && (ns.Kind == network.Unknown || ns.Kind == network.None) {
		err = fmt.Errorf("online with kind %s", network.KindString(ns.Kind))
	}
	check("network.Get", err)

	// tray + flyout
	t, err := tray.New(app, tray.Options{Icon: trayIcon(), Template: true, Tooltip: "Sysx Demo", MenuOn: tray.MenuOnSecondary})
	check("tray.New", err)
	f, err := flyout.New(app, t, flyout.Options{Width: 300, Height: 220, URL: "/panel.html"})
	check("flyout.New", err)
	if err == nil {
		time.Sleep(300 * time.Millisecond) // let the menu bar place the icon
		err = f.Open()
		if err == nil && (!f.IsOpen() || !f.Surface().IsVisible()) {
			err = errors.New("not visible after Open")
		}
		if err == nil {
			if tb, ok := t.Bounds(); ok {
				b := f.Surface().Bounds()
				log.Printf("tray %+v flyout %+v", tb, b)
				if runtime.GOOS == "darwin" && (b.Y < tb.Y+tb.Height || abs(b.X+b.Width/2-(tb.X+tb.Width/2)) > b.Width/2) {
					err = fmt.Errorf("flyout %+v not under tray %+v", b, tb)
				}
			}
		}
		check("flyout.Open", err)
		f.Surface().Send("power " + fmt.Sprint(st.Source) + ", network " + network.KindString(ns.Kind))
		if smoke {
			time.Sleep(500 * time.Millisecond)
			f.Close()
			if f.IsOpen() || f.Surface().IsVisible() {
				check("flyout.Close", errors.New("still visible"))
			} else {
				check("flyout.Close", nil)
			}
		}
	}

	// app shortcut menu
	err = jumplist.Set(app, []jumplist.Item{
		{ID: "new", Label: "New Window"},
		jumplist.Separator(),
		{ID: "sync", Label: "Sync Now", Description: "Fetch new data"},
	})
	if errors.Is(err, jumplist.ErrUnsupported) && runtime.GOOS == "linux" {
		log.Print("SKIP jumplist.Set (Linux: Desktop Actions are written at packaging time)")
	} else {
		check("jumplist.Set", err)
	}
	type dockSmoke interface {
		SmokeDockMenu() []string
		SmokeDockChoose(int) bool
	}
	if ds, ok := app.Backend().(dockSmoke); ok && err == nil {
		titles := tango.Await(app, ds.SmokeDockMenu)
		if !slices.Equal(titles, []string{"New Window", "-", "Sync Now"}) {
			check("dock menu items", fmt.Errorf("%q", titles))
		} else {
			check("dock menu items", nil)
		}
		tango.Await(app, func() bool { return ds.SmokeDockChoose(2) })
		select {
		case id := <-clicks:
			if id != "sync" {
				err = fmt.Errorf("clicked %q", id)
			}
		case <-time.After(scaled(2 * time.Second)):
			err = errors.New("no click")
		}
		check("dock menu click", err)
	}

	// drag-out: the call must be accepted; without a held mouse button the
	// session ends at once.
	ended := make(chan tango.DragOperation, 1)
	err = win.DragFiles([]string{dragFile}, nil, func(op tango.DragOperation) { ended <- op })
	check("DragFiles", err)
	if err == nil && smoke {
		select {
		case op := <-ended:
			log.Printf("drag ended without a press: op=%d", op)
		case <-time.After(scaled(time.Second)):
			log.Print("drag session still open without a press (ends with the app)")
		}
	}
	if err := win.DragFiles([]string{"relative.txt"}, nil, nil); err == nil {
		check("DragFiles rejects relative paths", errors.New("accepted"))
	}
	if smoke {
		_ = jumplist.Clear(app)
	}
}

func abs(v int) int {
	if v < 0 {
		return -v
	}
	return v
}

// trayIcon is a 36x36 black circle (a template image on macOS).
func trayIcon() []byte {
	img := image.NewNRGBA(image.Rect(0, 0, 36, 36))
	for y := range 36 {
		for x := range 36 {
			dx, dy := x-18, y-18
			if dx*dx+dy*dy <= 12*12 && !(dx*dx+dy*dy <= 6*6) {
				img.Set(x, y, color.Black)
			}
		}
	}
	var b bytes.Buffer
	_ = png.Encode(&b, img)
	return b.Bytes()
}

func scaled(d time.Duration) time.Duration {
	if f, err := strconv.ParseFloat(os.Getenv("TANGO_SMOKE_TIMEOUT_SCALE"), 64); err == nil && f > 1 {
		return time.Duration(float64(d) * f)
	}
	return d
}
