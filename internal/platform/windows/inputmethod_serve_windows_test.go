//go:build windows && (amd64 || arm64)

package windows

import (
	"encoding/binary"
	"net"
	"testing"
	"time"
	"unicode/utf16"

	"github.com/tangoui/tango/internal/platform"
)

// A key handler that outlasts the bridge's deadline must not lose the
// text it committed: the next Poll delivers it.
func TestTIPServeKeepsLateResponse(t *testing.T) {
	sink := &tipTestSink{}
	server := &tipServer{tipSessions: newTIPSessions(sink, func(uint32) string { return "" })}
	main := make(chan func(), 8)
	done := make(chan struct{})
	defer close(done)
	go func() {
		for {
			select {
			case fn := <-main:
				fn()
			case <-done:
				return
			}
		}
	}()
	runMain := func(fn func()) { main <- fn }
	onMain := func(fn func()) {
		finished := make(chan struct{})
		runMain(func() { fn(); close(finished) })
		<-finished
	}

	token := [16]byte{3}
	onMain(func() { server.dispatch(tipRequest{Version: tipVersion, Action: tipActivate, Token: token}) })
	sink.onKey = func(id uint64, _ platform.IMEKey) bool {
		server.byID[id].commit("慢")
		time.Sleep(150 * time.Millisecond)
		return true
	}
	client, conn := net.Pipe()
	served := make(chan struct{})
	go func() { server.serve(conn, runMain); close(served) }()
	if err := binary.Write(client, binary.LittleEndian, tipRequest{Version: tipVersion, Action: tipKey, Token: token, VK: 'M'}); err != nil {
		t.Fatal(err)
	}
	// The bridge gives up waiting, like tip.cpp's read timeout.
	client.Close()
	<-served

	var poll tipResponse
	deadline := time.Now().Add(2 * time.Second)
	for poll.Flags&tipCommit == 0 && time.Now().Before(deadline) {
		onMain(func() { poll = server.dispatch(tipRequest{Version: tipVersion, Action: tipPoll, Token: token}) })
		time.Sleep(10 * time.Millisecond)
	}
	if got := string(utf16.Decode(poll.Committed[:poll.CommittedLength])); poll.Flags&tipCommit == 0 || got != "慢" {
		t.Fatalf("poll after a late response: flags %d, committed %q", poll.Flags, got)
	}
}
