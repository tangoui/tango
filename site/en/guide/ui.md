# Self-drawn UI

`github.com/tangoui/tango/ui` provides an immediate-mode Go API with a retained scene, layout, and controls such as forms, tables and menus. `uihost` displays the scene in a native window or mobile page without requiring a WebView.

Apple Silicon macOS starts on native Core Animation layers. The default `ui.ContentGeneral` moves to Metal once on non-modifier key input, committed text, IME composition, or sustained high frame cost. `ui.RunnerOptions{Content: ui.ContentData}` keeps lists, tables and dashboards on native layers during keyboard navigation. Both modes can switch after six consecutive frames over 8 ms or six consecutive frames of full-window software fallback.

Intel Mac, Windows, Linux, iOS and Android default to GPU rendering for full windows or pages, with CPU fallback. macOS compiles only CoreText by default. Build with `-tags tango_gotext` to restore the go-text path. [Platform support](/en/guide/platforms)

## Open a self-drawn window

```go
package main

import (
    "fmt"

    "github.com/tangoui/tango"
    "github.com/tangoui/tango/ui"
    "github.com/tangoui/tango/ui/layout"
    "github.com/tangoui/tango/uihost"
)

var name string
var clicks int

func page(c *ui.Context) {
    ui.Column(c, func() {
        ui.Title(c, "Hello, TanGo", 3)
        ui.Input(c, &name)
        if ui.Button(c, "Click me").Clicked() {
            clicks++
        }
        ui.Text(c, fmt.Sprintf("Clicked %d times", clicks))
    }, layout.Gap(12))
}

func main() {
    app := tango.New(tango.Options{Name: "UI Demo", ID: "com.example.uidemo"})
    app.OnReady(func() {
        uihost.Open(app, uihost.Options{Surface: tango.SurfaceOptions{
            Title: "UI Demo", Width: 800, Height: 600, Resizable: true,
        }}, page, ui.RunnerOptions{})
    })
    app.Run()
}
```

The page function runs once per frame on the main thread. It builds nodes and reads interactions from the previous frame, such as `Clicked()` and `Changed()`. No frame is needed while idle. For complete examples, see `examples/ui-window` and `examples/ui-gallery`.

## Components and themes

Components use Ant Design 6.7 with Ant Design Pro v6, with variants based on Element Plus 2.14.8 and shadcn/ui v4. [Design baseline](https://github.com/tangoui/tango/blob/main/docs/design-baseline.md) · [Component guide](/components/)

The [BubbleList demo](/components/catalog/bubblelist) combines message bubbles and a composer with a simulated streaming reply. Find the other chat widgets under 「AI 对话」 in the component sidebar. The widgets do not connect to a model on their own.

`theme.AntDesign()` is the default. `theme.ElementPlus()` and `theme.Shadcn()` are alternatives. All support `theme.Dark()`, `theme.WithPrimary(...)` and `theme.Compact()`.

```go
t := theme.AntDesign(theme.Dark(), theme.WithPrimary(theme.Hex("#13c2c2")))
r, _, err := uihost.Open(app, opts, page, ui.RunnerOptions{Theme: t})
```

`theme.ShadcnOptions` can change the base palette, radius, color theme or primary color. The default base is `neutral`, with a 10 px radius. `Primary` overrides the primary color from `Color` when both are set. `t.Shadcn` retains the original shadcn values, which `t.CSSVars()` exports.

### Large data sets

- `AutoComplete`, `Mentions`, `Command`, `Cascader` and `CascaderPanel` accept `Virtual(bool)` to force virtualization on or off. Increment `DataVersion(int)` when changing option data in place to invalidate the cache.
- `List`, `ProList` and `Conversations` accept `Height(float32)` and `Virtual()` to limit which items are built. `List` accepts a fixed `ItemHeight(float32)` or an `EstimatedItemHeight(float32)` for unmeasured items. `RowKey(func(int) any)` identifies cached heights, `ScrollToKey(key, align, offset)` scrolls to an item, and `Range()` returns the last half-open built range `[first, end)`.
- `ProTable`, `EditableProTable`, `CellEditorTable` and `RowEditorTable` accept `ScrollY(float32)` and `Virtual()` for body virtualization. `ProTable` also accepts `RowHeight(float32)`. `Tree` can cache work with `Height` and `DataVersion(int)`. Increment the version when node structure, filtering or check rules change in place.

### Follow system appearance and language

| Option | Effect |
|---|---|
| `ThemeSystem` (default) | Follow OS light/dark mode. Set `AccentFromSystem` to use the OS accent |
| `ThemeLight` / `ThemeDark` | Fix light or dark mode and still use `Flavor`, `Customize` and `AccentFromSystem` |
| `ThemeFixed` | Use `RunnerOptions.Theme`. An explicit runner theme also takes precedence with default host options |
| `LocaleSystem` (default) | Follow `i18n.Locale()`, with English fallback |
| `LocaleFixed` | Leave the default component locale to the application |

Set `Flavor` on `uihost.Options` to generate tokens from the current mode and accent, and `Customize` to adjust the resulting tokens. To change a window theme manually, call `uihost.FollowSystemTheme(r, false)` and `r.SetTheme(t)` outside a frame on the UI thread. Set following to `true` to reapply system appearance. `uihost.FollowingSystemTheme(r)` reports the window's setting. Apps that replace the theme or locale manually also stop following when the next system change detects the override. `uihost.AttachWith` accepts the same host options. [Internationalization](/en/guide/i18n)

### Share tokens with WebView pages

Call `uivars.SyncPage(app, opts.ThemeFor)` before creating surfaces to share the host theme with pages. It injects `--tango-…` variables in `<style id="tango-ui-vars">` and updates them on system appearance changes. Call `uivars.Refresh(app)` after changing app state used by the generator. Read the values through CSS or `Theme.vars()` and `Theme.onVarsChange(fn)` in JS. [System theme API](/guide/system/theme)

## Mix Go-drawn UI and WebView

Use `uihost.Attach` to place a Go-drawn layer above or below a WebView on desktop or mobile. A layer below the page needs a transparent WebView. Mobile overlays use the CPU.

```go
r, att, err := uihost.Attach(webSurface, uihost.Layer{
    Above: true, PassThrough: true,
}, overlayPage, ui.RunnerOptions{})
```

Zero bounds fill the window. `PassThrough` lets pointer events at transparent pixels reach the WebView. Use `att.SetLayer(layer)` to change placement and `att.Detach()` to remove it. See `examples/webview-overlay`.

## Background data and threads

A Go-drawn UI runs in the same process as the backend: call Go functions and read Go structs directly. There is nothing to serialize and no IPC involved. The one thing to get right is threads.

**Thread rules**

- Read and write UI state on the UI thread. With `uihost.Open`, `uihost.Attach`, and `tan dev`, the UI thread is the app's main thread. Dedicated macOS UI threads are currently used only for internal testing.
- Slow or blocking work (files, network, child processes, `ipc` streams) goes on a goroutine, never into the page function, or the whole window freezes.
- Deliver background results with `runner.Post(func() { rows = res })`. `Post` is safe from any goroutine, runs tasks in order on the UI thread, and then requests a frame. `App.Post` schedules work on the main thread for calls such as `uihost.Open` and `uihost.FollowSystemTheme`. `Invalidate` is safe from any goroutine.
- Main-thread callbacks such as `uihost.Shortcut`, menus, tray, `App.OnMessage`, and `Surface.OnClose` can also update page state through `Runner.Post`. Development builds (`-tags tango_dev`) check thread affinity only for runners bound to a dedicated UI thread during internal testing.

**Streaming data** uses plain Go: channels, `io.Reader`/`io.Writer`, callbacks. Blocking reads and writes are the back-pressure: when the goroutine reads slowly, the producer is held back. A terminal panel, for example, has a goroutine read the shell's output and hand it to the UI thread, which appends it to a buffer and calls `Invalidate`; key presses are written to the shell from the event handler. Writing to a local pipe is usually quick but may block; a target that can make writes wait (such as an `ipc` stream waiting for credit, below) should get them through a channel read by a goroutine that does the writing.

**Coalesce high-rate data.** Posting once per chunk piles work up on the UI thread when chunks arrive faster than frames. `examples/duplex/panel.go` appends to a locked buffer from the goroutine and wakes the UI thread only when no refresh is pending yet; the page function takes everything at once and clears the flag.

```go
// push buffers item. It may be called on any goroutine and never blocks
// on the UI thread.
func (c *coalescer) push(item string) {
	c.mu.Lock()
	c.items = append(c.items, item)
	first := !c.pending
	c.pending = true
	c.mu.Unlock()
	if first {
		c.wake() // outside the lock: wake may even call drain
	}
}

// drain takes every buffered item and clears the pending flag, so the
// next push wakes the UI thread again. Call it on the UI thread.
func (c *coalescer) drain() []string {
	c.mu.Lock()
	defer c.mu.Unlock()
	items := c.items
	c.items, c.pending = nil, false
	return items
}
```

The wake function makes the UI thread run a frame, where the page function drains; the state (`p.recent`) only changes on the UI thread:

```go
p.echoes = newCoalescer(func() { app.Post(runner.Invalidate) })
```

```go
func (p *echoPanel) page(c *ui.Context) {
	p.recent = append(p.recent, p.echoes.drain()...)
	if n := len(p.recent); n > recentEchoes {
		p.recent = p.recent[n-recentEchoes:]
	}
	// ... draw p.recent
}
```

**Reuse the WebView's services.** Services already registered for pages with `Handle`, `HandleStream` or `HandleDuplex` are reachable in process with `r.Call(ctx, method, args...)` and `r.Open(ctx, method, args...)`, with exactly the page's semantics but without JSON messages or base64 (see [Core concepts](/en/guide/concepts#ipc-calls-events-streams)). In a hybrid window the page and a Go panel thus share one service instance. These calls block, so follow the thread rules above. The Go panel of `examples/duplex -panel` uses the page's `Echo` service this way:

```go
// run sends {"t": time} on an Echo stream every second and pushes each
// echo to the panel. Open, Write and Receive block, so this runs on its
// own goroutine, never on the UI thread.
func (p *echoPanel) run(ctx context.Context, r *ipc.Router) {
	s, err := r.Open(ctx, "Echo")
	if err != nil {
		p.echoes.push("Open: " + err.Error())
		return
	}
	defer s.Close()

	go func() {
		tick := time.NewTicker(time.Second)
		defer tick.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case now := <-tick.C:
				if s.Write(map[string]string{"t": now.Format("15:04:05")}) != nil {
					return
				}
			}
		}
	}()

	for {
		var m struct {
			T string `json:"t"`
		}
		if err := s.ReceiveJSON(&m); err != nil {
			p.echoes.push("closed: " + err.Error())
			return
		}
		p.echoes.push("← " + m.T)
	}
}
```

A stream from `r.Open` belongs to no page, so `Drop` does not end it when a page closes: `Close` it when done (the `defer s.Close()` above) or cancel `ctx` (the example cancels when the window closes), or the service handler's goroutine never exits. To connect a byte stream such as a shell on the service side, use `s.ReadWriter()` with `io.Copy`, as `examples/duplex/term.go` does. All code here is quoted from `examples/duplex`, where it compiles and is tested.

## Test with Driver

`ui.NewDriver(page, ui.DriverOptions{})` runs a page without a window using simulated time. It can click by label, find text, drag, type, compose text, press keys, scroll, and advance time.

```go
func TestCounter(t *testing.T) {
    d := ui.NewDriver(page, ui.DriverOptions{})
    d.Frame()
    if err := d.ClickLabel("Click me"); err != nil {
        t.Fatal(err)
    }
    d.Frame()
    if _, ok := d.FindText("Clicked 1 times"); !ok {
        t.Fatal("counter did not update")
    }
}
```

## Hot reload and automation

`tan dev` keeps supported `uihost` windows in a host process and replaces the Go worker after a successful rebuild. A compile failure leaves the old worker running and shows the error. Use `uihost.Persist(&state)` to retain gob-encodable state across rebuilds. WebView windows still restart on Go changes. Disable hot reload with `tan dev -hot=false` or `TANGO_HOTRELOAD=0`. See `examples/ui-hotreload`.

Use `automation.Launch` or `tan mcp` to drive a separate `tango_dev` build via its semantic tree, actions or screenshots. The hot-reload host draws the UI in another process, so the app's control channel cannot read that window's semantic tree. [Automation](https://github.com/tangoui/tango/blob/main/docs/automation.md)

[ui](/reference/api/ui) · [uihost](/reference/api/uihost) · [ui/theme](/reference/api/ui-theme)
