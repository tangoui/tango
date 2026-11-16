# macoslite-counter

The reference window for the [`macoslite`](../../macoslite) package: a counter
with three buttons, drawn with `Canvas.Fill`, `Canvas.RoundRect` and
`Canvas.Text` only. It is the page used to compare the idle memory of a
native lite window with other frameworks.

```bash
CGO_ENABLED=0 go run ./examples/macoslite-counter   # macOS only
```

Click the buttons, or press `+`/`=`, `-`, `0`; `Esc` quits.

## Memory

```bash
bash scripts/macoslite-memory.sh --rounds 3
```

Measured on an Apple Silicon Mac (Retina), screen unlocked, 2026-11-16:

| | |
|---|---|
| idle physical footprint (median of 3) | 25.5–27.2 MiB over three runs (rounds 25.4–29.1) |
| launch → first frame (median) | 193 ms (rounds 172–652; the first launch after a build is slowest) |
| stripped binary (`CGO_ENABLED=0 -trimpath -s -w`) | 2.2 MiB |
| WebView / Metal device | never created; no WebContent process |

AppKit still maps `Metal.framework` as a dependency, and on macOS 15+
WritingToolsUI maps `WebKit.framework` once the window becomes key; the
script lists both under "mapped images". Their dirty pages are counted in
the footprint above.

`macoslite` is a separate host for small utility windows; it is not a
replacement for `uihost.Open` and has no layout, widgets or accessibility.
