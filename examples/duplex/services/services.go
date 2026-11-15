// Package services holds the ipc services shared by the duplex examples
// (examples/duplex on the desktop, examples/duplex-mobile on iOS).
package services

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"sync"

	"github.com/tangoui/tango/ipc"
)

// benchDone, when set (SetBenchDone), runs after Bench.Report printed its
// line; examples/duplex -bench uses it to quit the app.
var (
	benchMu   sync.Mutex
	benchDone func()
)

// SetBenchDone sets the function Bench.Report runs after printing its line.
func SetBenchDone(fn func()) {
	benchMu.Lock()
	benchDone = fn
	benchMu.Unlock()
}

// Register registers the platform-independent services of the duplex
// examples on r. examples/duplex adds its local shell ("Term") on top;
// examples/duplex-mobile uses these as they are:
//
//   - "Echo" (duplex): writes back every frame as received, JSON or bytes.
//   - "Upload" (duplex, [name, size]): byte frames go to a temporary file;
//     {"received":n} every further MiB, {"path":p,"bytes":n} after the end.
//   - "Sink" (duplex): reads every frame, then replies {"bytes":n}.
//   - "Source" (stream, [frames, size, mod?, add?]): writes frames byte
//     frames of size bytes each. Without mod every frame holds byte(i) at
//     offset i; with mod > 0, byte j of the whole stream (counted across
//     frames) is (j + add) % mod.
//   - "Bench.Report" (call): prints its argument as one JSON line on stdout.
//   - "Debug.Stats" (call), only with TANGO_SCHEME_PROBE=1: Go-side
//     counters for the platform tests (Stats, testroutes.go).
func Register(r *ipc.Router) {
	r.HandleDuplex("Echo", echo)
	r.HandleDuplex("Upload", upload)
	r.HandleDuplex("Sink", sink)
	r.HandleStream("Source", source)
	r.Handle("Bench.Report", benchReport)
	if Probe() {
		r.Handle("Debug.Stats", debugStats)
	}
}

// echo writes back every frame until the peer ends its direction.
func echo(s *ipc.Stream, _ json.RawMessage) error {
	for {
		m, err := s.Receive()
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return err
		}
		if m.Binary {
			err = s.WriteBytes(m.Bytes)
		} else {
			err = s.Write(m.JSON)
		}
		if err != nil {
			return err
		}
	}
}

const mib = 1 << 20

// upload stores the byte frames in a temporary file, reporting progress
// every MiB, and replies with the file's path and size after the end.
func upload(s *ipc.Stream, args json.RawMessage) error {
	var name string
	var size int64
	if err := decodeArgs(args, &name, &size); err != nil {
		return ipc.NewError(ipc.CodeBadArgs, "Upload(name string, size int): "+err.Error())
	}
	f, err := os.CreateTemp("", "tango-upload-*")
	if err != nil {
		return err
	}
	keep := false
	defer func() {
		if !keep {
			f.Close()
			os.Remove(f.Name())
		}
	}()
	var n, reported int64
	for {
		b, err := s.ReceiveBytes()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return err
		}
		if _, err := f.Write(b); err != nil {
			return err
		}
		n += int64(len(b))
		if n/mib > reported/mib {
			reported = n
			if err := s.Write(map[string]int64{"received": n}); err != nil {
				return err
			}
		}
	}
	if err := f.Close(); err != nil {
		return err
	}
	keep = true
	if size > 0 && n != size {
		// Still reported: the page compares bytes with what it sent.
		fmt.Fprintf(os.Stderr, "upload %q: got %d bytes, announced %d\n", name, n, size)
	}
	return s.Write(map[string]any{"path": f.Name(), "bytes": n})
}

// sink counts the payload bytes of every frame and replies with the total
// after the end.
func sink(s *ipc.Stream, _ json.RawMessage) error {
	var n int64
	for {
		m, err := s.Receive()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return err
		}
		n += int64(len(m.Bytes) + len(m.JSON))
	}
	return s.Write(map[string]int64{"bytes": n})
}

// source writes frames byte frames of size bytes each.
func source(ctx context.Context, args json.RawMessage, w ipc.StreamWriter) error {
	var frames, size, mod, add int
	if err := decodeArgs(args, &frames, &size, &mod, &add); err != nil || frames < 0 || size < 0 || size > ipc.MaxFrameBytes ||
		mod < 0 || mod > 256 || add < 0 {
		return ipc.NewError(ipc.CodeBadArgs, "Source(frames int, size int, mod int?, add int?)")
	}
	// The StreamWriter of a stream from JS is its *ipc.Stream, which also
	// sends byte frames.
	bw, ok := w.(interface{ WriteBytes([]byte) error })
	if !ok {
		return ipc.NewError(ipc.CodeInternal, "Source: stream cannot send bytes")
	}
	buf := make([]byte, size)
	fillSource(buf, 0, mod, add)
	for k := range frames {
		if k > 0 && mod > 0 {
			fillSource(buf, k*size, mod, add) // WriteBytes is done with buf
		}
		if err := bw.WriteBytes(buf); err != nil {
			return err
		}
	}
	return nil
}

// fillSource fills buf, the frame at stream offset off: byte(i) for mod 0,
// else (off + i + add) % mod.
func fillSource(buf []byte, off, mod, add int) {
	if mod == 0 {
		for i := range buf {
			buf[i] = byte(i)
		}
		return
	}
	v := (off + add) % mod
	for i := range buf {
		buf[i] = byte(v)
		if v++; v == mod {
			v = 0
		}
	}
}

// benchReport prints its argument as one JSON line on stdout, then runs
// benchDone (quitting the app under -bench).
func benchReport(_ context.Context, args json.RawMessage) (any, error) {
	var a []json.RawMessage
	if err := json.Unmarshal(args, &a); err != nil || len(a) != 1 {
		return nil, ipc.NewError(ipc.CodeBadArgs, "Bench.Report(result object)")
	}
	var line bytes.Buffer
	if err := json.Compact(&line, a[0]); err != nil {
		return nil, ipc.NewError(ipc.CodeBadArgs, err.Error())
	}
	fmt.Println(line.String())
	benchMu.Lock()
	done := benchDone
	benchMu.Unlock()
	if done != nil {
		done()
	}
	return nil, nil
}

// decodeArgs decodes the JSON argument array into dst, one element each;
// missing trailing arguments keep their zero value.
func decodeArgs(args json.RawMessage, dst ...any) error {
	var a []json.RawMessage
	if err := json.Unmarshal(args, &a); err != nil {
		return err
	}
	if len(a) > len(dst) {
		return fmt.Errorf("%d arguments, want at most %d", len(a), len(dst))
	}
	for i, raw := range a {
		if err := json.Unmarshal(raw, dst[i]); err != nil {
			return fmt.Errorf("argument %d: %w", i+1, err)
		}
	}
	return nil
}
