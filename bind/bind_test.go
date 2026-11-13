package bind

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/tangoui/tango/ipc"
)

type Point struct{ X, Y int }

type svc struct{ calls int }

func (s *svc) Nothing()                  { s.calls++ }
func (s *svc) Hello(name string) string  { return "Hello " + name }
func (s *svc) Fail() error               { return errors.New("failed") }
func (s *svc) OK() error                 { return nil }
func (s *svc) Add(a, b int) (int, error) { return a + b, nil }
func (s *svc) Div(a, b int) (int, error) {
	if b == 0 {
		return 0, ipc.NewError(ipc.CodeBadArgs, "division by zero")
	}
	return a / b, nil
}
func (s *svc) WithCtx(ctx context.Context, p Point) (Point, error) {
	if _, ok := ipc.ConnFrom(ctx); !ok {
		return Point{}, errors.New("no conn in ctx")
	}
	return Point{p.Y, p.X}, nil
}
func (s *svc) CtxOnly(ctx context.Context) string { return "ctx" }
func (s *svc) Join(sep string, parts ...string) string {
	return strings.Join(parts, sep)
}
func (s *svc) Any(v any) any { return v }

type conn struct{ ch chan string }

func (c *conn) Send(m string) { c.ch <- m }

type ret struct {
	ID     int             `json:"id"`
	Result json.RawMessage `json:"result"`
	Error  *ipc.Error      `json:"error"`
}

func call(t *testing.T, r *ipc.Router, c *conn, id int, method, args string) ret {
	t.Helper()
	msg := fmt.Sprintf(`{"tango":"call","id":%d,"method":%q,"args":%s}`, id, method, args)
	if !r.HandleMessage(c, msg) {
		t.Fatal("not claimed")
	}
	select {
	case m := <-c.ch:
		var o ret
		if err := json.Unmarshal([]byte(m), &o); err != nil {
			t.Fatal(err)
		}
		if o.ID != id {
			t.Fatalf("id %d != %d", o.ID, id)
		}
		return o
	case <-time.After(2 * time.Second):
		t.Fatal("timeout")
	}
	return ret{}
}

func TestSignatures(t *testing.T) {
	r := ipc.NewRouter()
	s := &svc{}
	if err := Register(r, "Svc", s); err != nil {
		t.Fatal(err)
	}
	c := &conn{ch: make(chan string, 8)}
	ok := []struct{ method, args, want string }{
		{"Svc.Nothing", `[]`, "null"},
		{"Svc.Hello", `["world"]`, `"Hello world"`},
		{"Svc.OK", `[]`, "null"},
		{"Svc.Add", `[2,3]`, "5"},
		{"Svc.Div", `[9,3]`, "3"},
		{"Svc.WithCtx", `[{"X":1,"Y":2}]`, `{"X":2,"Y":1}`},
		{"Svc.CtxOnly", `[]`, `"ctx"`},
		{"Svc.Join", `["-"]`, `""`},
		{"Svc.Join", `["-","a","b","c"]`, `"a-b-c"`},
		{"Svc.Any", `[{"k":[1,true]}]`, `{"k":[1,true]}`},
	}
	for i, tc := range ok {
		o := call(t, r, c, i+1, tc.method, tc.args)
		if o.Error != nil || string(o.Result) != tc.want {
			t.Errorf("%s(%s) = %s, %v; want %s", tc.method, tc.args, o.Result, o.Error, tc.want)
		}
	}
	if s.calls != 1 {
		t.Errorf("Nothing called %d times", s.calls)
	}

	bad := []struct{ method, args, code string }{
		{"Svc.Fail", `[]`, ipc.CodeInternal},
		{"Svc.Div", `[1,0]`, ipc.CodeBadArgs},
		{"Svc.Hello", `[]`, ipc.CodeBadArgs},
		{"Svc.Hello", `["a","b"]`, ipc.CodeBadArgs},
		{"Svc.Hello", `[1]`, ipc.CodeBadArgs},
		{"Svc.Add", `["x",1]`, ipc.CodeBadArgs},
		{"Svc.Add", `{"a":1}`, ipc.CodeBadArgs},
		{"Svc.Join", `[]`, ipc.CodeBadArgs},
		{"Svc.Join", `["-",1]`, ipc.CodeBadArgs},
		{"Svc.Missing", `[]`, ipc.CodeNotFound},
	}
	for i, tc := range bad {
		o := call(t, r, c, 100+i, tc.method, tc.args)
		if o.Error == nil || o.Error.Code != tc.code {
			t.Errorf("%s(%s): error %+v, want code %s", tc.method, tc.args, o.Error, tc.code)
		}
	}
}

type valueSvc struct{}

func (valueSvc) Ping() string { return "pong" }
func (*valueSvc) Ptr() string { return "ptr" }

type badChan struct{}

func (badChan) C(ch chan int) {}

type badRes struct{}

func (badRes) R() (int, int) { return 0, 0 }

type badCtx struct{}

func (badCtx) X(a int, ctx context.Context) {}

type badThree struct{}

func (badThree) X() (int, int, error) { return 0, 0, nil }

type badErrFirst struct{}

func (badErrFirst) X() (error, int) { return nil, 0 }

type none struct{}

func TestRegisterErrors(t *testing.T) {
	r := ipc.NewRouter()
	for _, s := range []any{nil, none{}, badChan{}, badRes{}, badCtx{}, badThree{}, badErrFirst{}} {
		if err := Register(r, "X", s); err == nil {
			t.Errorf("Register(%T) succeeded", s)
		}
	}
	if err := Register(r, "", &svc{}); err == nil {
		t.Error("empty name accepted")
	}
	if r.Has("X.C") || r.Has("X.R") {
		t.Error("partial registration leaked")
	}

	// Value receivers only expose value methods; pointers expose both.
	if err := Register(r, "V", valueSvc{}); err != nil {
		t.Fatal(err)
	}
	if !r.Has("V.Ping") || r.Has("V.Ptr") {
		t.Error("value method set wrong")
	}
	if err := Register(r, "P", &valueSvc{}); err != nil {
		t.Fatal(err)
	}
	if !r.Has("P.Ping") || !r.Has("P.Ptr") {
		t.Error("pointer method set wrong")
	}
}
