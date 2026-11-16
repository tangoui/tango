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
| idle physical footprint (median of 3) | 26.3 MiB (25.7 / 26.3 / 26.4) |
| launch → first frame | 175–240 ms |
| stripped binary (`CGO_ENABLED=0 -trimpath -s -w`) | 2.2 MiB |
| WebKit / Metal | not loaded |

`macoslite` is a separate host for small utility windows; it is not a
replacement for `uihost.Open` and has no layout, widgets or accessibility.
