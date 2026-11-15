# Terminal workspace

A complete desktop application example built with Tango's own terminal view and local PTYs. Each tab owns independent shell sessions and a split layout. The smaller [`../terminal`](../terminal) example remains useful for learning the component and running replay/latency benchmarks.

## Run

From the repository root:

```sh
go run ./examples/terminal-workspace
go run ./examples/terminal-workspace -light
go run ./examples/terminal-workspace -dir "$HOME"
go run ./examples/terminal-workspace -shell /bin/zsh -- -f
go run ./examples/terminal-workspace -renderer cpu
```

`-shell` takes an executable path, not a shell command string. Arguments after `--` are passed as separate arguments to that executable. Without `-shell` the platform's default shell is used. New tabs and splits use the same shell and initial `-dir`; they do not inherit a running shell's current directory.

Build a release application bundle for the current platform:

```sh
tan build -C examples/terminal-workspace
# Apple Silicon Mac specifically:
tan build -C examples/terminal-workspace -platform darwin/arm64 -o /tmp/tango-terminal-workspace
```

For an ordinary executable: `go build -o /tmp/tango-terminal-workspace-bin ./examples/terminal-workspace`.

## Try it

1. Run `echo "$SHELL"` or `git status` in the first terminal.
2. Use **New tab**, run another command, and switch back. The original process, screen and scrollback remain.
3. Use **Split right** or **Split down**. Click a pane to select it; each pane has its own shell and PID.
4. Drag a divider, then run `stty size` on macOS/Linux. Only visible panes resize; hidden tabs keep their previous PTY dimensions until shown again.
5. Toggle **Light theme** / **Dark theme**. Terminal text and application controls follow the same theme.
6. Run `exit` in a pane. Its last output remains visible with an **exited** status. **Close pane** removes it and collapses its split. Closing the last pane closes that tab; closing the last tab shows **New terminal**.

There are at most 12 tabs and 4 panes in each tab. Tab close buttons end all the sessions in that tab. Closing the window closes all sessions and exits the example. Launch errors appear above the tabs; existing sessions are kept and a new launch can be retried.

## Shortcuts

| Action | macOS | Windows / Linux |
| --- | --- | --- |
| New tab | Cmd+T | Ctrl+Shift+T |
| Next / previous tab | Ctrl+PageDown / Ctrl+PageUp | Ctrl+PageDown / Ctrl+PageUp |
| Split right | Cmd+D | Ctrl+Shift+D |
| Split down | Cmd+Alt+D | Ctrl+Alt+D |
| Close active pane | Cmd+Shift+W | Ctrl+Shift+W |
| Next pane | Ctrl+Shift+Right | Ctrl+Shift+Right |
| Larger / smaller font | Cmd+= / Cmd+- | Ctrl+= / Ctrl+- |
| Reset font | Cmd+0 | Ctrl+0 |

Application shortcuts use `uihost.ShortcutWith`, which suppresses shortcuts during IME composition. Normal typing, Tab, Escape, shell Ctrl+C/Ctrl+D, selection, scrollback, copy and paste are handled by the terminal component. On macOS copying/pasting is Cmd+C/Cmd+V; on Windows/Linux it is Ctrl+Shift+C/Ctrl+Shift+V. Font size is bounded to 10–24 logical pixels.

## Structure

- `workspace.go`: application-owned sessions, bounded tabs, split tree/ratios and cleanup. View creation never creates or restarts a shell. Layout generations prevent moved sessions from recovering a stale view-size cache after split/collapse; application-owned divider ratios survive hidden-widget state eviction.
- `page.go`: keyed tab/pane UI, focus transitions, theme/error/empty/exited states and size/status display. Only the selected tab is rendered; other terminal read loops continue.
- `main.go`: local PTY factory, shell options, native window and application shortcuts.
- `workspace_test.go` / `page_test.go`: session ownership and headless interaction tests using the real terminal view/emulator.
- `e2e_test.go`: opt-in macOS acceptance with real shells and the existing Tango automation harness.

No dependencies or shared UI packages were added. PTYs are available on desktop platforms; unsupported platforms show the real launch error. This example does not add session persistence or SSH connections. Terminal protocol support is that of the existing **M1** component: ordinary shell use is supported; full-screen application compatibility, reflow, search and advanced protocols remain the component's M2/M3 work. See [`../../docs/specs/terminal-m1-report.md`](../../docs/specs/terminal-m1-report.md).

## Verify

```sh
go test ./examples/terminal-workspace ./ui/terminal/... ./sys/pty
go test -race ./examples/terminal-workspace
TANGO_E2E=1 E2E_SHOTS=/tmp/tango-terminal-workspace-shots \
  go test ./examples/terminal-workspace -run TestWorkspaceNative -v -count=1
```

Native acceptance uses `/bin/zsh -f` on macOS. It verifies distinct shell PIDs, three tab-switch cycles without shell restart or PTY size changes, both split directions, drag resizing, retained output after shell exit, and the final empty state. `E2E_SHOTS` saves dark/light screenshots. Headless tests additionally check failed-launch rollback, tab/pane limits, cleanup, narrow-window layout, background output and queued input isolation.
