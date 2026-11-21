//go:build tango_dev

package uihost

import (
	"bytes"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"time"

	"github.com/tangoui/tango/internal/corehook"
	"github.com/tangoui/tango/internal/devhost"
	"github.com/tangoui/tango/internal/platform"
)

type remoteEmbed struct {
	w         *remoteWin
	id        uint64
	ev        corehook.ProxyWebViewEvents
	placement platform.WebPlacement
	zoom      float64
	closed    bool
	pending   map[int]func(devhost.EmbedResult, error)
}

func (w *remoteWin) webEmbeds() *embeds { return w.emb }

func (n *remoteSurface) NewProxyWebView(id platform.SurfaceID, o platform.SurfaceOptions, web platform.WebOptions, ev corehook.ProxyWebViewEvents) (platform.EmbeddedWebView, error) {
	w := n.w
	v := &remoteEmbed{w: w, id: uint64(id), ev: ev, zoom: 1}
	w.web.Store(v.id, v)
	m, err := w.cl.Request(func(req int) any {
		return devhost.EmbedOpen{Req: req, Win: w.id, View: v.id, Opts: o, Web: web}
	}, 10*time.Second)
	if err == nil {
		r := m.(devhost.EmbedOpened)
		if r.Unsupported {
			err = fmt.Errorf("%s: %w", r.Err, platform.ErrUnsupported)
		} else if r.Err != "" {
			err = errors.New(r.Err)
		}
	}
	if err != nil {
		w.web.Delete(v.id)
		v.op("close", "") // creation may have succeeded after a request timed out
		return nil, err
	}
	return v, nil
}

func (v *remoteEmbed) send(m devhost.EmbedOp) error {
	if v.closed {
		return devhost.ErrClosed
	}
	m.Win, m.View = v.w.id, v.id
	return v.w.cl.Send(m)
}

func (v *remoteEmbed) op(op, text string) { _ = v.send(devhost.EmbedOp{Op: op, Text: text}) }

func (v *remoteEmbed) SetPlacement(p platform.WebPlacement) {
	if p != v.placement {
		v.placement = p
		_ = v.send(devhost.EmbedOp{Op: "place", Placement: p})
	}
}

func (v *remoteEmbed) Size() (int, int)    { return v.placement.Size() }
func (v *remoteEmbed) SetSize(int, int)    {}
func (v *remoteEmbed) SetTitle(string)     {}
func (v *remoteEmbed) Native() uintptr     { return 0 }
func (v *remoteEmbed) Navigate(url string) { v.op("navigate", url) }
func (v *remoteEmbed) Eval(js string)      { v.op("eval", js) }
func (v *remoteEmbed) OpenDevTools()       { v.op("devtools", "") }
func (v *remoteEmbed) Reload()             { v.op("reload", "") }
func (v *remoteEmbed) Print()              { v.op("print", "") }
func (v *remoteEmbed) Zoom() float64       { return v.zoom }
func (v *remoteEmbed) SetZoom(f float64) {
	v.zoom = f
	_ = v.send(devhost.EmbedOp{Op: "zoom", Number: f})
}
func (v *remoteEmbed) Show()     { v.op("show", "") }
func (v *remoteEmbed) Hide()     { v.op("hide", "") }
func (v *remoteEmbed) Focus()    { v.FocusWeb() }
func (v *remoteEmbed) FocusWeb() { v.op("focus", "") }
func (v *remoteEmbed) BlurWeb()  { v.op("blur", "") }

func (v *remoteEmbed) Close() {
	if !v.closed {
		v.op("close", "")
		v.finish()
	}
}

func (v *remoteEmbed) finish() {
	v.closed = true
	v.w.web.Delete(v.id)
	for req, done := range v.pending {
		delete(v.pending, req)
		done(devhost.EmbedResult{}, devhost.ErrClosed)
	}
	v.ev.Event(platform.SurfaceEvent{Kind: platform.SurfaceClosed})
}

func (v *remoteEmbed) Screenshot(done func([]byte, error)) {
	v.request("snapshot", func(r devhost.EmbedResult, err error) { done(r.PNG, err) })
}

func (v *remoteEmbed) GoInHistory(back bool, done func(bool, error)) {
	op := "forward"
	if back {
		op = "back"
	}
	v.request(op, func(r devhost.EmbedResult, err error) { done(r.Moved, err) })
}

func (v *remoteEmbed) request(op string, done func(devhost.EmbedResult, error)) {
	v.w.nextWebReq++
	id := v.w.nextWebReq
	if err := v.send(devhost.EmbedOp{Op: op, Req: id}); err != nil {
		done(devhost.EmbedResult{}, err)
		return
	}
	if v.pending == nil {
		v.pending = make(map[int]func(devhost.EmbedResult, error))
	}
	v.pending[id] = done
}

func (w *remoteWin) embedEvent(m devhost.EmbedEvent) {
	if value, ok := w.web.Load(m.View); ok {
		v := value.(*remoteEmbed)
		if m.NavKind != 0 {
			if v.ev.Navigation != nil {
				v.ev.Navigation(m.NavToken, m.NavKind, m.NavPage, m.NavError)
			}
			return
		}
		switch m.Event.Kind {
		case 0:
			v.ev.Message(m.Origin, m.Message)
		case platform.SurfaceClosed:
			v.finish()
		default:
			v.ev.Event(m.Event)
		}
	}
}

func (w *remoteWin) embedResult(m devhost.EmbedResult) {
	if value, ok := w.web.Load(m.View); ok {
		v := value.(*remoteEmbed)
		if done := v.pending[m.Req]; done != nil {
			delete(v.pending, m.Req)
			var err error
			if m.Unsupported {
				err = fmt.Errorf("%s: %w", m.Err, platform.ErrUnsupported)
			} else if m.Err != "" {
				err = errors.New(m.Err)
			}
			done(m, err)
		}
	}
}

func (w *remoteWin) serveEmbedAsset(m devhost.EmbedAssetRequest) {
	response := devhost.EmbedAssetResponse{Req: m.Req, Status: http.StatusNotFound}
	defer func() { _ = w.cl.Send(response) }()
	value, ok := w.web.Load(m.View)
	if !ok {
		return
	}
	r, err := http.NewRequest(m.Method, m.URL, bytes.NewReader(m.Body))
	if err != nil {
		response.Status = http.StatusBadRequest
		return
	}
	r.Header = m.Header
	out := httptest.NewRecorder()
	v := value.(*remoteEmbed)
	defer func() {
		if recover() != nil {
			response.Status = http.StatusInternalServerError
		}
	}()
	v.ev.Asset(out, r)
	response.Status, response.Header, response.Body = out.Code, out.Header(), out.Body.Bytes()
}
