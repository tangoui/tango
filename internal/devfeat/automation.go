//go:build tango_dev

package devfeat

// Whole-app automation (docs/automation.md): in development builds, when
// TANGO_AUTOMATION names a loopback address, the app opens a control
// channel that tests (package github.com/tangoui/tango/automation) use to
// list windows, drive self-drawn windows through their semantics tree
// (package uihost registers them), evaluate JavaScript in WebView windows,
// take screenshots and quit. Only built with -tags tango_dev: production
// builds, the default, compile all of this out (release.go), since any
// process of the same user can set environment variables.

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/tangoui/tango/internal/automation"
	"github.com/tangoui/tango/internal/corehook"
	"github.com/tangoui/tango/internal/platform"
)

// autoSurface is the part of *tango.Surface's exported API automation
// uses.
type autoSurface interface {
	ID() platform.SurfaceID
	Title() string
	IsVisible() bool
	IsFocused() bool
	Size() (w, h int)
	Screenshot() ([]byte, error)
	Eval(js string)
}

const (
	whereAutomation   = "automation"
	automationChannel = "automation" // internal page→Go channel of Eval results
)

// AttachAutomation starts the control channel of a (a *tango.App) once the
// app is ready when TANGO_AUTOMATION is set. The variables are then
// removed from the environment so child processes (helpers, other Tango
// apps) do not try to open a second channel; under tan dev the host leaves
// them to the worker. Called by package tango for every new App.
func AttachAutomation(a App) {
	addr := os.Getenv(automation.EnvAddr)
	if addr == "" {
		return
	}
	tokenFile := os.Getenv(automation.EnvTokenFile)
	switch os.Getenv("TANGO_DEV_ROLE") {
	case "host":
		return // the hot-reload host runs no app code; the worker gets the channel
	case "":
		os.Unsetenv(automation.EnvAddr)
		os.Unsetenv(automation.EnvTokenFile)
	}
	if _, _, err := automation.ParseAddr(addr); err != nil {
		log.Printf("tango: %s ignored: %v", automation.EnvAddr, err)
		return
	}
	h := &autoHost{a: a, pending: map[uint64]chan autoResult{}}
	corehook.HandleInternal(a, automationChannel, h.result)
	a.OnReady(func() {
		srv, err := automation.Listen(a, h, addr, tokenFile)
		if err != nil {
			log.Printf("tango: automation: %v", err)
			return
		}
		go func() {
			<-corehook.Done(a)
			srv.Close()
		}()
	})
}

// autoHost implements automation.Host for an App.
type autoHost struct {
	a      App
	nextID atomic.Uint64

	mu      sync.Mutex
	pending map[uint64]chan autoResult
}

type autoResult struct {
	ok    bool
	value string // JSON text
	err   string
}

func (h *autoHost) Main(fn func()) bool {
	ok := false
	if !corehook.Await(h.a, func() { ok = corehook.Guard(h.a, whereAutomation, fn) }) {
		return false
	}
	return ok
}

func (h *autoHost) Guard(fn func()) bool { return corehook.Guard(h.a, whereAutomation, fn) }

func (h *autoHost) Quit() { h.a.Quit() }

// Windows runs on the main thread.
func (h *autoHost) Windows() []automation.WindowInfo {
	var out []automation.WindowInfo
	for _, surface := range corehook.Surfaces(h.a) {
		if corehook.SurfaceClosed(surface) {
			continue
		}
		s := surface.(autoSurface)
		w, ht := s.Size()
		out = append(out, automation.WindowInfo{
			ID: uint64(s.ID()), Title: s.Title(), Visible: s.IsVisible(), Focused: s.IsFocused(),
			Width: w, Height: ht,
		})
	}
	return out
}

func (h *autoHost) surface(win uint64) (autoSurface, error) {
	s := corehook.SurfaceOf(h.a, platform.SurfaceID(win))
	if s == nil || corehook.SurfaceClosed(s) {
		return nil, &automation.Error{Code: automation.CodeNoWindow, Msg: fmt.Sprintf("automation: no window %d", win)}
	}
	return s.(autoSurface), nil
}

func (h *autoHost) Screenshot(win uint64) ([]byte, error) {
	s, err := h.surface(win)
	if err != nil {
		return nil, err
	}
	p, err := s.Screenshot()
	if errors.Is(err, platform.ErrUnsupported) {
		return nil, &automation.Error{Code: automation.CodeUnsupported, Msg: "automation: this platform cannot capture WebView windows"}
	}
	return p, err
}

// Eval wraps expr so the page posts its (awaited) JSON value back on the
// internal automation channel.
func (h *autoHost) Eval(win uint64, expr string, timeout time.Duration) ([]byte, error) {
	s, err := h.surface(win)
	if err != nil {
		return nil, err
	}
	// A script evaluated before the page has loaded runs in the previous
	// (or blank) document and its answer is lost: wait for Loaded first.
	deadline := time.Now().Add(timeout)
	for {
		var loaded bool
		ok := corehook.Await(h.a, func() { loaded = corehook.SurfaceLoaded(s) })
		if !ok || corehook.SurfaceClosed(s) {
			return nil, &automation.Error{Code: automation.CodeNoWindow, Msg: fmt.Sprintf("automation: window %d closed", win)}
		}
		if loaded {
			break
		}
		if time.Now().After(deadline) {
			return nil, &automation.Error{Code: automation.CodeTimeout, Msg: fmt.Sprintf(
				"automation: the page of window %d did not finish loading within %v", win, timeout)}
		}
		time.Sleep(20 * time.Millisecond)
	}
	timeout = time.Until(deadline)
	id := h.nextID.Add(1)
	ch := make(chan autoResult, 1)
	h.mu.Lock()
	h.pending[id] = ch
	h.mu.Unlock()
	defer func() {
		h.mu.Lock()
		delete(h.pending, id)
		h.mu.Unlock()
	}()
	s.Eval(evalScript(id, expr))
	select {
	case r := <-ch:
		if !r.ok {
			return nil, &automation.Error{Code: automation.CodeScript, Msg: "automation: script error: " + r.err}
		}
		if r.value == "" {
			return []byte("null"), nil
		}
		return []byte(r.value), nil
	case <-time.After(timeout):
		return nil, &automation.Error{Code: automation.CodeTimeout, Msg: fmt.Sprintf(
			"automation: script gave no result within %v (syntax error, page not loaded, or a page without the Tango bridge)", timeout)}
	case <-corehook.Done(h.a):
		return nil, &automation.Error{Code: automation.CodeQuit, Msg: "automation: the app has quit"}
	}
}

// result receives a page's answer (main thread).
func (h *autoHost) result(_ any, payload string) {
	var m struct {
		ID    uint64 `json:"id"`
		OK    bool   `json:"ok"`
		Value string `json:"value"`
		Error string `json:"error"`
	}
	if json.Unmarshal([]byte(payload), &m) != nil {
		return
	}
	h.mu.Lock()
	ch := h.pending[m.ID]
	h.mu.Unlock()
	if ch != nil {
		select {
		case ch <- autoResult{ok: m.OK, value: m.Value, err: m.Error}:
		default:
		}
	}
}

// evalScript builds the page script for one Eval. expr is embedded as
// source (not passed to eval()), so a Content-Security-Policy without
// 'unsafe-eval' does not block it. A Promise result is awaited; the value
// travels as JSON text.
func evalScript(id uint64, expr string) string {
	expr = strings.TrimRight(strings.TrimSpace(expr), "; \t\r\n")
	prefix := strconv.Quote(corehook.InternalPrefix + automationChannel + ":")
	return `(function(){var __id=` + strconv.FormatUint(id, 10) + `;` +
		`var __post=function(m){try{window.__tango.post(` + prefix + `+JSON.stringify(m))}catch(e){}};` +
		`var __err=function(e){__post({id:__id,ok:false,error:String(e&&e.stack?e.message+"\n"+e.stack:e)})};` +
		`try{Promise.resolve((function(){return (` + "\n" + expr + "\n" + `)})()).then(function(v){var s;` +
		`try{s=JSON.stringify(v===undefined?null:v)}catch(e){s=JSON.stringify(String(v))}` +
		`__post({id:__id,ok:true,value:s===undefined?"null":s})},__err)}catch(e){__err(e)}})();`
}
