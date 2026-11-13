// Package ipc implements Tango's IPC wire protocol on top of raw page
// messages: calls (JS→Go request/response), events (both directions) and
// flow-controlled streams (either side opens; Go produces).
//
// The package does not depend on the root tan package. A page connection
// is anything with a Send(msg string) method (*tango.Surface satisfies Conn).
// Feed every raw message from a page into Router.HandleMessage and call
// Router.Drop when the page goes away.
//
// Wire format (one JSON object per raw message, tagged by the "tango" field):
//
//	{"tango":"hello"}                                           JS runtime started (page (re)loaded)
//	{"tango":"call","id":1,"method":"Greeter.Hello","args":["world"]}
//	{"tango":"ret","id":1,"result":"Hello world"}
//	{"tango":"ret","id":1,"error":{"message":"boom","code":"internal"}}
//	{"tango":"event","name":"progress","data":{"pct":42}}
//	{"tango":"stream","id":3,"op":"open","method":"Logs.Tail","args":[]}
//	{"tango":"stream","id":3,"op":"data","data":"line"}
//	{"tango":"stream","id":3,"op":"ack","n":16}
//	{"tango":"stream","id":3,"op":"end"}
//	{"tango":"stream","id":3,"op":"error","error":{"message":"..."}}
//	{"tango":"stream","id":3,"op":"cancel"}
//
// Stream ids are allocated by the opener: JS uses odd numbers, Go even.
// In both directions Go is the producer and JS the consumer.
package ipc

import (
	"encoding/json"
	"strings"
)

// Error codes used on the wire.
const (
	CodeNotFound = "not_found"
	CodeBadArgs  = "bad_args"
	CodeInternal = "internal"
	CodeCanceled = "canceled"
)

// DefaultWindow is the number of unacknowledged stream data messages a
// producer may have in flight before Write blocks.
const DefaultWindow = 64

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
	N      int             `json:"n"`
	Error  *Error          `json:"error"`
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

type streamData struct {
	Tango string          `json:"tango"`
	ID    uint64          `json:"id"`
	Op    string          `json:"op"`
	Data  json.RawMessage `json:"data"`
}

type streamCtl struct {
	Tango string `json:"tango"`
	ID    uint64 `json:"id"`
	Op    string `json:"op"`
	Error *Error `json:"error,omitempty"`
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
