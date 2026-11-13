// Package automation is the app side of Tango's whole-app automation
// (docs/automation.md) and the wire protocol it shares with the test
// client, package github.com/tangoui/tango/automation.
//
// The control channel exists only in development builds (-tags tango_dev)
// and only when TANGO_AUTOMATION names a loopback address. Every
// connection must first present the random token the app wrote to
// TANGO_AUTOMATION_TOKEN_FILE (or printed to stderr).
//
// # Wire format
//
// Each message is a 4-byte big-endian length followed by that many bytes
// of JSON: the client sends Request, the server answers each with one
// Response carrying the same ID, in order. The first request of a
// connection must be OpHello with the token; anything else, or a wrong
// token, gets an error answer and the connection is closed. Before the
// token is checked a frame may be at most MaxHelloFrame bytes and must
// arrive within a few seconds; the server holds at most MaxConns
// connections.
package automation

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/tangoui/tango/ui/a11y"
)

const (
	// EnvAddr turns the control channel on: a loopback TCP address
	// ("127.0.0.1:0", "localhost:7777", "[::1]:0") or a unix socket
	// ("unix:/tmp/app.sock").
	EnvAddr = "TANGO_AUTOMATION"
	// EnvTokenFile, when set, is where the app writes TokenFile (JSON,
	// mode 0600) once it listens. Without it the address and token are
	// printed to stderr.
	EnvTokenFile = "TANGO_AUTOMATION_TOKEN_FILE"

	// Version is the protocol version reported by OpHello.
	Version = 1
	// MaxFrame bounds one message (screenshots included) once the
	// connection is authenticated.
	MaxFrame = 64 << 20
	// MaxHelloFrame bounds the first message of a connection, read before
	// the token is checked: an unauthenticated peer can make the server
	// buffer at most this much.
	MaxHelloFrame = 4 << 10
	// MaxConns bounds the connections the server holds at once
	// (authenticated or not); further ones are closed right away.
	MaxConns = 4
)

// Operations.
const (
	OpHello      = "hello"      // Token → Version
	OpPing       = "ping"       // → nothing
	OpWindows    = "windows"    // → Windows
	OpTree       = "tree"       // Window → Nodes, Dump (waits up to Timeout for a first tree)
	OpFind       = "find"       // Window, Query, Timeout → Node, Count (waits for a match)
	OpCount      = "count"      // Window, Query → Count, Nodes
	OpWait       = "wait"       // Window, Query, Gone, Timeout → Node (nil when Gone)
	OpAction     = "action"     // Window, Query, Action, Arg, Timeout → Node
	OpInput      = "input"      // Window, Input → nothing
	OpEval       = "eval"       // Window, Script, Timeout → Value (WebView)
	OpScreenshot = "screenshot" // Window → PNG
	OpQuit       = "quit"       // → nothing; the app quits after answering
)

// Error codes (Response.Code).
const (
	CodeAuth        = "auth"
	CodeBadRequest  = "bad_request"
	CodeNoWindow    = "no_window"
	CodeTimeout     = "timeout"
	CodeHotReload   = "hot_reload"
	CodeUnsupported = "unsupported"
	CodeAction      = "action"
	CodeScript      = "script"
	CodeQuit        = "quit"
	CodeInternal    = "internal"
)

// Window kinds (WindowInfo.Kind).
const (
	KindCanvas  = "canvas"  // self-drawn (package ui through uihost)
	KindWebView = "webview" // a WebView page
)

// Request is one call from the client.
type Request struct {
	ID      uint64 `json:"id"`
	Op      string `json:"op"`
	Token   string `json:"token,omitempty"`
	Window  uint64 `json:"window,omitempty"`
	Query   *Query `json:"query,omitempty"`
	Gone    bool   `json:"gone,omitempty"`
	Action  string `json:"action,omitempty"` // a11y.Action name: "press", "setValue", ...
	Arg     string `json:"arg,omitempty"`
	Input   *Input `json:"input,omitempty"`
	Script  string `json:"script,omitempty"`
	Timeout int64  `json:"timeoutMs,omitempty"` // 0: no waiting
}

// TimeoutDuration returns Timeout as a time.Duration.
func (r *Request) TimeoutDuration() time.Duration { return time.Duration(r.Timeout) * time.Millisecond }

// Response answers the Request with the same ID.
type Response struct {
	ID      uint64          `json:"id"`
	Error   string          `json:"error,omitempty"`
	Code    string          `json:"code,omitempty"`
	Version int             `json:"version,omitempty"`
	Windows []WindowInfo    `json:"windows,omitempty"`
	Node    *a11y.Node      `json:"node,omitempty"`
	Nodes   []a11y.Node     `json:"nodes,omitempty"`
	Count   int             `json:"count,omitempty"`
	Dump    string          `json:"dump,omitempty"`
	Value   json.RawMessage `json:"value,omitempty"`
	PNG     []byte          `json:"png,omitempty"`
}

// Err returns the response's error, nil when it has none.
func (r *Response) Err() error {
	if r.Error == "" {
		return nil
	}
	return &Error{Code: r.Code, Msg: r.Error}
}

// Error is an error answered by the app.
type Error struct{ Code, Msg string }

func (e *Error) Error() string { return e.Msg }

// WindowInfo describes an open window.
type WindowInfo struct {
	ID      uint64 `json:"id"`
	Title   string `json:"title"`
	Kind    string `json:"kind"`
	Visible bool   `json:"visible"`
	Focused bool   `json:"focused"`
	Width   int    `json:"width"`
	Height  int    `json:"height"`
}

// Input is coordinate-level input for a self-drawn window. Coordinates
// are logical pixels from the top-left of the window's content (the same
// space as a11y.Node.Bounds).
type Input struct {
	Kind   string   `json:"kind"` // move, click, down, up, drag, scroll, key, text
	X      float32  `json:"x,omitempty"`
	Y      float32  `json:"y,omitempty"`
	X2     float32  `json:"x2,omitempty"` // drag target
	Y2     float32  `json:"y2,omitempty"`
	DX     float32  `json:"dx,omitempty"` // scroll
	DY     float32  `json:"dy,omitempty"`
	Button string   `json:"button,omitempty"` // left (default), right, middle
	Clicks int      `json:"clicks,omitempty"` // click: 1 (default), 2 for a double click
	Steps  int      `json:"steps,omitempty"`  // drag: intermediate moves (default 8)
	Mods   []string `json:"mods,omitempty"`   // pointer and scroll modifiers: shift, ctrl, alt, meta
	Keys   string   `json:"keys,omitempty"`   // key: "Enter", "Cmd+N", "CmdOrCtrl+Shift+Z"
	Text   string   `json:"text,omitempty"`   // text: committed text
}

// TokenFile is what the app writes to EnvTokenFile.
type TokenFile struct {
	Addr    string `json:"addr"` // "tcp:127.0.0.1:54321" or "unix:/path"
	Token   string `json:"token"`
	PID     int    `json:"pid"`
	Version int    `json:"version"`
}

// ---------------------------------------------------------------------------
// Framing

// WriteFrame writes v as one length-prefixed JSON message.
func WriteFrame(w io.Writer, v any) error {
	b, err := json.Marshal(v)
	if err != nil {
		return err
	}
	if len(b) > MaxFrame {
		return fmt.Errorf("automation: message of %d bytes exceeds %d", len(b), MaxFrame)
	}
	buf := make([]byte, 4+len(b))
	binary.BigEndian.PutUint32(buf, uint32(len(b)))
	copy(buf[4:], b)
	_, err = w.Write(buf)
	return err
}

// ErrFrameTooLarge is returned by ReadFrame for a length over MaxFrame
// (ReadFrameLimit: over its limit).
var ErrFrameTooLarge = errors.New("automation: message too large")

// ReadFrame reads one length-prefixed JSON message of at most MaxFrame
// bytes into v.
func ReadFrame(r io.Reader, v any) error { return ReadFrameLimit(r, v, MaxFrame) }

// ReadFrameLimit is ReadFrame with a smaller bound: a length over max is
// refused before anything is allocated for the body.
func ReadFrameLimit(r io.Reader, v any, max int) error {
	if max <= 0 || max > MaxFrame {
		max = MaxFrame
	}
	var h [4]byte
	if _, err := io.ReadFull(r, h[:]); err != nil {
		return err
	}
	n := binary.BigEndian.Uint32(h[:])
	if uint64(n) > uint64(max) {
		return ErrFrameTooLarge
	}
	b := make([]byte, n)
	if _, err := io.ReadFull(r, b); err != nil {
		if err == io.EOF {
			err = io.ErrUnexpectedEOF
		}
		return err
	}
	return json.Unmarshal(b, v)
}

// ---------------------------------------------------------------------------
// Addresses and tokens

// ParseAddr validates an EnvAddr value and returns the network ("tcp" or
// "unix") and address for net.Listen. TCP addresses must be loopback:
// "localhost", 127.0.0.0/8 or ::1, with a port (0 picks a free one).
// "tcp:" and "unix:" prefixes are accepted; a value containing a path
// separator and no port is taken as a unix socket path.
func ParseAddr(s string) (network, addr string, err error) {
	s = strings.TrimSpace(s)
	switch {
	case s == "":
		return "", "", errors.New("automation: empty address")
	case strings.HasPrefix(s, "unix:"):
		p := strings.TrimPrefix(s, "unix:")
		if p == "" {
			return "", "", errors.New("automation: empty unix socket path")
		}
		return "unix", p, nil
	case strings.HasPrefix(s, "tcp:"):
		s = strings.TrimPrefix(s, "tcp:")
	case strings.HasPrefix(s, "/") || strings.HasPrefix(s, "./") || strings.HasPrefix(s, "."+string(filepath.Separator)):
		return "unix", s, nil
	}
	host, port, err := net.SplitHostPort(s)
	if err != nil {
		return "", "", fmt.Errorf("automation: bad address %q (want 127.0.0.1:port or unix:/path): %v", s, err)
	}
	if _, err := strconv.ParseUint(port, 10, 16); err != nil {
		return "", "", fmt.Errorf("automation: bad port in %q", s)
	}
	if strings.EqualFold(host, "localhost") {
		host = "127.0.0.1"
	}
	ip := net.ParseIP(host)
	if ip == nil || !ip.IsLoopback() {
		return "", "", fmt.Errorf("automation: %q is not a loopback address; only 127.0.0.1, ::1 or localhost are allowed", s)
	}
	return "tcp", net.JoinHostPort(ip.String(), port), nil
}

// Dial connects to a TokenFile.Addr (or any ParseAddr value).
func Dial(addr string, timeout time.Duration) (net.Conn, error) {
	network, a, err := ParseAddr(addr)
	if err != nil {
		return nil, err
	}
	return net.DialTimeout(network, a, timeout)
}

// NewToken returns a random 256-bit token as hex.
func NewToken() (string, error) {
	var b [32]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(b[:]), nil
}

// TokenOK compares tokens in constant time; an empty want never matches.
func TokenOK(want, got string) bool {
	return want != "" && subtle.ConstantTimeCompare([]byte(want), []byte(got)) == 1
}

// WriteTokenFile writes tf to path (mode 0600) atomically: a reader never
// sees a partial file.
func WriteTokenFile(path string, tf TokenFile) error {
	b, err := json.Marshal(tf)
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".tango-automation-*")
	if err != nil {
		return err
	}
	name := tmp.Name()
	if err := tmp.Chmod(0o600); err != nil && !isNotSupported(err) {
		tmp.Close()
		os.Remove(name)
		return err
	}
	if _, err := tmp.Write(b); err != nil {
		tmp.Close()
		os.Remove(name)
		return err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(name)
		return err
	}
	if err := os.Rename(name, path); err != nil {
		os.Remove(name)
		return err
	}
	return nil
}

// ReadTokenFile reads a file written by WriteTokenFile.
func ReadTokenFile(path string) (TokenFile, error) {
	var tf TokenFile
	b, err := os.ReadFile(path)
	if err != nil {
		return tf, err
	}
	if err := json.Unmarshal(b, &tf); err != nil {
		return tf, fmt.Errorf("automation: token file %s: %v", path, err)
	}
	if tf.Addr == "" || tf.Token == "" {
		return tf, fmt.Errorf("automation: token file %s is incomplete", path)
	}
	return tf, nil
}

func isNotSupported(err error) bool {
	return errors.Is(err, errors.ErrUnsupported)
}
