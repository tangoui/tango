//go:build ios

package ios

/*
#include <stdlib.h>
#include "bridge.h"
*/
import "C"

import (
	"context"
	"net/http"
	"strings"
	"unsafe"

	"github.com/tangoui/tango/internal/platform"
)

// The tango:// scheme handler (bridge.m) hands each WKURLSchemeTask to
// tangoIOSTaskStart on the main thread, which turns it into an
// *http.Request and runs Host.ServeAsset on a new goroutine, so a slow or
// long-lived response (an IPC fast lane's down stream) never holds the
// main thread. The writer (scheme_writer.go) queues the response and kicks
// a drain through tg_task_kick: a block on the main run loop in the common
// modes, which also runs inside Tango's nested run loops (tg_step,
// AwaitAsync) — the main dispatch queue would not. The drain
// (tangoIOSTaskDrain) makes the task calls with tg_task_respond / _data /
// _finish on the main thread, which copy what they are given. When WebKit
// stops the task (webView:stopURLSchemeTask:) the request's context is
// cancelled, blocked writes return context.Canceled, and nothing more
// reaches the task.

// schemeTask is one WKURLSchemeTask whose handler was started. It lives in
// backend.tasks (main thread only) until its didFinish ran or WebKit
// stopped it.
type schemeTask struct {
	w      *responseWriter
	cancel context.CancelFunc
	lane   *laneTrack // lane request counters (test hook, lanes.go); usually nil
}

//export tangoIOSTaskStart
func tangoIOSTaskStart(sid, handle C.uint64_t, method, rawURL, headers *C.char, body unsafe.Pointer, n C.int) {
	b := current.Load()
	if b == nil || !b.ready.Load() {
		failTask(handle, http.StatusServiceUnavailable, "app not running")
		return
	}
	var data []byte
	if n > 0 {
		data = C.GoBytes(body, n)
	}
	ctx, cancel := context.WithCancel(context.Background())
	req, err := buildRequest(ctx, C.GoString(method), C.GoString(rawURL), C.GoString(headers), data)
	if err != nil {
		cancel()
		failTask(handle, http.StatusBadRequest, err.Error())
		return
	}
	lt := laneTrackFor(req.URL.Path)
	sink := &nativeTask{b: b, handle: handle, lane: lt}
	w := newResponseWriter(ctx, sink, func() { C.tg_task_kick(handle) }, req.URL.Path)
	b.tasks[uint64(handle)] = &schemeTask{w: w, cancel: cancel, lane: lt}
	h := b.h
	go func() {
		defer cancel()
		runAssetHandler(w, func() { h.ServeAsset(platform.SurfaceID(sid), w, req) })
	}()
}

// tangoIOSTaskDrain is a kicked drain (tg_task_kick), on the main thread.
// A task that finished or was stopped is no longer in b.tasks.
//
//export tangoIOSTaskDrain
func tangoIOSTaskDrain(handle C.uint64_t) {
	b := current.Load()
	if b == nil {
		return
	}
	if t := b.tasks[uint64(handle)]; t != nil {
		t.w.drain()
	}
}

//export tangoIOSTaskStop
func tangoIOSTaskStop(handle C.uint64_t) {
	b := current.Load()
	if b == nil {
		return
	}
	if t := b.tasks[uint64(handle)]; t != nil {
		delete(b.tasks, uint64(handle))
		t.w.stop()
		t.cancel()
		t.lane.done()
	}
}

// nativeTask is the writer's taskSink: the WKURLSchemeTask calls, made on
// the main thread by the writer's drain. bridge.m copies the bytes before
// each call returns.
type nativeTask struct {
	b      *backend
	handle C.uint64_t
	lane   *laneTrack
}

func (t *nativeTask) respond(status int, h http.Header) {
	hs, free := cstring(headerLines(h))
	defer free()
	C.tg_task_respond(t.handle, C.int(status), hs)
	t.lane.respond(status)
}

func (t *nativeTask) data(p []byte) {
	if len(p) > 0 {
		C.tg_task_data(t.handle, unsafe.Pointer(&p[0]), C.size_t(len(p)))
		t.lane.data()
	}
}

func (t *nativeTask) finish() {
	C.tg_task_finish(t.handle)
	delete(t.b.tasks, uint64(t.handle))
	t.lane.done()
}

// failTask answers a task that never reaches a handler. Main thread.
func failTask(handle C.uint64_t, status int, msg string) {
	hs, free := cstring(headerLines(http.Header{"Content-Type": {"text/plain; charset=utf-8"}}))
	defer free()
	C.tg_task_respond(handle, C.int(status), hs)
	if msg != "" {
		p := []byte(msg)
		C.tg_task_data(handle, unsafe.Pointer(&p[0]), C.size_t(len(p)))
	}
	C.tg_task_finish(handle)
}

// headerLines formats h as the "Key: value\n" lines bridge.m reads.
func headerLines(h http.Header) string {
	var sb strings.Builder
	for k, vs := range h {
		sb.WriteString(k)
		sb.WriteString(": ")
		sb.WriteString(strings.Join(vs, ", "))
		sb.WriteByte('\n')
	}
	return sb.String()
}
