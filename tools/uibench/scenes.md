# uibench base scenes

Every framework's program builds the same scene, in its own idiomatic way,
in a window of the same content size, and does the same work per frame.
The numbers below are the contract; `tango/scene_base.go`,
`mygo/scene_base.go` and `nsgo/scene_base.go` each restate them as
constants.

| Scene | Window (content, logical px) | Content | Per frame | `work` |
|---|---|---|---|---|
| counter | 360×260 | centered column, gap 12: caption `Taps` (13 px, #6b7280), the tap count (48 px, bold), a row (gap 10) of buttons `Less`, `Reset`, `More` (the last primary) | nothing: after ready no frame is requested (CPU and memory at rest) | 0 |
| list | 800×600 | virtual list of 100,000 rows, 40 px high, padded 16 px left/right, gap 12, three texts | scroll 8 logical px further (back to 0 at the end) | 1 |
| grid | 800×600 | 30 rows × 20 columns of 40×20 cells, each with its own background, a centered 11 px number | every one of the 600 numbers changes | 600 |

## Data

- List row `i`: `"Row <i>"` (14 px, takes the free width), `"<i*7%1000>"`
  (13 px, #6b7280) and `"<i*13%100> ms"` (13 px, #6b7280).
- Grid cell `cell = row*20 + col` at frame `f`: `(cell*131 + f*17) % 10000`
  as decimal text, color #101010, background
  `[#f6c9c4, #f4e8b8, #c6e9c8, #c9d8f4][(row+col)%4]`. Since 17 is not a
  multiple of 10000, every cell changes every frame.

## Frames and ready

`proto.Frame(work)` is called once per frame that did the scene's work;
the first call prints `bench ready`.

- **Tango** drives frames by calling `c.WakeAt(c.Now())` in every build
  pass and tells a new frame by `c.Now()` changing (a frame can build more
  than once). The first frame is not counted; the second, which follows the
  first presented one, is ready. Window through `uihost.Open`, theme
  `theme.AntDesign()`, default renderer for the size (no `TANGO_RENDERER`).
- **MyGo** (v0.9.0) draws with its self-drawn `ui` package
  (`mygo.NewWindow(mygo.WindowOptions{Content: ui.View(view), UseContentSize: true})`),
  not a web view. The view calls `c.AnimationFrame()` every pass (MyGo's documented
  per-frame request, ui/api_gen.go: "asks for another frame as soon as the
  display can show it") and tells a new
  frame by `c.Now()`, ready on the second frame as Tango.
- **NSGo** chains `cx.RequestAnimationFrame`; its first callback, after the
  first commit, is ready. Default drawing: AppKit views.
- **counter**: Tango and MyGo ask for exactly one more frame after the
  first and report ready (work 0) in it; NSGo reports from one frame
  callback. None asks for frames afterwards.

## Idiomatic choices worth knowing

- Tango list uses `ui.Listy(...).Virtual().ItemHeight(40).Split(false).RowHoverable(false).ScrollTo(px)`:
  `ui.VirtualList` has no pixel scroll API (only `ScrollToIndex` /
  `ScrollIntoView`, whole rows), so it cannot scroll 8 px per frame. Listy
  pads its rows 12/16 and keeps a hit-test box per row; with dividers and
  hover off it draws the same as the others.
- MyGo list scrolls through `ui.List(...).TrackScroll(&state)` by setting
  `ScrollState.Y`; NSGo list through `Ref.ScrollTo(Point{Y: offset}, false)`.
- NSGo grid has one signal of the frame number that all 600 texts read, so
  a frame updates 600 texts, NSGo's fine-grained way; Tango and MyGo rebuild
  the grid every frame, which is theirs.
- Renderer names: Tango prints `uihost.Renderer(r)` (`native`, `gpu` or
  `cpu`); MyGo prints `metal` or `software` from `Painter.Accelerated()` of
  its first painted frame; NSGo prints `appkit`.
