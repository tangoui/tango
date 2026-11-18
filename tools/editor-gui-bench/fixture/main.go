// Command fixture is the opt-in real-window Tango editor fixture used by the
// GUI acceptance probe. It binds a random loopback port, requires a bearer
// token, and only changes its own synthetic editor state on App.Post.
package main

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"hash/fnv"
	"log"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/tangoui/tango"
	"github.com/tangoui/tango/internal/guiprobe"
	"github.com/tangoui/tango/ui"
	"github.com/tangoui/tango/ui/editor"
	"github.com/tangoui/tango/ui/editor/theme"
	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/input"
	"github.com/tangoui/tango/ui/layout"
	uitheme "github.com/tangoui/tango/ui/theme"
	"github.com/tangoui/tango/uihost"
)

type request struct {
	Scenario string `json:"scenario"`
	Line     int    `json:"line,omitempty"`
	Carets   int    `json:"carets,omitempty"`
}

// Content size equal to the visible Monaco viewport on the 1920×1080 (logical)
// test display: the menu bar (30pt) and Edge's app titlebar (32pt) leave
// 1920×944, and a taller Tango window would extend off screen and render
// ~14% more rows than Monaco. Deviation from the design's 1920×1080 is
// recorded in the design document.
const contentW, contentH = 1920, 944

type inputRecord struct {
	Kind, Text string
	Cursor     int
	Host       int64
}

type state struct {
	sync.Mutex
	app    *tango.App
	runner *ui.Runner
	st     *editor.State
	view   *editor.ViewW
	trace  []inputRecord
}

func controlHandler(token string, do func(request) (any, error)) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer "+token {
			http.Error(w, "unauthorized", 401)
			return
		}
		if r.Method != http.MethodPost {
			http.Error(w, "method", 405)
			return
		}
		var req request
		dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096))
		dec.DisallowUnknownFields()
		if dec.Decode(&req) != nil || dec.More() {
			http.Error(w, "bad request", 400)
			return
		}
		if r.URL.Path == "/state" {
			req.Scenario = "state"
		}
		if r.URL.Path == "/quit" {
			req.Scenario = "quit"
		}
		if r.URL.Path == "/undo" {
			req.Scenario = "undo"
		}
		if r.URL.Path == "/redo" {
			req.Scenario = "redo"
		}
		out, err := do(req)
		if err != nil {
			http.Error(w, err.Error(), 409)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(out)
	})
}

// typingDocument replaces one model line by a stable typing line. Its body
// is wide enough to keep later characters in the same visible row.
func typingDocument(text string, line, _ int) string {
	lines := strings.SplitAfter(text, "\n")
	if line >= 0 && line < len(lines) {
		lines[line] = "// qq" + strings.Repeat(" ", 160) + "\n"
	}
	return strings.Join(lines, "")
}

// historyText performs one fixture-only undo/redo and returns exact text for
// oracle comparison. It is never part of a timed input sample.
func historyText(st *editor.State, op string) string {
	if op == "undo" {
		st.Undo()
	} else {
		st.Redo()
	}
	return st.Snapshot().String()
}

func gen(n int) string {
	var b strings.Builder
	for i := range n {
		fmt.Fprintf(&b, "export const value%d = compute(%d, \"row %d\"); // fixture\n", i, i, i)
	}
	return b.String()
}

func checksum(s string) string {
	h := fnv.New64a()
	_, _ = h.Write([]byte(s))
	return fmt.Sprintf("%016x", h.Sum64())
}

func (s *state) onMain(fn func() (any, error)) (any, error) {
	type result struct {
		v   any
		err error
	}
	ch := make(chan result, 1)
	s.app.Post(func() { v, err := fn(); ch <- result{v, err} })
	select {
	case r := <-ch:
		return r.v, r.err
	case <-time.After(5 * time.Second):
		return nil, errors.New("UI thread timeout")
	}
}

func (s *state) control(req request) (any, error) {
	return s.onMain(func() (any, error) {
		switch req.Scenario {
		case "quit":
			s.app.Quit()
			return map[string]bool{"ok": true}, nil
		case "state":
		case "undo", "redo":
			historyText(s.st, req.Scenario)
		case "type", "ime", "multi":
			lines := 2000
			if req.Scenario == "multi" {
				lines = 6000
			}
			text := typingDocument(gen(lines), 20, 0)
			s.st.Close()
			s.st = editor.NewState(text, "fixture.ts", theme.Base("vs"))
			s.view = nil
			off := s.st.Snapshot().LineStart(20) + len("// qq")
			sels := []editor.Selection{{Anchor: off, Head: off}}
			for k := 1; k < req.Carets; k++ {
				o := s.st.Snapshot().LineStart(20+k*4) + 2
				sels = append(sels, editor.Selection{Anchor: o, Head: o})
			}
			s.st.SetSelections(sels)
			s.st.RevealCaret()
			s.Lock()
			s.trace = nil
			s.Unlock()
		default:
			return nil, fmt.Errorf("unknown scenario %q", req.Scenario)
		}
		s.runner.Invalidate()
		snap := s.st.Snapshot()
		var sels []map[string]int
		// rows: each caret line's text row in window points (the probe's
		// record/template regions), from the editor's own scroll geometry.
		var rows [][2]float64
		for _, x := range s.st.Sels {
			sels = append(sels, map[string]int{"anchor": x.Anchor, "head": x.Head})
		}
		s.Lock()
		trace := append([]inputRecord(nil), s.trace...)
		s.Unlock()
		var body geom.Rect
		focused := false
		if s.view != nil {
			body = s.view.Box().Rect()
			focused = s.view.Focused()
		}
		// After setup a new widget has not been laid out. Expose readiness,
		// don't ask a caller to trust a 0×0 ROI or a previous view's rect.
		ready := body.W > 0 && body.H > 0 && s.st.Viewport.Width > 0
		if ready {
			for _, x := range s.st.Sels {
				top := float64(body.Y) + s.st.LineTop(snap.LineOf(x.Head)) - s.st.ScrollTop()
				rows = append(rows, [2]float64{top, float64(s.st.Viewport.LineHeight)})
			}
		}
		return map[string]any{"version": snap.Version(), "checksum": checksum(snap.String()), "text": snap.String(), "selections": sels, "trace": trace, "focused": focused, "ready": ready,
			"body": []float32{body.X, body.Y, body.W, body.H}, "line20": snap.Line(20), "rows": rows,
			// The renderer now: a native-layer window may have promoted to Metal.
			"renderer": uihost.Renderer(s.runner),
			"scroll":   map[string]float32{"topLine": float32(s.st.Viewport.TopLine), "offsetY": s.st.Viewport.OffsetY, "lineHeight": s.st.Viewport.LineHeight}}, nil
	})
}

func main() {
	out := flag.String("ready", "", "private ready JSON path")
	flag.Parse()
	if !guiprobe.Enabled || *out == "" {
		log.Fatal("fixture: build with -tags tango_guiprobe and pass -ready")
	}
	tokenBytes := make([]byte, 24)
	if _, err := rand.Read(tokenBytes); err != nil {
		log.Fatal(err)
	}
	token := hex.EncodeToString(tokenBytes)
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		log.Fatal(err)
	}
	quit := true
	s := &state{app: tango.New(tango.Options{Name: "Tango GUI Probe", ID: "dev.tango.guiprobe", QuitOnLastClose: &quit})}
	s.st = editor.NewState(gen(2000), "fixture.ts", theme.Base("vs"))
	guiprobe.SetObserver(func(ev input.Event) {
		var r inputRecord
		switch e := ev.(type) {
		case input.Composition:
			r = inputRecord{"composition", e.Text, e.Cursor, hostNS()}
		case input.TextInput:
			r = inputRecord{"text", e.Text, 0, hostNS()}
		default:
			return
		}
		s.Lock()
		if len(s.trace) < 256 {
			s.trace = append(s.trace, r)
		}
		s.Unlock()
	})
	srv := &http.Server{Handler: controlHandler(token, s.control), ReadHeaderTimeout: time.Second}
	go func() { _ = srv.Serve(ln) }()
	s.app.OnReady(func() {
		// Fullscreen makes content fit the display without cropping a titlebar.
		r, _, err := uihost.Open(s.app, uihost.Options{Surface: tango.SurfaceOptions{Title: "Tango GUI Probe", Width: contentW, Height: contentH, Appearance: tango.Appearance{TitleBar: tango.TitleBarNone, NoShadow: true}, Resizable: false}}, func(c *ui.Context) {
			s.view = editor.View(c, s.st).FontSize(14).Layout(layout.Grow(1))
		}, ui.RunnerOptions{Theme: uitheme.AntDesign(), NoAnimations: true})
		if err != nil {
			log.Fatal(err)
		}
		s.runner = r
		var b bytes.Buffer
		_ = json.NewEncoder(&b).Encode(map[string]any{"pid": os.Getpid(), "url": "http://" + ln.Addr().String(), "token": token, "renderer": uihost.Renderer(r)})
		tmp := filepath.Join(filepath.Dir(*out), ".ready.tmp")
		if err := os.WriteFile(tmp, b.Bytes(), 0o600); err == nil {
			_ = os.Rename(tmp, *out)
		}
	})
	if err := s.app.Run(); err != nil {
		log.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	_ = srv.Shutdown(ctx)
}
