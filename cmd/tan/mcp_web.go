package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"time"
	"unicode/utf8"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	wire "github.com/tangoui/tango/internal/automation"
	"github.com/tangoui/tango/internal/automation/client"
)

// webEvalSlack is added to a tool's timeoutMs for the eval request: the
// page-side script gives up after timeoutMs itself and the app answers
// with its rejection (the same margin package automation uses).
const webEvalSlack = 2 * time.Second

// webEvalMax caps the serialized value tango_web_eval returns.
const webEvalMax = 64 << 10

type webEvalInput struct {
	Session   string `json:"session" jsonschema:"app session id from tango_app_start or tango_app_attach"`
	Window    string `json:"window,omitempty" jsonschema:"window id or exact title (default: the only window, else the focused one)"`
	Expr      string `json:"expr" jsonschema:"JavaScript expression evaluated in the page; a Promise is awaited. Wrap statements: (() => { ...; return x })()"`
	TimeoutMs int    `json:"timeoutMs,omitempty" jsonschema:"how long to wait for the result, in ms (default 10000, at most 120000)"`
}

type webSelectorInput struct {
	Session   string `json:"session" jsonschema:"app session id from tango_app_start or tango_app_attach"`
	Window    string `json:"window,omitempty" jsonschema:"window id or exact title (default: the only window, else the focused one)"`
	Selector  string `json:"selector" jsonschema:"CSS selector; the first matching element is used"`
	TimeoutMs int    `json:"timeoutMs,omitempty" jsonschema:"how long to wait for the element, in ms (default 10000, at most 120000)"`
}

type webFillInput struct {
	Session   string `json:"session" jsonschema:"app session id from tango_app_start or tango_app_attach"`
	Window    string `json:"window,omitempty" jsonschema:"window id or exact title (default: the only window, else the focused one)"`
	Selector  string `json:"selector" jsonschema:"CSS selector of an input, textarea or contenteditable element"`
	Text      string `json:"text" jsonschema:"the new value (replaces the current one)"`
	TimeoutMs int    `json:"timeoutMs,omitempty" jsonschema:"how long to wait for the element, in ms (default 10000, at most 120000)"`
}

type webWaitInput struct {
	Session   string `json:"session" jsonschema:"app session id from tango_app_start or tango_app_attach"`
	Window    string `json:"window,omitempty" jsonschema:"window id or exact title (default: the only window, else the focused one)"`
	Selector  string `json:"selector,omitempty" jsonschema:"wait until an element matches this CSS selector (give selector or expr)"`
	Expr      string `json:"expr,omitempty" jsonschema:"wait until this JavaScript expression is truthy (give selector or expr)"`
	TimeoutMs int    `json:"timeoutMs,omitempty" jsonschema:"how long to wait, in ms (default 10000, at most 120000)"`
}

type webEvalOutput struct {
	// Value is any so that its schema is unrestricted; it holds the
	// page's json.RawMessage, written out as is.
	Value any `json:"value" jsonschema:"the expression's value as JSON (null for undefined); when truncated, a string holding the first 64 KiB of that JSON"`
	// Truncated says Value is a preview of a larger value.
	Truncated bool `json:"truncated,omitempty" jsonschema:"the value's JSON was over 64 KiB: value is a string with its beginning"`
}

type webTextOutput struct {
	Text string `json:"text"`
}

type webEmptyOutput struct{}

// addWebTools registers the tools that drive the WebView pages of an app
// session through JavaScript evaluated in the page.
func (s *mcpServer) addWebTools() {
	mcp.AddTool(s.srv, &mcp.Tool{
		Name:        "tango_web_eval",
		Description: "Evaluate a JavaScript expression in a WebView page and return its value as JSON (a Promise is awaited).",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in webEvalInput) (*mcp.CallToolResult, webEvalOutput, error) {
		if in.Expr == "" {
			return nil, webEvalOutput{}, errors.New("expr: required")
		}
		v, err := s.webEval(ctx, in.Session, in.Window, in.TimeoutMs, func(time.Duration) string { return in.Expr })
		if err != nil {
			return nil, webEvalOutput{}, err
		}
		return nil, evalOutput(v), nil
	})

	mcp.AddTool(s.srv, &mcp.Tool{
		Name:        "tango_web_click",
		Description: "Wait for the element of a WebView page matching a CSS selector, scroll it into view, focus and click it.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in webSelectorInput) (*mcp.CallToolResult, webEmptyOutput, error) {
		if in.Selector == "" {
			return nil, webEmptyOutput{}, errors.New("selector: required")
		}
		_, err := s.webEval(ctx, in.Session, in.Window, in.TimeoutMs, func(to time.Duration) string { return client.ClickJS(in.Selector, to) })
		return nil, webEmptyOutput{}, err
	})

	mcp.AddTool(s.srv, &mcp.Tool{
		Name:        "tango_web_fill",
		Description: "Wait for the input, textarea or contenteditable element of a WebView page matching a CSS selector, replace its value with text and fire input and change events.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in webFillInput) (*mcp.CallToolResult, webEmptyOutput, error) {
		if in.Selector == "" {
			return nil, webEmptyOutput{}, errors.New("selector: required")
		}
		_, err := s.webEval(ctx, in.Session, in.Window, in.TimeoutMs, func(to time.Duration) string { return client.FillJS(in.Selector, in.Text, to) })
		return nil, webEmptyOutput{}, err
	})

	mcp.AddTool(s.srv, &mcp.Tool{
		Name:        "tango_web_text",
		Description: "Wait for the element of a WebView page matching a CSS selector and return its innerText.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in webSelectorInput) (*mcp.CallToolResult, webTextOutput, error) {
		if in.Selector == "" {
			return nil, webTextOutput{}, errors.New("selector: required")
		}
		v, err := s.webEval(ctx, in.Session, in.Window, in.TimeoutMs, func(to time.Duration) string { return client.TextJS(in.Selector, to) })
		if err != nil {
			return nil, webTextOutput{}, err
		}
		var text string
		if err := json.Unmarshal(v, &text); err != nil {
			return nil, webTextOutput{}, fmt.Errorf("text of %s: %s is not a string", in.Selector, v)
		}
		return nil, webTextOutput{Text: text}, nil
	})

	mcp.AddTool(s.srv, &mcp.Tool{
		Name:        "tango_web_wait",
		Description: "Wait until an element of a WebView page matches a CSS selector, or until a JavaScript expression is truthy (give exactly one of selector and expr).",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in webWaitInput) (*mcp.CallToolResult, webEmptyOutput, error) {
		var script func(time.Duration) string
		switch {
		case (in.Selector == "") == (in.Expr == ""):
			return nil, webEmptyOutput{}, errors.New("give exactly one of selector and expr")
		case in.Selector != "":
			script = func(to time.Duration) string { return client.WaitSelectorJS(in.Selector, to) }
		default:
			script = func(to time.Duration) string { return client.WaitExprJS(in.Expr, to) }
		}
		_, err := s.webEval(ctx, in.Session, in.Window, in.TimeoutMs, script)
		return nil, webEmptyOutput{}, err
	})
}

// webEval evaluates script(timeout) in the session's WebView window ref
// and returns its value (null for undefined). The app's errors, a
// script's exception among them, are returned with their message as is.
func (s *mcpServer) webEval(ctx context.Context, session, ref string, ms int, script func(time.Duration) string) (json.RawMessage, error) {
	a, err := s.apps.get(session)
	if err != nil {
		return nil, err
	}
	w, err := a.windowCtx(ctx, ref)
	if err != nil {
		return nil, err
	}
	if w.Kind != wire.KindWebView {
		return nil, selfDrawnError(w)
	}
	to := timeoutMs(ms, uiTimeoutDefault, uiTimeoutMax)
	req := &wire.Request{Op: wire.OpEval, Window: w.ID, Script: script(to), Timeout: (to + webEvalSlack).Milliseconds()}
	// connSlack on top: the app answers an eval that ran out of time itself.
	r, err := a.doOK(ctx, req, to+webEvalSlack+connSlack)
	if err != nil {
		return nil, err
	}
	if len(r.Value) == 0 {
		return json.RawMessage("null"), nil
	}
	return r.Value, nil
}

// evalOutput returns v as is, or when its JSON is over webEvalMax, a
// string with the first webEvalMax bytes of it (cut on a rune boundary).
func evalOutput(v json.RawMessage) webEvalOutput {
	if len(v) <= webEvalMax {
		return webEvalOutput{Value: v}
	}
	b := v[:webEvalMax]
	for i := len(b) - 1; i >= 0 && i >= len(b)-utf8.UTFMax; i-- {
		if utf8.RuneStart(b[i]) {
			if !utf8.FullRune(b[i:]) {
				b = b[:i]
			}
			break
		}
	}
	return webEvalOutput{Value: string(b), Truncated: true}
}

func selfDrawnError(w wire.WindowInfo) error {
	name := w.Title
	if name == "" {
		name = strconv.FormatUint(w.ID, 10)
	}
	return fmt.Errorf("window %q is self-drawn: use tango_ui_* tools", name)
}
