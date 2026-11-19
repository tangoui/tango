package main

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"image/png"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	wire "github.com/tangoui/tango/internal/automation"
)

// The TestMCPE2E tests run a real tan mcp, built from this package, as a
// subprocess and drive the example apps through it: the agent's view of
// tan mcp. They open real windows (in the background) and are skipped
// unless TANGO_E2E is set, and with -short:
//
//	TANGO_E2E=1 go test -count=1 -run TestMCPE2E -v .

// e2eTan holds a tan binary built for one test and the directory its
// apps' temporary files go to (TMPDIR of the tan process), where the
// tests read the started app's pid from its token file.
type e2eTan struct {
	bin  string
	root string // the repository root, the MCP root
	tmp  string
}

func newE2ETan(t *testing.T) *e2eTan {
	t.Helper()
	if os.Getenv("TANGO_E2E") == "" {
		t.Skip("set TANGO_E2E=1 to run the tan mcp end-to-end tests")
	}
	if testing.Short() {
		t.Skip("-short")
	}
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	bin := filepath.Join(dir, "tan")
	if runtime.GOOS == "windows" {
		bin += ".exe"
	}
	build := exec.Command("go", "build", "-o", bin, ".")
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("go build tan: %v\n%s", err, out)
	}
	tmp := filepath.Join(dir, "tmp")
	if err := os.Mkdir(tmp, 0o755); err != nil {
		t.Fatal(err)
	}
	return &e2eTan{bin: bin, root: root, tmp: tmp}
}

// cmd is the tan mcp command with args after "mcp -root <repo>".
func (e *e2eTan) cmd(args ...string) *exec.Cmd {
	c := exec.Command(e.bin, append([]string{"mcp", "-root", e.root}, args...)...)
	c.Env = append(os.Environ(), "TMPDIR="+e.tmp, "TMP="+e.tmp, "TEMP="+e.tmp)
	return c
}

// connectStdio starts tan mcp over stdio. Cleanup closes the client and
// kills the server if it is still running.
func (e *e2eTan) connectStdio(t *testing.T) (*mcp.ClientSession, *exec.Cmd) {
	t.Helper()
	cmd := e.cmd()
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	tr := &mcp.CommandTransport{Command: cmd, TerminateDuration: 15 * time.Second}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	cs, err := mcp.NewClient(&mcp.Implementation{Name: "e2e", Version: "0"}, nil).Connect(ctx, tr, nil)
	if err != nil {
		if cmd.Process != nil {
			cmd.Process.Kill()
		}
		t.Fatalf("connect to tan mcp: %v\n%s", err, stderr.String())
	}
	t.Cleanup(func() {
		cs.Close()
		if cmd.ProcessState == nil && cmd.Process != nil {
			cmd.Process.Kill()
		}
		if t.Failed() && stderr.Len() > 0 {
			t.Logf("tan mcp stderr:\n%s", stderr.String())
		}
	})
	return cs, cmd
}

// startApp calls tango_app_start for target and returns the session and
// the app's pid. Cleanup kills the app if it is still running.
func (e *e2eTan) startApp(t *testing.T, cs *mcp.ClientSession, target, name string, args ...string) (appSessionOutput, int) {
	t.Helper()
	before := frontmost(t)
	in := map[string]any{"target": target}
	if len(args) > 0 {
		in["args"] = args
	}
	var out appSessionOutput
	callOK(t, cs, "tango_app_start", in, &out)
	pid := e.appPID(t)
	t.Cleanup(func() {
		if processAlive(pid) {
			if p, err := os.FindProcess(pid); err == nil {
				p.Kill()
			}
		}
	})
	if out.Session == "" || len(out.Windows) == 0 {
		t.Fatalf("tango_app_start = %+v, want a session and a window", out)
	}
	if after := frontmost(t); after != "" {
		t.Logf("frontmost before tango_app_start: %q, after: %q", before, after)
		if after == "app" || strings.EqualFold(after, name) {
			t.Errorf("the started app %s came to the front (frontmost before: %q, after: %q)", target, before, after)
		}
	}
	return out, pid
}

// appPID reads the pid of the app tan mcp started from its token file.
func (e *e2eTan) appPID(t *testing.T) int {
	t.Helper()
	files, _ := filepath.Glob(filepath.Join(e.tmp, "tan-mcp-app-*", "automation.json"))
	if len(files) != 1 {
		t.Fatalf("token files of started apps: %v, want one", files)
	}
	tf, err := wire.ReadTokenFile(files[0])
	if err != nil {
		t.Fatal(err)
	}
	if tf.PID <= 0 {
		t.Fatalf("token file %s: pid %d", files[0], tf.PID)
	}
	return tf.PID
}

// processAlive reports whether a process with pid exists.
func processAlive(pid int) bool {
	p, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	// Signal 0 checks without signalling (Windows refuses it with an
	// error other than ErrProcessDone while the process lives).
	err = p.Signal(syscall.Signal(0))
	return err == nil || !errors.Is(err, os.ErrProcessDone)
}

// frontmost is the name of the frontmost application's process on macOS,
// "" elsewhere or when System Events cannot be asked (the assertion is
// then skipped).
func frontmost(t *testing.T) string {
	t.Helper()
	if runtime.GOOS != "darwin" {
		return ""
	}
	out, err := exec.Command("osascript", "-e", `tell application "System Events" to get name of first process whose frontmost is true`).Output()
	if err != nil {
		t.Logf("frontmost app unknown (%v): foreground assertion skipped", err)
		return ""
	}
	return strings.TrimSpace(string(out))
}

func q(role, name string) map[string]any { return map[string]any{"role": role, "name": name} }

// uiPress presses the node as automation.Locator.Click does: the press
// action when it has one, else a click at its center.
func uiPress(t *testing.T, cs *mcp.ClientSession, session string, query map[string]any) {
	t.Helper()
	n := uiFind(t, cs, session, query)
	if hasAction(n, "press") {
		callOK(t, cs, "tango_ui_act", map[string]any{"session": session, "query": query, "action": "press"}, nil)
		return
	}
	uiClickCenter(t, cs, session, n)
}

// uiType focuses the node (or clicks it) and types text, as
// automation.Locator.Type does.
func uiType(t *testing.T, cs *mcp.ClientSession, session string, query map[string]any, text string) {
	t.Helper()
	n := uiFind(t, cs, session, query)
	if hasAction(n, "focus") {
		callOK(t, cs, "tango_ui_act", map[string]any{"session": session, "query": query, "action": "focus"}, nil)
	} else {
		uiClickCenter(t, cs, session, n)
	}
	callOK(t, cs, "tango_ui_type", map[string]any{"session": session, "text": text}, nil)
}

func uiFind(t *testing.T, cs *mcp.ClientSession, session string, query map[string]any) *uiNode {
	t.Helper()
	var out uiFindOutput
	callOK(t, cs, "tango_ui_find", map[string]any{"session": session, "query": query}, &out)
	if out.Node == nil {
		t.Fatalf("tango_ui_find %v: no node", query)
	}
	return out.Node
}

func uiClickCenter(t *testing.T, cs *mcp.ClientSession, session string, n *uiNode) {
	t.Helper()
	callOK(t, cs, "tango_ui_click", map[string]any{
		"session": session, "x": n.Bounds.X + n.Bounds.W/2, "y": n.Bounds.Y + n.Bounds.H/2,
	}, nil)
}

func uiWait(t *testing.T, cs *mcp.ClientSession, session string, query map[string]any) {
	t.Helper()
	callOK(t, cs, "tango_ui_wait", map[string]any{"session": session, "query": query}, nil)
}

func hasAction(n *uiNode, action string) bool {
	for a := range strings.SplitSeq(n.Actions, "|") {
		if strings.EqualFold(strings.TrimSpace(a), action) {
			return true
		}
	}
	return false
}

// appStop stops the session and checks the app exited with code 0.
func appStop(t *testing.T, cs *mcp.ClientSession, session string) {
	t.Helper()
	var stop appStopOutput
	callOK(t, cs, "tango_app_stop", map[string]any{"session": session}, &stop)
	if stop.ExitCode == nil || *stop.ExitCode != 0 || stop.Warning != "" {
		code := "none"
		if stop.ExitCode != nil {
			code = fmt.Sprint(*stop.ExitCode)
		}
		t.Errorf("tango_app_stop: exit code %s, warning %q; want 0", code, stop.Warning)
	}
}

// TestMCPE2ELedger adds a record to the self-drawn ledger example the way
// examples/ledger/e2e_test.go does, through the tango_ui_* tools.
func TestMCPE2ELedger(t *testing.T) {
	e := newE2ETan(t)
	cs, _ := e.connectStdio(t)
	data := filepath.Join(t.TempDir(), "ledger.json")
	app, _ := e.startApp(t, cs, "./examples/ledger", "ledger", "-data", data)
	s := app.Session
	if w := app.Windows[0]; w.Kind != wire.KindCanvas || w.Title != "记账本" {
		t.Fatalf("window %+v, want the canvas window 记账本", w)
	}
	uiWait(t, cs, s, q("heading", "记账本"))

	var tree uiTreeOutput
	callOK(t, cs, "tango_ui_tree", map[string]any{"session": s}, &tree)
	if !strings.Contains(tree.Tree, "button") {
		t.Fatalf("tango_ui_tree has no button:\n%s", tree.Tree)
	}

	uiPress(t, cs, s, q("button", "记一笔"))
	uiWait(t, cs, s, q("dialog", "记一笔"))

	// Saving the empty form is refused by validation.
	uiPress(t, cs, s, q("button", "保存"))
	uiWait(t, cs, s, map[string]any{"name": "请输入金额", "match": "contains"})

	callOK(t, cs, "tango_ui_act", map[string]any{"session": s, "query": q("spinbutton", "金额"), "action": "setValue", "arg": "88.8"}, nil)
	callOK(t, cs, "tango_ui_act", map[string]any{"session": s, "query": q("combobox", "分类"), "action": "expand"}, nil)
	uiPress(t, cs, s, q("option", "交通"))
	uiWait(t, cs, s, map[string]any{"role": "combobox", "name": "分类", "value": "交通"})
	uiType(t, cs, s, q("textbox", "备注"), "打车回家")
	uiWait(t, cs, s, map[string]any{"role": "textbox", "name": "备注", "value": "打车回家"})
	uiPress(t, cs, s, q("button", "保存"))
	callOK(t, cs, "tango_ui_wait", map[string]any{"session": s, "query": q("dialog", "记一笔"), "gone": true}, nil)

	var found uiFindOutput
	callOK(t, cs, "tango_ui_find", map[string]any{"session": s, "query": map[string]any{"role": "row", "name": "打车回家", "match": "contains"}}, &found)
	if found.Node == nil || !strings.Contains(found.Node.Name, "交通") || !strings.Contains(found.Node.Name, "-¥88.80") {
		t.Errorf("new row %+v", found.Node)
	}

	res := call(t, cs, "tango_ui_screenshot", map[string]any{"session": s})
	if res.IsError || len(res.Content) != 1 {
		t.Fatalf("tango_ui_screenshot: %s", resultText(res))
	}
	img, ok := res.Content[0].(*mcp.ImageContent)
	if !ok || img.MIMEType != "image/png" {
		t.Fatalf("tango_ui_screenshot content %T", res.Content[0])
	}
	if m, err := png.Decode(bytes.NewReader(img.Data)); err != nil {
		t.Fatalf("screenshot: %v", err)
	} else if b := m.Bounds(); b.Dx() < 100 || b.Dy() < 100 {
		t.Errorf("screenshot %v", b)
	}

	appStop(t, cs, s)
}

// TestMCPE2EHello drives the WebView hello example the way
// examples/hello/e2e_test.go does, through the tango_web_* tools.
func TestMCPE2EHello(t *testing.T) {
	e := newE2ETan(t)
	cs, _ := e.connectStdio(t)
	app, _ := e.startApp(t, cs, "./examples/hello", "hello")
	s := app.Session
	if w := app.Windows[0]; w.Kind != wire.KindWebView || w.Title != "Tango Hello" {
		t.Fatalf("window %+v, want the WebView window Tango Hello", w)
	}
	callOK(t, cs, "tango_web_wait", map[string]any{"session": s, "expr": `document.readyState === "complete" && !!window.tango`}, nil)
	var eval struct{ Value string }
	callOK(t, cs, "tango_web_eval", map[string]any{"session": s, "expr": "document.title"}, &eval)
	if eval.Value != "Tango Hello" {
		t.Fatalf("title %q", eval.Value)
	}
	var sum struct{ Value int }
	callOK(t, cs, "tango_web_eval", map[string]any{"session": s, "expr": "new Promise(r => setTimeout(() => r(20 + 22), 10))"}, &sum)
	if sum.Value != 42 {
		t.Fatalf("awaited value %d", sum.Value)
	}

	callOK(t, cs, "tango_web_fill", map[string]any{"session": s, "selector": "#name", "text": "自动化"}, nil)
	callOK(t, cs, "tango_web_click", map[string]any{"session": s, "selector": "#go"}, nil)
	callOK(t, cs, "tango_web_wait", map[string]any{"session": s, "expr": `document.getElementById("out").textContent.includes("你好，自动化！")`}, nil)
	var text webTextOutput
	callOK(t, cs, "tango_web_text", map[string]any{"session": s, "selector": "#out"}, &text)
	if !strings.Contains(text.Text, "stream: tick 5") {
		t.Fatalf("output %q", text.Text)
	}

	// The self-drawn tools refuse a WebView window.
	if msg := callErr(t, cs, "tango_ui_tree", map[string]any{"session": s}); !strings.Contains(msg, "tango_web_") {
		t.Errorf("tango_ui_tree on a WebView: %q", msg)
	}

	appStop(t, cs, s)
}

// TestMCPE2EServerExitStopsApps closes the client's stdin (what an agent
// does when it goes away): tan mcp exits and its app is gone within 5 s.
func TestMCPE2EServerExitStopsApps(t *testing.T) {
	e := newE2ETan(t)
	cs, cmd := e.connectStdio(t)
	_, pid := e.startApp(t, cs, "./examples/ledger", "ledger", "-data", filepath.Join(t.TempDir(), "ledger.json"))
	if !processAlive(pid) {
		t.Fatalf("app pid %d is not running after tango_app_start", pid)
	}

	start := time.Now()
	closed := make(chan error, 1)
	go func() { closed <- cs.Close() }() // closes tan mcp's stdin, waits for it to exit
	deadline := start.Add(5 * time.Second)
	for processAlive(pid) {
		if time.Now().After(deadline) {
			t.Fatalf("app pid %d still running %v after the client closed", pid, time.Since(start).Round(time.Millisecond))
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Logf("app exited %v after the client closed", time.Since(start).Round(time.Millisecond))
	select {
	case <-closed:
	case <-time.After(10 * time.Second):
		t.Fatal("tan mcp did not exit after stdin closed")
	}
	if cmd.ProcessState == nil || !cmd.ProcessState.Exited() || cmd.ProcessState.ExitCode() != 0 {
		t.Errorf("tan mcp exit state %v, want exit status 0", cmd.ProcessState)
	}
}

var listeningRE = regexp.MustCompile(`tan mcp: listening on (http://\S+) \(token (\S+)\)`)

// TestMCPE2EHTTPReconnectKeepsSession starts an app over streamable HTTP
// with client A, drops A, and uses the same app session from client B.
func TestMCPE2EHTTPReconnectKeepsSession(t *testing.T) {
	e := newE2ETan(t)
	cmd := e.cmd("-http", "-token", "t", "-addr", "127.0.0.1:0")
	stderr, err := cmd.StderrPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	exited := make(chan struct{})
	t.Cleanup(func() {
		cmd.Process.Signal(os.Interrupt)
		select {
		case <-exited:
		case <-time.After(15 * time.Second):
			cmd.Process.Kill()
			<-exited
		}
	})
	lines := make(chan string, 16)
	go func() {
		sc := bufio.NewScanner(stderr)
		for sc.Scan() {
			select {
			case lines <- sc.Text():
			default:
			}
		}
		cmd.Wait()
		close(exited)
	}()
	var endpoint, token string
	timeout := time.After(30 * time.Second)
	for endpoint == "" {
		select {
		case l := <-lines:
			if m := listeningRE.FindStringSubmatch(l); m != nil {
				endpoint, token = m[1], m[2]
			} else {
				t.Log(l)
			}
		case <-exited:
			t.Fatal("tan mcp -http exited before listening")
		case <-timeout:
			t.Fatal("tan mcp -http printed no listening line")
		}
	}
	if token != "t" {
		t.Fatalf("token %q, want t", token)
	}
	connect := func() *mcp.ClientSession {
		t.Helper()
		tr := &mcp.StreamableClientTransport{
			Endpoint:   endpoint,
			HTTPClient: &http.Client{Transport: bearer{token, http.DefaultTransport}},
			MaxRetries: -1,
		}
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		cs, err := mcp.NewClient(&mcp.Implementation{Name: "e2e", Version: "0"}, nil).Connect(ctx, tr, nil)
		if err != nil {
			t.Fatalf("connect to %s: %v", endpoint, err)
		}
		return cs
	}

	a := connect()
	app, _ := e.startApp(t, a, "./examples/ledger", "ledger", "-data", filepath.Join(t.TempDir(), "ledger.json"))
	if err := a.Close(); err != nil {
		t.Logf("closing client A: %v", err)
	}

	b := connect()
	defer b.Close()
	var ws windowsOutput
	callOK(t, b, "tango_ui_windows", map[string]any{"session": app.Session}, &ws)
	if len(ws.Windows) == 0 || ws.Windows[0].Title != "记账本" {
		t.Fatalf("tango_ui_windows from client B = %+v", ws.Windows)
	}
	appStop(t, b, app.Session)
}
