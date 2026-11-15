package main

import (
	"io"
	"strings"
	"testing"
)

func TestCheckFlags(t *testing.T) {
	for _, tc := range []struct {
		name string
		o    runFlags
		bad  string // part of the error, "" for none
	}{
		{"shell", runFlags{shell: "/bin/zsh -f"}, ""},
		{"blank shell", runFlags{shell: "  "}, "-shell"},
		{"echo and replay", runFlags{echo: true, replay: "x.vt"}, "-echo"},
		{"auto and bench", runFlags{auto: true, bench: "f"}, "-auto"},
		{"auto with replay", runFlags{auto: true, replay: "x.vt"}, "-auto"},
		{"auto with echo", runFlags{auto: true, echo: true}, ""},
		{"bench with echo", runFlags{bench: "f", echo: true}, "-bench-cat"},
		{"bench with replay", runFlags{bench: "f", replay: "x.vt"}, "-bench-cat"},
	} {
		err := checkFlags(tc.o)
		switch {
		case tc.bad == "" && err != nil:
			t.Errorf("%s: unexpected error %v", tc.name, err)
		case tc.bad != "" && (err == nil || !strings.Contains(err.Error(), tc.bad)):
			t.Errorf("%s: error %v, want one about %s", tc.name, err, tc.bad)
		}
	}
}

// The marker is found in the program's output even when a read splits it.
func TestCountConnSeesMarkerAcrossReads(t *testing.T) {
	out := "lots of output\r\ntango-bench-cat-done\r\n$ "
	for split := 1; split < len(out); split++ {
		c := &countConn{Conn: &chunkConn{chunks: []string{out[:split], out[split:]}}}
		c.watch("tango-bench-cat-done")
		buf := make([]byte, 64)
		for {
			if _, err := c.Read(buf); err != nil {
				break
			}
		}
		if !c.seen.Load() {
			t.Fatalf("split at %d: marker not seen", split)
		}
		if int(c.n.Load()) != len(out) {
			t.Fatalf("split at %d: counted %d bytes, want %d", split, c.n.Load(), len(out))
		}
	}
	c := &countConn{Conn: &chunkConn{chunks: []string{"tango-bench-cat-don", "x"}}}
	c.watch("tango-bench-cat-done")
	buf := make([]byte, 64)
	for {
		if _, err := c.Read(buf); err != nil {
			break
		}
	}
	if c.seen.Load() {
		t.Fatal("a near miss was taken for the marker")
	}
}

func TestShellQuote(t *testing.T) {
	for in, want := range map[string]string{
		"/tmp/a.txt":    "'/tmp/a.txt'",
		"/tmp/a b;rm x": "'/tmp/a b;rm x'",
		"/tmp/it's.txt": `'/tmp/it'\''s.txt'`,
	} {
		if got := shellQuote(in); got != want {
			t.Errorf("shellQuote(%q) = %s, want %s", in, got, want)
		}
	}
}

// chunkConn returns its chunks one Read each, then EOF.
type chunkConn struct{ chunks []string }

func (c *chunkConn) Read(b []byte) (int, error) {
	if len(c.chunks) == 0 {
		return 0, io.EOF
	}
	n := copy(b, c.chunks[0])
	c.chunks = c.chunks[1:]
	return n, nil
}
func (c *chunkConn) Write(b []byte) (int, error)       { return len(b), nil }
func (c *chunkConn) Resize(cols, rows, w, h int) error { return nil }
func (c *chunkConn) Close() error                      { return nil }
