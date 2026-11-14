//go:build android

package android

// IPC fast lanes (platform.BinaryLane): down:android and up:android over
// the page's WebMessageListener channel. The protocol and the state are in
// lanes.go; this file has the surface methods and the JNI side.
//
// Default off: a page reports the lanes only when the WebView supports
// WebMessageListener and ArrayBuffer messages (Tango.laneSupported,
// WebView 116 and later) and TANGO_IPC_LANES_ANDROID was set to an "on"
// value when the page was created (laneCapsFromEnv).
//
// Threads and locks: SendLanePacket runs on any goroutine, under the
// core's lane lock: it takes only the queue's lock and calls Wake after
// releasing it. The main thread's drain posts the queue (flushLanes) and
// Tango.nativeLanePacket hands upstream packets to the core; neither holds
// the queue's lock while calling the core or Java.

/*
#include "jni_android.h"
*/
import "C"

import (
	"os"
	"slices"
	"strings"

	"github.com/tangoui/tango/internal/platform"
)

var _ platform.BinaryLane = (*surface)(nil)

// LaneCaps implements platform.BinaryLane. Any goroutine: laneCaps is set
// before the surface is published and never changes.
func (s *surface) LaneCaps() []string { return slices.Clone(s.laneCaps) }

// SendLanePacket implements platform.BinaryLane: it queues p for the main
// thread, which posts it if gen is the generation of the page's reply
// proxy by then (laneState.flush). Any goroutine; does not block.
func (s *surface) SendLanePacket(gen uint64, p []byte) bool {
	if len(s.laneCaps) == 0 || !s.lanes.push(gen, p) {
		return false
	}
	s.b.laneDirty.Store(true)
	s.b.Wake()
	return true
}

// envLaneCaps returns the lanes of a page created now. Called before the
// surface is published (NewSurface), so the page script of its first
// document already knows.
func envLaneCaps() []string {
	if laneCapsFromEnv(os.Getenv, true) == nil {
		return nil // switched off: do not ask Java
	}
	return laneCapsFromEnv(os.Getenv, javaLaneSupported() && !noArrayBufferHook())
}

// javaLaneSupported asks Tango.laneSupported.
func javaLaneSupported() bool {
	var ok bool
	withEnv(func(e env) {
		r, err := e.callStaticBoolean(mLaneSupported)
		ok = err == nil && r
	})
	return ok
}

// noArrayBufferHook: with the self-test hooks, TANGO_ANDROID_NO_ARRAYBUFFER
// set to anything but "", "0", "false" or "off" makes Go treat the WebView
// as one without ArrayBuffer messages (the capsOff selftest scenario).
func noArrayBufferHook() bool {
	if !testHooks {
		return false
	}
	switch strings.ToLower(strings.TrimSpace(os.Getenv("TANGO_ANDROID_NO_ARRAYBUFFER"))) {
	case "", "0", "false", "off":
		return false
	}
	return true
}

// flushLanes posts the surfaces' queued down packets. Main thread, from
// drain, before the core's queue.
func (b *Backend) flushLanes() {
	if !b.laneDirty.Swap(false) {
		return
	}
	b.mu.Lock()
	ss := make([]*surface, 0, len(b.surfaces))
	for _, s := range b.surfaces {
		ss = append(ss, s)
	}
	b.mu.Unlock()
	for _, s := range ss {
		s.flushLanes()
	}
}

// flushLanes posts this page's queued packets of the proxy generation with
// Page.laneSend and drops the others. Main thread.
func (s *surface) flushLanes() {
	if s.closed || !s.lanes.pending() {
		return
	}
	withEnv(func(e env) {
		s.lanes.flush(func(p []byte) bool {
			a := e.bytes(p)
			if a == 0 {
				e.check()
				return false
			}
			ok, err := e.callBoolean(s.page, mLaneSend, jvObj(a))
			e.deleteLocal(a) // the frame holds 32 references
			return err == nil && ok
		})
	})
}

// lanePacket handles an upstream lane message from the page's listener
// (Tango.nativeLanePacket, main thread); see handleUp for the result.
func (s *surface) lanePacket(h platform.Host, src string, data []byte) int {
	if s.closed || len(s.laneCaps) == 0 {
		return 0
	}
	lh, _ := h.(platform.LaneHost)
	return handleUp(s.lanes, s.tokens, src, data, func(origin string, gen uint64, p []byte) bool {
		return lh != nil && lh.OnLanePacket(s.id, origin, gen, p)
	})
}
