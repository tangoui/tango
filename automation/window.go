package automation

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	wire "github.com/tangoui/tango/internal/automation"
	"github.com/tangoui/tango/ui/a11y"
)

// Window is one window of the app under test.
type Window struct {
	app  *App
	info WindowInfo
}

// Info returns the window as listed when it was found.
func (w *Window) Info() WindowInfo { return w.info }

// ID returns the window's surface ID.
func (w *Window) ID() uint64 { return w.info.ID }

// Title returns the title the window had when it was found.
func (w *Window) Title() string { return w.info.Title }

// Kind returns KindCanvas or KindWebView.
func (w *Window) Kind() string { return w.info.Kind }

func (w *Window) String() string { return fmt.Sprintf("window %q", w.info.Title) }

func (w *Window) req(op string) *wire.Request { return &wire.Request{Op: op, Window: w.info.ID} }

// ---------------------------------------------------------------------------
// Semantics tree (self-drawn windows)

// Find locates nodes with role whose name is exactly name ("" any name;
// role a11y.RoleNone any role). The locator is lazy: every use looks the
// node up again, waiting for it.
func (w *Window) Find(role a11y.Role, name string) *Locator {
	return w.Locate(Query{Role: roleName(role), Name: name})
}

// FindContains locates nodes with role whose name contains sub
// (case-insensitive).
func (w *Window) FindContains(role a11y.Role, sub string) *Locator {
	return w.Locate(Query{Role: roleName(role), Name: sub, Match: MatchContains})
}

// FindRegexp locates nodes with role whose name matches the Go regular
// expression pattern.
func (w *Window) FindRegexp(role a11y.Role, pattern string) *Locator {
	return w.Locate(Query{Role: roleName(role), Name: pattern, Match: MatchRegexp})
}

// Locate locates the nodes q selects.
func (w *Window) Locate(q Query) *Locator { return &Locator{w: w, q: q} }

// WaitFor waits until l finds its node and returns it.
func (w *Window) WaitFor(l *Locator) a11y.Node {
	w.app.t.Helper()
	return l.Wait()
}

// WaitGone waits until l finds no node.
func (w *Window) WaitGone(l *Locator) {
	w.app.t.Helper()
	l.WaitGone()
}

// Nodes returns the current semantics tree's nodes (waiting for a first
// tree).
func (w *Window) Nodes() []a11y.Node {
	w.app.t.Helper()
	return w.app.call(w.req(wire.OpTree), w.String()+": tree").Nodes
}

// Dump returns the semantics tree as an indented outline (a11y.Tree.Dump),
// handy in failure messages.
func (w *Window) Dump() string {
	w.app.t.Helper()
	return w.app.call(w.req(wire.OpTree), w.String()+": tree").Dump
}

// ---------------------------------------------------------------------------
// Coordinate input (self-drawn windows). Coordinates are logical pixels
// from the top-left of the content, as in a11y.Node.Bounds.

func (w *Window) input(in wire.Input) {
	w.app.t.Helper()
	r := w.req(wire.OpInput)
	r.Input = &in
	w.app.call(r, fmt.Sprintf("%s: %s", w, in.Kind))
}

// Move moves the pointer to (x, y).
func (w *Window) Move(x, y float32) { w.app.t.Helper(); w.input(wire.Input{Kind: "move", X: x, Y: y}) }

// Click clicks the primary button at (x, y).
func (w *Window) Click(x, y float32) {
	w.app.t.Helper()
	w.input(wire.Input{Kind: "click", X: x, Y: y})
}

// DoubleClick double-clicks at (x, y).
func (w *Window) DoubleClick(x, y float32) {
	w.app.t.Helper()
	w.input(wire.Input{Kind: "click", X: x, Y: y, Clicks: 2})
}

// RightClick clicks the secondary button at (x, y).
func (w *Window) RightClick(x, y float32) {
	w.app.t.Helper()
	w.input(wire.Input{Kind: "click", X: x, Y: y, Button: "right"})
}

// Drag presses at (x1, y1), moves to (x2, y2) in a few steps and releases.
func (w *Window) Drag(x1, y1, x2, y2 float32) {
	w.app.t.Helper()
	w.input(wire.Input{Kind: "drag", X: x1, Y: y1, X2: x2, Y2: y2})
}

// Scroll scrolls by (dx, dy) with the pointer at (x, y).
func (w *Window) Scroll(x, y, dx, dy float32) {
	w.app.t.Helper()
	w.input(wire.Input{Kind: "scroll", X: x, Y: y, DX: dx, DY: dy})
}

// Press presses and releases a key with modifiers: "Enter", "Tab", "a",
// "Cmd+N", "CmdOrCtrl+Shift+Z" (key names as ui/input.Key.String prints
// them; CmdOrCtrl is Cmd on macOS). Window shortcuts see it first, as for
// real keys.
func (w *Window) Press(keys string) { w.app.t.Helper(); w.input(wire.Input{Kind: "key", Keys: keys}) }

// Type sends committed text to the focused widget.
func (w *Window) Type(text string) { w.app.t.Helper(); w.input(wire.Input{Kind: "text", Text: text}) }

// ---------------------------------------------------------------------------
// Screenshots (both kinds)

// Screenshot returns a PNG of the window's content: the last frame of a
// self-drawn window (painted again on the CPU), the visible page of a
// WebView window.
func (w *Window) Screenshot() []byte {
	w.app.t.Helper()
	return w.app.call(w.req(wire.OpScreenshot), w.String()+": screenshot").PNG
}

// SaveScreenshot writes Screenshot to path (creating directories).
func (w *Window) SaveScreenshot(path string) {
	w.app.t.Helper()
	png := w.Screenshot()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		w.app.t.Fatalf("automation: %v", err)
	}
	if err := os.WriteFile(path, png, 0o644); err != nil {
		w.app.t.Fatalf("automation: %v", err)
	}
}

// ---------------------------------------------------------------------------
// WebView windows

// Eval evaluates the JavaScript expression expr in the page and returns
// its value as JSON (a Promise is awaited). For statements wrap them:
// `(() => { ...; return x })()`.
func (w *Window) Eval(expr string) json.RawMessage {
	w.app.t.Helper()
	return w.evalTimeout(expr, w.app.timeout)
}

func (w *Window) evalTimeout(expr string, to time.Duration) json.RawMessage {
	w.app.t.Helper()
	r := w.req(wire.OpEval)
	r.Script = expr
	r.Timeout = to.Milliseconds()
	return w.app.call(r, w.String()+": eval").Value
}

// EvalInto evaluates expr and decodes its value into v.
func (w *Window) EvalInto(expr string, v any) {
	w.app.t.Helper()
	raw := w.Eval(expr)
	if err := json.Unmarshal(raw, v); err != nil {
		w.app.t.Fatalf("automation: %s: eval result %s: %v", w, raw, err)
	}
}

// EvalString evaluates expr and returns its value as a string (JSON
// strings unquoted, other values as JSON text).
func (w *Window) EvalString(expr string) string {
	w.app.t.Helper()
	raw := w.Eval(expr)
	var s string
	if json.Unmarshal(raw, &s) == nil {
		return s
	}
	return string(raw)
}

// waitJS is the page-side wait for a CSS selector: it resolves with the
// first matching element.
func waitJS(sel string, to time.Duration) string {
	return `new Promise(function(res,rej){var s=` + jsQuote(sel) + `,t0=Date.now();` +
		`(function p(){var e=document.querySelector(s);if(e)return res(e);` +
		`if(Date.now()-t0>` + fmt.Sprint(to.Milliseconds()) + `)return rej(new Error("no element matches "+s+" after ` + to.String() + `"));` +
		`setTimeout(p,30)})()})`
}

func jsQuote(s string) string {
	b, _ := json.Marshal(s)
	return string(b)
}

// WaitSelector waits until an element matches the CSS selector sel.
func (w *Window) WaitSelector(sel string) {
	w.app.t.Helper()
	to := w.app.timeout
	w.evalTimeout(waitJS(sel, to)+`.then(function(){return true})`, to+2*time.Second)
}

// WaitJS waits until the JavaScript expression expr is truthy.
func (w *Window) WaitJS(expr string) {
	w.app.t.Helper()
	to := w.app.timeout
	w.evalTimeout(`new Promise(function(res,rej){var t0=Date.now();(function p(){var v;try{v=(`+expr+`)}catch(e){}`+
		`if(v)return res(true);if(Date.now()-t0>`+fmt.Sprint(to.Milliseconds())+`)return rej(new Error(`+
		jsQuote("condition not met after "+to.String()+": "+expr)+`));setTimeout(p,30)})()})`, to+2*time.Second)
}

// ClickSelector waits for the element matching sel, scrolls it into view
// and clicks it (HTMLElement.click).
func (w *Window) ClickSelector(sel string) {
	w.app.t.Helper()
	to := w.app.timeout
	w.evalTimeout(waitJS(sel, to)+`.then(function(e){e.scrollIntoView({block:"center",inline:"center"});`+
		`if(e.focus)e.focus();e.click();return true})`, to+2*time.Second)
}

// Fill waits for the input, textarea or contenteditable element matching
// sel, replaces its value with text and fires input and change events (so
// frameworks such as React and Vue see the change).
func (w *Window) Fill(sel, text string) {
	w.app.t.Helper()
	to := w.app.timeout
	w.evalTimeout(waitJS(sel, to)+`.then(function(e){var v=`+jsQuote(text)+`;if(e.focus)e.focus();`+
		`if(e.isContentEditable){e.textContent=v}else{var d=Object.getOwnPropertyDescriptor(Object.getPrototypeOf(e),"value");`+
		`if(d&&d.set){d.set.call(e,v)}else{e.value=v}}`+
		`e.dispatchEvent(new Event("input",{bubbles:true}));e.dispatchEvent(new Event("change",{bubbles:true}));return true})`,
		to+2*time.Second)
}

// TextOf waits for the element matching sel and returns its innerText.
func (w *Window) TextOf(sel string) string {
	w.app.t.Helper()
	to := w.app.timeout
	var s string
	raw := w.evalTimeout(waitJS(sel, to)+`.then(function(e){return e.innerText!==undefined?e.innerText:e.textContent})`, to+2*time.Second)
	if err := json.Unmarshal(raw, &s); err != nil {
		w.app.t.Fatalf("automation: %s: text of %s: %v", w, sel, err)
	}
	return s
}

func roleName(r a11y.Role) string {
	if r == a11y.RoleNone {
		return ""
	}
	return r.String()
}
