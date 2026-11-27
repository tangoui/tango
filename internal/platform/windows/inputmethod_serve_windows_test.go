//go:build windows && (amd64 || arm64)

package windows

import (
	"os"
	"strconv"
	"testing"
	"time"

	winio "github.com/Microsoft/go-winio"
	"github.com/tangoui/tango/internal/imebridge/proto"
)

func TestTIPPersistentPipe(t *testing.T) {
	sink := &imeSink{}
	sessions := newTIPSessions(sink, []proto.Mode{{ID: "zh"}})
	name := "Test_" + strconv.Itoa(os.Getpid()) + "_" + strconv.FormatInt(time.Now().UnixNano(), 10)
	main := make(chan func(), 32)
	server, err := newTIPServer(name, sessions, func(fn func()) { main <- fn })
	if err != nil {
		t.Fatal(err)
	}
	stopMain := make(chan struct{})
	defer close(stopMain)
	defer server.close()
	go func() {
		for {
			select {
			case <-stopMain:
				return
			case fn := <-main:
				fn()
			}
		}
	}()
	go server.accept()
	timeout := time.Second
	client, err := winio.DialPipe(server.listener.Addr().String(), &timeout)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	_ = client.SetDeadline(time.Now().Add(timeout))
	send := func(seq uint64, payload any) {
		t.Helper()
		frame, err := proto.Marshal(proto.Message{Seq: seq, Body: payload})
		if err == nil {
			_, err = client.Write(frame)
		}
		if err != nil {
			t.Fatal(err)
		}
	}
	send(0, proto.Hello{PID: uint32(os.Getpid())})
	if msg, err := proto.Read(client); err != nil || msg.Body != (proto.Mode{ID: "zh"}) {
		t.Fatalf("initial mode = %+v, %v", msg, err)
	}
	send(0, proto.SessionOpen(7))
	send(0, proto.Focus{Session: 7, On: true})
	for seq := uint64(1); seq <= 2; seq++ {
		send(seq, proto.TestKey{Session: 7, VK: 'A', Flags: proto.KeyDown})
		msg, err := proto.Read(client)
		if err != nil || msg.Seq != seq || msg.Body != (proto.KeyResult{Session: 7, Eaten: true}) {
			t.Fatalf("reply %d = %+v, %v", seq, msg, err)
		}
	}
}
