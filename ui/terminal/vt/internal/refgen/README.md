# refgen

Development tool for `ui/terminal/vt`'s replay test. It feeds the samples
in `../../testdata/replay` through headless xterm.js (`@xterm/headless`
6.0.0 with `@xterm/addon-unicode-graphemes` 0.4.0, Unicode version
`15-graphemes`) and writes what ends up on the screen as a JSON snapshot
next to each sample. `TestReplay` replays the same input through
`vt.Terminal` and compares cell by cell. The snapshots are committed, so
the Go tests never need Node.

Not part of the product; nothing imports it.

## Usage

```sh
cd ui/terminal/vt/internal/refgen
npm ci
./samples.sh                                   # rewrite the hand-made samples
node refgen.mjs ../../testdata/replay/*.vt ../../testdata/replay/*.ops
cd ../.. && go test -run TestReplay -v .
```

For each `x.vt` or `x.ops`, refgen writes `x.json`.

## Samples

- `x.vt` — raw bytes, written to the terminal in one call.
- `x.ops` — a JSON array of steps, run in order:
  `{"write": "text"}` writes the UTF-8 text (use `\u001b` for ESC) and
  `{"resize": [cols, rows]}` resizes. Use it for anything that needs a
  resize between writes.
- `x.size` — optional, `COLSxROWS` (for example `10x4`); the default is
  `80x24`.

The Go test runs each sample twice, once with whole writes and once in
7-byte chunks, so sequences and UTF-8 characters split across writes are
covered too.

Recorded samples came from real programs on macOS through
`script -q FILE CMD </dev/null`, with the leading `^D` echo removed and
home paths, user and host names replaced by fixed strings
(`/home/user`, `user`, `host`):

| Sample | Command |
|---|---|
| `ls_color.vt` | `CLICOLOR_FORCE=1 ls -G -la /usr/bin \| head -50` |
| `git_log.vt` | `git --no-pager log --color=always --oneline --decorate -30` (this repository) |
| `go_build.vt` | `GOCACHE=$(mktemp -d) go build -v ./ui/...` |

The others are written by `samples.sh`. Keep every file under 200 KB
(`TestReplaySamplesSmall` checks).

## Snapshot format

```json
{"cols":80,"rows":24,"cursor":[x,y],"lines":[{"text":"...","cells":[[x,n,fg,bg,attrs]],"wide":[x],"chars":[[x,"é"]],"wrapped":true}],"replies":"..."}
```

- `cursor` is xterm.js's buffer position of the active area; `x` equals
  `cols` while a wrap is pending (the Go test maps `Terminal.pending` to
  that).
- `lines` are the active area's rows (no scrollback).
  - `text`: the row's characters from `translateToString(true)`: right
    halves of wide characters skipped, never-written cells as spaces,
    trailing never-written cells trimmed (written spaces are kept).
  - `cells`: runs of cells in a non-default style, `[x, count, fg, bg,
    attrs]`. `fg`/`bg` use the `vt.Color` encoding (kind in the high byte:
    0 default, 1 indexed, 2 RGB; index or 0xRRGGBB below) and `attrs` the
    `vt.Attr` bits (bold 1, faint 2, italic 4, underline 8 of any style,
    blink 16, inverse 32, invisible 64, strike 128). Overline has no
    `vt.Attr` bit yet and is not exported.
  - `wide`: columns holding a wide (two-cell) character.
  - `chars`: cells holding anything but one printable ASCII character, as
    `[x, "text"]`, so the column of every code point is known.
  - `wrapped`: the row continues the one above it (xterm.js `isWrapped`;
    `Line.Wrapped` of the row above in Go).
  - Fields with empty values are omitted.
- `replies`: everything the terminal sent back (`onData`), such as device
  attribute and cursor position reports; omitted when empty.

## Differences

Places where Tango deliberately differs from xterm.js are listed in
`../../testdata/replay/DIFFERENCES.md`; the samples avoid them so the
test stays exact. When a new sample disagrees, fix the Go code if xterm.js
is right, otherwise add an entry there and keep the case out of the
sample (or cover it with a unit test).
