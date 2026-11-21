package host

import (
	"errors"
	"io"
	"net/http"
	"time"

	"github.com/tangoui/tango/internal/corehook"
	"github.com/tangoui/tango/internal/devhost"
	"github.com/tangoui/tango/internal/platform"
)

type hostedEmbed struct {
	w         *worker
	x         *win
	id        uint64
	ev        platform.EmbeddedWebView
	placement platform.WebPlacement
	show      bool
}

func (h *Host) openEmbed(w *worker, m devhost.EmbedOpen) {
	x := h.wins[m.Win]
	reply := devhost.EmbedOpened{Req: m.Req}
	defer func() { w.send(reply) }()
	if x == nil || x.owner != w && x.next != w || w.embeds[m.View] != nil {
		reply.Err = "embedded WebView belongs to an unavailable window"
		return
	}
	v := &hostedEmbed{w: w, x: x, id: m.View, show: true}
	ev, err := corehook.HostProxyWebView(x.surf, m.Opts, m.Web, corehook.ProxyWebViewEvents{
		Event: func(ev platform.SurfaceEvent) {
			if ev.Kind == platform.SurfaceClosed {
				delete(w.embeds, v.id)
			}
			w.send(devhost.EmbedEvent{Win: x.id, View: v.id, Event: ev})
		},
		Message: func(origin, msg string) {
			w.send(devhost.EmbedEvent{Win: x.id, View: v.id, Origin: origin, Message: msg})
		},
		Asset: v.serveAsset,
	})
	if err != nil {
		reply.Err, reply.Unsupported = err.Error(), errors.Is(err, platform.ErrUnsupported)
		return
	}
	v.ev = ev
	if w.embeds == nil {
		w.embeds = map[uint64]*hostedEmbed{}
	}
	w.embeds[v.id] = v
}

func (v *hostedEmbed) place() {
	p := v.placement
	if !v.show || v.x.owner != v.w {
		p.Clip = platform.RectF{}
	}
	v.ev.SetPlacement(p)
}

func (h *Host) embedOp(w *worker, m devhost.EmbedOp) {
	v := w.embeds[m.View]
	if v == nil || v.x.id != m.Win || v.x.owner != w && v.x.next != w {
		if m.Req != 0 {
			w.send(devhost.EmbedResult{Win: m.Win, View: m.View, Req: m.Req, Err: "embedded WebView closed"})
		}
		return
	}
	switch m.Op {
	case "place":
		v.placement = m.Placement
		v.place()
	case "show", "hide":
		v.show = m.Op == "show"
		v.place()
	case "focus":
		if v.x.owner == w && v.show && !v.placement.Clip.Empty() {
			v.ev.FocusWeb()
		}
	case "blur":
		v.ev.BlurWeb()
	case "close":
		v.ev.Close()
	case "navigate":
		v.ev.Navigate(m.Text)
	case "eval":
		v.ev.Eval(m.Text)
	case "devtools":
		v.ev.OpenDevTools()
	case "reload":
		if page, ok := v.ev.(platform.PageControl); ok {
			page.Reload()
		}
	case "zoom":
		if page, ok := v.ev.(platform.PageControl); ok {
			page.SetZoom(m.Number)
		}
	case "print":
		if page, ok := v.ev.(platform.PageControl); ok {
			page.Print()
		}
	case "snapshot":
		done := func(png []byte, err error) {
			r := devhost.EmbedResult{Win: m.Win, View: m.View, Req: m.Req, PNG: png}
			if err != nil {
				r.Err = err.Error()
			}
			w.send(r)
		}
		if snap, ok := v.ev.(platform.WebSnapshot); ok {
			snap.Screenshot(done)
		} else {
			done(nil, platform.ErrUnsupported)
		}
	}
}

func (w *worker) closeEmbeds(x *win) {
	for _, v := range w.embeds {
		if x == nil || x == v.x {
			v.ev.Close()
		}
	}
}

func (v *hostedEmbed) serveAsset(out http.ResponseWriter, r *http.Request) {
	w := v.w
	m := devhost.EmbedAssetRequest{Win: v.x.id, View: v.id, Method: r.Method, URL: r.URL.String(), Header: r.Header}
	if r.Body != nil {
		const limit = 64 << 20
		body, err := io.ReadAll(io.LimitReader(r.Body, limit+1))
		if err != nil || len(body) > limit {
			http.Error(out, "invalid asset request body", http.StatusBadRequest)
			return
		}
		m.Body = body
	}
	ch := make(chan devhost.EmbedAssetResponse, 1)
	w.mu.Lock()
	w.nextAsset++
	m.Req = w.nextAsset
	if w.assets == nil {
		w.assets = map[int]chan devhost.EmbedAssetResponse{}
	}
	w.assets[m.Req] = ch
	w.mu.Unlock()
	defer func() {
		w.mu.Lock()
		delete(w.assets, m.Req)
		w.mu.Unlock()
	}()
	if err := w.conn.Send(m); err != nil {
		http.Error(out, "dev worker disconnected", http.StatusBadGateway)
		return
	}
	timer := time.NewTimer(10 * time.Second)
	defer timer.Stop()
	select {
	case a := <-ch:
		for name, values := range a.Header {
			out.Header()[name] = values
		}
		out.WriteHeader(a.Status)
		_, _ = out.Write(a.Body)
	case <-r.Context().Done():
	case <-w.connDone:
		http.Error(out, "dev worker disconnected", http.StatusBadGateway)
	case <-timer.C:
		http.Error(out, "dev worker asset request timed out", http.StatusGatewayTimeout)
	}
}

func (w *worker) assetResponse(m devhost.EmbedAssetResponse) {
	w.mu.Lock()
	if ch := w.assets[m.Req]; ch != nil && m.Status >= 100 && m.Status <= 599 {
		delete(w.assets, m.Req)
		ch <- m
	}
	w.mu.Unlock()
}
