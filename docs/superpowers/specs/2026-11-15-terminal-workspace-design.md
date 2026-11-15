# Tango terminal workspace example

The user approved a complete example with real local shells, multiple tabs and split panes. Keep the existing `examples/terminal` as the small component/benchmark example; add `examples/terminal-workspace` as an application example. Reuse Tango's terminal emulator, PTY, Tabs, Splitter and theme components, with no new dependencies and no changes to shared UI components.

## Approaches

- Extend the existing single-terminal example: fewer entry points, but mixes session management with its latency/replay benchmark code.
- Add an independent workspace example (chosen): demonstrates application-owned sessions and layout while keeping the original easy to study.
- Embed a web terminal: adds another rendering/runtime path and misses the purpose of demonstrating Tango's own terminal component.

## Behavior and layout

```
Tango Terminal                 New tab | Split right | Split down | Close pane | Light
[ Shell 1 × ] [ Shell 2 × ] [+]
┌ Shell 1 · running ──────────┬ Shell 3 · running ─────────┐
│ real local shell           │ independent real shell    │
│                            │                           │
└────────────────────────────┴───────────────────────────┘
Shell 1 · PID · columns × rows             shortcut hints
```

- Start one tab with one shell. Each tab owns a binary split tree and the focused pane. Limit the example to 12 tabs and 4 panes per tab; disable controls at the limit.
- New tabs and splits create independent PTYs using the user's default shell. Support `-shell PATH`, trailing shell arguments after `--`, `-dir`, `-light`, `-renderer cpu|gpu` and `-quit-after` for demonstrations.
- Split the active pane horizontally or vertically with draggable dividers. Give every tab, split and terminal stable keyed identities. Render only the selected tab; background sessions keep reading output at their previous dimensions.
- Selecting a tab or creating/closing a pane returns keyboard focus to its active terminal. Clicking a terminal changes the active pane. Closing a pane collapses its split; closing a tab closes all its sessions. Closing the last tab leaves a New terminal empty state.
- Exited shells retain their output and show an exited status. A failed launch leaves existing sessions/layout intact and shows an actionable error; the user can retry.
- Application shortcuts: new tab, next/previous tab, split right/down, close active pane, next pane, and font zoom/reset. Use the existing host shortcut registry so application shortcuts do not leak into the shell or IME composition. Keep ordinary terminal input and copy/paste with the terminal component.
- Dark by default with a light/dark toggle; consume existing Ant theme tokens. Provide text labels and accessible names. On unsupported platforms show the PTY error rather than a pretend shell.
- Close every owned terminal on application shutdown. Do not add persistence, remote SSH connections, a new emulator, or promise compatibility beyond the existing terminal component (M1; full-screen protocol work remains M2/M3).

## Validation

State tests cover creation failure, limits, tab cycling, nested split collapse, session identity retention, and exactly-once cleanup. Headless UI tests cover buttons, pane focus, background terminal size preservation and theme switching. A native macOS test runs commands in independent PTYs, switches tabs, splits/closes panes and checks output/session continuity. Build the example and run the repository test suite, recording existing unrelated failures explicitly.
