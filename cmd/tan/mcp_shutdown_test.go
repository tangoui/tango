package main

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	wire "github.com/tangoui/tango/internal/automation"
	"github.com/tangoui/tango/internal/automation/client"
)

// A signal while a long tango_ui_wait is in flight: cleanup starts at
// once, the wait fails fast, the started app is asked to quit (over a
// second connection, the first one being busy) and serving ends.
func TestMCPSignalShutdown(t *testing.T) {
	s, err := newMCPServer(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(s.close)
	waiting := make(chan struct{}, 1)
	addr, token, got := newFakeApp(t, canvasWindow, map[string]func(*wire.Request) *wire.Response{
		wire.OpWait: func(*wire.Request) *wire.Response {
			waiting <- struct{}{}
			return nil // never answered
		},
	})
	conn, err := client.Dial(addr, token)
	if err != nil {
		t.Fatal(err)
	}
	a := &appSession{conn: conn, started: true, exited: make(chan struct{}), exitErr: func() error { return nil }, addr: addr, token: token}
	s.apps.add(a)

	sigs := make(chan os.Signal, 2)
	st, ct := mcp.NewInMemoryTransports()
	served := make(chan error, 1)
	go func() {
		served <- s.serveUntilSignal(sigs, func(ctx context.Context) error { return s.srv.Run(ctx, st) })
	}()
	cs, err := mcp.NewClient(&mcp.Implementation{Name: "test", Version: "0"}, nil).Connect(context.Background(), ct, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer cs.Close()
	called := make(chan struct{})
	go func() {
		defer close(called)
		cs.CallTool(context.Background(), &mcp.CallToolParams{Name: "tango_ui_wait", Arguments: map[string]any{
			"session": a.ID, "query": map[string]any{"role": "button"}, "timeoutMs": 120000,
		}})
	}()
	select {
	case <-waiting:
	case <-time.After(10 * time.Second):
		t.Fatal("the wait never reached the app")
	}

	start := time.Now()
	sigs <- os.Interrupt
	select {
	case err := <-served:
		if err != nil {
			t.Errorf("serveUntilSignal = %v, want nil", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("serveUntilSignal still running 5 s after the signal")
	}
	select {
	case <-called:
	case <-time.After(5 * time.Second):
		t.Fatal("the tool call still running 5 s after the signal")
	}
	if d := time.Since(start); d > 3*time.Second {
		t.Errorf("shutdown took %v", d)
	}
	if o := ops(got()); !slices.Contains(o, wire.OpQuit) {
		t.Errorf("requests %v, want a quit", o)
	}
	if _, _, err := s.track(context.Background()); err == nil {
		t.Error("track after shutdown succeeded")
	}
}

// A second signal while cleanup waits for a job that ignores SIGTERM kills
// it at once and exits.
func TestMCPSecondSignalKills(t *testing.T) {
	s, err := newMCPServer(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(s.close)
	s.jobs = jobScript(t, `trap '' TERM; echo > "$READY"; sleep 60`)
	ready := filepath.Join(t.TempDir(), "ready")
	info, err := s.jobs.start("dev", t.TempDir(), nil, []string{"READY=" + ready})
	if err != nil {
		t.Fatal(err)
	}
	for deadline := time.Now().Add(10 * time.Second); ; time.Sleep(10 * time.Millisecond) {
		if _, err := os.Stat(ready); err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("the job did not set its trap")
		}
	}
	exitCode := make(chan int, 1)
	old := mcpExit
	mcpExit = func(code int) { exitCode <- code }
	t.Cleanup(func() { mcpExit = old })

	sigs := make(chan os.Signal, 2)
	served := make(chan error, 1)
	go func() {
		served <- s.serveUntilSignal(sigs, func(ctx context.Context) error { <-ctx.Done(); return nil })
	}()
	sigs <- os.Interrupt
	time.Sleep(500 * time.Millisecond) // stopAll is in its 5 s grace
	start := time.Now()
	sigs <- os.Interrupt
	select {
	case code := <-exitCode:
		if code != 1 {
			t.Errorf("exit code %d, want 1", code)
		}
	case err := <-served:
		t.Fatalf("serveUntilSignal returned %v before the second signal was handled (after %v)", err, time.Since(start))
	case <-time.After(3 * time.Second):
		t.Fatal("no exit 3 s after the second signal")
	}
	if err := <-served; err == nil {
		t.Error("serveUntilSignal = nil after a second signal")
	}
	j, _ := s.jobs.get(info.ID)
	select {
	case <-j.done:
	case <-time.After(2 * time.Second):
		t.Fatal("the job is still running after the kill")
	}
	if d := time.Since(start); d > 3*time.Second {
		t.Errorf("kill took %v", d)
	}
}
