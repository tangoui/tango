# Self-drawn UI

::: info Translation in progress
Most of this page has not been translated yet. Read the Chinese version: [Self-drawn UI（中文）](/guide/ui). Only the section below is available in English.
:::

Pages already available in English: [Getting started](/en/guide/getting-started), [Core concepts](/en/guide/concepts), [tan CLI](/en/reference/cli).

## Background data and threads

A Go-drawn UI runs in the same process as the backend: call Go functions and read Go structs directly. There is nothing to serialize and no IPC involved. The one thing to get right is threads.

**Thread rules**

- UI state is read and written only on the UI thread. The page function runs on the UI thread (the app's main thread).
- Slow or blocking work (files, network, child processes, `ipc` streams) goes on a goroutine, never into the page function, or the whole window freezes.
- Hand results back with `app.Post`, write them into the state there, and call `Runner.Invalidate()` to request a frame. `Invalidate` is safe from any goroutine; the frame itself still runs on the UI thread.

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
