package ipc

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"math/rand"
	"sync"
	"testing"
	"time"
)

// openRW opens method in process and returns the client stream.
func openRW(t *testing.T, r *Router, method string) *Stream {
	t.Helper()
	s, err := r.Open(context.Background(), method)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func TestReadWriterCopy(t *testing.T) {
	r := NewRouter()
	r.HandleDuplex("Echo", func(s *Stream, _ json.RawMessage) error {
		rw := s.ReadWriter()
		_, err := io.Copy(rw, rw)
		return err
	})
	s := openRW(t, r, "Echo")
	rw := s.ReadWriter()

	want := make([]byte, 40<<10)
	rand.New(rand.NewSource(1)).Read(want)
	werr := make(chan error, 1)
	go func() {
		// Uneven chunks, so frame boundaries do not line up with reads.
		for off := 0; off < len(want); off += 7000 {
			end := min(off+7000, len(want))
			if n, err := rw.Write(want[off:end]); err != nil || n != end-off {
				werr <- err
				return
			}
		}
		werr <- rw.Close()
	}()

	var got []byte
	buf := make([]byte, 1000)
	for len(got) < len(want) {
		n, err := io.ReadFull(rw, buf[:min(len(buf), len(want)-len(got))])
		if err != nil {
			t.Fatalf("ReadFull after %d bytes: %v", len(got), err)
		}
		got = append(got, buf[:n]...)
	}
	if !bytes.Equal(got, want) {
		t.Fatal("echoed bytes differ")
	}
	if err := within(t, werr, 5*time.Second, "writer"); err != nil {
		t.Fatal(err)
	}
	if n, err := rw.Read(buf); n != 0 || err != io.EOF {
		t.Fatalf("after echo: Read = %d, %v; want 0, io.EOF", n, err)
	}
}

func TestReadWriterSplitLarge(t *testing.T) {
	r := NewRouter()
	type res struct {
		lens []int
		data []byte
		err  error
	}
	got := make(chan res, 1)
	r.HandleDuplex("Sink", func(s *Stream, _ json.RawMessage) error {
		var out res
		for range 2 {
			b, err := s.ReceiveBytes()
			if err != nil {
				out.err = err
				break
			}
			out.lens = append(out.lens, len(b))
			out.data = append(out.data, b...)
		}
		got <- out
		return nil
	})
	s := openRW(t, r, "Sink")

	p := make([]byte, MaxFrameBytes+10)
	rand.New(rand.NewSource(2)).Read(p)
	type wres struct {
		n   int
		err error
	}
	wdone := make(chan wres, 1)
	go func() {
		n, err := s.ReadWriter().Write(p)
		wdone <- wres{n, err}
	}()
	// The second frame fits only once the server read the first, so Write
	// returning proves the server is receiving.
	if w := within(t, wdone, 30*time.Second, "Write returns"); w.err != nil || w.n != len(p) {
		t.Fatalf("Write = %d, %v; want %d, nil", w.n, w.err, len(p))
	}
	out := within(t, got, 30*time.Second, "server receives")
	if out.err != nil {
		t.Fatal(out.err)
	}
	if len(out.lens) != 2 || out.lens[0] != MaxFrameBytes || out.lens[1] != 10 {
		t.Fatalf("frame lengths = %v, want [%d 10]", out.lens, MaxFrameBytes)
	}
	if !bytes.Equal(out.data, p) {
		t.Fatal("split frames do not reassemble to the written bytes")
	}
}

func TestReadWriterJSONFrame(t *testing.T) {
	r := NewRouter()
	r.HandleDuplex("J", func(s *Stream, _ json.RawMessage) error {
		return s.Write(1)
	})
	s := openRW(t, r, "J")
	buf := make([]byte, 16)
	n, err := s.ReadWriter().Read(buf)
	if n != 0 {
		t.Fatalf("Read n = %d, want 0", n)
	}
	wantCode(t, err, CodeBadArgs, "")
	// The JSON frame stays at the head of the queue.
	if m, err := s.Receive(); err != nil || m.Binary || string(m.JSON) != "1" {
		t.Fatalf("Receive after bad Read = %+v, %v", m, err)
	}
	if _, err := s.ReadWriter().Read(buf); err != io.EOF {
		t.Fatalf("then Read = %v, want io.EOF", err)
	}
}

func TestReadWriterCloseIsHalf(t *testing.T) {
	r := NewRouter()
	srvErr := make(chan error, 1)
	r.HandleDuplex("Half", func(s *Stream, _ json.RawMessage) error {
		if _, err := s.ReceiveBytes(); err != io.EOF {
			srvErr <- err
			return nil
		}
		srvErr <- nil
		return s.WriteBytes([]byte("bye"))
	})
	s := openRW(t, r, "Half")
	rw := s.ReadWriter()
	if err := rw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := within(t, srvErr, 5*time.Second, "server sees EOF"); err != nil {
		t.Fatalf("server ReceiveBytes = %v, want io.EOF", err)
	}
	got, err := io.ReadAll(rw)
	if err != nil || string(got) != "bye" {
		t.Fatalf("ReadAll after Close = %q, %v; want \"bye\", nil", got, err)
	}
	if _, err := rw.Write([]byte("x")); err == nil {
		t.Fatal("Write after Close succeeded")
	}
}

// A partially read frame keeps its rest for the next Read, empty frames
// contribute nothing (Read never returns 0, nil for them), and every
// ReadWriter call shares the same leftover.
func TestReadWriterLeftoverAndEmptyFrames(t *testing.T) {
	r := NewRouter()
	r.HandleDuplex("Parts", func(s *Stream, _ json.RawMessage) error {
		for _, p := range []string{"", "abc", "", "", "de"} {
			if err := s.WriteBytes([]byte(p)); err != nil {
				return err
			}
		}
		return nil
	})
	s := openRW(t, r, "Parts")
	rw1, rw2 := s.ReadWriter(), s.ReadWriter()
	if rw1 != rw2 {
		t.Fatal("ReadWriter returned different views")
	}
	if n, err := rw1.Read(nil); n != 0 || err != nil {
		t.Fatalf("Read(nil) = %d, %v", n, err)
	}
	var got []string
	buf := make([]byte, 2)
	for i := 0; ; i++ {
		v := rw1
		if i%2 == 1 {
			v = s.ReadWriter()
		}
		n, err := v.Read(buf)
		if err == io.EOF {
			break
		}
		if err != nil || n == 0 {
			t.Fatalf("Read = %d, %v", n, err)
		}
		got = append(got, string(buf[:n]))
	}
	if want := []string{"ab", "c", "de"}; len(got) != 3 || got[0] != want[0] || got[1] != want[1] || got[2] != want[2] {
		t.Fatalf("reads = %q, want %q", got, want)
	}
}

// An empty Write returns 0, nil and sends no frame (like net.Conn).
func TestReadWriterEmptyWriteSendsNothing(t *testing.T) {
	r := NewRouter()
	got := make(chan []string, 1)
	r.HandleDuplex("Count", func(s *Stream, _ json.RawMessage) error {
		var seen []string
		for {
			m, err := s.Receive()
			if err != nil {
				seen = append(seen, describeErr(err))
				got <- seen
				return nil
			}
			seen = append(seen, describe(m, nil))
		}
	})
	s := openRW(t, r, "Count")
	rw := s.ReadWriter()
	for _, p := range [][]byte{nil, {}, []byte("x"), {}} {
		if n, err := rw.Write(p); n != len(p) || err != nil {
			t.Fatalf("Write(%q) = %d, %v", p, n, err)
		}
	}
	rw.Close()
	want := []string{"bytes [120] (nil=false)", "EOF"}
	if seen := within(t, got, 5*time.Second, "server"); len(seen) != 2 || seen[0] != want[0] || seen[1] != want[1] {
		t.Fatalf("server saw %q, want %q", seen, want)
	}
}

// One reader and one writer may use the view concurrently, through
// separate ReadWriter calls.
func TestReadWriterConcurrentReadWrite(t *testing.T) {
	r := NewRouter()
	r.HandleDuplex("Echo", func(s *Stream, _ json.RawMessage) error {
		_, err := io.Copy(s.ReadWriter(), s.ReadWriter())
		return err
	})
	s := openRW(t, r, "Echo")
	want := make([]byte, 3<<20) // several windows' worth
	rand.New(rand.NewSource(3)).Read(want)
	var wg sync.WaitGroup
	var werr error
	wg.Add(1)
	go func() {
		defer wg.Done()
		w := s.ReadWriter()
		for off := 0; off < len(want); off += 100 << 10 {
			if _, err := w.Write(want[off:min(off+100<<10, len(want))]); err != nil {
				werr = err
				return
			}
		}
		werr = w.Close()
	}()
	got, err := io.ReadAll(s.ReadWriter())
	wg.Wait()
	if werr != nil {
		t.Fatal(werr)
	}
	if err != nil || !bytes.Equal(got, want) {
		t.Fatalf("ReadAll = %d bytes, %v; want %d equal bytes", len(got), err, len(want))
	}
}
