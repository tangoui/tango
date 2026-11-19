package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"image/png"
	"math"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	wire "github.com/tangoui/tango/internal/automation"
	"github.com/tangoui/tango/internal/automation/client"
	"github.com/tangoui/tango/ui/a11y"
)

const (
	appOutputMax     = 256 << 10 // output kept per started app
	appOutputDefault = 16 << 10  // tango_app_output without bytes
	appExitTail      = 4 << 10   // output quoted when the app has exited

	uiTimeoutDefault = 10 * time.Second
	uiTimeoutMax     = 120 * time.Second
	appStartDefault  = 60 * time.Second
	appStartMax      = 300 * time.Second
	// connSlack is added to a request's own wait for the connection
	// deadline: the app answers a timed-out wait itself, and before the
	// window's first semantics tree exists it waits up to the automation
	// server's treeTimeout (5 s) for it on top of the request's timeout.
	// A deadline that fired first would break the connection for good.
	connSlack = 10 * time.Second
	// appStopTimeout bounds a polite quit before the process is killed.
	appStopTimeout = 5 * time.Second

	uiTreeDefault   = 400
	uiTreeMax       = 20000
	uiNotFoundNodes = 200 // tree lines quoted when a query matches nothing
)

// appManager holds the app sessions the UI tools started or attached to.
type appManager struct {
	mu      sync.Mutex
	n       int
	apps    map[string]*appSession
	closed  bool          // closeAll has run: no more sessions
	closing []*appSession // what closeAll (or adopt after it) is stopping, for killAll
}

func newAppManager() *appManager { return &appManager{apps: map[string]*appSession{}} }

// appSession is one app the tools talk to over its automation channel.
type appSession struct {
	ID   string
	conn *client.Conn
	proc *client.Process // nil for attach
	out  *tailBuffer     // the app's stdout and stderr; nil for attach
	// started: the session started the app, so closeAll asks it to quit.
	started bool
	// exited is closed when the app's process has exited (never, for
	// attach); exitErr is then its Wait result.
	exited  <-chan struct{}
	exitErr func() error
	tempDir string // build and token file of a started app; removed on stop
	// addr and token dial a second connection, for a quit while a
	// request holds conn; addr is "" when they are unknown.
	addr, token string
	busy        atomic.Int32 // requests in flight on conn
}

// add registers a and gives it the next ID (app-1, app-2, ...); after
// closeAll it refuses (false).
func (m *appManager) add(a *appSession) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed {
		return false
	}
	m.n++
	a.ID = "app-" + strconv.Itoa(m.n)
	m.apps[a.ID] = a
	return true
}

// adopt adds a, or stops it when the server is shutting down (an app that
// finished starting after closeAll would be left running).
func (m *appManager) adopt(a *appSession) error {
	if !m.add(a) {
		m.mu.Lock()
		m.closing = append(m.closing, a)
		m.mu.Unlock()
		a.stop()
		return errShuttingDown
	}
	return nil
}

func (m *appManager) get(id string) (*appSession, error) {
	if id == "" {
		return nil, errors.New("session: required")
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	a := m.apps[id]
	if a == nil {
		return nil, fmt.Errorf("no app session %q (stopped, or never started)", id)
	}
	return a, nil
}

// remove takes the session id out of the manager and returns it.
func (m *appManager) remove(id string) (*appSession, error) {
	a, err := m.get(id)
	if err != nil {
		return nil, err
	}
	m.mu.Lock()
	delete(m.apps, id)
	m.mu.Unlock()
	return a, nil
}

// list returns the sessions, oldest first.
func (m *appManager) list() []*appSession {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]*appSession, 0, len(m.apps))
	for _, a := range m.apps {
		out = append(out, a)
	}
	slices.SortFunc(out, func(a, b *appSession) int {
		if c := len(a.ID) - len(b.ID); c != 0 {
			return c
		}
		return strings.Compare(a.ID, b.ID)
	})
	return out
}

// closeAll ends every session at once: started apps are asked to quit
// (and killed if they do not), attached ones are only disconnected.
func (m *appManager) closeAll() {
	m.mu.Lock()
	all := m.apps
	m.apps = map[string]*appSession{}
	m.closed = true
	for _, a := range all {
		m.closing = append(m.closing, a)
	}
	m.mu.Unlock()
	var wg sync.WaitGroup
	for _, a := range all {
		wg.Go(func() { a.stop() })
	}
	wg.Wait()
}

// killAll kills the process of every started app at once, also those
// closeAll is still stopping (a second signal), and refuses new sessions.
func (m *appManager) killAll() {
	m.mu.Lock()
	m.closed = true
	all := slices.Clone(m.closing)
	for _, a := range m.apps {
		all = append(all, a)
	}
	m.mu.Unlock()
	for _, a := range all {
		if a.proc != nil {
			a.proc.Cmd.Process.Kill()
		}
	}
}

// stop ends the session: a started app is asked to quit (its process,
// when there is one, killed after appStopTimeout), an attached one is
// disconnected. exitCode is the process's exit code when known.
func (a *appSession) stop() (exitCode *int, err error) {
	if a.tempDir != "" {
		defer os.RemoveAll(a.tempDir)
	}
	if !a.started {
		a.conn.Close()
		return nil, nil
	}
	deadline := time.Now().Add(appStopTimeout)
	if a.proc != nil {
		// A quit stuck behind a request that slipped in: the kill ends it.
		kill := time.AfterFunc(2*appStopTimeout, func() { a.proc.Cmd.Process.Kill() })
		defer kill.Stop()
	}
	select {
	case <-a.exited:
	default:
		a.quit()
	}
	a.conn.Close()
	if a.proc == nil {
		return nil, nil
	}
	select {
	case <-a.exited:
	case <-time.After(time.Until(deadline)):
		a.proc.Cmd.Process.Kill()
		<-a.exited
		err = fmt.Errorf("the app did not exit within %v after quit", appStopTimeout)
	}
	if ps := a.proc.Cmd.ProcessState; ps != nil {
		c := ps.ExitCode()
		exitCode = &c
	}
	return exitCode, err
}

// quit asks the app to quit. A request in flight (a long wait) holds conn
// until it ends: conn is then closed, which fails that request at once,
// and the quit goes over a second connection.
func (a *appSession) quit() {
	if a.busy.Load() == 0 {
		a.conn.Do(&wire.Request{Op: wire.OpQuit}, appStopTimeout)
		return
	}
	a.conn.Close()
	if a.addr == "" {
		return
	}
	c, err := client.Dial(a.addr, a.token)
	if err != nil {
		return
	}
	defer c.Close()
	c.Do(&wire.Request{Op: wire.OpQuit}, appStopTimeout)
}

// exitError describes the exited app, with the end of its output.
func (a *appSession) exitError() error {
	status := "exit status 0"
	if a.exitErr != nil {
		if err := a.exitErr(); err != nil {
			status = err.Error()
		}
	}
	msg := "the app exited (" + status + ")"
	if a.out != nil {
		if tail, _ := a.out.Tail(appExitTail); tail != "" {
			msg += "; its last output:\n" + tail
		}
	}
	return errors.New(msg)
}

// do sends req and waits at most timeout for the answer. It fails at once
// when the app has exited, also while it waits; the connection is closed
// then, since nothing will answer on it. The app's own error stays in the
// response.
func (a *appSession) do(ctx context.Context, req *wire.Request, timeout time.Duration) (*wire.Response, error) {
	select {
	case <-a.exited:
		return nil, a.exitError()
	default:
	}
	type result struct {
		r   *wire.Response
		err error
	}
	ch := make(chan result, 1)
	a.busy.Add(1)
	go func() {
		defer a.busy.Add(-1)
		r, err := a.conn.Do(req, timeout)
		ch <- result{r, err}
	}()
	select {
	case res := <-ch:
		if res.err != nil && a.started {
			// The connection usually breaks a moment before Wait returns.
			select {
			case <-a.exited:
				return nil, a.exitError()
			case <-time.After(time.Second):
			}
		}
		return res.r, res.err
	case <-a.exited:
		a.conn.Close() // ends the Do above
		return nil, a.exitError()
	case <-ctx.Done():
		// The Do goes on and reads its answer, keeping the connection in step.
		return nil, ctx.Err()
	}
}

// doOK is do with the app's error returned as an error.
func (a *appSession) doOK(ctx context.Context, req *wire.Request, timeout time.Duration) (*wire.Response, error) {
	r, err := a.do(ctx, req, timeout)
	if err == nil {
		err = r.Err()
	}
	return r, err
}

func (a *appSession) windows(ctx context.Context) ([]wire.WindowInfo, error) {
	r, err := a.doOK(ctx, &wire.Request{Op: wire.OpWindows}, uiTimeoutDefault)
	if err != nil {
		return nil, err
	}
	if r.Windows == nil {
		return []wire.WindowInfo{}, nil
	}
	return r.Windows, nil
}

// windowCtx finds the window ref names: its ID in decimal or its exact
// title. "" is the only window, else the focused one, else the first. It
// gives up when ctx is done.
func (a *appSession) windowCtx(ctx context.Context, ref string) (wire.WindowInfo, error) {
	ws, err := a.windows(ctx)
	if err != nil {
		return wire.WindowInfo{}, err
	}
	if len(ws) == 0 {
		return wire.WindowInfo{}, errors.New("the app has no open window")
	}
	if ref == "" {
		for _, w := range ws {
			if w.Focused {
				return w, nil
			}
		}
		return ws[0], nil
	}
	if id, err := strconv.ParseUint(ref, 10, 64); err == nil {
		for _, w := range ws {
			if w.ID == id {
				return w, nil
			}
		}
	}
	for _, w := range ws {
		if w.Title == ref {
			return w, nil
		}
	}
	names := make([]string, len(ws))
	for i, w := range ws {
		names[i] = fmt.Sprintf("%d %q", w.ID, w.Title)
	}
	return wire.WindowInfo{}, fmt.Errorf("window: no window %q (open: %s)", ref, strings.Join(names, ", "))
}

// canvas is window refusing WebView windows.
func (a *appSession) canvas(ctx context.Context, ref string) (wire.WindowInfo, error) {
	w, err := a.windowCtx(ctx, ref)
	if err == nil && w.Kind == wire.KindWebView {
		return w, webViewError(w)
	}
	return w, err
}

func webViewError(w wire.WindowInfo) error {
	name := w.Title
	if name == "" {
		name = strconv.FormatUint(w.ID, 10)
	}
	return fmt.Errorf("window %q is a WebView page: use tango_web_* tools", name)
}

// waitOp sends a request that waits up to to for its query; when the app
// gives up, the error quotes the query and the window's tree.
func (a *appSession) waitOp(ctx context.Context, req *wire.Request, to time.Duration) (*wire.Response, error) {
	req.Timeout = to.Milliseconds()
	r, err := a.do(ctx, req, to+connSlack)
	if err != nil {
		return nil, err
	}
	if aerr := r.Err(); aerr != nil {
		if r.Code != wire.CodeTimeout {
			return nil, aerr
		}
		q, _ := json.Marshal(req.Query)
		msg := fmt.Sprintf("%v\nquery: %s", aerr, q)
		dump := r.Dump
		if dump == "" {
			if t, err := a.doOK(ctx, &wire.Request{Op: wire.OpTree, Window: req.Window}, uiTimeoutDefault); err == nil {
				dump = t.Dump
			}
		}
		if dump != "" {
			tree, _ := truncateTree(dump, uiNotFoundNodes)
			msg += "\ntree:\n" + tree
		}
		return nil, errors.New(msg)
	}
	return r, nil
}

// truncateTree keeps the first max lines of a tree dump and says how many
// were left out in a last line.
func truncateTree(dump string, max int) (string, int) {
	dump = strings.TrimRight(dump, "\n")
	if dump == "" {
		return "", 0
	}
	lines := strings.Split(dump, "\n")
	if len(lines) <= max {
		return dump + "\n", 0
	}
	more := len(lines) - max
	return strings.Join(lines[:max], "\n") + fmt.Sprintf("\n… %d more nodes (raise maxNodes)\n", more), more
}

// timeoutMs maps a tool's timeoutMs to a duration: def for 0 (or less),
// at most max.
func timeoutMs(ms int, def, max time.Duration) time.Duration {
	if ms <= 0 {
		return def
	}
	return min(time.Duration(ms)*time.Millisecond, max)
}

// uiNode is a semantics node as the tools report it.
type uiNode struct {
	ID          uint64 `json:"id"`
	Role        string `json:"role"`
	Name        string `json:"name,omitempty"`
	Value       string `json:"value,omitempty"`
	Description string `json:"description,omitempty"`
	States      string `json:"states,omitempty" jsonschema:"set states, | separated"`
	Actions     string `json:"actions,omitempty" jsonschema:"supported actions for tango_ui_act, | separated"`
	Bounds      uiRect `json:"bounds" jsonschema:"logical pixels from the window content's top-left (the space of tango_ui_click)"`
}

type uiRect struct {
	X float32 `json:"x"`
	Y float32 `json:"y"`
	W float32 `json:"w"`
	H float32 `json:"h"`
}

func toUINode(n *a11y.Node) *uiNode {
	if n == nil {
		return nil
	}
	return &uiNode{
		ID: uint64(n.ID), Role: n.Role.String(), Name: n.Name, Value: n.Value, Description: n.Description,
		States: n.States.String(), Actions: n.Actions.String(),
		Bounds: uiRect{n.Bounds.X, n.Bounds.Y, n.Bounds.W, n.Bounds.H},
	}
}

type appStartInput struct {
	Target    string   `json:"target,omitempty" jsonschema:"package directory, or an executable built with -tags tango_dev, relative to the MCP root (default: the root)"`
	Args      []string `json:"args,omitempty" jsonschema:"the app's arguments"`
	Env       []string `json:"env,omitempty" jsonschema:"environment entries to add, as KEY=VALUE; TANGO_ variables are refused"`
	Tags      []string `json:"tags,omitempty" jsonschema:"extra Go build tags (tango_dev is always added)"`
	TimeoutMs int      `json:"timeoutMs,omitempty" jsonschema:"wait for the app's control channel after the build, in ms (default 60000, at most 300000)"`
}

type appAttachInput struct {
	TokenFile string `json:"tokenFile,omitempty" jsonschema:"token file the app wrote (TANGO_AUTOMATION_TOKEN_FILE); a relative path is taken from the MCP root, and the file may be anywhere"`
	Addr      string `json:"addr,omitempty" jsonschema:"control channel address (loopback), with token, instead of tokenFile"`
	Token     string `json:"token,omitempty" jsonschema:"control channel token, with addr"`
}

type appSessionOutput struct {
	Session string            `json:"session"`
	Windows []wire.WindowInfo `json:"windows"`
}

type sessionInput struct {
	Session string `json:"session" jsonschema:"app session id from tango_app_start or tango_app_attach"`
}

type appStopOutput struct {
	ExitCode *int   `json:"exitCode,omitempty" jsonschema:"the process's exit code (absent for an attached app)"`
	Warning  string `json:"warning,omitempty"`
}

type appOutputInput struct {
	Session string `json:"session" jsonschema:"app session id from tango_app_start"`
	Bytes   int    `json:"bytes,omitempty" jsonschema:"last bytes to return (default 16384, at most 262144)"`
}

type appOutputOutput struct {
	Output    string `json:"output"`
	Truncated bool   `json:"truncated"`
}

type windowsOutput struct {
	Windows []wire.WindowInfo `json:"windows"`
}

type uiTreeInput struct {
	Session  string `json:"session" jsonschema:"app session id from tango_app_start or tango_app_attach"`
	Window   string `json:"window,omitempty" jsonschema:"window id or exact title (default: the only window, else the focused one)"`
	MaxNodes int    `json:"maxNodes,omitempty" jsonschema:"lines (nodes) to return (default 400)"`
}

type uiTreeOutput struct {
	Tree      string `json:"tree"`
	Truncated int    `json:"truncated" jsonschema:"nodes left out"`
}

type uiQueryInput struct {
	Session   string      `json:"session" jsonschema:"app session id from tango_app_start or tango_app_attach"`
	Window    string      `json:"window,omitempty" jsonschema:"window id or exact title (default: the only window, else the focused one)"`
	Query     *wire.Query `json:"query" jsonschema:"node query: role (a11y role name such as button, textbox), name, match (exact, contains, regexp), nth, states (such as checked or !disabled), value"`
	TimeoutMs int         `json:"timeoutMs,omitempty" jsonschema:"how long to wait, in ms (default 10000, at most 120000)"`
}

type uiActInput struct {
	Session   string      `json:"session" jsonschema:"app session id from tango_app_start or tango_app_attach"`
	Window    string      `json:"window,omitempty" jsonschema:"window id or exact title (default: the only window, else the focused one)"`
	Query     *wire.Query `json:"query" jsonschema:"node query: role (a11y role name such as button, textbox), name, match (exact, contains, regexp), nth, states (such as checked or !disabled), value"`
	Action    string      `json:"action" jsonschema:"press, increment, decrement, expand, collapse, setValue, focus, scrollIntoView or dismiss"`
	Arg       string      `json:"arg,omitempty" jsonschema:"the action's argument (setValue: the value)"`
	TimeoutMs int         `json:"timeoutMs,omitempty" jsonschema:"how long to wait for the node, in ms (default 10000, at most 120000)"`
}

type uiWaitInput struct {
	Session   string      `json:"session" jsonschema:"app session id from tango_app_start or tango_app_attach"`
	Window    string      `json:"window,omitempty" jsonschema:"window id or exact title (default: the only window, else the focused one)"`
	Query     *wire.Query `json:"query" jsonschema:"node query: role (a11y role name such as button, textbox), name, match (exact, contains, regexp), nth, states (such as checked or !disabled), value"`
	Gone      bool        `json:"gone,omitempty" jsonschema:"wait until no node matches instead"`
	TimeoutMs int         `json:"timeoutMs,omitempty" jsonschema:"how long to wait, in ms (default 10000, at most 120000)"`
}

type uiClickInput struct {
	Session string  `json:"session" jsonschema:"app session id from tango_app_start or tango_app_attach"`
	Window  string  `json:"window,omitempty" jsonschema:"window id or exact title (default: the only window, else the focused one)"`
	X       float32 `json:"x" jsonschema:"logical pixels from the left of the window content (node bounds space)"`
	Y       float32 `json:"y" jsonschema:"logical pixels from the top of the window content"`
	Button  string  `json:"button,omitempty" jsonschema:"left (default), right or middle"`
	Clicks  int     `json:"clicks,omitempty" jsonschema:"1 (default), 2 for a double click"`
}

type uiTypeInput struct {
	Session string `json:"session" jsonschema:"app session id from tango_app_start or tango_app_attach"`
	Window  string `json:"window,omitempty" jsonschema:"window id or exact title (default: the only window, else the focused one)"`
	Text    string `json:"text" jsonschema:"text to commit to the focused field"`
}

type uiKeyInput struct {
	Session string `json:"session" jsonschema:"app session id from tango_app_start or tango_app_attach"`
	Window  string `json:"window,omitempty" jsonschema:"window id or exact title (default: the only window, else the focused one)"`
	Keys    string `json:"keys" jsonschema:"key chord such as Enter, Cmd+S, CmdOrCtrl+Shift+Z"`
}

type uiScreenshotInput struct {
	Session string      `json:"session" jsonschema:"app session id from tango_app_start or tango_app_attach"`
	Window  string      `json:"window,omitempty" jsonschema:"window id or exact title (default: the only window, else the focused one)"`
	Query   *wire.Query `json:"query,omitempty" jsonschema:"crop to the bounds of the node this query picks (self-drawn windows)"`
}

type uiFindOutput struct {
	Node  *uiNode `json:"node,omitempty"`
	Count int     `json:"count" jsonschema:"nodes the query matched"`
}

type uiNodeOutput struct {
	Node *uiNode `json:"node,omitempty" jsonschema:"the node (absent after waiting for gone)"`
}

type uiEmptyOutput struct{}

// addUITools registers the tools that start or attach to apps and drive
// their self-drawn windows through the automation channel.
func (s *mcpServer) addUITools() {
	mcp.AddTool(s.srv, &mcp.Tool{
		Name:        "tango_app_start",
		Description: "Build target with -tags tango_dev (unless it is such an executable), start it in the background with the automation channel on and connect. Returns the session id the tango_ui_* tools take, and the open windows.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in appStartInput) (*mcp.CallToolResult, appSessionOutput, error) {
		out, err := s.startApp(ctx, in)
		return nil, out, err
	})

	mcp.AddTool(s.srv, &mcp.Tool{
		Name:        "tango_app_attach",
		Description: "Connect to a running tango_dev app through its token file, or its control address and token. tango_app_stop only disconnects from it.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in appAttachInput) (*mcp.CallToolResult, appSessionOutput, error) {
		out, err := s.attachApp(ctx, in)
		return nil, out, err
	})

	mcp.AddTool(s.srv, &mcp.Tool{
		Name:        "tango_app_stop",
		Description: "End a session: an app tango_app_start started is asked to quit (killed after 5 s); an attached one is disconnected.",
	}, func(_ context.Context, _ *mcp.CallToolRequest, in sessionInput) (*mcp.CallToolResult, appStopOutput, error) {
		a, err := s.apps.remove(in.Session)
		if err != nil {
			return nil, appStopOutput{}, err
		}
		code, err := a.stop()
		out := appStopOutput{ExitCode: code}
		if err != nil {
			out.Warning = err.Error()
		}
		return nil, out, nil
	})

	mcp.AddTool(s.srv, &mcp.Tool{
		Name:        "tango_app_output",
		Description: "The end of a started app's stdout and stderr (the last 256 KiB are kept).",
	}, func(_ context.Context, _ *mcp.CallToolRequest, in appOutputInput) (*mcp.CallToolResult, appOutputOutput, error) {
		a, err := s.apps.get(in.Session)
		if err != nil {
			return nil, appOutputOutput{}, err
		}
		if a.out == nil {
			return nil, appOutputOutput{}, fmt.Errorf("session %s is attached: its output is not captured", a.ID)
		}
		n := in.Bytes
		if n <= 0 {
			n = appOutputDefault
		}
		text, trunc := a.out.Tail(min(n, appOutputMax))
		return nil, appOutputOutput{Output: text, Truncated: trunc}, nil
	})

	mcp.AddTool(s.srv, &mcp.Tool{
		Name:        "tango_ui_windows",
		Description: "The app's open windows: id, title, kind (canvas for self-drawn, webview for a page), visible, focused, size in logical pixels.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in sessionInput) (*mcp.CallToolResult, windowsOutput, error) {
		a, err := s.apps.get(in.Session)
		if err != nil {
			return nil, windowsOutput{}, err
		}
		ws, err := a.windows(ctx)
		return nil, windowsOutput{Windows: ws}, err
	})

	mcp.AddTool(s.srv, &mcp.Tool{
		Name:        "tango_ui_tree",
		Description: "A self-drawn window's semantics tree as an indented outline, one node per line: role, \"name\", value, [states].",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in uiTreeInput) (*mcp.CallToolResult, uiTreeOutput, error) {
		a, w, err := s.canvasTarget(ctx, in.Session, in.Window)
		if err != nil {
			return nil, uiTreeOutput{}, err
		}
		r, err := a.doOK(ctx, &wire.Request{Op: wire.OpTree, Window: w.ID, Timeout: uiTimeoutDefault.Milliseconds()}, uiTimeoutDefault+connSlack)
		if err != nil {
			return nil, uiTreeOutput{}, err
		}
		max := in.MaxNodes
		if max <= 0 {
			max = uiTreeDefault
		}
		tree, more := truncateTree(r.Dump, min(max, uiTreeMax))
		return nil, uiTreeOutput{Tree: tree, Truncated: more}, nil
	})

	mcp.AddTool(s.srv, &mcp.Tool{
		Name:        "tango_ui_find",
		Description: "Wait for a node of a self-drawn window to match query and return it with its bounds and the number of matches. When nothing matches in time the error quotes the tree.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in uiQueryInput) (*mcp.CallToolResult, uiFindOutput, error) {
		r, err := s.queryOp(ctx, in.Session, in.Window, in.Query, &wire.Request{Op: wire.OpFind}, in.TimeoutMs)
		if err != nil {
			return nil, uiFindOutput{}, err
		}
		return nil, uiFindOutput{Node: toUINode(r.Node), Count: r.Count}, nil
	})

	mcp.AddTool(s.srv, &mcp.Tool{
		Name:        "tango_ui_act",
		Description: "Wait for a node of a self-drawn window to match query and perform an accessibility action on it (press, setValue with arg, focus, expand, ...), then let the UI settle.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in uiActInput) (*mcp.CallToolResult, uiNodeOutput, error) {
		if in.Action == "" {
			return nil, uiNodeOutput{}, errors.New("action: required")
		}
		r, err := s.queryOp(ctx, in.Session, in.Window, in.Query, &wire.Request{Op: wire.OpAction, Action: in.Action, Arg: in.Arg}, in.TimeoutMs)
		if err != nil {
			return nil, uiNodeOutput{}, err
		}
		return nil, uiNodeOutput{Node: toUINode(r.Node)}, nil
	})

	mcp.AddTool(s.srv, &mcp.Tool{
		Name:        "tango_ui_wait",
		Description: "Wait until a node of a self-drawn window matches query, or with gone until none does.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in uiWaitInput) (*mcp.CallToolResult, uiNodeOutput, error) {
		r, err := s.queryOp(ctx, in.Session, in.Window, in.Query, &wire.Request{Op: wire.OpWait, Gone: in.Gone}, in.TimeoutMs)
		if err != nil {
			return nil, uiNodeOutput{}, err
		}
		return nil, uiNodeOutput{Node: toUINode(r.Node)}, nil
	})

	mcp.AddTool(s.srv, &mcp.Tool{
		Name:        "tango_ui_click",
		Description: "Click at a point of a self-drawn window, in logical pixels from its content's top-left (the space of node bounds).",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in uiClickInput) (*mcp.CallToolResult, uiEmptyOutput, error) {
		return nil, uiEmptyOutput{}, s.input(ctx, in.Session, in.Window, &wire.Input{Kind: "click", X: in.X, Y: in.Y, Button: in.Button, Clicks: in.Clicks})
	})

	mcp.AddTool(s.srv, &mcp.Tool{
		Name:        "tango_ui_type",
		Description: "Commit text to the focused field of a self-drawn window, as an input method would.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in uiTypeInput) (*mcp.CallToolResult, uiEmptyOutput, error) {
		if in.Text == "" {
			return nil, uiEmptyOutput{}, errors.New("text: required")
		}
		return nil, uiEmptyOutput{}, s.input(ctx, in.Session, in.Window, &wire.Input{Kind: "text", Text: in.Text})
	})

	mcp.AddTool(s.srv, &mcp.Tool{
		Name:        "tango_ui_key",
		Description: "Press a key or chord in a self-drawn window (Enter, Escape, Tab, Cmd+S, CmdOrCtrl+Shift+Z).",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in uiKeyInput) (*mcp.CallToolResult, uiEmptyOutput, error) {
		if in.Keys == "" {
			return nil, uiEmptyOutput{}, errors.New("keys: required")
		}
		return nil, uiEmptyOutput{}, s.input(ctx, in.Session, in.Window, &wire.Input{Kind: "key", Keys: in.Keys})
	})

	// The result is an image, not structured output: Out is any, so no
	// output schema is declared.
	mcp.AddTool(s.srv, &mcp.Tool{
		Name:        "tango_ui_screenshot",
		Description: "A PNG of a window's content; with query, cropped to that node's bounds (self-drawn windows).",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in uiScreenshotInput) (*mcp.CallToolResult, any, error) {
		b, err := s.screenshot(ctx, in)
		if err != nil {
			return nil, nil, err
		}
		return &mcp.CallToolResult{Content: []mcp.Content{&mcp.ImageContent{Data: b, MIMEType: "image/png"}}}, nil, nil
	})
}

// canvasTarget finds the session and its self-drawn window ref.
func (s *mcpServer) canvasTarget(ctx context.Context, session, ref string) (*appSession, wire.WindowInfo, error) {
	a, err := s.apps.get(session)
	if err != nil {
		return nil, wire.WindowInfo{}, err
	}
	w, err := a.canvas(ctx, ref)
	return a, w, err
}

// queryOp sends req (find, wait or action) with q to a self-drawn window.
func (s *mcpServer) queryOp(ctx context.Context, session, ref string, q *wire.Query, req *wire.Request, ms int) (*wire.Response, error) {
	if q == nil {
		return nil, errors.New("query: required")
	}
	a, w, err := s.canvasTarget(ctx, session, ref)
	if err != nil {
		return nil, err
	}
	req.Window, req.Query = w.ID, q
	return a.waitOp(ctx, req, timeoutMs(ms, uiTimeoutDefault, uiTimeoutMax))
}

func (s *mcpServer) input(ctx context.Context, session, ref string, in *wire.Input) error {
	a, w, err := s.canvasTarget(ctx, session, ref)
	if err != nil {
		return err
	}
	_, err = a.doOK(ctx, &wire.Request{Op: wire.OpInput, Window: w.ID, Input: in}, uiTimeoutDefault+connSlack)
	return err
}

func (s *mcpServer) screenshot(ctx context.Context, in uiScreenshotInput) ([]byte, error) {
	a, err := s.apps.get(in.Session)
	if err != nil {
		return nil, err
	}
	w, err := a.windowCtx(ctx, in.Window)
	if err != nil {
		return nil, err
	}
	var node *a11y.Node
	if in.Query != nil {
		if w.Kind == wire.KindWebView {
			return nil, webViewError(w)
		}
		r, err := a.waitOp(ctx, &wire.Request{Op: wire.OpFind, Window: w.ID, Query: in.Query}, uiTimeoutDefault)
		if err != nil {
			return nil, err
		}
		node = r.Node
	}
	r, err := a.doOK(ctx, &wire.Request{Op: wire.OpScreenshot, Window: w.ID}, uiTimeoutDefault+connSlack)
	if err != nil {
		return nil, err
	}
	if node == nil {
		return r.PNG, nil
	}
	return cropPNG(r.PNG, node.Bounds, w.Width)
}

// cropPNG cuts the logical rectangle b out of a window screenshot; the
// scale is the image width over the window's logical width.
func cropPNG(data []byte, b a11y.Rect, logicalWidth int) ([]byte, error) {
	img, err := png.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("decoding the screenshot: %v", err)
	}
	ib := img.Bounds()
	scale := 1.0
	if logicalWidth > 0 {
		scale = float64(ib.Dx()) / float64(logicalWidth)
	}
	px := func(v float32, round func(float64) float64) int { return int(round(float64(v) * scale)) }
	r := image.Rect(px(b.X, math.Floor), px(b.Y, math.Floor), px(b.X+b.W, math.Ceil), px(b.Y+b.H, math.Ceil)).Add(ib.Min).Intersect(ib)
	if r.Empty() {
		return nil, fmt.Errorf("the node's bounds %+v are outside the window", b)
	}
	sub, ok := img.(interface {
		SubImage(image.Rectangle) image.Image
	})
	if !ok {
		return nil, fmt.Errorf("cannot crop a %T", img)
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, sub.SubImage(r)); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// checkAppEnv accepts KEY=VALUE entries whose key is not a TANGO_
// variable: tan mcp sets those (the control channel, the background start).
func checkAppEnv(env []string) error {
	for _, kv := range env {
		k, _, ok := strings.Cut(kv, "=")
		if !ok || k == "" {
			return fmt.Errorf("env: %q is not KEY=VALUE", kv)
		}
		if strings.HasPrefix(strings.ToUpper(k), "TANGO_") {
			return fmt.Errorf("env: %s: TANGO_ variables are set by tan mcp and cannot be passed", k)
		}
	}
	return nil
}

func (s *mcpServer) startApp(ctx context.Context, in appStartInput) (appSessionOutput, error) {
	ctx, done, err := s.track(ctx)
	if err != nil {
		return appSessionOutput{}, err
	}
	defer done()
	if err := checkAppEnv(in.Env); err != nil {
		return appSessionOutput{}, err
	}
	if err := checkArgValues("tags", in.Tags); err != nil {
		return appSessionOutput{}, err
	}
	p, err := s.resolve(in.Target)
	if err != nil {
		return appSessionOutput{}, err
	}
	fi, err := os.Stat(p)
	if err != nil {
		return appSessionOutput{}, fmt.Errorf("target: %w", err)
	}
	// A package builds and runs in its directory; an executable runs in
	// the root.
	target, dir := p, s.root
	if fi.IsDir() {
		target, dir = ".", p
	} else if len(in.Tags) > 0 {
		return appSessionOutput{}, errors.New("tags: target is an executable, which is not built")
	}
	tmp, err := os.MkdirTemp("", "tan-mcp-app-")
	if err != nil {
		return appSessionOutput{}, err
	}
	o := client.LaunchOptions{
		// Env comes after tan mcp's own environment, which cannot turn
		// off the background launch then (in.Env has no TANGO_ entry).
		Target: target, Dir: dir, Args: in.Args, Env: append(slices.Clone(in.Env), client.EnvLaunchBackground+"=1"),
		Output:       newTailBuffer(appOutputMax),
		StartTimeout: timeoutMs(in.TimeoutMs, appStartDefault, appStartMax),
		TempDir:      tmp,
	}
	if len(in.Tags) > 0 {
		o.BuildFlags = []string{"-tags", strings.Join(in.Tags, ",")}
	}
	out := o.Output.(*tailBuffer)
	proc, err := client.LaunchContext(ctx, o)
	if err != nil {
		os.RemoveAll(tmp)
		if ctx.Err() != nil {
			return appSessionOutput{}, context.Cause(ctx)
		}
		if tail, _ := out.Tail(appExitTail); tail != "" {
			err = fmt.Errorf("%v\nthe app's last output:\n%s", err, tail)
		}
		return appSessionOutput{}, err
	}
	a := &appSession{conn: proc.Conn, proc: proc, out: out, started: true, exited: proc.Exited(), exitErr: proc.ExitErr, tempDir: tmp,
		addr: proc.Channel.Addr, token: proc.Channel.Token}
	if err := s.apps.adopt(a); err != nil {
		return appSessionOutput{}, err
	}
	// The channel opens before the first window may have: give it a moment.
	deadline := time.Now().Add(5 * time.Second)
	for {
		ws, err := a.windows(ctx)
		if err != nil {
			// Not found: closeAll or tango_app_stop has it and stops it.
			if _, rerr := s.apps.remove(a.ID); rerr == nil {
				a.stop()
			}
			return appSessionOutput{}, err
		}
		if len(ws) > 0 || time.Now().After(deadline) {
			return appSessionOutput{Session: a.ID, Windows: ws}, nil
		}
		select {
		case <-ctx.Done():
			// The caller will not learn the session id: do not keep the app.
			if _, rerr := s.apps.remove(a.ID); rerr == nil {
				a.stop()
			}
			return appSessionOutput{}, context.Cause(ctx)
		case <-time.After(100 * time.Millisecond):
		}
	}
}

func (s *mcpServer) attachApp(ctx context.Context, in appAttachInput) (appSessionOutput, error) {
	addr, token := in.Addr, in.Token
	switch {
	case in.TokenFile != "":
		if addr != "" || token != "" {
			return appSessionOutput{}, errors.New("give tokenFile, or addr and token, not both")
		}
		// The file may be outside the root: the token in it decides
		// whether the app lets us in.
		p := in.TokenFile
		if !filepath.IsAbs(p) {
			p = filepath.Join(s.root, p)
		}
		tf, err := wire.ReadTokenFile(p)
		if err != nil {
			return appSessionOutput{}, err
		}
		addr, token = tf.Addr, tf.Token
	case addr == "" || token == "":
		return appSessionOutput{}, errors.New("tokenFile, or addr and token: required")
	default:
		// Checked before dialing, so nothing goes to another machine: a
		// loopback host:port, or a unix socket (wire.ParseAddr, which
		// Dial uses too, accepts only these).
		if _, _, err := wire.ParseAddr(addr); err != nil {
			return appSessionOutput{}, fmt.Errorf("addr: %w", err)
		}
	}
	conn, err := client.Dial(addr, token)
	if err != nil {
		return appSessionOutput{}, fmt.Errorf("connect to %s: %w", addr, err)
	}
	a := &appSession{conn: conn, exited: make(chan struct{}), exitErr: func() error { return nil }, addr: addr, token: token}
	ws, err := a.windows(ctx)
	if err != nil {
		conn.Close()
		return appSessionOutput{}, err
	}
	if err := s.apps.adopt(a); err != nil {
		return appSessionOutput{}, err
	}
	return appSessionOutput{Session: a.ID, Windows: ws}, nil
}
