//go:build unix

package main

import (
	"context"
	"encoding/json"
	"errors"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/tangoui/tango/ipc"
)

var pgidLine = regexp.MustCompile(`pgid=(\d+)\r?\n`)

// TestTermKillsRunningCommand: ending the stream while the shell runs a
// command kills the command too, and the handler returns at once. If only
// sh died, sleep would live on holding the output pipe, and the handler
// would wait for it (forever, or for cmd.WaitDelay).
func TestTermKillsRunningCommand(t *testing.T) {
	for _, end := range []string{"cancel", "close"} {
		t.Run(end, func(t *testing.T) {
			returned := make(chan struct{})
			r := ipc.NewRouter()
			r.HandleDuplex("Term", func(s *ipc.Stream, args json.RawMessage) error {
				defer close(returned)
				return term(s, args)
			})
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			s, err := r.Open(ctx, "Term")
			if err != nil {
				t.Fatal(err)
			}
			rw := s.ReadWriter()

			// The shell leads its own session and process group: $$ is the
			// group id.
			if _, err := rw.Write([]byte("echo pgid=$$\nsleep 30\n")); err != nil {
				t.Fatal(err)
			}
			var out []byte
			buf := make([]byte, 4096)
			pgid := 0
			for pgid == 0 {
				n, err := rw.Read(buf)
				if err != nil {
					t.Fatalf("read: %v (output %q)", err, out)
				}
				out = append(out, buf[:n]...)
				// bash echoes the command line ("pgid=$$") before the result.
				if m := pgidLine.FindSubmatch(out); m != nil {
					pgid, _ = strconv.Atoi(string(m[1]))
				}
			}
			sleepPid := 0
			for deadline := time.Now().Add(3 * time.Second); sleepPid == 0; {
				o, _ := exec.Command("pgrep", "-P", strconv.Itoa(pgid), "-x", "sleep").Output()
				sleepPid, _ = strconv.Atoi(strings.TrimSpace(string(o)))
				if sleepPid == 0 && time.Now().After(deadline) {
					t.Fatalf("no sleep under the shell %d (output %q)", pgid, out)
				}
				time.Sleep(20 * time.Millisecond)
			}

			if end == "cancel" {
				cancel()
			} else {
				s.Close()
			}
			t0 := time.Now()
			// Well under cmd.WaitDelay (2s): the kill, not the delay, ends it.
			select {
			case <-returned:
				t.Logf("handler returned after %v", time.Since(t0))
			case <-time.After(time.Second):
				t.Fatal("Term handler still running 1s after the stream ended")
			}
			for deadline := time.Now().Add(3 * time.Second); !errors.Is(syscall.Kill(sleepPid, 0), syscall.ESRCH); {
				if time.Now().After(deadline) {
					t.Fatalf("sleep %d still alive after the stream ended", sleepPid)
				}
				time.Sleep(20 * time.Millisecond)
			}
		})
	}
}
