// Package devhost implements hot reload of Tango's self-drawn UI under
// `tan dev` (docs/specs/ui-hotreload.md): the wire protocol between the
// long-lived host process (it owns the windows) and the worker processes
// (the user's app, replaced on every rebuild), shared-memory frame
// transport and state snapshots. The host itself lives in
// internal/devhost/host; package uihost is the worker side.
//
// This package is a leaf (standard library plus the plain ui/input and
// ui/geom types) so cmd/tan can speak the control protocol without linking
// platform code. Nothing here is linked into an app unless it is built with
// -tags tango_dev (tan dev does that).
package devhost

import (
	"bufio"
	"crypto/rand"
	"crypto/subtle"
	"encoding/gob"
	"encoding/hex"
	"errors"
	"fmt"
	"net"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/input"
)

// ProtoVersion changes whenever a message changes incompatibly. A worker
// with another version is refused, and tan dev restarts the host.
const ProtoVersion = 2

// Marker is a string tan dev looks for in a built binary to tell whether
// it links the dev host (package uihost built with tango_dev).
const Marker = "tango-devhost-protocol-v1"

// Environment variables set by tan dev.
const (
	EnvRole = "TANGO_DEV_ROLE" // "host" or "worker"
	EnvAddr = "TANGO_DEV_ADDR" // control channel: "unix:/path" or "tcp:127.0.0.1:port"
	EnvName = "TANGO_DEV_NAME" // app display name (host)
	EnvID   = "TANGO_DEV_ID"   // app id (host)
	EnvDir  = "TANGO_DEV_DIR"  // scratch directory for frame buffers (unix)
	// EnvToken is the secret every peer sends in Hello. tan dev makes a
	// fresh one per session; the host refuses connections without it, so
	// another local user cannot attach to a TCP control channel (Windows).
	EnvToken = "TANGO_DEV_TOKEN"
)

// Roles.
const (
	RoleHost   = "host"
	RoleWorker = "worker"
	RoleCtl    = "ctl" // tan dev's control connection to the host
)

// Role returns this process's dev role ("" outside tan dev).
func Role() string { return os.Getenv(EnvRole) }

// ---- messages ----

// Hello opens every connection.
type Hello struct {
	Proto  int
	Role   string
	PID    int
	Marker string
	Token  string // EnvToken of the session
}

// Welcome answers Hello. Err is set when the host refuses the peer.
type Welcome struct {
	Err string
	Gen int // worker generation assigned by the host
}

// WindowOpts is the serializable part of tango.SurfaceOptions.
type WindowOpts struct {
	Title               string
	Width, Height       int
	MinWidth, MinHeight int
	MaxWidth, MaxHeight int
	Resizable, Hidden   bool
	X, Y                int
	Centered            bool
	State               int
	Kind                int
	TitleBar            int
	Transparent         bool
	Background          uint32
	Material            int
	AlwaysOnTop         bool
	Opacity             float64
	NoShadow            bool
	NoClose             bool
	NoMinimize          bool
	NoMaximize          bool
}

// ---- worker → host ----

// OpenWindow asks for a canvas window. Seq is the call's index among this
// worker's OpenWindow calls: a replacement worker's Seq-th window (or one
// with the same title) takes over the old worker's window.
type OpenWindow struct {
	Req  int
	Seq  int
	Opts WindowOpts
}

// CloseWindow closes a window from the worker side (Surface.Close).
type CloseWindow struct{ Win int }

// Invalidate asks for a FrameRequest.
type Invalidate struct{ Win int }

// SetCursor mirrors ui.Window.SetCursor.
type SetCursor struct {
	Win    int
	Cursor input.Cursor
}

// SetTextInput mirrors ui.Window.SetTextInput.
type SetTextInput struct {
	Win     int
	Enabled bool
	Caret   geom.Rect
}

// ShmAttach announces a new frame segment for a window. Frames that follow
// name it by Seg. The host maps it and, on unix, unlinks the file.
type ShmAttach struct {
	Win  int
	Seg  int
	Name string
	Size int
}

// FrameReady says slot Slot of segment Seg holds a W×H premultiplied RGBA
// frame (device pixels, rows Stride bytes apart). The worker does not touch
// the slot again until FrameAck.
type FrameReady struct {
	Win, Seg, Slot int
	W, H, Stride   int
	Scale          float32
}

// ReadClipboard asks for the clipboard text; the host answers ClipboardText.
type ReadClipboard struct{ Req int }

// WriteClipboard sets the clipboard text.
type WriteClipboard struct{ Text string }

// SnapshotGet asks the host for the previous worker's state (Persist).
// The host answers SnapshotData.
type SnapshotGet struct{ Req int }

// SnapshotReply answers SnapshotReq.
type SnapshotReply struct {
	Req     int
	Entries []Entry
}

// ---- host → worker ----

// Opened answers OpenWindow.
type Opened struct {
	Req   int
	Win   int
	W, H  float32
	Scale float32
	Err   string
}

// Input delivers an input event.
type Input struct {
	Win int
	Ev  input.Event
}

// Focus reports a key-focus change.
type Focus struct {
	Win     int
	Focused bool
}

// FrameRequest asks the worker to render one frame at this geometry.
type FrameRequest struct {
	Win   int
	W, H  float32
	Scale float32
}

// FrameAck releases a slot.
type FrameAck struct{ Win, Seg, Slot int }

// WindowClosed reports that the user closed the window.
type WindowClosed struct{ Win int }

// ClipboardText answers ReadClipboard.
type ClipboardText struct {
	Req  int
	Text string
}

// SnapshotReq asks a worker for its Persist state.
type SnapshotReq struct{ Req int }

// SnapshotData answers SnapshotGet.
type SnapshotData struct {
	Req     int
	Entries []Entry
}

// ---- ctl (tan dev ↔ host) ----

// Status tells the host what to overlay: Kind "" clears the banner,
// anything else ("build", "crash", "start") shows Text in a red banner.
type Status struct {
	Kind string
	Text string
}

// Swapped tells tan dev that worker New replaced worker Old (pids; Old is
// 0 for the first worker) and is now on screen. Took is the time from the
// new worker's connection to its first frame being shown.
type Swapped struct {
	Old, New int
	Took     time.Duration
}

// WorkerLeft tells tan dev that a worker disconnected.
type WorkerLeft struct {
	PID    int
	Active bool
}

func init() {
	for _, m := range []any{
		Hello{}, Welcome{}, OpenWindow{}, CloseWindow{}, Invalidate{}, SetCursor{}, SetTextInput{},
		ShmAttach{}, FrameReady{}, ReadClipboard{}, WriteClipboard{}, SnapshotGet{}, SnapshotReply{},
		Opened{}, Input{}, Focus{}, FrameRequest{}, FrameAck{}, WindowClosed{}, ClipboardText{},
		SnapshotReq{}, SnapshotData{}, Status{}, Swapped{}, WorkerLeft{},
		input.Pointer{}, input.Scroll{}, input.KeyEvent{}, input.TextInput{}, input.Composition{}, input.WindowFocus{},
	} {
		gob.Register(m)
	}
}

// ---- connection ----

type envelope struct{ M any }

// Conn is a gob message stream. Send is safe for concurrent use; Recv must
// be called from one goroutine.
type Conn struct {
	c   net.Conn
	dec *gob.Decoder

	wmu sync.Mutex
	bw  *bufio.Writer
	enc *gob.Encoder
}

// NewConn wraps c.
func NewConn(c net.Conn) *Conn {
	bw := bufio.NewWriterSize(c, 64<<10)
	return &Conn{c: c, dec: gob.NewDecoder(bufio.NewReaderSize(c, 64<<10)), bw: bw, enc: gob.NewEncoder(bw)}
}

// Send writes one message.
func (c *Conn) Send(m any) error {
	c.wmu.Lock()
	defer c.wmu.Unlock()
	if err := c.enc.Encode(&envelope{m}); err != nil {
		return err
	}
	return c.bw.Flush()
}

// Recv reads one message.
func (c *Conn) Recv() (any, error) {
	var e envelope
	if err := c.dec.Decode(&e); err != nil {
		return nil, err
	}
	return e.M, nil
}

// Close closes the connection.
func (c *Conn) Close() error { return c.c.Close() }

// Handshake sends Hello as role with the session token from EnvToken and
// waits for Welcome.
func Handshake(c *Conn, role string) (Welcome, error) {
	return HandshakeToken(c, role, os.Getenv(EnvToken))
}

// HandshakeToken is Handshake with an explicit session token.
func HandshakeToken(c *Conn, role, token string) (Welcome, error) {
	if err := c.Send(Hello{Proto: ProtoVersion, Role: role, PID: os.Getpid(), Marker: Marker, Token: token}); err != nil {
		return Welcome{}, err
	}
	m, err := c.Recv()
	if err != nil {
		return Welcome{}, err
	}
	w, ok := m.(Welcome)
	if !ok {
		return Welcome{}, fmt.Errorf("devhost: expected Welcome, got %T", m)
	}
	if w.Err != "" {
		return w, errors.New("devhost: host refused connection: " + w.Err)
	}
	return w, nil
}

// ---- addresses ----

// Listen listens on addr ("unix:/path" or "tcp:host:port").
func Listen(addr string) (net.Listener, error) {
	netw, a, err := splitAddr(addr)
	if err != nil {
		return nil, err
	}
	if netw == "unix" {
		_ = os.Remove(a)
	}
	ln, err := net.Listen(netw, a)
	if err == nil && netw == "unix" {
		// tan dev puts the socket in a 0700 directory; keep the socket
		// itself owner-only as well.
		_ = os.Chmod(a, 0o600)
	}
	return ln, err
}

// NewToken returns a fresh random session token for EnvToken.
func NewToken() (string, error) {
	var b [32]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(b[:]), nil
}

// TokenOK reports whether got matches the session token want, in constant
// time. An empty want accepts any token (hosts started without one, as in
// tests).
func TokenOK(got, want string) bool {
	if want == "" {
		return true
	}
	return subtle.ConstantTimeCompare([]byte(got), []byte(want)) == 1
}

// Dial connects to addr, retrying until timeout (the host may still be
// starting).
func Dial(addr string, timeout time.Duration) (*Conn, error) {
	netw, a, err := splitAddr(addr)
	if err != nil {
		return nil, err
	}
	deadline := time.Now().Add(timeout)
	for {
		c, err := net.DialTimeout(netw, a, time.Second)
		if err == nil {
			return NewConn(c), nil
		}
		if time.Now().After(deadline) {
			return nil, err
		}
		time.Sleep(20 * time.Millisecond)
	}
}

func splitAddr(addr string) (netw, a string, err error) {
	netw, a, ok := strings.Cut(addr, ":")
	if !ok || (netw != "unix" && netw != "tcp") || a == "" {
		return "", "", fmt.Errorf("devhost: bad address %q", addr)
	}
	return netw, a, nil
}
