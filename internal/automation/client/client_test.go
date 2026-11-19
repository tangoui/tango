package client

import (
	"context"
	"errors"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	wire "github.com/tangoui/tango/internal/automation"
)

func TestWithDevTag(t *testing.T) {
	for in, want := range map[string]string{
		"":                       "-tags tango_dev",
		"-race":                  "-tags tango_dev -race",
		"-tags a,b":              "-tags a,b,tango_dev",
		"-tags=a -mod=mod":       "-tags=a,tango_dev -mod=mod",
		"-tags a -tags=b":        "-tags a -tags=b,tango_dev",
		"--tags=tango_dev":       "--tags=tango_dev",
		"-tags tango_release -v": "-tags tango_release,tango_dev -v",
	} {
		if got := strings.Join(WithDevTag(strings.Fields(in)), " "); got != want {
			t.Errorf("WithDevTag(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestHasChannel(t *testing.T) {
	dir := t.TempDir()
	yes, no := filepath.Join(dir, "yes"), filepath.Join(dir, "no") // Build returns a cleaned path
	os.WriteFile(yes, []byte("\x7fELF..TANGO_AUTOMATION_TOKEN_FILE.."), 0o755)
	os.WriteFile(no, []byte("\x7fELF.. production .."), 0o755)
	if !HasChannel(yes) || HasChannel(no) {
		t.Fatal("HasChannel wrong")
	}
	if _, err := Build(no, "", nil, dir); err == nil || !strings.Contains(err.Error(), "-tags tango_dev") {
		t.Fatalf("production executable: err %v", err)
	}
	if got, err := Build(yes, "", nil, dir); err != nil || got != yes {
		t.Fatalf("development executable: %q, %v", got, err)
	}
}

func TestFilterEnv(t *testing.T) {
	got := filterEnv([]string{"A=1", "TANGO_AUTOMATION=x", "TANGO_AUTOMATION_TOKEN_FILE=y", "TANGO_AUTOMATIONX=z"})
	if strings.Join(got, " ") != "A=1 TANGO_AUTOMATIONX=z" {
		t.Fatalf("%v", got)
	}
}

// The program under test starts in the background unless the caller's
// environment or extra says otherwise; the channel variables are always
// the launcher's own.
func TestLaunchEnv(t *testing.T) {
	last := func(env []string, key string) string {
		v := ""
		for _, kv := range env {
			if k, val, ok := strings.Cut(kv, "="); ok && k == key {
				v = val
			}
		}
		return v
	}
	env := LaunchEnv([]string{"A=1", "TANGO_AUTOMATION=stale"}, nil, "127.0.0.1:0", "/tmp/t.json")
	if last(env, EnvLaunchBackground) != "1" || last(env, "A") != "1" {
		t.Fatalf("default env %v", env)
	}
	if last(env, "TANGO_AUTOMATION") != "127.0.0.1:0" || last(env, "TANGO_AUTOMATION_TOKEN_FILE") != "/tmp/t.json" {
		t.Fatalf("channel env %v", env)
	}
	env = LaunchEnv([]string{EnvLaunchBackground + "=0"}, nil, "a", "b")
	if last(env, EnvLaunchBackground) != "0" {
		t.Fatalf("environment override lost: %v", env)
	}
	env = LaunchEnv(nil, []string{EnvLaunchBackground + "=0"}, "a", "b")
	if last(env, EnvLaunchBackground) != "0" {
		t.Fatalf("extra override lost: %v", env)
	}
}

// The expected strings are what automation/window.go built before the
// builders moved here; the page-side scripts must not change.
func TestWebJSMatchesAutomation(t *testing.T) {
	const (
		sel              = `#form input[name="amount"]`
		text             = "88.8 \"quoted\"\n"
		expr             = `document.querySelectorAll("li").length>2`
		to               = 3 * time.Second
		wantWaitSelector = `new Promise(function(res,rej){var s="#form input[name=\"amount\"]",t0=Date.now();(function p(){var e=document.querySelector(s);if(e)return res(e);if(Date.now()-t0>3000)return rej(new Error("no element matches "+s+" after 3s"));setTimeout(p,30)})()}).then(function(){return true})`
		wantWaitExpr     = `new Promise(function(res,rej){var t0=Date.now();(function p(){var v;try{v=(document.querySelectorAll("li").length>2)}catch(e){}if(v)return res(true);if(Date.now()-t0>3000)return rej(new Error("condition not met after 3s: document.querySelectorAll(\"li\").length\u003e2"));setTimeout(p,30)})()})`
		wantClick        = `new Promise(function(res,rej){var s="#form input[name=\"amount\"]",t0=Date.now();(function p(){var e=document.querySelector(s);if(e)return res(e);if(Date.now()-t0>3000)return rej(new Error("no element matches "+s+" after 3s"));setTimeout(p,30)})()}).then(function(e){e.scrollIntoView({block:"center",inline:"center"});if(e.focus)e.focus();e.click();return true})`
		wantFill         = `new Promise(function(res,rej){var s="#form input[name=\"amount\"]",t0=Date.now();(function p(){var e=document.querySelector(s);if(e)return res(e);if(Date.now()-t0>3000)return rej(new Error("no element matches "+s+" after 3s"));setTimeout(p,30)})()}).then(function(e){var v="88.8 \"quoted\"\n";if(e.focus)e.focus();if(e.isContentEditable){e.textContent=v}else{var d=Object.getOwnPropertyDescriptor(Object.getPrototypeOf(e),"value");if(d&&d.set){d.set.call(e,v)}else{e.value=v}}e.dispatchEvent(new Event("input",{bubbles:true}));e.dispatchEvent(new Event("change",{bubbles:true}));return true})`
		wantText         = `new Promise(function(res,rej){var s="#form input[name=\"amount\"]",t0=Date.now();(function p(){var e=document.querySelector(s);if(e)return res(e);if(Date.now()-t0>3000)return rej(new Error("no element matches "+s+" after 3s"));setTimeout(p,30)})()}).then(function(e){return e.innerText!==undefined?e.innerText:e.textContent})`
	)
	for _, c := range []struct{ name, got, want string }{
		{"WaitSelectorJS", WaitSelectorJS(sel, to), wantWaitSelector},
		{"WaitExprJS", WaitExprJS(expr, to), wantWaitExpr},
		{"ClickJS", ClickJS(sel, to), wantClick},
		{"FillJS", FillJS(sel, text, to), wantFill},
		{"TextJS", TextJS(sel, to), wantText},
	} {
		if c.got != c.want {
			t.Errorf("%s:\n got %s\nwant %s", c.name, c.got, c.want)
		}
	}
}

// Launch refuses an executable without the automation channel before
// starting it. The executable is a trivial program built here: the test
// binary itself links the channel's protocol package.
func TestLaunchNotDevBuild(t *testing.T) {
	if testing.Short() {
		t.Skip("runs go build")
	}
	goTool, err := exec.LookPath("go")
	if err != nil {
		t.Skip("no go tool")
	}
	src := t.TempDir()
	if err := os.WriteFile(filepath.Join(src, "go.mod"), []byte("module plain\n\ngo 1.21\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(src, "main.go"), []byte("package main\n\nfunc main() {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	bin := filepath.Join(src, "plain")
	if runtime.GOOS == "windows" {
		bin += ".exe"
	}
	cmd := exec.Command(goTool, "build", "-o", bin, ".")
	cmd.Dir = src
	cmd.Env = append(os.Environ(), "GOWORK=off")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("go build: %v\n%s", err, out)
	}
	p, err := Launch(LaunchOptions{Target: bin, TempDir: t.TempDir(), StartTimeout: 5 * time.Second})
	if err == nil {
		p.Quit(5 * time.Second)
		t.Fatal("Launch accepted a production executable")
	}
	if !strings.Contains(err.Error(), "build it with -tags tango_dev") {
		t.Fatalf("err %v", err)
	}
}

// A program that exits before writing its token file is reported with its
// exit status, in the words automation has always used.
func TestWaitTokenFileExited(t *testing.T) {
	exited := make(chan struct{})
	close(exited)
	_, err := waitTokenFile(context.Background(), filepath.Join(t.TempDir(), "automation.json"), exited,
		func() error { return errors.New("exit status 1") }, time.Minute)
	const want = "the program exited before opening its control channel (exit status 1); is it a Tango app built with -tags tango_dev?"
	if err == nil || err.Error() != want {
		t.Fatalf("err %v, want %q", err, want)
	}
}

// A done context ends the wait for the token file at once (LaunchContext
// then kills the program).
func TestWaitTokenFileContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	time.AfterFunc(50*time.Millisecond, cancel)
	start := time.Now()
	_, err := waitTokenFile(ctx, filepath.Join(t.TempDir(), "automation.json"), make(chan struct{}), nil, time.Minute)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err %v, want context.Canceled", err)
	}
	if d := time.Since(start); d > 2*time.Second {
		t.Fatalf("returned after %v", d)
	}
}

// LaunchContext with a done context starts nothing.
func TestLaunchContextDone(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := LaunchContext(ctx, LaunchOptions{Target: "./does-not-exist", TempDir: t.TempDir()}); !errors.Is(err, context.Canceled) {
		t.Fatalf("err %v, want context.Canceled", err)
	}
}

// Close ends a request that is waiting for its answer instead of waiting
// for the request's deadline.
func TestCloseEndsDoInFlight(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	go func() {
		c, err := ln.Accept()
		if err != nil {
			return
		}
		defer c.Close()
		var req wire.Request
		if wire.ReadFrame(c, &req) != nil {
			return
		}
		wire.WriteFrame(c, &wire.Response{ID: req.ID, Version: wire.Version})
		for wire.ReadFrame(c, &req) == nil { // never answers
		}
	}()
	conn, err := Dial(ln.Addr().String(), "t")
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() {
		_, err := conn.Do(&wire.Request{Op: wire.OpPing}, time.Minute)
		done <- err
	}()
	time.Sleep(50 * time.Millisecond)
	start := time.Now()
	conn.Close()
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("Do succeeded after Close")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Do still waiting after Close")
	}
	if d := time.Since(start); d > 2*time.Second {
		t.Fatalf("Close took %v", d)
	}
}
