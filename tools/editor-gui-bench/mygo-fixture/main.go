// Command mygo-fixture is the MyGo counterpart of the Tango GUI acceptance
// fixture (../fixture): MyGo v0.9.0's code editor (ui/editor, self-drawn)
// in a frameless 1920×944 window, the same synthetic document and carets,
// and the same loopback control API (/setup, /state, /quit, bearer token,
// private ready JSON). It changes only its own editor state, on the main
// thread. Build: GOWORK=off go build (a module of its own, so Tango's
// module does not depend on MyGo).
package main

import (
	"bytes"
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

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/editor"
)

// Same content size as the Tango fixture and the visible Monaco viewport.
const contentW, contentH = 1920, 944

type request struct {
	Scenario string `json:"scenario"`
	Line     int    `json:"line,omitempty"`
	Carets   int    `json:"carets,omitempty"`
}

func controlHandler(token string, do func(request) (any, error)) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer "+token {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		if r.Method != http.MethodPost {
			http.Error(w, "method", http.StatusMethodNotAllowed)
			return
		}
		var req request
		dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096))
		dec.DisallowUnknownFields()
		if dec.Decode(&req) != nil || dec.More() {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		switch r.URL.Path {
		case "/state":
			req.Scenario = "state"
		case "/quit":
			req.Scenario = "quit"
		}
		out, err := do(req)
		if err != nil {
			http.Error(w, err.Error(), http.StatusConflict)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(out)
	})
}

// gen and the typing line are the Tango fixture's document (fixture/main.go).
func gen(n int) string {
	var b strings.Builder
	for i := range n {
		fmt.Fprintf(&b, "export const value%d = compute(%d, \"row %d\"); // fixture\n", i, i, i)
	}
	return b.String()
}

const typingLine = 20

// document returns the scenario's text and caret byte offsets: after
// "// qq" on line 20, then two columns into lines 24, 28, ...
func document(scenario string, carets int) (string, []int) {
	n := 2000
	if scenario == "multi" {
		n = 6000
	}
	lines := strings.SplitAfter(gen(n), "\n")
	lines[typingLine] = "// qq" + strings.Repeat(" ", 160) + "\n"
	text := strings.Join(lines, "")
	start := func(line int) int {
		off := 0
		for i := range line {
			off += len(lines[i])
		}
		return off
	}
	offs := []int{start(typingLine) + len("// qq")}
	for k := 1; k < carets; k++ {
		offs = append(offs, start(typingLine+k*4)+2)
	}
	return text, offs
}

func checksum(s string) string {
	h := fnv.New64a()
	_, _ = h.Write([]byte(s))
	return fmt.Sprintf("%016x", h.Sum64())
}

type fixture struct {
	mu          sync.Mutex
	win         *mygo.Window
	st          *editor.State
	accelerated bool
	probed      bool
	pending     []editor.Selection // carets to set once the editor is laid out
}

func (f *fixture) onMain(fn func() (any, error)) (any, error) {
	type result struct {
		v   any
		err error
	}
	ch := make(chan result, 1)
	go mygo.RunOnMain(func() { v, err := fn(); ch <- result{v, err} })
	select {
	case r := <-ch:
		return r.v, r.err
	case <-time.After(5 * time.Second):
		return nil, errors.New("UI thread timeout")
	}
}

func (f *fixture) control(req request) (any, error) {
	return f.onMain(func() (any, error) {
		switch req.Scenario {
		case "quit":
			mygo.App.Quit()
			return map[string]bool{"ok": true}, nil
		case "state":
		case "type", "multi":
			text, offs := document(req.Scenario, max(1, req.Carets))
			// A new Buffer, not Buffer.Reset: the editor then forgets
			// everything of the old text, its horizontal scroll included
			// (which has no setter and Reset keeps), so every setup starts
			// from the same view. The element, and the focus AutoFocus gave
			// it, stay.
			f.st.Buffer = editor.NewBuffer(text)
			sels := make([]editor.Selection, len(offs))
			for i, o := range offs {
				sels[i] = editor.Selection{Anchor: o, Head: o}
			}
			// Carets revealed before the first layout would scroll against a
			// 0-wide view: set them in the first frame that has a size.
			f.pending = sels
			f.win.Invalidate()
		default:
			return nil, fmt.Errorf("unknown scenario %q", req.Scenario)
		}
		text := f.st.Buffer.String()
		var sels []map[string]int
		var rows [][2]float64
		ready := f.st.ViewportHeight() > 0 && f.pending == nil
		snap := f.st.Buffer.Snapshot()
		for _, s := range f.st.Selections() {
			sels = append(sels, map[string]int{"anchor": s.Anchor, "head": s.Head})
			if ready {
				line := snap.LineOf(s.Head)
				top := f.st.LineTop(line) - f.st.ScrollY()
				rows = append(rows, [2]float64{top, f.st.LineTop(line+1) - f.st.LineTop(line)})
			}
		}
		renderer := "unknown"
		if f.probed {
			renderer = map[bool]string{true: "metal", false: "software"}[f.accelerated]
		}
		return map[string]any{"version": f.st.Buffer.Version(), "checksum": checksum(text), "text": text, "selections": sels,
			"focused": f.st.Focused(), "ready": ready, "body": []float64{0, 0, contentW, contentH}, "rows": rows,
			"scroll": map[string]float64{"y": f.st.ScrollY()}, "renderer": renderer}, nil
	})
}

func main() {
	out := flag.String("ready", "", "private ready JSON path")
	flag.Parse()
	if *out == "" {
		log.Fatal("mygo-fixture: pass -ready")
	}
	os.Setenv("MYGO_LOCALE", "en")
	tokenBytes := make([]byte, 24)
	if _, err := rand.Read(tokenBytes); err != nil {
		log.Fatal(err)
	}
	token := hex.EncodeToString(tokenBytes)
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		log.Fatal(err)
	}
	text, _ := document("type", 1)
	f := &fixture{st: &editor.State{Buffer: editor.NewBuffer(text), Font: ui.Font{Size: 14}}}
	srv := &http.Server{Handler: controlHandler(token, f.control), ReadHeaderTimeout: time.Second}
	go func() { _ = srv.Serve(ln) }()
	mygo.App.WhenReady(func() {
		f.win = mygo.NewWindow(mygo.WindowOptions{
			Title: "MyGo GUI Probe", Width: contentW, Height: contentH, UseContentSize: true, Frameless: true,
			Content: ui.View(func(c *ui.Context) {
				c.SetTheme(ui.LightTheme()) // the light theme of the other two
				root := ui.Column(c).Fill()
				if !f.probed {
					root = root.Draw(func(p *ui.Painter, _ ui.Rect) { f.probed, f.accelerated = true, p.Accelerated() })
				}
				if f.pending != nil && f.st.ViewportHeight() > 0 {
					f.st.SetSelections(f.pending...)
					f.pending = nil
				}
				root.Children(func() { editor.View(c, f.st).Grow(1).AutoFocus() })
			}),
		})
		var b bytes.Buffer
		_ = json.NewEncoder(&b).Encode(map[string]any{"pid": os.Getpid(), "url": "http://" + ln.Addr().String(), "token": token, "renderer": "see /state"})
		tmp := filepath.Join(filepath.Dir(*out), ".ready.tmp")
		if err := os.WriteFile(tmp, b.Bytes(), 0o600); err == nil {
			_ = os.Rename(tmp, *out)
		}
	})
	if err := mygo.App.Run(); err != nil {
		log.Fatal(err)
	}
	_ = srv.Close()
}
