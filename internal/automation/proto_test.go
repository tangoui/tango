package automation

import (
	"bytes"
	"encoding/binary"
	"errors"
	"io"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tangoui/tango/ui/a11y"
	"github.com/tangoui/tango/ui/input"
)

func TestFrameRoundTrip(t *testing.T) {
	var b bytes.Buffer
	in := &Request{ID: 7, Op: OpAction, Window: 3, Query: &Query{Role: "button", Name: "记一笔", Nth: -1}, Action: "press", Timeout: 1500}
	if err := WriteFrame(&b, in); err != nil {
		t.Fatal(err)
	}
	if n := binary.BigEndian.Uint32(b.Bytes()[:4]); int(n) != b.Len()-4 {
		t.Fatalf("length prefix %d for %d bytes", n, b.Len()-4)
	}
	resp := &Response{ID: 7, Node: &a11y.Node{ID: 9, Role: a11y.RoleButton, Name: "OK", States: a11y.Focusable, Actions: a11y.Press}, PNG: []byte{1, 2, 3}}
	if err := WriteFrame(&b, resp); err != nil {
		t.Fatal(err)
	}
	var out Request
	if err := ReadFrame(&b, &out); err != nil {
		t.Fatal(err)
	}
	if out.ID != 7 || out.Op != OpAction || out.Query.Name != "记一笔" || out.Query.Nth != -1 || out.TimeoutDuration().Milliseconds() != 1500 {
		t.Fatalf("decoded %+v %+v", out, out.Query)
	}
	var r Response
	if err := ReadFrame(&b, &r); err != nil {
		t.Fatal(err)
	}
	if r.Node.Name != "OK" || r.Node.Role != a11y.RoleButton || !r.Node.Actions.Has(a11y.Press) || !bytes.Equal(r.PNG, []byte{1, 2, 3}) {
		t.Fatalf("decoded %+v", r)
	}
	if err := ReadFrame(&b, &r); err != io.EOF {
		t.Fatalf("empty stream: %v", err)
	}
}

func TestFrameErrors(t *testing.T) {
	var h [4]byte
	binary.BigEndian.PutUint32(h[:], MaxFrame+1)
	var v Request
	if err := ReadFrame(bytes.NewReader(h[:]), &v); !errors.Is(err, ErrFrameTooLarge) {
		t.Fatalf("oversized: %v", err)
	}
	binary.BigEndian.PutUint32(h[:], 10)
	if err := ReadFrame(bytes.NewReader(append(h[:], '{')), &v); err != io.ErrUnexpectedEOF {
		t.Fatalf("truncated: %v", err)
	}
	binary.BigEndian.PutUint32(h[:], 3)
	if err := ReadFrame(bytes.NewReader(append(h[:], "{x}"...)), &v); err == nil {
		t.Fatal("bad JSON accepted")
	}
}

func TestParseAddr(t *testing.T) {
	ok := map[string][2]string{
		"127.0.0.1:0":        {"tcp", "127.0.0.1:0"},
		"localhost:7777":     {"tcp", "127.0.0.1:7777"},
		"tcp:127.0.0.2:9":    {"tcp", "127.0.0.2:9"},
		"[::1]:0":            {"tcp", "[::1]:0"},
		"unix:/tmp/a.sock":   {"unix", "/tmp/a.sock"},
		"/tmp/b.sock":        {"unix", "/tmp/b.sock"},
		" 127.0.0.1:1234 \n": {"tcp", "127.0.0.1:1234"},
	}
	for in, want := range ok {
		n, a, err := ParseAddr(in)
		if err != nil || n != want[0] || a != want[1] {
			t.Errorf("ParseAddr(%q) = %q %q %v, want %v", in, n, a, err, want)
		}
	}
	for _, bad := range []string{"", "0.0.0.0:80", ":8080", "192.168.1.2:80", "[::]:0", "example.com:80", "127.0.0.1", "127.0.0.1:x", "127.0.0.1:70000", "unix:"} {
		if _, _, err := ParseAddr(bad); err == nil {
			t.Errorf("ParseAddr(%q) accepted", bad)
		}
	}
}

func TestToken(t *testing.T) {
	a, err := NewToken()
	if err != nil || len(a) != 64 {
		t.Fatalf("token %q %v", a, err)
	}
	b, _ := NewToken()
	if a == b {
		t.Fatal("tokens repeat")
	}
	if !TokenOK(a, a) || TokenOK(a, b) || TokenOK(a, "") || TokenOK("", "") || TokenOK(a, a[:63]) {
		t.Fatal("TokenOK")
	}
	path := filepath.Join(t.TempDir(), "t.json")
	if _, err := ReadTokenFile(path); err == nil {
		t.Fatal("missing file read")
	}
	tf := TokenFile{Addr: "tcp:127.0.0.1:5", Token: a, PID: 1, Version: Version}
	if err := WriteTokenFile(path, tf); err != nil {
		t.Fatal(err)
	}
	if got, err := ReadTokenFile(path); err != nil || got != tf {
		t.Fatalf("read back %+v %v", got, err)
	}
}

func TestParseKeys(t *testing.T) {
	cmdOrCtrl, _ := ParseMod("CmdOrCtrl")
	for in, want := range map[string]struct {
		k input.Key
		m input.Modifiers
	}{
		"Enter":              {input.KeyEnter, 0},
		"return":             {input.KeyEnter, 0},
		"a":                  {input.KeyA, 0},
		"Cmd+N":              {input.KeyN, input.ModMeta},
		"ctrl+shift+z":       {input.KeyZ, input.ModCtrl | input.ModShift},
		"CmdOrCtrl+F":        {input.KeyF, cmdOrCtrl},
		"Alt+F4":             {input.KeyF4, input.ModAlt},
		"Escape":             {input.KeyEscape, 0},
		"Space":              {input.KeySpace, 0},
		"/":                  {input.KeySlash, 0},
		"Shift+Tab":          {input.KeyTab, input.ModShift},
		"PageDown":           {input.KeyPageDown, 0},
		"Option+Backspace":   {input.KeyBackspace, input.ModAlt},
		"Meta+Shift+Equal":   {input.KeyEqual, input.ModMeta | input.ModShift},
		"7":                  {input.Key7, 0},
		"ArrowLeft":          {input.KeyLeft, 0},
		"Ctrl+NumpadEnter":   {input.KeyNumpadEnter, input.ModCtrl},
		"CommandOrControl+1": {input.Key1, cmdOrCtrl},
	} {
		k, m, err := ParseKeys(in)
		if err != nil || k != want.k || m != want.m {
			t.Errorf("ParseKeys(%q) = %v %v %v, want %v %v", in, k, m, err, want.k, want.m)
		}
	}
	for _, bad := range []string{"", "Hyper+A", "Cmd+", "NoSuchKey"} {
		if _, _, err := ParseKeys(bad); err == nil {
			t.Errorf("ParseKeys(%q) accepted", bad)
		}
	}
}

func TestEvents(t *testing.T) {
	b, err := Events(&Input{Kind: "click", X: 10, Y: 20, Clicks: 2, Mods: []string{"shift"}})
	if err != nil || len(b) != 1 || len(b[0]) != 5 {
		t.Fatalf("click %v %v", b, err)
	}
	if p := b[0][4].(input.Pointer); p.Kind != input.Release || p.Clicks != 2 || p.Mods != input.ModShift || p.Pos.X != 10 {
		t.Fatalf("second release %+v", p)
	}
	b, _ = Events(&Input{Kind: "drag", X: 0, Y: 0, X2: 80, Y2: 40, Steps: 4})
	if len(b) != 6 {
		t.Fatalf("drag batches %d", len(b))
	}
	if p := b[2][0].(input.Pointer); p.Kind != input.Move || p.Pos.X != 40 || p.Pos.Y != 20 {
		t.Fatalf("drag midpoint %+v", p)
	}
	if p := b[5][0].(input.Pointer); p.Kind != input.Release || p.Pos.X != 80 {
		t.Fatalf("drag end %+v", p)
	}
	b, _ = Events(&Input{Kind: "key", Keys: "Cmd+N"})
	if k := b[0][0].(input.KeyEvent); !k.Down || k.Key != input.KeyN || k.Mods != input.ModMeta || b[0][1].(input.KeyEvent).Down {
		t.Fatalf("key %+v", b)
	}
	b, _ = Events(&Input{Kind: "text", Text: "你好"})
	if b[0][0].(input.TextInput).Text != "你好" {
		t.Fatal("text")
	}
	b, _ = Events(&Input{Kind: "scroll", X: 5, Y: 5, DY: -120})
	if s := b[0][1].(input.Scroll); s.DY != -120 {
		t.Fatal("scroll")
	}
	for _, bad := range []*Input{nil, {Kind: "fly"}, {Kind: "click", Button: "fourth"}, {Kind: "key", Keys: "Nope"}, {Kind: "move", Mods: []string{"hyper"}}} {
		if _, err := Events(bad); err == nil {
			t.Errorf("Events(%+v) accepted", bad)
		}
	}
}

func TestErrorType(t *testing.T) {
	r := &Response{Error: "boom", Code: CodeTimeout}
	var e *Error
	if !errors.As(r.Err(), &e) || e.Code != CodeTimeout || !strings.Contains(e.Error(), "boom") {
		t.Fatalf("%v", r.Err())
	}
	if (&Response{}).Err() != nil {
		t.Fatal("no error expected")
	}
}

func TestReadFrameLimit(t *testing.T) {
	var b bytes.Buffer
	WriteFrame(&b, &Request{ID: 1, Op: OpHello, Token: strings.Repeat("a", 64)})
	raw := b.Bytes()
	var v Request
	if err := ReadFrameLimit(bytes.NewReader(raw), &v, MaxHelloFrame); err != nil || v.Op != OpHello {
		t.Fatalf("small hello: %v %+v", err, v)
	}
	if err := ReadFrameLimit(bytes.NewReader(raw), &v, len(raw)-5); !errors.Is(err, ErrFrameTooLarge) {
		t.Fatalf("over the limit: %v", err)
	}
	// A huge announced length is refused from the header alone.
	var h [4]byte
	binary.BigEndian.PutUint32(h[:], MaxHelloFrame+1)
	if err := ReadFrameLimit(bytes.NewReader(h[:]), &v, MaxHelloFrame); !errors.Is(err, ErrFrameTooLarge) {
		t.Fatalf("oversized hello header: %v", err)
	}
	// Limits above MaxFrame are clamped.
	binary.BigEndian.PutUint32(h[:], MaxFrame+1)
	if err := ReadFrameLimit(bytes.NewReader(h[:]), &v, 1<<30); !errors.Is(err, ErrFrameTooLarge) {
		t.Fatalf("limit not clamped: %v", err)
	}
}
