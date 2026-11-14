// Package ipc implements Tango's IPC wire protocol (version 2) on top of raw
// page messages: calls (JS→Go request/response), events (both directions)
// and flow-controlled, bidirectional streams that either side may open and
// that carry JSON values and bytes both ways.
//
// The package does not depend on the root tan package. A page connection
// is anything with a Send(msg string) method (*tango.Surface satisfies Conn).
// Feed every raw message from a page into Router.HandleMessage and call
// Router.Drop when the page goes away. Go code in the same process (a
// Go-drawn UI) reaches the same handlers with Router.Call and Router.Open,
// without the wire format.
//
// # Wire format
//
// One JSON object per raw message, tagged by the "tango" field. Messages
// without that field are not ipc and are left to the application.
//
// Handshake. runtime-js sends hello when it starts (the page (re)loaded).
// Go drops everything of the previous document on that connection and
// replies with its stream receive window in bytes (SetStreamWindow, default
// DefaultWindowBytes). A hello without "proto":2 is answered with an error
// and logged with slog.Warn; that runtime's calls and streams then fail.
// Before Go's hello arrives, JS may already send calls, events and stream
// messages, counting on a window of DefaultWindowBytes.
//
//	{"tango":"hello","proto":2}
//	{"tango":"hello","proto":2,"win":1048576}
//	{"tango":"hello","error":{"code":"bad_args","message":"runtime-js protocol mismatch"}}
//
// Calls and events:
//
//	{"tango":"call","id":1,"method":"Greeter.Hello","args":["world"]}
//	{"tango":"ret","id":1,"result":"Hello world"}
//	{"tango":"ret","id":1,"error":{"message":"boom","code":"internal"}}
//	{"tango":"event","name":"progress","data":{"pct":42}}
//
// Streams. Ids are allocated by the opener: JS uses odd numbers, Go even.
// Go never reuses an id on a connection across hello (until Router.Drop).
// Every stream message except open may travel in either direction:
//
//	{"tango":"stream","id":3,"op":"open","method":"Term.Attach","args":[]}
//	{"tango":"stream","id":3,"op":"accept","in":true}
//	{"tango":"stream","id":3,"op":"data","seq":0,"c":75,"data":{"cols":80}}
//	{"tango":"stream","id":3,"op":"data","seq":1,"c":69,"bin":"aGVsbG8="}
//	{"tango":"stream","id":3,"op":"ack","b":65536}
//	{"tango":"stream","id":3,"op":"boundary","seq":2}
//	{"tango":"stream","id":3,"op":"end","seq":2}
//	{"tango":"stream","id":3,"op":"close","seq":2}
//	{"tango":"stream","id":3,"op":"error","seq":2,"error":{"message":"...","code":"internal"}}
//	{"tango":"stream","id":3,"op":"cancel"}
//
// The stream ops:
//
//   - accept: the receiving side of open confirms the stream before its
//     handler runs. "in" tells whether that side receives data: false for
//     a HandleStream handler (send only), which drops (and acknowledges)
//     any data it gets; the opener's sends then fail with CodeBadArgs. The
//     opener may send data before accept arrives. An unknown method is
//     answered with error (CodeNotFound) instead of accept.
//   - data: one application message; frames are never split or merged.
//     Exactly one of "data" (a JSON payload) and "bin" (a byte payload in
//     standard, padded base64; "" is an empty byte frame) is present,
//     otherwise the receiver fails the stream with CodeBadArgs. "seq"
//     counts the frames of this direction from 0; the receiver delivers
//     them in seq order. "c" is the charged size computed by the sender:
//     the payload length (the sender's JSON encoding, or the raw bytes
//     before base64) plus FrameOverhead. A missing c, or one below
//     FrameOverhead, fails the stream with CodeBadArgs. A payload larger
//     than MaxFrameBytes is never sent: the sending call fails with
//     CodeBadArgs and the stream stays usable.
//   - boundary: reliable exclusive sequence bound, sent on the message channel
//     once each lane frame is on its way (Go: when the lane accepted it;
//     runtime-js: when the lane put it on the wire, not when it queued it,
//     so a stalled page never starts the receiver's gap deadline early;
//     every later message-channel frame of the stream, end and close
//     included, then also waits for those lane frames). It neither ends
//     the direction nor returns credit. A missing tail below it arms the same
//     five-second gap deadline; contiguous receipt clears it. Only unresolved
//     streams fail, even when there is no later frame/end and the send window
//     is full.
//   - ack: returns "b" bytes of credit to the sender (the sum of the c of
//     the frames read since the previous ack).
//   - end: half close. The sender of end sends no more data but still
//     receives. Once both directions ended, the stream is complete.
//   - close: graceful full close. Like end, and the sender no longer
//     receives: data still arriving is dropped, the peer's writes fail
//     ("stream closed by peer"), and the peer sends nothing more on the
//     stream. Go sends close when a stream handler returns nil.
//   - error: fails the stream in both directions. The receiver of error
//     stops sending at once (its writes fail with the error) and sends
//     nothing more; it still delivers the frames before "seq", then the
//     error. Go sends it when a stream handler returns an error. An error
//     without "seq" (a refused open, or another protocol-level failure)
//     aborts at once.
//   - cancel: abandons the stream at once; unread data is dropped.
//
// end, close and error carry in "seq" the number of data frames sent in
// that direction; the receiver acts on them only after every frame before
// them. A data payload larger than MaxFrameBytes fails the stream with
// CodeBadArgs. Unknown ops are ignored.
//
// # Flow control
//
// Each direction of each stream has a window in bytes: the receive window
// the receiver advertised (Go's "win" in hello; toward runtime-js it is
// always DefaultWindowBytes). The sender blocks while frames are in flight
// and the next frame's c would exceed the window; with nothing in flight a
// single frame may exceed it. The receiver returns a frame's c once the
// application has read the frame, and sends ack when the credit to return
// reaches half its window, and also at once whenever a read leaves its
// receive queue empty.
//
// # Fast lanes
//
// A connection that implements BinaryConn may also carry stream byte
// frames on fast lanes: binary side channels, negotiated per direction,
// that skip JSON and base64. Only byte data frames ever use a lane; JSON
// frames, every control message (open, accept, ack, end, close, error,
// cancel) and the handshake stay on the message channel. Frames of one
// stream may be split between a lane and the message channel; the
// receiver reorders them by seq as usual, and end/close/error still wait
// for every frame before their seq. A frame that stays missing (later
// frames, or an end/close/error counting it, are here) for GapTimeout is
// taken as lost: the receiver fails the stream with an error carrying code
// "transport", sent to the other end as usual.
//
// The opener of a stream keeps its byte frames off the lanes until the
// peer's accept arrived (they go on the message channel before that): a
// lane packet could overtake the open message and reach a peer that does
// not know the stream yet, which drops it. The acceptor may use a lane at
// once.
//
// When a lane is available, the hello reply carries the connection's
// BinaryConn.LaneHello JSON verbatim in "lanes"; without lanes the field
// is absent:
//
//	{"tango":"hello","proto":2,"win":1048576,"lanes":{"gen":1,"tok":"…","list":["down:scheme","up:post"]}}
//
// A lane carries packets, identical on every platform (big endian, not
// base64):
//
//	packet := count:u16 | frame*count
//	frame  := streamID:u64 | seq:u64 | len:u32 | payload[len]
//
// A packet may batch frames of several streams; count 0 is a valid empty
// packet. A frame on a lane has no "c": it is charged len+FrameOverhead.
// len is at most MaxFrameBytes, and a packet ends exactly after its last
// frame. Reusable document connections use SessionBinaryConn handles and
// Router.HandleBinarySession with the generation captured at authorization;
// simple non-reused BinaryConns may use Router.HandleBinary, which drops a
// malformed packet whole (logged with slog.Warn) and ignores frames for
// unknown streams.
package ipc

import (
	"encoding/json"
	"strings"
	"time"
)

// Error codes used on the wire.
const (
	CodeNotFound = "not_found"
	CodeBadArgs  = "bad_args"
	CodeInternal = "internal"
	CodeCanceled = "canceled"
	// CodeTransport reports a broken transport: a stream fails with it
	// when frames it is waiting for were lost (see GapTimeout).
	CodeTransport = "transport"
)

// GapTimeout is how long a stream's receiver waits for a missing frame
// before failing the stream with CodeTransport. Frames of one stream may
// arrive on two transports (the message channel and a fast lane), so a
// later frame, or the end/close/error counting the missing one, can be
// here first; normally the gap closes within milliseconds.
const GapTimeout = 5 * time.Second

// Wire protocol v2 parameters (the JS runtime uses the same values).
const (
	// ProtoVersion is the wire protocol version carried in "hello". A
	// runtime-js speaking any other version is rejected.
	ProtoVersion = 2
	// DefaultWindowBytes is the default stream receive window in bytes.
	DefaultWindowBytes = 1 << 20
	// FrameOverhead is the fixed number of bytes charged per stream frame
	// on top of its payload size.
	FrameOverhead = 64
	// MaxFrameBytes is the largest raw payload a single stream frame may
	// carry.
	MaxFrameBytes = 16 << 20
)

// Error is the error object carried on the wire. Handlers may return an
// *Error (directly or wrapped) to choose the code seen by JS; any other error
// is reported as CodeInternal (or CodeCanceled for context cancellation).
type Error struct {
	Message string `json:"message"`
	Code    string `json:"code,omitempty"`
}

func (e *Error) Error() string {
	if e.Code == "" {
		return e.Message
	}
	return e.Code + ": " + e.Message
}

// NewError returns an *Error with the given code and message.
func NewError(code, message string) *Error { return &Error{Message: message, Code: code} }

// wireIn is the decoded form of any incoming ipc message.
type wireIn struct {
	Tango  string          `json:"tango"`
	ID     uint64          `json:"id"`
	Method string          `json:"method"`
	Args   json.RawMessage `json:"args"`
	Name   string          `json:"name"`
	Data   json.RawMessage `json:"data"`
	Op     string          `json:"op"`
	Error  *Error          `json:"error"`
	Proto  int             `json:"proto"`
	Seq    *uint64         `json:"seq"` // nil when absent (an error without seq aborts at once)
	C      int             `json:"c"`
	Bin    *string         `json:"bin"` // nil when absent; "" is an empty byte frame
	B      int             `json:"b"`
	In     *bool           `json:"in"`
}

// seq returns the "seq" field, 0 when absent.
func (m *wireIn) seq() uint64 {
	if m.Seq == nil {
		return 0
	}
	return *m.Seq
}

// isIPC reports whether msg is a JSON object with a "tango" field. Only the
// tag is decoded, so malformed ipc payloads are still claimed (and dropped)
// instead of leaking to user raw-message handlers.
func isIPC(msg string) bool {
	s := strings.TrimLeft(msg, " \t\r\n")
	if s == "" || s[0] != '{' {
		return false
	}
	var probe struct {
		Tango json.RawMessage `json:"tango"`
	}
	if json.Unmarshal([]byte(s), &probe) != nil {
		return false
	}
	return probe.Tango != nil
}

// Outgoing messages. Separate structs keep "result"/"data" present even when
// null, which JS distinguishes from absent.

type helloOut struct {
	Tango string          `json:"tango"`
	Proto int             `json:"proto,omitempty"`
	Win   int             `json:"win,omitempty"`
	Lanes json.RawMessage `json:"lanes,omitempty"` // BinaryConn.LaneHello, verbatim
	Error *Error          `json:"error,omitempty"`
}

type retOK struct {
	Tango  string          `json:"tango"`
	ID     uint64          `json:"id"`
	Result json.RawMessage `json:"result"`
}

type retErr struct {
	Tango string `json:"tango"`
	ID    uint64 `json:"id"`
	Error *Error `json:"error"`
}

type eventOut struct {
	Tango string          `json:"tango"`
	Name  string          `json:"name"`
	Data  json.RawMessage `json:"data"`
}

type streamOpen struct {
	Tango  string          `json:"tango"`
	ID     uint64          `json:"id"`
	Op     string          `json:"op"`
	Method string          `json:"method"`
	Args   json.RawMessage `json:"args"`
}

// streamData is a data frame: exactly one of Data (JSON payload) and Bin
// (standard base64 byte payload; "" is an empty byte frame) is set.
type streamData struct {
	Tango string          `json:"tango"`
	ID    uint64          `json:"id"`
	Op    string          `json:"op"`
	Seq   uint64          `json:"seq"`
	C     int             `json:"c"`
	Data  json.RawMessage `json:"data,omitempty"`
	Bin   *string         `json:"bin,omitempty"`
}

// streamCtl is any stream message other than open and data.
type streamCtl struct {
	Tango string  `json:"tango"`
	ID    uint64  `json:"id"`
	Op    string  `json:"op"`
	Seq   *uint64 `json:"seq,omitempty"` // end/close/error: frames sent
	B     int     `json:"b,omitempty"`   // ack: credit returned in bytes
	In    *bool   `json:"in,omitempty"`  // accept
	Error *Error  `json:"error,omitempty"`
}

func encode(v any) string {
	b, err := json.Marshal(v)
	if err != nil {
		// Only json.RawMessage payloads that were already validated reach
		// here, so this cannot happen.
		panic("ipc: encode: " + err.Error())
	}
	return string(b)
}

func marshalValue(v any) (json.RawMessage, error) {
	if raw, ok := v.(json.RawMessage); ok {
		if len(raw) == 0 {
			return json.RawMessage("null"), nil
		}
		if !json.Valid(raw) {
			return nil, NewError(CodeInternal, "invalid json.RawMessage")
		}
		return raw, nil
	}
	return json.Marshal(v)
}
