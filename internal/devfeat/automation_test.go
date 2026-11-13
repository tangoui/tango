//go:build tango_dev

package devfeat_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/tangoui/tango"
	"github.com/tangoui/tango/internal/automation"
	"github.com/tangoui/tango/internal/corehook"
	"github.com/tangoui/tango/internal/devfeat"
	"github.com/tangoui/tango/internal/platform/fake/faketest"
)

func waitTokenFile(t *testing.T, path string) automation.TokenFile {
	t.Helper()
	deadline := time.Now().Add(faketest.Timeout)
	for {
		tf, err := automation.ReadTokenFile(path)
		if err == nil {
			return tf
		}
		if time.Now().After(deadline) {
			t.Fatalf("no token file: %v", err)
		}
		time.Sleep(5 * time.Millisecond)
	}
}

type autoConn struct {
	t  *testing.T
	id uint64
	c  interface {
		Read([]byte) (int, error)
		Write([]byte) (int, error)
	}
}

func (a *autoConn) do(req *automation.Request) *automation.Response {
	a.t.Helper()
	a.id++
	req.ID = a.id
	if err := automation.WriteFrame(a.c, req); err != nil {
		a.t.Fatal(err)
	}
	var r automation.Response
	if err := automation.ReadFrame(a.c, &r); err != nil {
		a.t.Fatalf("%s: %v", req.Op, err)
	}
	return &r
}

func dialAuto(t *testing.T, tf automation.TokenFile, token string) *autoConn {
	t.Helper()
	nc, err := automation.Dial(tf.Addr, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { nc.Close() })
	return &autoConn{t: t, c: nc}
}

func TestAutomationChannel(t *testing.T) {
	tokenFile := filepath.Join(t.TempDir(), "auto.json")
	t.Setenv(automation.EnvAddr, "127.0.0.1:0")
	t.Setenv(automation.EnvTokenFile, tokenFile)
	a, fb := faketest.NewApp(tango.Options{})
	fb.AutoLoad = true
	if os.Getenv(automation.EnvAddr) != "" || os.Getenv(automation.EnvTokenFile) != "" {
		t.Fatal("variables left for child processes")
	}
	faketest.Start(t, a, fb)
	tf := waitTokenFile(t, tokenFile)
	if tf.PID != os.Getpid() || !strings.HasPrefix(tf.Addr, "tcp:127.0.0.1:") || len(tf.Token) != 64 {
		t.Fatalf("token file %+v", tf)
	}

	// A wrong token is refused.
	bad := dialAuto(t, tf, "")
	if r := bad.do(&automation.Request{Op: automation.OpHello, Token: "x"}); r.Code != automation.CodeAuth {
		t.Fatalf("bad token: %+v", r)
	}

	c := dialAuto(t, tf, tf.Token)
	if r := c.do(&automation.Request{Op: automation.OpHello, Token: tf.Token}); r.Error != "" {
		t.Fatal(r.Error)
	}
	s, err := a.NewSurface(tango.SurfaceOptions{Title: "Page", URL: "/", Width: 300, Height: 200})
	if err != nil {
		t.Fatal(err)
	}
	ws := c.do(&automation.Request{Op: automation.OpWindows}).Windows
	if len(ws) != 1 || ws[0].ID != uint64(s.ID()) || ws[0].Title != "Page" || ws[0].Kind != automation.KindWebView {
		t.Fatalf("windows %+v", ws)
	}

	// Eval: the page answers on the internal channel.
	re := regexp.MustCompile(`var __id=(\d+);`)
	go func() {
		deadline := time.Now().Add(faketest.Timeout)
		for time.Now().Before(deadline) {
			for _, js := range fb.Surface(s.ID()).Evals() {
				if m := re.FindStringSubmatch(js); m != nil {
					if !strings.Contains(js, "\n6*7\n") {
						return
					}
					v, _ := json.Marshal(map[string]any{"id": json.Number(m[1]), "ok": true, "value": "42"})
					fb.Message(s.ID(), corehook.InternalPrefix+devfeat.AutomationChannel+":"+string(v))
					return
				}
			}
			time.Sleep(5 * time.Millisecond)
		}
	}()
	r := c.do(&automation.Request{Op: automation.OpEval, Window: uint64(s.ID()), Script: " 6*7 ;", Timeout: 2000})
	if r.Error != "" || string(r.Value) != "42" {
		t.Fatalf("eval %+v", r)
	}
	if r := c.do(&automation.Request{Op: automation.OpFind, Window: uint64(s.ID()), Query: &automation.Query{}}); r.Code != automation.CodeUnsupported {
		t.Fatalf("find on a WebView: %+v", r)
	}

	// Quit.
	if r := c.do(&automation.Request{Op: automation.OpQuit}); r.Error != "" {
		t.Fatal(r.Error)
	}
	select {
	case <-corehook.Done(a):
	case <-time.After(faketest.Timeout):
		t.Fatal("app did not quit")
	}
	deadline := time.Now().Add(faketest.Timeout)
	for {
		if _, err := os.Stat(tokenFile); os.IsNotExist(err) {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("token file left after quit")
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func TestAutomationRejectsRemoteAddr(t *testing.T) {
	tokenFile := filepath.Join(t.TempDir(), "auto.json")
	t.Setenv(automation.EnvAddr, "0.0.0.0:0")
	t.Setenv(automation.EnvTokenFile, tokenFile)
	faketest.Run(t, tango.Options{})
	time.Sleep(50 * time.Millisecond)
	if _, err := os.Stat(tokenFile); err == nil {
		t.Fatal("listened on a non-loopback address")
	}
}

func TestEvalScript(t *testing.T) {
	js := devfeat.EvalScript(5, "  document.title;; ")
	if !strings.Contains(js, "\ndocument.title\n") || !strings.Contains(js, `var __id=5;`) || !strings.Contains(js, `"\x00tango-int:automation:"`) {
		t.Fatalf("script %s", js)
	}
}
