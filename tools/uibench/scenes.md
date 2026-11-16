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
| Select, open, list scrolled | `ui.Select().Open()`; the open list scrolls only to follow the keyboard highlight (select.go `dropdown`), no scroll API | `ui.Select` returns an Element with no open control; only the unstyled `SelectBase` has `SetOpen` | `ui.Select[T]` (ui/select.go) has no open control | **Task 11**: select (nearest counterpart) |
| Tabs | `ui.Tabs` (nav.go) | `ui.Tabs` | `ui.Tabs[T]` (ui/tabs.go) | kept: tabs |
| Vertical menu, selectable | `ui.Menu`, inline mode (nav_menu.go) | `ui.Sidebar` + `ui.SidebarItem` (sidebar.go); `MenuButton` is a popup | `ui.Sidebar` (ui/sidebar.go); `ui.Menu` is a popup from a trigger | kept: menu |
| Tooltip, shown and hidden | `ui.Tooltip` | `Element.Tooltip` / `TooltipBase` (tooltip.go) show only after the pointer rests 0.6 s or the focus comes from the keyboard; no open flag | `ui.Tooltip{Open}` (ui/tooltip.go) | **Task 11**: tooltip (nearest counterpart) |
| Modal dialog | `ui.Modal` (feedback.go) | `ui.Modal(c, *bool, fn)` | `ui.Dialog{Open}` (ui/dialog.go) | kept: dialog |
| Toast | `ui.Message` (nav_notify.go) | `Context.AddToast` (toast.go) | `ui.Toaster` + `ui.Toasts` (ui/toast.go) | kept: toast |
| Table, 10,000 rows, scrolled | `ui.Table().Virtual()` (table.go) has no scroll API: it scrolls by wheel and keys only (its own benchmark injects `input.Scroll`) | `ui.Table` + `ListState.ScrollTo` | `datatable.DataTable` + `Ref.ScrollToRow` (base/datatable; no styled `ui` table) | **Task 11**: table (nearest counterpart) |
| Tree, 10,000 nodes, scrolled | `ui.VirtualTree` + `ScrollToKey` (fill_virtual_tree.go) | `ui.Outline` + `ListState.ScrollTo` (outline.go; docs send trees of thousands from `Tree` to `Outline`) | `ui.Tree` + its list `Ref.ScrollToIndexAt` (ui/tree.go) | kept: tree |
| Date panel, open | `ui.DatePickerPanel` (fill_date_panel.go) | `ui.Calendar` (what `DateInput` opens) | `ui.Calendar` (ui/calendar.go; what `DatePicker` opens) | kept: datepicker |
| Color panel, open | `ui.ColorPickerPanel` (fill_color_panel.go) | `ui.ColorPicker` (what `ColorWell` opens) | `ui.ColorPicker` (ui/colorpicker.go; what `ColorWell` opens) | kept: colorpicker |
| Pagination | `ui.Pagination` | none: no pagination in `ui/` or `docs/ui/` | `ui.Pagination` (ui/pagination.go) | **Task 11**: pagination |

### Moved to Task 11

Task 11 gives each a nearest-counterpart scene ("Nearest-counterpart
scenes" below); why they left Task 10:

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

# Nearest-counterpart scenes (Task 11)

The four scenes Task 10 moved here (select, tooltip, table, pagination)
and the twenty components the plan gives scenes of their own. Where an
opponent has the component, its program uses it; where it has none, it
composes the nearest one from the components it has, written from its
documentation the way its docs build such things (never a much weaker
widget). The numbers, texts and formulas are in `components_data.go`
(shared, byte for byte); `scene_nearest.go` in each program builds the
scenes, and `scene_nearest_test.go` in each runs them headless (Tango
`ui.NewDriver`, MyGo `ui.NewTester`, NSGo `nsgotest`) and checks that
frame f shows what the contract below says.

**Built-in texts in English.** From Task 11 on all three programs show
their components' built-in texts in English whatever the system's
language (this machine's is zh-Hans): Tango `ui.SetDefaultLocale(en)`
with `uihost.LocaleFixed`, MyGo `mygo.App.SetLocale("en")`, NSGo
`Scope.SetLocale("en")` (what its `--nsgo-lang` flag does). Without it
Tango drew 查询/重置, "4 项" and Chinese month and weekday names where
the others drew English. This applies to the earlier scenes too (the
date panels' month and weekday names).

## Contract

800×600 window, frame `f` and padding as for the component scenes.

| Scene | Content | Frame `f` | `work` |
|---|---|---|---|
| select | an open select, 240 px wide, of 8 options `Option 1`…`Option 8`, all in view | option `f%8` is chosen | 1 |
| tooltip | a button `Hover target`; a one-line popover `Tooltip text for the target` under it | when `f%30==0` the popover shows or hides | 1 on those frames, else 0 |
| table | a virtual table of 1000 rows, columns `ID`, `Name`, `City`, `Score`, `Latency`, `Updated` (`tableCell`; `Updated` starts at 0) | row `f%8`'s `Updated` cell reads `f` (rows 0–7 are in view in all three); no scrolling | 1 |
| pagination | the 10 items `Item <(p-1)*10+i+1>` of page `p` over a pager of 50 pages | page `p = 1+f%50` | 1 |
| cascader | a field showing the path (`Region a / City a.b / District a.b.c`, 480 px) and, open under it, 3 columns of 5 options, the chosen one marked in each | the path `cascadePath(f)`: the last level moves every frame, the middle every 5th | 1 |
| transfer | two lists 300×400 (`Source`, `Target`, each headed by its count, `N items`) of 10 items `Item 1`…`Item 10` with check boxes, between them the two move buttons; item `i` starts on the right when `i%3==0` | item `f%10` moves to the other side | 1 |
| treeselect | a field 300 px wide showing the chosen leaf and, open under it, a tree of 2 groups `Group g` of 3 leaves `Leaf g.j`, all expanded (8 rows) | leaf `treeSelPick(f)` is chosen | 1 |
| calendar | the framework's month calendar | the selected day is `pickDay(f)` | 1 |
| steps | 5 steps `Step i` with `Details i` | step `f%5` is current | 1 |
| timeline | 8 events, each `Event i` over its time `2026-03-0i 09:00` | event `f%8` reads `Event i, frame f` | 1 |
| descriptions | 12 items `Field i` / `Value i` in 3 bordered columns | item `f%12` reads `Value i @ f` | 1 |
| form | 10 labelled inputs `Field i` in two columns of 5, each `abcdefghij0123456789`, and a `Submit` button | input `f%10` types as in the inputs scene | 1 |
| upload | an `Upload` button over 8 files `file-i.png` uploading, file `i` at `12i` % | file `f%8` +1 %, past 100 back to 0 (so none finishes) | 1 |
| carousel | 5 slides `Slide i`, 240 px high, background #dfe7f2 | when `f%30==0` the next slide | 1 on those frames, else 0 |
| collapse | an accordion of 6 panels `Panel i`, each 3 lines `Panel i, line l: the content of the panel.` | when `f%30==0` the next panel opens and the open one closes | 1 on those frames, else 0 |
| drawer | a page line; a 360 px drawer at the right titled `Settings` with the dialog's 3 inputs and `Cancel`, `Save` | when `f%30==0` the drawer opens or closes | 1 on those frames, else 0 |
| tour | buttons `Upload`, `Save`, `More`; a tour of 3 steps at them (`Upload files`, `Save changes`, `More actions`; `Step i of the tour.`) over a dimming mask | when `f%30==0` the next step | 1 on those frames, else 0 |
| protable | a query form of one field `Name` with `Reset` and `Query` over a table of 20 rows of the table scene's columns | row `f%5`'s `Updated` cell reads `f` (rows 0–4 are in view under the form in all three) | 1 |
| segmented | 10 segmented controls of `Day`, `Week`, `Month`, `Year`; control `i` at choice `i%4` | control `f%10` takes its next choice | 1 |
| rate | 20 five-star ratings in two columns, rating `i` at `i%6` stars | rating `f%20` +1 star, past 5 back to 0 | 1 |
| tags | 60 tags `Tag i` in rows of 6 | tag `f%60` reads `Tag i · f` | 1 |
| badges | 40 squares 40×40 (#d9dde3, radius 6) in rows of 10, each with a count badge at its top-right corner, badge `i` at `i*7%99+1` | badge `f%40` +1, kept in 1…99 | 1 |
| cards | 12 cards in rows of 4: title `Card i`, `Line one of card i.`, `Updated at frame 0` | card `f%12`'s last line reads `Updated at frame f` | 1 |
| statistics | 12 statistics in rows of 4: `Metric i` over its value (100…999.25, two decimals) | statistic `f%12` takes `statValue(i, f)` | 1 |

Counts were chosen so that what changes is in view in all three at their
default sizes (Tango's headless test checks the bounds): the cascader's
5 options fit antd's 180 px columns, the tree select's 8 rows antd's
256 px dropdown, the timeline's 8 events and the form's two columns of
5 the 600 px window, the ProTable's rows 0–4 the room under its query
form and toolbar.

## The four moved scenes

- **select** (R36: Tango cannot scroll an open Select from code). The
  scene is an open select whose chosen option changes each frame, with
  no scrolling. Tango: `ui.Select(...).Open()`. MyGo's and NSGo's styled
  `Select` have no open control, so each shows its `Select`, closed, as
  the field, and under it the open list a select draws: MyGo a `Popover`
  anchored to it holding a `List` with `Selected`; NSGo a `HoverCard`
  (opened from code, its `Trigger` any node) whose trigger is the
  `Select`, holding a `List` with a `Selection`.
- **tooltip**. Tango's and MyGo's tooltips open only on hover or
  keyboard focus (Tango's `Tooltip` has no open flag either), so the
  nearest component all three open from code is a popover holding one
  line: Tango `ui.Popover().Trigger("manual").Open(&open)`, MyGo
  `ui.Popover`, NSGo `ui.Popover` (whose `Trigger` is the button). NSGo's
  real `Tooltip{Open}` is not used, so that all three draw the same kind
  of panel. Shown and hidden every 30th frame, period 30; Info (motion).
- **table** (R36: Tango's Table has no programmatic scroll). The scene
  updates one visible cell a frame in a 1000×6 table, without scrolling.
  Tango `ui.Table(...).Virtual()`, MyGo `ui.Table`, NSGo
  `datatable.DataTable` (base; each row's `Updated` is a signal, so the
  change updates that cell, NSGo's fine-grained way).
- **pagination**. MyGo has no pagination: its pager is composed of
  `Button`s, `‹`, the page buttons antd shows (`pagerItems`: the first and
  last, two either side of the current page, `…` for the pages left out),
  the current one a `PrimaryButton`, and `›`. NSGo's `Pagination` with
  `VisiblePages: 7` shows the same set as antd (headless check at page
  31: `1 … 29 30 31 32 33 … 50` in all three).

## Default motion of the new scenes

As for Task 10 (ruling R37): a scene whose change the frameworks animate
differently by default is reported `Info`, not judged
(`sceneMetas` in runner/judge.go).

| Scene | Tango (antd) | MyGo | NSGo | Judged |
|---|---|---|---|---|
| tooltip, show | 200 ms (`AnimFrom "pop"`, ui/data_popover.go) | none (`corePopover`) | none (ui.Popover sets no starting style) | Info |
| carousel, next slide | 500 ms (`speed`, ui/data_carousel.go) | none (composed) | 362 ms layout spring (base/carousel.go) | Info |
| collapse, panel | 200 ms height (`collMotion`, `DurationMid`) | 200 ms (`coreCollapsibleBase`) | 265 ms spring (`revealTime`, base/collapsible.go) | Info |
| drawer, open and close | 300 ms (`drawerMotion`, `DurationSlow`) | none (composed overlay) | 280 ms in, 200 ms out (base/internal/sheet) | Info |
| segmented, thumb | 300 ms slide (ui/entry_segmented.go) | none (`coreSegmented`) | 180 ms indicator slide (NSGo tabs) | Info |
| steps, current step | none for the default type (the 200 ms ink is the nav type's) | none (composed) | 160–400 ms marker, colour and connector transitions (ui/stepper.go) | Info |
| tour, next step | none (its mask fades in once, when the tour opens) | none | none (Popover) | judged |

The open lists of select, cascader and treeselect stay open, so their
open motion (NSGo's HoverCard fades in over 120 ms once) is not
measured; NSGo's 120 ms background transitions on buttons and rows are
per-control motion, left to be measured as in Task 10.

## What each framework draws

Each uses its component with its defaults where it has one; a composed
counterpart uses the framework's own components and theme colors. None
is made heavier or lighter to match another.

| Scene | Tango | MyGo | NSGo |
|---|---|---|---|
| select | antd `Select`, open (its 200 ms open motion runs once, at the start) | `Select` (closed) + `Popover` of a `List` (rows 6/12 padded) | `Select` (closed) as `HoverCard` trigger + `List` (8 rows of 32) |
| tooltip | antd `Popover`, text only, under the button | `Popover` (at least as wide as the button) | `Popover` with a secondary `Button` trigger |
| table | antd `Table`, virtual, 55 px rows (9 in view) | `Table`, 32 px rows (17 in view) | `DataTable` (base) |
| pagination | antd `Pagination` | composed of buttons | `Pagination` |
| cascader | antd `Cascader`, open: field and a popup of three 180 px columns | `Combobox` + `Popover` of three 160×180 `List`s | `Combobox` (options: the 125 paths) as `HoverCard` trigger + three 160×180 `List`s |
| transfer | antd `Transfer` (300×400 lists, header check box, item check boxes, move buttons) | two bordered columns: header text and count, `Checkbox` rows; two disabled buttons | the same with `ui.Checkbox` and `ui.Button` |
| treeselect | antd `TreeSelect`, open, all expanded | `Select` (closed) + `Popover` of a `Tree` (`TreeItem.Selected`) | `Select` (closed) as `HoverCard` trigger + `Tree` with a `Selection` |
| calendar | antd `Calendar`: the full-page month with year/month selects and a mode switch, cells 80+ px high (heavier by nature) | `Calendar` (its only one; the datepicker scene uses it too) | `Calendar` (likewise) |
| steps | antd `Steps` | composed: numbered circle (check when done, filled when current), title and details, line | `Stepper` |
| timeline | antd `Timeline`, each item's `Body` the two texts | composed: dot and line boxes + two texts | composed likewise with views |
| descriptions | antd `Descriptions`, bordered, 3 columns | composed: rows of bordered text cells, labels on the surface color | composed likewise |
| form | antd `Form` items, horizontal labels, two columns of `Form` | `Form` + `Field` + `TextInput` (labels right-aligned) | `FormSection` of `FormField`s (platform text fields) |
| upload | antd `Upload` text list: icon, name and progress line per file | composed: `Button` + name `Text` and `Progress` | composed likewise |
| carousel | antd `Carousel` with dots | composed: slide column + dot boxes | `Carousel` |
| collapse | antd `Collapse`, accordion | `Accordion` | `Accordion` |
| drawer | antd `Drawer` (mask, header with close button, body, footer) | composed: MyGo's documented drawer (docs/ui/overlays.md): a modal `Overlay` row with a dimmed background and a panel at its end | `base.Sheet{Side: right}` with ui's colors (ui.Sheet sits at the bottom only) |
| tour | antd `Tour`: mask with a cut-out at the target, panel with close button, indicators, Previous/Next | composed: dimming box in an `Overlay` + `Popover` at the step's button (title, text, two buttons) | composed: dimming view over the page + a `Popover` on each button, open for its step (title, text, two buttons) |
| protable | antd `ProTable`: query form, toolbar with 4 icon actions, table, pager (the toolbar and pager are ProTable's defaults: heavier by nature) | composed: `Form`/`Field` + `Reset`, `Query` + `Table` | composed: `FormField` + buttons + `DataTable` |
| segmented | antd `Segmented` | `Segmented` | `Tabs[int]` in its default segmented look, no panel |
| rate | antd `Rate` | `Rating` | `Rating` |
| tags | antd `Tag` | `Badge` (a pill: MyGo's tag-like component) | `Badge` (likewise) |
| badges | antd `Badge` with `Count` on a `Div` square (red, antd) | `Badge` positioned at the square's corner (MyGo's gray pill) | `Badge` positioned at the corner |
| cards | antd `Card` (header with divider, body) | composed: bordered rounded column, bold title | composed likewise |
| statistics | antd `Statistic` (title, value with precision 2) | composed: muted title + 24 px value | composed likewise |

## Product findings (recorded, not changed)

- **PF1** (R36, from Task 10): no public API scrolls an open `Select` or
  a `Table` from code.
- **PF2**: `TreeSelect.ListHeight` applies only to the virtual dropdown;
  the default dropdown keeps antd's 256 px (`layout.MaxH(256)`,
  ui/entry_treeselect.go), so `ListHeight(480)` had no effect. The scene
  keeps the tree to 8 rows instead of changing the component.

# Component mapping (Task 11)

Every non-Skip entry of Tango's first-layer fixture registry
(`registerPerf` in ui/perf_fx_*_test.go: each `Name` and each `Covers`
entry, as `perfFixtures()` returns them at d414712) has a row: the
scene it is compared in and why. The registry holds 224 entries: 198
fixtures, 19 `Covers` entries and 7 `Skip` fixtures (`HoverDelay`, `Key`,
`Prop`, `ScreenBreakpoint`, `SetHoverOpen`, `State`, `UseApp`: helpers that
draw nothing, which have no row). Of the 217 rows:

| Kind | Rows | Meaning |
|---|---|---|
| own scene | 40 | the scene measures this component itself (16 from the base and Task 10 scenes, 24 from Task 11) |
| grouped | 161 | compared through the nearest scene, for the reason given |
| 无对手，只看硬预算 | 16 | no comparison: the opponents have nothing equivalent, or the entry is a context or page shell that draws nothing of its own; only Tango's hard budgets apply |

The MyGo and NSGo columns say what the scene's program draws (for a
grouped row: the scene's, with the opponent's own counterpart in the
reason where it has one, though no scene measures it).

| Tango entry | Scene | MyGo | NSGo | Why |
|---|---|---|---|---|
| Addon | 归入 inputs | TextInput | Input (AppKit field) | text decoration beside an input |
| Affix | 归入 list | List + TrackScroll | List + Ref.ScrollTo | pins content while its container scrolls; the list scene is the scrolling scene |
| Alert | 归入 cards | composed: bordered Column | composed: bordered View | a static bordered box of icon and text |
| Anchor | 归入 menu | Sidebar + SidebarItem | Sidebar | vertical links with a current item, as the menu's selection |
| AnchorSection (in Anchor) | 归入 menu | Sidebar + SidebarItem | Sidebar | the sections Anchor tracks |
| App | 无对手，只看硬预算 | — | — | application root that provides message and notification context; draws nothing of its own |
| Approval | 归入 cards | composed: bordered Column | composed: bordered View | a card of texts and two buttons (Ant Design X) |
| AspectRatio | 归入 grid | Row/Column of Text in boxes | View/Text over one signal | a layout box; the grid scene lays out and redraws 600 boxes a frame |
| AutoComplete | 归入 select | Select (closed) + Popover of a List | Select (closed) as a HoverCard trigger + List | an input with an open suggestion list (MyGo Autocomplete, NSGo Combobox) |
| Avatar | 归入 badges | Badge on a Box corner | Badge on a View corner | a small round box with a letter, as the badges scene's squares |
| AvatarGroup | 归入 badges | Badge on a Box corner | Badge on a View corner | a row of avatars with a count |
| BackTop | 归入 buttons | Button | Button (core.Dynamic) | a floating button |
| Badge | badges | Badge on a Box corner | Badge on a View corner | the scene is this component |
| BadgeRibbon | 归入 badges | Badge on a Box corner | Badge on a View corner | a corner label on a box |
| BorderBeam | 无对手，只看硬预算 | — | — | an animated light running round a border; neither MyGo nor NSGo has such an effect, and a shader of one's own is not a component counterpart |
| Breadcrumb | 归入 menu | Sidebar + SidebarItem | Sidebar | a row of links with a current item (MyGo Breadcrumbs, NSGo Breadcrumbs) |
| BreadcrumbItems | 归入 menu | Sidebar + SidebarItem | Sidebar | Breadcrumb's items |
| Bubble | 归入 cards | composed: bordered Column | composed: bordered View | a rounded box of text (Ant Design X chat) |
| BubbleList | 归入 list | List + TrackScroll | List + Ref.ScrollTo | a virtual list of chat bubbles |
| Button | buttons | Button | Button (core.Dynamic) | the scene is this component |
| ButtonGroup | 归入 buttons | Button | Button (core.Dynamic) | buttons joined in a row |
| Calendar | calendar | Calendar | Calendar | the scene is this component |
| Card | cards | composed: bordered Column | composed: bordered View | the scene is this component |
| CardAction (in Card) | 归入 cards | composed: bordered Column | composed: bordered View | a card's action row |
| CardMeta (in Card) | 归入 cards | composed: bordered Column | composed: bordered View | a card's avatar, title and text |
| Carousel | carousel | composed: slide Column + dot Boxes | Carousel | the scene is this component |
| Cascader | cascader | composed: Combobox + Popover of 3 Lists | composed: Combobox as a HoverCard trigger + 3 Lists | the scene is this component |
| CascaderPanel | 归入 cascader | composed: Combobox + Popover of 3 Lists | composed: Combobox as a HoverCard trigger + 3 Lists | the cascader's columns without the field |
| CheckableTag | 归入 tags | Badge (a pill) | Badge (a pill) | a tag that toggles |
| CheckAll | 归入 checkboxes | Checkbox | Checkbox | a check box over a group of check boxes |
| Checkbox | checkboxes | Checkbox | Checkbox | the scene is this component |
| CheckboxGroup | 归入 checkboxes | Checkbox | Checkbox | check boxes from options |
| CheckCard | 归入 checkboxes | Checkbox | Checkbox | a card that toggles as a check box |
| CheckCardGroup | 归入 checkboxes | Checkbox | Checkbox | a group of CheckCards |
| CloseContextMenu (in ContextMenuAtWith) | 归入 select | Select (closed) + Popover of a List | Select (closed) as a HoverCard trigger + List | closes the context menu |
| CodeBlock | 归入 textarea | TextArea | Input{Multiline} (AppKit) | multi-line monospace text with colored spans; the nearest common scene is the text area (MyGo and NSGo have code editors, a heavier widget) |
| Col (in GridRow) | 归入 grid | Row/Column of Text in boxes | View/Text over one signal | a GridRow column |
| Collapse | collapse | Accordion + AccordionItem | Accordion | accordion mode |
| CollapsibleSider | 归入 menu | Sidebar + SidebarItem | Sidebar | a sider holding a menu |
| ColorPicker | 归入 colorpicker | ColorPicker | ColorPicker | the trigger of the color panel |
| ColorPickerGradient | 归入 colorpicker | ColorPicker | ColorPicker | the color panel with gradient stops |
| ColorPickerPanel | colorpicker | ColorPicker | ColorPicker | the scene is this component |
| ColorPickerPanelGradient | 归入 colorpicker | ColorPicker | ColorPicker | the gradient color panel |
| Column (in Row) | 归入 grid | Row/Column of Text in boxes | View/Text over one signal | a column layout |
| Command | 归入 select | Select (closed) + Popover of a List | Select (closed) as a HoverCard trigger + List | a searchable list of commands (NSGo Command; MyGo none) |
| Compact | 归入 buttons | Button | Button (core.Dynamic) | buttons and inputs joined in a row |
| ConfigProvider | 无对手，只看硬预算 | — | — | context that sets theme, size and locale for what it holds; draws nothing of its own |
| Confirm | 归入 dialog | Modal | Dialog | a modal confirm dialog |
| Content (in Layout) | 归入 grid | Row/Column of Text in boxes | View/Text over one signal | Layout part |
| ContextMenuAt | 归入 select | Select (closed) + Popover of a List | Select (closed) as a HoverCard trigger + List | an open popup list (MyGo ContextMenuBase, NSGo ContextMenu) |
| ContextMenuAtWith | 归入 select | Select (closed) + Popover of a List | Select (closed) as a HoverCard trigger + List | ContextMenuAt with options |
| ContextMenuOpen (in ContextMenuAt) | 归入 select | Select (closed) + Popover of a List | Select (closed) as a HoverCard trigger + List | whether a context menu is open |
| Conversations | 归入 list | List + TrackScroll | List + Ref.ScrollTo | a list of conversations with a selected one |
| Countdown | 归入 statistics | composed: muted Text + 24 px Text | composed: muted Text + 24 px Text | a statistic whose number changes |
| Custom | 归入 grid | Row/Column of Text in boxes | View/Text over one signal | a leaf the app measures and paints (MyGo Draw, NSGo canvas); the grid is the drawing scene |
| DatePicker | 归入 datepicker | Calendar | Calendar | the trigger of the date panel |
| DatePickerPanel | datepicker | Calendar | Calendar | the scene is this component |
| DateTimePicker | 归入 datepicker | Calendar | Calendar | date panel with a time column |
| DefaultFooter | 无对手，只看硬预算 | — | — | Ant Design Pro page footer; neither has a page-frame component, and an admin page shell is not a component counterpart |
| Descriptions | descriptions | composed: rows of bordered Text cells | composed: rows of bordered View cells | the scene is this component |
| Disabled | 无对手，只看硬预算 | — | — | runs its content disabled; a context, nothing drawn of its own |
| Div | 归入 grid | Row/Column of Text in boxes | View/Text over one signal | a plain box |
| Divider | 归入 grid | Row/Column of Text in boxes | View/Text over one signal | a line between boxes |
| Dock | 归入 tabs | Tabs | Tabs[T] | IDE-style tab groups (NSGo DockArea; MyGo Split + Tabs) |
| Drawer | drawer | composed: documented Overlay drawer | composed: base.Sheet{Side: right} in ui colors | the scene is this component |
| DrawerForm | 归入 drawer | composed: documented Overlay drawer | composed: base.Sheet{Side: right} in ui colors | a drawer holding a form |
| Dropdown | 归入 select | Select (closed) + Popover of a List | Select (closed) as a HoverCard trigger + List | a button with an open menu list |
| DropZone | 归入 upload | composed: Button + Text and Progress rows | composed: Button + Text and Progress rows | the area files are dropped on |
| EditableProTable | 归入 protable | composed: Form/Field + Buttons + Table | composed: FormField + Buttons + DataTable | ProTable with editable rows |
| Empty | 归入 cards | composed: bordered Column | composed: bordered View | a static illustration and text |
| FieldSet | 归入 form | Form + Field + TextInput | FormSection + FormField | a titled group of form items |
| Flex | 归入 grid | Row/Column of Text in boxes | View/Text over one signal | a flex layout box |
| FloatButton | 归入 buttons | Button | Button (core.Dynamic) | a floating round button |
| FloatButtonGroup | 归入 buttons | Button | Button (core.Dynamic) | floating buttons |
| Footer (in Layout) | 归入 grid | Row/Column of Text in boxes | View/Text over one signal | Layout part |
| FooterToolbar | 归入 buttons | Button | Button (core.Dynamic) | a bar of buttons pinned to the page bottom |
| Form | form | Form + Field + TextInput | FormSection + FormField | the scene is this component |
| Gap (in Row) | 归入 grid | Row/Column of Text in boxes | View/Text over one signal | fixed space |
| GridContent | 无对手，只看硬预算 | — | — | Ant Design Pro content width wrapper; a page shell, not a component counterpart |
| GridRow | 归入 grid | Row/Column of Text in boxes | View/Text over one signal | 24-column grid row |
| Header (in Layout) | 归入 grid | Row/Column of Text in boxes | View/Text over one signal | Layout part |
| HotkeyInput | 归入 inputs | TextInput | Input (AppKit field) | an input that records a key combination |
| Icon | 归入 buttons | Button | Button (core.Dynamic) | a vector glyph, as in the buttons |
| IconButton | 归入 buttons | Button | Button (core.Dynamic) | a button with an icon |
| Image | 归入 cards | composed: bordered Column | composed: bordered View | static content in a box: after the first frame it draws on damage only (MyGo Image, NSGo Image; no scene compares image decoding) |
| InfiniteScroll | 归入 list | List + TrackScroll | List + Ref.ScrollTo | a list that loads more at its end |
| Input | inputs | TextInput | Input (AppKit field) | the scene is this component |
| InputNumber | 归入 inputs | TextInput | Input (AppKit field) | a numeric input (MyGo NumberInput, NSGo NumberInput) |
| InputTag | 归入 inputs | TextInput | Input (AppKit field) | an input holding tags (MyGo TokenField, NSGo TagInput) |
| Item | 归入 form | Form + Field + TextInput | FormSection + FormField | a settings row of title, description and action (NSGo FormRow) |
| ItemGroup | 归入 form | Form + Field + TextInput | FormSection + FormField | a group of Items |
| JSONView | 归入 tree | Outline | Tree | an expandable tree of rows |
| JSONViewSource | 归入 tree | Outline | Tree | JSONView from source text |
| JSONViewValue | 归入 tree | Outline | Tree | JSONView from a value |
| Kbd | 归入 buttons | Button | Button (core.Dynamic) | a key cap: a small bordered box with text |
| Layout | 归入 grid | Row/Column of Text in boxes | View/Text over one signal | page layout of header, sider, content, footer |
| LightFilter | 归入 protable | composed: Form/Field + Buttons + Table | composed: FormField + Buttons + DataTable | a compact filter bar over a table |
| Link | 归入 buttons | Button | Button (core.Dynamic) | a text button (MyGo Link, NSGo Link) |
| List | 归入 list | List + TrackScroll | List + Ref.ScrollTo | rows of items |
| ListActions | 归入 list | List + TrackScroll | List + Ref.ScrollTo | a List row's actions |
| ListItemMeta (in List) | 归入 list | List + TrackScroll | List + Ref.ScrollTo | a List row's avatar, title and text |
| Listy | list | List + TrackScroll | List + Ref.ScrollTo | the list scene scrolls Listy (Virtual, ItemHeight 40) by 8 px a frame |
| Loading | 归入 progress | Progress | Progress | an indicator that moves every frame while shown (MyGo Spinner, NSGo Spinner) |
| LoginForm | 归入 form | Form + Field + TextInput | FormSection + FormField | a form of username, password and submit |
| LoginFormPage | 归入 form | Form + Field + TextInput | FormSection + FormField | LoginForm on a page |
| LogView | 归入 list | List + TrackScroll | List + Ref.ScrollTo | a virtual list of log lines |
| Markdown | 归入 textarea | TextArea | Input{Multiline} (AppKit) | multi-line rich text (MyGo Markdown; NSGo none) |
| MarkdownEditor | 归入 textarea | TextArea | Input{Multiline} (AppKit) | a multi-line editor |
| Masonry | 归入 cards | composed: bordered Column | composed: bordered View | cards in columns of uneven height |
| Mentions | 归入 inputs | TextInput | Input (AppKit field) | an input with @-suggestions |
| Menu | menu | Sidebar + SidebarItem | Sidebar | inline Menu; the opponents' vertical selectable menu is their Sidebar |
| Menubar | 归入 menu | Sidebar + SidebarItem | Sidebar | a row of menus |
| Message | toast | Context.AddToast | Toaster + Toasts | the scene is this component |
| MessageBox | 归入 dialog | Modal | Dialog | a modal message box (Element Plus) |
| MessageDestroy (in Message) | 归入 toast | Context.AddToast | Toaster + Toasts | removes Message toasts; the toast scene's toasts come and go |
| Modal | dialog | Modal | Dialog | the scene is this component |
| ModalForm | 归入 dialog | Modal | Dialog | a modal holding a form |
| MultiDatePicker | 归入 datepicker | Calendar | Calendar | a date panel choosing several days |
| MultiSelect | 归入 select | Select (closed) + Popover of a List | Select (closed) as a HoverCard trigger + List | a select with tags |
| NativeView | 无对手，只看硬预算 | — | — | hosts a platform view: the cost is the platform view's, not the framework's |
| Notification | 归入 toast | Context.AddToast | Toaster + Toasts | a toast with a title |
| NotificationCenter | 归入 list | List + TrackScroll | List + Ref.ScrollTo | a panel listing notifications |
| NotificationDestroy (in Notification) | 归入 toast | Context.AddToast | Toaster + Toasts | removes notifications |
| OpenDialogs (in Confirm) | 归入 dialog | Modal | Dialog | lists the open Confirm dialogs |
| OTP | 归入 inputs | TextInput | Input (AppKit field) | one input per code digit (NSGo OTPInput) |
| Overlay | 归入 tooltip | Popover on a Button | Popover with a Button trigger | an anchored panel opened from code |
| PageContainer | 无对手，只看硬预算 | — | — | Ant Design Pro page frame; a page shell, not a component counterpart |
| PageHeader | 无对手，只看硬预算 | — | — | Ant Design Pro page header with breadcrumb and actions; a page shell |
| Pagination | pagination | composed: Buttons (pagerItems) | Pagination{VisiblePages: 7} | MyGo has none and composes it from buttons |
| Paragraph | 归入 grid | Row/Column of Text in boxes | View/Text over one signal | text; the grid changes 600 texts a frame |
| Popconfirm | 归入 tooltip | Popover on a Button | Popover with a Button trigger | a popover with a question and two buttons |
| Popover | tooltip | Popover on a Button | Popover with a Button trigger | the tooltip scene opens and closes a Popover from code in all three |
| ProCard | 归入 cards | composed: bordered Column | composed: bordered View | a card |
| ProDescriptions | 归入 descriptions | composed: rows of bordered Text cells | composed: rows of bordered View cells | descriptions from columns |
| ProFieldOf | 归入 inputs | TextInput | Input (AppKit field) | a field shown read-only or as an input |
| ProForm | 归入 form | Form + Field + TextInput | FormSection + FormField | a form with submit and reset |
| ProFormFieldSet | 归入 form | Form + Field + TextInput | FormSection + FormField | fields in a row |
| ProFormGroup | 归入 form | Form + Field + TextInput | FormSection + FormField | a titled group of fields |
| ProFormItems | 归入 form | Form + Field + TextInput | FormSection + FormField | form items from a schema |
| Progress | progress | Progress | Progress | the scene is this component |
| ProLayout | 无对手，只看硬预算 | — | — | Ant Design Pro application frame (sider, header, menu, content); a page shell, not a component counterpart |
| ProList | 归入 list | List + TrackScroll | List + Ref.ScrollTo | a list of rows with title, description and actions |
| Prompt | 归入 dialog | Modal | Dialog | a modal with an input |
| Prompts | 归入 cards | composed: bordered Column | composed: bordered View | a row of prompt cards (Ant Design X) |
| ProSettingsProvider | 无对手，只看硬预算 | — | — | applies Pro layout settings as a theme; a context, nothing drawn of its own |
| ProSkeleton | 归入 progress | Progress | Progress | page-sized placeholder shimmer that moves every frame |
| ProTable | protable | composed: Form/Field + Buttons + Table | composed: FormField + Buttons + DataTable | table + query form |
| QRCode | 无对手，只看硬预算 | — | — | neither MyGo nor NSGo has a QR code; composing one means encoding the code, not a component counterpart |
| QueryFilter | 归入 protable | composed: Form/Field + Buttons + Table | composed: FormField + Buttons + DataTable | the query form above a table |
| Radio | 归入 radios | RadioGroup + Radio | RadioGroup + Radio | a single radio |
| RadioGroup | radios | RadioGroup + Radio | RadioGroup + Radio | the scene is this component |
| RangePicker | 归入 datepicker | Calendar | Calendar | a date panel choosing a range |
| Rate | rate | Rating | Rating | the scene is this component |
| Result | 归入 cards | composed: bordered Column | composed: bordered View | a static icon, title, text and buttons |
| RichText | 归入 textarea | TextArea | Input{Multiline} (AppKit) | multi-line text with styled spans |
| RouteContext | 无对手，只看硬预算 | — | — | reads the enclosing ProLayout's state; draws nothing |
| Row | 归入 grid | Row/Column of Text in boxes | View/Text over one signal | a row layout |
| SchemaForm | 归入 form | Form + Field + TextInput | FormSection + FormField | a form built from a schema |
| Scroll | 归入 list | List + TrackScroll | List + Ref.ScrollTo | a scroll container |
| ScrollH | 归入 list | List + TrackScroll | List + Ref.ScrollTo | a horizontal scroll container |
| Search | 归入 inputs | TextInput | Input (AppKit field) | an input with a search button (MyGo SearchField) |
| Segmented | segmented | Segmented | Tabs[T] (segmented look) | the scene is this component |
| Select | select | Select (closed) + Popover of a List | Select (closed) as a HoverCard trigger + List | open Select whose chosen option changes; no scroll (R36) |
| Sender | 归入 textarea | TextArea | Input{Multiline} (AppKit) | a chat message input |
| SettingDrawer | 归入 drawer | composed: documented Overlay drawer | composed: base.Sheet{Side: right} in ui colors | Ant Design Pro settings in a drawer |
| Sider (in Layout) | 归入 grid | Row/Column of Text in boxes | View/Text over one signal | Layout part |
| Skeleton | 归入 progress | Progress | Progress | a placeholder shimmer that moves every frame (NSGo Skeleton; MyGo none) |
| SkeletonAvatar | 归入 progress | Progress | Progress | Skeleton part |
| SkeletonButton | 归入 progress | Progress | Progress | Skeleton part |
| SkeletonImage | 归入 progress | Progress | Progress | Skeleton part |
| SkeletonInput | 归入 progress | Progress | Progress | Skeleton part |
| Slider | sliders | Slider | Slider | the scene is this component |
| SliderRange | 归入 sliders | Slider | Slider | a slider with two handles (MyGo RangeSlider) |
| Space | 归入 grid | Row/Column of Text in boxes | View/Text over one signal | spaced layout |
| Spacer (in Row) | 归入 grid | Row/Column of Text in boxes | View/Text over one signal | empty space in a row |
| Spin | 归入 progress | Progress | Progress | a spinner that moves every frame (MyGo Spinner, NSGo Spinner) |
| Splitter | 归入 grid | Row/Column of Text in boxes | View/Text over one signal | resizable panes (MyGo Split, NSGo Resizable) |
| SplitterPanel (in Splitter) | 归入 grid | Row/Column of Text in boxes | View/Text over one signal | a Splitter pane |
| StackBox | 归入 grid | Row/Column of Text in boxes | View/Text over one signal | overlapping children |
| Statistic | statistics | composed: muted Text + 24 px Text | composed: muted Text + 24 px Text | the scene is this component |
| StatisticCard | 归入 statistics | composed: muted Text + 24 px Text | composed: muted Text + 24 px Text | a statistic in a card |
| StatisticCardGroup | 归入 statistics | composed: muted Text + 24 px Text | composed: muted Text + 24 px Text | statistic cards in a row |
| StatisticTimer | 归入 statistics | composed: muted Text + 24 px Text | composed: muted Text + 24 px Text | a statistic counting time |
| StatusBar | 归入 buttons | Button | Button (core.Dynamic) | a bar of small text buttons |
| StatusBarItem | 归入 buttons | Button | Button (core.Dynamic) | a StatusBar item |
| Steps | steps | composed: circles, Texts, line Boxes | Stepper | the scene is this component |
| StepsForm | 归入 form | Form + Field + TextInput | FormSection + FormField | a form split into steps (with Steps) |
| Switch | switches | Switch | Switch | the scene is this component |
| SwitchValue | 归入 switches | Switch | Switch | a switch bound to a value |
| Table | table | Table | base/datatable DataTable | per-frame cell update in a visible 1000×6 table; no scroll (R36) |
| Tabs | tabs | Tabs | Tabs[T] | the scene is this component |
| Tag | tags | Badge (a pill) | Badge (a pill) | the scene is this component |
| Text | 归入 grid | Row/Column of Text in boxes | View/Text over one signal | text; the grid changes 600 texts a frame, the nearest scene for text (rather than buttons) |
| TextArea | textarea | TextArea | Input{Multiline} (AppKit) | the scene is this component |
| ThoughtChain | 归入 timeline | composed: dot and line Boxes + Texts | composed: dot and line Views + Texts | a vertical chain of steps (Ant Design X) |
| Timeline | timeline | composed: dot and line Boxes + Texts | composed: dot and line Views + Texts | the scene is this component |
| TimePanel | 归入 datepicker | Calendar | Calendar | a time panel |
| TimePicker | 归入 datepicker | Calendar | Calendar | the trigger of the time panel |
| TimeRangePicker | 归入 datepicker | Calendar | Calendar | a time range |
| TimeSelect | 归入 select | Select (closed) + Popover of a List | Select (closed) as a HoverCard trigger + List | a select of times (Element Plus) |
| Title | 归入 grid | Row/Column of Text in boxes | View/Text over one signal | heading text |
| TitleBar | 无对手，只看硬预算 | — | — | window chrome drawn by the app; the opponents' windows keep the system title bar |
| Toggle | 归入 switches | Switch | Switch | a two-state button (MyGo Toggle, NSGo ToggleButton) |
| ToggleGroup | 归入 segmented | Segmented | Tabs[T] (segmented look) | a row of exclusive toggles (MyGo ToggleGroup, NSGo ToggleGroup) |
| ToolCall | 归入 collapse | Accordion + AccordionItem | Accordion | a collapsible tool-call block (Ant Design X) |
| Tooltip | 归入 tooltip | Popover on a Button | Popover with a Button trigger | nearest counterpart: Tango and MyGo tooltips open only on hover or focus, so the scene opens a Popover from code |
| Tour | tour | composed: Overlay mask + Popover | composed: mask View + Popover per Button | popover + mask |
| Transfer | transfer | composed: 2 bordered Columns of Checkboxes + 2 Buttons | composed: 2 bordered Views of Checkboxes + 2 Buttons | the scene is this component |
| Tree | 归入 tree | Outline | Tree | a non-virtual tree; the tree scene uses VirtualTree |
| TreeSelect | treeselect | composed: Select (closed) + Popover of a Tree | composed: Select (closed) as a HoverCard trigger + Tree | the scene is this component |
| Upload | upload | composed: Button + Text and Progress rows | composed: Button + Text and Progress rows | the upload list (files uploading), not the file picker |
| VDivider (in Space) | 归入 grid | Row/Column of Text in boxes | View/Text over one signal | a vertical divider |
| VirtualList | 归入 list | List + TrackScroll | List + Ref.ScrollTo | whole-row scrolling only, so the list scene uses Listy |
| VirtualSelect | 归入 select | Select (closed) + Popover of a List | Select (closed) as a HoverCard trigger + List | a select with a virtual list |
| VirtualTree | tree | Outline | Tree | the scene is this component |
| Watermark | 无对手，只看硬预算 | — | — | neither has a watermark; repeated rotated text over the page is not a component counterpart |
| WindowControls | 无对手，只看硬预算 | — | — | window buttons drawn by the app; the opponents use the system's |
