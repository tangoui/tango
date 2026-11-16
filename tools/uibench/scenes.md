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
  as decimal text, color #1d2433, background
  `[#e2d4f3, #d2ebee, #f6dbc9, #dde7cd][(row+col)%4]`. Since 17 is not a
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

# Component scenes (Task 10)

Directly comparable component scenes: each is kept only where all three
frameworks have the component and can make the scene's change from the
frame callback through public API, without simulated pointer or key
input. The numbers, texts and formulas are in `components_data.go`, one
file identical in `tango/`, `mygo/` and `nsgo/` (a test in `tango/`
checks that the three copies are the same); `scene_components.go` in each
program builds the scenes.

## Which components all three have

Evidence is the public API: Tango `ui/`, MyGo v0.9.0 `ui/` and
`docs/ui/`, NSGo `ui/`, `base/` and `docs/public/en/ui/`.

| Component | Tango | MyGo v0.9.0 | NSGo | Scene |
|---|---|---|---|---|
| Button | `ui.Button` (button.go) | `ui.Button` (api_gen.go) | `ui.Button` (ui/button.go) | kept: buttons |
| Single-line input | `ui.Input` (input.go) | `ui.TextInput` | `ui.Input` (ui/input.go, the platform text field) | kept: inputs |
| Multi-line input | `ui.TextArea` | `ui.TextArea` | `ui.Input{Multiline: true}` | kept: textarea |
| Check box | `ui.Checkbox` | `ui.Checkbox` | `ui.Checkbox` (ui/checkbox.go) | kept: checkboxes |
| Radio group | `ui.RadioGroup` | `ui.RadioGroup` + `ui.Radio` | `ui.RadioGroup` + `ui.Radio` (ui/radio.go) | kept: radios |
| Switch | `ui.Switch` | `ui.Switch` | `ui.Switch` (ui/switch.go) | kept: switches |
| Slider | `ui.Slider` | `ui.Slider` | `ui.Slider` (ui/slider.go) | kept: sliders |
| Progress bar | `ui.Progress` (data_progress.go) | `ui.Progress` | `ui.Progress` (ui/progress.go) | kept: progress |
| Select, open, list scrolled | `ui.Select().Open()`; the open list scrolls only to follow the keyboard highlight (select.go `dropdown`), no scroll API | `ui.Select` returns an Element with no open control; only the unstyled `SelectBase` has `SetOpen` | `ui.Select[T]` (ui/select.go) has no open control | **moved to Task 11** |
| Tabs | `ui.Tabs` (nav.go) | `ui.Tabs` | `ui.Tabs[T]` (ui/tabs.go) | kept: tabs |
| Vertical menu, selectable | `ui.Menu`, inline mode (nav_menu.go) | `ui.Sidebar` + `ui.SidebarItem` (sidebar.go); `MenuButton` is a popup | `ui.Sidebar` (ui/sidebar.go); `ui.Menu` is a popup from a trigger | kept: menu |
| Tooltip, shown and hidden | `ui.Tooltip` | `Element.Tooltip` / `TooltipBase` (tooltip.go) show only after the pointer rests 0.6 s or the focus comes from the keyboard; no open flag | `ui.Tooltip{Open}` (ui/tooltip.go) | **moved to Task 11** |
| Modal dialog | `ui.Modal` (feedback.go) | `ui.Modal(c, *bool, fn)` | `ui.Dialog{Open}` (ui/dialog.go) | kept: dialog |
| Toast | `ui.Message` (nav_notify.go) | `Context.AddToast` (toast.go) | `ui.Toaster` + `ui.Toasts` (ui/toast.go) | kept: toast |
| Table, 10,000 rows, scrolled | `ui.Table().Virtual()` (table.go) has no scroll API: it scrolls by wheel and keys only (its own benchmark injects `input.Scroll`) | `ui.Table` + `ListState.ScrollTo` | `datatable.DataTable` + `Ref.ScrollToRow` (base/datatable; no styled `ui` table) | **moved to Task 11** |
| Tree, 10,000 nodes, scrolled | `ui.VirtualTree` + `ScrollToKey` (fill_virtual_tree.go) | `ui.Outline` + `ListState.ScrollTo` (outline.go; docs send trees of thousands from `Tree` to `Outline`) | `ui.Tree` + its list `Ref.ScrollToIndexAt` (ui/tree.go) | kept: tree |
| Date panel, open | `ui.DatePickerPanel` (fill_date_panel.go) | `ui.Calendar` (what `DateInput` opens) | `ui.Calendar` (ui/calendar.go; what `DatePicker` opens) | kept: datepicker |
| Color panel, open | `ui.ColorPickerPanel` (fill_color_panel.go) | `ui.ColorPicker` (what `ColorWell` opens) | `ui.ColorPicker` (ui/colorpicker.go; what `ColorWell` opens) | kept: colorpicker |
| Pagination | `ui.Pagination` | none: no pagination in `ui/` or `docs/ui/` | `ui.Pagination` (ui/pagination.go) | **moved to Task 11** |

### Moved to Task 11

- **select**: no framework can both open its styled select and scroll the
  open list from code. Tango's `Select` opens with `Open()`, but its list
  follows only the keyboard highlight; MyGo's `Select` and NSGo's
  `Select` have no open control (MyGo's unstyled `SelectBase` has, with a
  list the app styles itself, which is not the same component).
- **tooltip**: MyGo's tooltip has no open flag; it shows only after the
  pointer rests on the element or the keyboard moves the focus to it, both
  simulated input. A popover styled as a tooltip would be a different
  component.
- **table**: Tango's `Table` has no programmatic scroll (`ScrollY` sets a
  height; scrolling is wheel and keyboard input), so "scroll one row per
  frame" needs simulated input in Tango.
- **pagination**: MyGo has no pagination component.

## Contract

Every scene is in an 800×600 window. Frame `f` is 1 on the frame that
reports ready and counts up from there (Tango and MyGo: the second frame,
as in the base scenes; NSGo: the first frame callback). Pages are padded
16 px, controls 8 px apart, unless said otherwise.

| Scene | Content | Frame `f` | `work` |
|---|---|---|---|
| buttons | 60 buttons `Button 1`…`Button 60`, rows of 6 | button `f%60` reads `Clicked <f>` | 1 |
| inputs | 20 single-line inputs in two columns, each `abcdefghij0123456789` | input `f%20` gets letter `'a'+f%26` appended; past 40 characters it is the 20 again | 1 |
| textarea | one multi-line input filling the page, 40 lines of 50 characters (newline included) = 2000 | letter `'a'+f%26` appended; at 4000 characters back to the 2000 | 1 |
| checkboxes | 100 check boxes `Check 1`…`Check 100`, rows of 5, unchecked | check box `f%100` toggles | 1 |
| radios | 20 groups of 5 radios `Opt 1`…`Opt 5` in a row, two columns of 10 groups, the first chosen | group `f%20` chooses its next radio | 1 |
| switches | 100 switches, 10×10, switch `i` on when `i%3==0` | switch `f%100` toggles | 1 |
| sliders | 20 sliders 0…100 in two columns, slider `i` at `5i` | slider `f%20` +1, past 100 back to 0 | 1 |
| progress | 50 bars 0…100 % in two columns, bar `i` at `2i`, no text | every bar +1, past 100 back to 0 | 50 |
| tabs | 10 tabs `Tab 1`…`Tab 10`; below them the chosen tab's 20 lines `Tab <t>, line <l>: the content of the chosen tab.` | when `f%10==0` the next tab is chosen | 1 on those frames, else 0 |
| menu | a vertical menu of 12 items `Menu item 1`…`Menu item 12`, 240 px wide in a column the window's height | item `f%12` is selected | 1 |
| dialog | a page line of text; a modal titled `Edit profile` with 3 inputs (`Ada Lovelace`, `ada@example.org`, `London`) and buttons `Cancel`, `Save` | when `f%30==0` the dialog opens or closes | 1 on those frames, else 0 |
| toast | a page line of text | when `f%30==0` a toast `Toast number <f/30>` shows, for 1 s | 1 on those frames, else 0 |
| tree | 100 groups `Group <g>` of 99 leaves `Item <g>.<j>` = 10,000 nodes, all expanded, filling the window | row `50 + f%9900` is centered: one row further each frame | 1 |
| datepicker | an inline date panel | the selected day is 1 March 2026 + `f%31` days (one day later each frame, within March) | 1 |
| colorpicker | an inline color panel | the color is hue `f%360`°, saturation 0.75, value 0.85 (`hueRGB`) | 1 |

**Why 12 menu items, not the brief's 50.** The rows are not equally
tall: antd's inline items are 40 px plus a 4 px margin, MyGo's and NSGo's
sidebar rows about 28–32 px, and none of the three scrolls the selected
item into view. With 50 items Tango would show about 13 and the others
about 20, so a different share of the selection changes would be on
screen and painted. 12 items fit in 600 px in all three, so every
selection change is visible everywhere.

The tabs, dialog and toast scenes change something only every 10th or
30th frame, so their work per frame is 0.1 or 1/30 on average, the same
in all three. A one-second report holds about 60 frames, so its count is
6 (tabs) or 2 (dialog, toast) give or take one event at the report's
edges; read over a whole round, those edges are the only difference. The
judge allows for it (R35): its work tolerance for these scenes is
max(5%, 1/expected changes), the expected changes being the frames the
reports cover divided by the period (10 or 30).

## Default motion

Each framework animates some changes by default, for different times.
Where that motion is what a scene mostly measures, the judge reports the
scene as `Info` ("not judged (unequal default motion)") rather than
`Faster`/`NotFaster` (ruling R37; `sceneMetas` in runner/judge.go).

| Scene | Tango (antd) | MyGo | NSGo | Judged |
|---|---|---|---|---|
| dialog, open and close | 300 ms: panel zoom + mask fade, both ways (`modalMotion`, ui/feedback.go, `theme.DurationSlow`) | none: `Modal` shows or not (`coreModal`, ui/widgets.go) | 180 ms fade + pop, dim fades (`dialogFade`/`dialogPop`, ui/dialog.go) | Info |
| toast, in and out | 200 ms in, 200 ms out (`messageRow`, ui/nav_notify.go, `theme.DurationMid`) | 200 ms fade in, fade out at the end (ui/toast.go, `const fade`) | 400 ms spring in, 200 ms out (base/toast `SonnerMotion`; ui/toast.go 200 ms item transition) | Info |
| tabs, indicator | 300 ms ink bar slide (ui/nav.go, `DurationSlow`) | none (ui/tabs.go) | 180 ms indicator slide (`tabsSlide`, ui/tabs.go) | Info |

A motion-free `dialog-still` variant (option (a) of R37) is not offered:
NSGo has no public way for an app to turn its motion off. Its reduced
motion follows the system setting only (`core.Env.ReduceMotion`, set from
`internal/driver.Environment`). Tango could (`ui.RunnerOptions{NoAnimations: true}`),
and MyGo's dialog has no motion anyway.

Smaller per-control motions in the dense scenes (switch knobs, check
marks) are each framework's own and are left to be measured.

## What each framework draws

Each program uses its framework's component with its defaults (Tango:
`theme.AntDesign()`). The components are not equally heavy by nature;
nothing below was changed to compensate.

| Scene | Tango | MyGo | NSGo |
|---|---|---|---|
| buttons | antd buttons, self-drawn; a relabeled button is a new node (its id hashes the label) | self-drawn buttons | `ui.Button`'s `Text` is fixed when built, so each button is a `core.Dynamic` over its label signal and the changed button is built again |
| inputs, textarea | self-drawn inputs | self-drawn inputs | the platform's text field and text view (AppKit), set through `Value` |
| checkboxes, radios, switches, sliders | self-drawn; antd switches animate their knob | self-drawn; the switch knob slides | NSGo's styled views; radios each read their group's signal |
| progress | antd line progress with `HideInfo()`: antd shows the percent by default, the scene's bars show no text (MyGo and NSGo bars have none) | `ui.Progress(v/100)` | `ui.Progress{Value}` (0…100) |
| tabs | antd line tabs and a tab panel | MyGo's tab row; the page is built below it | default segmented tabs (NSGo's look); the 20 lines are texts reading the tab signal |
| menu | antd inline `Menu` in a `ui.Scroll` 600 px high | `Sidebar` of 12 `SidebarItem`s, 600 px high | `Sidebar` of one untitled section of 12 items in a 240×600 view |
| dialog | antd `Modal`: mask, title, close button, body of 3 inputs, footer `Cancel`/`Save`, open and close motion | `Modal` over the dimmed window: bold title, 3 inputs, `Cancel` and primary `Save`; it focuses its first input as it opens | `Dialog` with `Title`, 3 inputs, footer of 2 buttons |
| toast | antd `Message` (info icon) at the top, 1 s | `AddToast` (near the bottom), `Timeout` 1 s, fading in and out | `Toaster.Show`, `Duration` 1 s, `ui.Toasts` at its default placement (top) |
| tree | `VirtualTree`, `Height(600)`; `ScrollToKey` centers the row (and moves the keyboard cursor, drawn only while focused) | `Outline` (builds the rows in view), `ScrollTo(row, ui.Center)` | `ui.Tree`, list `ScrollToIndexAt(row, 0.5, 0, false)` |
| datepicker | `DatePickerPanel` | `Calendar` | `Calendar` |
| colorpicker | `ColorPickerPanel`, antd's panel (saturation/value area, hue and alpha sliders, value inputs); no presets given | `ColorPicker`: square, hue and opacity sliders, hex field, swatches of common colors | `ColorPicker`: saturation/brightness plane, hue and opacity sliders, RGB/HSB/HSL, hex and opacity fields, a copy menu |

Renderer names are as for the base scenes: Tango `native`/`gpu`/`cpu`,
MyGo `metal`/`software`, NSGo `appkit`.
